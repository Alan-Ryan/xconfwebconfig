package http

import (
	"net/http"
	"strings"

	"github.com/rdkcentral/xconfwebconfig/db"
	"github.com/rdkcentral/xconfwebconfig/util"

	"github.com/go-akka/configuration"
)

// PartnerTenantMapping is externally loaded partner-to-tenant mapping.
// Key is normalized partnerId (uppercase), value is tenantId.
var PartnerTenantMapping = map[string]string{}

func GetTenantId(r *http.Request, partnerId string) string {
	// TODO - we can enhance this function in the future to get tenantId from different sources (header, query param, etc.)
	// if needed, but for now we will just return default tenant id since we only have one tenant
	return db.GetDefaultTenantId()
}

func ResolveTenantIdFromPartner(partnerId string) string {
	defaultTenantId := db.GetDefaultTenantId()
	trimmedPartnerId := strings.TrimSpace(partnerId)
	if trimmedPartnerId == "" {
		return defaultTenantId
	}

	if tenantId, found := PartnerTenantMapping[strings.ToUpper(trimmedPartnerId)]; found {
		if strings.TrimSpace(tenantId) != "" {
			return tenantId
		}
	}

	return trimmedPartnerId
}

func LoadPartnerTenantMapping(conf *configuration.Config) map[string]string {
	partnerTenantMapping := map[string]string{}
	for partnerId, tenantId := range util.CreateConfigMapStringString(conf, "xconfwebconfig.xconf.partner_tenant_mapping") {
		trimmedPartnerId := strings.TrimSpace(partnerId)
		trimmedTenantId := strings.TrimSpace(tenantId)
		if trimmedPartnerId == "" || trimmedTenantId == "" {
			continue
		}
		partnerTenantMapping[strings.ToUpper(trimmedPartnerId)] = trimmedTenantId
	}

	return partnerTenantMapping
}
