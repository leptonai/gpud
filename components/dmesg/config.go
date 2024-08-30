package dmesg

import (
	"database/sql"
	"encoding/json"
	"os"
	"os/exec"

	query_config "github.com/leptonai/gpud/components/query/config"
	query_log_config "github.com/leptonai/gpud/components/query/log/config"
)

type Config struct {
	Log query_log_config.Config `json:"log"`
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

	if cfg.Log.Query.State != nil {
		cfg.Log.Query.State.DB = db
	}
	cfg.Log.DB = db

	return cfg, nil
}

func (cfg Config) Validate() error {
	return cfg.Log.Validate()
}

func DmesgExists() bool {
	p, err := exec.LookPath("dmesg")
	if err != nil {
		return false
	}
	return p != ""
}

const DefaultDmesgFile = "/var/log/dmesg"

func DefaultConfig() Config {
	scanCommands := [][]string{
		{"cat", DefaultDmesgFile},

		// some old dmesg versions don't support --since, thus fall back to the one without --since and tail the last 200 lines
		// ref. https://github.com/leptonai/gpud/issues/32
		{"dmesg --ctime --nopager --buffer-size 163920 --since '7 hours ago' || dmesg --ctime --nopager --buffer-size 163920 | tail -n 200"},
	}
	if _, err := os.Stat(DefaultDmesgFile); os.IsNotExist(err) {
		scanCommands = [][]string{
			// some old dmesg versions don't support --since, thus fall back to the one without --since and tail the last 200 lines
			// ref. https://github.com/leptonai/gpud/issues/32
			{"dmesg --ctime --nopager --buffer-size 163920 --since '7 hours ago' || dmesg --ctime --nopager --buffer-size 163920 | tail -n 200"},
		}
	}

	cfg := Config{
		Log: query_log_config.Config{
			Query:      query_config.DefaultConfig(),
			BufferSize: query_log_config.DefaultBufferSize,

			Commands: [][]string{
				// run last commands as fallback, in case dmesg flag only works in some machines
				{"dmesg --ctime --nopager --buffer-size 163920 -w || true"},
				{"dmesg --ctime --nopager --buffer-size 163920 -W"},
			},

			Scan: &query_log_config.Scan{
				Commands:    scanCommands,
				LinesToTail: 10000,
			},
		},
	}
	cfg.Log.SelectFilters = append(cfg.Log.SelectFilters, defaultFilters...)

	return cfg
}
