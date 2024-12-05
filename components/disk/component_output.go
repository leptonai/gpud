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
	ExtPartitions disk.Partitions `json:"ext_partitions"`
}

const (
	StateNameDiskExtPartitions             = "disk_ext_partitions"
	StateKeyDiskExtPartitionsData          = "data"
	StateKeyDiskExtPartitionsEncoding      = "encoding"
	StateValueDiskExtPartitionEncodingJSON = "json"

	StateNameMountedPartitionsEXT              = "mounted_partitions_ext"
	StateKeyMountedPartitionsEXTTotalBytes     = "mounted_total_bytes"
	StateKeyMountedPartitionsEXTTotalGB        = "mounted_total_gb"
	StateKeyMountedPartitionsEXTTotalHumanized = "mounted_total_humanized"
)

func (o *Output) States() ([]components.State, error) {
	b, err := o.ExtPartitions.JSON()
	if err != nil {
		return nil, err
	}

	totalMountedBytes := o.ExtPartitions.TotalBytes()
	totalMountedGB := float64(totalMountedBytes) / 1e9
	totalMountedBytesHumanized := humanize.Bytes(totalMountedBytes)

	return []components.State{
		{
			Name:    StateNameDiskExtPartitions,
			Healthy: true,
			Reason:  "",
			ExtraInfo: map[string]string{
				StateKeyDiskExtPartitionsData:     string(b),
				StateKeyDiskExtPartitionsEncoding: StateValueDiskExtPartitionEncodingJSON,
			},
		},
		{
			Name:    StateNameMountedPartitionsEXT,
			Healthy: true,
			Reason:  "",
			ExtraInfo: map[string]string{
				StateKeyMountedPartitionsEXTTotalBytes:     fmt.Sprintf("%d", totalMountedBytes),
				StateKeyMountedPartitionsEXTTotalGB:        fmt.Sprintf("%.2f", totalMountedGB),
				StateKeyMountedPartitionsEXTTotalHumanized: totalMountedBytesHumanized,
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
		extPartitions, err := disk.GetPartitions(cctx)
		if err != nil {
			return nil, err
		}
		for _, p := range extPartitions {
			o.ExtPartitions = append(o.ExtPartitions, p)
		}

		now := time.Now().UTC()
		nowUTC := float64(now.Unix())
		metrics.SetLastUpdateUnixSeconds(nowUTC)

		mountPointsToTrack := make(map[string]struct{})
		for _, p := range o.ExtPartitions {
			// do not track aggregated partitions
			if len(p.MountPoints) != 1 {
				continue
			}
			mountPointsToTrack[p.MountPoints[0]] = struct{}{}
		}

		// for _, path := range cfg.MountPointsToTrackUsage {
		for _, p := range o.ExtPartitions {
			// do not track aggregated partitions
			if len(p.MountPoints) != 1 {
				continue
			}
			if _, ok := mountPointsToTrack[p.MountPoints[0]]; !ok {
				continue
			}

			usage := p.Usage
			if usage == nil {
				log.Logger.Warnw("no usage found for mount point", "mount_point", p.MountPoints)
				continue
			}

			if err := metrics.SetTotalBytes(ctx, p.MountPoints[0], float64(usage.TotalBytes), now); err != nil {
				return nil, err
			}
			metrics.SetFreeBytes(p.MountPoints[0], float64(usage.FreeBytes))
			if err := metrics.SetUsedBytes(ctx, p.MountPoints[0], float64(usage.UsedBytes), now); err != nil {
				return nil, err
			}
			if err := metrics.SetUsedBytesPercent(ctx, p.MountPoints[0], usage.UsedPercentFloat, now); err != nil {
				return nil, err
			}
			metrics.SetUsedInodesPercent(p.MountPoints[0], usage.InodesUsedPercentFloat)
		}

		return o, nil
	}
}
