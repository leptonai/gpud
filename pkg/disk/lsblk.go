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

// Modified from https://github.com/dell/csi-baremetal/blob/v1.7.0/pkg/base/linuxutils/lsblk/lsblk.go

package disk

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"

	"github.com/dustin/go-humanize"
	"github.com/olekukonko/tablewriter"
	"sigs.k8s.io/yaml"
)

const (
	// CmdTmpl adds device name, if add empty string - command will print info about all devices
	CmdTmpl = "lsblk %s --paths --json --bytes --fs " +
		"--output NAME,TYPE,SIZE,ROTA,SERIAL,WWN,VENDOR,MODEL,REV,MOUNTPOINT,FSTYPE,PARTUUID"
	// outputKey is the key to find block devices in lsblk json output
	outputKey = "blockdevices"
	// romDeviceType is the constant that represents rom devices to exclude them from lsblk output
	romDeviceType = "rom"
)

// GetBlockDevices run os lsblk command for device and construct BlockDevice struct based on output
// Receives device path. If device is empty string, info about all devices will be collected
// Returns slice of BlockDevice structs or error if something went wrong
func GetBlockDevices(ctx context.Context, opts ...OpOption) (BlockDevices, error) {
	op := &Op{}
	if err := op.applyOpts(opts); err != nil {
		return nil, err
	}

	cmd := fmt.Sprintf(CmdTmpl, op.device)
	out, err := exec.CommandContext(ctx, cmd).Output()
	if err != nil {
		return nil, err
	}

	return Parse(out)
}

func Parse(b []byte) (BlockDevices, error) {
	println()
	println()
	println()
	println()
	fmt.Println("string(b)", string(b))
	println()
	println()
	println()
	println()

	raw := make(map[string]BlockDevices, 1)
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, err
	}

	rawDevs, ok := raw[outputKey]
	if !ok {
		return nil, fmt.Errorf("unexpected lsblk output format, missing %q key", outputKey)
	}

	devs := make(BlockDevices, 0)
	for _, d := range rawDevs {
		if d.Type == romDeviceType {
			continue
		}

		d.SizeHumanized = humanize.Bytes(uint64(d.Size.Int64))
		devs = append(devs, d)
	}
	return devs, nil
}

type BlockDevices []BlockDevice

func (blks BlockDevices) JSON() ([]byte, error) {
	return json.Marshal(blks)
}

func (blks BlockDevices) YAML() ([]byte, error) {
	return yaml.Marshal(blks)
}

func (blks BlockDevices) RenderTable(wr io.Writer) {
	table := tablewriter.NewWriter(wr)
	table.SetHeader([]string{"Name", "Type", "Size", "Mount Point"})

	for _, blk := range blks {
		table.Append([]string{
			blk.Name,
			blk.Type,
			blk.SizeHumanized,
			blk.MountPoint,
		})
	}

	table.Render()
}

// BlockDevice is the struct that represents output of lsblk command for a device
type BlockDevice struct {
	Name          string        `json:"name,omitempty"`
	Type          string        `json:"type,omitempty"`
	Size          CustomInt64   `json:"size,omitempty"`
	SizeHumanized string        `json:"size_humanized,omitempty"`
	Rota          CustomBool    `json:"rota,omitempty"`
	Serial        string        `json:"serial,omitempty"`
	WWN           string        `json:"wwn,omitempty"`
	Vendor        string        `json:"vendor,omitempty"`
	Model         string        `json:"model,omitempty"`
	Rev           string        `json:"rev,omitempty"`
	MountPoint    string        `json:"mountpoint,omitempty"`
	FSType        string        `json:"fstype,omitempty"`
	PartUUID      string        `json:"partuuid,omitempty"`
	Children      []BlockDevice `json:"children,omitempty"`
}

// CustomInt64 to handle Size lsblk output - 8001563222016 or "8001563222016"
type CustomInt64 struct {
	Int64 int64
}

// UnmarshalJSON customizes string size unmarshalling
func (ci *CustomInt64) UnmarshalJSON(data []byte) error {
	QuotesByte := byte(34)
	if data[0] == QuotesByte {
		err := json.Unmarshal(data[1:len(data)-1], &ci.Int64)
		if err != nil {
			return errors.New("CustomInt64: UnmarshalJSON: " + err.Error())
		}
	} else {
		err := json.Unmarshal(data, &ci.Int64)
		if err != nil {
			return errors.New("CustomInt64: UnmarshalJSON: " + err.Error())
		}
	}
	return nil
}

// MarshalJSON customizes size marshalling
func (ci *CustomInt64) MarshalJSON() ([]byte, error) {
	return json.Marshal(ci.Int64)
}

// CustomBool to handle Rota lsblk output - true/false or "1"/"0"
type CustomBool struct {
	Bool bool
}

// UnmarshalJSON customizes string rota unmarshalling
func (cb *CustomBool) UnmarshalJSON(data []byte) error {
	switch string(data) {
	case `"true"`, `true`, `"1"`, `1`:
		cb.Bool = true
		return nil
	case `"false"`, `false`, `"0"`, `0`, `""`:
		cb.Bool = false
		return nil
	default:
		return errors.New("CustomBool: parsing \"" + string(data) + "\": unknown value")
	}
}

// MarshalJSON customizes rota marshalling
func (cb CustomBool) MarshalJSON() ([]byte, error) {
	return json.Marshal(cb.Bool)
}
