// Package config provides the gpud configuration data for the server.
package config

import (
	"errors"
	"fmt"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/leptonai/gpud/components"
	pkgconfigcommon "github.com/leptonai/gpud/pkg/config/common"
)

// Config provides gpud configuration data for the server
type Config struct {
	APIVersion string `json:"api_version"`

	// Address for the server to listen on.
	Address string `json:"address"`

	// DataDir is the root directory for GPUd state and package artifacts.
	DataDir string `json:"data_dir"`

	// State file that persists the latest status.
	// If empty, the states are not persisted to file.
	State string `json:"state"`

	// Amount of time to retain states/metrics for.
	// Once elapsed, old states/metrics are purged/compacted.
	RetentionPeriod metav1.Duration `json:"retention_period"`

	// Interval at which to compact the state database.
	CompactPeriod metav1.Duration `json:"compact_period"`

	// Set true to enable profiler.
	Pprof bool `json:"pprof"`

	// Set false to disable auto update
	EnableAutoUpdate bool `json:"enable_auto_update"`

	// Exit code to exit with when auto updating.
	// Only valid when the auto update is enabled.
	// Set -1 to disable the auto update by exit code.
	AutoUpdateExitCode int `json:"auto_update_exit_code"`

	// VersionFile is the file that contains the target version.
	// If empty, the version file is not used.
	VersionFile string `json:"version_file"`

	// A list of nvidia tool command paths to overwrite the default paths.
	NvidiaToolOverwrites pkgconfigcommon.ToolOverwrites `json:"nvidia_tool_overwrites"`

	// PluginSpecsFile is the file that contains the plugin specs.
	PluginSpecsFile string `json:"plugin_specs_file"`

	// Components specifies the components to enable.
	// Leave empty, "*", or "all" to enable all components.
	// Or prefix component names with "-" to disable them.
	Components         []string       `json:"components"`
	selectedComponents map[string]any `json:"-"`
	disabledComponents map[string]any `json:"-"`

	// FailureInjector is the failure injector.
	FailureInjector *components.FailureInjector `json:"failure_injector,omitempty"`
}

func (config *Config) Validate() error {
	if config.Address == "" {
		return errors.New("address is required")
	}
	if config.RetentionPeriod.Duration < time.Minute {
		return fmt.Errorf("retention_period must be at least 1 minute, got %d", config.RetentionPeriod.Duration)
	}

	return nil
}

// ShouldEnable returns true if the component should be enabled.
// If the enable component sets are not specified, it will return true,
// meaning it should be enabled by default.
func (config *Config) ShouldEnable(componentName string) bool {
	// not specified, thus enable all components
	if len(config.Components) == 0 || config.Components[0] == "*" || config.Components[0] == "all" {
		return true
	}

	if config.selectedComponents == nil {
		config.selectedComponents = make(map[string]any)

		for _, c := range config.Components {
			if c == "*" || c == "all" {
				// enable all components
				return true
			}

			// prefix "-" is used to disable a component
			if strings.HasPrefix(c, "-") {
				continue
			}
			config.selectedComponents[c] = struct{}{}
		}
	}

	_, shouldEnable := config.selectedComponents[componentName]
	return shouldEnable
}

// ShouldDisable returns true if the component should be disabled.
// If the disable component sets are not specified, it will return false,
// meaning it should not be disabled, instead enabled by default.
func (config *Config) ShouldDisable(componentName string) bool {
	// not specified, thus enable all components (meaning should NOT disable any component)
	if len(config.Components) == 0 || config.Components[0] == "*" || config.Components[0] == "all" {
		return false
	}

	if config.disabledComponents == nil {
		config.disabledComponents = make(map[string]any)

		for _, c := range config.Components {
			if c == "*" || c == "all" {
				// enable all components
				return false
			}

			// prefix "-" is used to disable a component
			if !strings.HasPrefix(c, "-") {
				continue
			}
			config.disabledComponents[c] = struct{}{}
		}
	}

	_, shouldDisable := config.disabledComponents[componentName]
	return shouldDisable
}
