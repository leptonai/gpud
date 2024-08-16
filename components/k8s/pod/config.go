package pod

import (
	"database/sql"
	"encoding/json"
	"errors"

	query_config "github.com/leptonai/gpud/components/query/config"
)

type Config struct {
	Query query_config.Config `json:"query"`
	Port  int                 `json:"port"`
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
	if cfg.Port == 0 {
		return errors.New("kubelet port is required")
	}
	return nil
}
