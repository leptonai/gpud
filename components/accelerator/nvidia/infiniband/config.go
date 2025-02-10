package infiniband

import (
	"database/sql"
	"encoding/json"
	"sync"

	nvidia_common "github.com/leptonai/gpud/components/accelerator/nvidia/common"
	"github.com/leptonai/gpud/components/accelerator/nvidia/query/infiniband"
	query_config "github.com/leptonai/gpud/components/query/config"
	"github.com/leptonai/gpud/log"
)

type Config struct {
	Query query_config.Config `json:"query"`

	nvidia_common.ToolOverwrites
}

var (
	defaultExpectedPortStatesMu sync.RWMutex
	defaultExpectedPortStates   = infiniband.ExpectedPortStates{
		AtLeastPorts: -1,
		AtLeastRate:  -1,
	}
)

func GetDefaultExpectedPortStates() infiniband.ExpectedPortStates {
	defaultExpectedPortStatesMu.RLock()
	defer defaultExpectedPortStatesMu.RUnlock()
	return defaultExpectedPortStates
}

func SetDefaultExpectedPortStates(states infiniband.ExpectedPortStates) {
	log.Logger.Infow("setting default expected port states", "at_least_ports", states.AtLeastPorts, "at_least_rate", states.AtLeastRate)

	defaultExpectedPortStatesMu.Lock()
	defer defaultExpectedPortStatesMu.Unlock()
	defaultExpectedPortStates = states
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

func (cfg *Config) Validate() error {
	return nil
}
