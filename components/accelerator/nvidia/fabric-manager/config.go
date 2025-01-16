package fabricmanager

import (
	"database/sql"
	"encoding/json"

	fabric_manager_log "github.com/leptonai/gpud/components/accelerator/nvidia/query/fabric-manager-log"
	poller_config "github.com/leptonai/gpud/poller/config"
	poller_log_common "github.com/leptonai/gpud/poller/log/common"
	poller_log_config "github.com/leptonai/gpud/poller/log/config"

	"k8s.io/utils/ptr"
)

type Config struct {
	PollerConfig poller_config.Config     `json:"poller_config"`
	Log          poller_log_config.Config `json:"log"`
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
	if cfg.Log.PollerConfig.State != nil {
		cfg.Log.PollerConfig.State.DBRW = dbRW
		cfg.Log.PollerConfig.State.DBRO = dbRO
	}
	return cfg, nil
}

func (cfg Config) Validate() error {
	return cfg.Log.Validate()
}

const (
	fabricManagerLogFilePath = "/var/log/fabricmanager.log"

	// e.g.,
	// [Jul 23 2024 07:53:55] [ERROR] [tid 841] detected NVSwitch fatal error 20034 on fid 0 on NVSwitch pci bus id 00000000:86:00.0 physical id 3 port 33
	eventNVSwitchFatailSXid = "accelerator-nvidia-fabric-manager-nvswitch-sxid-log-fatal"

	// e.g.,
	// [Jul 09 2024 18:14:07] [ERROR] [tid 12727] detected NVSwitch non-fatal error 12028 on fid 0 on NVSwitch pci bus id 00000000:86:00.0 physical id 3 port 61
	eventNVSwitchNonFatailSXid = "accelerator-nvidia-fabric-manager-nvswitch-sxid-log-non-fatal"

	// e.g.,
	// [Sep 17 2024 06:01:46] [ERROR] [tid 1230079] failed to find the GPU handle 5410063385821516767 in the multicast team request setup 6130285411925746235.
	eventNVSwitchNVLinkFailure = "accelerator-nvidia-fabric-manager-nvlink-failure"
)

var (
	filters = []*poller_log_common.Filter{
		{
			Name:            eventNVSwitchFatailSXid,
			Regex:           ptr.To(fabric_manager_log.RegexNVSwitchFatalSXidFromLog),
			OwnerReferences: []string{Name},
		},
		{
			Name:            eventNVSwitchNonFatailSXid,
			Regex:           ptr.To(fabric_manager_log.RegexNVSwitchNonFatalSXidFromLog),
			OwnerReferences: []string{Name},
		},
		{
			Name:            eventNVSwitchNVLinkFailure,
			Regex:           ptr.To(fabric_manager_log.RegexNVSwitchNVLinkFailureFromLog),
			OwnerReferences: []string{Name},
		},
	}
)

func DefaultLogConfig() poller_log_config.Config {
	return poller_log_config.Config{
		PollerConfig:  poller_config.DefaultConfig(),
		BufferSize:    poller_log_config.DefaultBufferSize,
		File:          fabricManagerLogFilePath,
		SelectFilters: filters,
	}
}
