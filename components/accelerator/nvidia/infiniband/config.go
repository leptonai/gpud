package infiniband

import (
	"database/sql"
	"encoding/json"

	query_config "github.com/leptonai/gpud/components/query/config"
)

const (
	DefaultExpectedRate = 400
)

type Config struct {
	Query query_config.Config `json:"query"`

	ExpectedPortStates
}

// Configures the expected state of the ports.
type ExpectedPortStates struct {
	// The number of ports expected to be "Active" and "LinkUp".
	// If not set, it defaults to the number of GPUs.
	PortCount int `json:"port_count"`

	// The expected rate in Gb/sec.
	// If not set, it defaults to 400.
	Rate int `json:"rate"`
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

func (cfg *Config) Validate() error {
	if cfg.ExpectedPortStates.Rate == 0 {
		cfg.ExpectedPortStates.Rate = DefaultExpectedRate
	}
	return nil
}
