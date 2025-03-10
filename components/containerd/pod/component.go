// Package pod tracks the current pods from the containerd CRI.
package pod

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/leptonai/gpud/components"
	containerd_pod_id "github.com/leptonai/gpud/components/containerd/pod/id"
	components_metrics "github.com/leptonai/gpud/pkg/gpud-metrics"
	"github.com/leptonai/gpud/pkg/log"
)

type component struct {
	ctx    context.Context
	cancel context.CancelFunc

	endpoint string

	lastMu   sync.RWMutex
	lastData Data
}

func New(ctx context.Context) components.Component {
	cctx, cancel := context.WithCancel(ctx)
	c := &component{
		ctx:      cctx,
		cancel:   cancel,
		endpoint: defaultContainerRuntimeEndpoint,
	}
	return c
}

var _ components.Component = &component{}

func (c *component) Name() string { return containerd_pod_id.Name }

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
	log.Logger.Infow("checking containerd pods", "endpoint", c.endpoint)
	d := Data{
		ts: time.Now().UTC(),
	}

	cctx, ccancel := context.WithTimeout(c.ctx, 30*time.Second)
	d.Pods, d.err = listSandboxStatus(cctx, c.endpoint)
	ccancel()

	if d.err != nil {
		components_metrics.SetGetFailed(containerd_pod_id.Name)
	} else {
		components_metrics.SetGetSuccess(containerd_pod_id.Name)
	}

	c.lastMu.Lock()
	c.lastData = d
	c.lastMu.Unlock()
}

type Data struct {
	// Pods is the list of pods on the node.
	Pods []PodSandbox `json:"pods,omitempty"`

	// timestamp of the last check
	ts time.Time `json:"-"`
	// error from the last check
	err error `json:"-"`
}

func (d *Data) Reason() string {
	reason := fmt.Sprintf("total %d pod sandboxe(s)", len(d.Pods))

	if d.err != nil {
		// this is the error from "ListSandboxStatus"
		//
		// e.g.,
		// rpc error: code = Unimplemented desc = unknown service runtime.v1.RuntimeService
		reason = "failed gRPC call to the containerd socket"
		st, ok := status.FromError(d.err)
		if ok {
			if st.Code() == codes.Unimplemented {
				reason += "; containerd didn't enable CRI"
			} else {
				reason += fmt.Sprintf("; %s", st.Message())
			}
		}
	}

	return reason
}

func (d *Data) getHealth() (string, bool) {
	if d.err != nil {
		return components.StateUnhealthy, false
	}
	return components.StateHealthy, true
}

func (d *Data) getStates() ([]components.State, error) {
	state := components.State{
		Name:   containerd_pod_id.Name,
		Reason: d.Reason(),
	}
	state.Health, state.Healthy = d.getHealth()

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
