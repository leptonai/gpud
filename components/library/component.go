// Package library provides a component that returns healthy if and only if all the specified libraries exist.
package library

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"sigs.k8s.io/yaml"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/file"
	"github.com/leptonai/gpud/pkg/log"
	nvidianvml "github.com/leptonai/gpud/pkg/nvidia-query/nvml"
)

// Name is the name of the library component.
const Name = "library"

var (
	defaultNVIDIALibraries = map[string][]string{
		// core library for NVML
		// typically symlinked to "libnvidia-ml.so.1" or "libnvidia-ml.so.535.183.06" (or other driver versions)
		// some latest drivers do not have this symlink, only "libnvidia-ml.so.1"
		"libnvidia-ml.so": {
			// core library for NVML
			// typically symlinked to "libnvidia-ml.so.565.57.01" (or other driver versions)
			// some latest drivers do not have this "libnvidia-ml.so" symlink, only "libnvidia-ml.so.1"
			"libnvidia-ml.so.1",
		},

		// core library for CUDA support
		// typically symlinked to "libcuda.so.1" or "libcuda.so.535.183.06"
		"libcuda.so": {
			"libcuda.so.1",
		},
	}

	defaultNVIDIALibrariesSearchDirs = []string{
		// ref. https://github.com/NVIDIA/nvidia-container-toolkit/blob/main/internal/lookup/library.go#L33-L62
		"/",
		"/usr/lib64",
		"/usr/lib/x86_64-linux-gnu",
		"/usr/lib/aarch64-linux-gnu",
		"/usr/lib/x86_64-linux-gnu/nvidia/current",
		"/usr/lib/aarch64-linux-gnu/nvidia/current",
		"/lib64",
		"/lib/x86_64-linux-gnu",
		"/lib/aarch64-linux-gnu",
		"/lib/x86_64-linux-gnu/nvidia/current",
		"/lib/aarch64-linux-gnu/nvidia/current",
	}
)

var _ components.Component = &component{}

type component struct {
	ctx    context.Context
	cancel context.CancelFunc

	nvmlInstance nvidianvml.InstanceV2

	libraries   map[string][]string
	searchOpts  []file.OpOption
	findLibrary func(string, ...file.OpOption) (string, error)

	lastMu   sync.RWMutex
	lastData *Data
}

func New(gpudInstance *components.GPUdInstance) (components.Component, error) {
	cctx, ccancel := context.WithCancel(context.Background())
	c := &component{
		ctx:          cctx,
		cancel:       ccancel,
		nvmlInstance: gpudInstance.NVMLInstance,
		findLibrary:  file.FindLibrary,
	}

	searchDirs := make(map[string]any)
	if c.nvmlInstance != nil && c.nvmlInstance.NVMLExists() {
		c.libraries = defaultNVIDIALibraries
		for _, dir := range defaultNVIDIALibrariesSearchDirs {
			searchDirs[dir] = struct{}{}
		}
	}

	searchOpts := []file.OpOption{}
	for dir := range searchDirs {
		searchOpts = append(searchOpts, file.WithSearchDirs(dir))
	}
	c.searchOpts = searchOpts

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
	lastData := c.lastData
	c.lastMu.RUnlock()
	return lastData.getLastHealthStates()
}

func (c *component) Events(ctx context.Context, since time.Time) (apiv1.Events, error) {
	return nil, nil
}

func (c *component) Close() error {
	log.Logger.Debugw("closing component")

	c.cancel()

	return nil
}

func (c *component) Check() components.CheckResult {
	log.Logger.Infow("checking library")

	d := &Data{
		ts: time.Now().UTC(),
	}
	defer func() {
		c.lastMu.Lock()
		c.lastData = d
		c.lastMu.Unlock()
	}()

	notFounds := []string{}
	for lib, alternatives := range c.libraries {
		opts := []file.OpOption{}
		opts = append(opts, c.searchOpts...)
		for _, alt := range alternatives {
			opts = append(opts, file.WithAlternativeLibraryName(alt))
		}
		resolved, err := c.findLibrary(lib, opts...)
		if resolved == "" && errors.Is(err, file.ErrLibraryNotFound) {
			notFounds = append(notFounds, fmt.Sprintf("library %q does not exist", lib))
			continue
		}
		if err != nil {
			d.health = apiv1.HealthStateTypeUnhealthy
			d.err = err
			return d
		}

		d.ResolvedLibraries = append(d.ResolvedLibraries, resolved)
		log.Logger.Debugw("found library", "library", lib, "resolved", resolved)
	}
	sort.Strings(d.ResolvedLibraries)
	sort.Strings(notFounds)

	if len(notFounds) > 0 {
		d.health = apiv1.HealthStateTypeUnhealthy
		d.reason = strings.Join(notFounds, "; ")
		return d
	}

	d.health = apiv1.HealthStateTypeHealthy
	d.reason = "all libraries exist"
	return d
}

var _ components.CheckResult = &Data{}

type Data struct {
	ResolvedLibraries []string `json:"resolved_libraries"`

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

	b, err := yaml.Marshal(d)
	if err != nil {
		return fmt.Sprintf("error marshaling data: %v", err)
	}
	return string(b)
}

func (d *Data) Summary() string {
	if d == nil {
		return ""
	}
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

func (d *Data) getLastHealthStates() apiv1.HealthStates {
	if d == nil {
		return apiv1.HealthStates{
			{
				Name:   Name,
				Health: apiv1.HealthStateTypeHealthy,
				Reason: "no data yet",
			},
		}
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
	return apiv1.HealthStates{state}
}
