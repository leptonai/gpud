// Package pod tracks the current pods from the kubelet read-only port.
package pod

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/log"
	"github.com/leptonai/gpud/pkg/netutil"
)

const (
	// Name is the ID of the kubernetes pod component.
	Name = "kubelet"

	defaultFailedCountThreshold = 5
)

var _ components.Component = &component{}

type component struct {
	ctx    context.Context
	cancel context.CancelFunc

	checkDependencyInstalled func() bool
	checkKubeletRunning      func() bool
	kubeletReadOnlyPort      int

	failedCount          int
	failedCountThreshold int

	lastMu   sync.RWMutex
	lastData *Data
}

func New(ctx context.Context, kubeletReadOnlyPort int) components.Component {
	cctx, cancel := context.WithCancel(ctx)
	return &component{
		ctx:                      cctx,
		cancel:                   cancel,
		checkDependencyInstalled: checkKubeletInstalled,
		checkKubeletRunning: func() bool {
			return netutil.IsPortOpen(kubeletReadOnlyPort)
		},
		kubeletReadOnlyPort:  kubeletReadOnlyPort,
		failedCount:          0,
		failedCountThreshold: defaultFailedCountThreshold,
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

func (c *component) HealthStates(ctx context.Context) (apiv1.HealthStates, error) {
	c.lastMu.RLock()
	lastData := c.lastData
	c.lastMu.RUnlock()
	return lastData.getHealthStates()
}

func (c *component) Events(ctx context.Context, since time.Time) (apiv1.Events, error) {
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
	log.Logger.Infow("checking kubelet pods")
	d := Data{
		ts: time.Now().UTC(),
	}
	defer func() {
		c.lastMu.Lock()
		c.lastData = &d
		c.lastMu.Unlock()
	}()

	if c.checkDependencyInstalled == nil || !c.checkDependencyInstalled() {
		// "kubelet" is not installed, thus not needed to check its activeness
		d.healthy = true
		d.reason = "kubelet is not installed"
		return
	}

	if c.checkKubeletRunning == nil || !c.checkKubeletRunning() {
		// "kubelet" is not running, thus not needed to check its activeness
		d.healthy = true
		d.reason = "kubelet is installed but not running"
		return
	}

	// below are the checks in case "kubelet" is installed and running, thus requires activeness checks
	cctx, ccancel := context.WithTimeout(c.ctx, 30*time.Second)
	d.NodeName, d.Pods, d.err = listPodsFromKubeletReadOnlyPort(cctx, c.kubeletReadOnlyPort)
	ccancel()

	if d.err != nil {
		c.failedCount++
	} else {
		c.failedCount = 0
	}

	d.healthy = d.err == nil
	if d.err == nil {
		d.reason = fmt.Sprintf("total %d pods (node %s)", len(d.Pods), d.NodeName)
	}

	if c.failedCount >= c.failedCountThreshold {
		d.healthy = false
		d.reason = fmt.Sprintf("list pods from kubelet read-only port failed %d time(s)", c.failedCount)
	}
}

type Data struct {
	// KubeletServiceActive is true if the kubelet service is active.
	KubeletServiceActive bool `json:"kubelet_service_active"`
	// NodeName is the name of the node.
	NodeName string `json:"node_name,omitempty"`
	// Pods is the list of pods on the node.
	Pods []PodStatus `json:"pods,omitempty"`

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
				Name:              Name,
				Health:            apiv1.StateTypeHealthy,
				DeprecatedHealthy: true,
				Reason:            "no data yet",
			},
		}, nil
	}

	state := apiv1.HealthState{
		Name:   Name,
		Reason: d.reason,
		Error:  d.getError(),

		DeprecatedHealthy: d.healthy,
		Health:            apiv1.StateTypeHealthy,
	}
	if !d.healthy {
		state.Health = apiv1.StateTypeUnhealthy
	}

	if len(d.Pods) == 0 { // no pod found yet
		return []apiv1.HealthState{state}, nil
	}

	b, _ := json.Marshal(d)
	state.DeprecatedExtraInfo = map[string]string{
		"data":     string(b),
		"encoding": "json",
	}
	return []apiv1.HealthState{state}, nil
}
