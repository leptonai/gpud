package cpu

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

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
			assert.Equal(t, tt.expected, got, "HasBlockedTooLong(%q)", tt.line)
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
			assert.Equal(t, tt.expected, got, "HasSoftLockup(%q)", tt.line)
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
	assert.Equal(t, messageSoftLockup, lockupMatch.message)

	// Test the check functions
	validBlockedInput := "[Sun Jan  5 20:25:34 2025] INFO: task jfsmount:136986 blocked for more than 120 seconds."
	assert.True(t, blockedMatch.check(validBlockedInput))

	validLockupInput := "[Sun Jan  5 18:37:06 2025] watchdog: BUG: soft lockup - CPU#0 stuck for 27s! [cuda-EvtHandlr:2255424]"
	assert.True(t, lockupMatch.check(validLockupInput))

	invalidInput := "some random log message"
	assert.False(t, blockedMatch.check(invalidInput))
	assert.False(t, lockupMatch.check(invalidInput))
}
