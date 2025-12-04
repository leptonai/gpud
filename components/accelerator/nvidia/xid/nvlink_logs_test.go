package xid

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apiv1 "github.com/leptonai/gpud/api/v1"
)

// Test_NVLinkLogCoverage verifies severity and formatted user-facing reason for a
// representative set of NVLink5 XIDs (144-150) spanning many units/errorStatus values.
func Test_NVLinkLogCoverage(t *testing.T) {
	cases := []struct {
		name          string
		line          string
		expectedEvent apiv1.EventType
	}{
		// XID 144 SAW_MVB
		{"144_SAW_MVB_nonfatal", "NVRM: Xid (PCI:0000:04:00): 144, SAW_MVB Nonfatal XC0 i0 Link 00 (0x00000001 0x00000001 0x00000000 0x00000000)", apiv1.EventTypeWarning},
		{"144_SAW_MVB_fatal", "NVRM: Xid (PCI:0000:04:00): 144, SAW_MVB Fatal XC0 i0 Link 00 (0x00000001 0x00000002 0x00000000 0x00000000)", apiv1.EventTypeFatal},

		// XID 145 RLW*
		{"145_RLW_CTRL_nonfatal", "NVRM: Xid (PCI:0000:04:00): 145, RLW_CTRL Nonfatal XC0 i0 Link 00 (0x00000003 0x80000000 0x00000000 0x00000000)", apiv1.EventTypeWarning},
		{"145_RLW_REMAP_nonfatal", "NVRM: Xid (PCI:0000:04:00): 145, RLW_REMAP Nonfatal XC0 i0 Link 00 (0x00000004 0x00000001 0x00000000 0x00000000)", apiv1.EventTypeWarning},
		{"145_RLW_REMAP_fatal", "NVRM: Xid (PCI:0000:04:00): 145, RLW_REMAP Fatal XC0 i0 Link 00 (0x00000004 0x00000040 0x00000000 0x00000000)", apiv1.EventTypeFatal},
		{"145_RLW_REMAP_nonfatal_status100", "NVRM: Xid (PCI:0000:04:00): 145, RLW_REMAP Nonfatal XC0 i0 Link 00 (0x00000004 0x00000100 0x00000000 0x00000000)", apiv1.EventTypeWarning},
		{"145_RLW_RSPCOL_nonfatal", "NVRM: Xid (PCI:0000:04:00): 145, RLW_RSPCOL Nonfatal XC0 i0 Link 00 (0x00000005 0x00000001 0x00000000 0x00000000)", apiv1.EventTypeWarning},
		{"145_RLW_RSPCOL_fatal", "NVRM: Xid (PCI:0000:04:00): 145, RLW_RSPCOL Fatal XC0 i0 Link 00 (0x00000005 0x00000002 0x00000000 0x00000000)", apiv1.EventTypeFatal},
		{"145_RLW_RXPIPE_nonfatal", "NVRM: Xid (PCI:0000:04:00): 145, RLW_RXPIPE Nonfatal XC0 i0 Link 00 (0x00000006 0x00000001 0x00000000 0x00000000)", apiv1.EventTypeWarning},
		{"145_RLW_SRC_TRACK_fatal", "NVRM: Xid (PCI:0000:04:00): 145, RLW_SRC_TRACK Fatal XC0 i0 Link 00 (0x00000007 0x00000001 0x00000000 0x00000000)", apiv1.EventTypeFatal},
		{"145_RLW_SRC_TRACK_nonfatal2", "NVRM: Xid (PCI:0000:04:00): 145, RLW_SRC_TRACK Nonfatal XC0 i0 Link 00 (0x00000007 0x00000002 0x00000000 0x00000000)", apiv1.EventTypeWarning},
		{"145_RLW_SRC_TRACK_nonfatal4", "NVRM: Xid (PCI:0000:04:00): 145, RLW_SRC_TRACK Nonfatal XC0 i0 Link 00 (0x00000007 0x00000004 0x00000000 0x00000000)", apiv1.EventTypeWarning},
		{"145_RLW_TAGSTATE_nonfatal", "NVRM: Xid (PCI:0000:04:00): 145, RLW_TAGSTATE Nonfatal XC0 i0 Link 00 (0x00000008 0x00000001 0x00000000 0x00000000)", apiv1.EventTypeWarning},
		{"145_RLW_TAGSTATE_fatal", "NVRM: Xid (PCI:0000:04:00): 145, RLW_TAGSTATE Fatal XC0 i0 Link 00 (0x00000008 0x00000002 0x00000000 0x00000000)", apiv1.EventTypeFatal},

		// XID 146 TLW*
		{"146_TLW_CTRL_nonfatal", "NVRM: Xid (PCI:0000:04:00): 146, TLW_CTRL Nonfatal XC0 i0 Link 00 (0x00000009 0x00000001 0x00000000 0x00000000)", apiv1.EventTypeWarning},
		{"146_TLW_CTRL_fatal", "NVRM: Xid (PCI:0000:04:00): 146, TLW_CTRL Fatal XC0 i0 Link 00 (0x00000009 0x00000004 0x00000000 0x00000000)", apiv1.EventTypeFatal},
		{"146_TLW_RX_PIPE0_nonfatal", "NVRM: Xid (PCI:0000:04:00): 146, TLW_RX/TLW_RX_PIPE0 Nonfatal XC0 i0 Link 00 (0x0000000a 0x00000001 0x00000000 0x00000000)", apiv1.EventTypeWarning},
		{"146_TLW_RX_PIPE0_fatal", "NVRM: Xid (PCI:0000:04:00): 146, TLW_RX/TLW_RX_PIPE0 Fatal XC0 i0 Link 00 (0x0000000a 0x00000004 0x00000000 0x00000000)", apiv1.EventTypeWarning},
		{"146_TLW_RX_PIPE1_nonfatal", "NVRM: Xid (PCI:0000:04:00): 146, TLW_RX/TLW_RX_PIPE1 Nonfatal XC0 i0 Link 00 (0x0000000b 0x00000001 0x00000000 0x00000000)", apiv1.EventTypeWarning},
		{"146_TLW_RX_PIPE1_fatal", "NVRM: Xid (PCI:0000:04:00): 146, TLW_RX/TLW_RX_PIPE1 Fatal XC0 i0 Link 00 (0x0000000b 0x00000004 0x00000000 0x00000000)", apiv1.EventTypeWarning},
		{"146_TLW_TX_PIPE0_nonfatal", "NVRM: Xid (PCI:0000:04:00): 146, TLW_TX/TLW_TX_PIPE0 Nonfatal XC0 i0 Link 00 (0x0000000c 0x00000001 0x00000000 0x00000000)", apiv1.EventTypeWarning},
		{"146_TLW_TX_PIPE0_fatal", "NVRM: Xid (PCI:0000:04:00): 146, TLW_TX/TLW_TX_PIPE0 Fatal XC0 i0 Link 00 (0x0000000c 0x00000004 0x00000000 0x00000000)", apiv1.EventTypeWarning},
		{"146_TLW_TX_PIPE1_nonfatal", "NVRM: Xid (PCI:0000:04:00): 146, TLW_TX/TLW_TX_PIPE1 Nonfatal XC0 i0 Link 00 (0x0000000d 0x00000001 0x00000000 0x00000000)", apiv1.EventTypeWarning},
		{"146_TLW_TX_PIPE1_fatal", "NVRM: Xid (PCI:0000:04:00): 146, TLW_TX/TLW_TX_PIPE1 Fatal XC0 i0 Link 00 (0x0000000d 0x00000004 0x00000000 0x00000000)", apiv1.EventTypeWarning},

		// XID 147 TREX
		{"147_TREX_nonfatal", "NVRM: Xid (PCI:0000:04:00): 147, TREX Nonfatal XC0 i0 Link 00 (0x0000000e 0x00000001 0x00000000 0x00000000)", apiv1.EventTypeWarning},

		// XID 148 NVLPW
		{"148_NVLPW_nonfatal", "NVRM: Xid (PCI:0000:04:00): 148, NVLPW_CTRL/NVLPW Nonfatal XC0 i0 Link 00 (0x0000000f 0x80000000 0x00000000 0x00000000)", apiv1.EventTypeWarning},

		// XID 149 NETIR variants
		{"149_NETIR_INT_fatal", "NVRM: Xid (PCI:0000:04:00): 149, NETIR/NETIR_INT Fatal XC0 i0 Link 00 (0x00000018 0x00000000 0x00000000 0x00000000)", apiv1.EventTypeFatal},
		{"149_NETIR_BER_EVENT_nonfatal", "NVRM: Xid (PCI:0000:04:00): 149, NETIR_BER_EVENT Nonfatal XC0 i0 Link 00 (0x00000013 0x00000000 0x00000000 0x00000000)", apiv1.EventTypeWarning},
		{"149_NETIR_MFDE_EVENT_fatal", "NVRM: Xid (PCI:0000:04:00): 149, NETIR_MFDE_EVENT Fatal XC0 i0 Link 00 (0x00000014 0x00000001 0x00000000 0x00000000)", apiv1.EventTypeFatal},
		{"149_NETIR_MFDE_EVENT_nonfatal", "NVRM: Xid (PCI:0000:04:00): 149, NETIR_MFDE_EVENT Nonfatal XC0 i0 Link 00 (0x00000014 0x00000003 0x00000000 0x00000000)", apiv1.EventTypeWarning},

		// Sample NETIR_LINK_EVT/NETIR_LINK_DOWN fatal variations (multiple intrinfo patterns)
		{"149_NETIR_LINK_EVT_down_00000011", "NVRM: Xid (PCI:0000:04:00): 149, NETIR_LINK_EVT/NETIR_LINK_DOWN Fatal XC0 i0 Link 00 (0x00000011 0x00000000 0x00000000 0x00000000)", apiv1.EventTypeFatal},
		{"149_NETIR_LINK_EVT_down_01000011", "NVRM: Xid (PCI:0000:04:00): 149, NETIR_LINK_EVT/NETIR_LINK_DOWN Fatal XC0 i0 Link 00 (0x01000011 0x00000000 0x00000000 0x00000000)", apiv1.EventTypeFatal},
		{"149_NETIR_LINK_EVT_down_02500011", "NVRM: Xid (PCI:0000:04:00): 149, NETIR_LINK_EVT/NETIR_LINK_DOWN Fatal XC0 i0 Link 00 (0x02500011 0x00000000 0x00000000 0x00000000)", apiv1.EventTypeFatal},

		// XID 150
		{"150_MSE_WATCHDOG_fatal", "NVRM: Xid (PCI:0000:04:00): 150, MSE_WATCHDOG Fatal XC0 i0 Link 00 (0x00000000 0x00000000 0x00000000 0x00000000)", apiv1.EventTypeFatal},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			xidErr := Match(tc.line)
			require.NotNil(t, xidErr)
			require.NotNil(t, xidErr.Detail)

			// Recompute reason with status-aware formatting.
			reason := newXIDErrorReasonWithDetail(xidErr.Xid, xidErr.Detail.SubCode, xidErr.Detail.SubCodeDescription, xidErr.Detail.InvestigatoryHint, xidErr.DeviceUUID, xidErr.Detail.ErrorStatus, nil)

			expectedReason := fmt.Sprintf("XID %d.%d (err status 0x%08x) %s detected on GPU %s",
				xidErr.Xid,
				xidErr.Detail.SubCode,
				xidErr.Detail.ErrorStatus,
				mnemonicForXID(xidErr.Xid),
				xidErr.DeviceUUID,
			)

			assert.Equal(t, tc.expectedEvent, xidErr.Detail.EventType)
			assert.Equal(t, expectedReason, reason)
			// Ensure mnemonic appears only once (no redundant detail duplication).
			assert.Equal(t, 1, strings.Count(reason, mnemonicForXID(xidErr.Xid)))
		})
	}
}
