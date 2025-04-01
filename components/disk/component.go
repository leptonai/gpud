// Package disk tracks the disk usage of all the mount points specified in the configuration.
package disk

import (
	"context"
	"encoding/json"
	"errors"
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
		ctx:                     cctx,
		cancel:                  ccancel,
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
		blks, err := disk.GetBlockDevices(cctx, disk.WithDeviceType(func(dt string) bool {
			return dt == "disk"
		}))
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
		d.err = errors.New("no block device found")
		return
	}

	prevFailed = false
	for i := 0; i < 5; i++ {
		cctx, ccancel := context.WithTimeout(c.ctx, time.Minute)
		parts, err := disk.GetPartitions(cctx, disk.WithFstype(func(fs string) bool {
			return fs == "ext4"
		}))
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
		d.err = errors.New("no ext4 partition found")
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

		totalBytes.With(prometheus.Labels{pkgmetrics.MetricLabelKey: p.MountPoint}).Set(float64(usage.TotalBytes))
		freeBytes.With(prometheus.Labels{pkgmetrics.MetricLabelKey: p.MountPoint}).Set(float64(usage.FreeBytes))
		usedBytes.With(prometheus.Labels{pkgmetrics.MetricLabelKey: p.MountPoint}).Set(float64(usage.UsedBytes))
		usedBytesPercent.With(prometheus.Labels{pkgmetrics.MetricLabelKey: p.MountPoint}).Set(usage.UsedPercentFloat)
		usedInodesPercent.With(prometheus.Labels{pkgmetrics.MetricLabelKey: p.MountPoint}).Set(usage.InodesUsedPercentFloat)
	}

	for target := range c.mountPointsToTrackUsage {
		if _, err := os.Stat(target); err != nil {
			log.Logger.Errorw("mount target does not exist", "mount_target", target)
			continue
		}

		cctx, ccancel := context.WithTimeout(c.ctx, time.Minute)
		mntOut, err := disk.FindMnt(cctx, target)
		ccancel()
		if err != nil {
			log.Logger.Warnw("error finding mount target device", "mount_target", target, "error", err)
			continue
		}

		if d.MountTargetUsages == nil {
			d.MountTargetUsages = make(map[string]disk.FindMntOutput)
		}
		d.MountTargetUsages[target] = *mntOut
	}
}

type Data struct {
	ExtPartitions     disk.Partitions               `json:"ext_partitions"`
	BlockDevices      disk.BlockDevices             `json:"block_devices"`
	MountTargetUsages map[string]disk.FindMntOutput `json:"mount_target_usages"`

	// timestamp of the last check
	ts time.Time `json:"-"`
	// error from the last check
	err error `json:"-"`
}

func (d *Data) getReason() string {
	if d == nil {
		return "no disk data"
	}
	if d.err != nil {
		return fmt.Sprintf("failed to get disk data -- %s", d.err)
	}

	return fmt.Sprintf("found %d ext4 partitions and %d block devices", len(d.ExtPartitions), len(d.BlockDevices))
}

func (d *Data) getHealth() (string, bool) {
	healthy := d == nil || d.err == nil
	health := components.StateHealthy
	if !healthy {
		health = components.StateUnhealthy
	}

	return health, healthy
}

func (d *Data) getStates() ([]components.State, error) {
	state := components.State{
		Name:   "disk",
		Reason: d.getReason(),
	}
	state.Health, state.Healthy = d.getHealth()

	b, _ := json.Marshal(d)
	state.ExtraInfo = map[string]string{
		"data":     string(b),
		"encoding": "json",
	}
	return []components.State{state}, nil
}
