package dmesg

import (
	"bytes"
	"os"
	"strings"
	"testing"
	"time"
)

func TestParseISOtimeWithFile(t *testing.T) {
	b, err := os.ReadFile("dmesg-time-format-iso.log")
	if err != nil {
		t.Fatalf("failed to read dmesg-time-format-iso.log: %v", err)
	}

	for _, line := range strings.Split(string(b), "\n") {
		if len(line) == 0 {
			continue
		}
		time, extractedLine, err := ParseISOtimeWithError([]byte(line))
		if err != nil {
			t.Errorf("failed to parse line: %v", err)
		}
		t.Logf("time: %s, line: %s (original: %s)", time, extractedLine, line)
	}
}

func TestParseShortISOtimeWithFile(t *testing.T) {
	b, err := os.ReadFile("dmesg-time-format-short-iso.log")
	if err != nil {
		t.Fatalf("failed to read dmesg-time-format-short-iso.log: %v", err)
	}

	for _, line := range strings.Split(string(b), "\n") {
		if len(line) == 0 {
			continue
		}
		time, extractedLine, err := ParseShortISOtimeWithError([]byte(line))
		if err != nil {
			t.Errorf("failed to parse line: %v", err)
		}
		t.Logf("time: %s, line: %s (original: %s)", time, extractedLine, line)
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
			got, line, err := ParseISOtimeWithError(tt.line)
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
			got, line, err := ParseShortISOtimeWithError(tt.line)
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

func TestParseCtime(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected time.Time
		wantErr  bool
	}{
		{
			name:     "valid timestamp",
			input:    "[Wed Oct 23 11:07:23 2024] systemd[1]: Starting Journal Service...",
			expected: time.Date(2024, time.October, 23, 11, 7, 23, 0, time.UTC),
			wantErr:  false,
		},
		{
			name:     "ipv6 addrconf message",
			input:    "[Thu Aug  8 11:50:58 2024] IPv6: ADDRCONF(NETDEV_CHANGE): calic8a3d4799be: link becomes ready",
			expected: time.Date(2024, time.August, 8, 11, 50, 58, 0, time.UTC),
			wantErr:  false,
		},
		{
			name:     "systemd message",
			input:    "[Wed Oct 23 11:07:23 2024] systemd[1]: Starting Journal Service...",
			expected: time.Date(2024, time.October, 23, 11, 7, 23, 0, time.UTC),
			wantErr:  false,
		},
		{
			name:     "filesystem message",
			input:    "[Wed Oct 23 11:07:23 2024] EXT4-fs (sda1): re-mounted. Opts: (null). Quota mode: none.",
			expected: time.Date(2024, time.October, 23, 11, 7, 23, 0, time.UTC),
			wantErr:  false,
		},
		{
			name:     "audit message",
			input:    "[Wed Oct 23 11:07:23 2024] audit: type=1400 audit(1729681643.552:2): apparmor=\"STATUS\" operation=\"profile_load\"",
			expected: time.Date(2024, time.October, 23, 11, 7, 23, 0, time.UTC),
			wantErr:  false,
		},
		{
			name:     "device message",
			input:    "[Wed Oct 23 11:07:24 2024] virtio_net virtio1 eth0: renamed from enp1s0",
			expected: time.Date(2024, time.October, 23, 11, 7, 24, 0, time.UTC),
			wantErr:  false,
		},
		{
			name:     "filesystem resize",
			input:    "[Wed Oct 23 11:07:27 2024] EXT4-fs (sda1): resizing filesystem from 1085696 to 59934203 blocks",
			expected: time.Date(2024, time.October, 23, 11, 7, 27, 0, time.UTC),
			wantErr:  false,
		},
		{
			name:     "loop device message",
			input:    "[Wed Oct 23 11:07:28 2024] loop0: detected capacity change from 0 to 8",
			expected: time.Date(2024, time.October, 23, 11, 7, 28, 0, time.UTC),
			wantErr:  false,
		},
		{
			name:     "no timestamp",
			input:    "some random text without timestamp",
			expected: time.Time{},
			wantErr:  true,
		},
		{
			name:     "invalid timestamp format",
			input:    "[Invalid Oct 23 11:07:23 2024] some text",
			expected: time.Time{},
			wantErr:  true,
		},
		{
			name:     "empty string",
			input:    "",
			expected: time.Time{},
			wantErr:  true,
		},
		{
			name:     "malformed brackets",
			input:    "Wed Oct 23 11:07:23 2024] some text",
			expected: time.Time{},
			wantErr:  true,
		},
		{
			name:     "valid timestamp",
			input:    "[Mon Jan 2 15:04:05 2006] Some log message",
			expected: time.Date(2006, time.January, 2, 15, 4, 5, 0, time.UTC),
			wantErr:  false,
		},
		{
			name:     "no timestamp",
			input:    "Some log message without timestamp",
			expected: time.Time{},
			wantErr:  true,
		},
		{
			name:     "invalid timestamp format",
			input:    "[Invalid timestamp] Some log message",
			expected: time.Time{},
			wantErr:  true,
		},
		{
			name:     "empty input",
			input:    "",
			expected: time.Time{},
			wantErr:  true,
		},
		{
			name:     "malformed brackets",
			input:    "[ incomplete bracket Some log message",
			expected: time.Time{},
			wantErr:  true,
		},
		{
			name:     "fabric manager log format",
			input:    "[Jul 09 2024 18:14:07] [ERROR] [tid 12727] detected NVSwitch non-fatal error 12028 on fid 0 on NVSwitch pci bus id 00000000:86:00.0 physical id 3 port 61",
			expected: time.Time{},
			wantErr:  true,
		},
		{
			name:     "alternative date format",
			input:    "[2024-07-09 18:14:07] [ERROR] [tid 12727] detected NVSwitch non-fatal error 12028 on fid 0 on NVSwitch pci bus id 00000000:86:00.0 physical id 3 port 61",
			expected: time.Time{},
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _, err := ParseCtimeWithError([]byte(tt.input))
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseCtimeWithError() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !got.Equal(tt.expected) {
				t.Errorf("ParseCtime() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestLogLineIsEmpty(t *testing.T) {
	tests := []struct {
		name string
		line LogLine
		want bool
	}{
		{
			name: "completely empty",
			line: LogLine{},
			want: true,
		},
		{
			name: "only timestamp",
			line: LogLine{
				Timestamp: time.Now(),
			},
			want: false,
		},
		{
			name: "only facility",
			line: LogLine{
				Facility: "kern",
			},
			want: false,
		},
		{
			name: "only level",
			line: LogLine{
				Level: "info",
			},
			want: false,
		},
		{
			name: "only content",
			line: LogLine{
				Content: "test message",
			},
			want: false,
		},
		{
			name: "only error",
			line: LogLine{
				Error: "test error",
			},
			want: false,
		},
		{
			name: "all fields empty strings",
			line: LogLine{
				Facility: "",
				Level:    "",
				Content:  "",
				Error:    "",
			},
			want: true,
		},
		{
			name: "all fields populated",
			line: LogLine{
				Timestamp: time.Now(),
				Facility:  "kern",
				Level:     "info",
				Content:   "test message",
				Error:     "",
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.line.IsEmpty(); got != tt.want {
				t.Errorf("LogLine.IsEmpty() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseISOtimeWithErrorEdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		line     []byte
		want     time.Time
		wantLine []byte
		wantErr  bool
	}{
		{
			name:     "microseconds overflow",
			line:     []byte("2024-11-15T12:02:03,9999999+00:00 abc"),
			want:     time.Time{},
			wantLine: nil,
			wantErr:  true,
		},
		{
			name:     "invalid timezone offset",
			line:     []byte("2024-11-15T12:02:03,561522+25:00 abc"),
			want:     time.Time{},
			wantLine: nil,
			wantErr:  true,
		},
		{
			name:     "invalid month",
			line:     []byte("2024-13-15T12:02:03,561522+00:00 abc"),
			want:     time.Time{},
			wantLine: nil,
			wantErr:  true,
		},
		{
			name:     "invalid day",
			line:     []byte("2024-11-32T12:02:03,561522+00:00 abc"),
			want:     time.Time{},
			wantLine: nil,
			wantErr:  true,
		},
		{
			name:     "invalid hour",
			line:     []byte("2024-11-15T24:02:03,561522+00:00 abc"),
			want:     time.Time{},
			wantLine: nil,
			wantErr:  true,
		},
		{
			name:     "invalid minute",
			line:     []byte("2024-11-15T12:60:03,561522+00:00 abc"),
			want:     time.Time{},
			wantLine: nil,
			wantErr:  true,
		},
		{
			name:     "invalid second",
			line:     []byte("2024-11-15T12:02:60,561522+00:00 abc"),
			want:     time.Time{},
			wantLine: nil,
			wantErr:  true,
		},
		{
			name:     "missing T separator",
			line:     []byte("2024-11-15 12:02:03,561522+00:00 abc"),
			want:     time.Time{},
			wantLine: nil,
			wantErr:  true,
		},
		{
			name:     "missing microseconds",
			line:     []byte("2024-11-15T12:02:03+00:00 abc"),
			want:     time.Time{},
			wantLine: nil,
			wantErr:  true,
		},
		{
			name:     "missing timezone",
			line:     []byte("2024-11-15T12:02:03,561522 abc"),
			want:     time.Time{},
			wantLine: nil,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, line, err := ParseISOtimeWithError(tt.line)
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
