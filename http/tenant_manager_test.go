package http

import (
	"testing"

	"github.com/rdkcentral/xconfwebconfig/common"
	"github.com/rdkcentral/xconfwebconfig/db"
	"github.com/stretchr/testify/assert"
)

func TestLoadPartnerTenantMapping(t *testing.T) {
	sc, err := common.NewServerConfigFromText(`
xconfwebconfig {
  xconf {
    partner_tenant_mapping = {
      "partner1": "tenantA"
      "partner2": "tenantB"
    }
  }
}
`)
	assert.NoError(t, err)

	mapping := LoadPartnerTenantMapping(sc.Config)
	assert.Equal(t, "tenantA", mapping["PARTNER1"])
	assert.Equal(t, "tenantB", mapping["PARTNER2"])
}

func TestResolveTenantIdFromPartner_BlankPartnerUsesDefault(t *testing.T) {
	originalMapping := PartnerTenantMapping
	PartnerTenantMapping = map[string]string{}
	t.Cleanup(func() {
		PartnerTenantMapping = originalMapping
	})

	resolved := ResolveTenantIdFromPartner("")
	assert.Equal(t, db.GetDefaultTenantId(), resolved)
}

func TestResolveTenantIdFromPartner_UnknownOrNoaccountUsesDefault(t *testing.T) {
	originalMapping := PartnerTenantMapping
	PartnerTenantMapping = map[string]string{}
	t.Cleanup(func() {
		PartnerTenantMapping = originalMapping
	})

	testCases := []string{"unknown", "Unknown", "noaccount", "NoAccount"}
	for _, partnerId := range testCases {
		resolved := ResolveTenantIdFromPartner(partnerId)
		assert.Equal(t, db.GetDefaultTenantId(), resolved)
	}
}

func TestResolveTenantIdFromPartner_MappedPartners(t *testing.T) {
	originalMapping := PartnerTenantMapping
	PartnerTenantMapping = map[string]string{
		"PARTNER1": "tenantA",
		"PARTNER2": "tenantB",
		"PARTNER3": "tenantC",
		"PARTNER4": "tenantC",
	}
	t.Cleanup(func() {
		PartnerTenantMapping = originalMapping
	})

	testCases := []struct {
		partnerId        string
		expectedTenantId string
	}{
		{partnerId: "partner1", expectedTenantId: "tenantA"},
		{partnerId: "partner2", expectedTenantId: "tenantB"},
		{partnerId: "partner3", expectedTenantId: "tenantC"},
		{partnerId: "partner4", expectedTenantId: "tenantC"},
	}

	for _, testCase := range testCases {
		resolved := ResolveTenantIdFromPartner(testCase.partnerId)
		assert.Equal(t, testCase.expectedTenantId, resolved)
	}
}

func TestResolveTenantIdFromPartner_UnmappedReturnsPartner(t *testing.T) {
	originalMapping := PartnerTenantMapping
	PartnerTenantMapping = map[string]string{
		"PARTNER1": "tenantA",
		"PARTNER2": "tenantB",
	}
	t.Cleanup(func() {
		PartnerTenantMapping = originalMapping
	})

	resolved := ResolveTenantIdFromPartner("SOMENEWPARTNER")
	assert.Equal(t, "SOMENEWPARTNER", resolved)
}

func TestResolveTenantIdFromPartner_CaseInsensitiveMatch(t *testing.T) {
	originalMapping := PartnerTenantMapping
	PartnerTenantMapping = map[string]string{
		"PARTNER1": "tenantA",
		"PARTNER2": "tenantA",
	}
	t.Cleanup(func() {
		PartnerTenantMapping = originalMapping
	})

	resolved := ResolveTenantIdFromPartner("PartNer2")
	assert.Equal(t, "tenantA", resolved)
}

func TestResolveTenantIdFromPartner_NeverReturnsEmpty(t *testing.T) {
	originalMapping := PartnerTenantMapping
	PartnerTenantMapping = map[string]string{}
	t.Cleanup(func() {
		PartnerTenantMapping = originalMapping
	})

	resolved := ResolveTenantIdFromPartner("   ")
	assert.NotEmpty(t, resolved)
}
