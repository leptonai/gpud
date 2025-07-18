package cpu

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHasBlockedTooLong(t *testing.T) {
	tests := []struct {
		name            string
		line            string
		expectedMatch   bool
		expectedProcess string
	}{
		{
			name:            "task blocked message",
			line:            "INFO: task kcompactd1:1177 blocked for more than 120 seconds.",
			expectedMatch:   true,
			expectedProcess: "kcompactd1:1177",
		},
		{
			name:            "another task blocked message 1",
			line:            "task jfsmount:136986 blocked for more than 120 seconds.",
			expectedMatch:   true,
			expectedProcess: "jfsmount:136986",
		},
		{
			name:            "another task blocked message 2",
			line:            "INFO: task jfsmount:136986 blocked for more than 120 seconds.",
			expectedMatch:   true,
			expectedProcess: "jfsmount:136986",
		},
		{
			name:            "another task blocked message 3",
			line:            "[Sun Jan  5 20:25:34 2025] INFO: task jfsmount:136986 blocked for more than 120 seconds.",
			expectedMatch:   true,
			expectedProcess: "jfsmount:136986",
		},
		{
			name:            "another task blocked message 3 with different seconds",
			line:            "[Sun Jan  5 20:25:34 2025] INFO: task jfsmount:136986 blocked for more than 999 seconds.",
			expectedMatch:   true,
			expectedProcess: "jfsmount:136986",
		},
		{
			name:            "non-matching message 1",
			line:            "INFO: task running normally",
			expectedMatch:   false,
			expectedProcess: "",
		},
		{
			name:            "non-matching message 2",
			line:            "task running normally",
			expectedMatch:   false,
			expectedProcess: "",
		},
		{
			name:            "empty string",
			line:            "",
			expectedMatch:   false,
			expectedProcess: "",
		},
		{
			name:            "different time period",
			line:            "INFO: task xyz blocked for more than 60 seconds.",
			expectedMatch:   false,
			expectedProcess: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			processInfo, ok := HasBlockedTooLong(tt.line)
			assert.Equal(t, tt.expectedMatch, ok, "HasBlockedTooLong(%q) match", tt.line)
			assert.Equal(t, tt.expectedProcess, processInfo, "HasBlockedTooLong(%q) processInfo", tt.line)
		})
	}
}

func TestHasSoftLockup(t *testing.T) {
	tests := []struct {
		name            string
		line            string
		expectedMatch   bool
		expectedProcess string
	}{
		{
			name:            "cuda event handler lockup",
			line:            "[Sun Jan  5 18:37:06 2025] watchdog: BUG: soft lockup - CPU#0 stuck for 27s! [cuda-EvtHandlr:2255424]",
			expectedMatch:   true,
			expectedProcess: "cuda-EvtHandlr:2255424",
		},
		{
			name:            "python process lockup",
			line:            "[Sun Jan  5 18:28:55 2025] watchdog: BUG: soft lockup - CPU#18 stuck for 27s! [python3:2254956]",
			expectedMatch:   true,
			expectedProcess: "python3:2254956",
		},
		{
			name:            "non-matching message",
			line:            "normal CPU operation",
			expectedMatch:   false,
			expectedProcess: "",
		},
		{
			name:            "empty string",
			line:            "",
			expectedMatch:   false,
			expectedProcess: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			processInfo, ok := HasSoftLockup(tt.line)
			assert.Equal(t, tt.expectedMatch, ok, "HasSoftLockup(%q) match", tt.line)
			assert.Equal(t, tt.expectedProcess, processInfo, "HasSoftLockup(%q) processInfo", tt.line)
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
			wantMessage: "CPU task blocked for more than 120 seconds (kcompactd1:1177)",
		},
		{
			name:        "blocked too long with timestamp",
			line:        "[Sun Jan  5 20:25:34 2025] INFO: task jfsmount:136986 blocked for more than 120 seconds.",
			wantName:    eventBlockedTooLong,
			wantMessage: "CPU task blocked for more than 120 seconds (jfsmount:136986)",
		},
		{
			name:        "soft lockup",
			line:        "[Sun Jan  5 18:37:06 2025] watchdog: BUG: soft lockup - CPU#0 stuck for 27s! [cuda-EvtHandlr:2255424]",
			wantName:    eventSoftLockup,
			wantMessage: "CPU soft lockup detected, not releasing for a period of time (cuda-EvtHandlr:2255424)",
		},
		{
			name:        "soft lockup with different process",
			line:        "[Sun Jan  5 18:28:55 2025] watchdog: BUG: soft lockup - CPU#18 stuck for 27s! [python3:2254956]",
			wantName:    eventSoftLockup,
			wantMessage: "CPU soft lockup detected, not releasing for a period of time (python3:2254956)",
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
			gotName, gotMessage := Match(tt.line)
			assert.Equal(t, tt.wantName, gotName)
			assert.Equal(t, tt.wantMessage, gotMessage)
		})
	}
}

func TestGetMatches(t *testing.T) {
	matches := getMatches()

	// Verify we have the expected number of matchers
	assert.Len(t, matches, 2)

	// Verify the blocked too long matcher
	blockedMatch := matches[0]
	assert.Equal(t, eventBlockedTooLong, blockedMatch.eventName)
	assert.Equal(t, regexBlockedTooLong, blockedMatch.regex)
	assert.Equal(t, messageBlockedTooLong, blockedMatch.message)

	// Verify the soft lockup matcher
	lockupMatch := matches[1]
	assert.Equal(t, eventSoftLockup, lockupMatch.eventName)
	assert.Equal(t, regexSoftLockup, lockupMatch.regex)
	assert.Equal(t, messageSoftLockup, lockupMatch.message)

	// Test the check functions
	validBlockedInput := "[Sun Jan  5 20:25:34 2025] INFO: task jfsmount:136986 blocked for more than 120 seconds."
	processInfo, ok := blockedMatch.check(validBlockedInput)
	assert.True(t, ok)
	assert.Equal(t, "jfsmount:136986", processInfo)

	validLockupInput := "[Sun Jan  5 18:37:06 2025] watchdog: BUG: soft lockup - CPU#0 stuck for 27s! [cuda-EvtHandlr:2255424]"
	processInfo, ok = lockupMatch.check(validLockupInput)
	assert.True(t, ok)
	assert.Equal(t, "cuda-EvtHandlr:2255424", processInfo)

	invalidInput := "some random log message"
	_, ok = blockedMatch.check(invalidInput)
	assert.False(t, ok)
	_, ok = lockupMatch.check(invalidInput)
	assert.False(t, ok)
}
