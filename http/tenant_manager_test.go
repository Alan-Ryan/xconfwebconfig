package http

import (
	"testing"

	"github.com/rdkcentral/xconfwebconfig/db"
	"github.com/stretchr/testify/assert"
)

func withTenantIdsForTest(ids []string, fn func()) {
	orig := getCachedTenantIds
	getCachedTenantIds = func() []string { return ids }
	defer func() { getCachedTenantIds = orig }()
	fn()
}

func TestResolveTenantIdFromPartner_BlankPartnerUsesDefault(t *testing.T) {
	withTenantIdsForTest([]string{}, func() {
		resolved := ResolveTenantIdFromPartner("")
		assert.Equal(t, db.GetDefaultTenantId(), resolved)
	})
}

func TestResolveTenantIdFromPartner_UnknownOrNoaccountUsesDefault(t *testing.T) {
	withTenantIdsForTest([]string{}, func() {
		testCases := []string{"unknown", "Unknown", "noaccount", "NoAccount"}
		for _, partnerId := range testCases {
			resolved := ResolveTenantIdFromPartner(partnerId)
			assert.Equal(t, db.GetDefaultTenantId(), resolved)
		}
	})
}

func TestResolveTenantIdFromPartner_ExactMatchCaseInsensitive(t *testing.T) {
	withTenantIdsForTest([]string{"PARTNER1", "TEST1", "TEST2"}, func() {
		testCases := []struct {
			partnerId        string
			expectedTenantId string
		}{
			{"partner1", "PARTNER1"},
			{"PARTNER1", "PARTNER1"},
			{"test1", "TEST1"},
			{"Test2", "TEST2"},
		}
		for _, tc := range testCases {
			resolved := ResolveTenantIdFromPartner(tc.partnerId)
			assert.Equal(t, tc.expectedTenantId, resolved)
		}
	})
}

func TestResolveTenantIdFromPartner_PrefixMatch(t *testing.T) {
	withTenantIdsForTest([]string{"PARTNER"}, func() {
		resolved := ResolveTenantIdFromPartner("partner-dev")
		assert.Equal(t, "PARTNER", resolved)
	})
}

func TestResolveTenantIdFromPartner_LongestPrefixWins(t *testing.T) {
	withTenantIdsForTest([]string{"PARTNER", "PARTNER-DEV"}, func() {
		resolved := ResolveTenantIdFromPartner("partner-dev-foo")
		assert.Equal(t, "PARTNER-DEV", resolved)
	})
}

func TestResolveTenantIdFromPartner_UnmappedReturnsDefault(t *testing.T) {
	withTenantIdsForTest([]string{"PARTNER", "TEST"}, func() {
		resolved := ResolveTenantIdFromPartner("other")
		assert.Equal(t, db.GetDefaultTenantId(), resolved)
	})
}

func TestResolveTenantIdFromPartner_NeverReturnsEmpty(t *testing.T) {
	withTenantIdsForTest([]string{}, func() {
		resolved := ResolveTenantIdFromPartner("   ")
		assert.NotEmpty(t, resolved)
	})
}

func TestResolveTenantIdFromPartner_WhitespaceIsTrimmed(t *testing.T) {
	withTenantIdsForTest([]string{"PARTNER1"}, func() {
		resolved := ResolveTenantIdFromPartner("  partner1  ")
		assert.Equal(t, "PARTNER1", resolved)
	})
}
