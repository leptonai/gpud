// Package pci tracks the PCI devices and their Access Control Services (ACS) status.
package pci

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/dustin/go-humanize"
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

	currentVirtEnv    pkghost.VirtualizationEnvironment
	getPCIDevicesFunc func(ctx context.Context) (pci.Devices, error)

	eventBucket eventstore.Bucket

	lastMu   sync.RWMutex
	lastData *Data
}

func New(ctx context.Context, eventStore eventstore.Store) (components.Component, error) {
	eventBucket, err := eventStore.Bucket(Name)
	if err != nil {
		return nil, err
	}

	cctx, ccancel := context.WithCancel(ctx)
	return &component{
		ctx:    cctx,
		cancel: ccancel,

		currentVirtEnv:    pkghost.VirtualizationEnv(),
		getPCIDevicesFunc: pci.List,

		eventBucket: eventBucket,
	}, nil
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

func (c *component) HealthStates(ctx context.Context) (apiv1.HealthStates, error) {
	c.lastMu.RLock()
	lastData := c.lastData
	c.lastMu.RUnlock()
	return lastData.getHealthStates()
}

func (c *component) Events(ctx context.Context, since time.Time) (apiv1.Events, error) {
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

// CheckOnce checks the current pods
// run this periodically
func (c *component) CheckOnce() {
	log.Logger.Infow("checking pci")
	d := Data{
		ts: time.Now().UTC(),
	}
	defer func() {
		c.lastMu.Lock()
		c.lastData = &d
		c.lastMu.Unlock()
	}()

	// Virtual machines
	// Virtual machines require ACS to function, hence disabling ACS is not an option.
	//
	// ref. https://docs.nvidia.com/deeplearning/nccl/user-guide/docs/troubleshooting.html#pci-access-control-services-acs
	if c.currentVirtEnv.IsKVM {
		return
	}
	// unknown virtualization environment
	if c.currentVirtEnv.Type == "" {
		return
	}

	lastEvent, err := c.eventBucket.Latest(c.ctx)
	if err != nil {
		d.err = err
		d.healthy = false
		d.reason = fmt.Sprintf("error getting latest event: %s", err)
		return
	}

	nowUTC := time.Now().UTC()
	if lastEvent != nil && nowUTC.Sub(lastEvent.Time.Time) < 24*time.Hour {
		log.Logger.Debugw("found events thus skipping -- we only check once per day", "since", humanize.Time(nowUTC))
		return
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
	d.Devices, d.err = c.getPCIDevicesFunc(cctx)
	cancel()
	if d.err != nil {
		d.healthy = false
		d.reason = fmt.Sprintf("error listing devices: %s", d.err)
		return
	}

	acsEnabledDevices := findACSEnabledDeviceUUIDs(d.Devices)
	if len(acsEnabledDevices) == 0 {
		d.healthy = true
		d.reason = "no acs enabled devices found"
		return
	}

	// no need to check duplicates
	// since we check once above

	cctx, cancel = context.WithTimeout(c.ctx, 15*time.Second)
	d.err = c.eventBucket.Insert(cctx, apiv1.Event{
		Time:    metav1.Time{Time: nowUTC},
		Name:    "acs_enabled",
		Type:    apiv1.EventTypeWarning,
		Message: fmt.Sprintf("host virt env is %q, ACS is enabled on the following PCI devices: %s", pkghost.VirtualizationEnv().Type, strings.Join(acsEnabledDevices, ", ")),
	})
	cancel()
	if d.err != nil {
		d.healthy = false
		d.reason = fmt.Sprintf("error creating event: %s", d.err)
		return
	}

	d.healthy = true
	d.reason = fmt.Sprintf("found %d acs enabled devices (out of %d total)", len(acsEnabledDevices), len(d.Devices))
}

type Data struct {
	Devices []pci.Device `json:"devices,omitempty"`

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

func (d *Data) getHealthStates() (apiv1.HealthStates, error) {
	if d == nil {
		return []apiv1.HealthState{
			{
				Name:   Name,
				Health: apiv1.StateTypeHealthy,
				Reason: "no data yet",
			},
		}, nil
	}

	state := apiv1.HealthState{
		Name:   Name,
		Reason: d.reason,
		Error:  d.getError(),

		Health: apiv1.StateTypeHealthy,
	}
	if !d.healthy {
		state.Health = apiv1.StateTypeUnhealthy
	}

	b, _ := json.Marshal(d)
	state.DeprecatedExtraInfo = map[string]string{
		"data":     string(b),
		"encoding": "json",
	}
	return []apiv1.HealthState{state}, nil
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
