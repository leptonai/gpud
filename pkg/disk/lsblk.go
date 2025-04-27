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
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/dustin/go-humanize"
	"github.com/olekukonko/tablewriter"
	"sigs.k8s.io/yaml"

	"github.com/leptonai/gpud/pkg/file"
	"github.com/leptonai/gpud/pkg/log"
	"github.com/leptonai/gpud/pkg/process"
)

// GetBlockDevicesWithLsblk run "lsblk" command for device and construct BlockDevice struct based on output
// Receives device path. If device is empty string, info about all devices will be collected
// Returns slice of BlockDevice structs or error if something went wrong
func GetBlockDevicesWithLsblk(ctx context.Context, opts ...OpOption) (BlockDevices, error) {
	lsblkPath, err := file.LocateExecutable("lsblk")
	if err != nil {
		return nil, err
	}

	// pre-check lsblk version
	flags, parseFunc, checkErr := decideLsblkFlag(ctx)
	if checkErr != nil {
		log.Logger.Warnw("failed to decide lsblk flag and parser -- falling back to latest version", "error", checkErr)
		flags, parseFunc = lsblkFlags+" "+lsblkJsonFlag, parseLsblkJSON
	}

	p, err := process.New(
		process.WithCommand(lsblkPath+" "+flags),
		process.WithRunAsBashScript(),
	)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := p.Close(ctx); err != nil {
			log.Logger.Warnw("failed to abort command", "err", err)
		}
	}()

	b, err := p.StartAndWaitForCombinedOutput(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to run lsblk command: %w", err)
	}

	return parseFunc(b, opts...)
}

const (
	lsblkVersionFlags = "--version"
	// lsblkFlags adds device name, if add empty string - command will print info about all devices
	lsblkFlags = "--paths --bytes --fs --output NAME,TYPE,SIZE,ROTA,SERIAL,WWN,VENDOR,MODEL,REV,MOUNTPOINT,FSTYPE,PARTUUID"
	// lsblkJsonFlag lsblk from version 2.37 support json response
	lsblkJsonFlag = "--json"
	// lsblkMinSupportJsonVersion lsblk from version 2.37 support json response
	// https://github.com/util-linux/util-linux/blob/stable/v2.27/misc-utils/lsblk.c#L1626
	lsblkMinSupportJsonVersion = 2.37
	// lsblkPairsFlag lsblk lower than 2.37 only support raw and pairs response
	lsblkPairsFlag = "--pairs"
	// outputKey is the key to find block devices in lsblk json output
	outputKey = "blockdevices"
)

var lsblkVersionRegPattern = regexp.MustCompile(`\d+\.\d+`)

// decideLsblkFlagAndParserFromVersion decides the lsblk command flags based on the "lsblk --version" output
func decideLsblkFlagAndParserFromVersion(verOutput string) (string, func([]byte, ...OpOption) (BlockDevices, error), error) {
	matches := lsblkVersionRegPattern.FindString(verOutput)
	if matches != "" {
		if versionF, parseErr := strconv.ParseFloat(matches, 64); parseErr == nil {
			if versionF >= lsblkMinSupportJsonVersion {
				return lsblkFlags + " " + lsblkJsonFlag, parseLsblkJSON, nil
			}

			return lsblkFlags + " " + lsblkPairsFlag, parseLsblkPairs, nil
		}
	}

	return "", nil, fmt.Errorf("failed to parse 'lsblk --version' output: %q", verOutput)
}

func decideLsblkFlag(ctx context.Context) (string, func([]byte, ...OpOption) (BlockDevices, error), error) {
	lsblkVersion, err := file.LocateExecutable("lsblk")
	if err != nil {
		return "", nil, err
	}

	p, err := process.New(
		process.WithCommand(lsblkVersion+" "+lsblkVersionFlags),
		process.WithRunAsBashScript(),
	)
	if err != nil {
		return "", nil, err
	}

	if err := p.Start(ctx); err != nil {
		return "", nil, err
	}
	defer func() {
		if err := p.Close(ctx); err != nil {
			log.Logger.Warnw("failed to abort command", "err", err)
		}
	}()

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
		return "", nil, fmt.Errorf("failed to check lsblk version: %w", err)
	}

	line := strings.Join(lines, "\n")
	line = strings.TrimSpace(line)

	return decideLsblkFlagAndParserFromVersion(line)
}

// parseLsblkJSON parses the "lsblk --json" output.
func parseLsblkJSON(b []byte, opts ...OpOption) (BlockDevices, error) {
	if len(b) == 0 {
		return nil, errors.New("empty input provided to Parse")
	}

	op := &Op{}
	if err := op.applyOpts(opts); err != nil {
		return nil, err
	}

	raw := make(map[string]BlockDevices, 1)
	if err := json.Unmarshal(b, &raw); err != nil {
		log.Logger.Debugw("failed to unmarshal lsblk output", "error", err, "bytes", len(b), "raw_input", string(b))
		return nil, fmt.Errorf("failed to unmarshal lsblk output (len=%d): %w", len(b), err)
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

		parentDev.SizeHumanized = humanize.Bytes(uint64(parentDev.Size.Uint64))

		children := make([]BlockDevice, 0)
		for _, child := range parentDev.Children {
			if !op.matchFuncFstype(child.FSType) {
				continue
			}
			if !op.matchFuncDeviceType(child.Type) {
				continue
			}

			child.ParentDeviceName = parentDev.Name
			child.SizeHumanized = humanize.Bytes(child.Size.Uint64)
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

// parseLsblkPairs parses the "lsblk --pairs" output.
func parseLsblkPairs(b []byte, opts ...OpOption) (BlockDevices, error) {
	if len(b) == 0 {
		return nil, errors.New("empty input provided to ParsePairs")
	}

	devs := make(BlockDevices, 0)

	// parse each line
	lines := strings.Split(string(b), "\n")
	for _, line := range lines {
		// skip empty line
		if len(line) == 0 {
			continue
		}

		// parse each row then return BlockDevice
		disk, err := parseLineToDisk(line)
		if err != nil {
			return nil, err
		}

		// parse each block then add blocks slice
		devs = append(devs, disk)
	}

	// build disk hierarchy
	devs = buildDiskHierarchy(devs)
	if len(devs) == 0 {
		return nil, errors.New("build disk hierarchy failed")
	}

	// to JSON bytes
	jsonData, err := json.MarshalIndent(struct {
		BlockDevices BlockDevices `json:"blockdevices"`
	}{BlockDevices: devs}, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal lsblk-blockdevices json mode")
	}

	return parseLsblkJSON(jsonData, opts...)
}

func parseLineToDisk(line string) (BlockDevice, error) {
	disk := BlockDevice{}
	parts := strings.Fields(line)

	for _, part := range parts {
		kv := strings.Split(part, "=")
		if len(kv) != 2 {
			continue
		}
		key, value := kv[0], strings.Trim(kv[1], `"`)
		switch key {
		case "NAME":
			disk.Name = value
		case "TYPE":
			disk.Type = value
		case "SIZE":
			disk.Size = toCustomUint64(value)
		case "ROTA":
			disk.Rota = toCustomBool(value)
		case "SERIAL":
			disk.Serial = value
		case "WWN":
			disk.WWN = value
		case "VENDOR":
			disk.Vendor = value
		case "MODEL":
			disk.Model = value
		case "REV":
			disk.Rev = value
		case "MOUNTPOINT":
			disk.MountPoint = value
		case "FSTYPE":
			disk.FSType = value
		case "PARTUUID":
			disk.PartUUID = value
		case "PKNAME":
			disk.PKName = value
		}
	}

	return disk, nil
}

func buildDiskHierarchy(disks BlockDevices) (finalDisks BlockDevices) {
	// Recursive function to nest child disks into their parent disks
	var recursiveAdd func(disk BlockDevice, disks *BlockDevices)

	// Implementation of the recursive nesting function
	recursiveAdd = func(disk BlockDevice, disks *BlockDevices) {
		// Find the parent disk of the current disk and recursively nest
		for i := range *disks {
			if (*disks)[i].Name == disk.PKName {
				// Found the parent disk, add the current disk to the parent's Children
				(*disks)[i].Children = append((*disks)[i].Children, disk)
				return
			}

			// If the current disk has children, continue recursively
			recursiveAdd(disk, (*BlockDevices)(&(*disks)[i].Children))
		}
	}

	// Add disks that don't have a parent disk to finalDisks
	for i := range disks {
		if disks[i].PKName == "" {
			finalDisks = append(finalDisks, disks[i])
		}
	}

	// Perform recursive nesting for each disk
	for i := range disks {
		if disks[i].PKName != "" {
			recursiveAdd(disks[i], &finalDisks)
		}
	}

	return finalDisks
}

func toCustomUint64(value string) CustomUint64 {
	n, err := strconv.ParseUint(value, 10, 64)
	if err != nil {
		return CustomUint64{}
	}
	return CustomUint64{n}
}

func toCustomBool(value string) CustomBool {
	n, err := strconv.ParseBool(value)
	if err != nil {
		return CustomBool{}
	}
	return CustomBool{n}
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
		total += blk.Size.Uint64
	}
	return total
}

// BlockDevice is the struct that represents output of lsblk command for a device
type BlockDevice struct {
	Name             string        `json:"name,omitempty"`
	ParentDeviceName string        `json:"parent_device_name,omitempty"`
	Type             string        `json:"type,omitempty"`
	Size             CustomUint64  `json:"size,omitempty"`
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
	PKName           string        `json:"-"`
	Children         []BlockDevice `json:"children,omitempty"`
}

// CustomUint64 to handle Size lsblk output - 8001563222016 or "8001563222016"
type CustomUint64 struct {
	Uint64 uint64
}

// UnmarshalJSON customizes string size unmarshaling
// "63.9M" should be parsed to 63.9 million (63900000)
func (ci *CustomUint64) UnmarshalJSON(data []byte) error {
	var err error
	ci.Uint64, err = parseLsblkSize(data)
	return err
}

// "63.9M" should be parsed to 63.9 million (63900000)
func parseLsblkSize(data []byte) (uint64, error) {
	s := strings.TrimSpace(string(data))
	if len(s) == 0 || string(s) == "null" {
		return 0, nil
	}

	// remove quotes if present
	if len(s) > 1 && s[0] == '"' && s[len(s)-1] == '"' {
		s = strings.TrimSpace(string(s[1 : len(s)-1]))
		if s == "" {
			return 0, nil
		}
	}

	// try to parse as a human-readable size
	val, err := humanize.ParseBytes(s)

	if err != nil {
		// if failed, try to parse as a plain number
		if numVal, numErr := strconv.ParseUint(s, 10, 64); numErr == nil {
			return numVal, nil
		}

		if err := json.Unmarshal([]byte(s), &val); err != nil {
			return 0, fmt.Errorf("failed to unmarshal uint64: %w", err)
		}
	}

	return val, nil
}

// MarshalJSON customizes size marshaling
func (ci *CustomUint64) MarshalJSON() ([]byte, error) {
	return []byte(strconv.FormatUint(ci.Uint64, 10)), nil
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
