// Package badenvs tracks any bad environment variables that are globally set for the NVIDIA GPUs.
package badenvs

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/olekukonko/tablewriter"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/log"
	nvidianvml "github.com/leptonai/gpud/pkg/nvidia-query/nvml"
)

const Name = "accelerator-nvidia-bad-envs"

var _ components.Component = &component{}

type component struct {
	ctx    context.Context
	cancel context.CancelFunc

	nvmlInstance nvidianvml.Instance

	// returns true if the specified environment variable is set
	checkEnvFunc func(key string) bool

	lastMu          sync.RWMutex
	lastCheckResult *checkResult
}

func New(gpudInstance *components.GPUdInstance) (components.Component, error) {
	cctx, ccancel := context.WithCancel(gpudInstance.RootCtx)
	c := &component{
		ctx:          cctx,
		cancel:       ccancel,
		nvmlInstance: gpudInstance.NVMLInstance,
		checkEnvFunc: func(key string) bool {
			return os.Getenv(key) == "1"
		},
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
	return nil, nil
}

func (c *component) Close() error {
	log.Logger.Debugw("closing component")

	c.cancel()

	return nil
}

// ports "DCGM_FR_BAD_CUDA_ENV"; The environment has variables that hurt CUDA.
// This is derived from "DCGM_FR_BAD_CUDA_ENV" in DCGM.
// ref. https://github.com/NVIDIA/DCGM/blob/903d745504f50153be8293f8566346f9de3b3c93/nvvs/plugin_src/software/Software.cpp#L839-L876
var BAD_CUDA_ENV_KEYS = map[string]string{
	"NSIGHT_CUDA_DEBUGGER":              "Setting NSIGHT_CUDA_DEBUGGER=1 can degrade the performance of an application, since the debugger is made resident. See https://docs.nvidia.com/nsight-visual-studio-edition/3.2/Content/Attach_CUDA_to_Process.htm.",
	"CUDA_INJECTION32_PATH":             "Captures information about CUDA execution trace. See https://docs.nvidia.com/nsight-systems/2020.3/tracing/index.html.",
	"CUDA_INJECTION64_PATH":             "Captures information about CUDA execution trace. See https://docs.nvidia.com/nsight-systems/2020.3/tracing/index.html.",
	"CUDA_AUTO_BOOST":                   "Automatically selects the highest possible clock rate allowed by the thermal and power budget. Independent of the global default setting the autoboost behavior can be overridden by setting the environment variable CUDA_AUTO_BOOST. Set CUDA_AUTO_BOOST=0 to disable frequency throttling/boosting. You may run 'nvidia-smi --auto-boost-default=0' to disable autoboost by default. See https://developer.nvidia.com/blog/increase-performance-gpu-boost-k80-autoboost/.",
	"CUDA_ENABLE_COREDUMP_ON_EXCEPTION": "Enables GPU core dumps.",
	"CUDA_COREDUMP_FILE":                "Enables GPU core dumps.",
	"CUDA_DEVICE_WAITS_ON_EXCEPTION":    "CUDA kernel will pause when an exception occurs. This is only useful for debugging.",
	"CUDA_PROFILE":                      "Enables CUDA profiling.",
	"COMPUTE_PROFILE":                   "Enables compute profiling.",
	"OPENCL_PROFILE":                    "Enables OpenCL profiling.",
}

func (c *component) Check() components.CheckResult {
	log.Logger.Infow("checking nvidia gpu bad env variables")

	cr := &checkResult{
		ts: time.Now().UTC(),
	}
	defer func() {
		c.lastMu.Lock()
		c.lastCheckResult = cr
		c.lastMu.Unlock()
	}()

	if c.nvmlInstance == nil {
		cr.health = apiv1.HealthStateTypeHealthy
		cr.reason = "NVIDIA NVML instance is nil"
		return cr
	}
	if !c.nvmlInstance.NVMLExists() {
		cr.health = apiv1.HealthStateTypeHealthy
		cr.reason = "NVIDIA NVML library is not loaded"
		return cr
	}
	if c.nvmlInstance.ProductName() == "" {
		cr.health = apiv1.HealthStateTypeHealthy
		cr.reason = "NVIDIA NVML is loaded but GPU is not detected (missing product name)"
		return cr
	}

	foundBadEnvsForCUDA := make(map[string]string)
	for k, desc := range BAD_CUDA_ENV_KEYS {
		if c.checkEnvFunc(k) {
			foundBadEnvsForCUDA[k] = desc
		}
	}
	if len(foundBadEnvsForCUDA) > 0 {
		cr.FoundBadEnvsForCUDA = foundBadEnvsForCUDA
	}

	if len(foundBadEnvsForCUDA) == 0 {
		cr.reason = "no bad envs found"
	} else {
		kvs := make([]string, 0, len(cr.FoundBadEnvsForCUDA))
		for k, v := range cr.FoundBadEnvsForCUDA {
			kvs = append(kvs, fmt.Sprintf("%s: %s", k, v))
		}
		cr.reason = strings.Join(kvs, "; ")
	}

	cr.health = apiv1.HealthStateTypeHealthy
	return cr
}

var _ components.CheckResult = &checkResult{}

type checkResult struct {
	// FoundBadEnvsForCUDA is a map of environment variables that are known to hurt CUDA.
	// that is set globally for the host.
	// This implements "DCGM_FR_BAD_CUDA_ENV" logic in DCGM.
	FoundBadEnvsForCUDA map[string]string `json:"found_bad_envs_for_cuda"`

	// timestamp of the last check
	ts time.Time
	// error from the last check
	err error

	// tracks the healthy evaluation result of the last check
	health apiv1.HealthStateType
	// tracks the reason of the last check
	reason string
}

func (cr *checkResult) String() string {
	if cr == nil {
		return ""
	}
	if len(cr.FoundBadEnvsForCUDA) == 0 {
		return "no bad envs found"
	}

	buf := bytes.NewBuffer(nil)
	table := tablewriter.NewWriter(buf)
	table.SetAlignment(tablewriter.ALIGN_CENTER)
	table.SetHeader([]string{"Found Env Key", "Description"})
	for k, v := range cr.FoundBadEnvsForCUDA {
		table.Append([]string{k, v})
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
				Component: Name,
				Name:      Name,
				Health:    apiv1.HealthStateTypeHealthy,
				Reason:    "no data yet",
			},
		}
	}

	state := apiv1.HealthState{
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
