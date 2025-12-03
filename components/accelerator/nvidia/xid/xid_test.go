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
	infoWarn := &XidExtractedInfo{
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
	infoFatal := &XidExtractedInfo{
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
	// Suggested action should come from the rule (RESET_GPU -> RebootSystem)
	require.NotNil(t, detailFatal.SuggestedActionsByGPUd)
	assert.Contains(t, detailFatal.SuggestedActionsByGPUd.RepairActions, apiv1.RepairActionTypeRebootSystem)

	// Unknown errorStatus should fall back to base/subcode detail (warning)
	infoUnknown := &XidExtractedInfo{
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
