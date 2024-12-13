package disk

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/leptonai/gpud/components"
	disk_id "github.com/leptonai/gpud/components/disk/id"
	"github.com/leptonai/gpud/components/disk/metrics"
	components_metrics "github.com/leptonai/gpud/components/metrics"
	"github.com/leptonai/gpud/components/query"
	"github.com/leptonai/gpud/log"
	"github.com/leptonai/gpud/pkg/disk"
)

type MountTargetUsage struct {
	Device string     `json:"device"`
	Usage  disk.Usage `json:"usage"`
}

type Output struct {
	DiskExtPartitions disk.Partitions             `json:"disk_ext_partitions"`
	DiskBlockDevices  disk.BlockDevices           `json:"disk_block_devices"`
	MountTargetUsages map[string]MountTargetUsage `json:"mount_target_usages"`
}

const (
	StateNameDiskExtPartitions = "disk_ext_partitions"
	StateNameDiskBlockDevices  = "disk_block_devices"
	StateNameMountTargetUsages = "mount_target_usages"

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

	mb, err := json.Marshal(o.MountTargetUsages)
	if err != nil {
		return nil, err
	}

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
		{
			Name:    StateNameMountTargetUsages,
			Healthy: true,
			Reason:  "",
			ExtraInfo: map[string]string{
				StateKeyData:     string(mb),
				StateKeyEncoding: StateValueEncodingJSON,
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
		defaultPoller = query.New(disk_id.Name, cfg.Query, CreateGet(cfg))
	})
}

func getDefaultPoller() query.Poller {
	return defaultPoller
}

func CreateGet(cfg Config) query.GetFunc {
	mountPointsToTrackUsage := make(map[string]struct{})
	for _, mp := range cfg.MountPointsToTrackUsage {
		mountPointsToTrackUsage[mp] = struct{}{}
	}
	for _, mt := range cfg.MountTargetsToTrackUsage {
		mountPointsToTrackUsage[mt] = struct{}{}
	}

	return func(ctx context.Context) (_ any, e error) {
		defer func() {
			if e != nil {
				components_metrics.SetGetFailed(disk_id.Name)
			} else {
				components_metrics.SetGetSuccess(disk_id.Name)
			}
		}()

		o := &Output{}

		cctx, ccancel := context.WithTimeout(ctx, time.Minute)
		defer ccancel()

		parts, err := disk.GetPartitions(cctx, disk.WithFstype(func(fs string) bool {
			return fs == "ext4"
		}))
		if err != nil {
			return nil, fmt.Errorf("failed to get partitions: %w", err)
		}
		o.DiskExtPartitions = parts

		blks, err := disk.GetBlockDevices(cctx, disk.WithDeviceType(func(dt string) bool {
			return dt == "disk"
		}))
		if err != nil {
			return nil, fmt.Errorf("failed to get block devices: %w", err)
		}
		o.DiskBlockDevices = blks

		now := time.Now().UTC()
		nowUTC := float64(now.Unix())
		metrics.SetLastUpdateUnixSeconds(nowUTC)

		devToUsage := make(map[string]disk.Usage)
		for _, p := range o.DiskExtPartitions {
			usage := p.Usage
			if usage == nil {
				log.Logger.Warnw("no usage found for mount point", "mount_point", p.MountPoint)
				continue
			}

			devToUsage[p.Device] = *usage

			if _, ok := mountPointsToTrackUsage[p.MountPoint]; !ok {
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

		for _, target := range cfg.MountTargetsToTrackUsage {
			if _, err := os.Stat(target); err != nil {
				log.Logger.Warnw("mount target does not exist", "mount_target", target)
				continue
			}
			device, err := disk.FindMntTargetDevice(target)
			if err != nil {
				log.Logger.Warnw("error finding mount target device", "mount_target", target, "error", err)
				continue
			}

			usage, ok := devToUsage[device]
			if !ok {
				log.Logger.Warnw("no usage found for mount target", "mount_target", target)
				continue
			}
			if o.MountTargetUsages == nil {
				o.MountTargetUsages = make(map[string]MountTargetUsage)
			}
			o.MountTargetUsages[target] = MountTargetUsage{Device: device, Usage: usage}
		}

		return o, nil
	}
}
