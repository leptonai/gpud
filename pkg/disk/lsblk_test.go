/*
Copyright © 2020-2024 Dell Inc. or its subsidiaries. All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

   http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Modified from https://github.com/dell/csi-baremetal/blob/v1.7.0/pkg/base/linuxutils/lsblk/lsblk_test.go

package disk

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/dustin/go-humanize"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParse(t *testing.T) {
	t.Parallel()

	for _, f := range []string{
		"lsblk.1.json",
		"lsblk.2.json",
		"lsblk.3.json",
	} {
		dat, err := os.ReadFile("testdata/" + f)
		if err != nil {
			t.Fatal(err)
		}

		blks, err := parseLsblkJSON(dat)
		if err != nil {
			t.Fatal(err)
		}
		blks.RenderTable(os.Stdout)

		blks, err = parseLsblkJSON(dat, WithDeviceType(func(deviceType string) bool {
			return deviceType == "disk"
		}))
		if err != nil {
			t.Fatal(err)
		}
		blks.RenderTable(os.Stdout)

		totalBytes := blks.GetTotalBytes()
		t.Logf("Total bytes: %s", humanize.Bytes(totalBytes))
	}
}

func TestParseWithMultipleDevices(t *testing.T) {
	t.Parallel()

	dat, err := os.ReadFile("testdata/lsblk.3.json")
	if err != nil {
		t.Fatal(err)
	}

	blks, err := parseLsblkJSON(dat)
	if err != nil {
		t.Fatal(err)
	}
	blks.RenderTable(os.Stdout)

	blks, err = parseLsblkJSON(dat, WithDeviceType(func(deviceType string) bool {
		return deviceType == "disk"
	}))
	if err != nil {
		t.Fatal(err)
	}
	blks.RenderTable(os.Stdout)

	for _, blk := range blks {
		if !strings.HasPrefix(blk.Name, "nvme") {
			continue
		}

		t.Logf("Device: %s, Size: %s", blk.Name, humanize.Bytes(blk.Size.Uint64))
	}
}

func TestParsePairs(t *testing.T) {
	t.Parallel()

	for _, f := range []string{"lsblk.3.txt"} {
		dat, err := os.ReadFile("testdata/" + f)
		if err != nil {
			t.Fatal(err)
		}

		blks, err := parseLsblkPairs(dat, WithDeviceType(func(deviceType string) bool {
			return deviceType == "disk"
		}))
		if err != nil {
			t.Fatal(err)
		}

		blks.RenderTable(os.Stdout)
		totalBytes := blks.GetTotalBytes()
		t.Logf("Total bytes: %s", humanize.Bytes(totalBytes))
	}
}

func TestCheckVersion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		input         string
		expectedFlags string
		expectError   bool
	}{
		{
			name:          "Chinese locale version 2.23.2",
			input:         "lsblk，来自 util-linux 2.23.2",
			expectedFlags: "--paths --bytes --fs --output NAME,TYPE,SIZE,ROTA,SERIAL,WWN,VENDOR,MODEL,REV,MOUNTPOINT,FSTYPE,FSUSED,PARTUUID --pairs",
			expectError:   false,
		},
		{
			name:          "English locale version 2.37.2",
			input:         "lsblk from util-linux 2.37.2",
			expectedFlags: "--paths --bytes --fs --output NAME,TYPE,SIZE,ROTA,SERIAL,WWN,VENDOR,MODEL,REV,MOUNTPOINT,FSTYPE,FSUSED,PARTUUID --json",
			expectError:   false,
		},
		{
			name:          "English locale version 2.37.4",
			input:         "lsblk from util-linux 2.37.4",
			expectedFlags: "--paths --bytes --fs --output NAME,TYPE,SIZE,ROTA,SERIAL,WWN,VENDOR,MODEL,REV,MOUNTPOINT,FSTYPE,FSUSED,PARTUUID --json",
			expectError:   false,
		},
		{
			name:          "Empty string",
			input:         "",
			expectedFlags: "",
			expectError:   true,
		},
		{
			name:          "Invalid version format",
			input:         "lsblk from util-linux abc.def",
			expectedFlags: "",
			expectError:   true,
		},
		{
			name:          "No version number",
			input:         "lsblk from util-linux",
			expectedFlags: "",
			expectError:   true,
		},
	}

	for _, tt := range tests {
		tt := tt // capture range variable
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			flags, _, err := decideLsblkFlag(tt.input)
			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				if flags != tt.expectedFlags {
					t.Errorf("Expected flags %q, got %q", tt.expectedFlags, flags)
				}
			}
		})
	}
}

func Test_parseLsblkSize(t *testing.T) {
	got, err := parseLsblkSize([]byte("     \"64.9M\"        "))
	require.NoError(t, err)
	require.Equal(t, uint64(64900000), got)

	got, err = parseLsblkSize([]byte("     \"   64.9M  \"        "))
	require.NoError(t, err)
	require.Equal(t, uint64(64900000), got)

	got, err = parseLsblkSize([]byte("\"64.9M\""))
	require.NoError(t, err)
	require.Equal(t, uint64(64900000), got)

	got, err = parseLsblkSize([]byte("64.9M"))
	require.NoError(t, err)
	require.Equal(t, uint64(64900000), got)

	got, err = parseLsblkSize([]byte("  \"894.3G\" "))
	require.NoError(t, err)
	require.Equal(t, uint64(894300000000), got)

	got, err = parseLsblkSize([]byte("  \"  894.3G \" "))
	require.NoError(t, err)
	require.Equal(t, uint64(894300000000), got)

	got, err = parseLsblkSize([]byte("    894.3G  "))
	require.NoError(t, err)
	require.Equal(t, uint64(894300000000), got)

	got, err = parseLsblkSize([]byte("894.3G"))
	require.NoError(t, err)
	require.Equal(t, uint64(894300000000), got)
}

func TestCustomUint64_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    uint64
		wantErr bool
	}{
		{
			name:    "numeric string",
			input:   `"8001563222016"`,
			want:    8001563222016,
			wantErr: false,
		},
		{
			name:    "human readable string",
			input:   `"63.9M"`,
			want:    63900000,
			wantErr: false,
		},
		{
			name:    "numeric value",
			input:   `8001563222016`,
			want:    8001563222016,
			wantErr: false,
		},
		{
			name:    "empty string",
			input:   `""`,
			want:    0,
			wantErr: false,
		},
		{
			name:    "null value",
			input:   `null`,
			want:    0,
			wantErr: false,
		},
		{
			name:    "invalid human readable string",
			input:   `"invalid"`,
			want:    0,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var ci CustomUint64
			err := json.Unmarshal([]byte(tt.input), &ci)

			if (err != nil) != tt.wantErr {
				t.Errorf("CustomUint64.UnmarshalJSON() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && ci.Uint64 != tt.want {
				t.Errorf("CustomUint64.UnmarshalJSON() got = %v, want %v", ci.Uint64, tt.want)
			}
		})
	}
}

func TestCustomUint64_MarshalJSON(t *testing.T) {
	tests := []struct {
		name  string
		value uint64
		want  string
	}{
		{
			name:  "zero value",
			value: 0,
			want:  "0",
		},
		{
			name:  "non-zero value",
			value: 8001563222016,
			want:  "8001563222016",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ci := CustomUint64{Uint64: tt.value}
			got, err := ci.MarshalJSON()
			if err != nil {
				t.Errorf("CustomUint64.MarshalJSON() error = %v", err)
				return
			}

			if string(got) != tt.want {
				t.Errorf("CustomUint64.MarshalJSON() = %v, want %v", string(got), tt.want)
			}
		})
	}
}

func TestGetBlockDevicesWithMountPointFilter(t *testing.T) {
	t.Parallel()

	// Create a temporary file for mock lsblk output
	tmpfile, err := os.CreateTemp("", "lsblk-test-*.json")
	require.NoError(t, err)
	defer os.Remove(tmpfile.Name())

	jsonData := `{
		"blockdevices": [
			{
				"name": "/dev/vda1",
				"type": "part",
				"size": 8001563222016,
				"mountpoint": "/",
				"fstype": "ext4"
			},
			{
				"name": "/dev/vda2",
				"type": "part",
				"size": 1073741824,
				"mountpoint": "",
				"fstype": "swap"
			},
			{
				"name": "/dev/vda3",
				"type": "part",
				"size": 1073741824,
				"mountpoint": "/mnt/customfs",
				"fstype": "virtiofs"
			},
			{
				"name": "/dev/vda4",
				"type": "part",
				"size": 1073741824,
				"mountpoint": "/mnt/cloud-metadata",
				"fstype": "virtiofs"
			},
			{
				"name": "/dev/vda5",
				"type": "part",
				"size": 1073741824,
				"mountpoint": "/home",
				"fstype": "ext4"
			}
		]
	}`

	// Test with parseLsblkJSON directly (no mount point filtering at parse level)
	blks, err := parseLsblkJSON([]byte(jsonData))
	require.NoError(t, err)
	require.Len(t, blks, 5) // All devices are parsed

	// Test filtering logic that would be applied by getBlockDevicesWithLsblk
	op := &Op{}
	err = op.applyOpts([]OpOption{WithMountPoint(DefaultMountPointFunc)})
	require.NoError(t, err)

	filteredBlks := make(BlockDevices, 0)
	for _, blk := range blks {
		if op.matchFuncMountPoint(blk.MountPoint) {
			filteredBlks = append(filteredBlks, blk)
		}
	}

	// Should have 3 devices (/, /home, /mnt/customfs)
	// Only empty mount points and /mnt/cloud-metadata are filtered out
	require.Len(t, filteredBlks, 3)

	// Verify the correct devices were kept
	mountPoints := make(map[string]bool)
	for _, blk := range filteredBlks {
		mountPoints[blk.MountPoint] = true
	}

	assert.True(t, mountPoints["/"], "Should include root mount point")
	assert.True(t, mountPoints["/home"], "Should include /home mount point")
	assert.True(t, mountPoints["/mnt/customfs"], "Should include /mnt/customfs")
	assert.False(t, mountPoints[""], "Should not include empty mount point")
	assert.False(t, mountPoints["/mnt/cloud-metadata"], "Should not include /mnt/cloud-metadata")
}

func TestGetBlockDevicesWithCustomMountPointFilter(t *testing.T) {
	t.Parallel()

	// Custom filter that only accepts root
	rootOnlyFilter := func(mountPoint string) bool {
		return mountPoint == "/"
	}

	jsonData := `{
		"blockdevices": [
			{
				"name": "/dev/vda1",
				"type": "part",
				"size": 8001563222016,
				"mountpoint": "/",
				"fstype": "ext4"
			},
			{
				"name": "/dev/vda2",
				"type": "part",
				"size": 1073741824,
				"mountpoint": "/home",
				"fstype": "ext4"
			},
			{
				"name": "/dev/vda3",
				"type": "part",
				"size": 1073741824,
				"mountpoint": "/var",
				"fstype": "ext4"
			}
		]
	}`

	// Parse all devices first
	blks, err := parseLsblkJSON([]byte(jsonData))
	require.NoError(t, err)
	require.Len(t, blks, 3)

	// Apply custom mount point filter
	op := &Op{}
	err = op.applyOpts([]OpOption{WithMountPoint(rootOnlyFilter)})
	require.NoError(t, err)

	filteredBlks := make(BlockDevices, 0)
	for _, blk := range blks {
		if op.matchFuncMountPoint(blk.MountPoint) {
			filteredBlks = append(filteredBlks, blk)
		}
	}

	// Should only have 1 device (/)
	require.Len(t, filteredBlks, 1)
	assert.Equal(t, "/", filteredBlks[0].MountPoint)
	assert.Equal(t, "/dev/vda1", filteredBlks[0].Name)
}

func TestGetBlockDevicesCombinedFilters(t *testing.T) {
	t.Parallel()

	jsonData := `{
		"blockdevices": [
			{
				"name": "/dev/vda1",
				"type": "part",
				"size": 8001563222016,
				"mountpoint": "/",
				"fstype": "ext4"
			},
			{
				"name": "/dev/vda2",
				"type": "disk",
				"size": 1073741824,
				"mountpoint": "/data",
				"fstype": "ext4"
			},
			{
				"name": "/dev/vda3",
				"type": "part",
				"size": 1073741824,
				"mountpoint": "/mnt/customfs",
				"fstype": "virtiofs"
			},
			{
				"name": "/dev/vda4",
				"type": "loop",
				"size": 1073741824,
				"mountpoint": "/snap",
				"fstype": "squashfs"
			}
		]
	}`

	// Parse with device type filter only (mount point filter is applied later)
	blks, err := parseLsblkJSON([]byte(jsonData),
		WithDeviceType(func(dt string) bool {
			return dt == "part" || dt == "disk"
		}))
	require.NoError(t, err)

	// After device type filtering, should have 3 devices (vda1, vda2, vda3)
	require.Len(t, blks, 3)

	// Now apply mount point filter manually (as done in getBlockDevicesWithLsblk)
	op := &Op{}
	err = op.applyOpts([]OpOption{WithMountPoint(DefaultMountPointFunc)})
	require.NoError(t, err)

	filteredBlks := make(BlockDevices, 0)
	for _, blk := range blks {
		if op.matchFuncMountPoint(blk.MountPoint) {
			filteredBlks = append(filteredBlks, blk)
		}
	}

	// Should have 3 devices: vda1 (part, /), vda2 (disk, /data), and vda3 (part, /mnt/customfs)
	// Only /mnt/cloud-metadata is filtered by mount point, vda4 was already filtered by device type
	require.Len(t, filteredBlks, 3)

	names := make(map[string]bool)
	for _, blk := range filteredBlks {
		names[blk.Name] = true
	}

	assert.True(t, names["/dev/vda1"], "Should include vda1")
	assert.True(t, names["/dev/vda2"], "Should include vda2")
	assert.True(t, names["/dev/vda3"], "Should include vda3 (/mnt/customfs is allowed)")
	assert.False(t, names["/dev/vda4"], "Should not include vda4 (filtered by device type)")
}
