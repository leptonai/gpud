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

	eventNVSwitchFatailSXid    = "accelerator-nvidia-fabric-manager-nvswitch-sxid-log-fatal"
	eventNVSwitchNonFatailSXid = "accelerator-nvidia-fabric-manager-nvswitch-sxid-log-non-fatal"

	regexNVSwitchFatalSXidFromLog    = `.+detected NVSwitch fatal error (\d+)`
	regexNVSwitchNonFatalSXidFromLog = `.+detected NVSwitch non-fatal error (\d+)`
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
