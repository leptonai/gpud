// Package pci tracks the PCI devices and their Access Control Services (ACS) status.
package pci

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/olekukonko/tablewriter"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/eventstore"
	pkghost "github.com/leptonai/gpud/pkg/host"
	"github.com/leptonai/gpud/pkg/log"
	"github.com/leptonai/gpud/pkg/pci"
)

// Name is the name of the PCI ID component.
const Name = "pci"

var _ components.Component = &component{}

type component struct {
	ctx    context.Context
	cancel context.CancelFunc

	currentVirtEnv                pkghost.VirtualizationEnvironment
	getPCIDevicesFunc             func(ctx context.Context) (pci.Devices, error)
	findACSEnabledDeviceUUIDsFunc func(devs []pci.Device) []string

	eventBucket eventstore.Bucket

	lastMu          sync.RWMutex
	lastCheckResult *checkResult
}

func New(gpudInstance *components.GPUdInstance) (components.Component, error) {
	cctx, ccancel := context.WithCancel(gpudInstance.RootCtx)
	c := &component{
		ctx:    cctx,
		cancel: ccancel,

		currentVirtEnv:                pkghost.VirtualizationEnv(),
		getPCIDevicesFunc:             pci.List,
		findACSEnabledDeviceUUIDsFunc: findACSEnabledDeviceUUIDs,
	}

	if gpudInstance.EventStore != nil && runtime.GOOS == "linux" {
		var err error
		c.eventBucket, err = gpudInstance.EventStore.Bucket(Name)
		if err != nil {
			ccancel()
			return nil, err
		}
	}

	return c, nil
}

func (c *component) Name() string { return Name }

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

			if c.eventBucket != nil {
				lastEvent, err := c.eventBucket.Latest(c.ctx)
				if err != nil {
					log.Logger.Errorw("error getting latest event", "error", err)
					continue
				}

				nowUTC := time.Now().UTC()
				if lastEvent != nil && nowUTC.Sub(lastEvent.Time.Time) < 24*time.Hour {
					log.Logger.Debugw("found events thus skipping to not overwrite latest data -- we only check once per day", "since", humanize.Time(nowUTC))
					continue
				}
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
	if c.eventBucket == nil {
		return nil, nil
	}
	return c.eventBucket.Get(ctx, since)
}

func (c *component) Close() error {
	log.Logger.Debugw("closing component")

	c.cancel()

	if c.eventBucket != nil {
		c.eventBucket.Close()
	}

	return nil
}

func (c *component) Check() components.CheckResult {
	log.Logger.Infow("checking pci")

	cr := &checkResult{
		ts: time.Now().UTC(),
	}

	defer func() {
		c.lastMu.Lock()
		c.lastCheckResult = cr
		c.lastMu.Unlock()
	}()

	// Virtual machines
	// Virtual machines require ACS to function, hence disabling ACS is not an option.
	//
	// ref. https://docs.nvidia.com/deeplearning/nccl/user-guide/docs/troubleshooting.html#pci-access-control-services-acs
	if c.currentVirtEnv.IsKVM {
		cr.health = apiv1.HealthStateTypeHealthy
		cr.reason = "host virt env is KVM (no need to check ACS)"
		return cr
	}

	// unknown virtualization environment
	if c.currentVirtEnv.Type == "" {
		cr.health = apiv1.HealthStateTypeHealthy
		cr.reason = "unknown virtualization environment (no need to check ACS)"
		return cr
	}

	// in linux, and not in VM
	// then, check all ACS enabled devices
	//
	// Baremetal systems
	// IO virtualization (also known as VT-d or IOMMU) can interfere with GPU Direct by redirecting all
	// PCI point-to-point traffic to the CPU root complex, causing a significant performance reduction or even a hang.
	// If PCI switches have ACS enabled, it needs to be disabled.
	//
	// Virtual machines
	// Virtual machines require ACS to function, hence disabling ACS is not an option.
	//
	// ref. https://docs.nvidia.com/deeplearning/nccl/user-guide/docs/troubleshooting.html#pci-access-control-services-acs
	cctx, cancel := context.WithTimeout(c.ctx, 15*time.Second)
	cr.Devices, cr.err = c.getPCIDevicesFunc(cctx)
	cancel()
	if cr.err != nil {
		cr.health = apiv1.HealthStateTypeUnhealthy
		cr.reason = "error listing devices"
		log.Logger.Errorw(cr.reason, "error", cr.err)
		return cr
	}

	acsEnabledDevices := c.findACSEnabledDeviceUUIDsFunc(cr.Devices)
	if len(acsEnabledDevices) == 0 {
		cr.health = apiv1.HealthStateTypeHealthy
		cr.reason = "non-KVM host env, no acs enabled device found (no need to disable)"
		return cr
	}

	if c.eventBucket != nil {
		cctx, cancel = context.WithTimeout(c.ctx, 15*time.Second)
		cr.err = c.eventBucket.Insert(cctx, apiv1.Event{
			Time:    metav1.Time{Time: time.Now().UTC()},
			Name:    "acs_enabled",
			Type:    apiv1.EventTypeWarning,
			Message: fmt.Sprintf("host virt env is %q, ACS is enabled on the following PCI devices: %s (needs to be disabled)", c.currentVirtEnv.Type, strings.Join(acsEnabledDevices, ", ")),
		})
		cancel()
		if cr.err != nil {
			cr.health = apiv1.HealthStateTypeUnhealthy
			cr.reason = "error creating event"
			log.Logger.Errorw(cr.reason, "error", cr.err)
			return cr
		}
	}

	cr.health = apiv1.HealthStateTypeHealthy
	cr.reason = "found some acs enabled devices (needs to be disabled)"
	log.Logger.Debugw(cr.reason, "enabledDevices", len(acsEnabledDevices))

	return cr
}

var _ components.CheckResult = &checkResult{}

type checkResult struct {
	Devices []pci.Device `json:"devices,omitempty"`

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
	if cr == nil {
		return ""
	}

	cnt := 0
	buf := bytes.NewBuffer(nil)
	table := tablewriter.NewWriter(buf)
	table.SetAlignment(tablewriter.ALIGN_CENTER)
	table.SetHeader([]string{"Device ID", "Device Name", "ACS Enabled"})
	for _, dev := range cr.Devices {
		acsEnabled := dev.AccessControlService != nil && dev.AccessControlService.ACSCtl.SrcValid
		if acsEnabled {
			cnt++
			table.Append([]string{dev.ID, dev.Name, "yes"})
		}
	}
	table.Render()

	if cnt > 0 {
		return buf.String()
	}

	return "no devices with ACS enabled (ok)"
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
				Time:          metav1.NewTime(time.Now().UTC()),
				Component:     Name,
				ComponentType: apiv1.ComponentTypeComponent,
				Name:          Name,
				Health:        apiv1.HealthStateTypeHealthy,
				Reason:        "no data yet",
			},
		}
	}

	state := apiv1.HealthState{
		Time:          metav1.NewTime(cr.ts),
		Component:     Name,
		ComponentType: apiv1.ComponentTypeComponent,
		Name:          Name,
		Reason:        cr.reason,
		Error:         cr.getError(),
		Health:        cr.health,
	}

	b, _ := json.Marshal(cr)
	state.ExtraInfo = map[string]string{
		"data":     string(b),
		"encoding": "json",
	}
	return apiv1.HealthStates{state}
}

func findACSEnabledDeviceUUIDs(devs []pci.Device) []string {
	uuids := make([]string, 0)
	for _, dev := range devs {
		// check whether ACS is enabled on PCI bridges
		if dev.AccessControlService != nil && dev.AccessControlService.ACSCtl.SrcValid {
			uuids = append(uuids, dev.ID)
		}
	}
	if len(uuids) == 0 {
		return nil
	}

	return uuids
}
