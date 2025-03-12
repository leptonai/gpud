// Package cpu tracks the combined usage of all CPUs (not per-CPU).
package cpu

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/shirou/gopsutil/v4/load"

	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/components/cpu/metrics"
	"github.com/leptonai/gpud/pkg/dmesg"
	"github.com/leptonai/gpud/pkg/eventstore"
	"github.com/leptonai/gpud/pkg/kmsg"
	"github.com/leptonai/gpud/pkg/log"
)

// Name is the ID of the CPU component.
const Name = "cpu"

var _ components.Component = &component{}

type component struct {
	ctx    context.Context
	cancel context.CancelFunc

	logLineProcessor *dmesg.LogLineProcessor
	eventBucket      eventstore.Bucket
	gatherer         prometheus.Gatherer

	// experimental
	kmsgWatcher kmsg.Watcher

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

	kmsgWatcher, err := kmsg.StartWatch(Match)
	if err != nil {
		return nil, err
	}

	info, err := getInfo()
	if err != nil {
		return nil, err
	}
	cores, err := getCores()
	if err != nil {
		return nil, err
	}

	// TODO: deprecate
	cctx, ccancel := context.WithCancel(ctx)
	logLineProcessor, err := dmesg.NewLogLineProcessor(cctx, Match, eventBucket)
	if err != nil {
		ccancel()
		return nil, err
	}

	return &component{
		ctx:              ctx,
		cancel:           ccancel,
		logLineProcessor: logLineProcessor,
		eventBucket:      eventBucket,
		kmsgWatcher:      kmsgWatcher,
		info:             info,
		cores:            cores,
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

func (c *component) Metrics(ctx context.Context, since time.Time) ([]components.Metric, error) {
	log.Logger.Debugw("querying metrics", "since", since)

	loadAverage5mins, err := metrics.ReadLoadAverage5mins(ctx, since)
	if err != nil {
		return nil, fmt.Errorf("failed to read load average 5mins: %w", err)
	}

	usedPercents, err := metrics.ReadUsedPercents(ctx, since)
	if err != nil {
		return nil, fmt.Errorf("failed to read used percents: %w", err)
	}

	ms := make([]components.Metric, 0, len(loadAverage5mins)+len(usedPercents))
	for _, m := range loadAverage5mins {
		ms = append(ms, components.Metric{
			Metric: m,
		})
	}
	for _, m := range usedPercents {
		ms = append(ms, components.Metric{
			Metric: m,
		})
	}

	return ms, nil
}

func (c *component) Close() error {
	log.Logger.Debugw("closing component")
	c.cancel()

	c.logLineProcessor.Close()
	c.eventBucket.Close()

	if c.kmsgWatcher != nil {
		c.kmsgWatcher.Close()
	}

	return nil
}

var _ components.PromRegisterer = (*component)(nil)

func (c *component) RegisterCollectors(reg *prometheus.Registry, dbRW *sql.DB, dbRO *sql.DB, tableName string) error {
	c.gatherer = reg
	return metrics.Register(reg, dbRW, dbRO, tableName)
}

// CheckOnce checks the current pods
// run this periodically
func (c *component) CheckOnce() {
	log.Logger.Infow("checking cpu")
	d := Data{
		ts:    time.Now().UTC(),
		Info:  &c.info,
		Cores: &c.cores,
	}
	metrics.SetLastUpdateUnixSeconds(float64(d.ts.Unix()))
	defer func() {
		c.lastMu.Lock()
		c.lastData = &d
		c.lastMu.Unlock()
	}()

	curStat, usedPercent, err := calculateCPUUsage(
		c.ctx,
		getPrevTimeStat(),
		getTimeStatForAllCPUs,
		getUsedPercentForAllCPUs,
	)
	if err != nil {
		d.Usage.err = err
		return
	}
	setPrevTimeStat(curStat)

	d.Usage = &Usage{}
	d.Usage.usedPercent = usedPercent
	d.Usage.UsedPercent = fmt.Sprintf("%.2f", usedPercent)

	cctx, ccancel := context.WithTimeout(c.ctx, 5*time.Second)
	err = metrics.SetUsedPercent(cctx, d.Usage.usedPercent, d.ts)
	ccancel()
	if err != nil {
		d.Usage.err = err
		return
	}

	cctx, ccancel = context.WithTimeout(c.ctx, 5*time.Second)
	loadAvg, err := load.AvgWithContext(cctx)
	ccancel()
	if err != nil {
		d.Usage.err = err
		return
	}
	d.Usage.LoadAvg1Min = fmt.Sprintf("%.2f", loadAvg.Load1)
	d.Usage.LoadAvg5Min = fmt.Sprintf("%.2f", loadAvg.Load5)
	d.Usage.LoadAvg15Min = fmt.Sprintf("%.2f", loadAvg.Load15)

	cctx, ccancel = context.WithTimeout(c.ctx, 5*time.Second)
	err = metrics.SetLoadAverage(cctx, time.Minute, loadAvg.Load1, d.ts)
	ccancel()
	if err != nil {
		d.Usage.err = err
		return
	}

	cctx, ccancel = context.WithTimeout(c.ctx, 5*time.Second)
	err = metrics.SetLoadAverage(cctx, 5*time.Minute, loadAvg.Load5, d.ts)
	ccancel()
	if err != nil {
		d.Usage.err = err
		return
	}

	cctx, ccancel = context.WithTimeout(c.ctx, 5*time.Second)
	err = metrics.SetLoadAverage(cctx, 15*time.Minute, loadAvg.Load15, d.ts)
	ccancel()
	if err != nil {
		d.Usage.err = err
		return
	}
}

type Data struct {
	Info  *Info  `json:"info"`
	Cores *Cores `json:"cores"`
	Usage *Usage `json:"usage"`

	// timestamp of the last check
	ts time.Time `json:"-"`
}

type Info struct {
	Arch      string `json:"arch"`
	CPU       string `json:"cpu"`
	Family    string `json:"family"`
	Model     string `json:"model"`
	ModelName string `json:"model_name"`

	// error from the last check
	err error `json:"-"`
}

func (i *Info) getReason() string {
	if i == nil {
		return "no cpu info found"
	}
	if i.err != nil {
		return fmt.Sprintf("failed to get CPU information -- %s", i.err)
	}

	return fmt.Sprintf("arch: %s, cpu: %s, family: %s, model: %s, model_name: %s",
		i.Arch, i.CPU, i.Family, i.Model, i.ModelName)
}

func (i *Info) getHealth() (string, bool) {
	healthy := i == nil || i.err == nil
	health := components.StateHealthy
	if !healthy {
		health = components.StateUnhealthy
	}
	return health, healthy
}

type Cores struct {
	Logical int `json:"logical"`

	// error from the last check
	err error `json:"-"`
}

func (c *Cores) getReason() string {
	if c == nil {
		return "no cpu cores found"
	}
	if c.err != nil {
		return fmt.Sprintf("failed to get CPU cores -- %s", c.err)
	}

	return fmt.Sprintf("logical: %d cores", c.Logical)
}

func (c *Cores) getHealth() (string, bool) {
	healthy := c == nil || c.err == nil
	health := components.StateHealthy
	if !healthy {
		health = components.StateUnhealthy
	}
	return health, healthy
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

	// error from the last check
	err error `json:"-"`
}

func (u *Usage) getReason() string {
	if u == nil {
		return "no cpu usage found"
	}
	if u.err != nil {
		return fmt.Sprintf("failed to get CPU usage -- %s", u.err)
	}

	return fmt.Sprintf("used_percent: %s%%, load_avg_1min: %s, load_avg_5min: %s, load_avg_15min: %s",
		u.UsedPercent, u.LoadAvg1Min, u.LoadAvg5Min, u.LoadAvg15Min)
}

func (u *Usage) getHealth() (string, bool) {
	healthy := u == nil || u.err == nil
	health := components.StateHealthy
	if !healthy {
		health = components.StateUnhealthy
	}
	return health, healthy
}

func (d *Data) getStates() ([]components.State, error) {
	if d == nil {
		return []components.State{
			{
				Name:    Name,
				Health:  components.StateHealthy,
				Healthy: true,
				Reason:  "no data yet",
			},
		}, nil
	}

	stateInfo := components.State{
		Name:   "info",
		Reason: d.Info.getReason(),
	}
	stateInfo.Health, stateInfo.Healthy = d.Info.getHealth()
	b, _ := json.Marshal(d)
	stateInfo.ExtraInfo = map[string]string{
		"data":     string(b),
		"encoding": "json",
	}

	stateCores := components.State{
		Name:   "cores",
		Reason: d.Cores.getReason(),
	}
	stateCores.Health, stateCores.Healthy = d.Cores.getHealth()
	b, _ = json.Marshal(d)
	stateCores.ExtraInfo = map[string]string{
		"data":     string(b),
		"encoding": "json",
	}

	stateUsage := components.State{
		Name:   "usage",
		Reason: d.Usage.getReason(),
	}
	stateUsage.Health, stateUsage.Healthy = d.Usage.getHealth()
	b, _ = json.Marshal(d)
	stateUsage.ExtraInfo = map[string]string{
		"data":     string(b),
		"encoding": "json",
	}

	return []components.State{
		stateInfo,
		stateCores,
		stateUsage,
	}, nil
}
