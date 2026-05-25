// Package nfs writes to and reads from the specified NFS mount points.
package nfs

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/olekukonko/tablewriter"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/disk"
	"github.com/leptonai/gpud/pkg/eventstore"
	pkghost "github.com/leptonai/gpud/pkg/host"
	"github.com/leptonai/gpud/pkg/kmsg"
	"github.com/leptonai/gpud/pkg/log"
	pkgnfschecker "github.com/leptonai/gpud/pkg/nfs-checker"
)

// Name is the name of the NFS component.
const Name = "nfs"

// defaultLookbackPeriod is how far back to query kmsg and reboot history when
// evaluating NFS hang suggested actions. Bounded by eventstore.DefaultRetention
// (3 days) — querying further yields no events (design doc §9 U3).
const defaultLookbackPeriod = 3 * 24 * time.Hour

var _ components.Component = &component{}

type kmsgSyncerCloser interface {
	Close()
}

type component struct {
	ctx    context.Context
	cancel context.CancelFunc

	machineID string

	getGroupConfigsFunc func() pkgnfschecker.Configs
	findMntTargetDevice func(dir string) (string, string, error)
	isNFSFSType         func(fsType string) bool

	// Function fields for testable NFS operations
	validateMemberConfigs func(ctx context.Context, configs pkgnfschecker.MemberConfigs) error
	newChecker            func(ctx context.Context, cfg *pkgnfschecker.MemberConfig) (pkgnfschecker.Checker, error)
	writeChecker          func(ctx context.Context, checker pkgnfschecker.Checker) error
	checkChecker          func(ctx context.Context, checker pkgnfschecker.Checker) pkgnfschecker.CheckResult
	cleanChecker          func(checker pkgnfschecker.Checker) error

	// eventBucket stores NFS-related kmsg events for the kmsg-based hang
	// detection path. Nil when no EventStore is configured (e.g., in unit
	// tests that exercise only the prober path).
	eventBucket eventstore.Bucket
	// kmsgSyncer streams kernel messages into eventBucket; only created when
	// running as root on Linux.
	kmsgSyncer kmsgSyncerCloser
	// rebootEventStore is consulted to decide between REBOOT_SYSTEM and
	// HARDWARE_INSPECTION when an NFS hang is detected. May be nil.
	rebootEventStore pkghost.RebootEventStore
	// lookbackPeriod controls how far back we query events when evaluating
	// suggested actions.
	lookbackPeriod time.Duration

	lastMu          sync.RWMutex
	lastCheckResult *checkResult
}

// New creates the NFS component.
func New(gpudInstance *components.GPUdInstance) (components.Component, error) {
	return newComponent(gpudInstance, runtime.GOOS, os.Geteuid(), kmsg.NewSyncer)
}

// newComponent is the testable constructor. It accepts the runtime OS, the
// effective UID, and a kmsg syncer factory so tests can exercise the
// EventStore wiring without privileged kernel access.
func newComponent(
	gpudInstance *components.GPUdInstance,
	goos string,
	euid int,
	newKmsgSyncerFunc func(ctx context.Context, matchFunc kmsg.MatchFunc, eventBucket eventstore.Bucket, opts ...kmsg.OpOption) (*kmsg.Syncer, error),
) (components.Component, error) {
	if newKmsgSyncerFunc == nil {
		newKmsgSyncerFunc = kmsg.NewSyncer
	}

	cctx, ccancel := context.WithCancel(gpudInstance.RootCtx)
	c := &component{
		ctx:    cctx,
		cancel: ccancel,

		machineID: gpudInstance.MachineID,

		getGroupConfigsFunc: GetDefaultConfigs,
		findMntTargetDevice: disk.FindMntTargetDevice,
		isNFSFSType:         disk.DefaultNFSFsTypeFunc,

		// Initialize NFS operation function fields with real implementations
		validateMemberConfigs: func(ctx context.Context, configs pkgnfschecker.MemberConfigs) error {
			return configs.Validate(ctx)
		},
		newChecker: pkgnfschecker.NewChecker,
		writeChecker: func(ctx context.Context, checker pkgnfschecker.Checker) error {
			return checker.Write(ctx)
		},
		checkChecker: func(ctx context.Context, checker pkgnfschecker.Checker) pkgnfschecker.CheckResult {
			return checker.Check(ctx)
		},
		cleanChecker: func(checker pkgnfschecker.Checker) error {
			return checker.Clean()
		},

		rebootEventStore: gpudInstance.RebootEventStore,
		lookbackPeriod:   defaultLookbackPeriod,
	}

	if gpudInstance.EventStore != nil {
		bucket, err := gpudInstance.EventStore.Bucket(Name)
		if err != nil {
			ccancel()
			return nil, err
		}
		c.eventBucket = bucket

		// Only sync kmsg on Linux as root — /dev/kmsg is privileged.
		if goos == "linux" && euid == 0 {
			syncer, err := newKmsgSyncerFunc(
				cctx,
				Match,
				c.eventBucket,
				// Coalesce repeated kmsg lines (e.g., bursty writeback stacks)
				// within a 5-minute window to keep the event store bounded.
				kmsg.WithCacheKeyTruncateSeconds(300),
			)
			if err != nil {
				ccancel()
				return nil, err
			}
			c.kmsgSyncer = syncer
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
			// Run an initial check immediately so the first health state is
			// available without waiting one tick interval.
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
	log.Logger.Infow("checking nfs")

	cr := &checkResult{
		ts: time.Now().UTC(),
	}

	defer func() {
		c.lastMu.Lock()
		c.lastCheckResult = cr
		c.lastMu.Unlock()
	}()

	// kmsg-based NFS hang detection takes priority over the prober: when the
	// kernel has already reported writeback hangs / lock-reclaim failures /
	// unresponsive servers, a probe-issued open()/write() may itself hang
	// forever, so we surface the suggested action and skip the prober for
	// this tick. If a reboot has already been observed after the hang
	// events, EvaluateSuggestedActions returns nil and we fall through to
	// the prober for live verification (design doc §3.6).
	if c.eventBucket != nil {
		evs, err := c.eventBucket.Get(c.ctx, cr.ts.Add(-c.lookbackPeriod))
		if err != nil {
			log.Logger.Warnw("failed to query nfs event bucket", "error", err)
		} else {
			hangEvents, hangReason := collectNFSHangEvents(evs)
			if len(hangEvents) > 0 {
				var rebootEvents eventstore.Events
				if c.rebootEventStore != nil {
					var err error
					rebootEvents, err = c.rebootEventStore.GetRebootEvents(c.ctx, cr.ts.Add(-c.lookbackPeriod))
					if err != nil {
						cr.err = err
						cr.health = apiv1.HealthStateTypeUnhealthy
						cr.reason = "failed to get reboot events"
						log.Logger.Warnw(cr.reason, "error", cr.err)
						return cr
					}
					// GetRebootEvents returns events in DESCENDING order
					// (latest first), but EvaluateSuggestedActions expects
					// ASCENDING order (oldest first) — see design doc §9 U2.
					sort.Slice(rebootEvents, func(i, j int) bool {
						return rebootEvents[i].Time.Before(rebootEvents[j].Time)
					})
					if len(rebootEvents) > 0 {
						hangEvents, hangReason = collectNFSHangEvents(nfsEventsOnOrAfter(evs, rebootEvents[0].Time))
					}
				}
				if len(hangEvents) > 0 {
					sa := eventstore.EvaluateSuggestedActions(rebootEvents, hangEvents, 2)
					if sa != nil {
						sa.Description = "NFS hang requires immediate reboot; drain may hang on NFS volumes"
						cr.health = apiv1.HealthStateTypeUnhealthy
						cr.reason = hangReason
						cr.suggestedActions = sa
						return cr
					}
				}
				// No unresolved post-reboot hang evidence, or sa == nil
				// because reboot history already resolved the hang; fall
				// through to the prober for live verification.
			}
		}
	}

	groupConfigs := c.getGroupConfigsFunc()
	if len(groupConfigs) == 0 {
		cr.health = apiv1.HealthStateTypeHealthy
		cr.reason = "no nfs group configs found"
		log.Logger.Debugw(cr.reason)
		return cr
	}

	memberConfigs := groupConfigs.GetMemberConfigs(c.machineID)
	timeoutCtx, cancel := context.WithTimeout(c.ctx, 5*time.Second)
	err := c.validateMemberConfigs(timeoutCtx, memberConfigs)
	cancel()
	if err != nil {
		cr.err = err
		cr.health = apiv1.HealthStateTypeDegraded

		if errors.Is(err, context.DeadlineExceeded) {
			cr.reason = "NFS validation timed out - server may be unresponsive"
		} else {
			cr.reason = "invalid nfs group configs"
		}
		log.Logger.Warnw(cr.reason, "health", cr.health, "error", err)
		return cr
	}

	// verify the volume path is an nfs mount point
	for _, groupConfig := range groupConfigs {
		dev, fsType, err := c.findMntTargetDevice(groupConfig.VolumePath)
		if dev == "" || err != nil {
			cr.err = err
			cr.health = apiv1.HealthStateTypeDegraded
			cr.reason = "failed to find mount target device for " + groupConfig.VolumePath
			log.Logger.Warnw(cr.reason, "health", cr.health, "error", err)
			return cr
		}
		if !c.isNFSFSType(fsType) {
			cr.health = apiv1.HealthStateTypeDegraded
			cr.reason = fmt.Sprintf("The user applied path %q as NFS volume, but in fact the file system type %q is not NFS.", groupConfig.VolumePath, fsType)
			log.Logger.Warnw(cr.reason, "health", cr.health)
			return cr
		}
		log.Logger.Infow("nfs mount point found", "volume_path", groupConfig.VolumePath, "device", dev, "fs_type", fsType)
	}

	msg := make([]string, 0, len(memberConfigs))
	for _, memberConfig := range memberConfigs {
		// Create checker with timeout
		var checker pkgnfschecker.Checker
		{
			timeoutCtx, cancel := context.WithTimeout(c.ctx, 5*time.Second)
			checker, err = c.newChecker(timeoutCtx, &memberConfig)
			cancel()
		}
		if err != nil {
			cr.err = err
			cr.health = apiv1.HealthStateTypeDegraded

			if errors.Is(err, context.DeadlineExceeded) {
				cr.reason = "NFS checker creation timed out for " + memberConfig.VolumePath + " - server may be unresponsive"
			} else {
				cr.reason = "failed to create nfs checker for " + memberConfig.VolumePath
			}
			log.Logger.Warnw(cr.reason, "health", cr.health, "error", err)
			return cr
		}

		// Write with timeout
		{
			timeoutCtx, cancel := context.WithTimeout(c.ctx, 5*time.Second)
			err = c.writeChecker(timeoutCtx, checker)
			cancel()
		}
		if err != nil {
			cr.err = err
			cr.health = apiv1.HealthStateTypeDegraded
			if errors.Is(err, context.DeadlineExceeded) {
				cr.reason = "NFS write timed out for " + memberConfig.VolumePath + " - server may be unresponsive"
			} else {
				cr.reason = "failed to write to nfs checker for " + memberConfig.VolumePath
			}
			log.Logger.Warnw(cr.reason, "health", cr.health, "error", err)
			return cr
		}

		// Check with timeout
		var nfsResult pkgnfschecker.CheckResult
		{
			timeoutCtx, cancel := context.WithTimeout(c.ctx, 5*time.Second)
			nfsResult = c.checkChecker(timeoutCtx, checker)
			cancel()
		}
		if len(nfsResult.Error) > 0 {
			cr.err = errors.New(nfsResult.Error)
			cr.health = apiv1.HealthStateTypeDegraded

			if nfsResult.TimeoutError {
				cr.reason = "NFS check timed out for " + memberConfig.VolumePath + " - server may be unresponsive"
			} else {
				cr.reason = "failed to check nfs checker for " + memberConfig.VolumePath
			}
			log.Logger.Warnw(cr.reason, "health", cr.health, "error", cr.err)
			return cr
		}

		if err := c.cleanChecker(checker); err != nil {
			cr.err = err
			cr.health = apiv1.HealthStateTypeDegraded
			cr.reason = "failed to clean nfs checker for " + memberConfig.VolumePath
			log.Logger.Warnw(cr.reason, "health", cr.health, "error", err)
			return cr
		}

		cr.NFSCheckResults = append(cr.NFSCheckResults, nfsResult)
		msg = append(msg, nfsResult.Message)
	}

	cr.health = apiv1.HealthStateTypeHealthy
	cr.reason = strings.Join(msg, ", ")
	log.Logger.Debugw(cr.reason)

	return cr
}

var _ components.CheckResult = &checkResult{}

type checkResult struct {
	NFSCheckResults []pkgnfschecker.CheckResult `json:"nfs_check_results,omitempty"`

	// timestamp of the last check
	ts time.Time
	// error from the last check
	err error

	// tracks the healthy evaluation result of the last check
	health apiv1.HealthStateType
	// suggestedActions is populated when the kmsg-based hang detection
	// determines the system needs a reboot or hardware inspection. It is a
	// pointer so the JSON encoder elides it when unset.
	suggestedActions *apiv1.SuggestedActions
	// tracks the reason of the last check
	reason string
}

func (cr *checkResult) ComponentName() string {
	return Name
}

func (cr *checkResult) String() string {
	if cr == nil {
		return ""
	}

	if len(cr.NFSCheckResults) == 0 {
		return ""
	}

	buf := bytes.NewBuffer(nil)
	table := tablewriter.NewWriter(buf)
	table.SetAlignment(tablewriter.ALIGN_CENTER)
	table.SetHeader([]string{"Directory", "Message"})
	for _, nfsResult := range cr.NFSCheckResults {
		table.Append([]string{nfsResult.Dir, nfsResult.Message})
	}
	table.Render()

	return buf.String()
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
		SuggestedActions: cr.suggestedActions,
	}

	return apiv1.HealthStates{state}
}
