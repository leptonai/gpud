// Package pod tracks the current pods from the kubelet read-only port.
package pod

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

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

	// In case the kubelet does not open the read-only port, we ignore such errors as
	// 'Get "http://localhost:10255/pods": dial tcp 127.0.0.1:10255: connect: connection refused'.
	ignoreConnectionErrors bool

	failedCount          int
	failedCountThreshold int

	lastMu      sync.RWMutex
	lastData    *Data
	lastHealthy bool
	lastReason  string
}

func New(ctx context.Context, kubeletReadOnlyPort int, ignoreConnectionErrors bool) components.Component {
	cctx, cancel := context.WithCancel(ctx)
	return &component{
		ctx:                      cctx,
		cancel:                   cancel,
		checkDependencyInstalled: checkKubeletInstalled,
		checkKubeletRunning: func() bool {
			return netutil.IsPortOpen(kubeletReadOnlyPort)
		},
		kubeletReadOnlyPort:    kubeletReadOnlyPort,
		ignoreConnectionErrors: ignoreConnectionErrors,
		failedCount:            0,
		failedCountThreshold:   defaultFailedCountThreshold,
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

func (c *component) States(ctx context.Context) ([]components.State, error) {
	c.lastMu.RLock()
	lastData := c.lastData
	c.lastMu.RUnlock()
	return lastData.getStates(c.lastReason, c.lastHealthy)
}

func (c *component) Events(ctx context.Context, since time.Time) ([]components.Event, error) {
	return nil, nil
}

func (c *component) Metrics(ctx context.Context, since time.Time) ([]components.Metric, error) {
	log.Logger.Debugw("querying metrics", "since", since)

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
		c.lastHealthy = true
		c.lastReason = "kubelet is not installed"
		return
	}

	if c.checkKubeletRunning == nil || !c.checkKubeletRunning() {
		// "kubelet" is not running, thus not needed to check its activeness
		c.lastHealthy = true
		c.lastReason = "kubelet is installed but not running"
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

	c.lastHealthy = d.err == nil
	if d.err == nil {
		c.lastReason = fmt.Sprintf("total %d pods (node %s)", len(d.Pods), d.NodeName)
	}

	if isConnectionRefusedError(d.err) && c.ignoreConnectionErrors {
		// e.g.,
		// Get "http://localhost:10255/pods": dial tcp [::1]:10255: connect: connection refused
		c.lastHealthy = true
		c.lastReason = "connection error but ignored"
	} else if c.failedCount >= c.failedCountThreshold {
		c.lastHealthy = false
		c.lastReason = fmt.Sprintf("list pods from kubelet read-only port failed %d time(s)", c.failedCount)
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
	ts time.Time `json:"-"`
	// error from the last check
	err error `json:"-"`
}

func (d *Data) getError() string {
	if d == nil || d.err == nil {
		return ""
	}
	return d.err.Error()
}

func (d *Data) getStates(lastReason string, lastHealthy bool) ([]components.State, error) {
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
		Reason: lastReason,
		Error:  d.getError(),

		Healthy: lastHealthy,
		Health:  components.StateHealthy,
	}
	if !lastHealthy {
		state.Health = components.StateUnhealthy
	}

	if len(d.Pods) == 0 { // no pod found yet
		return []components.State{state}, nil
	}

	b, _ := json.Marshal(d)
	state.ExtraInfo = map[string]string{
		"data":     string(b),
		"encoding": "json",
	}
	return []components.State{state}, nil
}
