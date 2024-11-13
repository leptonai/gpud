package dmesg

import (
	"testing"
	"time"
)

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
			got, err := ParseCtimeWithError([]byte(tt.input))
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
