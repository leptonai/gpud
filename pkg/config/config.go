// Package config provides the gpud configuration data for the server.
package config

import (
	"errors"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	nvidia_common "github.com/leptonai/gpud/pkg/config/common"
)

// Config provides gpud configuration data for the server
type Config struct {
	APIVersion string `json:"api_version"`

	// Address for the server to listen on.
	Address string `json:"address"`

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

	// A list of nvidia tool command paths to overwrite the default paths.
	NvidiaToolOverwrites nvidia_common.ToolOverwrites `json:"nvidia_tool_overwrites"`

	// PluginSpecsFile is the file that contains the plugin specs.
	PluginSpecsFile string `json:"plugin_specs_file"`

	// EnablePluginAPI enables the plugin API.
	EnablePluginAPI bool `json:"enable_plugin_api"`
	// EnableFaultInjector enables the fault injector.
	EnableFaultInjector bool `json:"enable_fault_injector"`

	// EnableComponents specifies the components to enable.
	// Leave empty to enable all components.
	EnableComponents []string       `json:"enable_components"`
	enableComponents map[string]any `json:"-"`

	// DisableComponents specifies the components to disable.
	// Leave empty to enable all components.
	DisableComponents []string       `json:"disable_components"`
	disableComponents map[string]any `json:"-"`
}

var (
	ErrInvalidAutoUpdateExitCode      = errors.New("auto_update_exit_code is only valid when auto_update is enabled")
	ErrInvalidEnableDisableComponents = errors.New("enable_components and disable_components cannot be set at the same time")
)

func (config *Config) Validate() error {
	if config.Address == "" {
		return errors.New("address is required")
	}
	if config.RetentionPeriod.Duration < time.Minute {
		return fmt.Errorf("retention_period must be at least 1 minute, got %d", config.RetentionPeriod.Duration)
	}
	if !config.EnableAutoUpdate && config.AutoUpdateExitCode != -1 {
		return ErrInvalidAutoUpdateExitCode
	}
	if len(config.EnableComponents) > 0 && len(config.DisableComponents) > 0 {
		return ErrInvalidEnableDisableComponents
	}
	return nil
}

// ShouldEnable returns true if the component should be enabled.
// If the enable component sets are not specified, it will return true,
// meaning it should be enabled by default.
func (config *Config) ShouldEnable(componentName string) bool {
	// not specified, thus enable all components
	if len(config.EnableComponents) == 0 {
		return true
	}

	if config.enableComponents == nil {
		config.enableComponents = make(map[string]any)
		for _, c := range config.EnableComponents {
			config.enableComponents[c] = struct{}{}
		}
	}

	_, shouldEnable := config.enableComponents[componentName]
	return shouldEnable
}

// ShouldDisable returns true if the component should be disabled.
// If the disable component sets are not specified, it will return false,
// meaning it should not be disabled, instead enabled by default.
func (config *Config) ShouldDisable(componentName string) bool {
	// not specified, thus enable all components (meaning should NOT disable any component)
	if len(config.DisableComponents) == 0 {
		return false
	}

	if config.disableComponents == nil {
		config.disableComponents = make(map[string]any)
		for _, c := range config.DisableComponents {
			config.disableComponents[c] = struct{}{}
		}
	}

	_, shouldDisable := config.disableComponents[componentName]
	return shouldDisable
}
