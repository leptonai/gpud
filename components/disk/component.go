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

	// lookback period to query the past reboot + disk failure events
	lookbackPeriod time.Duration

	lastMu          sync.RWMutex
	lastCheckResult *checkResult
}

const defaultRetryInterval = 5 * time.Second
const defaultLookbackPeriod = 3 * 24 * time.Hour

func New(gpudInstance *components.GPUdInstance) (components.Component, error) {
	cctx, ccancel := context.WithCancel(gpudInstance.RootCtx)
	c := &component{
		ctx:    cctx,
		cancel: ccancel,

		retryInterval: defaultRetryInterval,

		rebootEventStore: gpudInstance.RebootEventStore,
		lookbackPeriod:   defaultLookbackPeriod,

		getExt4PartitionsFunc: func(ctx context.Context) (disk.Partitions, error) {
			return disk.GetPartitions(ctx, disk.WithFstype(disk.DefaultExt4FsTypeFunc))
		},
		getNFSPartitionsFunc: func(ctx context.Context) (disk.Partitions, error) {
			// statfs on nfs can incur network I/O or impact disk I/O performance
			// do not track usage for nfs partitions
			return disk.GetPartitions(ctx, disk.WithFstype(disk.DefaultNFSFsTypeFunc), disk.WithSkipUsage())
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

func (c *component) recordDiskFailureEvent(cr *checkResult, eventName string, message string) error {
	ev := eventstore.Event{
		Component: Name,
		Time:      cr.ts,
		Name:      eventName,
		Type:      string(apiv1.EventTypeWarning),
		Message:   message,
	}

	cctx, ccancel := context.WithTimeout(c.ctx, 15*time.Second)
	found, err := c.eventBucket.Find(cctx, ev)
	ccancel()
	if err != nil {
		cr.err = err
		cr.health = apiv1.HealthStateTypeUnhealthy
		cr.reason = "error finding disk failure event"
		log.Logger.Warnw(cr.reason, "error", cr.err)
		return err
	}

	if found != nil {
		log.Logger.Infow("disk failure event already found in db", "event", eventName)
		return nil
	}

	// persist in event store (as it hasn't been)
	if err := c.eventBucket.Insert(c.ctx, ev); err != nil {
		cr.err = err
		cr.health = apiv1.HealthStateTypeUnhealthy
		cr.reason = "error inserting disk failure event"
		log.Logger.Warnw(cr.reason, "error", cr.err)
		return err
	}

	log.Logger.Infow("inserted disk failure event to db", "event", eventName)
	return nil
}

// evaluateSuggestedActions determines repair actions based on reboot history and disk failure events.
// Suggests up to 2 reboots, otherwise suggests hardware inspection.
//
// Logic mirrors GPU counts component:
// - Case 1: First disk failure (no previous reboots) → suggest reboot
// - Case 2: Disk failure after 1 reboot → suggest second reboot
// - Case 3: Disk failure after 2+ reboots → suggest hardware inspection
func evaluateSuggestedActions(cr *checkResult, rebootEvents eventstore.Events, diskFailureEvents eventstore.Events) {
	// since we just inserted the failure event before calling evaluateSuggestedActions
	// this should never happen... but handle just in case!
	if len(diskFailureEvents) == 0 {
		cr.err = errors.New("no disk failure event found after insert")
		cr.health = apiv1.HealthStateTypeUnhealthy
		cr.reason = "no disk failure event found after insert"
		log.Logger.Warnw(cr.reason, "error", cr.err)
		return
	}

	if len(rebootEvents) == 0 {
		// case 1. disk failure (first time); suggest "reboot"
		// (no previous reboot found)
		cr.suggestedActions = &apiv1.SuggestedActions{
			RepairActions: []apiv1.RepairActionType{
				apiv1.RepairActionTypeRebootSystem,
			},
		}
		log.Logger.Warnw("disk failure event found but no reboot event found -- suggesting reboot")
		return
	}

	// edge case:
	// we just inserted above, before calling evaluateSuggestedActions
	// assume reboot (if ever) happened before disk failure
	// if there's no following disk failure event after reboot,
	// we should ignore (reboot could have been triggered just now!)
	firstRebootTime := rebootEvents[0].Time
	firstFailureTime := diskFailureEvents[0].Time
	if firstRebootTime.After(firstFailureTime) {
		log.Logger.Warnw("no disk failure event found after reboot -- suggesting none")
		return
	}

	// now it's guaranteed that we have at least
	// one sequence of "disk failure -> reboot"
	if len(rebootEvents) == 1 || len(diskFailureEvents) == 1 {
		// there's been only one reboot event,
		// so now we know there's only one possible sequence of
		// "disk failure -> reboot"
		//
		// case 2.
		// disk failure -> reboot
		// -> disk failure; suggest second "reboot"
		// (after first reboot, we still get disk failure)
		//
		// or there's been only one reboot + only one disk failure event; suggest second "reboot"
		// (after first reboot, we still get disk failure)
		cr.suggestedActions = &apiv1.SuggestedActions{
			RepairActions: []apiv1.RepairActionType{
				apiv1.RepairActionTypeRebootSystem,
			},
		}
		log.Logger.Warnw("disk failure event found -- suggesting reboot", "rebootCount", len(rebootEvents), "failureCount", len(diskFailureEvents))
		return
	}

	// now that we have >=2 reboot events AND >=2 disk failure events
	// just check if there's been >=2 sequences of "reboot -> failure"
	// since it's possible that "reboot -> reboot -> failure -> failure"
	// which should only count as one sequence of "reboot -> failure"
	failureToReboot := make(map[time.Time]time.Time)

	for i := 0; i < len(diskFailureEvents); i++ {
		failureTime := diskFailureEvents[i].Time

		for j := 0; j < len(rebootEvents); j++ {
			rebootTime := rebootEvents[j].Time

			if failureTime.Before(rebootTime) {
				continue
			}

			if _, ok := failureToReboot[failureTime]; ok {
				// already seen this failure event with a corresponding reboot event
				continue
			}

			failureToReboot[failureTime] = rebootTime
		}
	}

	// case 3.
	// disk failure -> reboot
	// -> disk failure -> reboot
	// -> disk failure; suggest "hw inspection"
	// (after >=2 reboots, we still get disk failure)
	if len(failureToReboot) >= 2 {
		cr.suggestedActions = &apiv1.SuggestedActions{
			RepairActions: []apiv1.RepairActionType{
				apiv1.RepairActionTypeHardwareInspection,
			},
		}
		log.Logger.Warnw("multiple reboot -> disk failure event sequences found -- suggesting hw inspection", "rebootCount", len(rebootEvents), "failureCount", len(failureToReboot))
		return
	}

	// only one valid sequence of "reboot -> disk failure"
	// still suggest "reboot"
	cr.suggestedActions = &apiv1.SuggestedActions{
		RepairActions: []apiv1.RepairActionType{
			apiv1.RepairActionTypeRebootSystem,
		},
	}
	log.Logger.Warnw("only one valid sequence of 'reboot -> disk failure' found -- suggesting reboot", "rebootCount", len(rebootEvents), "failureCount", len(diskFailureEvents))
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

	if len(cr.NFSPartitions) == 0 && len(cr.ExtPartitions) == 0 {
		cr.health = apiv1.HealthStateTypeHealthy
		cr.reason = "no ext4/nfs partition found"
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

	cr.health = apiv1.HealthStateTypeHealthy
	cr.reason = "ok"

	for _, p := range cr.NFSPartitions {
		if p.StatTimedOut {
			cr.reason = fmt.Sprintf("%s not mounted and stat timed out", p.MountPoint)
			cr.health = apiv1.HealthStateTypeDegraded
			break
		}
	}

	log.Logger.Debugw(cr.reason, "extPartitions", len(cr.ExtPartitions), "nfsPartitions", len(cr.NFSPartitions), "blockDevices", len(cr.BlockDevices))

	// Check for disk failure events from kmsg (only if event store is available)
	if c.eventBucket != nil && c.rebootEventStore != nil {
		// Query recent events to check for disk failures
		recentEvents, err := c.eventBucket.Get(c.ctx, cr.ts.Add(-5*time.Minute))
		if err != nil {
			log.Logger.Warnw("failed to get recent events", "error", err)
		} else {
			// Check if we have any critical disk failure events
			var foundFailure bool
			var failureEventName string
			var failureMessage string

			for _, ev := range recentEvents {
				switch ev.Name {
				case eventRAIDArrayFailure:
					foundFailure = true
					failureEventName = ev.Name // Use the original event name
					failureMessage = "RAID array failure detected"
					cr.health = apiv1.HealthStateTypeUnhealthy
					cr.reason = failureMessage
				case eventFilesystemReadOnly:
					foundFailure = true
					failureEventName = ev.Name // Use the original event name
					failureMessage = "Filesystem remounted read-only"
					cr.health = apiv1.HealthStateTypeUnhealthy
					cr.reason = failureMessage
				case eventNVMePathFailure:
					foundFailure = true
					failureEventName = ev.Name // Use the original event name
					failureMessage = "NVMe path failure detected"
					cr.health = apiv1.HealthStateTypeUnhealthy
					cr.reason = failureMessage
				case eventNVMeTimeout:
					foundFailure = true
					failureEventName = ev.Name // Use the original event name
					failureMessage = "NVME controller timeout detected"
					cr.health = apiv1.HealthStateTypeUnhealthy
					cr.reason = failureMessage
				case eventNVMeDeviceDisabled:
					foundFailure = true
					failureEventName = ev.Name // Use the original event name
					failureMessage = "NVME device disabled after reset failure"
					cr.health = apiv1.HealthStateTypeUnhealthy
					cr.reason = failureMessage
				case eventBeyondEndOfDevice:
					foundFailure = true
					failureEventName = ev.Name // Use the original event name
					failureMessage = "I/O beyond device boundaries detected"
					cr.health = apiv1.HealthStateTypeUnhealthy
					cr.reason = failureMessage
				case eventBufferIOError:
					foundFailure = true
					failureEventName = ev.Name // Use the original event name
					failureMessage = "Buffer I/O error detected"
					cr.health = apiv1.HealthStateTypeUnhealthy
					cr.reason = failureMessage
				}

				if foundFailure {
					break
				}
			}

			// If we found a disk failure, record it and evaluate suggested actions
			if foundFailure {
				if err := c.recordDiskFailureEvent(cr, failureEventName, failureMessage); err != nil {
					// Error already handled in recordDiskFailureEvent
					return cr
				}

				// Look up past events to derive the suggested actions
				rebootEvents, err := c.rebootEventStore.GetRebootEvents(c.ctx, cr.ts.Add(-c.lookbackPeriod))
				if err != nil {
					cr.err = err
					cr.health = apiv1.HealthStateTypeUnhealthy
					log.Logger.Warnw("failed to get reboot events", "error", cr.err)
					return cr
				}

				diskFailureEvents, err := c.eventBucket.Get(c.ctx, cr.ts.Add(-c.lookbackPeriod))
				if err != nil {
					cr.err = err
					cr.health = apiv1.HealthStateTypeUnhealthy
					log.Logger.Warnw("failed to get disk failure events", "error", cr.err)
					return cr
				}

				// Filter disk failure events to only include the hardware-related ones
				var hardwareFailureEvents eventstore.Events
				for _, ev := range diskFailureEvents {
					if ev.Name == eventRAIDArrayFailure ||
						ev.Name == eventFilesystemReadOnly ||
						ev.Name == eventNVMePathFailure ||
						ev.Name == eventNVMeTimeout ||
						ev.Name == eventNVMeDeviceDisabled ||
						ev.Name == eventBeyondEndOfDevice ||
						ev.Name == eventBufferIOError {
						hardwareFailureEvents = append(hardwareFailureEvents, ev)
					}
				}

				// Evaluate the suggested actions (along with the reboot history)
				evaluateSuggestedActions(cr, rebootEvents, hardwareFailureEvents)
			}
		}
	}

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
		cr.health = apiv1.HealthStateTypeHealthy
		cr.reason = "no block device found"
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
