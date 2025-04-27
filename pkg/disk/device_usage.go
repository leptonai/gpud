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

	TotalBytes       uint64  `json:"total_bytes"`
	FreeBytes        uint64  `json:"free_bytes"`
	UsedBytes        uint64  `json:"used_bytes"`
	UsedPercent      string  `json:"used_percent"`
	UsedPercentFloat float64 `json:"-"`
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
			FlattenedBlockDevice: blkDev,

			TotalBytes:       usage.TotalBytes,
			FreeBytes:        usage.FreeBytes,
			UsedBytes:        usage.UsedBytes,
			UsedPercent:      usage.UsedPercent,
			UsedPercentFloat: usage.UsedPercentFloat,
		})
	}

	return devUsages
}

func (devs DeviceUsages) RenderTable(wr io.Writer) {
	table := tablewriter.NewWriter(wr)
	table.SetHeader([]string{"Device", "Mount Point", "Device Type", "FSType", "Total", "Used", "Free", "Used %", "Parents", "Children"})
	for _, dev := range devs {
		table.Append([]string{
			dev.FlattenedBlockDevice.Name,
			dev.MountPoint,
			dev.FlattenedBlockDevice.Type,
			dev.FlattenedBlockDevice.FSType,
			humanize.Bytes(dev.TotalBytes),
			humanize.Bytes(dev.UsedBytes),
			humanize.Bytes(dev.FreeBytes),
			dev.UsedPercent,
			strings.Join(dev.Parents, "\n"),
			strings.Join(dev.Children, "\n"),
		})
	}
	table.Render()
}
