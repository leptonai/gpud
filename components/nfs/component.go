// Package nfs writes to and reads from the specified NFS mount points.
package nfs

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/olekukonko/tablewriter"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/disk"
	"github.com/leptonai/gpud/pkg/log"
	pkgnfschecker "github.com/leptonai/gpud/pkg/nfs-checker"
)

// Name is the name of the NFS component.
const Name = "nfs"

var _ components.Component = &component{}

type component struct {
	ctx    context.Context
	cancel context.CancelFunc

	machineID string

	getGroupConfigsFunc func() pkgnfschecker.Configs
	findMntTargetDevice func(dir string) (string, string, error)
	isNFSFSType         func(fsType string) bool

	lastMu          sync.RWMutex
	lastCheckResult *checkResult
}

func New(gpudInstance *components.GPUdInstance) (components.Component, error) {
	cctx, ccancel := context.WithCancel(gpudInstance.RootCtx)
	c := &component{
		ctx:    cctx,
		cancel: ccancel,

		machineID: gpudInstance.MachineID,

		getGroupConfigsFunc: GetDefaultConfigs,
		findMntTargetDevice: disk.FindMntTargetDevice,
		isNFSFSType:         disk.DefaultNFSFsTypeFunc,
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
			select {
			case <-c.ctx.Done():
				return
			case <-ticker.C:
			}

			_ = c.Check()
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
	return nil, nil
}

func (c *component) Close() error {
	log.Logger.Debugw("closing component")

	c.cancel()

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

	groupConfigs := c.getGroupConfigsFunc()
	if len(groupConfigs) == 0 {
		cr.health = apiv1.HealthStateTypeHealthy
		cr.reason = "no nfs group configs found"
		log.Logger.Debugw(cr.reason)
		return cr
	}

	// verify the volume path is an nfs mount point
	for _, groupConfig := range groupConfigs {
		dev, fsType, err := c.findMntTargetDevice(groupConfig.VolumePath)
		if dev == "" || err != nil {
			cr.err = err
			cr.health = apiv1.HealthStateTypeDegraded
			cr.reason = "failed to find mount target device for " + groupConfig.VolumePath
			log.Logger.Warnw(cr.reason, "error", err)
			return cr
		}
		if !c.isNFSFSType(fsType) {
			cr.health = apiv1.HealthStateTypeDegraded
			cr.reason = fmt.Sprintf("volume path %s mounted on %s, but not nfs", groupConfig.VolumePath, dev)
			log.Logger.Warnw(cr.reason)
			return cr
		}
		log.Logger.Infow("nfs mount point found", "volume_path", groupConfig.VolumePath, "device", dev, "fs_type", fsType)
	}

	memberConfigs := groupConfigs.GetMemberConfigs(c.machineID)
	if err := memberConfigs.Validate(); err != nil {
		cr.err = err
		cr.health = apiv1.HealthStateTypeDegraded
		cr.reason = "invalid nfs group configs"
		log.Logger.Warnw(cr.reason, "error", err)
		return cr
	}

	msg := make([]string, 0, len(memberConfigs))
	for _, memberConfig := range memberConfigs {
		checker, err := pkgnfschecker.NewChecker(&memberConfig)
		if err != nil {
			cr.err = err
			cr.health = apiv1.HealthStateTypeDegraded
			cr.reason = "failed to create nfs checker for " + memberConfig.VolumePath
			log.Logger.Warnw(cr.reason, "error", err)
			return cr
		}

		if err := checker.Write(); err != nil {
			cr.err = err
			cr.health = apiv1.HealthStateTypeDegraded
			cr.reason = "failed to write to nfs checker for " + memberConfig.VolumePath
			log.Logger.Warnw(cr.reason, "error", err)
			return cr
		}

		nfsResult := checker.Check()
		if len(nfsResult.Error) > 0 {
			cr.err = errors.New(nfsResult.Error)
			cr.health = apiv1.HealthStateTypeDegraded
			cr.reason = "failed to check nfs checker for " + memberConfig.VolumePath
			log.Logger.Warnw(cr.reason, "error", err)
			return cr
		}

		if err := checker.Clean(); err != nil {
			cr.err = err
			cr.health = apiv1.HealthStateTypeDegraded
			cr.reason = "failed to clean nfs checker for " + memberConfig.VolumePath
			log.Logger.Warnw(cr.reason, "error", err)
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
		Time:      metav1.NewTime(cr.ts),
		Component: Name,
		Name:      Name,
		Reason:    cr.reason,
		Error:     cr.getError(),
		Health:    cr.health,
	}

	return apiv1.HealthStates{state}
}
