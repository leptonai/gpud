// Package disk provides utilities for disk operations.
package disk

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/leptonai/gpud/pkg/log"

	"github.com/dustin/go-humanize"
	"github.com/olekukonko/tablewriter"
	"github.com/shirou/gopsutil/v4/disk"
	"sigs.k8s.io/yaml"
)

func GetPartitions(ctx context.Context, opts ...OpOption) (Partitions, error) {
	op := &Op{}
	if err := op.applyOpts(opts); err != nil {
		return nil, err
	}

	partitions, err := disk.PartitionsWithContext(ctx, true)
	if err != nil {
		return nil, err
	}

	ps := make([]Partition, 0, len(partitions))
	deviceToPartitions := make(map[string]Partitions)
	for _, p := range partitions {
		if !op.matchFuncFstype(p.Fstype) {
			log.Logger.Debugw("skipping partition", "fstype", p.Fstype, "device", p.Device, "mountPoint", p.Mountpoint)
			continue
		}

		part := Partition{
			Device:     p.Device,
			Fstype:     p.Fstype,
			MountPoint: p.Mountpoint,
		}

		_, err := os.Stat(p.Mountpoint)
		part.Mounted = err == nil

		if err != nil && os.IsNotExist(err) {
			// e.g., deleted pod then "stat /var/lib/kubelet/pods/80017f21-3c73-48" will fail
			log.Logger.Debugw("skipping partition because mount point does not exist", "error", err, "device", part.Device, "mountPoint", part.MountPoint)
			continue
		}

		if part.Mounted {
			part.Usage, err = GetUsage(ctx, p.Mountpoint)
			if err != nil {
				// mount point is gone
				// e.g., "no such file or directory"
				if strings.Contains(err.Error(), "no such file or directory") {
					log.Logger.Debugw("skipping partition because mount point does not exist", "error", err, "device", part.Device, "mountPoint", part.MountPoint)
					continue
				}

				return nil, fmt.Errorf("failed to get usage for mounted partition %q: %w", p.Mountpoint, err)
			}
		}

		ps = append(ps, part)

		if _, ok := deviceToPartitions[part.Device]; !ok {
			deviceToPartitions[part.Device] = make([]Partition, 0)
		}
		deviceToPartitions[part.Device] = append(deviceToPartitions[part.Device], part)
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

func GetUsage(ctx context.Context, mountPoint string) (*Usage, error) {
	usage, err := disk.UsageWithContext(ctx, mountPoint)
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

func (parts Partitions) RenderTable(wr io.Writer) {
	table := tablewriter.NewWriter(wr)
	table.SetHeader([]string{"Device", "Fstype", "Mount Point", "Mounted", "Total", "Used", "Free"})

	for _, part := range parts {
		total := "n/a"
		used := "n/a"
		free := "n/a"
		if part.Usage != nil {
			total = part.Usage.TotalHumanized
			used = part.Usage.UsedHumanized
			free = part.Usage.FreeHumanized
		}

		table.Append([]string{
			part.Device,
			part.Fstype,
			part.MountPoint,
			strconv.FormatBool(part.Mounted),
			total,
			used,
			free,
		})
	}

	table.Render()
}

// Returns the total bytes of all mounted partitions.
func (parts Partitions) GetMountedTotalBytes() uint64 {
	var total uint64
	for _, p := range parts {
		if p.Usage == nil {
			continue
		}

		// skip unmounted partitions
		if !p.Mounted {
			continue
		}

		total += p.Usage.TotalBytes
	}
	return total
}

type Partition struct {
	Device string `json:"device"`

	Fstype     string `json:"fstype"`
	MountPoint string `json:"mount_point"`
	Mounted    bool   `json:"mounted"`

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
