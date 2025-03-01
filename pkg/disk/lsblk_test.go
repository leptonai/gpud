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
	"os"
	"testing"

	"github.com/dustin/go-humanize"
)

func TestParse(t *testing.T) {
	t.Parallel()

	for _, f := range []string{"lsblk.1.json", "lsblk.2.json"} {
		dat, err := os.ReadFile("testdata/" + f)
		if err != nil {
			t.Fatal(err)
		}

		blks, err := ParseJSON(dat)
		if err != nil {
			t.Fatal(err)
		}
		blks.RenderTable(os.Stdout)

		blks, err = ParseJSON(dat, WithDeviceType(func(deviceType string) bool {
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

func TestParsePairs(t *testing.T) {
	t.Parallel()

	for _, f := range []string{"lsblk.3.txt"} {
		dat, err := os.ReadFile("testdata/" + f)
		if err != nil {
			t.Fatal(err)
		}

		blks, err := ParsePairs(dat, WithDeviceType(func(deviceType string) bool {
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
			expectedFlags: "--paths --bytes --fs --output NAME,TYPE,SIZE,ROTA,SERIAL,WWN,VENDOR,MODEL,REV,MOUNTPOINT,FSTYPE,PARTUUID --pairs",
			expectError:   false,
		},
		{
			name:          "English locale version 2.37.2",
			input:         "lsblk from util-linux 2.37.2",
			expectedFlags: "--paths --bytes --fs --output NAME,TYPE,SIZE,ROTA,SERIAL,WWN,VENDOR,MODEL,REV,MOUNTPOINT,FSTYPE,PARTUUID --json",
			expectError:   false,
		},
		{
			name:          "English locale version 2.37.4",
			input:         "lsblk from util-linux 2.37.4",
			expectedFlags: "--paths --bytes --fs --output NAME,TYPE,SIZE,ROTA,SERIAL,WWN,VENDOR,MODEL,REV,MOUNTPOINT,FSTYPE,PARTUUID --json",
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
			flags, _, err := decideLsblkFlagAndParserFromVersion(tt.input)
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
