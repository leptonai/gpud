package disk

import (
	"database/sql"
	"encoding/json"
	"errors"
	"os"

	query_config "github.com/leptonai/gpud/components/query/config"
)

type Config struct {
	Query       query_config.Config `json:"query"`
	MountPoints []string            `json:"mount_points"`
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
	if len(cfg.MountPoints) == 0 {
		return errors.New("paths are required")
	}

	for _, path := range cfg.MountPoints {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			return errors.New("path does not exist: " + path)
		}
	}

	return nil
}
