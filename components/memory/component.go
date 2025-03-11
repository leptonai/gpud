// Package memory tracks the memory usage of the host.
package memory

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/shirou/gopsutil/v4/mem"

	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/components/memory/metrics"
	"github.com/leptonai/gpud/pkg/dmesg"
	"github.com/leptonai/gpud/pkg/eventstore"
	"github.com/leptonai/gpud/pkg/kmsg"
	"github.com/leptonai/gpud/pkg/log"
)

// Name is the ID of the memory component.
const Name = "memory"

var _ components.Component = &component{}

type component struct {
	ctx    context.Context
	cancel context.CancelFunc

	logLineProcessor *dmesg.LogLineProcessor
	eventBucket      eventstore.Bucket
	gatherer         prometheus.Gatherer

	// experimental
	kmsgWatcher kmsg.Watcher

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

	// TODO: deprecate
	cctx, ccancel := context.WithCancel(ctx)
	logLineProcessor, err := dmesg.NewLogLineProcessor(cctx, Match, eventBucket)
	if err != nil {
		ccancel()
		return nil, err
	}

	return &component{
		ctx:              cctx,
		cancel:           ccancel,
		logLineProcessor: logLineProcessor,
		eventBucket:      eventBucket,
		kmsgWatcher:      kmsgWatcher,
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

	totalBytes, err := metrics.ReadTotalBytes(ctx, since)
	if err != nil {
		return nil, fmt.Errorf("failed to read total bytes: %w", err)
	}
	usedBytes, err := metrics.ReadUsedBytes(ctx, since)
	if err != nil {
		return nil, fmt.Errorf("failed to read used bytes: %w", err)
	}
	usedPercents, err := metrics.ReadUsedPercents(ctx, since)
	if err != nil {
		return nil, fmt.Errorf("failed to read used bytes percents: %w", err)
	}

	ms := make([]components.Metric, 0, len(totalBytes)+len(usedBytes)+len(usedPercents))
	for _, m := range totalBytes {
		ms = append(ms, components.Metric{Metric: m})
	}
	for _, m := range usedBytes {
		ms = append(ms, components.Metric{Metric: m})
	}
	for _, m := range usedPercents {
		ms = append(ms, components.Metric{Metric: m})
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
	log.Logger.Infow("checking memory")
	d := Data{
		ts: time.Now().UTC(),
	}
	metrics.SetLastUpdateUnixSeconds(float64(d.ts.Unix()))
	defer func() {
		c.lastMu.Lock()
		c.lastData = &d
		c.lastMu.Unlock()
	}()

	cctx, ccancel := context.WithTimeout(c.ctx, 5*time.Second)
	vm, err := mem.VirtualMemoryWithContext(cctx)
	ccancel()
	if err != nil {
		d.err = err
		return
	}

	d.TotalBytes = vm.Total
	d.AvailableBytes = vm.Available
	d.UsedBytes = vm.Used
	d.UsedPercent = fmt.Sprintf("%.2f", vm.UsedPercent)
	d.FreeBytes = vm.Free
	d.VMAllocTotalBytes = vm.VmallocTotal
	d.VMAllocUsedBytes = vm.VmallocUsed
	vmAllocUsedPercent := 0.0
	if vm.VmallocTotal > 0 {
		vmAllocUsedPercent = float64(vm.VmallocUsed) / float64(vm.VmallocTotal)
		vmAllocUsedPercent *= 100
	}
	d.VMAllocUsedPercent = fmt.Sprintf("%.2f", vmAllocUsedPercent)

	cctx, ccancel = context.WithTimeout(c.ctx, 5*time.Second)
	err = metrics.SetTotalBytes(cctx, float64(vm.Total), d.ts)
	ccancel()
	if err != nil {
		d.err = err
		return
	}

	metrics.SetAvailableBytes(float64(vm.Available))

	cctx, ccancel = context.WithTimeout(c.ctx, 5*time.Second)
	err = metrics.SetUsedBytes(cctx, float64(vm.Used), d.ts)
	ccancel()
	if err != nil {
		d.err = err
		return
	}

	cctx, ccancel = context.WithTimeout(c.ctx, 5*time.Second)
	err = metrics.SetUsedPercent(cctx, vm.UsedPercent, d.ts)
	ccancel()
	if err != nil {
		d.err = err
		return
	}

	metrics.SetFreeBytes(float64(vm.Free))

	cctx, ccancel = context.WithTimeout(c.ctx, 5*time.Second)
	bpfJITBufferBytes, err := getCurrentBPFJITBufferBytes(cctx)
	ccancel()
	if err != nil {
		d.err = err
		return
	}
	d.BPFJITBufferBytes = bpfJITBufferBytes
}

type Data struct {
	TotalBytes     uint64 `json:"total_bytes"`
	AvailableBytes uint64 `json:"available_bytes"`
	UsedBytes      uint64 `json:"used_bytes"`
	UsedPercent    string `json:"used_percent"`
	FreeBytes      uint64 `json:"free_bytes"`

	VMAllocTotalBytes  uint64 `json:"vm_alloc_total_bytes"`
	VMAllocUsedBytes   uint64 `json:"vm_alloc_used_bytes"`
	VMAllocUsedPercent string `json:"vm_alloc_used_percent"`

	// Represents the current BPF JIT buffer size in bytes.
	// ref. "cat /proc/vmallocinfo | grep bpf_jit | awk '{s+=$2} END {print s}'"
	BPFJITBufferBytes uint64 `json:"bpf_jit_buffer_bytes"`

	// timestamp of the last check
	ts time.Time `json:"-"`
	// error from the last check
	err error `json:"-"`
}

func (d *Data) getReason() string {
	if d == nil {
		return "no memory data"
	}
	if d.err != nil {
		return fmt.Sprintf("failed to get memory data -- %s", d.err)
	}
	return fmt.Sprintf("using %s out of total %s (%s %%)", humanize.Bytes(d.UsedBytes), humanize.Bytes(d.TotalBytes), d.UsedPercent)
}

func (d *Data) getHealth() (string, bool) {
	healthy := d == nil || d.err == nil
	health := components.StateHealthy
	if !healthy {
		health = components.StateUnhealthy
	}
	return health, healthy
}

func (d *Data) getStates() ([]components.State, error) {
	state := components.State{
		Name:   Name,
		Reason: d.getReason(),
	}
	state.Health, state.Healthy = d.getHealth()

	b, _ := json.Marshal(d)
	state.ExtraInfo = map[string]string{
		"data":     string(b),
		"encoding": "json",
	}
	return []components.State{state}, nil
}
