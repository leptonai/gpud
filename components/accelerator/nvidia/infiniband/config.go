package infiniband

import (
	"database/sql"
	"encoding/json"

	query_config "github.com/leptonai/gpud/components/query/config"
)

type Config struct {
	Query query_config.Config `json:"query"`

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
	return nil
}
