package estbfirmware

import (
	"testing"

	"github.com/rdkcentral/xconfwebconfig/common"
	corefw "github.com/rdkcentral/xconfwebconfig/shared/firmware"
	"gotest.tools/assert"
)

func TestFirmwareVersionRegExIsMatched(t *testing.T) {
	e := NewEstbFirmwareRuleBaseDefault()
	action := corefw.NewApplicableAction("RULE", "config-1")
	action.ActivationFirmwareVersions[common.REGULAR_EXPRESSIONS] = []string{`^1\.2\..*$`}

	assert.Assert(t, e.firmwareVersionRegExIsMatched("1.2.3", action))
	assert.Assert(t, !e.firmwareVersionRegExIsMatched("1.3.0", action))
}
