// Package disk tracks the disk usage of all the mount points specified in the configuration.
package disk

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/olekukonko/tablewriter"
	"github.com/prometheus/client_golang/prometheus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/disk"
	"github.com/leptonai/gpud/pkg/eventstore"
	pkgfile "github.com/leptonai/gpud/pkg/file"
	pkghost "github.com/leptonai/gpud/pkg/host"
	"github.com/leptonai/gpud/pkg/kmsg"
	"github.com/leptonai/gpud/pkg/log"
)

// Name is the ID of the disk component.
const Name = "disk"

var _ components.Component = &component{}

type component struct {
	ctx    context.Context
	cancel context.CancelFunc

	retryInterval time.Duration

	getBlockDevicesFunc func(ctx context.Context) (disk.BlockDevices, error)

	getExt4PartitionsFunc func(ctx context.Context) (disk.Partitions, error)
	getNFSPartitionsFunc  func(ctx context.Context) (disk.Partitions, error)

	findMntFunc func(ctx context.Context, target string) (*disk.FindMntOutput, error)

	// Function field for testable file operations
	statWithTimeoutFunc func(ctx context.Context, path string) (os.FileInfo, error)

	mountPointsToTrackUsage map[string]struct{}

	rebootEventStore pkghost.RebootEventStore
	eventBucket      eventstore.Bucket
	kmsgSyncer       *kmsg.Syncer

	// lookbackPeriod defines how far back to query historical reboot and disk failure events
	// when evaluating suggested repair actions. Default is 3 days.
	lookbackPeriod time.Duration

	lastMu          sync.RWMutex
	lastCheckResult *checkResult
}

const (
	defaultRetryInterval  = 5 * time.Second
	defaultLookbackPeriod = 3 * 24 * time.Hour // 3 days
)

func New(gpudInstance *components.GPUdInstance) (components.Component, error) {
	cctx, ccancel := context.WithCancel(gpudInstance.RootCtx)
	c := &component{
		ctx:    cctx,
		cancel: ccancel,

		retryInterval: defaultRetryInterval,

		rebootEventStore: gpudInstance.RebootEventStore,
		lookbackPeriod:   defaultLookbackPeriod,

		getExt4PartitionsFunc: func(ctx context.Context) (disk.Partitions, error) {
			return disk.GetPartitions(ctx, disk.WithFstype(disk.DefaultExt4FsTypeFunc), disk.WithMountPoint(disk.DefaultMountPointFunc))
		},
		getNFSPartitionsFunc: func(ctx context.Context) (disk.Partitions, error) {
			// statfs on nfs can incur network I/O or impact disk I/O performance
			// do not track usage for nfs partitions
			return disk.GetPartitions(ctx, disk.WithFstype(disk.DefaultNFSFsTypeFunc), disk.WithMountPoint(disk.DefaultMountPointFunc))
		},

		findMntFunc: disk.FindMnt,

		// Initialize file operation function field with real implementation
		statWithTimeoutFunc: pkgfile.StatWithTimeout,
	}

	if runtime.GOOS == "linux" {
		// relies on "lsblk" command
		c.getBlockDevicesFunc = func(ctx context.Context) (disk.BlockDevices, error) {
			return disk.GetBlockDevicesWithLsblk(
				ctx,
				disk.WithFstype(disk.DefaultFsTypeFunc),
				disk.WithDeviceType(disk.DefaultDeviceTypeFunc),
				disk.WithMountPoint(disk.DefaultMountPointFunc),
			)
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

	if gpudInstance.EventStore != nil {
		var err error
		c.eventBucket, err = gpudInstance.EventStore.Bucket(Name)
		if err != nil {
			ccancel()
			return nil, err
		}

		if os.Geteuid() == 0 {
			c.kmsgSyncer, err = kmsg.NewSyncer(cctx, Match, c.eventBucket)
			if err != nil {
				ccancel()
				return nil, err
			}
		}
	}

	return c, nil
}

func (c *component) Name() string { return Name }

func (c *component) Tags() []string {
	return []string{
		Name,
	}
}

func (c *component) IsSupported() bool {
	return true
}

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
	return lastCheckResult.HealthStates()
}

func (c *component) Events(ctx context.Context, since time.Time) (apiv1.Events, error) {
	if c.eventBucket == nil {
		return nil, nil
	}
	evs, err := c.eventBucket.Get(ctx, since)
	if err != nil {
		return nil, err
	}
	return evs.Events(), nil
}

func (c *component) Close() error {
	log.Logger.Debugw("closing component")

	c.cancel()

	if c.kmsgSyncer != nil {
		c.kmsgSyncer.Close()
	}
	if c.eventBucket != nil {
		c.eventBucket.Close()
	}

	return nil
}

func (c *component) Check() components.CheckResult {
	log.Logger.Infow("checking disk")

	cr := &checkResult{
		ts: time.Now().UTC(),

		health: apiv1.HealthStateTypeHealthy,
		reason: "ok",
	}
	defer func() {
		c.lastMu.Lock()
		c.lastCheckResult = cr
		c.lastMu.Unlock()
	}()

	// Check for disk failure events from kmsg (only if event store is available)
	if c.eventBucket != nil && c.rebootEventStore != nil {
		// Query recent events to check for disk failures
		recentEvents, err := c.eventBucket.Get(c.ctx, cr.ts.Add(-c.lookbackPeriod))
		if err != nil {
			cr.err = err
			cr.health = apiv1.HealthStateTypeUnhealthy
			cr.reason = "failed to get recent events"
			log.Logger.Warnw(cr.reason, "error", cr.err)
			return cr
		}

		// Filter recent events to only include the hardware-related disk failure events
		// Group events by type for individual evaluation
		raidFailureEvents := make(eventstore.Events, 0)
		fsReadOnlyEvents := make(eventstore.Events, 0)
		nvmePathFailureEvents := make(eventstore.Events, 0)
		nvmeTimeoutEvents := make(eventstore.Events, 0)
		nvmeDeviceDisabledEvents := make(eventstore.Events, 0)
		beyondEndOfDeviceEvents := make(eventstore.Events, 0)
		bufferIOErrorEvents := make(eventstore.Events, 0)
		superblockWriteErrorEvents := make(eventstore.Events, 0)
		failureReasons := make(map[string]string)
		for _, ev := range recentEvents {
			switch ev.Name {
			case eventRAIDArrayFailure:
				raidFailureEvents = append(raidFailureEvents, ev)
				cr.health = apiv1.HealthStateTypeUnhealthy
				failureReasons[eventRAIDArrayFailure] = messageRAIDArrayFailure

			case eventFilesystemReadOnly:
				fsReadOnlyEvents = append(fsReadOnlyEvents, ev)
				cr.health = apiv1.HealthStateTypeUnhealthy
				failureReasons[eventFilesystemReadOnly] = messageFilesystemReadOnly

			case eventNVMePathFailure:
				nvmePathFailureEvents = append(nvmePathFailureEvents, ev)
				cr.health = apiv1.HealthStateTypeUnhealthy
				failureReasons[eventNVMePathFailure] = messageNVMePathFailure

			case eventNVMeTimeout:
				nvmeTimeoutEvents = append(nvmeTimeoutEvents, ev)
				cr.health = apiv1.HealthStateTypeUnhealthy
				failureReasons[eventNVMeTimeout] = messageNVMeTimeout

			case eventNVMeDeviceDisabled:
				nvmeDeviceDisabledEvents = append(nvmeDeviceDisabledEvents, ev)
				cr.health = apiv1.HealthStateTypeUnhealthy
				failureReasons[eventNVMeDeviceDisabled] = messageNVMeDeviceDisabled

			case eventBeyondEndOfDevice:
				beyondEndOfDeviceEvents = append(beyondEndOfDeviceEvents, ev)
				cr.health = apiv1.HealthStateTypeUnhealthy
				failureReasons[eventBeyondEndOfDevice] = messageBeyondEndOfDevice

			case eventBufferIOError:
				bufferIOErrorEvents = append(bufferIOErrorEvents, ev)
				cr.health = apiv1.HealthStateTypeUnhealthy
				failureReasons[eventBufferIOError] = messageBufferIOError

			case eventSuperblockWriteError:
				superblockWriteErrorEvents = append(superblockWriteErrorEvents, ev)
				cr.health = apiv1.HealthStateTypeUnhealthy
				failureReasons[eventSuperblockWriteError] = messageSuperblockWriteError
			}
		}

		// Evaluate suggested actions for each event type
		// this remains null if no failures are detected
		// especially the case where no failure after reboot is detected
		var allSuggestedActions []*apiv1.SuggestedActions

		// If we found disk failures, evaluate suggested actions for each type
		hasFailures := len(raidFailureEvents) > 0 ||
			len(fsReadOnlyEvents) > 0 ||
			len(nvmePathFailureEvents) > 0 ||
			len(nvmeTimeoutEvents) > 0 ||
			len(nvmeDeviceDisabledEvents) > 0 ||
			len(beyondEndOfDeviceEvents) > 0 ||
			len(bufferIOErrorEvents) > 0 ||
			len(superblockWriteErrorEvents) > 0
		if hasFailures {
			// Look up past events to derive the suggested actions
			rebootEvents, err := c.rebootEventStore.GetRebootEvents(c.ctx, cr.ts.Add(-c.lookbackPeriod))
			if err != nil {
				cr.err = err
				cr.health = apiv1.HealthStateTypeUnhealthy
				log.Logger.Warnw("failed to get reboot events", "error", cr.err)
				return cr
			}

			// Process RAID failure events
			if len(raidFailureEvents) > 0 {
				suggestedActions := eventstore.EvaluateSuggestedActions(rebootEvents, raidFailureEvents, 2)
				if suggestedActions != nil {
					allSuggestedActions = append(allSuggestedActions, suggestedActions)
				} else {
					// e.g., failure -> reboot -> no failure
					delete(failureReasons, eventRAIDArrayFailure)
				}
			}

			// Process filesystem read-only events
			if len(fsReadOnlyEvents) > 0 {
				suggestedActions := eventstore.EvaluateSuggestedActions(rebootEvents, fsReadOnlyEvents, 2)
				if suggestedActions != nil {
					allSuggestedActions = append(allSuggestedActions, suggestedActions)
				} else {
					// e.g., failure -> reboot -> no failure
					delete(failureReasons, eventFilesystemReadOnly)
				}
			}

			// Process NVMe path failure events
			if len(nvmePathFailureEvents) > 0 {
				suggestedActions := eventstore.EvaluateSuggestedActions(rebootEvents, nvmePathFailureEvents, 2)
				if suggestedActions != nil {
					allSuggestedActions = append(allSuggestedActions, suggestedActions)
				} else {
					// e.g., failure -> reboot -> no failure
					delete(failureReasons, eventNVMePathFailure)
				}
			}

			// Process NVME controller timeout events
			if len(nvmeTimeoutEvents) > 0 {
				suggestedActions := eventstore.EvaluateSuggestedActions(rebootEvents, nvmeTimeoutEvents, 2)
				if suggestedActions != nil {
					allSuggestedActions = append(allSuggestedActions, suggestedActions)
				} else {
					// e.g., failure -> reboot -> no failure
					delete(failureReasons, eventNVMeTimeout)
				}
			}

			// Process NVME device disabled events
			if len(nvmeDeviceDisabledEvents) > 0 {
				suggestedActions := eventstore.EvaluateSuggestedActions(rebootEvents, nvmeDeviceDisabledEvents, 2)
				if suggestedActions != nil {
					allSuggestedActions = append(allSuggestedActions, suggestedActions)
				} else {
					// e.g., failure -> reboot -> no failure
					delete(failureReasons, eventNVMeDeviceDisabled)
				}
			}

			// Process I/O beyond device boundaries events
			if len(beyondEndOfDeviceEvents) > 0 {
				suggestedActions := eventstore.EvaluateSuggestedActions(rebootEvents, beyondEndOfDeviceEvents, 2)
				if suggestedActions != nil {
					allSuggestedActions = append(allSuggestedActions, suggestedActions)
				} else {
					// e.g., failure -> reboot -> no failure
					delete(failureReasons, eventBeyondEndOfDevice)
				}
			}

			// Process buffer I/O error events
			if len(bufferIOErrorEvents) > 0 {
				suggestedActions := eventstore.EvaluateSuggestedActions(rebootEvents, bufferIOErrorEvents, 2)
				if suggestedActions != nil {
					allSuggestedActions = append(allSuggestedActions, suggestedActions)
				} else {
					// e.g., failure -> reboot -> no failure
					delete(failureReasons, eventBufferIOError)
				}
			}

			// Process superblock write error events
			if len(superblockWriteErrorEvents) > 0 {
				suggestedActions := eventstore.EvaluateSuggestedActions(rebootEvents, superblockWriteErrorEvents, 2)
				if suggestedActions != nil {
					allSuggestedActions = append(allSuggestedActions, suggestedActions)
				} else {
					// e.g., failure -> reboot -> no failure
					delete(failureReasons, eventSuperblockWriteError)
				}
			}

			// Aggregate suggested actions with HW_INSPECTION priority
			cr.suggestedActions = eventstore.AggregateSuggestedActions(allSuggestedActions)
		}

		// Append failure reasons to existing reason if any failures were detected
		if len(failureReasons) > 0 {
			// reset since it's not healhty, not "ok" anymore
			if cr.reason == "ok" {
				cr.reason = ""
			}

			// Sort the keys lexicographically
			var sortedReasons []string
			for _, reason := range failureReasons {
				sortedReasons = append(sortedReasons, reason)
			}
			sort.Strings(sortedReasons)

			newReason := strings.Join(sortedReasons, ", ")
			if cr.reason != "" {
				cr.reason += "; " + newReason
			} else {
				cr.reason = newReason
			}
		} else if hasFailures {
			// If we had failures initially but all were resolved after reboot,
			// reset the health state back to healthy
			cr.health = apiv1.HealthStateTypeHealthy
		}
	}

	if c.getBlockDevicesFunc != nil {
		if !c.fetchBlockDevices(cr) {
			return cr
		}
	}
	if !c.fetchExt4Partitions(cr) {
		return cr
	}
	if !c.fetchNFSPartitions(cr) {
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

		metricTotalBytes.With(prometheus.Labels{"mount_point": p.MountPoint}).Set(float64(usage.TotalBytes))
		metricFreeBytes.With(prometheus.Labels{"mount_point": p.MountPoint}).Set(float64(usage.FreeBytes))
		metricUsedBytes.With(prometheus.Labels{"mount_point": p.MountPoint}).Set(float64(usage.UsedBytes))
	}

	for _, p := range cr.NFSPartitions {
		usage := p.Usage
		if usage == nil {
			log.Logger.Warnw("no usage found for mount point", "mount_point", p.MountPoint)
			continue
		}

		devToUsage[p.Device] = *usage

		if _, ok := c.mountPointsToTrackUsage[p.MountPoint]; !ok {
			continue
		}

		metricTotalBytes.With(prometheus.Labels{"mount_point": p.MountPoint}).Set(float64(usage.TotalBytes))
		metricFreeBytes.With(prometheus.Labels{"mount_point": p.MountPoint}).Set(float64(usage.FreeBytes))
		metricUsedBytes.With(prometheus.Labels{"mount_point": p.MountPoint}).Set(float64(usage.UsedBytes))
	}

	for target := range c.mountPointsToTrackUsage {
		timeoutCtx, cancel := context.WithTimeout(c.ctx, 5*time.Second)
		_, err := c.statWithTimeoutFunc(timeoutCtx, target)
		cancel()
		if err != nil {
			if os.IsNotExist(err) {
				log.Logger.Debugw("mount target does not exist", "target", target)
				continue
			}

			if errors.Is(err, context.DeadlineExceeded) {
				log.Logger.Warnw("stat operation timed out for mount target, may indicate unresponsive filesystem", "target", target, "error", err)
			} else {
				log.Logger.Errorw("failed to check mount target", "target", target, "error", err)
			}
			continue
		}

		// in case the command is flaky with unknown characters
		// e.g.,
		// "unexpected end of JSON input"
		prevFailed := false
		for i := 0; i < 5; i++ {
			cctx, ccancel := context.WithTimeout(c.ctx, time.Minute)
			mntOut, err := c.findMntFunc(cctx, target)
			ccancel()
			if err != nil {
				log.Logger.Errorw("failed to find mnt", "error", err)

				select {
				case <-c.ctx.Done():
					cr.health = apiv1.HealthStateTypeUnhealthy
					cr.err = c.ctx.Err()
					return cr
				case <-time.After(c.retryInterval):
				}

				prevFailed = true
				continue
			}

			if cr.MountTargetUsages == nil {
				cr.MountTargetUsages = make(map[string]disk.FindMntOutput)
			}
			cr.MountTargetUsages[target] = *mntOut
			if prevFailed {
				log.Logger.Infow("successfully ran findmnt after retries")
			}
			break
		}
	}

	if len(cr.BlockDevices) > 0 {
		if len(cr.ExtPartitions) > 0 {
			cr.DeviceUsages = cr.BlockDevices.GetDeviceUsages(cr.ExtPartitions)
		}

		if len(cr.NFSPartitions) > 0 {
			for _, p := range cr.NFSPartitions {
				usage := p.Usage
				if usage == nil {
					log.Logger.Warnw("no usage found for mount point", "mount_point", p.MountPoint)
					continue
				}

				cr.DeviceUsages = append(cr.DeviceUsages, disk.DeviceUsage{
					DeviceName: p.Device,
					MountPoint: p.MountPoint,
					TotalBytes: usage.TotalBytes,
					FreeBytes:  usage.FreeBytes,
					UsedBytes:  usage.UsedBytes,
				})
			}
		}
	}

	for _, p := range cr.NFSPartitions {
		if p.StatTimedOut {
			if cr.reason == "ok" {
				cr.reason = ""
			}
			if cr.reason != "" {
				cr.reason += "; "
			}
			cr.reason += fmt.Sprintf("%s not mounted and stat timed out", p.MountPoint)
			if cr.health == apiv1.HealthStateTypeHealthy {
				cr.health = apiv1.HealthStateTypeDegraded
			}
			break
		}
	}

	log.Logger.Debugw(cr.reason, "extPartitions", len(cr.ExtPartitions), "nfsPartitions", len(cr.NFSPartitions), "blockDevices", len(cr.BlockDevices))

	return cr
}

func (c *component) fetchBlockDevices(cr *checkResult) bool {
	// in case the command is flaky with unknown characters
	// e.g.,
	// "unexpected end of JSON input"
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
				return false
			case <-time.After(c.retryInterval):
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
		log.Logger.Warnw("no block device found -- something must be wrong with lsblk command")

		if cr.health == "" {
			cr.health = apiv1.HealthStateTypeHealthy
		}

		if cr.reason == "ok" {
			cr.reason = ""
		}
		if cr.reason != "" {
			cr.reason += "; "
		}
		cr.reason += "no block device found"

		return false
	}
	return true
}

func (c *component) fetchExt4Partitions(cr *checkResult) bool {
	// in case the command is flaky with unknown characters
	// e.g.,
	// "unexpected end of JSON input"
	prevFailed := false
	for i := 0; i < 5; i++ {
		cctx, ccancel := context.WithTimeout(c.ctx, time.Minute)
		parts, err := c.getExt4PartitionsFunc(cctx)
		ccancel()
		if err != nil {
			log.Logger.Errorw("failed to get ext4 partitions", "error", err)

			select {
			case <-c.ctx.Done():
				cr.health = apiv1.HealthStateTypeUnhealthy

				if cr.reason == "ok" {
					cr.reason = ""
				}
				if cr.reason != "" {
					cr.reason += "; "
				}
				cr.reason += "failed to get ext4 partitions"

				cr.err = c.ctx.Err()
				return false
			case <-time.After(c.retryInterval):
			}

			prevFailed = true
			continue
		}

		cr.ExtPartitions = parts
		if prevFailed {
			log.Logger.Infow("successfully got ext4 partitions after retries", "num_partitions", len(parts))
		}
		break
	}
	return true
}

func (c *component) fetchNFSPartitions(cr *checkResult) bool {
	prevFailed := false
	for i := 0; i < 5; i++ {
		cctx, ccancel := context.WithTimeout(c.ctx, time.Minute)
		parts, err := c.getNFSPartitionsFunc(cctx)
		ccancel()
		if err != nil {
			log.Logger.Errorw("failed to get nfs partitions", "error", err)

			select {
			case <-c.ctx.Done():
				cr.health = apiv1.HealthStateTypeUnhealthy
				cr.err = c.ctx.Err()
				return false
			case <-time.After(c.retryInterval):
			}

			prevFailed = true
			continue
		}

		cr.NFSPartitions = parts
		if prevFailed {
			log.Logger.Infow("successfully got nfs partitions after retries", "num_partitions", len(parts))
		}
		break
	}
	return true
}

var _ components.CheckResult = &checkResult{}

type checkResult struct {
	ExtPartitions disk.Partitions `json:"ext_partitions"`
	NFSPartitions disk.Partitions `json:"nfs_partitions"`

	BlockDevices disk.FlattenedBlockDevices `json:"block_devices"`

	// DeviceUsages is derived from BlockDevices and ExtPartitions/NFSPartitions.
	DeviceUsages disk.DeviceUsages `json:"device_usages"`

	MountTargetUsages map[string]disk.FindMntOutput `json:"mount_target_usages"`

	// timestamp of the last check
	ts time.Time
	// error from the last check
	err error

	// tracks the healthy evaluation result of the last check
	health apiv1.HealthStateType
	// tracks the suggested actions for the last check
	suggestedActions *apiv1.SuggestedActions
	// tracks the reason of the last check
	reason string
}

func (cr *checkResult) ComponentName() string {
	return Name
}

func (cr *checkResult) String() string {
	if cr == nil || len(cr.ExtPartitions) == 0 {
		return ""
	}

	buf := bytes.NewBuffer(nil)
	cr.ExtPartitions.RenderTable(buf)
	output := buf.String()

	if len(cr.NFSPartitions) > 0 {
		output += "\n\n"

		buf.Reset()
		cr.NFSPartitions.RenderTable(buf)
		output += buf.String()
	}

	if len(cr.BlockDevices) > 0 {
		output += "\n\n"

		buf.Reset()
		cr.BlockDevices.RenderTable(buf)
		output += buf.String()
	}

	if len(cr.DeviceUsages) > 0 {
		output += "\n\n"

		buf.Reset()
		cr.DeviceUsages.RenderTable(buf)
		output += buf.String()
	}

	if len(cr.MountTargetUsages) > 0 {
		output += "\n\n"

		buf.Reset()
		table := tablewriter.NewWriter(buf)
		table.SetAlignment(tablewriter.ALIGN_CENTER)
		table.SetHeader([]string{"Mount Point", "Sources", "Total", "Free", "Used", "Used %"})
		for target, usage := range cr.MountTargetUsages {
			for _, fs := range usage.Filesystems {
				table.Append([]string{
					target,
					strings.Join(fs.Sources, "\n"),
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

func (cr *checkResult) HealthStateType() apiv1.HealthStateType {
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

func (cr *checkResult) getSuggestedActions() *apiv1.SuggestedActions {
	if cr == nil {
		return nil
	}
	return cr.suggestedActions
}

func (cr *checkResult) HealthStates() apiv1.HealthStates {
	if cr == nil {
		return apiv1.HealthStates{
			{
				Time:      metav1.NewTime(time.Now().UTC()),
				Component: Name,
				Name:      Name,
				Health:    apiv1.HealthStateTypeHealthy,
				Reason:    "no data yet",
			},
		}
	}

	state := apiv1.HealthState{
		Time:             metav1.NewTime(cr.ts),
		Component:        Name,
		Name:             Name,
		Reason:           cr.reason,
		Error:            cr.getError(),
		Health:           cr.health,
		SuggestedActions: cr.getSuggestedActions(),
	}

	if len(cr.ExtPartitions) > 0 && len(cr.BlockDevices) > 0 {
		b, _ := json.Marshal(cr)
		state.ExtraInfo = map[string]string{"data": string(b)}
	}
	return apiv1.HealthStates{state}
}
