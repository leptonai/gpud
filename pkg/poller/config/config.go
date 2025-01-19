// Package config provides the poller configuration.
package config

import (
	"database/sql"
	"time"

	_ "github.com/mattn/go-sqlite3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	DefaultPollInterval   = time.Minute
	DefaultGetTimeout     = 7 * time.Minute
	DefaultQueueSize      = 60
	DefaultStateRetention = 30 * time.Minute
)

type Config struct {
	Interval metav1.Duration `json:"interval"`

	// Timeout for each get operation.
	GetTimeout metav1.Duration `json:"get_timeout"`

	QueueSize int    `json:"queue_size"`
	State     *State `json:"state,omitempty"`
}

func DefaultConfig() Config {
	return Config{
		Interval:   metav1.Duration{Duration: DefaultPollInterval},
		GetTimeout: metav1.Duration{Duration: DefaultGetTimeout},
		QueueSize:  DefaultQueueSize,
		State: &State{
			Retention: metav1.Duration{Duration: DefaultStateRetention},
		},
	}
}

func (cfg *Config) SetDefaultsIfNotSet() {
	if cfg.Interval.Duration == 0 {
		cfg.Interval.Duration = DefaultPollInterval
	}
	if cfg.GetTimeout.Duration == 0 {
		cfg.GetTimeout.Duration = DefaultGetTimeout
	}
	if cfg.QueueSize == 0 {
		cfg.QueueSize = DefaultQueueSize
	}
	if cfg.State == nil {
		cfg.State = &State{
			Retention: metav1.Duration{Duration: DefaultStateRetention},
		}
	}
	if cfg.State != nil && cfg.State.Retention.Duration == 0 {
		cfg.State.Retention = metav1.Duration{Duration: DefaultStateRetention}
	}
}

type State struct {
	// DB instance for read-write.
	DBRW *sql.DB `json:"-"`

	// DB instance for read-only.
	DBRO *sql.DB `json:"-"`

	// Duration to keep states for.
	Retention metav1.Duration `json:"retention"`
}
