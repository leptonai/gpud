package containerd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/pelletier/go-toml/v2"

	"github.com/leptonai/gpud/pkg/log"
)

const nvidiaRuntimeName = "nvidia"

// containerdConfigFile is the part of a containerd config that decides
// whether workloads can reach the NVIDIA runtime. Decoding the TOML instead of
// matching text accepts every key quoting style: the k3s/RKE2 templates write
// runtimes."nvidia" (version 2) and plugins.'io.containerd.cri.v1.runtime'
// with runtimes.'nvidia' (version 3).
type containerdConfigFile struct {
	Version int      `toml:"version"`
	Imports []string `toml:"imports"`
	Plugins struct {
		// containerd 1.x and config version 2.
		CRI criRuntimePlugin `toml:"io.containerd.grpc.v1.cri"`
		// containerd 2.x config version 3.
		CRIRuntime criRuntimePlugin `toml:"io.containerd.cri.v1.runtime"`
	} `toml:"plugins"`
}

type criRuntimePlugin struct {
	EnableCDI  *bool `toml:"enable_cdi"`
	Containerd struct {
		DefaultRuntimeName string         `toml:"default_runtime_name"`
		Runtimes           map[string]any `toml:"runtimes"`
	} `toml:"containerd"`
}

// nvidiaRuntimeConfigured reads the selected containerd config and reports
// whether it registers the NVIDIA runtime handler and makes it usable.
func (c *component) nvidiaRuntimeConfigured() (bool, error) {
	config, err := c.getContainerdConfigFunc()
	if err != nil {
		return false, err
	}
	return hasNvidiaRuntimeConfiguration(config, c.getRuntimeConfigFunc)
}

// hasNvidiaRuntimeConfiguration accepts both supported toolkit layouts:
// NVIDIA as containerd's default runtime, or native CDI enabled with an
// NVIDIA runtime handler. The handler is required in both cases. Files listed
// in "imports" (e.g. drop-ins written by nvidia-container-toolkit) are merged
// after the main config, so their explicit settings win, as in containerd.
// Unreadable or invalid imports are skipped. Live status resolves the CDI
// default when a version 2 config could belong to containerd 1.x or 2.x.
func hasNvidiaRuntimeConfiguration(config []byte, getRuntimeConfig func() ([]byte, error)) (bool, error) {
	var main containerdConfigFile
	if err := toml.Unmarshal(config, &main); err != nil {
		return false, fmt.Errorf("failed to parse containerd config: %w", err)
	}
	files := []containerdConfigFile{main}
	for _, pattern := range main.Imports {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			log.Logger.Warnw("invalid containerd config import", "pattern", pattern, "error", err)
			continue
		}
		for _, path := range matches {
			b, err := os.ReadFile(path)
			if err != nil {
				log.Logger.Warnw("failed to read containerd config import", "path", path, "error", err)
				continue
			}
			var imported containerdConfigFile
			if err := toml.Unmarshal(b, &imported); err != nil {
				log.Logger.Warnw("failed to parse containerd config import", "path", path, "error", err)
				continue
			}
			files = append(files, imported)
		}
	}

	var cdi *bool
	handler := false
	defaultRuntime := ""
	for _, f := range files {
		for _, plugin := range []criRuntimePlugin{f.Plugins.CRI, f.Plugins.CRIRuntime} {
			if _, ok := plugin.Containerd.Runtimes[nvidiaRuntimeName]; ok {
				handler = true
			}
			if plugin.Containerd.DefaultRuntimeName != "" {
				defaultRuntime = plugin.Containerd.DefaultRuntimeName
			}
			if plugin.EnableCDI != nil {
				cdi = plugin.EnableCDI
			}
		}
	}
	if !handler {
		return false, nil
	}
	if defaultRuntime == nvidiaRuntimeName {
		return true, nil
	}
	if cdi != nil {
		return *cdi, nil
	}
	// Only containerd 2.x reads version 3, but it can also migrate version 2
	// without changing its enabled CDI default. Version 2 alone is ambiguous.
	if main.Version >= 3 {
		return true, nil
	}
	if getRuntimeConfig == nil {
		return false, nil
	}
	data, err := getRuntimeConfig()
	if err != nil {
		return false, fmt.Errorf("get containerd CDI default: %w", err)
	}
	live, err := parseRuntimeConfig(data)
	if err != nil {
		return false, err
	}
	return live.EnableCDI, nil
}
