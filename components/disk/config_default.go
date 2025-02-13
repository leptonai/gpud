package disk

import (
	query_config "github.com/leptonai/gpud/pkg/query/config"
)

func DefaultConfig() Config {
	cfg := Config{
		Query:                    query_config.DefaultConfig(),
		MountPointsToTrackUsage:  []string{"/"},
		MountTargetsToTrackUsage: []string{"/var/lib/kubelet"},
	}
	return cfg
}
