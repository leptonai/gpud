// Package cpu tracks the combined usage of all CPUs (not per-CPU).
package cpu

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

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

	setPrevTimeStatFunc func(cpu.TimesStat)
	getPrevTimeStatFunc func() *cpu.TimesStat

	kmsgSyncer  *kmsg.Syncer
	eventBucket eventstore.Bucket

	info  Info
	cores Cores

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

		setPrevTimeStatFunc: setPrevTimeStat,
		getPrevTimeStatFunc: getPrevTimeStat,

		kmsgSyncer:  kmsgSyncer,
		eventBucket: eventBucket,

		info: getInfo(),
		cores: Cores{
			Logical: pkghost.CPULogicalCores(),
		},
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

func (c *component) States(ctx context.Context) ([]apiv1.State, error) {
	c.lastMu.RLock()
	lastData := c.lastData
	c.lastMu.RUnlock()
	return lastData.getStates()
}

func (c *component) Events(ctx context.Context, since time.Time) ([]apiv1.Event, error) {
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
	d := Data{
		ts:    time.Now().UTC(),
		Info:  c.info,
		Cores: c.cores,
	}
	defer func() {
		c.lastMu.Lock()
		c.lastData = &d
		c.lastMu.Unlock()
	}()

	cctx, ccancel := context.WithTimeout(c.ctx, 5*time.Second)
	curStat, err := c.getTimeStatFunc(cctx)
	ccancel()
	if err != nil {
		d.err = err
		d.healthy = false
		d.reason = fmt.Sprintf("error calculating CPU usage -- %s", err)
		return
	}

	cctx, ccancel = context.WithTimeout(c.ctx, 5*time.Second)
	usedPct, err := c.getUsedPctFunc(cctx)
	ccancel()
	if err != nil {
		d.err = err
		d.healthy = false
		d.reason = fmt.Sprintf("error calculating CPU usage -- %s", err)
		return
	}

	usedPct = calculateCPUUsage(
		c.getPrevTimeStatFunc(),
		curStat,
		usedPct,
	)
	if err != nil {
		d.err = err
		d.healthy = false
		d.reason = fmt.Sprintf("error calculating CPU usage -- %s", err)
		return
	}
	c.setPrevTimeStatFunc(curStat)

	d.Usage = Usage{}
	d.Usage.usedPercent = usedPct
	d.Usage.UsedPercent = fmt.Sprintf("%.2f", usedPct)
	metricUsedPercent.With(prometheus.Labels{}).Set(usedPct)

	cctx, ccancel = context.WithTimeout(c.ctx, 5*time.Second)
	loadAvg, err := c.getLoadAvgStatFunc(cctx)
	ccancel()
	if err != nil {
		d.err = err
		d.healthy = false
		d.reason = fmt.Sprintf("error calculating load average -- %s", err)
		return
	}
	d.Usage.LoadAvg1Min = fmt.Sprintf("%.2f", loadAvg.Load1)
	d.Usage.LoadAvg5Min = fmt.Sprintf("%.2f", loadAvg.Load5)
	d.Usage.LoadAvg15Min = fmt.Sprintf("%.2f", loadAvg.Load15)

	metricLoadAverage.With(prometheus.Labels{pkgmetrics.MetricLabelKey: oneMinute}).Set(loadAvg.Load1)
	metricLoadAverage.With(prometheus.Labels{pkgmetrics.MetricLabelKey: fiveMinute}).Set(loadAvg.Load5)
	metricLoadAverage.With(prometheus.Labels{pkgmetrics.MetricLabelKey: fifteenMin}).Set(loadAvg.Load15)

	d.healthy = true
	d.reason = fmt.Sprintf("arch: %s, cpu: %s, family: %s, model: %s, model_name: %s",
		d.Info.Arch, d.Info.CPU, d.Info.Family, d.Info.Model, d.Info.ModelName)
}

type Data struct {
	Info  Info  `json:"info"`
	Cores Cores `json:"cores"`
	Usage Usage `json:"usage"`

	// timestamp of the last check
	ts time.Time
	// error from the last check
	err error

	// tracks the healthy evaluation result of the last check
	healthy bool
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
				Name:              Name,
				Health:            apiv1.StateTypeHealthy,
				DeprecatedHealthy: true,
				Reason:            "no data yet",
			},
		}, nil
	}

	state := apiv1.State{
		Name:   Name,
		Reason: d.reason,
		Error:  d.getError(),

		DeprecatedHealthy: d.healthy,
		Health:            apiv1.StateTypeHealthy,
	}
	if !d.healthy {
		state.Health = apiv1.StateTypeUnhealthy
	}

	b, _ := json.Marshal(d)
	state.DeprecatedExtraInfo = map[string]string{
		"data":     string(b),
		"encoding": "json",
	}
	return []apiv1.State{state}, nil
}
