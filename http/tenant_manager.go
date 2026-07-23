package http

import (
	"strings"

	"github.com/rdkcentral/xconfwebconfig/db"
	"github.com/rdkcentral/xconfwebconfig/util"
)

const tenantCacheKey = "TenantIDList"

// getCachedTenantIds returns all tenant IDs from cache or DB.
// Declared as a variable so tests can substitute a lightweight stub.
var getCachedTenantIds = func() []string {
	dbClient := db.GetDatabaseClient()
	if dbClient == nil {
		return []string{}
	}

	cm := db.GetCacheManager()
	defaultTenantId := db.GetDefaultTenantId()
	cached := cm.ApplicationCacheGet(defaultTenantId, db.TABLE_TENANTS, tenantCacheKey)
	if cached != nil {
		if tenantIds, ok := cached.([]string); ok {
			return tenantIds
		}
	}

	tenants := dbClient.GetAllTenants()
	tenantIds := make([]string, 0, len(tenants))
	for _, t := range tenants {
		if t == nil || util.IsBlank(t.ID) {
			continue
		}
		tenantIds = append(tenantIds, strings.ToUpper(t.ID))
	}

	cm.ApplicationCacheSet(defaultTenantId, db.TABLE_TENANTS, tenantCacheKey, tenantIds)
	return tenantIds
}

// ResolveTenantIdFromPartner resolves a partnerId to a tenantId using the tenant table.
//
// Resolution order:
//  1. Blank, unknown, or noaccount partnerId → default tenant.
//  2. Exact match (case-insensitive) against a tenant ID in the tenant table → that tenant.
//  3. A tenant ID in the table is a prefix of the partnerId → longest matching prefix wins.
//  4. No match → default tenant.
func ResolveTenantIdFromPartner(partnerId string) string {
	defaultTenantId := db.GetDefaultTenantId()
	trimmedPartnerId := strings.TrimSpace(partnerId)
	if util.IsUnknownValue(trimmedPartnerId) || trimmedPartnerId == "" {
		return strings.ToUpper(defaultTenantId)
	}

	normalizedPartnerId := strings.ToUpper(trimmedPartnerId)
	tenantIds := getCachedTenantIds()

	// Exact match
	for _, tid := range tenantIds {
		if tid == normalizedPartnerId {
			return tid
		}
	}

	// Longest-prefix match
	bestMatch := ""
	for _, tid := range tenantIds {
		if strings.HasPrefix(normalizedPartnerId, tid) && len(tid) > len(bestMatch) {
			bestMatch = tid
		}
	}
	if bestMatch != "" {
		return bestMatch
	}

	return strings.ToUpper(defaultTenantId)
}

// InvalidateTenantCache clears the cached tenant ID list, forcing a fresh DB lookup on next call.
// Intended for use in integration tests after inserting or deleting tenants.
func InvalidateTenantCache() {
	cm := db.GetCacheManager()
	defaultTenantId := db.GetDefaultTenantId()
	cm.ApplicationCacheSet(defaultTenantId, db.TABLE_TENANTS, tenantCacheKey, nil)
}
