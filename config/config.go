// Package config provides the gpud configuration data for the server.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"
)

// Config provides gpud configuration data for the server
type Config struct {
	APIVersion string `json:"api_version"`

	// Basic server annotations (e.g., machine id, host name, etc.).
	Annotations map[string]string `json:"annotations,omitempty"`

	// Address for the server to listen on.
	Address string `json:"address"`

	// Component specific configurations.
	Components map[string]any `json:"components,omitempty"`

	// State file that persists the latest status.
	// If empty, the states are not persisted to file.
	State string `json:"state"`

	// Amount of time to retain states/metrics for.
	// Once elapsed, old states/metrics are purged/compacted.
	RetentionPeriod metav1.Duration `json:"retention_period"`

	// Interval at which to refresh selected components.
	// Disables refresh if not set.
	RefreshComponentsInterval metav1.Duration `json:"refresh_components_interval"`

	// Set true to enable profiler.
	Pprof bool `json:"pprof"`

	// Configures the local web configuration.
	Web *Web `json:"web,omitempty"`

	// Set false to disable auto update
	EnableAutoUpdate bool `json:"enable_auto_update"`

	// Exit code to exit with when auto updating.
	// Only valid when the auto update is enabled.
	// Set -1 to disable the auto update by exit code.
	AutoUpdateExitCode int `json:"auto_update_exit_code"`
}

// Configures the local web configuration.
type Web struct {
	// Enable the web interface.
	Enable bool `json:"enable"`

	// Enable the admin interface.
	Admin bool `json:"admin"`

	// RefreshPeriod is the time period to refresh metrics.
	RefreshPeriod metav1.Duration `json:"refresh_period"`

	// SincePeriod is the time period to start displaying metrics from.
	SincePeriod metav1.Duration `json:"since_period"`
}

var ErrInvalidAutoUpdateExitCode = errors.New("auto_update_exit_code is only valid when auto_update is enabled")

func (config *Config) Validate() error {
	if config.Address == "" {
		return errors.New("address is required")
	}
	if config.RetentionPeriod.Duration < time.Minute {
		return fmt.Errorf("retention_period must be at least 1 minute, got %d", config.RetentionPeriod.Duration)
	}
	if config.RefreshComponentsInterval.Duration < time.Minute {
		return fmt.Errorf("refresh_components_interval must be at least 1 minute, got %d", config.RefreshComponentsInterval.Duration)
	}
	if config.Web != nil && config.Web.RefreshPeriod.Duration < time.Minute {
		return fmt.Errorf("web_refresh_period must be at least 1 minute, got %d", config.Web.RefreshPeriod.Duration)
	}
	if config.Web != nil && config.Web.SincePeriod.Duration < 10*time.Minute {
		return fmt.Errorf("web_metrics_since_period must be at least 10 minutes, got %d", config.Web.SincePeriod.Duration)
	}
	if !config.EnableAutoUpdate && config.AutoUpdateExitCode != -1 {
		return ErrInvalidAutoUpdateExitCode
	}
	return nil
}

func (config *Config) YAML() ([]byte, error) {
	return yaml.Marshal(config)
}

func (config *Config) SyncYAML(file string) error {
	if _, err := os.Stat(filepath.Dir(file)); os.IsNotExist(err) {
		if err = os.MkdirAll(filepath.Dir(file), 0755); err != nil {
			return err
		}
	}
	data, err := config.YAML()
	if err != nil {
		return err
	}
	return os.WriteFile(file, data, 0644)
}

func LoadConfigYAML(file string) (*Config, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	return ParseConfigYAML(data)
}

func ParseConfigYAML(data []byte) (*Config, error) {
	config := new(Config)
	err := yaml.Unmarshal(data, config)
	if err != nil {
		return nil, err
	}
	return config, nil
}
