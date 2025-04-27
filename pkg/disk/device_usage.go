package disk

import (
	"io"
	"strings"

	"github.com/dustin/go-humanize"
	"github.com/olekukonko/tablewriter"
)

type DeviceUsages []DeviceUsage

// DeviceUsage is derived from the output of "lsblk" command,
// and the size and usage information based on its mount point
type DeviceUsage struct {
	FlattenedBlockDevice

	TotalBytes uint64 `json:"total_bytes"`
	FreeBytes  uint64 `json:"free_bytes"`
	UsedBytes  uint64 `json:"used_bytes"`
}

func (devs *DeviceUsages) RenderTable(wr io.Writer) {
	table := tablewriter.NewWriter(wr)
	table.SetHeader([]string{"Device", "Mount Point", "Device Type", "FSType", "Total", "Used", "Free", "Parents", "Children"})
	for _, dev := range devs {
		table.Append([]string{
			dev.FlattenedBlockDevice.Name,
			dev.MountPoint,
			dev.FlattenedBlockDevice.Type,
			dev.FlattenedBlockDevice.FSType,
			humanize.Bytes(dev.TotalBytes),
			humanize.Bytes(dev.UsedBytes),
			humanize.Bytes(dev.FreeBytes),
			strings.Join(dev.Parents, "\n"),
			strings.Join(dev.Children, "\n"),
		})
	}
	table.Render()
}
