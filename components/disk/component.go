// Package disk tracks the disk usage of all the mount points specified in the configuration.
package disk

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/disk"
	"github.com/leptonai/gpud/pkg/log"
	pkgmetrics "github.com/leptonai/gpud/pkg/metrics"
)

// Name is the ID of the disk component.
const Name = "disk"

var _ components.Component = &component{}

type component struct {
	ctx    context.Context
	cancel context.CancelFunc

	getBlockDevicesFunc   func(ctx context.Context) (disk.BlockDevices, error)
	getExt4PartitionsFunc func(ctx context.Context) (disk.Partitions, error)
	findMntFunc           func(ctx context.Context, target string) (*disk.FindMntOutput, error)

	mountPointsToTrackUsage map[string]struct{}

	lastMu   sync.RWMutex
	lastData *Data
}

func New(ctx context.Context, mountPoints []string, mountTargets []string) components.Component {
	mountPointsToTrackUsage := make(map[string]struct{})
	for _, mp := range mountPoints {
		mountPointsToTrackUsage[mp] = struct{}{}
	}
	for _, mt := range mountTargets {
		mountPointsToTrackUsage[mt] = struct{}{}
	}

	cctx, ccancel := context.WithCancel(ctx)
	return &component{
		ctx:    cctx,
		cancel: ccancel,

		getBlockDevicesFunc: func(ctx context.Context) (disk.BlockDevices, error) {
			return disk.GetBlockDevices(ctx, disk.WithDeviceType(func(dt string) bool {
				return dt == "disk"
			}))
		},
		getExt4PartitionsFunc: func(ctx context.Context) (disk.Partitions, error) {
			return disk.GetPartitions(ctx, disk.WithFstype(func(fs string) bool {
				return fs == "ext4"
			}))
		},
		findMntFunc: disk.FindMnt,

		mountPointsToTrackUsage: mountPointsToTrackUsage,
	}
}

func (c *component) Name() string { return Name }

func (c *component) Start() error {
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()

		for {
			c.CheckOnce()

			select {
			case <-c.ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	return nil
}

func (c *component) States(ctx context.Context) ([]components.State, error) {
	c.lastMu.RLock()
	lastData := c.lastData
	c.lastMu.RUnlock()
	return lastData.getStates()
}

func (c *component) Events(ctx context.Context, since time.Time) ([]components.Event, error) {
	return nil, nil
}

func (c *component) Close() error {
	log.Logger.Debugw("closing component")

	c.cancel()

	return nil
}

// CheckOnce checks the current pods
// run this periodically
func (c *component) CheckOnce() {
	log.Logger.Infow("checking disk")
	d := Data{
		ts: time.Now().UTC(),
	}
	defer func() {
		c.lastMu.Lock()
		c.lastData = &d
		c.lastMu.Unlock()
	}()

	prevFailed := false
	for i := 0; i < 5; i++ {
		cctx, ccancel := context.WithTimeout(c.ctx, time.Minute)
		blks, err := c.getBlockDevicesFunc(cctx)
		ccancel()
		if err != nil {
			log.Logger.Errorw("failed to get block devices", "error", err)

			select {
			case <-c.ctx.Done():
				d.err = c.ctx.Err()
				return
			case <-time.After(5 * time.Second):
			}

			prevFailed = true
			continue
		}

		d.BlockDevices = blks
		if prevFailed {
			log.Logger.Infow("successfully got block devices after retries", "num_block_devices", len(blks))
		}
		break
	}
	if len(d.BlockDevices) == 0 {
		d.healthy = true
		d.reason = "no block device found"
		return
	}

	prevFailed = false
	for i := 0; i < 5; i++ {
		cctx, ccancel := context.WithTimeout(c.ctx, time.Minute)
		parts, err := c.getExt4PartitionsFunc(cctx)
		ccancel()
		if err != nil {
			log.Logger.Errorw("failed to get partitions", "error", err)

			select {
			case <-c.ctx.Done():
				d.err = c.ctx.Err()
				return
			case <-time.After(5 * time.Second):
			}

			prevFailed = true
			continue
		}

		d.ExtPartitions = parts
		if prevFailed {
			log.Logger.Infow("successfully got partitions after retries", "num_partitions", len(parts))
		}
		break
	}
	if len(d.ExtPartitions) == 0 {
		d.healthy = true
		d.reason = "no ext4 partition found"
		return
	}

	devToUsage := make(map[string]disk.Usage)
	for _, p := range d.ExtPartitions {
		usage := p.Usage
		if usage == nil {
			log.Logger.Warnw("no usage found for mount point", "mount_point", p.MountPoint)
			continue
		}

		devToUsage[p.Device] = *usage

		if _, ok := c.mountPointsToTrackUsage[p.MountPoint]; !ok {
			continue
		}

		metricTotalBytes.With(prometheus.Labels{pkgmetrics.MetricLabelKey: p.MountPoint}).Set(float64(usage.TotalBytes))
		metricFreeBytes.With(prometheus.Labels{pkgmetrics.MetricLabelKey: p.MountPoint}).Set(float64(usage.FreeBytes))
		metricUsedBytes.With(prometheus.Labels{pkgmetrics.MetricLabelKey: p.MountPoint}).Set(float64(usage.UsedBytes))
		metricUsedBytesPercent.With(prometheus.Labels{pkgmetrics.MetricLabelKey: p.MountPoint}).Set(usage.UsedPercentFloat)
		metricUsedInodesPercent.With(prometheus.Labels{pkgmetrics.MetricLabelKey: p.MountPoint}).Set(usage.InodesUsedPercentFloat)
	}

	for target := range c.mountPointsToTrackUsage {
		if _, err := os.Stat(target); err != nil {
			log.Logger.Errorw("mount target does not exist", "mount_target", target)
			continue
		}

		cctx, ccancel := context.WithTimeout(c.ctx, time.Minute)
		mntOut, err := c.findMntFunc(cctx, target)
		ccancel()
		if err != nil {
			log.Logger.Errorw("error finding mount target device", "mount_target", target, "error", err)
			continue
		}

		if d.MountTargetUsages == nil {
			d.MountTargetUsages = make(map[string]disk.FindMntOutput)
		}
		d.MountTargetUsages[target] = *mntOut
	}

	d.healthy = true
	d.reason = fmt.Sprintf("found %d ext4 partition(s) and %d block device(s)", len(d.ExtPartitions), len(d.BlockDevices))
}

type Data struct {
	ExtPartitions     disk.Partitions               `json:"ext_partitions"`
	BlockDevices      disk.BlockDevices             `json:"block_devices"`
	MountTargetUsages map[string]disk.FindMntOutput `json:"mount_target_usages"`

	// timestamp of the last check
	ts time.Time
	// error from the last check
	err error

	// tracks the healthy evaluation result of the last check
	healthy bool
	// tracks the reason of the last check
	reason string
}

func (d *Data) getError() string {
	if d == nil || d.err == nil {
		return ""
	}
	return d.err.Error()
}

func (d *Data) getStates() ([]components.State, error) {
	if d == nil {
		return []components.State{
			{
				Name:    Name,
				Health:  components.StateHealthy,
				Healthy: true,
				Reason:  "no data yet",
			},
		}, nil
	}

	state := components.State{
		Name:   Name,
		Reason: d.reason,
		Error:  d.getError(),

		Healthy: d.healthy,
		Health:  components.StateHealthy,
	}
	if !d.healthy {
		state.Health = components.StateUnhealthy
	}

	b, _ := json.Marshal(d)
	state.ExtraInfo = map[string]string{
		"data":     string(b),
		"encoding": "json",
	}
	return []components.State{state}, nil
}
