package infiniband

import (
	"database/sql"
	"encoding/json"
	"sync"

	nvidia_common "github.com/leptonai/gpud/components/accelerator/nvidia/common"
	query_config "github.com/leptonai/gpud/components/query/config"
)

type Config struct {
	Query query_config.Config `json:"query"`

	nvidia_common.ToolOverwrites
}

var (
	defaultExpectedPortStatesMu sync.RWMutex
	defaultExpectedPortStates   = ExpectedPortStates{
		AtLeastPorts: 0,
		AtLeastRate:  0,
	}
)

func GetDefaultExpectedPortStates() ExpectedPortStates {
	defaultExpectedPortStatesMu.RLock()
	defer defaultExpectedPortStatesMu.RUnlock()
	return defaultExpectedPortStates
}

func SetDefaultExpectedPortStates(states ExpectedPortStates) {
	defaultExpectedPortStatesMu.Lock()
	defer defaultExpectedPortStatesMu.Unlock()
	defaultExpectedPortStates = states
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
