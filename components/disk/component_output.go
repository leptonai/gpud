package disk

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/components/disk/metrics"
	components_metrics "github.com/leptonai/gpud/components/metrics"
	"github.com/leptonai/gpud/components/query"
	"github.com/leptonai/gpud/log"
	"github.com/leptonai/gpud/pkg/disk"
)

type Output struct {
	DiskExtPartitions disk.Partitions   `json:"disk_ext_partitions"`
	DiskBlockDevices  disk.BlockDevices `json:"disk_block_devices"`
}

const (
	StateNameDiskExtPartitions = "disk_ext_partitions"
	StateNameDiskBlockDevices  = "disk_block_devices"

	StateKeyData           = "data"
	StateKeyEncoding       = "encoding"
	StateValueEncodingJSON = "json"

	StateNameDiskExtPartitionsTotal         = "disk_ext_partitions_total"
	StateKeyDiskExtPartitionsTotalBytes     = "disk_ext_partitions_total_bytes"
	StateKeyDiskExtPartitionsTotalGB        = "disk_ext_partitions_total_gb"
	StateKeyDiskExtPartitionsTotalHumanized = "disk_ext_partitions_total_humanized"

	StateNameDiskBlockDevicesTotal         = "disk_block_devices_total"
	StateKeyDiskBlockDevicesTotalBytes     = "disk_block_devices_total_bytes"
	StateKeyDiskBlockDevicesTotalGB        = "disk_block_devices_total_gb"
	StateKeyDiskBlockDevicesTotalHumanized = "disk_block_devices_total_humanized"
)

func (o *Output) States() ([]components.State, error) {
	diskExtPartitionsData, err := o.DiskExtPartitions.JSON()
	if err != nil {
		return nil, err
	}
	diskBlockDevicesData, err := o.DiskBlockDevices.JSON()
	if err != nil {
		return nil, err
	}

	totalMountedBytes := o.DiskExtPartitions.GetMountedTotalBytes()
	totalMountedGB := float64(totalMountedBytes) / 1e9
	totalMountedBytesHumanized := humanize.Bytes(totalMountedBytes)

	blkDevTotalBytes := o.DiskBlockDevices.GetTotalBytes()
	blkDevTotalGB := float64(blkDevTotalBytes) / 1e9
	blkDevTotalBytesHumanized := humanize.Bytes(blkDevTotalBytes)

	return []components.State{
		{
			Name:    StateNameDiskExtPartitions,
			Healthy: true,
			Reason:  "",
			ExtraInfo: map[string]string{
				StateKeyData:     string(diskExtPartitionsData),
				StateKeyEncoding: StateValueEncodingJSON,
			},
		},
		{
			Name:    StateNameDiskBlockDevices,
			Healthy: true,
			Reason:  "",
			ExtraInfo: map[string]string{
				StateKeyData:     string(diskBlockDevicesData),
				StateKeyEncoding: StateValueEncodingJSON,
			},
		},
		{
			Name:    StateNameDiskExtPartitionsTotal,
			Healthy: true,
			Reason:  "",
			ExtraInfo: map[string]string{
				StateKeyDiskExtPartitionsTotalBytes:     fmt.Sprintf("%d", totalMountedBytes),
				StateKeyDiskExtPartitionsTotalGB:        fmt.Sprintf("%.2f", totalMountedGB),
				StateKeyDiskExtPartitionsTotalHumanized: totalMountedBytesHumanized,
			},
		},
		{
			Name:    StateNameDiskBlockDevicesTotal,
			Healthy: true,
			Reason:  "",
			ExtraInfo: map[string]string{
				StateKeyDiskBlockDevicesTotalBytes:     fmt.Sprintf("%d", blkDevTotalBytes),
				StateKeyDiskBlockDevicesTotalGB:        fmt.Sprintf("%.2f", blkDevTotalGB),
				StateKeyDiskBlockDevicesTotalHumanized: blkDevTotalBytesHumanized,
			},
		},
	}, nil
}

var (
	defaultPollerOnce sync.Once
	defaultPoller     query.Poller
)

// only set once since it relies on the kube client and specific port
func setDefaultPoller(cfg Config) {
	defaultPollerOnce.Do(func() {
		defaultPoller = query.New(Name, cfg.Query, CreateGet(cfg))
	})
}

func getDefaultPoller() query.Poller {
	return defaultPoller
}

func CreateGet(cfg Config) query.GetFunc {
	return func(ctx context.Context) (_ any, e error) {
		defer func() {
			if e != nil {
				components_metrics.SetGetFailed(Name)
			} else {
				components_metrics.SetGetSuccess(Name)
			}
		}()

		o := &Output{}

		cctx, ccancel := context.WithTimeout(ctx, 30*time.Second)
		defer ccancel()
		parts, err := disk.GetPartitions(cctx)
		if err != nil {
			return nil, err
		}
		for _, p := range parts {
			if p.Fstype == "ext4" {
				o.DiskExtPartitions = append(o.DiskExtPartitions, p)
			}
		}

		now := time.Now().UTC()
		nowUTC := float64(now.Unix())
		metrics.SetLastUpdateUnixSeconds(nowUTC)

		// for _, path := range cfg.MountPointsToTrackUsage {
		for _, p := range o.DiskExtPartitions {
			usage := p.Usage
			if usage == nil {
				log.Logger.Warnw("no usage found for mount point", "mount_point", p.MountPoint)
				continue
			}

			if err := metrics.SetTotalBytes(ctx, p.MountPoint, float64(usage.TotalBytes), now); err != nil {
				return nil, err
			}
			metrics.SetFreeBytes(p.MountPoint, float64(usage.FreeBytes))
			if err := metrics.SetUsedBytes(ctx, p.MountPoint, float64(usage.UsedBytes), now); err != nil {
				return nil, err
			}
			if err := metrics.SetUsedBytesPercent(ctx, p.MountPoint, usage.UsedPercentFloat, now); err != nil {
				return nil, err
			}
			metrics.SetUsedInodesPercent(p.MountPoint, usage.InodesUsedPercentFloat)
		}

		return o, nil
	}
}
