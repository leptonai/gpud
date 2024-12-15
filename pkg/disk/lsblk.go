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
	"sort"
	"strings"

	"github.com/dustin/go-humanize"
	"github.com/olekukonko/tablewriter"
	"sigs.k8s.io/yaml"

	"github.com/leptonai/gpud/pkg/file"
	"github.com/leptonai/gpud/pkg/process"
)

const (
	// lsblkFlags adds device name, if add empty string - command will print info about all devices
	lsblkFlags = "--paths --json --bytes --fs --output NAME,TYPE,SIZE,ROTA,SERIAL,WWN,VENDOR,MODEL,REV,MOUNTPOINT,FSTYPE,PARTUUID"
	// outputKey is the key to find block devices in lsblk json output
	outputKey = "blockdevices"
)

// GetBlockDevices run os lsblk command for device and construct BlockDevice struct based on output
// Receives device path. If device is empty string, info about all devices will be collected
// Returns slice of BlockDevice structs or error if something went wrong
func GetBlockDevices(ctx context.Context, opts ...OpOption) (BlockDevices, error) {
	lsblkPath, err := file.LocateExecutable("lsblk")
	if err != nil {
		return nil, nil
	}

	p, err := process.New(
		process.WithCommand(lsblkPath+" "+lsblkFlags),
		process.WithRunAsBashScript(),
	)
	if err != nil {
		return nil, err
	}

	if err := p.Start(ctx); err != nil {
		return nil, err
	}

	lines := make([]string, 0)
	if err := process.Read(
		ctx,
		p,
		process.WithReadStdout(),
		process.WithReadStderr(),
		process.WithProcessLine(func(line string) {
			lines = append(lines, line)
		}),
		process.WithWaitForCmd(),
	); err != nil {
		return nil, fmt.Errorf("failed to read lsblk output: %w\n\noutput:\n%s", err, strings.Join(lines, "\n"))
	}

	return Parse([]byte(strings.Join(lines, "\n")), opts...)
}

func Parse(b []byte, opts ...OpOption) (BlockDevices, error) {
	if len(b) == 0 {
		return nil, errors.New("empty input provided to Parse")
	}

	op := &Op{}
	if err := op.applyOpts(opts); err != nil {
		return nil, err
	}

	raw := make(map[string]BlockDevices, 1)
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, fmt.Errorf("failed to unmarshal lsblk output (len=%d): %w, raw input: %q", len(b), err, string(b))
	}

	rawDevs, ok := raw[outputKey]
	if !ok {
		return nil, fmt.Errorf("unexpected lsblk output format, missing %q key", outputKey)
	}

	devs := make(BlockDevices, 0)
	for _, parentDev := range rawDevs {
		if !op.matchFuncFstype(parentDev.FSType) {
			continue
		}
		if !op.matchFuncDeviceType(parentDev.Type) {
			continue
		}

		parentDev.SizeHumanized = humanize.Bytes(uint64(parentDev.Size.Int64))

		children := make([]BlockDevice, 0)
		for _, child := range parentDev.Children {
			if !op.matchFuncFstype(child.FSType) {
				continue
			}
			if !op.matchFuncDeviceType(child.Type) {
				continue
			}

			child.ParentDeviceName = parentDev.Name
			child.SizeHumanized = humanize.Bytes(uint64(child.Size.Int64))
			children = append(children, child)
		}
		parentDev.Children = children

		devs = append(devs, parentDev)
	}

	sort.Slice(devs, func(i, j int) bool {
		return devs[i].Name < devs[j].Name
	})
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
	table.SetHeader([]string{"Name", "Parent", "Type", "FSType", "Size", "Mount Point"})

	for _, blk := range blks {
		table.Append([]string{
			blk.Name,
			"",
			blk.Type,
			blk.FSType,
			blk.SizeHumanized,
			blk.MountPoint,
		})

		for _, child := range blk.Children {
			table.Append([]string{
				child.Name,
				child.ParentDeviceName,
				child.Type,
				child.FSType,
				child.SizeHumanized,
				child.MountPoint,
			})
		}
	}

	table.Render()
}

// Returns the total bytes of all block devices.
func (blks BlockDevices) GetTotalBytes() uint64 {
	var total uint64
	for _, blk := range blks {
		total += uint64(blk.Size.Int64)
	}
	return total
}

// BlockDevice is the struct that represents output of lsblk command for a device
type BlockDevice struct {
	Name             string        `json:"name,omitempty"`
	ParentDeviceName string        `json:"parent_device_name,omitempty"`
	Type             string        `json:"type,omitempty"`
	Size             CustomInt64   `json:"size,omitempty"`
	SizeHumanized    string        `json:"size_humanized,omitempty"`
	Rota             CustomBool    `json:"rota,omitempty"`
	Serial           string        `json:"serial,omitempty"`
	WWN              string        `json:"wwn,omitempty"`
	Vendor           string        `json:"vendor,omitempty"`
	Model            string        `json:"model,omitempty"`
	Rev              string        `json:"rev,omitempty"`
	MountPoint       string        `json:"mountpoint,omitempty"`
	FSType           string        `json:"fstype,omitempty"`
	PartUUID         string        `json:"partuuid,omitempty"`
	Children         []BlockDevice `json:"children,omitempty"`
}

// CustomInt64 to handle Size lsblk output - 8001563222016 or "8001563222016"
type CustomInt64 struct {
	Int64 int64
}

// UnmarshalJSON customizes string size unmarshaling
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

// MarshalJSON customizes size marshaling
func (ci *CustomInt64) MarshalJSON() ([]byte, error) {
	return json.Marshal(ci.Int64)
}

// CustomBool to handle Rota lsblk output - true/false or "1"/"0"
type CustomBool struct {
	Bool bool
}

// UnmarshalJSON customizes string rota unmarshaling
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

// MarshalJSON customizes rota marshaling
func (cb CustomBool) MarshalJSON() ([]byte, error) {
	return json.Marshal(cb.Bool)
}
