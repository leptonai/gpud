// Package memory tracks the memory usage of the host.
package memory

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"runtime"
	"sync"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/olekukonko/tablewriter"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/shirou/gopsutil/v4/mem"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
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

	getVirtualMemoryFunc            func(context.Context) (*mem.VirtualMemoryStat, error)
	getCurrentBPFJITBufferBytesFunc func() (uint64, error)

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

		getVirtualMemoryFunc:            mem.VirtualMemoryWithContext,
		getCurrentBPFJITBufferBytesFunc: getCurrentBPFJITBufferBytes,

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

// CheckOnce checks the current pods
// run this periodically
func (c *component) CheckOnce() {
	log.Logger.Infow("checking memory")

	d := checkHealthState(
		c.ctx,
		c.getVirtualMemoryFunc,
		c.getCurrentBPFJITBufferBytesFunc,
	)

	c.lastMu.Lock()
	c.lastData = d
	c.lastMu.Unlock()
}

func CheckHealthState(ctx context.Context) (components.HealthStateCheckResult, error) {
	d := checkHealthState(
		ctx,
		mem.VirtualMemoryWithContext,
		getCurrentBPFJITBufferBytes,
	)
	if d.err != nil {
		return nil, d.err
	}
	return d, nil
}

func checkHealthState(
	ctx context.Context,
	getVirtualMemoryFunc func(context.Context) (*mem.VirtualMemoryStat, error),
	getCurrentBPFJITBufferBytesFunc func() (uint64, error),
) *Data {
	d := &Data{
		ts: time.Now().UTC(),
	}

	cctx, ccancel := context.WithTimeout(ctx, 5*time.Second)
	vm, err := getVirtualMemoryFunc(cctx)
	ccancel()
	if err != nil {
		log.Logger.Errorw("failed to get virtual memory", "error", err)
		d.err = err
		d.health = apiv1.StateTypeUnhealthy
		d.reason = fmt.Sprintf("failed to get virtual memory: %s", err)
		return d
	}

	d.TotalBytes = vm.Total
	d.AvailableBytes = vm.Available
	d.UsedBytes = vm.Used
	d.UsedPercent = fmt.Sprintf("%.2f", vm.UsedPercent)
	d.FreeBytes = vm.Free
	d.VMAllocTotalBytes = vm.VmallocTotal
	d.VMAllocUsedBytes = vm.VmallocUsed

	metricTotalBytes.With(prometheus.Labels{}).Set(float64(vm.Total))
	metricAvailableBytes.With(prometheus.Labels{}).Set(float64(vm.Available))
	metricUsedBytes.With(prometheus.Labels{}).Set(float64(vm.Used))
	metricUsedPercent.With(prometheus.Labels{}).Set(vm.UsedPercent)
	metricFreeBytes.With(prometheus.Labels{}).Set(float64(vm.Free))

	bpfJITBufferBytes, err := getCurrentBPFJITBufferBytesFunc()
	if err != nil {
		log.Logger.Errorw("failed to get bpf jit buffer bytes", "error", err)
		d.err = err
		d.health = apiv1.StateTypeUnhealthy
		d.reason = fmt.Sprintf("failed to get bpf jit buffer bytes: %s", err)
		return d
	}
	d.BPFJITBufferBytes = bpfJITBufferBytes

	d.health = apiv1.StateTypeHealthy
	d.reason = fmt.Sprintf("using %s out of total %s", humanize.Bytes(d.UsedBytes), humanize.Bytes(d.TotalBytes))

	return d
}

var _ components.HealthStateCheckResult = &Data{}

type Data struct {
	TotalBytes     uint64 `json:"total_bytes"`
	AvailableBytes uint64 `json:"available_bytes"`
	UsedBytes      uint64 `json:"used_bytes"`
	UsedPercent    string `json:"used_percent"`
	FreeBytes      uint64 `json:"free_bytes"`

	VMAllocTotalBytes uint64 `json:"vm_alloc_total_bytes"`
	VMAllocUsedBytes  uint64 `json:"vm_alloc_used_bytes"`

	// Represents the current BPF JIT buffer size in bytes.
	// ref. "cat /proc/vmallocinfo | grep bpf_jit | awk '{s+=$2} END {print s}'"
	// Useful to debug "failed to create shim task: OCI" due to insufficient BPF JIT buffer.
	// ref. https://github.com/awslabs/amazon-eks-ami/issues/1179
	// ref. https://github.com/deckhouse/deckhouse/issues/7402
	BPFJITBufferBytes uint64 `json:"bpf_jit_buffer_bytes"`

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
	table.Append([]string{"Total", humanize.Bytes(d.TotalBytes)})
	table.Append([]string{"Used", humanize.Bytes(d.UsedBytes)})
	table.Append([]string{"Used %", d.UsedPercent + " %"})
	table.Append([]string{"Available", humanize.Bytes(d.AvailableBytes)})
	if runtime.GOOS == "linux" {
		table.Append([]string{"BPF JIT Buffer", humanize.Bytes(d.BPFJITBufferBytes)})
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
