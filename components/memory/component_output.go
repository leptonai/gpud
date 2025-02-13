package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/leptonai/gpud/components"
	memory_id "github.com/leptonai/gpud/components/memory/id"
	"github.com/leptonai/gpud/components/memory/metrics"
	components_metrics "github.com/leptonai/gpud/pkg/gpud-metrics"
	"github.com/leptonai/gpud/pkg/query"

	"github.com/dustin/go-humanize"
	"github.com/shirou/gopsutil/v4/mem"
)

type Output struct {
	TotalBytes     uint64 `json:"total_bytes"`
	TotalHumanized string `json:"total_humanized"`

	AvailableBytes     uint64 `json:"available_bytes"`
	AvailableHumanized string `json:"available_humanized"`

	UsedBytes     uint64 `json:"used_bytes"`
	UsedHumanized string `json:"used_humanized"`

	UsedPercent string `json:"used_percent"`

	FreeBytes     uint64 `json:"free_bytes"`
	FreeHumanized string `json:"free_humanized"`

	VMAllocTotalBytes     uint64 `json:"vm_alloc_total_bytes"`
	VMAllocTotalHumanized string `json:"vm_alloc_total_humanized"`
	VMAllocUsedBytes      uint64 `json:"vm_alloc_used_bytes"`
	VMAllocUsedHumanized  string `json:"vm_alloc_used_humanized"`
	VMAllocUsedPercent    string `json:"vm_alloc_used_percent"`

	// Represents the current BPF JIT buffer size in bytes.
	// ref. "cat /proc/vmallocinfo | grep bpf_jit | awk '{s+=$2} END {print s}'"
	BPFJITBufferBytes     uint64 `json:"bpf_jit_buffer_bytes"`
	BPFJITBufferHumanized string `json:"bpf_jit_buffer_humanized"`
}

func (o Output) GetUsedPercent() (float64, error) {
	return strconv.ParseFloat(o.UsedPercent, 64)
}

func (o *Output) JSON() ([]byte, error) {
	return json.Marshal(o)
}

func ParseOutputJSON(data []byte) (*Output, error) {
	o := new(Output)
	if err := json.Unmarshal(data, o); err != nil {
		return nil, err
	}
	return o, nil
}

const (
	StateKeyVirtualMemory = "virtual_memory"

	StateKeyTotalBytes         = "total_bytes"
	StateKeyTotalHumanized     = "total_humanized"
	StateKeyAvailableBytes     = "available_bytes"
	StateKeyAvailableHumanized = "available_humanized"
	StateKeyUsedBytes          = "used_bytes"
	StateKeyUsedHumanized      = "used_humanized"
	StateKeyUsedPercent        = "used_percent"
	StateKeyFreeBytes          = "free_bytes"
	StateKeyFreeHumanized      = "free_humanized"

	StateKeyVMAllocTotalBytes     = "vm_alloc_total_bytes"
	StateKeyVMAllocTotalHumanized = "vm_alloc_total_humanized"
	StateKeyVMAllocUsedBytes      = "vm_alloc_used_bytes"
	StateKeyVMAllocUsedHumanized  = "vm_alloc_used_humanized"
	StateKeyVMAllocUsedPercent    = "vm_alloc_used_percent"

	StateKeyBPFJITBufferBytes     = "bpf_jit_buffer_bytes"
	StateKeyBPFJITBufferHumanized = "bpf_jit_buffer_humanized"
)

func ParseStateKeyVirtualMemory(m map[string]string) (*Output, error) {
	o := &Output{}

	var err error
	o.TotalBytes, err = strconv.ParseUint(m[StateKeyTotalBytes], 10, 64)
	if err != nil {
		return nil, err
	}
	o.TotalHumanized = m[StateKeyTotalHumanized]

	o.AvailableBytes, err = strconv.ParseUint(m[StateKeyAvailableBytes], 10, 64)
	if err != nil {
		return nil, err
	}
	o.AvailableHumanized = m[StateKeyAvailableHumanized]

	o.UsedBytes, err = strconv.ParseUint(m[StateKeyUsedBytes], 10, 64)
	if err != nil {
		return nil, err
	}
	o.UsedHumanized = m[StateKeyUsedHumanized]

	o.UsedPercent = m[StateKeyUsedPercent]

	o.FreeBytes, err = strconv.ParseUint(m[StateKeyFreeBytes], 10, 64)
	if err != nil {
		return nil, err
	}
	o.FreeHumanized = m[StateKeyFreeHumanized]

	o.VMAllocTotalBytes, err = strconv.ParseUint(m[StateKeyVMAllocTotalBytes], 10, 64)
	if err != nil {
		return nil, err
	}
	o.VMAllocTotalHumanized = m[StateKeyVMAllocTotalHumanized]

	o.VMAllocUsedBytes, err = strconv.ParseUint(m[StateKeyVMAllocUsedBytes], 10, 64)
	if err != nil {
		return nil, err
	}
	o.VMAllocUsedHumanized = m[StateKeyVMAllocUsedHumanized]

	o.VMAllocUsedPercent = m[StateKeyVMAllocUsedPercent]

	o.BPFJITBufferBytes, err = strconv.ParseUint(m[StateKeyBPFJITBufferBytes], 10, 64)
	if err != nil {
		return nil, err
	}
	o.BPFJITBufferHumanized = m[StateKeyBPFJITBufferHumanized]

	return o, nil
}

func ParseStatesToOutput(states ...components.State) (*Output, error) {
	for _, state := range states {
		switch state.Name {
		case StateKeyVirtualMemory:
			return ParseStateKeyVirtualMemory(state.ExtraInfo)

		default:
			return nil, fmt.Errorf("unknown state name: %s", state.Name)
		}
	}
	return nil, fmt.Errorf("no state found")
}

func (o *Output) States() ([]components.State, error) {
	state := components.State{
		Name:    StateKeyVirtualMemory,
		Healthy: true,
		Reason:  fmt.Sprintf("using %s out of total %s (%s %%)", o.UsedHumanized, o.TotalHumanized, o.UsedPercent),
		ExtraInfo: map[string]string{
			StateKeyTotalBytes:            fmt.Sprintf("%d", o.TotalBytes),
			StateKeyTotalHumanized:        o.TotalHumanized,
			StateKeyAvailableBytes:        fmt.Sprintf("%d", o.AvailableBytes),
			StateKeyAvailableHumanized:    o.AvailableHumanized,
			StateKeyUsedBytes:             fmt.Sprintf("%d", o.UsedBytes),
			StateKeyUsedHumanized:         o.UsedHumanized,
			StateKeyUsedPercent:           o.UsedPercent,
			StateKeyFreeBytes:             fmt.Sprintf("%d", o.FreeBytes),
			StateKeyFreeHumanized:         o.FreeHumanized,
			StateKeyVMAllocTotalBytes:     fmt.Sprintf("%d", o.VMAllocTotalBytes),
			StateKeyVMAllocTotalHumanized: o.VMAllocTotalHumanized,
			StateKeyVMAllocUsedBytes:      fmt.Sprintf("%d", o.VMAllocUsedBytes),
			StateKeyVMAllocUsedHumanized:  o.VMAllocUsedHumanized,
			StateKeyVMAllocUsedPercent:    o.VMAllocUsedPercent,
			StateKeyBPFJITBufferBytes:     fmt.Sprintf("%d", o.BPFJITBufferBytes),
			StateKeyBPFJITBufferHumanized: o.BPFJITBufferHumanized,
		},
	}
	return []components.State{state}, nil
}

var (
	defaultPollerOnce sync.Once
	defaultPoller     query.Poller
)

// only set once since it relies on the kube client and specific port
func setDefaultPoller(cfg Config) {
	defaultPollerOnce.Do(func() {
		defaultPoller = query.New(
			memory_id.Name,
			cfg.Query,
			Get,
			nil,
		)
	})
}

func getDefaultPoller() query.Poller {
	return defaultPoller
}

func Get(ctx context.Context) (_ any, e error) {
	defer func() {
		if e != nil {
			components_metrics.SetGetFailed(memory_id.Name)
		} else {
			components_metrics.SetGetSuccess(memory_id.Name)
		}
	}()

	vm, err := mem.VirtualMemoryWithContext(ctx)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	nowUnix := float64(now.Unix())
	metrics.SetLastUpdateUnixSeconds(nowUnix)
	if err := metrics.SetTotalBytes(ctx, float64(vm.Total), now); err != nil {
		return nil, err
	}
	metrics.SetAvailableBytes(float64(vm.Available))
	if err := metrics.SetUsedBytes(ctx, float64(vm.Used), now); err != nil {
		return nil, err
	}
	if err := metrics.SetUsedPercent(ctx, vm.UsedPercent, now); err != nil {
		return nil, err
	}
	metrics.SetFreeBytes(float64(vm.Free))

	vmAllocUsedPercent := 0.0
	if vm.VmallocTotal > 0 {
		vmAllocUsedPercent = float64(vm.VmallocUsed) / float64(vm.VmallocTotal)
		vmAllocUsedPercent *= 100
	}

	bpfJITBufferBytes, err := getCurrentBPFJITBufferBytes(ctx)
	if err != nil {
		return nil, err
	}

	return &Output{
		TotalBytes:            vm.Total,
		TotalHumanized:        humanize.Bytes(vm.Total),
		AvailableBytes:        vm.Available,
		AvailableHumanized:    humanize.Bytes(vm.Available),
		UsedBytes:             vm.Used,
		UsedHumanized:         humanize.Bytes(vm.Used),
		UsedPercent:           fmt.Sprintf("%.2f", vm.UsedPercent),
		FreeBytes:             vm.Free,
		FreeHumanized:         humanize.Bytes(vm.Free),
		VMAllocTotalBytes:     vm.VmallocTotal,
		VMAllocTotalHumanized: humanize.Bytes(vm.VmallocTotal),
		VMAllocUsedBytes:      vm.VmallocUsed,
		VMAllocUsedHumanized:  humanize.Bytes(vm.VmallocUsed),
		VMAllocUsedPercent:    fmt.Sprintf("%.2f", vmAllocUsedPercent),
		BPFJITBufferBytes:     bpfJITBufferBytes,
		BPFJITBufferHumanized: humanize.Bytes(bpfJITBufferBytes),
	}, nil
}
