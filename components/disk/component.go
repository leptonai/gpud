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

	"github.com/dustin/go-humanize"
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

	lastMu          sync.RWMutex
	lastCheckResult *checkResult
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
			return disk.GetBlockDevicesWithLsblk(
				ctx,
				disk.WithFstype(func(fs string) bool {
					return fs == "" || fs == "ext4" || fs == "LVM2_member"
				}),
				disk.WithDeviceType(func(dt string) bool {
					return dt == "disk" || dt == "lvm" || dt == "part"
				},
				))
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
	lastCheckResult := c.lastCheckResult
	c.lastMu.RUnlock()
	return lastCheckResult.getLastHealthStates()
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

	cr := &checkResult{
		ts: time.Now().UTC(),
	}
	defer func() {
		c.lastMu.Lock()
		c.lastCheckResult = cr
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
					cr.health = apiv1.HealthStateTypeUnhealthy
					cr.err = c.ctx.Err()
					return cr
				case <-time.After(5 * time.Second):
				}

				prevFailed = true
				continue
			}

			cr.BlockDevices = blks.Flatten()
			if prevFailed {
				log.Logger.Infow("successfully got block devices after retries", "num_block_devices", len(cr.BlockDevices))
			}
			break
		}
		if len(cr.BlockDevices) == 0 {
			cr.health = apiv1.HealthStateTypeHealthy
			cr.reason = "no block device found"
			return cr
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
				cr.health = apiv1.HealthStateTypeUnhealthy
				cr.err = c.ctx.Err()
				return cr
			case <-time.After(5 * time.Second):
			}

			prevFailed = true
			continue
		}

		cr.ExtPartitions = parts
		if prevFailed {
			log.Logger.Infow("successfully got partitions after retries", "num_partitions", len(parts))
		}
		break
	}
	if len(cr.ExtPartitions) == 0 {
		cr.health = apiv1.HealthStateTypeHealthy
		cr.reason = "no ext4 partition found"
		return cr
	}

	devToUsage := make(map[string]disk.Usage)
	for _, p := range cr.ExtPartitions {
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
			if os.IsNotExist(err) {
				log.Logger.Debugw("mount target does not exist", "target", target)
				continue
			}

			log.Logger.Errorw("failed to check mount target", "target", target, "error", err)
			continue
		}

		cctx, ccancel := context.WithTimeout(c.ctx, time.Minute)
		mntOut, err := c.findMntFunc(cctx, target)
		ccancel()
		if err != nil {
			log.Logger.Errorw("error finding mount target device", "mount_target", target, "error", err)
			continue
		}

		if cr.MountTargetUsages == nil {
			cr.MountTargetUsages = make(map[string]disk.FindMntOutput)
		}
		cr.MountTargetUsages[target] = *mntOut
	}

	cr.health = apiv1.HealthStateTypeHealthy
	cr.reason = fmt.Sprintf("found %d ext4 partition(s) and %d block device(s)", len(cr.ExtPartitions), len(cr.BlockDevices))

	return cr
}

var _ components.CheckResult = &checkResult{}

type checkResult struct {
	ExtPartitions     disk.Partitions               `json:"ext_partitions"`
	BlockDevices      disk.FlattenedBlockDevices    `json:"block_devices"`
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

func (cr *checkResult) String() string {
	if cr == nil || len(cr.ExtPartitions) == 0 {
		return ""
	}

	buf := bytes.NewBuffer(nil)
	table := tablewriter.NewWriter(buf)
	table.SetAlignment(tablewriter.ALIGN_CENTER)
	table.SetHeader([]string{"Mount Point", "Total", "Free", "Used", "Used %"})
	for _, p := range cr.ExtPartitions {
		if p.Usage == nil {
			continue
		}

		table.Append([]string{
			p.MountPoint,
			humanize.Bytes(p.Usage.TotalBytes),
			humanize.Bytes(p.Usage.FreeBytes),
			humanize.Bytes(p.Usage.UsedBytes),
			p.Usage.UsedPercent + " %",
		})
	}
	table.Render()
	output := buf.String()

	if len(cr.BlockDevices) > 0 {
		output += "\n\n"

		buf.Reset()
		cr.BlockDevices.RenderTable(buf)
		output += buf.String()
	}

	if len(cr.MountTargetUsages) > 0 {
		output += "\n\n"

		buf.Reset()
		table := tablewriter.NewWriter(buf)
		table.SetAlignment(tablewriter.ALIGN_CENTER)
		table.SetHeader([]string{"Mount Point", "Total", "Free", "Used", "Used %"})
		for target, usage := range cr.MountTargetUsages {
			for _, fs := range usage.Filesystems {
				table.Append([]string{
					target,
					fs.SizeHumanized,
					fs.AvailableHumanized,
					fs.UsedHumanized,
					fs.UsedPercentHumanized,
				})
			}
		}
		table.Render()
		output += buf.String()
	}

	return output
}

func (cr *checkResult) Summary() string {
	if cr == nil {
		return ""
	}
	return cr.reason
}

func (cr *checkResult) HealthState() apiv1.HealthStateType {
	if cr == nil {
		return ""
	}
	return cr.health
}

func (cr *checkResult) getError() string {
	if cr == nil || cr.err == nil {
		return ""
	}
	return cr.err.Error()
}

func (cr *checkResult) getLastHealthStates() apiv1.HealthStates {
	if cr == nil {
		return apiv1.HealthStates{
			{
				Component: Name,
				Name:      Name,
				Health:    apiv1.HealthStateTypeHealthy,
				Reason:    "no data yet",
			},
		}
	}

	state := apiv1.HealthState{
		Component: Name,
		Name:      Name,
		Reason:    cr.reason,
		Error:     cr.getError(),
		Health:    cr.health,
	}

	b, _ := json.Marshal(cr)
	state.ExtraInfo = map[string]string{
		"data":     string(b),
		"encoding": "json",
	}
	return apiv1.HealthStates{state}
}
