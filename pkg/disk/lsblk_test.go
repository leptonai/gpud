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
	"reflect"
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

	expecteds := []string{
		"--paths --bytes --fs --output NAME,TYPE,SIZE,ROTA,SERIAL,WWN,VENDOR,MODEL,REV,MOUNTPOINT,FSTYPE,PARTUUID --pairs",
		"--paths --bytes --fs --output NAME,TYPE,SIZE,ROTA,SERIAL,WWN,VENDOR,MODEL,REV,MOUNTPOINT,FSTYPE,PARTUUID --json",
		"--paths --bytes --fs --output NAME,TYPE,SIZE,ROTA,SERIAL,WWN,VENDOR,MODEL,REV,MOUNTPOINT,FSTYPE,PARTUUID --json",
	}

	for i, s := range []string{"lsblk，来自 util-linux 2.23.2", "lsblk from util-linux 2.37.2", "lsblk from util-linux 2.37.4"} {
		lsblkCmd, _, err := decideLsblkFlagAndParserFromVersion(s)
		if err != nil {
			t.Errorf("Expected %v, got %v", expecteds[i], lsblkCmd)
		}
		if !reflect.DeepEqual(lsblkCmd, expecteds[i]) {
			t.Errorf("Expected %v, got %v", expecteds[i], lsblkCmd)
		}
	}
}
