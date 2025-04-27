package disk

import (
	"io"

	"github.com/dustin/go-humanize"
	"github.com/olekukonko/tablewriter"
)

type FlattenBlockDevices []FlattenBlockDevice

// FlattenBlockDevice represents the flattened output of lsblk command for a device
type FlattenBlockDevice struct {
	Name             string   `json:"name,omitempty"`
	Type             string   `json:"type,omitempty"`
	Size             uint64   `json:"size,omitempty"`
	Rota             bool     `json:"rota,omitempty"`
	Serial           string   `json:"serial,omitempty"`
	WWN              string   `json:"wwn,omitempty"`
	Vendor           string   `json:"vendor,omitempty"`
	Model            string   `json:"model,omitempty"`
	Rev              string   `json:"rev,omitempty"`
	MountPoint       string   `json:"mountpoint,omitempty"`
	FSType           string   `json:"fstype,omitempty"`
	PartUUID         string   `json:"partuuid,omitempty"`
	ParentDeviceName string   `json:"parentDeviceName,omitempty"`
	Children         []string `json:"children,omitempty"`
}

func (blk BlockDevice) flatten() FlattenBlockDevices {
	blkFlat := FlattenBlockDevice{
		Name:             blk.Name,
		Type:             blk.Type,
		Size:             blk.Size.Uint64,
		Rota:             blk.Rota.Bool,
		Serial:           blk.Serial,
		WWN:              blk.WWN,
		Vendor:           blk.Vendor,
		Model:            blk.Model,
		Rev:              blk.Rev,
		MountPoint:       blk.MountPoint,
		FSType:           blk.FSType,
		PartUUID:         blk.PartUUID,
		ParentDeviceName: blk.ParentDeviceName,
	}
	flattened := FlattenBlockDevices{blkFlat}

	for _, child := range blk.Children {
		flattened = append(flattened, FlattenBlockDevice{
			Name:             child.Name,
			Type:             child.Type,
			Size:             child.Size.Uint64,
			Rota:             child.Rota.Bool,
			Serial:           child.Serial,
			WWN:              child.WWN,
			Vendor:           child.Vendor,
			Model:            child.Model,
			Rev:              child.Rev,
			MountPoint:       child.MountPoint,
			FSType:           child.FSType,
			PartUUID:         child.PartUUID,
			ParentDeviceName: child.ParentDeviceName,
		})

		flattened[0].Children = append(flattened[0].Children, child.Name)
	}

	return flattened
}

func (blks BlockDevices) Flatten() FlattenBlockDevices {
	flattened := FlattenBlockDevices{}
	for _, blk := range blks {
		flattened = append(flattened, blk.flatten()...)
	}
	return flattened
}

func (blks FlattenBlockDevices) RenderTable(wr io.Writer) {
	table := tablewriter.NewWriter(wr)
	table.SetHeader([]string{"Name", "Parent", "Type", "FSType", "Size", "Mount Point"})

	for _, blk := range blks {
		table.Append([]string{
			blk.Name,
			blk.ParentDeviceName,
			blk.Type,
			blk.FSType,
			humanize.Bytes(uint64(blk.Size)),
			blk.MountPoint,
		})
	}

	table.Render()
}
