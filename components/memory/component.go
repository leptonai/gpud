// Package memory tracks the memory usage of the host.
package memory

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/olekukonko/tablewriter"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/shirou/gopsutil/v4/mem"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

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

		getVirtualMemoryFunc:            mem.VirtualMemoryWithContext,
		getCurrentBPFJITBufferBytesFunc: getCurrentBPFJITBufferBytes,
	}

	if gpudInstance.EventStore != nil {
		var err error
		c.eventBucket, err = gpudInstance.EventStore.Bucket(Name)
		if err != nil {
			ccancel()
			return nil, err
		}

		if os.Geteuid() == 0 {
			c.kmsgSyncer, err = kmsg.NewSyncer(cctx, createMatchFunc(), c.eventBucket)
			if err != nil {
				ccancel()
				return nil, err
			}
		}
	}

	return c, nil
}

func (c *component) Name() string { return Name }

func (c *component) Tags() []string {
	return []string{
		Name,
	}
}

func (c *component) IsSupported() bool {
	return true
}

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
	evs, err := c.eventBucket.Get(ctx, since)
	if err != nil {
		return nil, err
	}
	return evs.Events(), nil
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

func (c *component) Check() components.CheckResult {
	log.Logger.Infow("checking memory")

	cr := &checkResult{
		ts: time.Now().UTC(),
	}

	defer func() {
		c.lastMu.Lock()
		c.lastCheckResult = cr
		c.lastMu.Unlock()
	}()

	cctx, ccancel := context.WithTimeout(c.ctx, 5*time.Second)
	vm, err := c.getVirtualMemoryFunc(cctx)
	ccancel()
	if err != nil {
		cr.err = err
		cr.health = apiv1.HealthStateTypeUnhealthy
		cr.reason = "error getting virtual memory"
		log.Logger.Warnw(cr.reason, "error", cr.err)
		return cr
	}

	cr.TotalBytes = vm.Total
	cr.AvailableBytes = vm.Available
	cr.UsedBytes = vm.Used
	cr.UsedPercent = fmt.Sprintf("%.2f", vm.UsedPercent)
	cr.FreeBytes = vm.Free
	cr.VMAllocTotalBytes = vm.VmallocTotal
	cr.VMAllocUsedBytes = vm.VmallocUsed

	metricTotalBytes.With(prometheus.Labels{}).Set(float64(vm.Total))
	metricAvailableBytes.With(prometheus.Labels{}).Set(float64(vm.Available))
	metricUsedBytes.With(prometheus.Labels{}).Set(float64(vm.Used))
	metricUsedPercent.With(prometheus.Labels{}).Set(vm.UsedPercent)
	metricFreeBytes.With(prometheus.Labels{}).Set(float64(vm.Free))

	if c.getCurrentBPFJITBufferBytesFunc != nil {
		bpfJITBufferBytes, err := c.getCurrentBPFJITBufferBytesFunc()
		if err != nil {
			cr.err = err
			cr.health = apiv1.HealthStateTypeUnhealthy
			cr.reason = "error getting bpf jit buffer bytes"
			log.Logger.Warnw(cr.reason, "error", cr.err)
			return cr
		}
		cr.BPFJITBufferBytes = bpfJITBufferBytes
	}

	cr.health = apiv1.HealthStateTypeHealthy
	cr.reason = "ok"
	log.Logger.Debugw(cr.reason, "used", humanize.Bytes(cr.UsedBytes), "total", humanize.Bytes(cr.TotalBytes))

	return cr
}

var _ components.CheckResult = &checkResult{}

type checkResult struct {
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

func (cr *checkResult) ComponentName() string {
	return Name
}

func (cr *checkResult) String() string {
	if cr == nil {
		return ""
	}

	buf := bytes.NewBuffer(nil)
	table := tablewriter.NewWriter(buf)
	table.SetAlignment(tablewriter.ALIGN_CENTER)
	table.Append([]string{"Total", humanize.Bytes(cr.TotalBytes)})
	table.Append([]string{"Used", humanize.Bytes(cr.UsedBytes)})
	table.Append([]string{"Used %", cr.UsedPercent + " %"})
	table.Append([]string{"Available", humanize.Bytes(cr.AvailableBytes)})
	if runtime.GOOS == "linux" {
		table.Append([]string{"BPF JIT Buffer", humanize.Bytes(cr.BPFJITBufferBytes)})
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
	state.ExtraInfo = map[string]string{"data": string(b)}
	return apiv1.HealthStates{state}
}
