// Package latency tracks the global network connectivity statistics.
package latency

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/log"
	pkgmetrics "github.com/leptonai/gpud/pkg/metrics"
	"github.com/leptonai/gpud/pkg/netutil/latency"
	latency_edge "github.com/leptonai/gpud/pkg/netutil/latency/edge"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	// Name is the ID of the network latency component.
	Name = "network-latency"

	// 1 second
	MinGlobalMillisecondThreshold = 1000
	// 7 seconds by default to reach any of the DERP servers.
	DefaultGlobalMillisecondThreshold = 7000
)

var _ components.Component = &component{}

type component struct {
	ctx    context.Context
	cancel context.CancelFunc

	// GlobalMillisecondThreshold is the global threshold in milliseconds for the DERP latency.
	// If all DERP latencies are greater than this threshold, the component will be marked as failed.
	// If at least one DERP latency is less than this threshold, the component will be marked as healthy.
	globalMillisecondThreshold int64

	lastMu   sync.RWMutex
	lastData *Data
}

func New(ctx context.Context) components.Component {
	cctx, ccancel := context.WithCancel(ctx)
	return &component{
		ctx:    cctx,
		cancel: ccancel,

		globalMillisecondThreshold: DefaultGlobalMillisecondThreshold,
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
	return lastData.getStates(c.globalMillisecondThreshold)
}

func (c *component) Events(ctx context.Context, since time.Time) ([]components.Event, error) {
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
	log.Logger.Infow("checking disk")
	d := Data{
		ts: time.Now().UTC(),
	}
	defer func() {
		c.lastMu.Lock()
		c.lastData = &d
		c.lastMu.Unlock()
	}()

	cctx, ccancel := context.WithTimeout(c.ctx, 30*time.Second)
	defer ccancel()

	var err error
	d.EgressLatencies, err = latency_edge.Measure(cctx)
	if err != nil {
		d.err = err
		return
	}

	for _, lat := range d.EgressLatencies {
		region := fmt.Sprintf("%s (%s)", lat.RegionName, lat.Provider)
		edgeInMilliseconds.With(prometheus.Labels{
			pkgmetrics.MetricLabelKey: region,
		}).Set(float64(lat.LatencyMilliseconds))
	}
}

type Data struct {
	// EgressLatencies is the list of egress latencies to global edge servers.
	EgressLatencies latency.Latencies `json:"egress_latencies"`

	// timestamp of the last check
	ts time.Time
	// error from the last check
	err error
}

func (d *Data) getReason(globalMillisecondThreshold int64) string {
	if d == nil {
		return "no network latency data"
	}
	if d.err != nil {
		return fmt.Sprintf("failed to get network latency data -- %s", d.err)
	}

	reasons := []string{}
	if globalMillisecondThreshold > 0 {
		for _, latency := range d.EgressLatencies {
			reasons = append(reasons, fmt.Sprintf("latency to %s edge derp server (%s) is %dms", latency.RegionName, latency.Latency, latency.LatencyMilliseconds))
		}
	}
	if len(reasons) == 0 {
		return "no issue"
	}

	return strings.Join(reasons, "; ")
}

func (d *Data) getHealth(globalMillisecondThreshold int64) (string, bool) {
	healthy := d == nil || d.err == nil
	health := components.StateHealthy
	if !healthy {
		health = components.StateUnhealthy
	}

	if globalMillisecondThreshold > 0 && d != nil && len(d.EgressLatencies) > 0 {
		for _, latency := range d.EgressLatencies {
			if latency.LatencyMilliseconds > globalMillisecondThreshold {
				healthy = false
				health = components.StateUnhealthy
				break
			}
		}
	}

	return health, healthy
}

func (d *Data) getError() string {
	if d == nil || d.err == nil {
		return ""
	}
	return d.err.Error()
}

func (d *Data) getStates(globalMillisecondThreshold int64) ([]components.State, error) {
	state := components.State{
		Name:   Name,
		Reason: d.getReason(globalMillisecondThreshold),
		Error:  d.getError(),
	}
	state.Health, state.Healthy = d.getHealth(globalMillisecondThreshold)

	b, _ := json.Marshal(d)
	state.ExtraInfo = map[string]string{
		"data":     string(b),
		"encoding": "json",
	}
	return []components.State{state}, nil
}
