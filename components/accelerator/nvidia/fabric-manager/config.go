package fabricmanager

import (
	"database/sql"
	"encoding/json"

	"k8s.io/utils/ptr"

	query_config "github.com/leptonai/gpud/components/query/config"
	query_log_config "github.com/leptonai/gpud/components/query/log/config"
	query_log_filter "github.com/leptonai/gpud/components/query/log/filter"
)

type Config struct {
	Query query_config.Config     `json:"query"`
	Log   query_log_config.Config `json:"log"`
}

func ParseConfig(b any, db *sql.DB) (*Config, error) {
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
		cfg.Query.State.DB = db
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
	eventNVSwitchFatailSXid       = "accelerator-nvidia-fabric-manager-nvswitch-sxid-log-fatal"
	regexNVSwitchFatalSXidFromLog = `.+detected NVSwitch fatal error (\d+)`

	// e.g.,
	// [Jul 09 2024 18:14:07] [ERROR] [tid 12727] detected NVSwitch non-fatal error 12028 on fid 0 on NVSwitch pci bus id 00000000:86:00.0 physical id 3 port 61
	eventNVSwitchNonFatailSXid       = "accelerator-nvidia-fabric-manager-nvswitch-sxid-log-non-fatal"
	regexNVSwitchNonFatalSXidFromLog = `.+detected NVSwitch non-fatal error (\d+)`

	// e.g.,
	// [Sep 17 2024 06:01:46] [ERROR] [tid 1230079] failed to find the GPU handle 5410063385821516767 in the multicast team request setup 6130285411925746235.
	eventNVSwitchNVLinkFailure        = "accelerator-nvidia-fabric-manager-nvlink-failure"
	regexNVSwitchNVLinkFailureFromLog = `.+failed to find the GPU handle \d+ in the multicast team .*`
)

var (
	filters = []*query_log_filter.Filter{
		{
			Name:            eventNVSwitchFatailSXid,
			Regex:           ptr.To(regexNVSwitchFatalSXidFromLog),
			OwnerReferences: []string{Name},
		},
		{
			Name:            eventNVSwitchNonFatailSXid,
			Regex:           ptr.To(regexNVSwitchNonFatalSXidFromLog),
			OwnerReferences: []string{Name},
		},
		{
			Name:            eventNVSwitchNVLinkFailure,
			Regex:           ptr.To(regexNVSwitchNVLinkFailureFromLog),
			OwnerReferences: []string{Name},
		},
	}
)

func DefaultLogConfig() query_log_config.Config {
	return query_log_config.Config{
		Query:         query_config.DefaultConfig(),
		BufferSize:    query_log_config.DefaultBufferSize,
		File:          fabricManagerLogFilePath,
		SelectFilters: filters,
	}
}
