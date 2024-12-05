package disk

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"

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
	for _, p := range partitions {
		part := Partition{
			Device:     p.Device,
			MountPoint: p.Mountpoint,
			Fstype:     p.Fstype,
		}

		_, err := os.Stat(part.MountPoint)
		part.Mounted = err == nil

		if err != nil && os.IsNotExist(err) {
			return nil, err
		}

		if part.Mounted {
			usage, err := GetUsage(part.MountPoint)
			if err != nil {
				return nil, err
			}
			part.Usage = usage
		}

		ps = append(ps, part)
	}
	return ps, nil
}

func GetUsage(mountPoint string) (*Usage, error) {
	usage, err := disk.Usage(mountPoint)
	if err != nil {
		return nil, err
	}
	return &Usage{
		MountPoint:             usage.Path,
		Fstype:                 usage.Fstype,
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
		total += p.Usage.TotalBytes
	}
	return total
}

func (parts Partitions) RenderTable(wr io.Writer) {
	table := tablewriter.NewWriter(wr)
	table.SetHeader([]string{"Device", "Mount", "Mounted", "Fstype", "Total", "Free", "Used"})

	for _, part := range parts {
		table.Append([]string{
			part.Device,
			part.MountPoint,
			strconv.FormatBool(part.Mounted),
			part.Fstype,
			part.Usage.TotalHumanized,
			part.Usage.FreeHumanized,
			part.Usage.UsedHumanized,
		})
	}

	table.Render()
}

type Partition struct {
	Device     string `json:"device"`
	MountPoint string `json:"mount_point"`
	Mounted    bool   `json:"mounted"`
	Fstype     string `json:"fstype"`
	Usage      *Usage `json:"usage"`
}

type Usage struct {
	MountPoint string `json:"path"`
	Fstype     string `json:"fstype"`

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
