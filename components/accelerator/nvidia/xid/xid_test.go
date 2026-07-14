package xid

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apiv1 "github.com/leptonai/gpud/api/v1"
)

// Status-aware detail selection for NVLink rules
func Test_detailFromNVLinkInfo_StatusSpecific(t *testing.T) {
	// Non-fatal SAW_MVB (errorStatus 0x8) should stay warning
	infoWarn := &ExtractedInfo{
		Xid:         144,
		SubCodeName: "SAW_MVB",
		SubCode:     0,
		Severity:    "Nonfatal",
		Intrinfo:    0x00000021, // matches SAW_MVB pattern
		ErrorStatus: 0x00000008,
	}
	detailWarn, ok := detailFromNVLinkInfo(infoWarn)
	require.True(t, ok)
	assert.Equal(t, apiv1.EventTypeWarning, detailWarn.EventType)

	// Fatal SAW_MVB (errorStatus 0x2) should be fatal
	infoFatal := &ExtractedInfo{
		Xid:         144,
		SubCodeName: "SAW_MVB",
		SubCode:     0,
		Severity:    "Nonfatal", // log says Nonfatal but rule severity is Fatal
		Intrinfo:    0x00000021,
		ErrorStatus: 0x00000002,
	}
	detailFatal, ok := detailFromNVLinkInfo(infoFatal)
	require.True(t, ok)
	assert.Equal(t, apiv1.EventTypeFatal, detailFatal.EventType)
	// Suggested action comes from go-health, independent of the NVLink rule.
	require.NotNil(t, detailFatal.SuggestedActionsByGPUd)
	assert.Equal(t, "workflow_nvlink5_err", detailFatal.SuggestedActionsByGPUd.Description)
	assert.Equal(t, []apiv1.RepairActionType{apiv1.RepairActionTypeHardwareInspection}, detailFatal.SuggestedActionsByGPUd.RepairActions)

	// Unknown errorStatus should fall back to base/subcode detail (warning)
	infoUnknown := &ExtractedInfo{
		Xid:         144,
		SubCodeName: "SAW_MVB",
		SubCode:     0,
		Severity:    "Nonfatal",
		Intrinfo:    0x00000021,
		ErrorStatus: 0xDEADBEEF,
	}
	detailUnknown, ok := detailFromNVLinkInfo(infoUnknown)
	require.True(t, ok)
	assert.Equal(t, apiv1.EventTypeWarning, detailUnknown.EventType)
}
