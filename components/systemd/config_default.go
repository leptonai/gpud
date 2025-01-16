package systemd

import (
	poller_config "github.com/leptonai/gpud/poller/config"
)

func DefaultConfig() Config {
	return Config{
		PollerConfig: poller_config.DefaultConfig(),
		Units: []string{
			"systemd-logind.service",
			"systemd-journald.service",
			"network.target",
			"local-fs.target",
		},
	}
}
