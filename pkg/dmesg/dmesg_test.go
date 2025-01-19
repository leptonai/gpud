package dmesg

import (
	"bytes"
	"os"
	"strings"
	"testing"
	"time"
)

func TestParseDmesgTimeISO(t *testing.T) {
	b, err := os.ReadFile("testdata/dmesg-time-format-iso.log")
	if err != nil {
		t.Fatalf("failed to read dmesg-time-format-iso.log: %v", err)
	}

	for _, line := range strings.Split(string(b), "\n") {
		if len(line) == 0 {
			continue
		}
		time, extractedLine, err := ParseDmesgTimeISO([]byte(line))
		if err != nil {
			t.Errorf("failed to parse line: %v", err)
		}
		if time.IsZero() {
			t.Errorf("failed to parse line: time is zero")
		}
		t.Logf("time: %s, line: %s (original: %s)", time, extractedLine, line)
	}
}

func TestParseJournalctlTimeShortISO(t *testing.T) {
	tests := []struct {
		name     string
		filepath string
	}{
		{
			name:     "journalctl-time-format-short-iso-1",
			filepath: "testdata/journalctl-time-format-short-iso-1.log",
		},
		{
			name:     "journalctl-time-format-short-iso-2",
			filepath: "testdata/journalctl-time-format-short-iso-2.log",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, err := os.ReadFile(tt.filepath)
			if err != nil {
				t.Fatalf("failed to read %s: %v", tt.filepath, err)
			}

			lines := strings.Split(string(b), "\n")
			for i, line := range lines {
				if len(line) == 0 {
					continue
				}
				time, extractedLine, err := ParseJournalctlTimeShortISO([]byte(line))
				if err != nil {
					t.Errorf("failed to parse line %d: %v", i+1, err)
					continue
				}
				if time.IsZero() {
					t.Errorf("failed to parse line %d: time is zero", i+1)
					continue
				}
				t.Logf("time: %s, line: %s (original: %s)", time, extractedLine, extractedLine)
			}
		})
	}
}

func TestParseISOtimeWithError(t *testing.T) {
	tests := []struct {
		name     string
		line     []byte
		want     time.Time
		wantLine []byte
		wantErr  bool
	}{
		{
			name:     "ValidISO8601",
			line:     []byte("2024-11-15T12:02:03,561522+00:00 abc"),
			want:     time.Date(2024, 11, 15, 12, 2, 3, 561522000, time.UTC),
			wantLine: []byte("abc"),
			wantErr:  false,
		},
		{
			name:     "ShortLine",
			line:     []byte("2024-11-15"),
			want:     time.Time{},
			wantLine: nil,
			wantErr:  true,
		},
		{
			name:     "InvalidFormat",
			line:     []byte("2024-11-15 12:02:03,561522+00:00 abc"),
			want:     time.Time{},
			wantLine: nil,
			wantErr:  true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, line, err := ParseDmesgTimeISO(tt.line)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseISOtimeWithError() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !got.Equal(tt.want) {
				t.Errorf("ParseISOtimeWithError() got = %v, want %v", got, tt.want)
			}
			if !bytes.Equal(line, tt.wantLine) {
				t.Errorf("ParseISOtimeWithError() line = %v, want %v", string(line), string(tt.wantLine))
			}
		})
	}
}

func TestParseShortISOtimeWithError(t *testing.T) {
	tests := []struct {
		name     string
		line     []byte
		want     time.Time
		wantLine []byte
		wantErr  bool
	}{
		{
			name:     "ValidISO8601",
			line:     []byte("2024-11-15T12:02:03+0000 abc"),
			want:     time.Date(2024, 11, 15, 12, 2, 3, 0, time.UTC),
			wantLine: []byte("abc"),
			wantErr:  false,
		},
		{
			name:     "ShortLine",
			line:     []byte("2024-11-15"),
			want:     time.Time{},
			wantLine: nil,
			wantErr:  true,
		},
		{
			name:     "InvalidFormat",
			line:     []byte("2024-11-15 12:02:03,561522+00:00 abc"),
			want:     time.Time{},
			wantLine: nil,
			wantErr:  true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, line, err := ParseJournalctlTimeShortISO(tt.line)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseShortISOtimeWithError() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !got.Equal(tt.want) {
				t.Errorf("ParseShortISOtimeWithError() got = %v, want %v", got, tt.want)
			}
			if !bytes.Equal(line, tt.wantLine) {
				t.Errorf("ParseShortISOtimeWithError() line = %v, want %v", string(line), string(tt.wantLine))
			}
		})
	}
}
