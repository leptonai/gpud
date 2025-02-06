package systemd

import (
	"database/sql"
	"encoding/json"
	"errors"

	query_config "github.com/leptonai/gpud/internal/query/config"
)

type Config struct {
	Query query_config.Config `json:"query"`
	Units []string            `json:"units"`
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
	if len(cfg.Units) == 0 {
		return errors.New("units is required")
	}
	return nil
}
