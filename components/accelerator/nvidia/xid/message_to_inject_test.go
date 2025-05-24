package xid

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetMessageToInject(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		xid           int
		expectedXid   int
		expectedPrio  string
		shouldContain string
	}{
		{
			name:          "known XID 63",
			xid:           63,
			expectedXid:   63,
			expectedPrio:  "KERN_WARNING",
			shouldContain: "Row remapping event",
		},
		{
			name:          "known XID 64",
			xid:           64,
			expectedXid:   64,
			expectedPrio:  "KERN_WARNING",
			shouldContain: "Failed to persist row remap table",
		},
		{
			name:          "known XID 69",
			xid:           69,
			expectedXid:   69,
			expectedPrio:  "KERN_WARNING",
			shouldContain: "BAR1 access failure",
		},
		{
			name:          "known XID 74",
			xid:           74,
			expectedXid:   74,
			expectedPrio:  "KERN_WARNING",
			shouldContain: "MMU Fault",
		},
		{
			name:          "known XID 79",
			xid:           79,
			expectedXid:   79,
			expectedPrio:  "KERN_ERR",
			shouldContain: "GPU has fallen off the bus",
		},
		{
			name:          "unknown XID 42",
			xid:           42,
			expectedXid:   42,
			expectedPrio:  "KERN_WARNING",
			shouldContain: "unknown",
		},
		{
			name:          "unknown XID 999",
			xid:           999,
			expectedXid:   999,
			expectedPrio:  "KERN_WARNING",
			shouldContain: "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetMessageToInject(tt.xid)

			// Check priority
			require.Equal(t, tt.expectedPrio, result.Priority)

			// Check that message contains expected substring
			require.NotEmpty(t, result.Message)

			// For this test, we just check that the message contains some expected content
			// The actual XID extraction test is separate
			if tt.shouldContain != "" {
				require.Contains(t, result.Message, tt.shouldContain)
			}
		})
	}
}

func TestGetMessageToInject_XidExtraction(t *testing.T) {
	t.Parallel()

	// Test all known XIDs
	knownXids := []int{63, 64, 69, 74, 79}

	for _, xid := range knownXids {
		t.Run(fmt.Sprintf("known_xid_%d", xid), func(t *testing.T) {
			msg := GetMessageToInject(xid)
			extractedXid := ExtractNVRMXid(msg.Message)

			require.NotZero(t, extractedXid, "ExtractNVRMXid failed to extract XID from message: %s", msg.Message)
			require.Equal(t, xid, extractedXid)
		})
	}

	// Test unknown XIDs
	unknownXids := []int{1, 25, 42, 100, 999}

	for _, xid := range unknownXids {
		t.Run(fmt.Sprintf("unknown_xid_%d", xid), func(t *testing.T) {
			msg := GetMessageToInject(xid)
			extractedXid := ExtractNVRMXid(msg.Message)

			require.NotZero(t, extractedXid, "ExtractNVRMXid failed to extract XID from message: %s", msg.Message)
			require.Equal(t, xid, extractedXid)
		})
	}
}

func TestGetMessageToInject_MessageFormat(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		xid  int
	}{
		{"known XID 63", 63},
		{"known XID 79", 79},
		{"unknown XID 42", 42},
		{"unknown XID 999", 999},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := GetMessageToInject(tt.xid)

			// All messages should start with "NVRM: Xid"
			require.True(t, strings.HasPrefix(msg.Message, "NVRM: Xid"), "GetMessageToInject(%d).Message should start with 'NVRM: Xid', got: %s", tt.xid, msg.Message)

			// All messages should contain PCI device information
			require.Contains(t, msg.Message, "PCI:0000:04:00")

			// Priority should be valid
			validPriorities := []string{"KERN_WARNING", "KERN_ERR", "KERN_INFO", "KERN_DEBUG"}
			require.Contains(t, validPriorities, msg.Priority)
		})
	}
}

func TestAllKnownXidsHaveValidMessages(t *testing.T) {
	t.Parallel()

	// Test every XID in the xidExampleMsgs map
	for xid := range xidExampleMsgs {
		t.Run(fmt.Sprintf("xid_%d", xid), func(t *testing.T) {
			msg := GetMessageToInject(xid)
			extractedXid := ExtractNVRMXid(msg.Message)

			require.NotZero(t, extractedXid, "XID %d: ExtractNVRMXid failed to extract XID from message: %s", xid, msg.Message)
			require.Equal(t, xid, extractedXid)
		})
	}
}

func TestExamplesWithExtractNVRMXid(t *testing.T) {
	t.Parallel()
	for xid, m := range xidExampleMsgs {
		t.Run(fmt.Sprintf("xid_%s", m.Message), func(t *testing.T) {
			extractedXid := ExtractNVRMXid(m.Message)
			require.Equal(t, xid, extractedXid)
		})
	}
}
