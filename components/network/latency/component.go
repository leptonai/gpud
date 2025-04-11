// Package latency tracks the global network connectivity statistics.
package latency

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	apiv1 "github.com/leptonai/gpud/api/v1"
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

var _ apiv1.Component = &component{}

type component struct {
	ctx    context.Context
	cancel context.CancelFunc

	getEgressLatenciesFunc func(context.Context, ...latency_edge.OpOption) (latency.Latencies, error)

	// GlobalMillisecondThreshold is the global threshold in milliseconds for the DERP latency.
	// If all DERP latencies are greater than this threshold, the component will be marked as failed.
	// If at least one DERP latency is less than this threshold, the component will be marked as healthy.
	globalMillisecondThreshold int64

	lastMu   sync.RWMutex
	lastData *Data
}

func New(ctx context.Context) apiv1.Component {
	cctx, ccancel := context.WithCancel(ctx)
	return &component{
		ctx:    cctx,
		cancel: ccancel,

		getEgressLatenciesFunc: latency_edge.Measure,

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

func (c *component) States(ctx context.Context) ([]apiv1.State, error) {
	c.lastMu.RLock()
	lastData := c.lastData
	c.lastMu.RUnlock()
	return lastData.getStates()
}

func (c *component) Events(ctx context.Context, since time.Time) ([]apiv1.Event, error) {
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

	ctx, cancel := context.WithTimeout(c.ctx, 30*time.Second)
	defer cancel()

	d.EgressLatencies, d.err = c.getEgressLatenciesFunc(ctx)
	if d.err != nil {
		d.healthy = false
		d.reason = fmt.Sprintf("error measuring egress latencies: %v", d.err)
		return
	}

	exceededMsgs := []string{}
	for _, lat := range d.EgressLatencies {
		region := fmt.Sprintf("%s (%s)", lat.RegionName, lat.Provider)
		metricEdgeInMilliseconds.With(prometheus.Labels{
			pkgmetrics.MetricLabelKey: region,
		}).Set(float64(lat.LatencyMilliseconds))

		if c.globalMillisecondThreshold > 0 && lat.LatencyMilliseconds > c.globalMillisecondThreshold {
			exceededMsgs = append(exceededMsgs, fmt.Sprintf("latency to %s edge server is %s (exceeded threshold %dms)", lat.RegionName, lat.Latency, c.globalMillisecondThreshold))
		}
	}

	if len(exceededMsgs) == 0 {
		d.healthy = true
		d.reason = fmt.Sprintf("checked egress latencies for %d edge servers, and no issue found", len(d.EgressLatencies))
	} else {
		d.healthy = false
		d.reason = strings.Join(exceededMsgs, "; ")
	}
}

type Data struct {
	// EgressLatencies is the list of egress latencies to global edge servers.
	EgressLatencies latency.Latencies `json:"egress_latencies"`

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

func (d *Data) getStates() ([]apiv1.State, error) {
	if d == nil {
		return []apiv1.State{
			{
				Name:    Name,
				Health:  apiv1.StateHealthy,
				Healthy: true,
				Reason:  "no data yet",
			},
		}, nil
	}

	state := apiv1.State{
		Name:   Name,
		Reason: d.reason,
		Error:  d.getError(),

		Healthy: d.healthy,
		Health:  apiv1.StateHealthy,
	}
	if !d.healthy {
		state.Health = apiv1.StateUnhealthy
	}

	b, _ := json.Marshal(d)
	state.ExtraInfo = map[string]string{
		"data":     string(b),
		"encoding": "json",
	}
	return []apiv1.State{state}, nil
}
