package common

import (
	"database/sql"
	"encoding/json"

	query_config "github.com/leptonai/gpud/pkg/query/config"
)

type Config struct {
	Query query_config.Config `json:"query"`

	ToolOverwrites
}

type ToolOverwrites struct {
	NvidiaSMICommand      string `json:"nvidia_smi_command"`
	NvidiaSMIQueryCommand string `json:"nvidia_smi_query_command"`
	IbstatCommand         string `json:"ibstat_command"`
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
	return nil
}
