package fabricmanager

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHasNVSwitchFatalSXid(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{
			name:     "match fatal error",
			input:    "[Jul 23 2024 07:53:55] [ERROR] [tid 841] detected NVSwitch fatal error 20034 on fid 0 on NVSwitch pci bus id 00000000:86:00.0 physical id 3 port 33",
			expected: true,
		},
		{
			name:     "match fatal error with different id",
			input:    "[Feb 15 2025 12:34:56] [ERROR] [tid 1234] detected NVSwitch fatal error 98765 on fid 0 on NVSwitch pci bus id 00000000:47:00.0 physical id 1 port 21",
			expected: true,
		},
		{
			name:     "no match - non-fatal error",
			input:    "[Jul 09 2024 18:14:07] [ERROR] [tid 12727] detected NVSwitch non-fatal error 12028 on fid 0 on NVSwitch pci bus id 00000000:86:00.0 physical id 3 port 61",
			expected: false,
		},
		{
			name:     "no match - unrelated error",
			input:    "[Sep 17 2024 06:01:46] [ERROR] [tid 1230079] failed to find the GPU handle 5410063385821516767 in the multicast team request setup 6130285411925746235.",
			expected: false,
		},
		{
			name:     "no match - info message",
			input:    "[Feb 27 2025 14:10:02] [INFO] [tid 1808] multicast group 1 is allocated.",
			expected: false,
		},
		{
			name:     "no match - empty string",
			input:    "",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := HasNVSwitchFatalSXid(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestHasNVSwitchNonFatalSXid(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{
			name:     "match non-fatal error",
			input:    "[Jul 09 2024 18:14:07] [ERROR] [tid 12727] detected NVSwitch non-fatal error 12028 on fid 0 on NVSwitch pci bus id 00000000:86:00.0 physical id 3 port 61",
			expected: true,
		},
		{
			name:     "match non-fatal error with different id",
			input:    "[Feb 15 2025 12:34:56] [ERROR] [tid 4321] detected NVSwitch non-fatal error 54321 on fid 1 on NVSwitch pci bus id 00000000:47:00.0 physical id 2 port 15",
			expected: true,
		},
		{
			name:     "no match - fatal error",
			input:    "[Jul 23 2024 07:53:55] [ERROR] [tid 841] detected NVSwitch fatal error 20034 on fid 0 on NVSwitch pci bus id 00000000:86:00.0 physical id 3 port 33",
			expected: false,
		},
		{
			name:     "no match - unrelated error",
			input:    "[Sep 17 2024 06:01:46] [ERROR] [tid 1230079] failed to find the GPU handle 5410063385821516767 in the multicast team request setup 6130285411925746235.",
			expected: false,
		},
		{
			name:     "no match - info message",
			input:    "[Feb 27 2025 14:10:02] [INFO] [tid 1808] multicast group 1 is allocated.",
			expected: false,
		},
		{
			name:     "no match - empty string",
			input:    "",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := HasNVSwitchNonFatalSXid(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestHasNVSwitchNVLinkFailure(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{
			name:     "match nvlink failure",
			input:    "[Sep 17 2024 06:01:46] [ERROR] [tid 1230079] failed to find the GPU handle 5410063385821516767 in the multicast team request setup 6130285411925746235.",
			expected: true,
		},
		{
			name:     "match nvlink failure with different id",
			input:    "[Apr 17 2024 01:51:39] [ERROR] [tid 2999877] failed to find the GPU handle 10187860174420860981 in the multicast team request setup 5653964288847403984.",
			expected: true,
		},
		{
			name:     "no match - fatal error",
			input:    "[Jul 23 2024 07:53:55] [ERROR] [tid 841] detected NVSwitch fatal error 20034 on fid 0 on NVSwitch pci bus id 00000000:86:00.0 physical id 3 port 33",
			expected: false,
		},
		{
			name:     "no match - non-fatal error",
			input:    "[Jul 09 2024 18:14:07] [ERROR] [tid 12727] detected NVSwitch non-fatal error 12028 on fid 0 on NVSwitch pci bus id 00000000:86:00.0 physical id 3 port 61",
			expected: false,
		},
		{
			name:     "no match - info message",
			input:    "[Feb 27 2025 14:10:02] [INFO] [tid 1808] multicast group 1 is allocated.",
			expected: false,
		},
		{
			name:     "no match - empty string",
			input:    "",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := HasNVSwitchNVLinkFailure(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestHasNVSwitchTopologyMismatch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{
			name:     "match topology mismatch from log file",
			input:    "[Sep 08 2025 23:21:50] [ERROR] detected number of NVSwitches don't match with any supported system topology, aborting fabric manager",
			expected: true,
		},
		{
			name:     "match topology mismatch from journalctl (no timestamp prefix)",
			input:    "detected number of NVSwitches don't match with any supported system topology, aborting fabric manager",
			expected: true,
		},
		{
			name:     "match topology mismatch with leading text",
			input:    "nv-fabricmanager[1929729]: detected number of NVSwitches don't match with any supported system topology, aborting fabric manager",
			expected: true,
		},
		{
			name:     "no match - fatal error",
			input:    "[Jul 23 2024 07:53:55] [ERROR] [tid 841] detected NVSwitch fatal error 20034 on fid 0 on NVSwitch pci bus id 00000000:86:00.0 physical id 3 port 33",
			expected: false,
		},
		{
			name:     "no match - non-fatal error",
			input:    "[Jul 09 2024 18:14:07] [ERROR] [tid 12727] detected NVSwitch non-fatal error 12028 on fid 0 on NVSwitch pci bus id 00000000:86:00.0 physical id 3 port 61",
			expected: false,
		},
		{
			name:     "no match - info message",
			input:    "[Feb 27 2025 14:10:02] [INFO] [tid 1808] multicast group 1 is allocated.",
			expected: false,
		},
		{
			name:     "no match - empty string",
			input:    "",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := HasNVSwitchTopologyMismatch(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestHasNVSwitchNothingToDo(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{
			name:     "match nothing to do from journalctl",
			input:    "request to query NVSwitch device information from NVSwitch driver failed with error:WARNING Nothing to do [NV_WARN_NOTHING_TO_DO]",
			expected: true,
		},
		{
			name:     "match nothing to do with timestamp prefix",
			input:    "[Jan 30 2025 10:15:30] [ERROR] [tid 12345] request to query NVSwitch device information from NVSwitch driver failed with error:WARNING Nothing to do [NV_WARN_NOTHING_TO_DO]",
			expected: true,
		},
		{
			name:     "match nothing to do from systemd log format",
			input:    "nv-fabricmanager[1929729]: request to query NVSwitch device information from NVSwitch driver failed with error:WARNING Nothing to do [NV_WARN_NOTHING_TO_DO]",
			expected: true,
		},
		{
			name:     "no match - fatal error",
			input:    "[Jul 23 2024 07:53:55] [ERROR] [tid 841] detected NVSwitch fatal error 20034 on fid 0 on NVSwitch pci bus id 00000000:86:00.0 physical id 3 port 33",
			expected: false,
		},
		{
			name:     "no match - topology mismatch",
			input:    "detected number of NVSwitches don't match with any supported system topology, aborting fabric manager",
			expected: false,
		},
		{
			name:     "no match - info message",
			input:    "[Feb 27 2025 14:10:02] [INFO] [tid 1808] multicast group 1 is allocated.",
			expected: false,
		},
		{
			name:     "no match - empty string",
			input:    "",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := HasNVSwitchNothingToDo(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMatch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		input         string
		expectedEvent string
		expectedMsg   string
		shouldMatch   bool
	}{
		{
			name:          "match fatal error",
			input:         "[Jul 23 2024 07:53:55] [ERROR] [tid 841] detected NVSwitch fatal error 20034 on fid 0 on NVSwitch pci bus id 00000000:86:00.0 physical id 3 port 33",
			expectedEvent: eventNVSwitchFatalSXid,
			expectedMsg:   messageNVSwitchFatalSXid,
			shouldMatch:   true,
		},
		{
			name:          "match non-fatal error",
			input:         "[Jul 09 2024 18:14:07] [ERROR] [tid 12727] detected NVSwitch non-fatal error 12028 on fid 0 on NVSwitch pci bus id 00000000:86:00.0 physical id 3 port 61",
			expectedEvent: eventNVSwitchNonFatalSXid,
			expectedMsg:   messageNVSwitchNonFatalSXid,
			shouldMatch:   true,
		},
		{
			name:          "match nvlink failure",
			input:         "[Sep 17 2024 06:01:46] [ERROR] [tid 1230079] failed to find the GPU handle 5410063385821516767 in the multicast team request setup 6130285411925746235.",
			expectedEvent: eventNVSwitchNVLinkFailure,
			expectedMsg:   messageNVSwitchNVLinkFailure,
			shouldMatch:   true,
		},
		{
			name:          "match topology mismatch",
			input:         "detected number of NVSwitches don't match with any supported system topology, aborting fabric manager",
			expectedEvent: eventNVSwitchTopologyMismatch,
			expectedMsg:   messageNVSwitchTopologyMismatch,
			shouldMatch:   true,
		},
		{
			name:          "match nothing to do",
			input:         "request to query NVSwitch device information from NVSwitch driver failed with error:WARNING Nothing to do [NV_WARN_NOTHING_TO_DO]",
			expectedEvent: EventNVSwitchNothingToDo,
			expectedMsg:   messageNVSwitchNothingToDo,
			shouldMatch:   true,
		},
		{
			name:          "no match - info message",
			input:         "[Feb 27 2025 14:10:02] [INFO] [tid 1808] multicast group 1 is allocated.",
			expectedEvent: "",
			expectedMsg:   "",
			shouldMatch:   false,
		},
		{
			name:          "no match - different error format",
			input:         "[Jan 15 2025 09:12:34] [ERROR] [tid 5678] Some other error message not related to NVSwitch.",
			expectedEvent: "",
			expectedMsg:   "",
			shouldMatch:   false,
		},
		{
			name:          "no match - empty string",
			input:         "",
			expectedEvent: "",
			expectedMsg:   "",
			shouldMatch:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eventName, message := Match(tt.input)
			if tt.shouldMatch {
				assert.Equal(t, tt.expectedEvent, eventName, "event name should match")
				assert.Equal(t, tt.expectedMsg, message, "message should match")
			} else {
				assert.Empty(t, eventName, "event name should be empty")
				assert.Empty(t, message, "message should be empty")
			}
		})
	}
}

func TestGetMatches(t *testing.T) {
	t.Parallel()

	matches := getMatches()

	// Check if we have the expected number of matchers
	assert.Equal(t, 5, len(matches), "should have 5 matchers")

	// Verify all expected matchers are present
	matcherTypes := map[string]bool{
		eventNVSwitchFatalSXid:        false,
		eventNVSwitchNonFatalSXid:     false,
		eventNVSwitchNVLinkFailure:    false,
		eventNVSwitchTopologyMismatch: false,
		EventNVSwitchNothingToDo:      false,
	}

	for _, m := range matches {
		matcherTypes[m.eventName] = true
	}

	for eventType, found := range matcherTypes {
		assert.True(t, found, "matcher for %s should exist", eventType)
	}
}
