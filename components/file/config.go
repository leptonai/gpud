package file

import (
	"database/sql"
	"encoding/json"
	"errors"

	query_config "github.com/leptonai/gpud/components/query/config"
)

type Config struct {
	Query query_config.Config `json:"query"`

	Files []File `json:"files"`
}

type File struct {
	Path string `json:"path"`

	// State.Healthy is set to false if the file does not exist.
	RequireExists bool `json:"require_exists"`

	Exists        bool   `json:"exists"`
	Size          int64  `json:"size"`
	SizeHumanized string `json:"size_humanized"`
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
	if len(cfg.Files) == 0 {
		return errors.New("files is empty")
	}
	for _, f := range cfg.Files {
		if f.Path == "" {
			return errors.New("file path is empty")
		}
	}
	return nil
}
