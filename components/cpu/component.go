// Package cpu tracks the combined usage of all CPUs (not per-CPU).
package cpu

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/olekukonko/tablewriter"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/load"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/eventstore"
	pkghost "github.com/leptonai/gpud/pkg/host"
	"github.com/leptonai/gpud/pkg/kmsg"
	"github.com/leptonai/gpud/pkg/log"
	pkgmetrics "github.com/leptonai/gpud/pkg/metrics"
)

// Name is the ID of the CPU component.
const Name = "cpu"

var _ components.Component = &component{}

type component struct {
	ctx    context.Context
	cancel context.CancelFunc

	getTimeStatFunc    func(ctx context.Context) (cpu.TimesStat, error)
	getUsedPctFunc     func(ctx context.Context) (float64, error)
	getLoadAvgStatFunc func(ctx context.Context) (*load.AvgStat, error)

	getPrevTimeStatFunc func() *cpu.TimesStat
	setPrevTimeStatFunc func(cpu.TimesStat)

	eventBucket eventstore.Bucket
	kmsgSyncer  *kmsg.Syncer

	lastMu          sync.RWMutex
	lastCheckResult *checkResult
}

func New(gpudInstance *components.GPUdInstance) (components.Component, error) {
	cctx, ccancel := context.WithCancel(gpudInstance.RootCtx)
	c := &component{
		ctx:    cctx,
		cancel: ccancel,

		getTimeStatFunc:    getTimeStatForAllCPUs,
		getUsedPctFunc:     getUsedPercentForAllCPUs,
		getLoadAvgStatFunc: load.AvgWithContext,

		getPrevTimeStatFunc: getPrevTimeStat,
		setPrevTimeStatFunc: setPrevTimeStat,
	}

	if gpudInstance.EventStore != nil && runtime.GOOS == "linux" {
		var err error
		c.eventBucket, err = gpudInstance.EventStore.Bucket(Name)
		if err != nil {
			ccancel()
			return nil, err
		}

		if os.Geteuid() == 0 {
			c.kmsgSyncer, err = kmsg.NewSyncer(cctx, Match, c.eventBucket)
			if err != nil {
				ccancel()
				return nil, err
			}
		}
	}

	return c, nil
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
	if c.eventBucket == nil {
		return nil, nil
	}
	return c.eventBucket.Get(ctx, since)
}

func (c *component) Close() error {
	log.Logger.Debugw("closing component")

	c.cancel()

	if c.kmsgSyncer != nil {
		c.kmsgSyncer.Close()
	}
	if c.eventBucket != nil {
		c.eventBucket.Close()
	}

	return nil
}

var (
	oneMinute  = time.Minute.String()
	fiveMinute = (5 * time.Minute).String()
	fifteenMin = (15 * time.Minute).String()
)

func (c *component) Check() components.CheckResult {
	log.Logger.Infow("checking cpu")

	cr := &checkResult{
		ts: time.Now().UTC(),

		Info: getInfo(),
		Cores: Cores{
			Logical: pkghost.CPULogicalCores(),
		},
	}

	defer func() {
		c.lastMu.Lock()
		c.lastCheckResult = cr
		c.lastMu.Unlock()
	}()

	cctx, ccancel := context.WithTimeout(c.ctx, 5*time.Second)
	curStat, err := c.getTimeStatFunc(cctx)
	ccancel()
	if err != nil {
		cr.err = err
		cr.health = apiv1.HealthStateTypeUnhealthy
		cr.reason = "error calculating CPU usage"
		log.Logger.Errorw(cr.reason, "error", cr.err)
		return cr
	}

	cctx, ccancel = context.WithTimeout(c.ctx, 5*time.Second)
	usedPct, err := c.getUsedPctFunc(cctx)
	ccancel()
	if err != nil {
		cr.err = err
		cr.health = apiv1.HealthStateTypeUnhealthy
		cr.reason = "error calculating CPU usage"
		log.Logger.Errorw(cr.reason, "error", cr.err)
		return cr
	}

	if c.getPrevTimeStatFunc != nil && c.setPrevTimeStatFunc != nil {
		usedPct = calculateCPUUsage(
			c.getPrevTimeStatFunc(),
			curStat,
			usedPct,
		)
		c.setPrevTimeStatFunc(curStat)

		cr.Usage = Usage{}
		cr.Usage.usedPercent = usedPct
		cr.Usage.UsedPercent = fmt.Sprintf("%.2f", usedPct)
		metricUsedPercent.With(prometheus.Labels{}).Set(usedPct)
	}

	cctx, ccancel = context.WithTimeout(c.ctx, 5*time.Second)
	loadAvg, err := c.getLoadAvgStatFunc(cctx)
	ccancel()
	if err != nil {
		cr.err = err
		cr.health = apiv1.HealthStateTypeUnhealthy
		cr.reason = "error calculating load average"
		log.Logger.Errorw(cr.reason, "error", cr.err)
		return cr
	}
	cr.Usage.LoadAvg1Min = fmt.Sprintf("%.2f", loadAvg.Load1)
	cr.Usage.LoadAvg5Min = fmt.Sprintf("%.2f", loadAvg.Load5)
	cr.Usage.LoadAvg15Min = fmt.Sprintf("%.2f", loadAvg.Load15)

	metricLoadAverage.With(prometheus.Labels{pkgmetrics.MetricLabelKey: oneMinute}).Set(loadAvg.Load1)
	metricLoadAverage.With(prometheus.Labels{pkgmetrics.MetricLabelKey: fiveMinute}).Set(loadAvg.Load5)
	metricLoadAverage.With(prometheus.Labels{pkgmetrics.MetricLabelKey: fifteenMin}).Set(loadAvg.Load15)

	cr.health = apiv1.HealthStateTypeHealthy
	cr.reason = "ok"

	return cr
}

var _ components.CheckResult = &checkResult{}

type checkResult struct {
	Info  Info  `json:"info"`
	Cores Cores `json:"cores"`
	Usage Usage `json:"usage"`

	// timestamp of the last check
	ts time.Time
	// error from the last check
	err error

	// tracks the healthy evaluation result of the last check
	health apiv1.HealthStateType
	// tracks the reason of the last check
	reason string
}

type Info struct {
	Arch      string `json:"arch"`
	GoArch    string `json:"go_arch"`
	CPU       string `json:"cpu"`
	Family    string `json:"family"`
	Model     string `json:"model"`
	ModelName string `json:"model_name"`
}

type Cores struct {
	Logical int `json:"logical"`
}

type Usage struct {
	// Used CPU in percentage.
	// Parse into float64 to get the actual value.
	UsedPercent string  `json:"used_percent"`
	usedPercent float64 `json:"-"`

	// Load average for the last 1-minute, with the scale of 1.00.
	// Parse into float64 to get the actual value.
	LoadAvg1Min string `json:"load_avg_1min"`
	// Load average for the last 5-minutes, with the scale of 1.00.
	// Parse into float64 to get the actual value.
	LoadAvg5Min string `json:"load_avg_5min"`
	// Load average for the last 15-minutes, with the scale of 1.00.
	// Parse into float64 to get the actual value.
	LoadAvg15Min string `json:"load_avg_15min"`
}

func (cr *checkResult) String() string {
	if cr == nil {
		return ""
	}

	buf := bytes.NewBuffer(nil)
	table := tablewriter.NewWriter(buf)
	table.SetAlignment(tablewriter.ALIGN_CENTER)
	table.Append([]string{"Arch", cr.Info.Arch})
	table.Append([]string{"Go Arch", cr.Info.GoArch})
	table.Append([]string{"CPU", cr.Info.CPU})
	table.Append([]string{"Family", cr.Info.Family})
	table.Append([]string{"Model", cr.Info.Model})
	table.Append([]string{"Model Name", cr.Info.ModelName})
	table.Append([]string{"Logical Cores", fmt.Sprintf("%d", cr.Cores.Logical)})
	table.Append([]string{"Avg Load 1-min", cr.Usage.LoadAvg1Min})
	table.Append([]string{"Avg Load 5-min", cr.Usage.LoadAvg5Min})
	table.Append([]string{"Avg Load 15-min", cr.Usage.LoadAvg15Min})
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
