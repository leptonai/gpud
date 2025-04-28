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

	"github.com/olekukonko/tablewriter"
	"github.com/prometheus/client_golang/prometheus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/log"
	pkgmetrics "github.com/leptonai/gpud/pkg/metrics"
	"github.com/leptonai/gpud/pkg/netutil/latency"
	latencyedge "github.com/leptonai/gpud/pkg/netutil/latency/edge"
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

	lastMu          sync.RWMutex
	lastCheckResult *checkResult
}

func New(gpudInstance *components.GPUdInstance) (components.Component, error) {
	cctx, ccancel := context.WithCancel(gpudInstance.RootCtx)
	return &component{
		ctx:                        cctx,
		cancel:                     ccancel,
		getEgressLatenciesFunc:     latencyedge.Measure,
		globalMillisecondThreshold: DefaultGlobalMillisecondThreshold,
	}, nil
}

func (c *component) Name() string { return Name }

func (c *component) Start() error {
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()

		for {
			_ = c.Check()

			select {
			case <-c.ctx.Done():
				return
			case <-ticker.C:
			}
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
	return nil, nil
}

func (c *component) Close() error {
	log.Logger.Debugw("closing component")

	c.cancel()

	return nil
}

func (c *component) Check() components.CheckResult {
	log.Logger.Infow("checking network egress latency")

	cr := &checkResult{
		ts: time.Now().UTC(),
	}

	defer func() {
		c.lastMu.Lock()
		c.lastCheckResult = cr
		c.lastMu.Unlock()
	}()

	cctx, ccancel := context.WithTimeout(c.ctx, 15*time.Second)
	cr.EgressLatencies, cr.err = c.getEgressLatenciesFunc(cctx)
	ccancel()
	if cr.err != nil {
		cr.health = apiv1.HealthStateTypeUnhealthy
		cr.reason = fmt.Sprintf("error measuring egress latencies: %v", cr.err)
		return cr
	}

	exceededMsgs := []string{}
	for _, lat := range cr.EgressLatencies {
		region := fmt.Sprintf("%s (%s)", lat.RegionName, lat.Provider)
		metricEdgeInMilliseconds.With(prometheus.Labels{
			pkgmetrics.MetricLabelKey: region,
		}).Set(float64(lat.LatencyMilliseconds))

		if c.globalMillisecondThreshold > 0 && lat.LatencyMilliseconds > c.globalMillisecondThreshold {
			exceededMsgs = append(exceededMsgs, fmt.Sprintf("latency to %s edge server is %s (exceeded threshold %dms)", lat.RegionName, lat.Latency, c.globalMillisecondThreshold))
		}
	}

	if len(exceededMsgs) == 0 {
		cr.health = apiv1.HealthStateTypeHealthy
		cr.reason = fmt.Sprintf("checked egress latencies for %d edge servers, and no issue found", len(cr.EgressLatencies))
	} else {
		cr.health = apiv1.HealthStateTypeUnhealthy
		cr.reason = strings.Join(exceededMsgs, "; ")
	}

	return cr
}

var _ components.CheckResult = &checkResult{}

type checkResult struct {
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

func (cr *checkResult) String() string {
	if cr == nil {
		return ""
	}

	buf := bytes.NewBuffer(nil)
	table := tablewriter.NewWriter(buf)
	table.SetAlignment(tablewriter.ALIGN_CENTER)

	table.SetHeader([]string{"Region", "Latency"})
	for _, lat := range cr.EgressLatencies {
		table.Append([]string{
			fmt.Sprintf("%s (%s)", lat.RegionName, lat.Provider),
			lat.Latency.Duration.String(),
		})
	}

	table.Render()

	return buf.String()
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
				Time:      metav1.NewTime(time.Now().UTC()),
				Component: Name,
				Name:      Name,
				Health:    apiv1.HealthStateTypeHealthy,
				Reason:    "no data yet",
			},
		}
	}

	state := apiv1.HealthState{
		Time:      metav1.NewTime(cr.ts),
		Component: Name,
		Name:      Name,
		Reason:    cr.reason,
		Error:     cr.getError(),
		Health:    cr.health,
	}

	b, _ := json.Marshal(cr)
	state.ExtraInfo = map[string]string{
		"data":     string(b),
		"encoding": "json",
	}
	return apiv1.HealthStates{state}
}
