// Package config provides the query/poller configuration.
package config

import (
	"database/sql"
	"time"

	_ "github.com/mattn/go-sqlite3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	DefaultPollInterval   = time.Minute
	DefaultQueueSize      = 60
	DefaultStateRetention = 30 * time.Minute
)

type Config struct {
	Interval  metav1.Duration `json:"interval"`
	QueueSize int             `json:"queue_size"`
	State     *State          `json:"state,omitempty"`
}

func DefaultConfig() Config {
	return Config{
		Interval:  metav1.Duration{Duration: DefaultPollInterval},
		QueueSize: DefaultQueueSize,
		State: &State{
			Retention: metav1.Duration{Duration: DefaultStateRetention},
		},
	}
}

func (cfg *Config) SetDefaultsIfNotSet() {
	if cfg.Interval.Duration == 0 {
		cfg.Interval.Duration = DefaultPollInterval
	}
	if cfg.QueueSize == 0 {
		cfg.QueueSize = DefaultQueueSize
	}
	if cfg.State != nil && cfg.State.Retention.Duration == 0 {
		cfg.State.Retention = metav1.Duration{Duration: DefaultStateRetention}
	}
}

type State struct {
	DB *sql.DB `json:"-"`

	// Duration to keep states for.
	Retention metav1.Duration `json:"retention"`
}
