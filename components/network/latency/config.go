package latency

import (
	"database/sql"
	"encoding/json"
	"fmt"

	poller_config "github.com/leptonai/gpud/poller/config"
)

const (
	// 1 second
	MinGlobalMillisecondThreshold = 1000
	// 7 seconds by default to reach any of the DERP servers.
	DefaultGlobalMillisecondThreshold = 7000
)

type Config struct {
	PollerConfig poller_config.Config `json:"poller_config"`

	// GlobalMillisecondThreshold is the global threshold in milliseconds for the DERP latency.
	// If all DERP latencies are greater than this threshold, the component will be marked as failed.
	// If at least one DERP latency is less than this threshold, the component will be marked as healthy.
	GlobalMillisecondThreshold int64 `json:"global_millisecond_threshold"`
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

func (cfg Config) Validate() error {
	if cfg.GlobalMillisecondThreshold > 0 && cfg.GlobalMillisecondThreshold < MinGlobalMillisecondThreshold {
		return fmt.Errorf("global millisecond threshold must be greater than %d", MinGlobalMillisecondThreshold)
	}
	return nil
}
