package fuse

import (
	"database/sql"
	"encoding/json"

	poller_config "github.com/leptonai/gpud/poller/config"
)

type Config struct {
	PollerConfig poller_config.Config `json:"poller_config"`

	// CongestedPercentAgainstThreshold is the percentage of the FUSE connections waiting
	// at which we consider the system to be congested.
	CongestedPercentAgainstThreshold float64 `json:"congested_percent_against_threshold"`

	// MaxBackgroundPercentAgainstThreshold is the percentage of the FUSE connections waiting
	// at which we consider the system to be congested.
	MaxBackgroundPercentAgainstThreshold float64 `json:"max_background_percent_against_threshold"`
}

func ParseConfig(b any, dbRW *sql.DB, dbRO *sql.DB) (*Config, error) {
	raw, err := json.Marshal(b)
	if err != nil {
		return nil, err
	}
	cfg := new(Config)
	err = json.Unmarshal(raw, cfg)
	if err != nil {
		return nil, err
	}
	if cfg.PollerConfig.State != nil {
		cfg.PollerConfig.State.DBRW = dbRW
		cfg.PollerConfig.State.DBRO = dbRO
	}
	return cfg, nil
}

const (
	DefaultCongestedPercentAgainstThreshold     = float64(90)
	DefaultMaxBackgroundPercentAgainstThreshold = float64(80)
)

func (cfg *Config) Validate() error {
	if cfg.CongestedPercentAgainstThreshold == 0 {
		cfg.CongestedPercentAgainstThreshold = DefaultCongestedPercentAgainstThreshold
	}
	if cfg.MaxBackgroundPercentAgainstThreshold == 0 {
		cfg.MaxBackgroundPercentAgainstThreshold = DefaultMaxBackgroundPercentAgainstThreshold
	}
	return nil
}
