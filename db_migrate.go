//go:build migrate

/**
 * Copyright 2022 Comcast Cable Communications Management, LLC
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 * SPDX-License-Identifier: Apache-2.0
 */

// Command migrate copies data from the legacy xconf Cassandra schema
// (db_create_tables.cql) to the new multi-tenant sharded schema
// (db_create_tables_xconf.cql).
//
// Old schema tables use (key text, column1 text, value blob) with no tenant/shard
// partitioning. New schema tables add (tenant_id, shard_id) to the partition key
// and an updated timestamp column.
//
// Prerequisites:
//   - Source keyspace must contain old-schema tables (from db_create_tables.cql).
//   - Destination keyspace must already have new-schema tables created
//     (run db_create_tables_xconf.cql first).
//
// Usage:
//
//	migrate [-f <config_file>] \
//	        [-table <OldTableName>] \
//	        [-clear] \
//	        [-dry-run]
//
// Flags:
//
//	-f: Path to xconfwebconfig config file (default: /app/xconfwebconfig/xconfwebconfig.conf).
//	-table: Migrate only the specified legacy table name from tableMappings.
//	-clear: TRUNCATE each destination table before migrating so the run starts on a clean slate.
//	        Combine with -dry-run to log which tables would be truncated without touching the DB.
//	-dry-run: Read/count rows and log planned writes without writing destination rows.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gocql/gocql"
	"github.com/rdkcentral/xconfwebconfig/common"
	"github.com/rdkcentral/xconfwebconfig/db"
)

// migrationKind describes how a legacy table maps to its new-schema counterpart.
type migrationKind string

const (
	defaultConfigFile = "/app/xconfwebconfig/xconfwebconfig.conf"

	// kindSimple: (key, column1, value) → (tenant_id, shard_id, key, value, updated)
	// The column1 clustering key is dropped; if a key has multiple column1 rows the
	// last-written value wins (Cassandra LWT semantics are not needed here).
	kindSimple migrationKind = "simple"

	// kindTwoKey: (key, column1, value) → (tenant_id, shard_id, key, key2, value, updated)
	// Used for GenericXconfNamedList → generic_named_lists where key2 is secondary key.
	kindTwoKey migrationKind = "two-key"

	// kindTagMetadata: TagBucketMetadata (tag_id, bucket_id) → tag_member_metadata (tenant_id, shard_id, tag_id, bucket_id)
	kindTagMetadata migrationKind = "tag-metadata"

	// kindTagMembers: TagMembersBucketed (tag_id, bucket_id, member, created) →
	// tag_members_bucketed (tenant_id, tag_id, bucket_id, member, updated)
	// Uses concurrent workers to handle tags with millions of members.
	kindTagMembers migrationKind = "tag-members"

	// tagMigrateWorkers is the number of concurrent goroutines used to migrate
	// tag member partitions in parallel.
	tagMigrateWorkers = 20

	// tagProgressInterval is the wall-clock cadence for the tag-members heartbeat
	// log that reports how many partitions/members have been processed so far.
	tagProgressInterval = 15 * time.Second

	// tagMemberLogInterval is how often (in members read) a single large partition
	// emits an intra-partition progress line, so a multi-million-member partition
	// does not appear hung while one worker streams through it.
	tagMemberLogInterval = 100_000

	// tagMemberBatchSize is the number of member rows grouped into one single-partition
	// unlogged batch. Kept well under Cassandra's batch_size_fail_threshold_in_kb (50KB
	// default): each row is ~70-120 bytes (tenant_id, tag_id, bucket_id, member, timestamp),
	// so 300 rows is ~30-36KB. The app's per-call cap is MaxBatchSizeV2 = 5000, but a
	// smaller batch keeps us safely below the byte threshold.
	tagMemberBatchSize = 300
)

// tableMapping describes one old→new migration unit.
type tableMapping struct {
	oldName string
	newName string
	kind    migrationKind
}

// tableMappings is the complete list of migrations performed by this tool.
// Tables omitted here (Tag, Logs2) have no new-schema equivalent.
var tableMappings = []tableMapping{
	// Simple entity tables -------------------------------------------------------
	{"DcmRule", db.TABLE_DCM_RULES, kindSimple},
	{"DeviceSettings2", db.TABLE_DEVICE_SETTINGS, kindSimple},
	{"Environment", db.TABLE_ENVIRONMENTS, kindSimple},
	{"FeatureControlRule2", db.TABLE_FEATURE_CONTROL_RULES, kindSimple},
	{"FirmwareConfig", db.TABLE_FIRMWARE_CONFIGS, kindSimple},
	{"FirmwareRule4", db.TABLE_FIRMWARE_RULES, kindSimple},
	{"FirmwareRuleTemplate", db.TABLE_FIRMWARE_RULE_TEMPLATES, kindSimple},
	{"LogFile", db.TABLE_LOG_FILES, kindSimple},
	{"LogFileList", db.TABLE_LOG_FILE_LISTS, kindSimple},
	{"LogFilesGroups", db.TABLE_LOG_FILE_GROUPS, kindSimple},
	{"LogUploadSettings2", db.TABLE_LOG_UPLOAD_SETTINGS, kindSimple},
	{"Model", db.TABLE_MODELS, kindSimple},
	{"PermanentTelemetry", db.TABLE_PERMANENT_TELEMETRY_PROFILES, kindSimple},
	{"SettingProfiles", db.TABLE_SETTING_PROFILES, kindSimple},
	{"SettingRules", db.TABLE_SETTING_RULES, kindSimple},
	{"SingletonFilterValue", db.TABLE_SINGLETON_FILTER_VALUES, kindSimple},
	{"Telemetry", db.TABLE_TELEMETRY_PROFILES, kindSimple},
	{"TelemetryRules", db.TABLE_TELEMETRY_RULES, kindSimple},
	{"UploadRepository", db.TABLE_UPLOAD_REPOSITORIES, kindSimple},
	{"VodSettings2", db.TABLE_VOD_SETTINGS, kindSimple},
	{"XconfApprovedChange", db.TABLE_TELEMETRY_APPROVED_CHANGES, kindSimple},
	{"XconfChange", db.TABLE_TELEMETRY_CHANGES, kindSimple},
	{"XconfFeature", db.TABLE_FEATURES, kindSimple},
	{"TelemetryTwoProfiles", db.TABLE_TELEMETRY_TWO_PROFILES, kindSimple},
	{"TelemetryTwoRules", db.TABLE_TELEMETRY_TWO_RULES, kindSimple},
	{"XconfTelemetryTwoChange", db.TABLE_TELEMETRY_TWO_CHANGES, kindSimple},
	{"XconfApprovedTelemetryTwoChange", db.TABLE_TELEMETRY_APPROVED_TWO_CHANGES, kindSimple},
	{"AppSettings", db.TABLE_APP_SETTINGS, kindSimple},
	{"ApplicationTypes", db.TABLE_APPLICATION_TYPES, kindSimple},

	// Two-key table --------------------------------------------------------------
	// column1 is renamed as key2, the second clustering column in the new schema.
	{"GenericXconfNamedList", db.TABLE_GENERIC_NS_LIST, kindTwoKey},

	// Tag tables -----------------------------------------------------------------
	{"TagBucketMetadata", "tag_member_metadata", kindTagMetadata},
	{"TagMembersBucketed", "tag_members_bucketed", kindTagMembers},
}

func main() {
	configFile := flag.String("f", defaultConfigFile, "config file")
	tableName := flag.String("table", "", "Migrate only this old table name; omit to migrate all tables")
	clear := flag.Bool("clear", false, "TRUNCATE destination tables before migrating (allows re-running on existing data)")
	dryRun := flag.Bool("dry-run", false, "Count rows without writing to the destination")
	flag.Parse()

	// read new hocon config
	sc, err := common.NewServerConfig(*configFile)
	if err != nil {
		log.Fatalf("ERROR reading config file: %v", err)
	}

	dcc := db.DefaultCassandraConnection{Connection_type: "local"}
	dbClient, err := dcc.NewCassandraClient(sc.Config, false)
	if err != nil {
		log.Fatalf("ERROR cassandra db init error: %v", err)
	}
	defer dbClient.Close()

	// Select which tables to migrate.
	mappings := tableMappings
	if *tableName != "" {
		mappings = nil
		for _, m := range tableMappings {
			if m.oldName == *tableName {
				mappings = append(mappings, m)
			}
		}
		if len(mappings) == 0 {
			log.Fatalf("unknown old table %q\nKnown tables: %s", *tableName, knownOldNames())
		}
	}

	if *dryRun {
		log.Println("Dry run — no rows will be written.")
	}
	if *clear {
		log.Println("Clear mode — destination tables will be truncated before migration.")
	}

	// Validate all destination tables exist before starting migration
	log.Println("Validating destination tables...")
	dstKeyspace := dbClient.Keyspace
	if err := validateDestinationTables(dbClient.Session, dstKeyspace, mappings); err != nil {
		log.Fatalf("ERROR: %v", err)
	}
	log.Printf("All %d destination table(s) validated in keyspace %q.", len(mappings), dstKeyspace)

	// Optionally truncate destination tables so migration starts from a clean state.
	if *clear {
		if err := clearDestinationTables(dbClient.Session, dstKeyspace, mappings, *dryRun); err != nil {
			log.Fatalf("ERROR deleting data from destination tables: %v", err)
		}
	}

	tenantID := db.GetDefaultTenantId()
	srcKeyspace := dbClient.GetLogKeyspace() // old tables are in ApplicationsDiscoveryDataService keyspace

	var totalRows, totalErrors int
	for _, m := range mappings {
		rows, migrateErr := runMigration(dbClient.Session, m, srcKeyspace, tenantID, *dryRun)
		totalRows += rows
		if migrateErr != nil {
			log.Printf("ERROR %q → %q: %v", m.oldName, m.newName, migrateErr)
			totalErrors++
		}
	}

	action := "Migrated"
	if *dryRun {
		action = "Would migrate"
	}
	log.Printf("%s %d rows across %d table(s) with %d error(s).",
		action, totalRows, len(mappings), totalErrors)

	if totalErrors > 0 {
		os.Exit(1)
	}
}

// clearDestinationTables truncates every destination table listed in mappings so that
// the migration can be re-run on a clean database.  When dryRun is true the TRUNCATE
// statements are logged but not executed.
func clearDestinationTables(session *gocql.Session, keyspace string, mappings []tableMapping, dryRun bool) error {
	for _, m := range mappings {
		if dryRun {
			log.Printf("  dry-run: would TRUNCATE %q.%q", keyspace, m.newName)
			continue
		}
		log.Printf("  TRUNCATE %q.%q ...", keyspace, m.newName)
		stmt := fmt.Sprintf(`TRUNCATE "%s"."%s"`, keyspace, m.newName)
		if err := session.Query(stmt).Exec(); err != nil {
			return fmt.Errorf("truncate %q: %w", m.newName, err)
		}
	}
	return nil
}

// validateDestinationTables checks that all destination tables exist before migration.
// Returns error listing any missing tables. Fetches all tables in one query for efficiency.
func validateDestinationTables(session *gocql.Session, keyspace string, mappings []tableMapping) error {
	// Fetch all table names in the keyspace with a single query
	query := `SELECT table_name FROM system_schema.tables WHERE keyspace_name = ?`
	iter := session.Query(query, keyspace).Iter()

	existingTables := make(map[string]bool)
	var tableName string
	for iter.Scan(&tableName) {
		existingTables[tableName] = true
	}
	if err := iter.Close(); err != nil {
		return fmt.Errorf("error fetching tables from keyspace %q: %w", keyspace, err)
	}

	// Check each destination table against the fetched set
	var missingTables []string
	for _, m := range mappings {
		if !existingTables[m.newName] {
			missingTables = append(missingTables, m.newName)
		}
	}

	if len(missingTables) > 0 {
		return fmt.Errorf("missing destination table(s): %s\nRun db_create_tables_xconf.cql first", strings.Join(missingTables, ", "))
	}
	return nil
}

// runMigration dispatches to the correct handler and returns the row count.
func runMigration(session *gocql.Session, m tableMapping, srcKeyspace, tenantID string, dryRun bool) (int, error) {
	switch m.kind {
	case kindSimple:
		return migrateSimple(session, m.oldName, m.newName, srcKeyspace, tenantID, dryRun)
	case kindTwoKey:
		return migrateTwoKey(session, m.oldName, m.newName, srcKeyspace, tenantID, dryRun)
	case kindTagMetadata:
		return migrateTagMetadata(session, m.oldName, m.newName, srcKeyspace, tenantID, dryRun)
	case kindTagMembers:
		return migrateTagMembers(session, m.oldName, m.newName, srcKeyspace, tenantID, dryRun)
	default:
		return 0, fmt.Errorf("unknown migration kind %q", m.kind)
	}
}

// migrateSimple reads every (key, value) row from the legacy (key, column1, value)
// table and writes it to the new (tenant_id, shard_id, key, value, updated) table.
// When a key has multiple column1 rows, each is written to the destination; the last
// write wins (Cassandra upsert semantics ensure idempotency on retries).
func migrateSimple(session *gocql.Session, oldTable, newTable, srcKeyspace, tenantID string, dryRun bool) (int, error) {
	readStmt := fmt.Sprintf(`SELECT key, value FROM "%s"."%s"`, srcKeyspace, oldTable)
	writeStmt := fmt.Sprintf(`INSERT INTO "%s" (tenant_id, shard_id, key, value, updated) VALUES (?, ?, ?, ?, ?)`, newTable)
	now := time.Now()

	iter := session.Query(readStmt).Iter()
	count, writeErrors := 0, 0

	for {
		row := make(map[string]any)
		if !iter.MapScan(row) {
			break
		}
		key, _ := row["key"].(string)
		if key == "" {
			continue
		}
		value, _ := row["value"].([]byte)
		count++

		if dryRun {
			log.Printf("    dry-run: would write key=%q to %q (tenant_id=%s, shard_id=%d)", key, newTable, tenantID, db.GetShardId(key))
		} else {
			if err := session.Query(writeStmt, tenantID, db.GetShardId(key), key, value, now).Exec(); err != nil {
				log.Printf("    warn: write key=%q to %q: %v", key, newTable, err)
				writeErrors++
			}
		}
	}
	if err := iter.Close(); err != nil {
		return count, fmt.Errorf("read %q: %w", oldTable, err)
	}

	logResult(dryRun, oldTable, newTable, count, writeErrors, "")
	return count, nil
}

// migrateTwoKey reads (key, column1, value) rows and writes them with the full
// (tenant_id, shard_id, key, key2, value, updated) primary key, preserving
// the column1 clustering column.  Used for GenericXconfNamedList → generic_named_lists.
func migrateTwoKey(session *gocql.Session, oldTable, newTable, srcKeyspace, tenantID string, dryRun bool) (int, error) {
	readStmt := fmt.Sprintf(`SELECT key, column1, value FROM "%s"."%s"`, srcKeyspace, oldTable)
	writeStmt := fmt.Sprintf(`INSERT INTO "%s" (tenant_id, shard_id, key, key2, value, updated) VALUES (?, ?, ?, ?, ?, ?)`, newTable)
	now := time.Now()

	iter := session.Query(readStmt).Iter()
	count, writeErrors := 0, 0

	for {
		row := make(map[string]any)
		if !iter.MapScan(row) {
			break
		}
		key, _ := row["key"].(string)
		if key == "" {
			continue
		}
		key2, _ := row["column1"].(string)
		value, _ := row["value"].([]byte)
		count++

		if dryRun {
			log.Printf("    dry-run: would write key=%q key2=%q to %q (tenant_id=%s, shard_id=%d)", key, key2, newTable, tenantID, db.GetShardId(key))
		} else {
			if err := session.Query(writeStmt, tenantID, db.GetShardId(key), key, key2, value, now).Exec(); err != nil {
				log.Printf("    warn: write key=%q key2=%q to %q: %v", key, key2, newTable, err)
				writeErrors++
			}
		}
	}
	if err := iter.Close(); err != nil {
		return count, fmt.Errorf("read %q: %w", oldTable, err)
	}

	logResult(dryRun, oldTable, newTable, count, writeErrors, "")
	return count, nil
}

// tagPartition identifies a single legacy TagMembersBucketed partition.
type tagPartition struct {
	tagID    string
	bucketID int
}

// migrateTagMetadata reads all (tag_id, bucket_id) rows from the legacy
// TagBucketMetadata table and writes them to tag_member_metadata with the
// partition key (tenant_id, shard_id) and clustering (tag_id, bucket_id).
func migrateTagMetadata(session *gocql.Session, oldTable, newTable, srcKeyspace, tenantID string, dryRun bool) (int, error) {
	readStmt := fmt.Sprintf(`SELECT tag_id, bucket_id FROM "%s"."%s"`, srcKeyspace, oldTable)
	writeStmt := fmt.Sprintf(`INSERT INTO "%s" (tenant_id, shard_id, tag_id, bucket_id) VALUES (?, ?, ?, ?)`, newTable)

	iter := session.Query(readStmt).Iter()
	count, writeErrors := 0, 0

	for {
		row := make(map[string]any)
		if !iter.MapScan(row) {
			break
		}
		tagID, _ := row["tag_id"].(string)
		if tagID == "" {
			continue
		}
		bucketID, _ := row["bucket_id"].(int)
		count++

		if dryRun {
			log.Printf("    dry-run: would write tag_id=%q bucket_id=%d to %q (shard_id=%d)", tagID, bucketID, newTable, db.GetShardId(tagID))
		} else {
			if err := session.Query(writeStmt, tenantID, db.GetShardId(tagID), tagID, bucketID).Exec(); err != nil {
				log.Printf("    warn: write tag_id=%q bucket_id=%d to %q: %v", tagID, bucketID, newTable, err)
				writeErrors++
			}
		}

		if count%tagMemberLogInterval == 0 {
			log.Printf("  %q: %d metadata row(s) processed so far...", oldTable, count)
		}
	}
	if err := iter.Close(); err != nil {
		return count, fmt.Errorf("read %q: %w", oldTable, err)
	}

	logResult(dryRun, oldTable, newTable, count, writeErrors, "")
	return count, nil
}

// migrateTagMembers reads members from the legacy TagMembersBucketed table using
// concurrent workers. It first enumerates all (tag_id, bucket_id) partitions from
// the source metadata table, then dispatches partition reads across a worker pool.
// Each partition is read sequentially; workers process partitions in parallel.
func migrateTagMembers(session *gocql.Session, oldTable, newTable, srcKeyspace, tenantID string, dryRun bool) (int, error) {
	// Enumerate all partitions from the old metadata table.
	metaStmt := fmt.Sprintf(`SELECT tag_id, bucket_id FROM "%s"."%s"`, srcKeyspace, oldTable)
	metaIter := session.Query(metaStmt).Iter()

	var partitions []tagPartition
	for {
		row := make(map[string]any)
		if !metaIter.MapScan(row) {
			break
		}
		tagID, _ := row["tag_id"].(string)
		if tagID == "" {
			continue
		}
		bucketID, _ := row["bucket_id"].(int)
		partitions = append(partitions, tagPartition{tagID: tagID, bucketID: bucketID})
	}
	if err := metaIter.Close(); err != nil {
		return 0, fmt.Errorf("read TagBucketMetadata for partition list: %w", err)
	}

	log.Printf("  %q: found %d partition(s) to migrate with %d worker(s)", oldTable, len(partitions), tagMigrateWorkers)

	// Feed partitions to workers through a channel.
	partCh := make(chan tagPartition, len(partitions))
	for _, p := range partitions {
		partCh <- p
	}
	close(partCh)

	var totalCount int64
	var totalErrors int64
	var donePartitions int64
	totalPartitions := int64(len(partitions))
	var wg sync.WaitGroup

	writeStmt := fmt.Sprintf(`INSERT INTO "%s" (tenant_id, tag_id, bucket_id, member, updated) VALUES (?, ?, ?, ?, ?)`, newTable)

	// Heartbeat goroutine: periodically report progress so a long-running migration
	// is visibly making progress and is not mistaken for a hang.
	start := time.Now()
	stopHeartbeat := make(chan struct{})
	var hbWg sync.WaitGroup
	hbWg.Add(1)
	go func() {
		defer hbWg.Done()
		ticker := time.NewTicker(tagProgressInterval)
		defer ticker.Stop()
		for {
			select {
			case <-stopHeartbeat:
				return
			case <-ticker.C:
				log.Printf("  %q: progress %d/%d partitions, %d members, %d write error(s), elapsed %s",
					oldTable, atomic.LoadInt64(&donePartitions), totalPartitions,
					atomic.LoadInt64(&totalCount), atomic.LoadInt64(&totalErrors),
					time.Since(start).Round(time.Second))
			}
		}
	}()

	for i := 0; i < tagMigrateWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for p := range partCh {
				rows, errs := migrateTagPartition(session, srcKeyspace, oldTable, writeStmt, tenantID, p, dryRun)
				atomic.AddInt64(&totalCount, int64(rows))
				atomic.AddInt64(&totalErrors, int64(errs))
				atomic.AddInt64(&donePartitions, 1)
			}
		}()
	}
	wg.Wait()
	close(stopHeartbeat)
	hbWg.Wait()
	log.Printf("  %q: all %d partition(s) processed in %s", oldTable, totalPartitions, time.Since(start).Round(time.Second))

	count := int(totalCount)
	writeErrors := int(totalErrors)
	logResult(dryRun, oldTable, newTable, count, writeErrors, fmt.Sprintf(" (%d partitions)", len(partitions)))
	return count, nil
}

// migrateTagPartition reads all members from a single legacy (tag_id, bucket_id) partition
// and writes them to the new table. Members are written in single-partition unlogged batches
// (all rows share the destination partition key tenant_id/tag_id/bucket_id) to cut round
// trips. Returns the number of rows read and write errors.
func migrateTagPartition(session *gocql.Session, srcKeyspace, oldTable, writeStmt, tenantID string, p tagPartition, dryRun bool) (int, int) {
	readStmt := fmt.Sprintf(`SELECT member, created FROM "%s"."%s" WHERE tag_id = ? AND bucket_id = ?`, srcKeyspace, oldTable)
	iter := session.Query(readStmt, p.tagID, p.bucketID).Iter()

	count, writeErrors := 0, 0
	batch := session.NewBatch(gocql.UnloggedBatch)
	for {
		row := make(map[string]any)
		if !iter.MapScan(row) {
			break
		}
		member, _ := row["member"].(string)
		if member == "" {
			continue
		}
		// Convert created (Unix milliseconds) to time.Time
		var updated time.Time
		switch v := row["created"].(type) {
		case int64:
			updated = time.UnixMilli(v)
		case int:
			updated = time.UnixMilli(int64(v))
		default:
			updated = time.Now()
		}
		count++

		// In dry-run we only count rows: a single partition can hold millions of
		// members, so logging one line per member would be prohibitively noisy.
		// A per-partition summary is logged once the partition is fully read.
		if !dryRun {
			batch.Query(writeStmt, tenantID, p.tagID, p.bucketID, member, updated)
			if batch.Size() >= tagMemberBatchSize {
				writeErrors += flushTagBatch(session, batch, p)
				batch = session.NewBatch(gocql.UnloggedBatch)
			}
		}

		// Emit an intra-partition heartbeat for very large partitions so a single
		// worker streaming millions of members is not mistaken for a hang.
		if count%tagMemberLogInterval == 0 {
			log.Printf("    tag_id=%q bucket_id=%d: %d members processed so far...", p.tagID, p.bucketID, count)
		}
	}
	// Flush any members left in a partially-filled batch.
	if !dryRun {
		writeErrors += flushTagBatch(session, batch, p)
	}
	if err := iter.Close(); err != nil {
		log.Printf("    warn: read partition tag_id=%q bucket_id=%d: %v", p.tagID, p.bucketID, err)
	}
	if dryRun {
		log.Printf("    dry-run: would write %d member(s) for tag_id=%q bucket_id=%d", count, p.tagID, p.bucketID)
	}
	return count, writeErrors
}

// flushTagBatch executes a single-partition unlogged batch of member inserts. On success it
// returns 0. If the batch fails, it replays each entry individually so that one bad row does
// not drop the whole batch, and returns the count of rows that still failed.
func flushTagBatch(session *gocql.Session, batch *gocql.Batch, p tagPartition) int {
	if batch.Size() == 0 {
		return 0
	}
	if err := session.ExecuteBatch(batch); err == nil {
		return 0
	} else {
		log.Printf("    warn: batch write tag_id=%q bucket_id=%d (%d rows) failed, retrying individually: %v",
			p.tagID, p.bucketID, batch.Size(), err)
	}

	writeErrors := 0
	for _, entry := range batch.Entries {
		if err := session.Query(entry.Stmt, entry.Args...).Exec(); err != nil {
			log.Printf("    warn: write tag_id=%q bucket_id=%d: %v", p.tagID, p.bucketID, err)
			writeErrors++
		}
	}
	return writeErrors
}
func logResult(dryRun bool, oldTable, newTable string, count, writeErrors int, extra string) {
	verb := "migrated"
	if dryRun {
		verb = "would migrate"
	}
	log.Printf("  %q → %q: %s %d rows, %d write errors%s",
		oldTable, newTable, verb, count, writeErrors, extra)
}

// knownOldNames returns a comma-separated string of all old table names.
func knownOldNames() string {
	names := make([]string, len(tableMappings))
	for i, m := range tableMappings {
		names[i] = m.oldName
	}
	return strings.Join(names, ", ")
}
