/**
 * Copyright 2025 Comcast Cable Communications Management, LLC
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
package dataapi

import (
	"net/http/httptest"
	"testing"

	"github.com/rdkcentral/xconfwebconfig/common"
	xhttp "github.com/rdkcentral/xconfwebconfig/http"
	conversion "github.com/rdkcentral/xconfwebconfig/protobuf"
	"github.com/rdkcentral/xconfwebconfig/util"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

// xacGroupServiceMock is a standalone Group Service test double for the
// Xc.EnableXacGroupService flow. It returns the accountId (xac keyspace) and
// accountProducts (ada keyspace) supplied by the test instead of calling a real
// server, and records how many times each keyspace was queried.
type xacGroupServiceMock struct {
	// xac keyspace response (accountId lookup by MAC)
	accountIdData *conversion.XBOAccount
	accountIdErr  error
	// ada keyspace response (account products lookup by accountId)
	accountProducts map[string]string
	productsErr     error

	getAccountIdCalls       int
	getAccountProductsCalls int
	lastMacRequested        string
	lastAccountIdRequested  string
}

// compile-time interface assertion
var _ xhttp.GroupServiceConnector = (*xacGroupServiceMock)(nil)

func (m *xacGroupServiceMock) GetAccountIdData(mac string, fields log.Fields) (*conversion.XBOAccount, error) {
	m.getAccountIdCalls++
	m.lastMacRequested = mac
	return m.accountIdData, m.accountIdErr
}

func (m *xacGroupServiceMock) GetAccountProductsData(accountId string, fields log.Fields) (map[string]string, error) {
	m.getAccountProductsCalls++
	m.lastAccountIdRequested = accountId
	return m.accountProducts, m.productsErr
}

func (m *xacGroupServiceMock) GroupServiceHost() string        { return "" }
func (m *xacGroupServiceMock) SetGroupServiceHost(host string) {}
func (m *xacGroupServiceMock) GroupPrefix() string             { return "" }
func (m *xacGroupServiceMock) SetGroupPrefix(prefix string)    {}
func (m *xacGroupServiceMock) GetRfcPrecookDetails(cpeMac string, fields log.Fields) (*conversion.XconfDevice, error) {
	return nil, nil
}
func (m *xacGroupServiceMock) GetCpeGroups(cpeMac string, fields log.Fields) ([]string, error) {
	return nil, nil
}
func (m *xacGroupServiceMock) CreateListFromGroupServiceProto(cpeGroup *conversion.CpeGroup) []string {
	return nil
}
func (m *xacGroupServiceMock) GetFeatureTagsHashedItems(name string, fields log.Fields) (map[string]string, error) {
	return nil, nil
}

// newXacFlowAccountProducts returns a representative ada-keyspace payload.
func newXacFlowAccountProducts() map[string]string {
	return map[string]string{
		"Partner":         "comcast",
		"TimeZone":        "America/New_York",
		"CountryCode":     "US",
		"Type":            "residential",
		"State":           "active",
		"AccountProducts": `{"productKey":"productValue"}`,
	}
}

// setupXacFlow wires the dataapi globals (Ws, Xc) and the SAT token manager to
// the supplied connector/config so the context functions run without a real
// server. The returned cleanup restores the previous globals.
func setupXacFlow(connector xhttp.GroupServiceConnector, xc *XconfConfigs) (*xhttp.XconfServer, func()) {
	origWs, origXc := Ws, Xc
	sc, _ := common.NewServerConfigFromText("")
	server := &xhttp.XconfServer{
		ServerConfig:          sc,
		GroupServiceConnector: connector,
	}
	xhttp.InitSatTokenManager(server, true) // test-only mode: no network SAT calls
	WebServerInjection(server, xc)
	cleanup := func() {
		Ws = origWs
		Xc = origXc
	}
	return server, cleanup
}

// baseXacConfig returns a config with only the XAC group service flag toggled;
// every other downstream service (account, tagging, groups, ft) is disabled so
// the tests exercise the XAC keyspace flow in isolation.
func baseXacConfig(enableXac bool) *XconfConfigs {
	return &XconfConfigs{
		EnableXacGroupService:   enableXac,
		AccountTypeModelSet:     util.NewSet(), // empty => applies to all models
		EnableAccountService:    false,
		EnableGroupService:      false,
		EnableTaggingService:    false,
		EnableTaggingServiceRFC: false,
		EnableFtMacTags:         false,
		EnableFtAccountTags:     false,
		EnableFtPartnerTags:     false,
		EnableFtGroups:          false,
		EnableTaggingComparison: false,
		EnablePartnerRouting:    false,
	}
}

func TestAddEstbFirmwareContext_XacGroupServiceFlag(t *testing.T) {
	t.Run("enabled fetches accountId from xac and products from ada keyspace", func(t *testing.T) {
		connector := &xacGroupServiceMock{
			accountIdData: &conversion.XBOAccount{
				AccountId:   "acc-estb-1",
				AccountType: "xboType",
			},
			accountProducts: newXacFlowAccountProducts(),
		}
		_, cleanup := setupXacFlow(connector, baseXacConfig(true))
		defer cleanup()

		req := httptest.NewRequest("GET", "/xconf/swu/stb", nil)
		req.RemoteAddr = "10.0.0.1:12345"
		contextMap := map[string]string{
			common.MODEL:      "TESTMODEL",
			common.ESTB_MAC:   "AA:BB:CC:DD:EE:FF",
			common.ACCOUNT_ID: "unknown",
			common.PARTNER_ID: "unknown",
		}

		err := AddEstbFirmwareContext(Ws, req, contextMap, false, false, log.Fields{})

		assert.NoError(t, err)
		// accountId comes from the xac keyspace
		assert.Equal(t, 1, connector.getAccountIdCalls)
		assert.Equal(t, "acc-estb-1", contextMap[common.ACCOUNT_ID])
		// account products (and enrichment) come from the ada keyspace
		assert.Equal(t, 1, connector.getAccountProductsCalls)
		assert.Equal(t, "acc-estb-1", connector.lastAccountIdRequested)
		assert.Equal(t, "COMCAST", contextMap[common.PARTNER_ID])
		assert.Equal(t, "America/New_York", contextMap[common.TIME_ZONE])
		assert.Equal(t, "US", contextMap[common.COUNTRY_CODE])
		assert.Equal(t, "residential", contextMap[common.ACCOUNT_TYPE])
		assert.Equal(t, "active", contextMap[common.ACCOUNT_STATE])
		assert.Equal(t, "productValue", contextMap["productKey"])
	})

	t.Run("disabled skips the group service entirely", func(t *testing.T) {
		connector := &xacGroupServiceMock{
			accountIdData:   &conversion.XBOAccount{AccountId: "acc-estb-1"},
			accountProducts: newXacFlowAccountProducts(),
		}
		_, cleanup := setupXacFlow(connector, baseXacConfig(false))
		defer cleanup()

		req := httptest.NewRequest("GET", "/xconf/swu/stb", nil)
		req.RemoteAddr = "10.0.0.1:12345"
		contextMap := map[string]string{
			common.MODEL:      "TESTMODEL",
			common.ESTB_MAC:   "AA:BB:CC:DD:EE:FF",
			common.ACCOUNT_ID: "unknown",
			common.PARTNER_ID: "unknown",
		}

		err := AddEstbFirmwareContext(Ws, req, contextMap, false, false, log.Fields{})

		assert.NoError(t, err)
		assert.Equal(t, 0, connector.getAccountIdCalls)
		assert.Equal(t, 0, connector.getAccountProductsCalls)
		assert.Equal(t, "unknown", contextMap[common.ACCOUNT_ID])
		// NormalizeCommonContext uppercases partnerId, so compare case-insensitively
		assert.True(t, util.IsUnknownValue(contextMap[common.PARTNER_ID]))
	})
}

func TestAddFeatureControlContext_XacGroupServiceFlag(t *testing.T) {
	t.Run("enabled fetches products from ada keyspace using request accountId", func(t *testing.T) {
		connector := &xacGroupServiceMock{
			accountProducts: newXacFlowAccountProducts(),
		}
		server, cleanup := setupXacFlow(connector, baseXacConfig(true))
		defer cleanup()

		req := httptest.NewRequest("GET", "/featureControl/getSettings", nil)
		req.RemoteAddr = "10.0.0.1:12345"
		contextMap := map[string]string{
			common.MODEL:      "TESTMODEL",
			common.ACCOUNT_ID: "acc-fc-1", // accountId already present in the device request
			common.PARTNER_ID: "unknown",
		}

		_, _, td := AddFeatureControlContext(server, req, contextMap, "", log.Fields{})

		// feature control fetches products directly (no xac accountId lookup)
		assert.Equal(t, 0, connector.getAccountIdCalls)
		assert.Equal(t, 1, connector.getAccountProductsCalls)
		assert.Equal(t, "acc-fc-1", connector.lastAccountIdRequested)
		assert.Equal(t, "COMCAST", contextMap[common.PARTNER_ID])
		assert.Equal(t, "America/New_York", contextMap[common.TIME_ZONE])
		assert.Equal(t, "US", contextMap[common.COUNTRY_CODE])
		assert.Equal(t, "residential", contextMap[common.ACCOUNT_TYPE])
		assert.Equal(t, "productValue", contextMap["productKey"])
		assert.NotEmpty(t, contextMap[common.ACCOUNT_HASH])
		// direct-fetch path does not build AccountServiceData
		assert.Nil(t, td)
	})

	t.Run("disabled skips the group service entirely", func(t *testing.T) {
		connector := &xacGroupServiceMock{
			accountProducts: newXacFlowAccountProducts(),
		}
		server, cleanup := setupXacFlow(connector, baseXacConfig(false))
		defer cleanup()

		req := httptest.NewRequest("GET", "/featureControl/getSettings", nil)
		req.RemoteAddr = "10.0.0.1:12345"
		contextMap := map[string]string{
			common.MODEL:      "TESTMODEL",
			common.ACCOUNT_ID: "acc-fc-1",
			common.PARTNER_ID: "comcast", // already known => no account service fallback
		}

		AddFeatureControlContext(server, req, contextMap, "", log.Fields{})

		assert.Equal(t, 0, connector.getAccountIdCalls)
		assert.Equal(t, 0, connector.getAccountProductsCalls)
		assert.Equal(t, "comcast", contextMap[common.PARTNER_ID])
		_, hasProduct := contextMap["productKey"]
		assert.False(t, hasProduct)
	})
}

func TestAddLogUploaderContext_XacGroupServiceFlag(t *testing.T) {
	t.Run("enabled fetches accountId from xac and products from ada keyspace", func(t *testing.T) {
		connector := &xacGroupServiceMock{
			accountIdData: &conversion.XBOAccount{
				AccountId:   "acc-log-1",
				AccountType: "xboType",
			},
			accountProducts: newXacFlowAccountProducts(),
		}
		server, cleanup := setupXacFlow(connector, baseXacConfig(true))
		defer cleanup()

		req := httptest.NewRequest("GET", "/loguploader/getSettings", nil)
		req.RemoteAddr = "10.0.0.1:12345"
		contextMap := map[string]string{
			common.MODEL:            "TESTMODEL",
			common.ESTB_MAC_ADDRESS: "AA:BB:CC:DD:EE:FF",
			common.ACCOUNT_ID:       "unknown",
			common.PARTNER_ID:       "unknown",
		}

		tags, err := AddLogUploaderContext(server, req, contextMap, false, log.Fields{})

		assert.NoError(t, err)
		assert.Empty(t, tags)
		// accountId comes from the xac keyspace
		assert.Equal(t, 1, connector.getAccountIdCalls)
		assert.Equal(t, "acc-log-1", contextMap[common.ACCOUNT_ID])
		// products (and enrichment) come from the ada keyspace
		assert.Equal(t, 1, connector.getAccountProductsCalls)
		assert.Equal(t, "acc-log-1", connector.lastAccountIdRequested)
		assert.Equal(t, "COMCAST", contextMap[common.PARTNER_ID])
		assert.Equal(t, "America/New_York", contextMap[common.TIME_ZONE])
		assert.Equal(t, "residential", contextMap[common.ACCOUNT_TYPE])
		assert.Equal(t, "productValue", contextMap["productKey"])
	})

	t.Run("disabled skips the group service entirely", func(t *testing.T) {
		connector := &xacGroupServiceMock{
			accountIdData:   &conversion.XBOAccount{AccountId: "acc-log-1"},
			accountProducts: newXacFlowAccountProducts(),
		}
		server, cleanup := setupXacFlow(connector, baseXacConfig(false))
		defer cleanup()

		req := httptest.NewRequest("GET", "/loguploader/getSettings", nil)
		req.RemoteAddr = "10.0.0.1:12345"
		contextMap := map[string]string{
			common.MODEL:            "TESTMODEL",
			common.ESTB_MAC_ADDRESS: "AA:BB:CC:DD:EE:FF",
			common.ACCOUNT_ID:       "unknown",
			common.PARTNER_ID:       "unknown",
		}

		_, err := AddLogUploaderContext(server, req, contextMap, false, log.Fields{})

		assert.NoError(t, err)
		assert.Equal(t, 0, connector.getAccountIdCalls)
		assert.Equal(t, 0, connector.getAccountProductsCalls)
		assert.Equal(t, "unknown", contextMap[common.ACCOUNT_ID])
		// NormalizeCommonContext uppercases partnerId, so compare case-insensitively
		assert.True(t, util.IsUnknownValue(contextMap[common.PARTNER_ID]))
	})
}
