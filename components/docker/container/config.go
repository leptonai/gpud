package container

import (
	"database/sql"
	"encoding/json"

	poller_config "github.com/leptonai/gpud/poller/config"
)

type Config struct {
	PollerConfig poller_config.Config `json:"poller_config"`

	// In case the docker daemon is not running, we ignore such errors as
	// 'Cannot connect to the Docker daemon at unix:///var/run/docker.sock. Is the docker daemon running?'.
	IgnoreConnectionErrors bool `json:"ignore_connection_errors"`
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
	return nil
}
