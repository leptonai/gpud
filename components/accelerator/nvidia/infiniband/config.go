package infiniband

import (
	"database/sql"
	"encoding/json"

	poller_config "github.com/leptonai/gpud/poller/config"
)

type Config struct {
	PollerConfig poller_config.Config `json:"poller_config"`

	ExpectedPortStates
}

// Configures the expected state of the ports.
type ExpectedPortStates struct {
	// The minimum number of ports.
	// If not set, it defaults to the number of GPUs.
	AtLeastPorts int `json:"at_least_ports"`

	// The expected rate in Gb/sec.
	// If not set, it defaults to 200.
	AtLeastRate int `json:"at_least_rate"`
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

func (cfg *Config) Validate() error {
	return nil
}
