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
	withTenantIdsForTest([]string{"COMCAST", "SKY", "ROGERS"}, func() {
		testCases := []struct {
			partnerId        string
			expectedTenantId string
		}{
			{"comcast", "COMCAST"},
			{"COMCAST", "COMCAST"},
			{"sky", "SKY"},
			{"Rogers", "ROGERS"},
		}
		for _, tc := range testCases {
			resolved := ResolveTenantIdFromPartner(tc.partnerId)
			assert.Equal(t, tc.expectedTenantId, resolved)
		}
	})
}

func TestResolveTenantIdFromPartner_PrefixMatch(t *testing.T) {
	withTenantIdsForTest([]string{"COMCAST"}, func() {
		resolved := ResolveTenantIdFromPartner("comcast-dev")
		assert.Equal(t, "COMCAST", resolved)
	})
}

func TestResolveTenantIdFromPartner_LongestPrefixWins(t *testing.T) {
	withTenantIdsForTest([]string{"COMCAST", "COMCAST-DEV"}, func() {
		resolved := ResolveTenantIdFromPartner("comcast-dev-foo")
		assert.Equal(t, "COMCAST-DEV", resolved)
	})
}

func TestResolveTenantIdFromPartner_UnmappedReturnsDefault(t *testing.T) {
	withTenantIdsForTest([]string{"COMCAST", "SKY"}, func() {
		resolved := ResolveTenantIdFromPartner("cox")
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
	withTenantIdsForTest([]string{"COMCAST"}, func() {
		resolved := ResolveTenantIdFromPartner("  comcast  ")
		assert.Equal(t, "COMCAST", resolved)
	})
}

