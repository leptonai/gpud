package xid

import (
	"testing"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestExtractNVRMXidInfoExtended tests the extended XID parsing for NVLink5 errors (XIDs 144-150)
// with production log lines and examples from the NVIDIA XID error catalog.
// Reference: https://docs.nvidia.com/deploy/xid-errors/analyzing-xid-catalog.html
func TestExtractNVRMXidInfoExtended(t *testing.T) {
	tests := []struct {
		name                string
		logLine             string
		expectedXid         int
		expectedDeviceUUID  string
		expectedSubCode     int
		expectedSubCodeName string
		expectedSeverity    string
		expectedLink        int
		expectedIntrinfo    uint32
		expectedErrorStatus uint32
		shouldMatch         bool
	}{
		// Test case 1: From user requirements - XID 149, NETIR_BER_EVENT, subcode 0
		{
			name:                "XID 149 NETIR_BER_EVENT Nonfatal Link 03 subcode 0",
			logLine:             "NVRM: Xid (PCI:0018:01:00): 149, NETIR_BER_EVENT Nonfatal XC0 i0 Link 03 (0x00086226 0x00000001 0x00000000 0x00000000 0x00000000 0x00000000)",
			expectedXid:         149,
			expectedDeviceUUID:  "0018:01:00",
			expectedSubCode:     0, // (0x00086226 >> 20) & 0x3F = 0
			expectedSubCodeName: "NETIR_BER_EVENT",
			expectedSeverity:    "Nonfatal",
			expectedLink:        3,
			expectedIntrinfo:    0x00086226,
			expectedErrorStatus: 0x00000001,
			shouldMatch:         true,
		},
		// Test case 2: From user requirements - XID 145, RLW_RXPIPE with pid/name, subcode 0
		{
			name:                "XID 145 RLW_RXPIPE with pid and name subcode 0",
			logLine:             "NVRM: Xid (PCI:0018:01:00): 145, pid=876767, name=ray::WorkerDict, RLW_RXPIPE Nonfatal XC0 i0 Link 13 (0x0409a0c2 0x00000008 0x00000001 0x00000000 0x00100000 0x00000004)",
			expectedXid:         145,
			expectedDeviceUUID:  "0018:01:00",
			expectedSubCode:     0, // (0x0409a0c2 >> 20) & 0x3F = 0
			expectedSubCodeName: "RLW_RXPIPE",
			expectedSeverity:    "Nonfatal",
			expectedLink:        13,
			expectedIntrinfo:    0x0409a0c2,
			expectedErrorStatus: 0x00000008,
			shouldMatch:         true,
		},
		// Test case 3: From user requirements - XID 145, RLW_RXPIPE another instance
		{
			name:                "XID 145 RLW_RXPIPE Link 12 subcode 0",
			logLine:             "NVRM: Xid (PCI:0019:01:00): 145, pid=876766, name=ray::WorkerDict, RLW_RXPIPE Nonfatal XC0 i0 Link 12 (0x040980c2 0x00000008 0x00000001 0x00000000 0x00100000 0x00000004)",
			expectedXid:         145,
			expectedDeviceUUID:  "0019:01:00",
			expectedSubCode:     0, // (0x040980c2 >> 20) & 0x3F = 0
			expectedSubCodeName: "RLW_RXPIPE",
			expectedSeverity:    "Nonfatal",
			expectedLink:        12,
			expectedIntrinfo:    0x040980c2,
			expectedErrorStatus: 0x00000008,
			shouldMatch:         true,
		},
		// Test case 4: From user requirements - XID 149, NETIR_BER_EVENT Link 01
		{
			name:                "XID 149 NETIR_BER_EVENT Link 01 subcode 0",
			logLine:             "NVRM: Xid (PCI:0008:01:00): 149, NETIR_BER_EVENT Nonfatal XC0 i0 Link 01 (0x00082226 0x00000001 0x00000000 0x00000000 0x00000000 0x00000000)",
			expectedXid:         149,
			expectedDeviceUUID:  "0008:01:00",
			expectedSubCode:     0, // (0x00082226 >> 20) & 0x3F = 0
			expectedSubCodeName: "NETIR_BER_EVENT",
			expectedSeverity:    "Nonfatal",
			expectedLink:        1,
			expectedIntrinfo:    0x00082226,
			expectedErrorStatus: 0x00000001,
			shouldMatch:         true,
		},
		// Test case 5: From user requirements - XID 145, RLW_SRC_TRACK
		{
			name:                "XID 145 RLW_SRC_TRACK XC1 subcode 0",
			logLine:             "NVRM: Xid (PCI:0019:01:00): 145, RLW_SRC_TRACK Nonfatal XC1 i0 Link 12 (0x040988e2 0x00000008 0x00000000 0x00000000 0x00000000 0x00000000)",
			expectedXid:         145,
			expectedDeviceUUID:  "0019:01:00",
			expectedSubCode:     0, // (0x040988e2 >> 20) & 0x3F = 0
			expectedSubCodeName: "RLW_SRC_TRACK",
			expectedSeverity:    "Nonfatal",
			expectedLink:        12,
			expectedIntrinfo:    0x040988e2,
			expectedErrorStatus: 0x00000008,
			shouldMatch:         true,
		},
		// Test case 6: From user requirements - XID 149, NETIR Fatal Link -1
		{
			name:                "XID 149 NETIR Fatal Link -1 subcode 0",
			logLine:             "NVRM: Xid (PCI:0019:01:00): 149, NETIR Fatal XC0 i0 Link -1 (0x000fe406 0x00000000 0x00000000 0x00000000 0x00000000 0x00000000)",
			expectedXid:         149,
			expectedDeviceUUID:  "0019:01:00",
			expectedSubCode:     0, // (0x000fe406 >> 20) & 0x3F = 0
			expectedSubCodeName: "NETIR",
			expectedSeverity:    "Fatal",
			expectedLink:        -1,
			expectedIntrinfo:    0x000fe406,
			expectedErrorStatus: 0x00000000,
			shouldMatch:         true,
		},
		// Test case 7: From user requirements - XID 145, RLW_SRC_TRACK another instance
		{
			name:                "XID 145 RLW_SRC_TRACK Link 12 another instance",
			logLine:             "NVRM: Xid (PCI:0008:01:00): 145, RLW_SRC_TRACK Nonfatal XC1 i0 Link 12 (0x040988e2 0x00000008 0x00000000 0x00000000 0x00000000 0x00000000)",
			expectedXid:         145,
			expectedDeviceUUID:  "0008:01:00",
			expectedSubCode:     0, // (0x040988e2 >> 20) & 0x3F = 0
			expectedSubCodeName: "RLW_SRC_TRACK",
			expectedSeverity:    "Nonfatal",
			expectedLink:        12,
			expectedIntrinfo:    0x040988e2,
			expectedErrorStatus: 0x00000008,
			shouldMatch:         true,
		},
		// Test case 8: From user requirements - XID 145 with pid/name
		{
			name:                "XID 145 RLW_SRC_TRACK with pid and name",
			logLine:             "NVRM: Xid (PCI:0008:01:00): 145, pid=11436, name=cache_mgr_main, RLW_SRC_TRACK Nonfatal XC1 i0 Link 03 (0x040868e2 0x00000008 0x00000000 0x00000000 0x00000000 0x00000000)",
			expectedXid:         145,
			expectedDeviceUUID:  "0008:01:00",
			expectedSubCode:     0, // (0x040868e2 >> 20) & 0x3F = 0
			expectedSubCodeName: "RLW_SRC_TRACK",
			expectedSeverity:    "Nonfatal",
			expectedLink:        3,
			expectedIntrinfo:    0x040868e2,
			expectedErrorStatus: 0x00000008,
			shouldMatch:         true,
		},
		// Test case 9: From user requirements - XID 145 with pid/name acctg
		{
			name:                "XID 145 RLW_SRC_TRACK with acctg process",
			logLine:             "NVRM: Xid (PCI:0018:01:00): 145, pid=922932, name=acctg, RLW_SRC_TRACK Nonfatal XC1 i0 Link 03 (0x040868e2 0x00000008 0x00000000 0x00000000 0x00000000 0x00000000)",
			expectedXid:         145,
			expectedDeviceUUID:  "0018:01:00",
			expectedSubCode:     0, // (0x040868e2 >> 20) & 0x3F = 0
			expectedSubCodeName: "RLW_SRC_TRACK",
			expectedSeverity:    "Nonfatal",
			expectedLink:        3,
			expectedIntrinfo:    0x040868e2,
			expectedErrorStatus: 0x00000008,
			shouldMatch:         true,
		},
		// Test case 10: From user requirements - XID 145 Link 16
		{
			name:                "XID 145 RLW_SRC_TRACK Link 16",
			logLine:             "NVRM: Xid (PCI:0008:01:00): 145, RLW_SRC_TRACK Nonfatal XC1 i0 Link 16 (0x040a08e2 0x00000008 0x00000000 0x00000000 0x00000000 0x00000000)",
			expectedXid:         145,
			expectedDeviceUUID:  "0008:01:00",
			expectedSubCode:     0, // (0x040a08e2 >> 20) & 0x3F = 0
			expectedSubCodeName: "RLW_SRC_TRACK",
			expectedSeverity:    "Nonfatal",
			expectedLink:        16,
			expectedIntrinfo:    0x040a08e2,
			expectedErrorStatus: 0x00000008,
			shouldMatch:         true,
		},
		// Test case 11: From user requirements - XID 149 NETIR Fatal Link -1
		{
			name:                "XID 149 NETIR Fatal Link -1 second instance",
			logLine:             "NVRM: Xid (PCI:0008:01:00): 149, NETIR Fatal XC0 i0 Link -1 (0x000fe406 0x00000000 0x00000000 0x00000000 0x00000000 0x00000000)",
			expectedXid:         149,
			expectedDeviceUUID:  "0008:01:00",
			expectedSubCode:     0, // (0x000fe406 >> 20) & 0x3F = 0
			expectedSubCodeName: "NETIR",
			expectedSeverity:    "Fatal",
			expectedLink:        -1,
			expectedIntrinfo:    0x000fe406,
			expectedErrorStatus: 0x00000000,
			shouldMatch:         true,
		},
		// Test case 12: From user requirements - XID 149 NETIR_LINK_EVT subcode 4 (CRITICAL!)
		{
			name:                "XID 149 NETIR_LINK_EVT Fatal Link 08 subcode 4",
			logLine:             "NVRM: Xid (PCI:0008:01:00): 149, NETIR_LINK_EVT Fatal XC0 i0 Link 08 (0x004505c6 0x00000000 0x00000000 0x00000000 0x00000000 0x00000000)",
			expectedXid:         149,
			expectedDeviceUUID:  "0008:01:00",
			expectedSubCode:     4, // (0x004505c6 >> 20) & 0x3F = 4 -- CARTRIDGE ERROR!
			expectedSubCodeName: "NETIR_LINK_EVT",
			expectedSeverity:    "Fatal",
			expectedLink:        8,
			expectedIntrinfo:    0x004505c6,
			expectedErrorStatus: 0x00000000,
			shouldMatch:         true,
		},
		// Test case 13: From user requirements - XID 149 NETIR Fatal Link -1
		{
			name:                "XID 149 NETIR Fatal Link -1 third instance",
			logLine:             "NVRM: Xid (PCI:0018:01:00): 149, NETIR Fatal XC0 i0 Link -1 (0x000fe406 0x00000000 0x00000000 0x00000000 0x00000000 0x00000000)",
			expectedXid:         149,
			expectedDeviceUUID:  "0018:01:00",
			expectedSubCode:     0, // (0x000fe406 >> 20) & 0x3F = 0
			expectedSubCodeName: "NETIR",
			expectedSeverity:    "Fatal",
			expectedLink:        -1,
			expectedIntrinfo:    0x000fe406,
			expectedErrorStatus: 0x00000000,
			shouldMatch:         true,
		},
		// Test case 14: From user requirements - XID 149 NETIR_LINK_EVT subcode 4 again
		{
			name:                "XID 149 NETIR_LINK_EVT Fatal Link 08 subcode 4 again",
			logLine:             "NVRM: Xid (PCI:0008:01:00): 149, NETIR_LINK_EVT Fatal XC0 i0 Link 08 (0x004505c6 0x00000000 0x00000000 0x00000000 0x00000000 0x00000000)",
			expectedXid:         149,
			expectedDeviceUUID:  "0008:01:00",
			expectedSubCode:     4, // (0x004505c6 >> 20) & 0x3F = 4 -- CARTRIDGE ERROR!
			expectedSubCodeName: "NETIR_LINK_EVT",
			expectedSeverity:    "Fatal",
			expectedLink:        8,
			expectedIntrinfo:    0x004505c6,
			expectedErrorStatus: 0x00000000,
			shouldMatch:         true,
		},
		// Test case 15: From user requirements - XID 149 NETIR_LINK_EVT Link 09 subcode 4
		{
			name:                "XID 149 NETIR_LINK_EVT Fatal Link 09 subcode 4",
			logLine:             "NVRM: Xid (PCI:0018:01:00): 149, NETIR_LINK_EVT Fatal XC0 i0 Link 09 (0x004525c6 0x00000000 0x00000000 0x00000000 0x00000000 0x00000000)",
			expectedXid:         149,
			expectedDeviceUUID:  "0018:01:00",
			expectedSubCode:     4, // (0x004525c6 >> 20) & 0x3F = 4 -- CARTRIDGE ERROR!
			expectedSubCodeName: "NETIR_LINK_EVT",
			expectedSeverity:    "Fatal",
			expectedLink:        9,
			expectedIntrinfo:    0x004525c6,
			expectedErrorStatus: 0x00000000,
			shouldMatch:         true,
		},
		// Test case 16: From user requirements - XID 149 NETIR Fatal Link -1
		{
			name:                "XID 149 NETIR Fatal Link -1 fourth instance",
			logLine:             "NVRM: Xid (PCI:0019:01:00): 149, NETIR Fatal XC0 i0 Link -1 (0x000fe406 0x00000000 0x00000000 0x00000000 0x00000000 0x00000000)",
			expectedXid:         149,
			expectedDeviceUUID:  "0019:01:00",
			expectedSubCode:     0, // (0x000fe406 >> 20) & 0x3F = 0
			expectedSubCodeName: "NETIR",
			expectedSeverity:    "Fatal",
			expectedLink:        -1,
			expectedIntrinfo:    0x000fe406,
			expectedErrorStatus: 0x00000000,
			shouldMatch:         true,
		},
		// Test case 17: From user requirements - XID 149 NETIR Fatal Link -1
		{
			name:                "XID 149 NETIR Fatal Link -1 fifth instance",
			logLine:             "NVRM: Xid (PCI:0018:01:00): 149, NETIR Fatal XC0 i0 Link -1 (0x000fe406 0x00000000 0x00000000 0x00000000 0x00000000 0x00000000)",
			expectedXid:         149,
			expectedDeviceUUID:  "0018:01:00",
			expectedSubCode:     0, // (0x000fe406 >> 20) & 0x3F = 0
			expectedSubCodeName: "NETIR",
			expectedSeverity:    "Fatal",
			expectedLink:        -1,
			expectedIntrinfo:    0x000fe406,
			expectedErrorStatus: 0x00000000,
			shouldMatch:         true,
		},
		// Test case 18: From user requirements - XID 149 NETIR_LINK_EVT Link 09 subcode 4
		{
			name:                "XID 149 NETIR_LINK_EVT Fatal Link 09 subcode 4 again",
			logLine:             "NVRM: Xid (PCI:0018:01:00): 149, NETIR_LINK_EVT Fatal XC0 i0 Link 09 (0x004525c6 0x00000000 0x00000000 0x00000000 0x00000000 0x00000000)",
			expectedXid:         149,
			expectedDeviceUUID:  "0018:01:00",
			expectedSubCode:     4, // (0x004525c6 >> 20) & 0x3F = 4 -- CARTRIDGE ERROR!
			expectedSubCodeName: "NETIR_LINK_EVT",
			expectedSeverity:    "Fatal",
			expectedLink:        9,
			expectedIntrinfo:    0x004525c6,
			expectedErrorStatus: 0x00000000,
			shouldMatch:         true,
		},
		// Test case 19: From user requirements - XID 149 NETIR_LINK_EVT Link 09 subcode 4
		{
			name:                "XID 149 NETIR_LINK_EVT Fatal Link 09 subcode 4 third time",
			logLine:             "NVRM: Xid (PCI:0018:01:00): 149, NETIR_LINK_EVT Fatal XC0 i0 Link 09 (0x004525c6 0x00000000 0x00000000 0x00000000 0x00000000 0x00000000)",
			expectedXid:         149,
			expectedDeviceUUID:  "0018:01:00",
			expectedSubCode:     4, // (0x004525c6 >> 20) & 0x3F = 4 -- CARTRIDGE ERROR!
			expectedSubCodeName: "NETIR_LINK_EVT",
			expectedSeverity:    "Fatal",
			expectedLink:        9,
			expectedIntrinfo:    0x004525c6,
			expectedErrorStatus: 0x00000000,
			shouldMatch:         true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := ExtractNVRMXidInfoExtended(tt.logLine)

			if !tt.shouldMatch {
				assert.Nilf(t, info, "expected no match for log line %q", tt.logLine)
				return
			}

			require.NotNilf(t, info, "expected to match log line %q", tt.logLine)

			assert.Equalf(t, tt.expectedXid, info.Xid, "log line: %s", tt.logLine)
			assert.Equalf(t, tt.expectedDeviceUUID, info.DeviceUUID, "log line: %s", tt.logLine)
			assert.Equalf(t, tt.expectedSubCode, info.SubCode, "log line: %s (intrinfo=0x%08x)", tt.logLine, info.Intrinfo)
			assert.Equalf(t, tt.expectedSubCodeName, info.SubCodeName, "log line: %s", tt.logLine)
			assert.Equalf(t, tt.expectedSeverity, info.Severity, "log line: %s", tt.logLine)
			assert.Equalf(t, tt.expectedLink, info.Link, "log line: %s", tt.logLine)
			assert.Equalf(t, tt.expectedIntrinfo, info.Intrinfo, "log line: %s", tt.logLine)
			assert.Equalf(t, tt.expectedErrorStatus, info.ErrorStatus, "log line: %s", tt.logLine)

			for i := 0; i < 3; i++ {
				info2 := ExtractNVRMXidInfoExtended(tt.logLine)
				require.NotNilf(t, info2, "consistency check %d failed for log line %q", i+1, tt.logLine)
				assert.Equalf(t, info.SubCode, info2.SubCode, "consistency check %d failed for log line %q", i+1, tt.logLine)
				assert.Equalf(t, info.Intrinfo, info2.Intrinfo, "consistency check %d failed for log line %q", i+1, tt.logLine)
			}
		})
	}
}

// TestCalculateSubCode tests the subcode calculation function
func TestCalculateSubCode(t *testing.T) {
	tests := []struct {
		name         string
		intrinfo     uint32
		expectedCode int
	}{
		{"Subcode 0", 0x00086226, 0},                  // From NETIR_BER_EVENT
		{"Subcode 4", 0x004505c6, 4},                  // From NETIR_LINK_EVT - cartridge error!
		{"Subcode 4 Link 09", 0x004525c6, 4},          // Another NETIR_LINK_EVT
		{"Subcode 0 from RLW", 0x0409a0c2, 0},         // From RLW_RXPIPE
		{"Subcode 0 from NETIR Fatal", 0x000fe406, 0}, // From NETIR Fatal
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := calculateSubCode(tt.intrinfo)
			assert.Equalf(t, tt.expectedCode, got, "calculateSubCode failed for intrinfo 0x%08x (bits 20-25: %06b)", tt.intrinfo, (tt.intrinfo>>20)&0x3F)
		})
	}
}

// TestExtractNVRMXidInfoExtendedShortMatch ensures the parser tolerates truncated log lines without panicking.
func TestExtractNVRMXidInfoExtendedShortMatch(t *testing.T) {
	line := "NVRM: Xid (PCI:0018:01:00): 149, NETIR Fatal XC0 i0 Link -1 (0x000fe406"
	require.Nilf(t, ExtractNVRMXidInfoExtended(line), "expected nil info when passing truncated log line %q", line)
}

// TestGetDetailWithSubCode tests the subcode detail lookup function
func TestGetDetailWithSubCode(t *testing.T) {
	tests := []struct {
		name                   string
		xid                    int
		subCode                int
		expectedFound          bool
		expectedEventTypeFatal bool
	}{
		// XID 144 tests
		{
			name:                   "XID 144 subcode 0 should exist (SAW_MVB)",
			xid:                    144,
			subCode:                0,
			expectedFound:          true,
			expectedEventTypeFatal: false, // EventTypeCritical
		},
		// XID 145 tests
		{
			name:                   "XID 145 subcode 0 should exist (RLW)",
			xid:                    145,
			subCode:                0,
			expectedFound:          true,
			expectedEventTypeFatal: false, // EventTypeCritical
		},
		// XID 146 tests
		{
			name:                   "XID 146 subcode 0 should exist (TLW)",
			xid:                    146,
			subCode:                0,
			expectedFound:          true,
			expectedEventTypeFatal: false, // EventTypeCritical
		},
		// XID 147 tests
		{
			name:                   "XID 147 subcode 0 should exist (TREX)",
			xid:                    147,
			subCode:                0,
			expectedFound:          true,
			expectedEventTypeFatal: false, // EventTypeWarning
		},
		// XID 148 tests
		{
			name:                   "XID 148 subcode 0 should exist (NVLPW_CTRL)",
			xid:                    148,
			subCode:                0,
			expectedFound:          true,
			expectedEventTypeFatal: false, // EventTypeWarning
		},
		// XID 149 tests
		{
			name:                   "XID 149 subcode 0 should exist",
			xid:                    149,
			subCode:                0,
			expectedFound:          true,
			expectedEventTypeFatal: false, // EventTypeCritical
		},
		{
			name:                   "XID 149 subcode 4 should be Fatal (cartridge error)",
			xid:                    149,
			subCode:                4,
			expectedFound:          true,
			expectedEventTypeFatal: true, // EventTypeFatal - requires provider intervention
		},
		{
			name:                   "XID 149 subcode 10 (0xA) should be Fatal (PHY timeout)",
			xid:                    149,
			subCode:                10,
			expectedFound:          true,
			expectedEventTypeFatal: true, // EventTypeFatal - requires provider intervention
		},
		{
			name:                   "XID 149 subcode 15 (0xF) should exist",
			xid:                    149,
			subCode:                15,
			expectedFound:          true,
			expectedEventTypeFatal: false, // EventTypeCritical
		},
		// XID 150 tests
		{
			name:                   "XID 150 subcode 0 should exist (MSE)",
			xid:                    150,
			subCode:                0,
			expectedFound:          true,
			expectedEventTypeFatal: false, // EventTypeCritical
		},
		// Fallback test
		{
			name:                   "XID 13 has no subcodes - should fall back to base detail",
			xid:                    13,
			subCode:                0,
			expectedFound:          true,
			expectedEventTypeFatal: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			detail, found := getDetailWithSubCode(tt.xid, tt.subCode)
			require.Equalf(t, tt.expectedFound, found, "GetDetailWithSubCode(%d, %d) mismatch", tt.xid, tt.subCode)
			if !found {
				return
			}
			if tt.expectedEventTypeFatal {
				assert.Equalf(t, apiv1.EventTypeFatal, detail.EventType, "XID %d.%d should be Fatal", tt.xid, tt.subCode)
			}
			assert.Equalf(t, tt.xid, detail.Code, "XID %d.%d code mismatch", tt.xid, tt.subCode)
		})
	}
}

func TestMatchNVLinkExamples(t *testing.T) {
	tests := []struct {
		name           string
		logLine        string
		expectedSub    int
		expectedDesc   string
		expectedEvent  apiv1.EventType
		expectedAction []apiv1.RepairActionType
	}{
		{"Log1 NETIR_BER_EVENT", "NVRM: Xid (PCI:0018:01:00): 149, NETIR_BER_EVENT Nonfatal XC0 i0 Link 03 (0x00086226 0x00000001 0x00000000 0x00000000 0x00000000 0x00000000)", 0, "NETIR_BER_EVENT", apiv1.EventTypeWarning, []apiv1.RepairActionType{apiv1.RepairActionTypeIgnoreNoActionRequired}},
		{"Log2 RLW_RXPIPE", "NVRM: Xid (PCI:0018:01:00): 145, pid=876767, name=ray::WorkerDict, RLW_RXPIPE Nonfatal XC0 i0 Link 13 (0x0409a0c2 0x00000008 0x00000001 0x00000000 0x00100000 0x00000004)", 0, "RLW_RXPIPE", apiv1.EventTypeWarning, []apiv1.RepairActionType{apiv1.RepairActionTypeIgnoreNoActionRequired}},
		{"Log3 RLW_RXPIPE", "NVRM: Xid (PCI:0019:01:00): 145, pid=876766, name=ray::WorkerDict, RLW_RXPIPE Nonfatal XC0 i0 Link 12 (0x040980c2 0x00000008 0x00000001 0x00000000 0x00100000 0x00000004)", 0, "RLW_RXPIPE", apiv1.EventTypeWarning, []apiv1.RepairActionType{apiv1.RepairActionTypeIgnoreNoActionRequired}},
		{"Log4 NETIR_BER_EVENT", "NVRM: Xid (PCI:0008:01:00): 149, NETIR_BER_EVENT Nonfatal XC0 i0 Link 01 (0x00082226 0x00000001 0x00000000 0x00000000 0x00000000 0x00000000)", 0, "NETIR_BER_EVENT", apiv1.EventTypeWarning, []apiv1.RepairActionType{apiv1.RepairActionTypeIgnoreNoActionRequired}},
		{"Log5 RLW_SRC_TRACK", "NVRM: Xid (PCI:0019:01:00): 145, RLW_SRC_TRACK Nonfatal XC1 i0 Link 12 (0x040988e2 0x00000008 0x00000000 0x00000000 0x00000000 0x00000000)", 0, "RLW_SRC_TRACK", apiv1.EventTypeWarning, []apiv1.RepairActionType{apiv1.RepairActionTypeHardwareInspection}},
		{"Log6 NETIR Fatal", "NVRM: Xid (PCI:0019:01:00): 149, NETIR Fatal XC0 i0 Link -1 (0x000fe406 0x00000000 0x00000000 0x00000000 0x00000000 0x00000000)", 0, "NETIR", apiv1.EventTypeFatal, []apiv1.RepairActionType{apiv1.RepairActionTypeRebootSystem}},
		{"Log7 RLW_SRC_TRACK", "NVRM: Xid (PCI:0008:01:00): 145, RLW_SRC_TRACK Nonfatal XC1 i0 Link 12 (0x040988e2 0x00000008 0x00000000 0x00000000 0x00000000 0x00000000)", 0, "RLW_SRC_TRACK", apiv1.EventTypeWarning, []apiv1.RepairActionType{apiv1.RepairActionTypeHardwareInspection}},
		{"Log8 RLW_SRC_TRACK", "NVRM: Xid (PCI:0008:01:00): 145, pid=11436, name=cache_mgr_main, RLW_SRC_TRACK Nonfatal XC1 i0 Link 03 (0x040868e2 0x00000008 0x00000000 0x00000000 0x00000000 0x00000000)", 0, "RLW_SRC_TRACK", apiv1.EventTypeWarning, []apiv1.RepairActionType{apiv1.RepairActionTypeHardwareInspection}},
		{"Log9 RLW_SRC_TRACK", "NVRM: Xid (PCI:0018:01:00): 145, pid=922932, name=acctg, RLW_SRC_TRACK Nonfatal XC1 i0 Link 03 (0x040868e2 0x00000008 0x00000000 0x00000000 0x00000000 0x00000000)", 0, "RLW_SRC_TRACK", apiv1.EventTypeWarning, []apiv1.RepairActionType{apiv1.RepairActionTypeHardwareInspection}},
		{"Log10 RLW_SRC_TRACK", "NVRM: Xid (PCI:0008:01:00): 145, RLW_SRC_TRACK Nonfatal XC1 i0 Link 16 (0x040a08e2 0x00000008 0x00000000 0x00000000 0x00000000 0x00000000)", 0, "RLW_SRC_TRACK", apiv1.EventTypeWarning, []apiv1.RepairActionType{apiv1.RepairActionTypeHardwareInspection}},
		{"Log11 NETIR Fatal", "NVRM: Xid (PCI:0008:01:00): 149, NETIR Fatal XC0 i0 Link -1 (0x000fe406 0x00000000 0x00000000 0x00000000 0x00000000 0x00000000)", 0, "NETIR", apiv1.EventTypeFatal, []apiv1.RepairActionType{apiv1.RepairActionTypeRebootSystem}},
		{"Log12 NETIR_LINK_EVT", "NVRM: Xid (PCI:0008:01:00): 149, NETIR_LINK_EVT Fatal XC0 i0 Link 08 (0x004505c6 0x00000000 0x00000000 0x00000000 0x00000000 0x00000000)", 4, "NETIR_LINK_EVT", apiv1.EventTypeFatal, []apiv1.RepairActionType{apiv1.RepairActionTypeRebootSystem}},
		{"Log13 NETIR Fatal", "NVRM: Xid (PCI:0018:01:00): 149, NETIR Fatal XC0 i0 Link -1 (0x000fe406 0x00000000 0x00000000 0x00000000 0x00000000 0x00000000)", 0, "NETIR", apiv1.EventTypeFatal, []apiv1.RepairActionType{apiv1.RepairActionTypeRebootSystem}},
		{"Log14 NETIR_LINK_EVT", "NVRM: Xid (PCI:0008:01:00): 149, NETIR_LINK_EVT Fatal XC0 i0 Link 08 (0x004505c6 0x00000000 0x00000000 0x00000000 0x00000000 0x00000000)", 4, "NETIR_LINK_EVT", apiv1.EventTypeFatal, []apiv1.RepairActionType{apiv1.RepairActionTypeRebootSystem}},
		{"Log15 NETIR_LINK_EVT", "NVRM: Xid (PCI:0018:01:00): 149, NETIR_LINK_EVT Fatal XC0 i0 Link 09 (0x004525c6 0x00000000 0x00000000 0x00000000 0x00000000 0x00000000)", 4, "NETIR_LINK_EVT", apiv1.EventTypeFatal, []apiv1.RepairActionType{apiv1.RepairActionTypeRebootSystem}},
		{"Log16 NETIR Fatal", "NVRM: Xid (PCI:0019:01:00): 149, NETIR Fatal XC0 i0 Link -1 (0x000fe406 0x00000000 0x00000000 0x00000000 0x00000000 0x00000000)", 0, "NETIR", apiv1.EventTypeFatal, []apiv1.RepairActionType{apiv1.RepairActionTypeRebootSystem}},
		{"Log17 NETIR Fatal", "NVRM: Xid (PCI:0018:01:00): 149, NETIR Fatal XC0 i0 Link -1 (0x000fe406 0x00000000 0x00000000 0x00000000 0x00000000 0x00000000)", 0, "NETIR", apiv1.EventTypeFatal, []apiv1.RepairActionType{apiv1.RepairActionTypeRebootSystem}},
		{"Log18 NETIR_LINK_EVT", "NVRM: Xid (PCI:0018:01:00): 149, NETIR_LINK_EVT Fatal XC0 i0 Link 09 (0x004525c6 0x00000000 0x00000000 0x00000000 0x00000000 0x00000000)", 4, "NETIR_LINK_EVT", apiv1.EventTypeFatal, []apiv1.RepairActionType{apiv1.RepairActionTypeRebootSystem}},
		{"Log19 NETIR_LINK_EVT", "NVRM: Xid (PCI:0018:01:00): 149, NETIR_LINK_EVT Fatal XC0 i0 Link 09 (0x004525c6 0x00000000 0x00000000 0x00000000 0x00000000 0x00000000)", 4, "NETIR_LINK_EVT", apiv1.EventTypeFatal, []apiv1.RepairActionType{apiv1.RepairActionTypeRebootSystem}},

		// Test cases for RLW_CTRL and RLW_REMAP - verifies distinguishable subcode names
		{"Log20 RLW_CTRL", "NVRM: Xid (PCI:0000:04:00): 145, RLW_CTRL Nonfatal XC0 i0 Link 00 (0x00000003 0x80000000 0x00000000 0x00000000)", 0, "RLW_CTRL", apiv1.EventTypeWarning, []apiv1.RepairActionType{apiv1.RepairActionTypeIgnoreNoActionRequired}},
		{"Log21 RLW_REMAP", "NVRM: Xid (PCI:0000:04:00): 145, RLW_REMAP Nonfatal XC0 i0 Link 00 (0x00000004 0x00000001 0x00000000 0x00000000)", 0, "RLW_REMAP", apiv1.EventTypeWarning, []apiv1.RepairActionType{apiv1.RepairActionTypeHardwareInspection}},

		// Test cases for XID 144 (SAW Error) - verifies different SAW subcodes are distinguishable
		{"Log22 SAW_MVB", "NVRM: Xid (PCI:0000:04:00): 144, SAW_MVB Nonfatal XC0 i0 Link 00 (0x00000003 0x80000000 0x00000000 0x00000000)", 0, "SAW_MVB", apiv1.EventTypeWarning, nil},
		{"Log23 SAW_EGR", "NVRM: Xid (PCI:0000:04:00): 144, SAW_EGR Nonfatal XC0 i0 Link 00 (0x00000004 0x00000001 0x00000000 0x00000000)", 0, "SAW_EGR", apiv1.EventTypeWarning, nil},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			xidErr := Match(tc.logLine)
			if assert.NotNil(t, xidErr, "Match(%q) should not be nil", tc.logLine) {
				assert.NotNil(t, xidErr.Detail)
				if xidErr.Detail != nil {
					assert.Equal(t, tc.expectedSub, xidErr.Detail.SubCode)
					assert.Equal(t, tc.expectedDesc, xidErr.Detail.SubCodeDescription)
					assert.Equal(t, tc.expectedEvent, xidErr.Detail.EventType)
					if tc.expectedAction != nil {
						if assert.NotNil(t, xidErr.Detail.SuggestedActionsByGPUd) {
							for _, act := range tc.expectedAction {
								assert.Contains(t, xidErr.Detail.SuggestedActionsByGPUd.RepairActions, act)
							}
						}
					}
				}
			}
		})
	}
}
