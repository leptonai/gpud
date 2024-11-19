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

	// The number of ports expected to be "Active" and "LinkUp".
	// If not set, it defaults to the number of GPUs.
	ExpectedPortCount int `json:"expected_port_count"`

	// The rate expected to be "Active" and "LinkUp".
	// If not set, it defaults to 400.
	ExpectedRate int `json:"expected_rate"`
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
	if cfg.ExpectedRate == 0 {
		cfg.ExpectedRate = DefaultExpectedRate
	}
	return nil
}
