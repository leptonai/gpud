// Package pod tracks the current pods from the kubelet read-only port.
package pod

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/log"
	"github.com/leptonai/gpud/pkg/process"
)

// Name is the ID of the kubernetes pod component.
const Name = "kubelet"

var _ components.Component = &component{}

type component struct {
	ctx    context.Context
	cancel context.CancelFunc

	checkDependencyInstalled func() bool
	kubeletReadOnlyPort      int

	// In case the kubelet does not open the read-only port, we ignore such errors as
	// 'Get "http://localhost:10255/pods": dial tcp 127.0.0.1:10255: connect: connection refused'.
	ignoreConnectionErrors bool

	lastMu   sync.RWMutex
	lastData *Data
}

func New(ctx context.Context, kubeletReadOnlyPort int, ignoreConnectionErrors bool) components.Component {
	cctx, cancel := context.WithCancel(ctx)
	c := &component{
		ctx:                      cctx,
		cancel:                   cancel,
		checkDependencyInstalled: checkKubeletInstalled,
		kubeletReadOnlyPort:      kubeletReadOnlyPort,
		ignoreConnectionErrors:   ignoreConnectionErrors,
	}
	return c
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
	return lastData.getStates(c.ignoreConnectionErrors)
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

	// assume "kubelet" is not installed, thus not needed to check its activeness
	if c.checkDependencyInstalled == nil || !c.checkDependencyInstalled() {
		return
	}

	// below are the checks in case "kubelet" is installed, thus requires activeness checks
	cctx, ccancel := context.WithTimeout(c.ctx, 15*time.Second)
	running := checkKubeletReadOnlyPortListening(cctx, c.kubeletReadOnlyPort)
	ccancel()
	if !running {
		d.err = errors.New("kubelet is installed but kubelet is not running")
		return
	}

	cctx, ccancel = context.WithTimeout(c.ctx, 15*time.Second)
	d.KubeletPidFound = process.CheckRunningByPid(cctx, "kubelet")
	ccancel()

	cctx, ccancel = context.WithTimeout(c.ctx, 30*time.Second)
	d.NodeName, d.Pods, d.err = listPodsFromKubeletReadOnlyPort(cctx, c.kubeletReadOnlyPort)
	ccancel()

	d.connErr = isConnectionRefusedError(d.err)
}

type Data struct {
	// KubeletPidFound is true if the kubelet pid is found.
	KubeletPidFound bool `json:"kubelet_pid_found"`
	// NodeName is the name of the node.
	NodeName string `json:"node_name,omitempty"`
	// Pods is the list of pods on the node.
	Pods []PodStatus `json:"pods,omitempty"`

	// timestamp of the last check
	ts time.Time `json:"-"`
	// error from the last check
	err error `json:"-"`
	// set to true if the error is the connection error to kubelet
	connErr bool `json:"-"`
}

func (d *Data) getReason() string {
	if d == nil || len(d.Pods) == 0 {
		return "no pod found or kubelet is not running"
	}

	if d.err != nil {
		if d.connErr {
			// e.g.,
			// Get "http://localhost:10255/pods": dial tcp [::1]:10255: connect: connection refused
			return fmt.Sprintf("connection error to node %q -- %v", d.NodeName, d.err)
		}

		return fmt.Sprintf("failed to list pods from kubelet read-only port -- %v", d.err)
	}

	return fmt.Sprintf("total %d pods (node %s)", len(d.Pods), d.NodeName)
}

func (d *Data) getHealth(ignoreConnErr bool) (string, bool) {
	healthy := d == nil || d.err == nil
	if d != nil && d.err != nil && d.connErr && ignoreConnErr {
		healthy = true
	}
	health := components.StateHealthy
	if !healthy {
		health = components.StateUnhealthy
	}
	return health, healthy
}

func (d *Data) getStates(ignoreConnErr bool) ([]components.State, error) {
	state := components.State{
		Name:   Name,
		Reason: d.getReason(),
	}
	state.Health, state.Healthy = d.getHealth(ignoreConnErr)

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
