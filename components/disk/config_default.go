package disk

import (
	query_config "github.com/leptonai/gpud/components/query/config"
)

func DefaultConfig() Config {
	cfg := Config{
		Query:       query_config.DefaultConfig(),
		MountPoints: []string{"/"},
	}
	return cfg
}
