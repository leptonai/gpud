// Package cpu tracks the combined usage of all CPUs (not per-CPU).
package cpu

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/shirou/gopsutil/v4/load"

	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/eventstore"
	pkghost "github.com/leptonai/gpud/pkg/host"
	"github.com/leptonai/gpud/pkg/kmsg"
	"github.com/leptonai/gpud/pkg/log"
	pkgmetrics "github.com/leptonai/gpud/pkg/metrics"
)

// Name is the ID of the CPU component.
const (
	Name = "cpu"
)

var _ components.Component = &component{}

type component struct {
	ctx    context.Context
	cancel context.CancelFunc

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

	info := getInfo()
	cores := Cores{
		Logical: pkghost.CPULogicalCores(),
	}

	cctx, ccancel := context.WithCancel(ctx)
	kmsgSyncer, err := kmsg.NewSyncer(cctx, Match, eventBucket)
	if err != nil {
		ccancel()
		return nil, err
	}

	return &component{
		ctx:         ctx,
		cancel:      ccancel,
		kmsgSyncer:  kmsgSyncer,
		eventBucket: eventBucket,
		info:        info,
		cores:       cores,
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

func (c *component) States(ctx context.Context) ([]components.State, error) {
	c.lastMu.RLock()
	lastData := c.lastData
	c.lastMu.RUnlock()
	return lastData.getStates()
}

func (c *component) Events(ctx context.Context, since time.Time) ([]components.Event, error) {
	return c.eventBucket.Get(ctx, since)
}

// TO BE DEPRECATED
func (c *component) Metrics(ctx context.Context, since time.Time) ([]components.Metric, error) {
	log.Logger.Debugw("querying metrics", "since", since)

	return nil, nil
}

func (c *component) Close() error {
	log.Logger.Debugw("closing component")
	c.cancel()
	c.kmsgSyncer.Close()
	c.eventBucket.Close()
	return nil
}

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

	curStat, usedPct, err := calculateCPUUsage(
		c.ctx,
		getPrevTimeStat(),
		getTimeStatForAllCPUs,
		getUsedPercentForAllCPUs,
	)
	if err != nil {
		d.err = err
		return
	}
	setPrevTimeStat(curStat)

	d.Usage = Usage{}
	d.Usage.usedPercent = usedPct
	d.Usage.UsedPercent = fmt.Sprintf("%.2f", usedPct)
	usedPercent.With(prometheus.Labels{}).Set(usedPct)

	cctx, ccancel := context.WithTimeout(c.ctx, 5*time.Second)
	loadAvg, err := load.AvgWithContext(cctx)
	ccancel()
	if err != nil {
		d.err = err
		return
	}
	d.Usage.LoadAvg1Min = fmt.Sprintf("%.2f", loadAvg.Load1)
	d.Usage.LoadAvg5Min = fmt.Sprintf("%.2f", loadAvg.Load5)
	d.Usage.LoadAvg15Min = fmt.Sprintf("%.2f", loadAvg.Load15)

	loadAverage.With(prometheus.Labels{pkgmetrics.MetricLabelKey: oneMinute}).Set(loadAvg.Load1)
	loadAverage.With(prometheus.Labels{pkgmetrics.MetricLabelKey: fiveMinute}).Set(loadAvg.Load5)
	loadAverage.With(prometheus.Labels{pkgmetrics.MetricLabelKey: fifteenMin}).Set(loadAvg.Load15)
}

var (
	oneMinute  = time.Minute.String()
	fiveMinute = (5 * time.Minute).String()
	fifteenMin = (15 * time.Minute).String()
)

type Data struct {
	Info  Info  `json:"info"`
	Cores Cores `json:"cores"`
	Usage Usage `json:"usage"`

	// timestamp of the last check
	ts time.Time `json:"-"`
	// error from the last check
	err error `json:"-"`
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

func (d *Data) getReason() string {
	if d == nil {
		return "no cpu data found"
	}
	if d.err != nil {
		return fmt.Sprintf("failed to get CPU data -- %s", d.err)
	}

	return fmt.Sprintf("arch: %s, cpu: %s, family: %s, model: %s, model_name: %s",
		d.Info.Arch, d.Info.CPU, d.Info.Family, d.Info.Model, d.Info.ModelName)
}

func (d *Data) getHealth() (string, bool) {
	healthy := d == nil || d.err == nil
	health := components.StateHealthy
	if !healthy {
		health = components.StateUnhealthy
	}
	return health, healthy
}

func (d *Data) getError() string {
	if d == nil || d.err == nil {
		return ""
	}
	return d.err.Error()
}

func (d *Data) getStates() ([]components.State, error) {
	state := components.State{
		Name:   Name,
		Reason: d.getReason(),
		Error:  d.getError(),
	}
	state.Health, state.Healthy = d.getHealth()

	b, _ := json.Marshal(d)
	state.ExtraInfo = map[string]string{
		"data":     string(b),
		"encoding": "json",
	}
	return []components.State{state}, nil
}
