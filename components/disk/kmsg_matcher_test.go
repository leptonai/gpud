package disk

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHasNoSpaceLeft(t *testing.T) {
	tests := []struct {
		name string
		line string
		want bool
	}{
		{
			name: "Exact match",
			line: "No space left",
			want: true,
		},
		{
			name: "Real log example",
			line: "[Sun Dec  8 09:23:39 2024] systemd-journald[889]: Failed to open system journal: No space left on device",
			want: true,
		},
		{
			name: "Partial match at the beginning",
			line: "No space left on this disk",
			want: true,
		},
		{
			name: "Partial match in the middle",
			line: "There is No space left on this disk",
			want: true,
		},
		{
			name: "No match",
			line: "Everything is fine",
			want: false,
		},
		{
			name: "Empty string",
			line: "",
			want: false,
		},
		{
			name: "Case mismatch",
			line: "no space left",
			want: false, // Current regex is case-sensitive
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, HasNoSpaceLeft(tt.line), "HasNoSpaceLeft()")
		})
	}
}

func TestMatch(t *testing.T) {
	tests := []struct {
		name          string
		line          string
		wantEventName string
		wantMessage   string
	}{
		{
			name:          "No space left event",
			line:          "[Sun Dec  8 09:23:39 2024] systemd-journald[889]: Failed to open system journal: No space left on device",
			wantEventName: eventNoSpaceLeft,
			wantMessage:   messageNoSpaceLeft,
		},
		{
			name:          "No space left event - simple",
			line:          "This is a line with No space left",
			wantEventName: eventNoSpaceLeft,
			wantMessage:   messageNoSpaceLeft,
		},
		{
			name:          "No match",
			line:          "Another log line without any specific event",
			wantEventName: "",
			wantMessage:   "",
		},
		{
			name:          "Empty line",
			line:          "",
			wantEventName: "",
			wantMessage:   "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotEventName, gotMessage := Match(tt.line)
			assert.Equal(t, tt.wantEventName, gotEventName)
			assert.Equal(t, tt.wantMessage, gotMessage)
		})
	}
}
