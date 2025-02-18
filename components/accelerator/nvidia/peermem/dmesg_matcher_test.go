package peermem

import "testing"

func TestHasPeermemInvalidContext(t *testing.T) {
	tests := []struct {
		name string
		line string
		want bool
	}{
		{
			name: "exact match",
			line: "nvidia-peermem nv_get_p2p_free_callback:127 ERROR detected invalid context, skipping further processing",
			want: true,
		},
		{
			name: "with timestamp prefix",
			line: "[Thu Sep 19 02:29:46 2024] nvidia-peermem nv_get_p2p_free_callback:127 ERROR detected invalid context, skipping further processing",
			want: true,
		},
		{
			name: "with ISO timestamp and facility",
			line: "kern  :warn  : 2025-01-21T04:41:44,285060+00:00 nvidia-peermem nv_get_p2p_free_callback:127 ERROR detected invalid context, skipping further processing",
			want: true,
		},
		{
			name: "with different line number",
			line: "nvidia-peermem nv_get_p2p_free_callback:128 ERROR detected invalid context, skipping further processing",
			want: true,
		},
		{
			name: "no match - different error message",
			line: "nvidia-peermem nv_get_p2p_free_callback:127 some other error message",
			want: false,
		},
		{
			name: "empty string",
			line: "",
			want: false,
		},
		{
			name: "partial match - missing error message",
			line: "nvidia-peermem nv_get_p2p_free_callback:127",
			want: false,
		},
		{
			name: "partial match - missing callback info",
			line: "nvidia-peermem ERROR detected invalid context, skipping further processing",
			want: true, // should still match as regex only looks for error message
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := HasPeermemInvalidContext(tt.line); got != tt.want {
				t.Errorf("HasPeermemInvalidContext(%q) = %v, want %v", tt.line, got, tt.want)
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
			name:        "peermem invalid context with full details",
			line:        "[Thu Sep 19 02:29:46 2024] nvidia-peermem nv_get_p2p_free_callback:127 ERROR detected invalid context, skipping further processing",
			wantName:    eventPeermemInvalidContext,
			wantMessage: messagePeermemInvalidContext,
		},
		{
			name:        "peermem invalid context with ISO timestamp",
			line:        "kern  :warn  : 2025-01-21T04:41:44,285060+00:00 nvidia-peermem nv_get_p2p_free_callback:127 ERROR detected invalid context, skipping further processing",
			wantName:    eventPeermemInvalidContext,
			wantMessage: messagePeermemInvalidContext,
		},
		{
			name:        "no match - different error",
			line:        "nvidia-peermem nv_get_p2p_free_callback:127 some other error",
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
	if len(matches) != 1 {
		t.Errorf("getMatches() returned %d matches, want 1", len(matches))
	}

	// Verify the peermem invalid context matcher
	peermemMatch := matches[0]
	if peermemMatch.eventName != eventPeermemInvalidContext {
		t.Errorf("first match name = %v, want %v", peermemMatch.eventName, eventPeermemInvalidContext)
	}
	if peermemMatch.regex != regexPeermemInvalidContext {
		t.Errorf("first match regex = %v, want %v", peermemMatch.regex, regexPeermemInvalidContext)
	}
	if peermemMatch.message != messagePeermemInvalidContext {
		t.Errorf("first match message = %v, want %v", peermemMatch.message, messagePeermemInvalidContext)
	}

	// Test the check function with valid and invalid inputs
	validInput := "kern  :warn  : 2025-01-21T04:41:44,285060+00:00 nvidia-peermem nv_get_p2p_free_callback:127 ERROR detected invalid context, skipping further processing"
	if !peermemMatch.check(validInput) {
		t.Error("check function failed to match valid input")
	}

	invalidInput := "some random log message"
	if peermemMatch.check(invalidInput) {
		t.Error("check function matched invalid input")
	}
}
