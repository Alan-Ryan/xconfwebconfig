package http

import (
	"strings"

	"github.com/rdkcentral/xconfwebconfig/db"
	"github.com/rdkcentral/xconfwebconfig/util"

	"github.com/go-akka/configuration"
)

// PartnerTenantMapping is externally loaded partner-to-tenant mapping.
// Key is normalized partnerId (uppercase), value is tenantId.
var PartnerTenantMapping = map[string]string{}

func ResolveTenantIdFromPartner(partnerId string) string {
	defaultTenantId := db.GetDefaultTenantId()
	trimmedPartnerId := strings.TrimSpace(partnerId)
	// If device sends partnerId=unknown/noaccount or a blank partnerId, use the default tenantId
	if util.IsUnknownValue(trimmedPartnerId) || trimmedPartnerId == "" {
		return strings.ToUpper(defaultTenantId)
	}

	if tenantId, found := PartnerTenantMapping[strings.ToUpper(trimmedPartnerId)]; found {
		if strings.TrimSpace(tenantId) != "" {
			return strings.ToUpper(tenantId)
		}
	}

	return strings.ToUpper(trimmedPartnerId)
}

func LoadPartnerTenantMapping(conf *configuration.Config) map[string]string {
	partnerTenantMapping := map[string]string{}
	for partnerId, tenantId := range util.CreateConfigMapStringString(conf, "xconfwebconfig.xconf.partner_tenant_mapping") {
		trimmedPartnerId := strings.TrimSpace(partnerId)
		trimmedTenantId := strings.TrimSpace(tenantId)
		if trimmedPartnerId == "" || trimmedTenantId == "" {
			continue
		}
		partnerTenantMapping[strings.ToUpper(trimmedPartnerId)] = strings.ToUpper(trimmedTenantId)
	}

	return partnerTenantMapping
}
