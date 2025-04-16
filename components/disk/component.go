// Package disk tracks the disk usage of all the mount points specified in the configuration.
package disk

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/olekukonko/tablewriter"
	"github.com/prometheus/client_golang/prometheus"

	apiv1 "github.com/leptonai/gpud/api/v1"
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

func New(gpudInstance *components.GPUdInstance) (components.Component, error) {
	cctx, ccancel := context.WithCancel(gpudInstance.RootCtx)
	c := &component{
		ctx:    cctx,
		cancel: ccancel,

		getExt4PartitionsFunc: func(ctx context.Context) (disk.Partitions, error) {
			return disk.GetPartitions(ctx, disk.WithFstype(func(fs string) bool {
				return fs == "ext4"
			}))
		},
		findMntFunc: disk.FindMnt,
	}
	if runtime.GOOS == "linux" {
		// relies on "lsblk" command
		c.getBlockDevicesFunc = func(ctx context.Context) (disk.BlockDevices, error) {
			return disk.GetBlockDevices(ctx, disk.WithDeviceType(func(dt string) bool {
				return dt == "disk"
			}))
		}
	}

	muntPointsToTrackUsage := make(map[string]struct{})
	for _, mp := range gpudInstance.MountPoints {
		muntPointsToTrackUsage[mp] = struct{}{}
	}
	for _, mt := range gpudInstance.MountTargets {
		muntPointsToTrackUsage[mt] = struct{}{}
	}
	c.mountPointsToTrackUsage = muntPointsToTrackUsage

	return c, nil
}

func (c *component) Name() string { return Name }

func (c *component) Start() error {
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()

		for {
			_ = c.Check()
			select {
			case <-c.ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	return nil
}

func (c *component) LastHealthStates() apiv1.HealthStates {
	c.lastMu.RLock()
	lastData := c.lastData
	c.lastMu.RUnlock()
	return lastData.getLastHealthStates()
}

func (c *component) Events(ctx context.Context, since time.Time) (apiv1.Events, error) {
	return nil, nil
}

func (c *component) Close() error {
	log.Logger.Debugw("closing component")

	c.cancel()

	return nil
}

func (c *component) Check() components.CheckResult {
	log.Logger.Infow("checking disk")

	d := &Data{
		ts: time.Now().UTC(),
	}
	defer func() {
		c.lastMu.Lock()
		c.lastData = d
		c.lastMu.Unlock()
	}()

	if c.getBlockDevicesFunc != nil {
		prevFailed := false
		for i := 0; i < 5; i++ {
			cctx, ccancel := context.WithTimeout(c.ctx, time.Minute)
			blks, err := c.getBlockDevicesFunc(cctx)
			ccancel()
			if err != nil {
				log.Logger.Errorw("failed to get block devices", "error", err)

				select {
				case <-c.ctx.Done():
					d.health = apiv1.StateTypeUnhealthy
					d.err = c.ctx.Err()
					return d
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
			d.health = apiv1.StateTypeHealthy
			d.reason = "no block device found"
			return d
		}
	}

	prevFailed := false
	for i := 0; i < 5; i++ {
		cctx, ccancel := context.WithTimeout(c.ctx, time.Minute)
		parts, err := c.getExt4PartitionsFunc(cctx)
		ccancel()
		if err != nil {
			log.Logger.Errorw("failed to get partitions", "error", err)

			select {
			case <-c.ctx.Done():
				d.health = apiv1.StateTypeUnhealthy
				d.err = c.ctx.Err()
				return d
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
		d.health = apiv1.StateTypeHealthy
		d.reason = "no ext4 partition found"
		return d
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

	d.health = apiv1.StateTypeHealthy
	d.reason = fmt.Sprintf("found %d ext4 partition(s) and %d block device(s)", len(d.ExtPartitions), len(d.BlockDevices))

	return d
}

var _ components.CheckResult = &Data{}

type Data struct {
	ExtPartitions     disk.Partitions               `json:"ext_partitions"`
	BlockDevices      disk.BlockDevices             `json:"block_devices"`
	MountTargetUsages map[string]disk.FindMntOutput `json:"mount_target_usages"`

	// timestamp of the last check
	ts time.Time
	// error from the last check
	err error

	// tracks the healthy evaluation result of the last check
	health apiv1.HealthStateType
	// tracks the reason of the last check
	reason string
}

func (d *Data) String() string {
	if d == nil || len(d.ExtPartitions) == 0 {
		return ""
	}

	buf := bytes.NewBuffer(nil)
	table := tablewriter.NewWriter(buf)
	table.SetAlignment(tablewriter.ALIGN_CENTER)
	table.SetHeader([]string{"Mount Point", "Total", "Free", "Used", "Used %"})

	for _, p := range d.ExtPartitions {
		table.Append([]string{
			p.MountPoint,
			p.Usage.TotalHumanized,
			p.Usage.FreeHumanized,
			p.Usage.UsedHumanized,
			p.Usage.UsedPercent + " %",
		})
	}

	table.Render()
	return buf.String()
}

func (d *Data) Summary() string {
	if d == nil {
		return ""
	}
	return d.reason
}

func (d *Data) HealthState() apiv1.HealthStateType {
	if d == nil {
		return ""
	}
	return d.health
}

func (d *Data) getError() string {
	if d == nil || d.err == nil {
		return ""
	}
	return d.err.Error()
}

func (d *Data) getLastHealthStates() apiv1.HealthStates {
	if d == nil {
		return []apiv1.HealthState{
			{
				Name:   Name,
				Health: apiv1.StateTypeHealthy,
				Reason: "no data yet",
			},
		}
	}

	state := apiv1.HealthState{
		Name:   Name,
		Reason: d.reason,
		Error:  d.getError(),
		Health: d.health,
	}

	b, _ := json.Marshal(d)
	state.DeprecatedExtraInfo = map[string]string{
		"data":     string(b),
		"encoding": "json",
	}
	return apiv1.HealthStates{state}
}
