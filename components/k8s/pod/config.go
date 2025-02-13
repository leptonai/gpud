package pod

import (
	"database/sql"
	"encoding/json"
	"errors"

	query_config "github.com/leptonai/gpud/pkg/query/config"
)

type Config struct {
	Query query_config.Config `json:"query"`
	Port  int                 `json:"port"`

	// In case the kubelet does not open the read-only port, we ignore such errors as
	// 'Get "http://localhost:10255/pods": dial tcp 127.0.0.1:10255: connect: connection refused'.
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
	if cfg.Query.State != nil {
		cfg.Query.State.DBRW = dbRW
		cfg.Query.State.DBRO = dbRO
	}
	return cfg, nil
}

func (cfg Config) Validate() error {
	if cfg.Port == 0 {
		return errors.New("kubelet port is required")
	}
	return nil
}
