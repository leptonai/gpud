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
		blk1Flattened := FlattenedBlockDevice{
			Name:       blk1.Name,
			Type:       blk1.Type,
			Size:       blk1.Size.Uint64,
			Rota:       blk1.Rota.Bool,
			Serial:     blk1.Serial,
			WWN:        blk1.WWN,
			Vendor:     blk1.Vendor,
			Model:      blk1.Model,
			Rev:        blk1.Rev,
			MountPoint: blk1.MountPoint,
			FSType:     blk1.FSType,
			PartUUID:   blk1.PartUUID,
		}
		if blk1.ParentDeviceName != "" {
			blk1Flattened.Parents = []string{blk1.ParentDeviceName}
		}
		all[blk1Flattened.Name] = blk1Flattened

		// 2nd gen children
		for _, blk2 := range blk1.Children {
			parentDev := all[blk1Flattened.Name]
			parentDev.Children = append(parentDev.Children, blk2.Name)
			all[blk1Flattened.Name] = parentDev

			// now flatten the child as its own entry
			// but multiple children may share the same parent
			blk2Flattened := FlattenedBlockDevice{
				Name:       blk2.Name,
				Type:       blk2.Type,
				Size:       blk2.Size.Uint64,
				Rota:       blk2.Rota.Bool,
				Serial:     blk2.Serial,
				WWN:        blk2.WWN,
				Vendor:     blk2.Vendor,
				Model:      blk2.Model,
				Rev:        blk2.Rev,
				MountPoint: blk2.MountPoint,
				FSType:     blk2.FSType,
				PartUUID:   blk2.PartUUID,
				Parents:    []string{blk1Flattened.Name},
			}
			if prev, ok := all[blk2Flattened.Name]; ok {
				prev.Parents = append(prev.Parents, blk2Flattened.Parents...)
				all[blk2Flattened.Name] = prev
			} else {
				all[blk2Flattened.Name] = blk2Flattened
			}

			// track up to 3rd gen children
			for _, blk3 := range blk2.Children {
				parentDev := all[blk2Flattened.Name]
				parentDev.Children = append(parentDev.Children, blk3.Name)
				all[blk2Flattened.Name] = parentDev

				// now flatten the child as its own entry
				// but multiple children may share the same parent
				blk3Flattened := FlattenedBlockDevice{
					Name:       blk3.Name,
					Type:       blk3.Type,
					Size:       blk3.Size.Uint64,
					Rota:       blk3.Rota.Bool,
					Serial:     blk3.Serial,
					WWN:        blk3.WWN,
					Vendor:     blk3.Vendor,
					Model:      blk3.Model,
					Rev:        blk3.Rev,
					MountPoint: blk3.MountPoint,
					FSType:     blk3.FSType,
					PartUUID:   blk3.PartUUID,
					Parents:    []string{blk2Flattened.Name},
				}
				if prev, ok := all[blk3Flattened.Name]; ok {
					prev.Parents = append(prev.Parents, blk3Flattened.Parents...)
					all[blk3Flattened.Name] = prev
				} else {
					all[blk3Flattened.Name] = blk3Flattened
				}
			}
		}
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
