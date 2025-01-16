package disk

import (
	poller_config "github.com/leptonai/gpud/poller/config"
)

func DefaultConfig() Config {
	cfg := Config{
		PollerConfig:             poller_config.DefaultConfig(),
		MountPointsToTrackUsage:  []string{"/"},
		MountTargetsToTrackUsage: []string{"/var/lib/kubelet"},
	}
	return cfg
}
