package latency

import (
	"database/sql"
	"encoding/json"
	"fmt"

	query_config "github.com/leptonai/gpud/components/query/config"
)

type Config struct {
	Query       query_config.Config `json:"query"`
	RegionCodes []string            `json:"region_codes"`
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

func (cfg Config) Validate() error {
	if len(cfg.RegionCodes) == 0 {
		return fmt.Errorf("region codes are required")
	}
	return nil
}
