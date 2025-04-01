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

	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/common"
	"github.com/leptonai/gpud/pkg/eventstore"
	pkghost "github.com/leptonai/gpud/pkg/host"
	"github.com/leptonai/gpud/pkg/log"
	"github.com/leptonai/gpud/pkg/pci"
)

// Name is the name of the PCI ID component.
const Name = "pci"

var _ components.Component = &component{}

type component struct {
	ctx         context.Context
	cancel      context.CancelFunc
	eventBucket eventstore.Bucket

	currentVirtEnv pkghost.VirtualizationEnvironment
	listFunc       func(ctx context.Context) (pci.Devices, error)

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
		ctx:            cctx,
		cancel:         ccancel,
		eventBucket:    eventBucket,
		currentVirtEnv: pkghost.VirtualizationEnv(),
		listFunc:       pci.List,
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

func (c *component) States(ctx context.Context) ([]components.State, error) {
	c.lastMu.RLock()
	lastData := c.lastData
	c.lastMu.RUnlock()
	return lastData.getStates()
}

func (c *component) Events(ctx context.Context, since time.Time) ([]components.Event, error) {
	return c.eventBucket.Get(ctx, since)
}

func (c *component) Metrics(ctx context.Context, since time.Time) ([]components.Metric, error) {
	log.Logger.Debugw("querying metrics", "since", since)

	return nil, nil
}

func (c *component) Close() error {
	log.Logger.Debugw("closing component")

	c.cancel()
	c.eventBucket.Close()

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
	d.Devices, err = c.listFunc(cctx)
	cancel()
	if err != nil {
		d.err = err
		return
	}

	ev := createEvent(nowUTC, d.Devices)
	if ev == nil {
		return
	}

	// no need to check duplicates
	// since we check once above

	cctx, cancel = context.WithTimeout(c.ctx, 15*time.Second)
	err = c.eventBucket.Insert(cctx, *ev)
	cancel()
	if err != nil {
		d.err = err
		return
	}
}

type Data struct {
	Devices []pci.Device `json:"devices,omitempty"`

	// timestamp of the last check
	ts time.Time `json:"-"`
	// error from the last check
	err error `json:"-"`
}

func (d *Data) getReason() string {
	if d == nil {
		return "no pci data"
	}
	if d.err != nil {
		return fmt.Sprintf("failed to get pci data -- %s", d.err)
	}

	return fmt.Sprintf("found %d devices", len(d.Devices))
}

func (d *Data) getHealth() (string, bool) {
	healthy := d == nil || d.err == nil
	health := components.StateHealthy
	if !healthy {
		health = components.StateUnhealthy
	}

	return health, healthy
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
		Reason: d.getReason(),
		Error:  d.getError(),
	}
	state.Health, state.Healthy = d.getHealth()

	b, _ := json.Marshal(d)
	state.ExtraInfo = map[string]string{
		"data":     string(b),
		"encoding": "json",
	}
	return []components.State{state}, nil
}

func createEvent(time time.Time, devices []pci.Device) *components.Event {
	uuids := make([]string, 0)
	for _, dev := range devices {
		// check whether ACS is enabled on PCI bridges
		if dev.AccessControlService != nil  && dev.AccessControlService.ACSCtl.SrcValid {
			uuids = append(uuids, dev.ID)
		}
	}

	if len(uuids) == 0 {
		return nil
	}

	return &components.Event{
		Time:    metav1.Time{Time: time.UTC()},
		Name:    "acs_enabled",
		Type:    common.EventTypeWarning,
		Message: fmt.Sprintf("host virt env is %q, ACS is enabled on the following PCI devices: %s", pkghost.VirtualizationEnv().Type, strings.Join(uuids, ", ")),
	}
}
