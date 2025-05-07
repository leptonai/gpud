package disk

import (
	"io"
	"sort"
	"strings"

	"github.com/dustin/go-humanize"
	"github.com/olekukonko/tablewriter"
)

type FlattenedBlockDevices []FlattenedBlockDevice

// FlattenedBlockDevice represents the flattened output of lsblk command for a device,
// meaning the child device is included as its own entry in the device list.
type FlattenedBlockDevice struct {
	Name       string `json:"name,omitempty"`
	Type       string `json:"type,omitempty"`
	Size       uint64 `json:"size,omitempty"`
	Rota       bool   `json:"rota,omitempty"`
	Serial     string `json:"serial,omitempty"`
	WWN        string `json:"wwn,omitempty"`
	Vendor     string `json:"vendor,omitempty"`
	Model      string `json:"model,omitempty"`
	Rev        string `json:"rev,omitempty"`
	MountPoint string `json:"mountpoint,omitempty"`
	FSType     string `json:"fstype,omitempty"`
	PartUUID   string `json:"partuuid,omitempty"`

	Parents  []string `json:"parents,omitempty"`
	Children []string `json:"children,omitempty"`
}

func (blks BlockDevices) Flatten() FlattenedBlockDevices {
	// to dedup by volume name
	all := make(map[string]FlattenedBlockDevice)

	for _, blk1 := range blks {
		flattenBlockDevice(blk1, "", 0, all)
	}

	flattened := make(FlattenedBlockDevices, 0, len(all))
	for _, blk := range all {
		flattened = append(flattened, blk)
	}
	sort.Slice(flattened, func(i, j int) bool {
		return flattened[i].Name < flattened[j].Name
	})
	return flattened
}

// flattenBlockDevice recursively processes a block device and its children
// up to maxDepth (2 levels deep), adding them to the all map.
func flattenBlockDevice(blk BlockDevice, parentName string, depth int, all map[string]FlattenedBlockDevice) {
	// Flatten the current device
	flattened := FlattenedBlockDevice{
		Name:       blk.Name,
		Type:       blk.Type,
		Size:       blk.Size.Uint64,
		Rota:       blk.Rota.Bool,
		Serial:     blk.Serial,
		WWN:        blk.WWN,
		Vendor:     blk.Vendor,
		Model:      blk.Model,
		Rev:        blk.Rev,
		MountPoint: blk.MountPoint,
		FSType:     blk.FSType,
		PartUUID:   blk.PartUUID,
	}

	// Add parent from function parameter
	if parentName != "" {
		flattened.Parents = []string{parentName}
	} else if blk.ParentDeviceName != "" {
		// For top-level devices that might have a parent
		flattened.Parents = []string{blk.ParentDeviceName}
	}

	// Update or add the device to the map
	if prev, ok := all[flattened.Name]; ok {
		// If device already exists, merge parents
		if len(flattened.Parents) > 0 {
			prev.Parents = append(prev.Parents, flattened.Parents...)
		}
		all[flattened.Name] = prev
	} else {
		all[flattened.Name] = flattened
	}

	// Process children if we haven't reached max depth (2)
	if depth < 2 {
		for _, child := range blk.Children {
			// Update parent's children list
			parentDev := all[flattened.Name]
			parentDev.Children = append(parentDev.Children, child.Name)
			all[flattened.Name] = parentDev

			// Recursively process the child
			flattenBlockDevice(child, flattened.Name, depth+1, all)
		}
	}
}

func (blks FlattenedBlockDevices) RenderTable(wr io.Writer) {
	table := tablewriter.NewWriter(wr)
	table.SetHeader([]string{"Name", "Type", "FSType", "Size", "Mount Point", "Parents", "Children"})

	for _, blk := range blks {
		table.Append([]string{
			blk.Name,
			blk.Type,
			blk.FSType,
			humanize.Bytes(blk.Size),
			blk.MountPoint,
			strings.Join(blk.Parents, "\n"),
			strings.Join(blk.Children, "\n"),
		})
	}

	table.Render()
}

func (blks FlattenedBlockDevices) GetDeviceUsages(parts Partitions) DeviceUsages {
	usages := make(map[string]Usage)
	for _, part := range parts {
		if part.Usage == nil {
			continue
		}
		usages[part.MountPoint] = *part.Usage
	}

	devUsages := make(DeviceUsages, 0, len(blks))

	for _, blkDev := range blks {
		if blkDev.MountPoint == "" {
			continue
		}

		usage, ok := usages[blkDev.MountPoint]
		if !ok {
			continue
		}

		devUsages = append(devUsages, DeviceUsage{
			DeviceName: blkDev.Name,
			MountPoint: blkDev.MountPoint,

			TotalBytes: usage.TotalBytes,
			FreeBytes:  usage.FreeBytes,
			UsedBytes:  usage.UsedBytes,
		})
	}

	return devUsages
}
