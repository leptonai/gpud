package dmesg

import "testing"

func TestHasBlockedTooLong(t *testing.T) {
	tests := []struct {
		name         string
		line         string
		expectedTask string
		expected     bool
	}{
		{
			name:         "task blocked message",
			line:         "INFO: task kcompactd1:1177 blocked for more than 120 seconds.",
			expectedTask: "kcompactd1",
			expected:     true,
		},
		{
			name:         "another task blocked message 1",
			line:         "task jfsmount:136986 blocked for more than 120 seconds.",
			expectedTask: "jfsmount",
			expected:     true,
		},
		{
			name:         "another task blocked message 2",
			line:         "INFO: task jfsmount:136986 blocked for more than 120 seconds.",
			expectedTask: "jfsmount",
			expected:     true,
		},
		{
			name:         "another task blocked message 3",
			line:         "[Sun Jan  5 20:25:34 2025] INFO: task jfsmount:136986 blocked for more than 120 seconds.",
			expectedTask: "jfsmount",
			expected:     true,
		},
		{
			name:         "non-matching message 1",
			line:         "INFO: task running normally",
			expectedTask: "",
			expected:     false,
		},
		{
			name:         "non-matching message 2",
			line:         "task running normally",
			expectedTask: "",
			expected:     false,
		},
		{
			name:         "empty string",
			line:         "",
			expectedTask: "",
			expected:     false,
		},
		{
			name:         "different time period",
			line:         "INFO: task xyz blocked for more than 60 seconds.",
			expectedTask: "",
			expected:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotTask, got := HasBlockedTooLong(tt.line)
			if got != tt.expected || gotTask != tt.expectedTask {
				t.Errorf("HasBlockedTooLong() = (%v, %v), want (%v, %v)", gotTask, got, tt.expectedTask, tt.expected)
			}
		})
	}
}

func TestHasSoftLockup(t *testing.T) {
	tests := []struct {
		name         string
		line         string
		expectedTask string
		expected     bool
	}{
		{
			name:         "cuda event handler lockup",
			line:         "[Sun Jan  5 18:37:06 2025] watchdog: BUG: soft lockup - CPU#0 stuck for 27s! [cuda-EvtHandlr:2255424]",
			expectedTask: "cuda-EvtHandlr",
			expected:     true,
		},
		{
			name:         "python process lockup",
			line:         "[Sun Jan  5 18:28:55 2025] watchdog: BUG: soft lockup - CPU#18 stuck for 27s! [python3:2254956]",
			expectedTask: "python3",
			expected:     true,
		},
		{
			name:         "non-matching message",
			line:         "normal CPU operation",
			expectedTask: "",
			expected:     false,
		},
		{
			name:         "empty string",
			line:         "",
			expectedTask: "",
			expected:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotTask, got := HasSoftLockup(tt.line)
			if got != tt.expected || gotTask != tt.expectedTask {
				t.Errorf("HasSoftLockup() = (%v, %v), want (%v, %v)", gotTask, got, tt.expectedTask, tt.expected)
			}
		})
	}
}
