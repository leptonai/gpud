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

func TestHasRAIDArrayFailure(t *testing.T) {
	tests := []struct {
		name string
		line string
		want bool
	}{
		{
			name: "RAID0 failure on nvme0n1p1",
			line: "md/raid0md0: Disk failure on nvme0n1p1 detected, failing array.",
			want: true,
		},
		{
			name: "RAID0 failure on nvme1n1p1",
			line: "md/raid0md0: Disk failure on nvme1n1p1 detected, failing array.",
			want: true,
		},
		{
			name: "RAID0 failure with kernel prefix",
			line: "kernel: md/raid0md0: Disk failure on nvme0n1p1 detected, failing array.",
			want: true,
		},
		{
			name: "RAID1 failure",
			line: "md/raid1:md1: Disk failure on sda1 detected, failing array",
			want: true,
		},
		{
			name: "No match - different message",
			line: "md: raid1 personality registered for level 1",
			want: false,
		},
		{
			name: "No match - partial message",
			line: "md/raid0: Disk failure detected",
			want: false,
		},
		{
			name: "RAID10 failure on nvme device",
			line: "md/raid10:md2: Disk failure on nvme2n1p1 detected, failing array",
			want: true,
		},
		{
			name: "RAID5 failure",
			line: "md/raid5:md5: Disk failure on sdb3 detected, failing array.",
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, HasRAIDArrayFailure(tt.line), "HasRAIDArrayFailure()")
		})
	}
}

func TestHasFilesystemReadOnly(t *testing.T) {
	tests := []struct {
		name string
		line string
		want bool
	}{
		{
			name: "EXT4 remounting read-only",
			line: "EXT4-fs (md0): Remounting filesystem read-only",
			want: true,
		},
		{
			name: "EXT4 remounting read-only with different device",
			line: "EXT4-fs (sda1): Remounting filesystem read-only",
			want: true,
		},
		{
			name: "EXT4 with prefix",
			line: "[Fri Jul  4 10:29:39 2025] EXT4-fs (md0): Remounting filesystem read-only",
			want: true,
		},
		{
			name: "No match - different EXT4 message",
			line: "EXT4-fs (sda1): mounted filesystem with ordered data mode",
			want: false,
		},
		{
			name: "No match - not read-only",
			line: "EXT4-fs: Remounting filesystem read-write",
			want: false,
		},
		{
			name: "XFS remounting read-only",
			line: "XFS (sda1): Remounting filesystem read-only",
			want: true,
		},
		{
			name: "BTRFS remounting read-only",
			line: "BTRFS: Remounting filesystem read-only",
			want: true,
		},
		{
			name: "EXT3 remounting read-only",
			line: "EXT3-fs (dm-6): Remounting filesystem read-only",
			want: true,
		},
		{
			name: "Generic remounting read-only without filesystem prefix",
			line: "Remounting filesystem read-only",
			want: true,
		},
		{
			name: "XFS with timestamp prefix",
			line: "[Mon Jan  1 10:00:00 2025] XFS (nvme0n1p1): Remounting filesystem read-only",
			want: true,
		},
		{
			name: "BTRFS with kernel prefix",
			line: "kernel: BTRFS: Remounting filesystem read-only",
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, HasFilesystemReadOnly(tt.line), "HasFilesystemReadOnly()")
		})
	}
}

func TestHasNVMePathFailure(t *testing.T) {
	tests := []struct {
		name string
		line string
		want bool
	}{
		{
			name: "NVMe path failure nvme0n1",
			line: "block nvme0n1: no available path - failing I/O",
			want: true,
		},
		{
			name: "NVMe path failure nvme1n1",
			line: "block nvme1n1: no available path - failing I/O",
			want: true,
		},
		{
			name: "NVMe path failure with prefix",
			line: "[Fri Jul  4 10:29:37 2025] block nvme0n1: no available path - failing I/O",
			want: true,
		},
		{
			name: "NVMe path failure with kernel prefix",
			line: "kernel: block nvme0n1: no available path - failing I/O",
			want: true,
		},
		{
			name: "Message repeated notation with NVMe failure",
			line: "kernel: message repeated 9 times: [block nvme0n1: no available path - failing I/O]",
			want: true,
		},
		{
			name: "No match - different nvme message",
			line: "nvme nvme0: pci function 0000:01:00.0",
			want: false,
		},
		{
			name: "No match - partial message",
			line: "block nvme0n1: no available path",
			want: false,
		},
		{
			name: "NVMe path failure on different device",
			line: "block nvme2n1: no available path - failing I/O",
			want: true,
		},
		{
			name: "NVMe path failure nvme10n1",
			line: "block nvme10n1: no available path - failing I/O",
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, HasNVMePathFailure(tt.line), "HasNVMePathFailure()")
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
			name:          "RAID array failure",
			line:          "md/raid0md0: Disk failure on nvme0n1p1 detected, failing array.",
			wantEventName: eventRAIDArrayFailure,
			wantMessage:   messageRAIDArrayFailure,
		},
		{
			name:          "Filesystem read-only",
			line:          "EXT4-fs (md0): Remounting filesystem read-only",
			wantEventName: eventFilesystemReadOnly,
			wantMessage:   messageFilesystemReadOnly,
		},
		{
			name:          "XFS filesystem read-only",
			line:          "XFS (sda1): Remounting filesystem read-only",
			wantEventName: eventFilesystemReadOnly,
			wantMessage:   messageFilesystemReadOnly,
		},
		{
			name:          "BTRFS filesystem read-only",
			line:          "BTRFS: Remounting filesystem read-only",
			wantEventName: eventFilesystemReadOnly,
			wantMessage:   messageFilesystemReadOnly,
		},
		{
			name:          "Generic filesystem read-only",
			line:          "Remounting filesystem read-only",
			wantEventName: eventFilesystemReadOnly,
			wantMessage:   messageFilesystemReadOnly,
		},
		{
			name:          "NVMe path failure",
			line:          "block nvme0n1: no available path - failing I/O",
			wantEventName: eventNVMePathFailure,
			wantMessage:   messageNVMePathFailure,
		},
		{
			name:          "NVMe path failure with kernel prefix",
			line:          "kernel: block nvme0n1: no available path - failing I/O",
			wantEventName: eventNVMePathFailure,
			wantMessage:   messageNVMePathFailure,
		},
		{
			name:          "RAID failure with kernel prefix",
			line:          "kernel: md/raid0md0: Disk failure on nvme0n1p1 detected, failing array.",
			wantEventName: eventRAIDArrayFailure,
			wantMessage:   messageRAIDArrayFailure,
		},
		{
			name:          "Message repeated with NVMe failure",
			line:          "kernel: message repeated 9 times: [block nvme0n1: no available path - failing I/O]",
			wantEventName: eventNVMePathFailure,
			wantMessage:   messageNVMePathFailure,
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
