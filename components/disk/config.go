package disk

import (
	"database/sql"
	"encoding/json"
	"errors"
	"os"

	poller_config "github.com/leptonai/gpud/poller/config"
)

type Config struct {
	PollerConfig poller_config.Config `json:"poller_config"`

	// Specifies the mount points to track the disk usage for (e.g., metrics).
	MountPointsToTrackUsage []string `json:"mount_points_to_track_usage"`

	// Mount targets to track the disk usage for (e.g., /var/lib/kubelet).
	MountTargetsToTrackUsage []string `json:"mount_targets_to_track_usage"`
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
	if cfg.PollerConfig.State != nil {
		cfg.PollerConfig.State.DBRW = dbRW
		cfg.PollerConfig.State.DBRO = dbRO
	}
	return cfg, nil
}

func (cfg Config) Validate() error {
	if len(cfg.MountPointsToTrackUsage) == 0 {
		return errors.New("paths are required")
	}

	for _, path := range cfg.MountPointsToTrackUsage {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			return errors.New("path does not exist: " + path)
		}
	}

	return nil
}
