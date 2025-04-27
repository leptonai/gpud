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
		blkParent := FlattenedBlockDevice{
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
			blkParent.Parents = []string{blk1.ParentDeviceName}
		}
		all[blkParent.Name] = blkParent

		for _, blk2 := range blk1.Children {
			parent := all[blkParent.Name]
			parent.Children = append(parent.Children, blk2.Name)
			all[blkParent.Name] = parent

			// now flatten the child as its own entry
			// but multiple children may share the same parent
			blkChild := FlattenedBlockDevice{
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
				Parents:    []string{blkParent.Name},
			}
			if prev, ok := all[blkChild.Name]; ok {
				prev.Parents = append(prev.Parents, blkChild.Parents...)
				all[blkChild.Name] = prev
			} else {
				all[blkChild.Name] = blkChild
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
