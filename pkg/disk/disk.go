package disk

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/dustin/go-humanize"
	"github.com/olekukonko/tablewriter"
	"github.com/shirou/gopsutil/v4/disk"
	"sigs.k8s.io/yaml"
)

func GetPartitions() (Partitions, error) {
	partitions, err := disk.Partitions(true)
	if err != nil {
		return nil, err
	}

	ps := make([]Partition, 0, len(partitions))
	deviceToPartitions := make(map[string]Partitions)
	for _, p := range partitions {
		part := Partition{
			Device:      p.Device,
			FSTypes:     []string{p.Fstype},
			MountPoints: []string{p.Mountpoint},
		}

		_, err := os.Stat(part.MountPoints[0])
		part.Mounted = err == nil

		if err != nil && os.IsNotExist(err) {
			return nil, err
		}

		if part.Mounted {
			part.Usage, err = GetUsage(part.MountPoints[0])
			if err != nil {
				return nil, err
			}
		}

		ps = append(ps, part)

		if _, ok := deviceToPartitions[part.Device]; !ok {
			deviceToPartitions[part.Device] = make([]Partition, 0)
		}
		deviceToPartitions[part.Device] = append(deviceToPartitions[part.Device], part)
	}

	for dev, parts := range deviceToPartitions {
		if len(parts) < 2 { // no need to aggregate
			continue
		}

		fsTypesSet := make(map[string]struct{})
		mountPoints := make([]string, 0, len(parts))
		mounted := true
		for _, p := range parts {
			for _, fsType := range p.FSTypes {
				fsTypesSet[fsType] = struct{}{}
			}

			mountPoints = append(mountPoints, p.MountPoints...)
			if !p.Mounted {
				mounted = false
			}
		}
		sort.Strings(mountPoints)

		fsTypes := make([]string, 0, len(fsTypesSet))
		for fsType := range fsTypesSet {
			fsTypes = append(fsTypes, fsType)
		}
		sort.Strings(fsTypes)

		// multiple mount points for the same device
		aggPart := Partition{
			Device:  dev,
			FSTypes: fsTypes,

			MountPoints: mountPoints,
			Mounted:     mounted,

			Usage: parts.AggregateUsage(),
		}
		ps = append(ps, aggPart)
	}

	// sort in descending order of total bytes
	sort.Slice(ps, func(i, j int) bool {
		if ps[j].Usage == nil {
			return true
		}
		if ps[i].Usage == nil {
			return false
		}
		return ps[i].Usage.TotalBytes > ps[j].Usage.TotalBytes
	})

	return ps, nil
}

func GetUsage(mountPoint string) (*Usage, error) {
	usage, err := disk.Usage(mountPoint)
	if err != nil {
		return nil, err
	}
	return &Usage{
		TotalBytes:             usage.Total,
		TotalHumanized:         humanize.Bytes(usage.Total),
		FreeBytes:              usage.Free,
		FreeHumanized:          humanize.Bytes(usage.Free),
		UsedBytes:              usage.Used,
		UsedHumanized:          humanize.Bytes(usage.Used),
		UsedPercent:            fmt.Sprintf("%.2f", usage.UsedPercent),
		UsedPercentFloat:       usage.UsedPercent,
		InodesTotal:            usage.InodesTotal,
		InodesUsed:             usage.InodesUsed,
		InodesFree:             usage.InodesFree,
		InodesUsedPercent:      fmt.Sprintf("%.2f", usage.InodesUsedPercent),
		InodesUsedPercentFloat: usage.InodesUsedPercent,
	}, nil
}

type Partitions []Partition

func (parts Partitions) JSON() ([]byte, error) {
	return json.Marshal(parts)
}

func (parts Partitions) YAML() ([]byte, error) {
	return yaml.Marshal(parts)
}

func (parts Partitions) TotalBytes() uint64 {
	var total uint64
	for _, p := range parts {
		if p.Usage == nil {
			continue
		}

		// skip unmounted partitions
		if !p.Mounted {
			continue
		}

		// skip aggregated partitions
		if len(p.MountPoints) > 1 {
			continue
		}

		total += p.Usage.TotalBytes
	}
	return total
}

func (parts Partitions) AggregateUsage() *Usage {
	var agg Usage
	for _, p := range parts {
		agg = agg.Add(*p.Usage)
	}
	return &agg
}

func (parts Partitions) RenderTable(wr io.Writer) {
	table := tablewriter.NewWriter(wr)
	table.SetHeader([]string{"Device", "FS Types", "Mount Points", "Mounted", "Total", "Free", "Used"})

	for _, part := range parts {
		table.Append([]string{
			part.Device,
			strings.Join(part.FSTypes, "\n"),
			strings.Join(part.MountPoints, "\n"),
			strconv.FormatBool(part.Mounted),
			part.Usage.TotalHumanized,
			part.Usage.FreeHumanized,
			part.Usage.UsedHumanized,
		})
	}

	table.Render()
}

type Partition struct {
	Device string `json:"device"`

	// FSTypes is a list of filesystem types for the device.
	// Only multiple mount points for aggregated partitions.
	// For example, ["xfs", "ext4"] for the device "/dev/mapper/vgroot-lvroot".
	FSTypes []string `json:"fs_types"`

	// MountPoints is a list of mount points for the device.
	// Only multiple mount points for aggregated partitions.
	// For example, "/var/lib/kubelet/pods/*" will be aggregated for the device "/dev/mapper/vgroot-lvroot".
	MountPoints []string `json:"mount_points"`
	Mounted     bool     `json:"mounted"`

	Usage *Usage `json:"usage"`
}

type Usage struct {
	TotalBytes     uint64 `json:"total_bytes"`
	TotalHumanized string `json:"total_humanized"`

	FreeBytes     uint64 `json:"free_bytes"`
	FreeHumanized string `json:"free_humanized"`

	UsedBytes     uint64 `json:"used_bytes"`
	UsedHumanized string `json:"used_humanized"`

	UsedPercent      string  `json:"used_percent"`
	UsedPercentFloat float64 `json:"-"`

	InodesTotal       uint64 `json:"inodes_total"`
	InodesUsed        uint64 `json:"inodes_used"`
	InodesFree        uint64 `json:"inodes_free"`
	InodesUsedPercent string `json:"inodes_used_percent"`

	InodesUsedPercentFloat float64 `json:"-"`
}

func (u Usage) GetUsedPercent() (float64, error) {
	return strconv.ParseFloat(u.UsedPercent, 64)
}

func (a Usage) Add(b Usage) Usage {
	totalBytes := a.TotalBytes + b.TotalBytes
	freeBytes := a.FreeBytes + b.FreeBytes
	usedBytes := a.UsedBytes + b.UsedBytes

	// Calculate used percentage
	usedPercent := 0.0
	if totalBytes > 0 {
		usedPercent = float64(usedBytes) / float64(totalBytes) * 100
	}

	// Calculate inodes
	inodesTotal := a.InodesTotal + b.InodesTotal
	inodesUsed := a.InodesUsed + b.InodesUsed
	inodesFree := a.InodesFree + b.InodesFree

	// Calculate inodes percentage
	inodesUsedPercent := 0.0
	if inodesTotal > 0 {
		inodesUsedPercent = float64(inodesUsed) / float64(inodesTotal) * 100
	}

	return Usage{
		TotalBytes:             totalBytes,
		TotalHumanized:         humanize.Bytes(totalBytes),
		FreeBytes:              freeBytes,
		FreeHumanized:          humanize.Bytes(freeBytes),
		UsedBytes:              usedBytes,
		UsedHumanized:          humanize.Bytes(usedBytes),
		UsedPercent:            fmt.Sprintf("%.2f", usedPercent),
		UsedPercentFloat:       usedPercent,
		InodesTotal:            inodesTotal,
		InodesUsed:             inodesUsed,
		InodesFree:             inodesFree,
		InodesUsedPercent:      fmt.Sprintf("%.2f", inodesUsedPercent),
		InodesUsedPercentFloat: inodesUsedPercent,
	}
}
