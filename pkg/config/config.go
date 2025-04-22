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

	// Basic server annotations (e.g., machine id, host name, etc.).
	Annotations map[string]string `json:"annotations,omitempty"`

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

	// EnableAPIPluginRegistration enables the API plugin registration.
	EnableAPIPluginRegistration bool `json:"enable_api_plugin_registration"`
}

var ErrInvalidAutoUpdateExitCode = errors.New("auto_update_exit_code is only valid when auto_update is enabled")

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
	return nil
}
