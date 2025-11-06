package netstat

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/log"
	"github.com/leptonai/gpud/pkg/netutil"
)

// Name is the ID of the network netstat component.
const Name = "network-netstat"

var _ components.Component = (*component)(nil)

type component struct {
	ctx    context.Context
	cancel context.CancelFunc

	collectCounters func() (netutil.NetStatCounters, error)
	now             func() time.Time

	lastMu          sync.RWMutex
	lastCheckResult *checkResult
}

// New constructs the network netstat component.
func New(gpudInstance *components.GPUdInstance) (components.Component, error) {
	cctx, ccancel := context.WithCancel(gpudInstance.RootCtx)

	return &component{
		ctx:    cctx,
		cancel: ccancel,
		collectCounters: func() (netutil.NetStatCounters, error) {
			// The netstat collector shape matches the Prometheus node_exporter
			// implementation (collector/netstat_linux.go).
			return netutil.ReadNetStatCounters()
		},
		now: func() time.Time {
			return time.Now().UTC()
		},
	}, nil
}

func (c *component) Name() string { return Name }

func (c *component) Tags() []string {
	return []string{
		"network",
		Name,
	}
}

func (c *component) IsSupported() bool {
	return runtime.GOOS == "linux"
}

func (c *component) Start() error {
	// No background poller yet; metrics are gathered on-demand via Check.
	return nil
}

func (c *component) LastHealthStates() apiv1.HealthStates {
	c.lastMu.RLock()
	last := c.lastCheckResult
	c.lastMu.RUnlock()

	if last == nil {
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

	return last.HealthStates()
}

func (c *component) Events(context.Context, time.Time) (apiv1.Events, error) {
	return nil, nil
}

func (c *component) Close() error {
	log.Logger.Debugw("closing component")
	c.cancel()
	return nil
}

func (c *component) Check() components.CheckResult {
	log.Logger.Infow("collecting netstat counters")

	cr := &checkResult{
		ts:     c.now(),
		health: apiv1.HealthStateTypeHealthy, // experimental metrics must not impact health
	}

	defer func() {
		c.lastMu.Lock()
		c.lastCheckResult = cr
		c.lastMu.Unlock()
	}()

	counters, err := c.collectCounters()
	if err != nil {
		cr.err = err
		cr.reason = "failed to collect netstat counters (experimental; health unaffected)"
		// Experimental metrics: emit a warning but keep the component healthy.
		log.Logger.Warnw(cr.reason, "error", err)
		return cr
	}

	cr.counters = counters
	cr.reason = "collected netstat counters"
	setMetrics(counters)

	return cr
}

func setMetrics(counters netutil.NetStatCounters) {
	labels := prometheus.Labels{}
	metricTCPRetransSegmentsTotal.With(labels).Set(float64(counters.TCPRetransSegments))
	metricTCPExtSegmentRetransmitsTotal.With(labels).Set(float64(counters.TcpExtSegmentRetransmits))
	metricUDPInErrorsTotal.With(labels).Set(float64(counters.UDPInErrors))
	metricUDPRcvbufErrorsTotal.With(labels).Set(float64(counters.UDPRcvbufErrors))
	metricUDPSndbufErrorsTotal.With(labels).Set(float64(counters.UDPSndbufErrors))
}

var _ components.CheckResult = (*checkResult)(nil)

type checkResult struct {
	counters netutil.NetStatCounters

	ts     time.Time
	err    error
	health apiv1.HealthStateType
	reason string
}

func (cr *checkResult) ComponentName() string {
	return Name
}

func (cr *checkResult) String() string {
	if cr == nil {
		return ""
	}

	return fmt.Sprintf(
		"tcp_retrans_segments_total=%d tcp_ext_segment_retransmits_total=%d udp_in_errors_total=%d udp_rcvbuf_errors_total=%d udp_sndbuf_errors_total=%d",
		cr.counters.TCPRetransSegments,
		cr.counters.TcpExtSegmentRetransmits,
		cr.counters.UDPInErrors,
		cr.counters.UDPRcvbufErrors,
		cr.counters.UDPSndbufErrors,
	)
}

func (cr *checkResult) Summary() string {
	if cr == nil {
		return ""
	}
	return cr.reason
}

func (cr *checkResult) HealthStateType() apiv1.HealthStateType {
	if cr == nil {
		return apiv1.HealthStateTypeHealthy
	}
	return cr.health
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

	return apiv1.HealthStates{
		{
			Time:      metav1.NewTime(cr.ts),
			Component: Name,
			Name:      Name,
			Health:    cr.health,
			Reason:    cr.reason,
			Error:     cr.getError(),
		},
	}
}

func (cr *checkResult) getError() string {
	if cr == nil || cr.err == nil {
		return ""
	}
	return cr.err.Error()
}
