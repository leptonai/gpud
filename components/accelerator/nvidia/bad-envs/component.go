// Package badenvs tracks any bad environment variables that are globally set for the NVIDIA GPUs.
package badenvs

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/log"
)

const Name = "accelerator-nvidia-bad-envs"

var _ components.Component = &component{}

type component struct {
	ctx    context.Context
	cancel context.CancelFunc

	// returns true if the specified environment variable is set
	checkEnvFunc func(key string) bool

	lastMu   sync.RWMutex
	lastData *Data
}

func New(ctx context.Context) components.Component {
	cctx, ccancel := context.WithCancel(ctx)
	return &component{
		ctx:    cctx,
		cancel: ccancel,

		checkEnvFunc: func(key string) bool {
			return os.Getenv(key) == "1"
		},
	}
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

// CheckOnce checks the current pods
// run this periodically
func (c *component) CheckOnce() {
	log.Logger.Infow("checking memory")
	d := Data{
		ts: time.Now().UTC(),
	}
	defer func() {
		c.lastMu.Lock()
		c.lastData = &d
		c.lastMu.Unlock()
	}()

	foundBadEnvsForCUDA := make(map[string]string)
	for k, desc := range BAD_CUDA_ENV_KEYS {
		if c.checkEnvFunc(k) {
			foundBadEnvsForCUDA[k] = desc
		}
	}
	if len(foundBadEnvsForCUDA) > 0 {
		d.FoundBadEnvsForCUDA = foundBadEnvsForCUDA
	}

	d.healthy = true

	if len(foundBadEnvsForCUDA) == 0 {
		d.reason = "no bad envs found"
	} else {
		kvs := make([]string, 0, len(d.FoundBadEnvsForCUDA))
		for k, v := range d.FoundBadEnvsForCUDA {
			kvs = append(kvs, fmt.Sprintf("%s: %s", k, v))
		}
		d.reason = strings.Join(kvs, "; ")
	}
}

type Data struct {
	// FoundBadEnvsForCUDA is a map of environment variables that are known to hurt CUDA.
	// that is set globally for the host.
	// This implements "DCGM_FR_BAD_CUDA_ENV" logic in DCGM.
	FoundBadEnvsForCUDA map[string]string `json:"found_bad_envs_for_cuda"`

	// timestamp of the last check
	ts time.Time
	// error from the last check
	err error

	// tracks the healthy evaluation result of the last check
	healthy bool
	// tracks the reason of the last check
	reason string
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
				Name:    Name,
				Health:  apiv1.StateHealthy,
				Healthy: true,
				Reason:  "no data yet",
			},
		}, nil
	}

	state := apiv1.State{
		Name:   Name,
		Reason: d.reason,
		Error:  d.getError(),

		Healthy: d.healthy,
		Health:  apiv1.StateHealthy,
	}
	if !d.healthy {
		state.Health = apiv1.StateUnhealthy
	}

	b, _ := json.Marshal(d)
	state.ExtraInfo = map[string]string{
		"data":     string(b),
		"encoding": "json",
	}
	return []apiv1.State{state}, nil
}
