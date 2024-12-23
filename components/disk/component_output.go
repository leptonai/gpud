package disk

import (
	"context"
	"encoding/json"
	"errors"
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

type Output struct {
	DiskExtPartitions disk.Partitions               `json:"disk_ext_partitions"`
	DiskBlockDevices  disk.BlockDevices             `json:"disk_block_devices"`
	MountTargetUsages map[string]disk.FindMntOutput `json:"mount_target_usages"`
}

const (
	StateNameDiskExtPartition  = "disk_ext_partition"
	StateNameDiskBlockDevices  = "disk_block_devices"
	StateNameMountTargetUsages = "mount_target_usages"

	StateKeyData           = "data"
	StateKeyEncoding       = "encoding"
	StateValueEncodingJSON = "json"

	StateNameDiskBlockDevicesTotal         = "disk_block_devices_total"
	StateKeyDiskBlockDevicesTotalBytes     = "disk_block_devices_total_bytes"
	StateKeyDiskBlockDevicesTotalGB        = "disk_block_devices_total_gb"
	StateKeyDiskBlockDevicesTotalHumanized = "disk_block_devices_total_humanized"

	StateNameDiskExtPartitionsTotal         = "disk_ext_partitions_total"
	StateKeyDiskExtPartitionsTotalBytes     = "disk_ext_partitions_total_bytes"
	StateKeyDiskExtPartitionsTotalGB        = "disk_ext_partitions_total_gb"
	StateKeyDiskExtPartitionsTotalHumanized = "disk_ext_partitions_total_humanized"
)

func (o *Output) States() ([]components.State, error) {
	querySucceededState := components.State{
		Name:    disk_id.Name,
		Healthy: true,
		Reason:  "query succeeded",
	}

	var (
		diskBlockDevicesData      []byte
		blkDevTotalBytes          uint64
		blkDevTotalGB             float64
		blkDevTotalBytesHumanized string
	)
	if len(o.DiskBlockDevices) > 0 {
		var err error
		diskBlockDevicesData, err = o.DiskBlockDevices.JSON()
		if err != nil {
			return nil, err
		}
		blkDevTotalBytes = o.DiskBlockDevices.GetTotalBytes()
		blkDevTotalGB = float64(blkDevTotalBytes) / 1e9
		blkDevTotalBytesHumanized = humanize.Bytes(blkDevTotalBytes)
	}

	var (
		diskExtPartitionsData      []byte
		totalMountedBytes          uint64
		totalMountedGB             float64
		totalMountedBytesHumanized string
	)
	if len(o.DiskExtPartitions) > 0 {
		var err error
		diskExtPartitionsData, err = o.DiskExtPartitions.JSON()
		if err != nil {
			return nil, err
		}

		totalMountedBytes = o.DiskExtPartitions.GetMountedTotalBytes()
		totalMountedGB = float64(totalMountedBytes) / 1e9
		totalMountedBytesHumanized = humanize.Bytes(totalMountedBytes)
	}

	var mountTargetUsagesBytes []byte
	if len(o.MountTargetUsages) > 0 {
		var err error
		mountTargetUsagesBytes, err = json.Marshal(o.MountTargetUsages)
		if err != nil {
			return nil, err
		}
	}

	return []components.State{
		querySucceededState,
		{
			Name:    StateNameDiskBlockDevices,
			Healthy: true,
			Reason:  fmt.Sprintf("found %d block devices", len(o.DiskBlockDevices)),
			ExtraInfo: map[string]string{
				StateKeyData:     string(diskBlockDevicesData),
				StateKeyEncoding: StateValueEncodingJSON,
			},
		},
		{
			Name:    StateNameDiskBlockDevicesTotal,
			Healthy: true,
			Reason:  fmt.Sprintf("found %d block devices", len(o.DiskBlockDevices)),
			ExtraInfo: map[string]string{
				StateKeyDiskBlockDevicesTotalBytes:     fmt.Sprintf("%d", blkDevTotalBytes),
				StateKeyDiskBlockDevicesTotalGB:        fmt.Sprintf("%.2f", blkDevTotalGB),
				StateKeyDiskBlockDevicesTotalHumanized: blkDevTotalBytesHumanized,
			},
		},
		{
			Name:    StateNameDiskExtPartition,
			Healthy: len(o.DiskExtPartitions) > 0,
			Reason:  "",
			ExtraInfo: map[string]string{
				StateKeyData:     string(diskExtPartitionsData),
				StateKeyEncoding: StateValueEncodingJSON,
			},
		},
		{
			Name:    StateNameDiskExtPartitionsTotal,
			Healthy: len(o.DiskExtPartitions) > 0,
			Reason:  "",
			ExtraInfo: map[string]string{
				StateKeyDiskExtPartitionsTotalBytes:     fmt.Sprintf("%d", totalMountedBytes),
				StateKeyDiskExtPartitionsTotalGB:        fmt.Sprintf("%.2f", totalMountedGB),
				StateKeyDiskExtPartitionsTotalHumanized: totalMountedBytesHumanized,
			},
		},
		{
			Name:    StateNameMountTargetUsages,
			Healthy: true,
			Reason:  "",
			ExtraInfo: map[string]string{
				StateKeyData:     string(mountTargetUsagesBytes),
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
		diskGetErrHandler := func(err error) error {
			if err == nil {
				return nil
			}
			// TODO: match error
			log.Logger.Errorw("error getting disk data -- ignoring for now", "error", err)
			return nil
		}

		defaultPoller = query.New(
			disk_id.Name,
			cfg.Query,
			CreateGet(cfg),
			diskGetErrHandler,
		)
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

		prevFailed := false
		for i := 0; i < 5; i++ {
			cctx, ccancel := context.WithTimeout(ctx, time.Minute)
			blks, err := disk.GetBlockDevices(cctx, disk.WithDeviceType(func(dt string) bool {
				return dt == "disk"
			}))
			ccancel()
			if err != nil {
				log.Logger.Errorw("failed to get block devices", "error", err)

				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-time.After(5 * time.Second):
				}

				prevFailed = true
				continue
			}

			o.DiskBlockDevices = blks
			if prevFailed {
				log.Logger.Infow("successfully got block devices after retries", "num_block_devices", len(blks))
			}
			break
		}
		if len(o.DiskBlockDevices) == 0 {
			return nil, errors.New("no block device found")
		}

		prevFailed = false
		for i := 0; i < 5; i++ {
			cctx, ccancel := context.WithTimeout(ctx, time.Minute)
			parts, err := disk.GetPartitions(cctx, disk.WithFstype(func(fs string) bool {
				return fs == "ext4"
			}))
			ccancel()
			if err != nil {
				log.Logger.Errorw("failed to get partitions", "error", err)

				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-time.After(5 * time.Second):
				}

				prevFailed = true
				continue
			}

			o.DiskExtPartitions = parts
			if prevFailed {
				log.Logger.Infow("successfully got partitions after retries", "num_partitions", len(parts))
			}
			break
		}
		if len(o.DiskExtPartitions) == 0 {
			return nil, errors.New("no ext4 partition found")
		}

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

			cctx, ccancel := context.WithTimeout(ctx, time.Minute)
			mntOut, err := disk.FindMnt(cctx, target)
			ccancel()
			if err != nil {
				log.Logger.Warnw("error finding mount target device", "mount_target", target, "error", err)
				continue
			}

			if o.MountTargetUsages == nil {
				o.MountTargetUsages = make(map[string]disk.FindMntOutput)
			}
			o.MountTargetUsages[target] = *mntOut
		}

		return o, nil
	}
}
