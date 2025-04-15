// Package latency tracks the global network connectivity statistics.
package latency

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/log"
	pkgmetrics "github.com/leptonai/gpud/pkg/metrics"
	"github.com/leptonai/gpud/pkg/netutil/latency"
	latencyedge "github.com/leptonai/gpud/pkg/netutil/latency/edge"
	"github.com/olekukonko/tablewriter"
	"github.com/prometheus/client_golang/prometheus"
)

// Name is the ID of the network latency component.
const Name = "network-latency"

const (
	// 1 second
	MinGlobalMillisecondThreshold = 1000
	// 7 seconds by default to reach any of the DERP servers.
	DefaultGlobalMillisecondThreshold = 7000
)

var _ components.Component = &component{}

type component struct {
	ctx    context.Context
	cancel context.CancelFunc

	getEgressLatenciesFunc func(context.Context, ...latencyedge.OpOption) (latency.Latencies, error)

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

		getEgressLatenciesFunc: latencyedge.Measure,

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
	log.Logger.Infow("checking network egress latency")

	d := checkHealthState(
		c.ctx,
		c.getEgressLatenciesFunc,
		c.globalMillisecondThreshold,
	)

	c.lastMu.Lock()
	c.lastData = d
	c.lastMu.Unlock()
}

func CheckHealthState(ctx context.Context) (components.HealthStateCheckResult, error) {
	d := checkHealthState(
		ctx,
		latencyedge.Measure,
		DefaultGlobalMillisecondThreshold,
	)
	if d.err != nil {
		return nil, d.err
	}
	return d, nil
}

func checkHealthState(
	ctx context.Context,
	getEgressLatenciesFunc func(context.Context, ...latencyedge.OpOption) (latency.Latencies, error),
	globalMillisecondThreshold int64,
) *Data {
	d := &Data{
		ts: time.Now().UTC(),
	}

	d.EgressLatencies, d.err = getEgressLatenciesFunc(ctx)
	if d.err != nil {
		d.health = apiv1.StateTypeUnhealthy
		d.reason = fmt.Sprintf("error measuring egress latencies: %v", d.err)
		return d
	}

	exceededMsgs := []string{}
	for _, lat := range d.EgressLatencies {
		region := fmt.Sprintf("%s (%s)", lat.RegionName, lat.Provider)
		metricEdgeInMilliseconds.With(prometheus.Labels{
			pkgmetrics.MetricLabelKey: region,
		}).Set(float64(lat.LatencyMilliseconds))

		if globalMillisecondThreshold > 0 && lat.LatencyMilliseconds > globalMillisecondThreshold {
			exceededMsgs = append(exceededMsgs, fmt.Sprintf("latency to %s edge server is %s (exceeded threshold %dms)", lat.RegionName, lat.Latency, globalMillisecondThreshold))
		}
	}

	if len(exceededMsgs) == 0 {
		d.health = apiv1.StateTypeHealthy
		d.reason = fmt.Sprintf("checked egress latencies for %d edge servers, and no issue found", len(d.EgressLatencies))
	} else {
		d.health = apiv1.StateTypeUnhealthy
		d.reason = strings.Join(exceededMsgs, "; ")
	}

	return d
}

var _ components.HealthStateCheckResult = &Data{}

type Data struct {
	// EgressLatencies is the list of egress latencies to global edge servers.
	EgressLatencies latency.Latencies `json:"egress_latencies"`

	// timestamp of the last check
	ts time.Time
	// error from the last check
	err error

	// tracks the healthy evaluation result of the last check
	health apiv1.HealthStateType
	// tracks the reason of the last check
	reason string
}

func (d *Data) String() string {
	if d == nil {
		return ""
	}

	buf := bytes.NewBuffer(nil)
	table := tablewriter.NewWriter(buf)
	table.SetAlignment(tablewriter.ALIGN_CENTER)

	table.SetHeader([]string{"Region", "Latency"})
	for _, lat := range d.EgressLatencies {
		table.Append([]string{
			fmt.Sprintf("%s (%s)", lat.RegionName, lat.Provider),
			lat.Latency.Duration.String(),
		})
	}

	table.Render()

	return buf.String()
}

func (d *Data) Summary() string {
	return d.reason
}

func (d *Data) HealthState() apiv1.HealthStateType {
	if d == nil {
		return ""
	}
	return d.health
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
		Health: d.health,
	}

	b, _ := json.Marshal(d)
	state.DeprecatedExtraInfo = map[string]string{
		"data":     string(b),
		"encoding": "json",
	}
	return []apiv1.HealthState{state}, nil
}
