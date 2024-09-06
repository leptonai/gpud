package fd

import (
	"database/sql"
	"encoding/json"

	query_config "github.com/leptonai/gpud/components/query/config"
)

type Config struct {
	Query query_config.Config `json:"query"`

	// ThresholdLimit is the number of file descriptor limit at which
	// we consider the system to be under high file descriptor usage.
	// This is useful for triggering alerts when the system is under high load.
	// And useful when the actual system fd-max is set to unlimited.
	ThresholdLimit uint64 `json:"threshold_limit"`
}

func ParseConfig(b any, db *sql.DB) (*Config, error) {
	raw, err := json.Marshal(b)
	if err != nil {
		return nil, err
	}
	cfg := new(Config)
	err = json.Unmarshal(raw, cfg)
	if err != nil {
		return nil, err
	}
	if cfg.Query.State != nil {
		cfg.Query.State.DB = db
	}
	return cfg, nil
}

// DefaultThresholdLimit is some high number, in case fd-max is unlimited
const DefaultThresholdLimit = 1048576

func (cfg Config) Validate() error {
	if cfg.ThresholdLimit == 0 {
		cfg.ThresholdLimit = DefaultThresholdLimit
	}
	return nil
}
