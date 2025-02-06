package fd

import (
	"database/sql"
	"encoding/json"

	query_config "github.com/leptonai/gpud/components/query/config"
)

type Config struct {
	Query query_config.Config `json:"query"`

	// ThresholdAllocatedFileHandles is the number of file descriptors that are currently allocated,
	// at which we consider the system to be under high file descriptor usage.
	ThresholdAllocatedFileHandles uint64 `json:"threshold_allocated_file_handles"`

	// ThresholdRunningPIDs is the number of running pids at which
	// we consider the system to be under high file descriptor usage.
	// This is useful for triggering alerts when the system is under high load.
	// And useful when the actual system fd-max is set to unlimited.
	ThresholdRunningPIDs uint64 `json:"threshold_running_pids"`
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
	if cfg.Query.State != nil {
		cfg.Query.State.DBRW = dbRW
		cfg.Query.State.DBRO = dbRO
	}
	return cfg, nil
}

// DefaultThresholdAllocatedFileHandles is some high number, in case the system is under high file descriptor usage.
const DefaultThresholdAllocatedFileHandles = 10000000

// DefaultThresholdRunningPIDs is some high number, in case fd-max is unlimited
const DefaultThresholdRunningPIDs = 900000

func (cfg *Config) Validate() error {
	if cfg.ThresholdAllocatedFileHandles == 0 {
		cfg.ThresholdAllocatedFileHandles = DefaultThresholdAllocatedFileHandles
	}
	if cfg.ThresholdRunningPIDs == 0 {
		cfg.ThresholdRunningPIDs = DefaultThresholdRunningPIDs
	}
	return nil
}
