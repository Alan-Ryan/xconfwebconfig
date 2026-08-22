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
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rdkcentral/xconfwebconfig/common"
	xhttp "github.com/rdkcentral/xconfwebconfig/http"
	conversion "github.com/rdkcentral/xconfwebconfig/protobuf"
	"github.com/rdkcentral/xconfwebconfig/util"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

// These tests exercise the two mutually-exclusive account-enrichment paths that
// every dataapi context builder supports:
//
//   - Xc.EnableXacGroupService : accountId/products come from the Group Service
//     (xac + ada keyspaces) via GroupServiceConnector.
//   - Xc.EnableAccountService  : partner/account come from the legacy Account
//     Service via AccountServiceConnector.
//
// For each entry point (estb_firmware_context.go, feature_control_context.go,
// log_uploader_context.go) we run the same request through both flows with two
// back ends configured to describe the *same* device, then assert that the
// contextMap fields both flows are responsible for writing agree. This guards
// against the two code paths drifting apart in what they publish to contextMap.

const (
	sharedParityMac       = "AA:BB:CC:DD:EE:FF"
	sharedParityPartner   = "comcast" // lowercase from the back end; flows uppercase it
	sharedParityPartnerUC = "COMCAST"
)

// accountServiceFlowMock is a standalone Account Service test double for the
// Xc.EnableAccountService flow. It returns the device/account payloads supplied
// by the test instead of calling a real server and records how it was queried.
type accountServiceFlowMock struct {
	devices    xhttp.AccountServiceDevices
	devicesErr error
	account    xhttp.Account
	accountErr error

	getDevicesCalls int
	getAccountCalls int
	lastMacKey      string
	lastMacValue    string
}

// compile-time interface assertion
var _ xhttp.AccountServiceConnector = (*accountServiceFlowMock)(nil)

func (m *accountServiceFlowMock) AccountServiceHost() string        { return "" }
func (m *accountServiceFlowMock) SetAccountServiceHost(host string) {}

func (m *accountServiceFlowMock) GetAccountData(serviceAccountId string, token string, fields log.Fields) (xhttp.Account, error) {
	m.getAccountCalls++
	return m.account, m.accountErr
}

func (m *accountServiceFlowMock) GetDevices(macKey string, macValue string, token string, fields log.Fields) (xhttp.AccountServiceDevices, error) {
	m.getDevicesCalls++
	m.lastMacKey = macKey
	m.lastMacValue = macValue
	return m.devices, m.devicesErr
}

// setupAccountVsXacFlow wires the dataapi globals (Ws, Xc) and the SAT token
// manager to the supplied connectors/config so the context functions run
// without a real server. The returned cleanup restores the previous globals.
func setupAccountVsXacFlow(grp xhttp.GroupServiceConnector, acct xhttp.AccountServiceConnector, xc *XconfConfigs) (*xhttp.XconfServer, func()) {
	origWs, origXc := Ws, Xc
	sc, _ := common.NewServerConfigFromText("")
	server := &xhttp.XconfServer{
		ServerConfig:            sc,
		GroupServiceConnector:   grp,
		AccountServiceConnector: acct,
	}
	xhttp.InitSatTokenManager(server, true) // test-only mode: no network SAT calls
	WebServerInjection(server, xc)
	cleanup := func() {
		Ws = origWs
		Xc = origXc
	}
	return server, cleanup
}

// accountServiceOnlyConfig mirrors baseXacConfig but selects the legacy Account
// Service path instead of the XAC Group Service path.
func accountServiceOnlyConfig() *XconfConfigs {
	cfg := baseXacConfig(false)
	cfg.EnableAccountService = true
	return cfg
}

// newXacGroupMock builds a Group Service double that resolves the given MAC to
// accountId and returns the representative ada-keyspace products payload.
func newXacGroupMock(accountId string) *xacGroupServiceMock {
	return &xacGroupServiceMock{
		accountIdData:   &conversion.XBOAccount{AccountId: accountId, AccountType: "xboType"},
		accountProducts: newXacFlowAccountProducts(),
	}
}

// newAccountServiceMock builds an Account Service double describing the same
// device as newXacGroupMock (same accountId, same partner, same countryCode).
func newAccountServiceMock(accountId string) *accountServiceFlowMock {
	return &accountServiceFlowMock{
		devices: xhttp.AccountServiceDevices{
			DeviceData: xhttp.DeviceData{
				Partner:           sharedParityPartner,
				ServiceAccountUri: accountId,
			},
		},
		account: xhttp.Account{
			AccountData: xhttp.AccountData{
				AccountAttributes: xhttp.AccountAttributes{CountryCode: "US"},
			},
		},
	}
}

// assertContextParity asserts the two contextMaps agree on the given keys.
func assertContextParity(t *testing.T, xacCtx, acctCtx map[string]string, keys ...string) {
	t.Helper()
	for _, key := range keys {
		assert.Equalf(t, xacCtx[key], acctCtx[key],
			"contextMap[%q] diverged: xac=%q account=%q", key, xacCtx[key], acctCtx[key])
	}
}

// --- estb_firmware_context.go -------------------------------------------------

func TestAddEstbFirmwareContext_AccountServiceVsXacParity(t *testing.T) {
	const accountId = "acc-estb-parity"

	newReq := func() *http.Request {
		req := httptest.NewRequest("GET", "/xconf/swu/stb", nil)
		req.RemoteAddr = "10.0.0.1:12345"
		return req
	}
	newCtx := func() map[string]string {
		return map[string]string{
			common.MODEL:      "TESTMODEL",
			common.ESTB_MAC:   sharedParityMac,
			common.ACCOUNT_ID: "unknown",
			common.PARTNER_ID: "unknown",
		}
	}

	// XAC Group Service flow
	xacGrp := newXacGroupMock(accountId)
	xacAcct := newAccountServiceMock(accountId)
	_, cleanupXac := setupAccountVsXacFlow(xacGrp, xacAcct, baseXacConfig(true))
	xacCtx := newCtx()
	err := AddEstbFirmwareContext(Ws, newReq(), xacCtx, false, false, log.Fields{})
	assert.NoError(t, err)
	cleanupXac()

	// Legacy Account Service flow
	acctGrp := newXacGroupMock(accountId)
	acctAcct := newAccountServiceMock(accountId)
	_, cleanupAcct := setupAccountVsXacFlow(acctGrp, acctAcct, accountServiceOnlyConfig())
	acctCtx := newCtx()
	err = AddEstbFirmwareContext(Ws, newReq(), acctCtx, false, false, log.Fields{})
	assert.NoError(t, err)
	cleanupAcct()

	// each flow used only its own back end
	assert.Equal(t, 1, xacGrp.getAccountIdCalls, "xac flow should query the group service")
	assert.Equal(t, 0, xacAcct.getDevicesCalls, "xac flow should not query the account service")
	assert.Equal(t, 1, acctAcct.getDevicesCalls, "account flow should query the account service")
	assert.Equal(t, 0, acctGrp.getAccountIdCalls, "account flow should not query the group service")
	// AddEstbFirmwareContext's account-service path publishes only the partnerId.
	assert.Equal(t, sharedParityPartnerUC, xacCtx[common.PARTNER_ID])
	assertContextParity(t, xacCtx, acctCtx, common.PARTNER_ID)
}

func TestAddEstbFirmwareContext_AccountServiceFlow_WritesPartnerId(t *testing.T) {
	acct := newAccountServiceMock("acc-estb-1")
	_, cleanup := setupAccountVsXacFlow(newXacGroupMock("acc-estb-1"), acct, accountServiceOnlyConfig())
	defer cleanup()

	req := httptest.NewRequest("GET", "/xconf/swu/stb", nil)
	req.RemoteAddr = "10.0.0.1:12345"
	contextMap := map[string]string{
		common.MODEL:      "TESTMODEL",
		common.ESTB_MAC:   sharedParityMac,
		common.ACCOUNT_ID: "unknown",
		common.PARTNER_ID: "unknown",
	}

	err := AddEstbFirmwareContext(Ws, req, contextMap, false, false, log.Fields{})

	assert.NoError(t, err)
	assert.Equal(t, 1, acct.getDevicesCalls)
	assert.Equal(t, common.HOST_MAC_PARAM, acct.lastMacKey)
	assert.Equal(t, sharedParityMac, acct.lastMacValue)
	assert.Equal(t, sharedParityPartnerUC, contextMap[common.PARTNER_ID])
}

// --- feature_control_context.go ----------------------------------------------

func TestAddFeatureControlContext_AccountServiceVsXacParity(t *testing.T) {
	const accountId = "acc-fc-parity"

	newReq := func() *http.Request {
		req := httptest.NewRequest("GET", "/featureControl/getSettings", nil)
		req.RemoteAddr = "10.0.0.1:12345"
		return req
	}
	newCtx := func() map[string]string {
		return map[string]string{
			common.MODEL:            "TESTMODEL",
			common.ESTB_MAC_ADDRESS: sharedParityMac,
			common.ACCOUNT_ID:       "unknown",
			common.PARTNER_ID:       "unknown",
			common.ACCOUNT_HASH:     "unknown",
		}
	}

	// XAC Group Service flow: products carry partner/countryCode.
	xacGrp := newXacGroupMock(accountId)
	xacAcct := newAccountServiceMock(accountId)
	xacServer, cleanupXac := setupAccountVsXacFlow(xacGrp, xacAcct, baseXacConfig(true))
	xacCtx := newCtx()
	AddFeatureControlContext(xacServer, newReq(), xacCtx, "", log.Fields{})
	cleanupXac()

	// Legacy Account Service flow: partner/accountId from GetDevices, countryCode
	// from GetAccountData (enabled via RfcReturnCountryCode for this model/partner).
	acctCfg := accountServiceOnlyConfig()
	acctCfg.RfcReturnCountryCode = true
	models := util.NewSet()
	models.Add("TESTMODEL")
	partners := util.NewSet()
	partners.Add(sharedParityPartnerUC)
	acctCfg.RfcCountryCodeModelsSet = models
	acctCfg.RfcCountryCodePartnersSet = partners

	acctGrp := newXacGroupMock(accountId)
	acctAcct := newAccountServiceMock(accountId)
	acctServer, cleanupAcct := setupAccountVsXacFlow(acctGrp, acctAcct, acctCfg)
	acctCtx := newCtx()
	AddFeatureControlContext(acctServer, newReq(), acctCtx, "", log.Fields{})
	cleanupAcct()

	// each flow used only its own back end
	assert.Equal(t, 1, xacGrp.getAccountIdCalls, "xac flow should query the group service")
	assert.Equal(t, 0, xacAcct.getDevicesCalls, "xac flow should not query the account service")
	assert.Equal(t, 1, acctAcct.getDevicesCalls, "account flow should query the account service")
	assert.Equal(t, 1, acctAcct.getAccountCalls, "account flow should query account data for country code")
	assert.Equal(t, 0, acctGrp.getAccountIdCalls, "account flow should not query the group service")

	// both flows resolve the same accountId, so the derived hash must match too
	assert.Equal(t, accountId, xacCtx[common.ACCOUNT_ID])
	assert.Equal(t, util.CalculateHash(accountId), xacCtx[common.ACCOUNT_HASH])
	assert.Equal(t, sharedParityPartnerUC, xacCtx[common.PARTNER_ID])
	assert.Equal(t, "US", xacCtx[common.COUNTRY_CODE])

	assertContextParity(t, xacCtx, acctCtx,
		common.ACCOUNT_ID,
		common.PARTNER_ID,
		common.ACCOUNT_HASH,
		common.COUNTRY_CODE,
	)
}

func TestAddFeatureControlContext_AccountServiceFlow_WritesAccountFields(t *testing.T) {
	const accountId = "acc-fc-1"

	cfg := accountServiceOnlyConfig()
	cfg.RfcReturnCountryCode = true
	models := util.NewSet()
	models.Add("TESTMODEL")
	partners := util.NewSet()
	partners.Add(sharedParityPartnerUC)
	cfg.RfcCountryCodeModelsSet = models
	cfg.RfcCountryCodePartnersSet = partners

	acct := newAccountServiceMock(accountId)
	server, cleanup := setupAccountVsXacFlow(newXacGroupMock(accountId), acct, cfg)
	defer cleanup()

	req := httptest.NewRequest("GET", "/featureControl/getSettings", nil)
	req.RemoteAddr = "10.0.0.1:12345"
	contextMap := map[string]string{
		common.MODEL:            "TESTMODEL",
		common.ESTB_MAC_ADDRESS: sharedParityMac,
		common.ACCOUNT_ID:       "unknown",
		common.PARTNER_ID:       "unknown",
		common.ACCOUNT_HASH:     "unknown",
	}

	AddFeatureControlContext(server, req, contextMap, "", log.Fields{})

	assert.Equal(t, 1, acct.getDevicesCalls)
	assert.Equal(t, common.HOST_MAC_PARAM, acct.lastMacKey)
	assert.Equal(t, sharedParityMac, acct.lastMacValue)
	assert.Equal(t, 1, acct.getAccountCalls)
	assert.Equal(t, accountId, contextMap[common.ACCOUNT_ID])
	assert.Equal(t, sharedParityPartnerUC, contextMap[common.PARTNER_ID])
	assert.Equal(t, util.CalculateHash(accountId), contextMap[common.ACCOUNT_HASH])
	assert.Equal(t, "US", contextMap[common.COUNTRY_CODE])
}

// --- log_uploader_context.go -------------------------------------------------

func TestAddLogUploaderContext_AccountServiceVsXacParity(t *testing.T) {
	const accountId = "acc-log-parity"

	newReq := func() *http.Request {
		req := httptest.NewRequest("GET", "/loguploader/getSettings", nil)
		req.RemoteAddr = "10.0.0.1:12345"
		return req
	}
	newCtx := func() map[string]string {
		return map[string]string{
			common.MODEL:            "TESTMODEL",
			common.ESTB_MAC_ADDRESS: sharedParityMac,
			common.ACCOUNT_ID:       "unknown",
			common.PARTNER_ID:       "unknown",
		}
	}

	// XAC Group Service flow
	xacGrp := newXacGroupMock(accountId)
	xacAcct := newAccountServiceMock(accountId)
	xacServer, cleanupXac := setupAccountVsXacFlow(xacGrp, xacAcct, baseXacConfig(true))
	xacCtx := newCtx()
	_, err := AddLogUploaderContext(xacServer, newReq(), xacCtx, false, log.Fields{})
	assert.NoError(t, err)
	cleanupXac()

	// Legacy Account Service flow
	acctGrp := newXacGroupMock(accountId)
	acctAcct := newAccountServiceMock(accountId)
	acctServer, cleanupAcct := setupAccountVsXacFlow(acctGrp, acctAcct, accountServiceOnlyConfig())
	acctCtx := newCtx()
	_, err = AddLogUploaderContext(acctServer, newReq(), acctCtx, false, log.Fields{})
	assert.NoError(t, err)
	cleanupAcct()

	// each flow used only its own back end
	assert.Equal(t, 1, xacGrp.getAccountIdCalls, "xac flow should query the group service")
	assert.Equal(t, 0, xacAcct.getDevicesCalls, "xac flow should not query the account service")
	assert.Equal(t, 1, acctAcct.getDevicesCalls, "account flow should query the account service")
	assert.Equal(t, 0, acctGrp.getAccountIdCalls, "account flow should not query the group service")
	// AddLogUploaderContext's account-service path publishes only the partnerId.
	assert.Equal(t, sharedParityPartnerUC, xacCtx[common.PARTNER_ID])
	assertContextParity(t, xacCtx, acctCtx, common.PARTNER_ID)
}

func TestAddLogUploaderContext_AccountServiceFlow_WritesPartnerId(t *testing.T) {
	acct := newAccountServiceMock("acc-log-1")
	server, cleanup := setupAccountVsXacFlow(newXacGroupMock("acc-log-1"), acct, accountServiceOnlyConfig())
	defer cleanup()

	req := httptest.NewRequest("GET", "/loguploader/getSettings", nil)
	req.RemoteAddr = "10.0.0.1:12345"
	contextMap := map[string]string{
		common.MODEL:            "TESTMODEL",
		common.ESTB_MAC_ADDRESS: sharedParityMac,
		common.ACCOUNT_ID:       "unknown",
		common.PARTNER_ID:       "unknown",
	}

	_, err := AddLogUploaderContext(server, req, contextMap, false, log.Fields{})

	assert.NoError(t, err)
	assert.Equal(t, 1, acct.getDevicesCalls)
	assert.Equal(t, common.HOST_MAC_PARAM, acct.lastMacKey)
	assert.Equal(t, sharedParityMac, acct.lastMacValue)
	assert.Equal(t, sharedParityPartnerUC, contextMap[common.PARTNER_ID])
}
