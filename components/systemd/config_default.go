package systemd

import (
	query_config "github.com/leptonai/gpud/pkg/query/config"
)

func DefaultConfig() Config {
	return Config{
		Query: query_config.DefaultConfig(),
		Units: []string{
			// TODO: move these to its own component
			"network.target",
			"local-fs.target",
		},
	}
}
