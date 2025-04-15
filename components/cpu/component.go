// Package cpu tracks the combined usage of all CPUs (not per-CPU).
package cpu

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/olekukonko/tablewriter"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/load"

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

	kmsgSyncer  *kmsg.Syncer
	eventBucket eventstore.Bucket

	lastMu   sync.RWMutex
	lastData *Data
}

func New(ctx context.Context, eventStore eventstore.Store) (components.Component, error) {
	eventBucket, err := eventStore.Bucket(Name)
	if err != nil {
		return nil, err
	}

	cctx, ccancel := context.WithCancel(ctx)
	kmsgSyncer, err := kmsg.NewSyncer(cctx, Match, eventBucket)
	if err != nil {
		ccancel()
		return nil, err
	}

	return &component{
		ctx:    cctx,
		cancel: ccancel,

		getTimeStatFunc:    getTimeStatForAllCPUs,
		getUsedPctFunc:     getUsedPercentForAllCPUs,
		getLoadAvgStatFunc: load.AvgWithContext,

		getPrevTimeStatFunc: getPrevTimeStat,
		setPrevTimeStatFunc: setPrevTimeStat,

		kmsgSyncer:  kmsgSyncer,
		eventBucket: eventBucket,
	}, nil
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

// CheckOnce checks the current pods
// run this periodically
func (c *component) CheckOnce() {
	log.Logger.Infow("checking cpu")

	d := checkHealthState(
		c.ctx,
		c.getTimeStatFunc,
		c.getUsedPctFunc,
		c.getLoadAvgStatFunc,
		c.getPrevTimeStatFunc,
		c.setPrevTimeStatFunc,
	)

	c.lastMu.Lock()
	c.lastData = d
	c.lastMu.Unlock()
}

func CheckHealthState(ctx context.Context) (components.HealthStateCheckResult, error) {
	d := checkHealthState(
		ctx,
		getTimeStatForAllCPUs,
		getUsedPercentForAllCPUs,
		load.AvgWithContext,
		nil,
		nil,
	)
	if d.err != nil {
		return nil, d.err
	}
	return d, nil
}

func checkHealthState(
	ctx context.Context,
	getTimeStatFunc func(ctx context.Context) (cpu.TimesStat, error),
	getUsedPctFunc func(ctx context.Context) (float64, error),
	getLoadAvgStatFunc func(ctx context.Context) (*load.AvgStat, error),
	getPrevTimeStatFunc func() *cpu.TimesStat,
	setPrevTimeStatFunc func(cpu.TimesStat),
) *Data {
	d := &Data{
		ts: time.Now().UTC(),

		Info: getInfo(),
		Cores: Cores{
			Logical: pkghost.CPULogicalCores(),
		},
	}

	cctx, ccancel := context.WithTimeout(ctx, 5*time.Second)
	curStat, err := getTimeStatFunc(cctx)
	ccancel()
	if err != nil {
		d.err = err
		d.health = apiv1.StateTypeUnhealthy
		d.reason = fmt.Sprintf("error calculating CPU usage -- %s", err)
		return d
	}

	cctx, ccancel = context.WithTimeout(ctx, 5*time.Second)
	usedPct, err := getUsedPctFunc(cctx)
	ccancel()
	if err != nil {
		d.err = err
		d.health = apiv1.StateTypeUnhealthy
		d.reason = fmt.Sprintf("error calculating CPU usage -- %s", err)
		return d
	}

	if getPrevTimeStatFunc != nil && setPrevTimeStatFunc != nil {
		usedPct = calculateCPUUsage(
			getPrevTimeStatFunc(),
			curStat,
			usedPct,
		)
		if err != nil {
			d.err = err
			d.health = apiv1.StateTypeUnhealthy
			d.reason = fmt.Sprintf("error calculating CPU usage -- %s", err)
			return d
		}
		setPrevTimeStatFunc(curStat)

		d.Usage = Usage{}
		d.Usage.usedPercent = usedPct
		d.Usage.UsedPercent = fmt.Sprintf("%.2f", usedPct)
		metricUsedPercent.With(prometheus.Labels{}).Set(usedPct)
	}

	cctx, ccancel = context.WithTimeout(ctx, 5*time.Second)
	loadAvg, err := getLoadAvgStatFunc(cctx)
	ccancel()
	if err != nil {
		d.err = err
		d.health = apiv1.StateTypeUnhealthy
		d.reason = fmt.Sprintf("error calculating load average -- %s", err)
		return d
	}
	d.Usage.LoadAvg1Min = fmt.Sprintf("%.2f", loadAvg.Load1)
	d.Usage.LoadAvg5Min = fmt.Sprintf("%.2f", loadAvg.Load5)
	d.Usage.LoadAvg15Min = fmt.Sprintf("%.2f", loadAvg.Load15)

	metricLoadAverage.With(prometheus.Labels{pkgmetrics.MetricLabelKey: oneMinute}).Set(loadAvg.Load1)
	metricLoadAverage.With(prometheus.Labels{pkgmetrics.MetricLabelKey: fiveMinute}).Set(loadAvg.Load5)
	metricLoadAverage.With(prometheus.Labels{pkgmetrics.MetricLabelKey: fifteenMin}).Set(loadAvg.Load15)

	d.health = apiv1.StateTypeHealthy
	d.reason = fmt.Sprintf("arch: %s, cpu: %s, family: %s, model: %s, model_name: %s",
		d.Info.Arch, d.Info.CPU, d.Info.Family, d.Info.Model, d.Info.ModelName)

	return d
}

var _ components.HealthStateCheckResult = &Data{}

type Data struct {
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

func (d *Data) String() string {
	if d == nil {
		return ""
	}

	buf := bytes.NewBuffer(nil)
	table := tablewriter.NewWriter(buf)
	table.SetAlignment(tablewriter.ALIGN_CENTER)
	table.Append([]string{"Arch", d.Info.Arch})
	table.Append([]string{"CPU", d.Info.CPU})
	table.Append([]string{"Family", d.Info.Family})
	table.Append([]string{"Model", d.Info.Model})
	table.Append([]string{"Model Name", d.Info.ModelName})
	table.Append([]string{"Avg Load 1-min", d.Usage.LoadAvg1Min})
	table.Append([]string{"Avg Load 5-min", d.Usage.LoadAvg5Min})
	table.Append([]string{"Avg Load 15-min", d.Usage.LoadAvg15Min})
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
