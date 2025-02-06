package systemd

import (
	query_config "github.com/leptonai/gpud/internal/query/config"
)

func DefaultConfig() Config {
	return Config{
		Query: query_config.DefaultConfig(),
		Units: []string{
			"systemd-logind.service",
			"systemd-journald.service",
			"network.target",
			"local-fs.target",
		},
	}
}
