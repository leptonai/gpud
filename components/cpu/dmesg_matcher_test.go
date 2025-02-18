package cpu

import "testing"

func TestHasBlockedTooLong(t *testing.T) {
	tests := []struct {
		name     string
		line     string
		expected bool
	}{
		{
			name:     "task blocked message",
			line:     "INFO: task kcompactd1:1177 blocked for more than 120 seconds.",
			expected: true,
		},
		{
			name:     "another task blocked message 1",
			line:     "task jfsmount:136986 blocked for more than 120 seconds.",
			expected: true,
		},
		{
			name:     "another task blocked message 2",
			line:     "INFO: task jfsmount:136986 blocked for more than 120 seconds.",
			expected: true,
		},
		{
			name:     "another task blocked message 3",
			line:     "[Sun Jan  5 20:25:34 2025] INFO: task jfsmount:136986 blocked for more than 120 seconds.",
			expected: true,
		},
		{
			name:     "another task blocked message 3 with different seconds",
			line:     "[Sun Jan  5 20:25:34 2025] INFO: task jfsmount:136986 blocked for more than 999 seconds.",
			expected: true,
		},
		{
			name:     "non-matching message 1",
			line:     "INFO: task running normally",
			expected: false,
		},
		{
			name:     "non-matching message 2",
			line:     "task running normally",
			expected: false,
		},
		{
			name:     "empty string",
			line:     "",
			expected: false,
		},
		{
			name:     "different time period",
			line:     "INFO: task xyz blocked for more than 60 seconds.",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := HasBlockedTooLong(tt.line)
			if got != tt.expected {
				t.Errorf("HasBlockedTooLong(%q) = %v, want %v", tt.line, got, tt.expected)
			}
		})
	}
}

func TestHasSoftLockup(t *testing.T) {
	tests := []struct {
		name     string
		line     string
		expected bool
	}{
		{
			name:     "cuda event handler lockup",
			line:     "[Sun Jan  5 18:37:06 2025] watchdog: BUG: soft lockup - CPU#0 stuck for 27s! [cuda-EvtHandlr:2255424]",
			expected: true,
		},
		{
			name:     "python process lockup",
			line:     "[Sun Jan  5 18:28:55 2025] watchdog: BUG: soft lockup - CPU#18 stuck for 27s! [python3:2254956]",
			expected: true,
		},
		{
			name:     "non-matching message",
			line:     "normal CPU operation",
			expected: false,
		},
		{
			name:     "empty string",
			line:     "",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := HasSoftLockup(tt.line)
			if got != tt.expected {
				t.Errorf("HasSoftLockup(%q) = %v, want %v", tt.line, got, tt.expected)
			}
		})
	}
}

func TestMatch(t *testing.T) {
	tests := []struct {
		name        string
		line        string
		wantName    string
		wantMessage string
	}{
		{
			name:        "blocked too long",
			line:        "INFO: task kcompactd1:1177 blocked for more than 120 seconds.",
			wantName:    eventBlockedTooLong,
			wantMessage: messageBlockedTooLong,
		},
		{
			name:        "blocked too long with timestamp",
			line:        "[Sun Jan  5 20:25:34 2025] INFO: task jfsmount:136986 blocked for more than 120 seconds.",
			wantName:    eventBlockedTooLong,
			wantMessage: messageBlockedTooLong,
		},
		{
			name:        "soft lockup",
			line:        "[Sun Jan  5 18:37:06 2025] watchdog: BUG: soft lockup - CPU#0 stuck for 27s! [cuda-EvtHandlr:2255424]",
			wantName:    eventSoftLockup,
			wantMessage: messageSoftLockup,
		},
		{
			name:        "soft lockup with different process",
			line:        "[Sun Jan  5 18:28:55 2025] watchdog: BUG: soft lockup - CPU#18 stuck for 27s! [python3:2254956]",
			wantName:    eventSoftLockup,
			wantMessage: messageSoftLockup,
		},
		{
			name:        "no match",
			line:        "normal CPU operation",
			wantName:    "",
			wantMessage: "",
		},
		{
			name:        "empty string",
			line:        "",
			wantName:    "",
			wantMessage: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotName, _, gotMessage := Match(tt.line)
			if gotName != tt.wantName {
				t.Errorf("Match() name = %v, want %v", gotName, tt.wantName)
			}
			if gotMessage != tt.wantMessage {
				t.Errorf("Match() message = %v, want %v", gotMessage, tt.wantMessage)
			}
		})
	}
}

func TestGetMatches(t *testing.T) {
	matches := getMatches()

	// Verify we have the expected number of matchers
	if len(matches) != 2 {
		t.Errorf("getMatches() returned %d matches, want 2", len(matches))
	}

	// Verify the blocked too long matcher
	blockedMatch := matches[0]
	if blockedMatch.eventName != eventBlockedTooLong {
		t.Errorf("first match name = %v, want %v", blockedMatch.eventName, eventBlockedTooLong)
	}
	if blockedMatch.regex != regexBlockedTooLong {
		t.Errorf("first match regex = %v, want %v", blockedMatch.regex, regexBlockedTooLong)
	}
	if blockedMatch.message != messageBlockedTooLong {
		t.Errorf("first match message = %v, want %v", blockedMatch.message, messageBlockedTooLong)
	}

	// Verify the soft lockup matcher
	lockupMatch := matches[1]
	if lockupMatch.eventName != eventSoftLockup {
		t.Errorf("second match name = %v, want %v", lockupMatch.eventName, eventSoftLockup)
	}
	if lockupMatch.message != messageSoftLockup {
		t.Errorf("second match message = %v, want %v", lockupMatch.message, messageSoftLockup)
	}

	// Test the check functions
	validBlockedInput := "[Sun Jan  5 20:25:34 2025] INFO: task jfsmount:136986 blocked for more than 120 seconds."
	if !blockedMatch.check(validBlockedInput) {
		t.Error("blocked check function failed to match valid input")
	}

	validLockupInput := "[Sun Jan  5 18:37:06 2025] watchdog: BUG: soft lockup - CPU#0 stuck for 27s! [cuda-EvtHandlr:2255424]"
	if !lockupMatch.check(validLockupInput) {
		t.Error("lockup check function failed to match valid input")
	}

	invalidInput := "some random log message"
	if blockedMatch.check(invalidInput) {
		t.Error("blocked check function matched invalid input")
	}
	if lockupMatch.check(invalidInput) {
		t.Error("lockup check function matched invalid input")
	}
}
