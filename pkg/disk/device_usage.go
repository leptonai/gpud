package disk

import (
	"io"

	"github.com/dustin/go-humanize"
	"github.com/olekukonko/tablewriter"
)

type DeviceUsages []DeviceUsage

// DeviceUsage is derived from the output of "lsblk" command,
// and the size and usage information based on its mount point
type DeviceUsage struct {
	DeviceName string `json:"device_name,omitempty"`
	MountPoint string `json:"mountpoint,omitempty"`

	TotalBytes uint64 `json:"total_bytes"`
	FreeBytes  uint64 `json:"free_bytes"`
	UsedBytes  uint64 `json:"used_bytes"`
}

func (devs DeviceUsages) RenderTable(wr io.Writer) {
	if len(devs) == 0 {
		return
	}

	table := tablewriter.NewWriter(wr)
	table.SetHeader([]string{"Device", "Mount Point", "Total", "Used", "Free"})
	for _, dev := range devs {
		table.Append([]string{
			dev.DeviceName,
			dev.MountPoint,
			humanize.IBytes(dev.TotalBytes),
			humanize.IBytes(dev.UsedBytes),
			humanize.IBytes(dev.FreeBytes),
		})
	}
	table.Render()
}
