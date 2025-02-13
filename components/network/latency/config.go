package latency

import (
	"database/sql"
	"encoding/json"
	"fmt"

	query_config "github.com/leptonai/gpud/pkg/query/config"
)

const (
	// 1 second
	MinGlobalMillisecondThreshold = 1000
	// 7 seconds by default to reach any of the DERP servers.
	DefaultGlobalMillisecondThreshold = 7000
)

type Config struct {
	Query query_config.Config `json:"query"`

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
	if cfg.Query.State != nil {
		cfg.Query.State.DBRW = dbRW
		cfg.Query.State.DBRO = dbRO
	}
	return cfg, nil
}

func (cfg Config) Validate() error {
	if cfg.GlobalMillisecondThreshold > 0 && cfg.GlobalMillisecondThreshold < MinGlobalMillisecondThreshold {
		return fmt.Errorf("global millisecond threshold must be greater than %d", MinGlobalMillisecondThreshold)
	}
	return nil
}
