// Package disk provides utilities for disk operations.
package disk

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/olekukonko/tablewriter"
	"github.com/shirou/gopsutil/v4/disk"

	"github.com/leptonai/gpud/pkg/log"
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

		_, err := statWithTimeout(ctx, p.Mountpoint, op.statTimeout)
		part.Mounted = err == nil

		if err != nil {
			if os.IsNotExist(err) {
				// e.g., deleted pod then "stat /var/lib/kubelet/pods/80017f21-3c73-48" will fail
				log.Logger.Debugw("skipping partition because mount point does not exist", "error", err, "device", part.Device, "mountPoint", part.MountPoint)
				continue
			}

			if errors.Is(err, context.DeadlineExceeded) {
				// NFS or other network filesystem might be unresponsive
				log.Logger.Warnw("stat operation timed out, marking partition as not mounted", "error", err, "device", part.Device, "mountPoint", part.MountPoint)
				part.StatTimedOut = true
			}
		}

		if part.Mounted && !op.skipUsage {
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
		TotalBytes: usage.Total,
		FreeBytes:  usage.Free,
		UsedBytes:  usage.Used,
	}, nil
}

type Partitions []Partition

func (parts Partitions) RenderTable(wr io.Writer) {
	table := tablewriter.NewWriter(wr)
	table.SetHeader([]string{"Device", "Fstype", "Mount Point", "Mounted", "Total", "Used", "Free"})

	for _, part := range parts {
		total := "n/a"
		used := "n/a"
		free := "n/a"
		if part.Usage != nil {
			total = humanize.Bytes(part.Usage.TotalBytes)
			used = humanize.Bytes(part.Usage.UsedBytes)
			free = humanize.Bytes(part.Usage.FreeBytes)
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
	// StatTimedOut is true if the stat operation timed out.
	StatTimedOut bool `json:"stat_timed_out"`

	Usage *Usage `json:"usage"`
}

type Usage struct {
	TotalBytes uint64 `json:"total_bytes"`
	FreeBytes  uint64 `json:"free_bytes"`
	UsedBytes  uint64 `json:"used_bytes"`
}

// statWithTimeout performs os.Stat with a timeout to prevent blocking on unresponsive filesystems like NFS
func statWithTimeout(ctx context.Context, path string, timeout time.Duration) (os.FileInfo, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	type result struct {
		info os.FileInfo
		err  error
	}

	resultCh := make(chan result, 1)

	go func() {
		info, err := os.Stat(path)
		select {
		case resultCh <- result{info: info, err: err}:
		case <-ctx.Done():
			// Context canceled, result will be discarded
		}
	}()

	select {
	case res := <-resultCh:
		return res.info, res.err
	case <-ctx.Done():
		return nil, context.DeadlineExceeded
	}
}
