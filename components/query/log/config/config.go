// Package config provides the log poller configuration.
package config

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	query_config "github.com/leptonai/gpud/components/query/config"
	query_log_filter "github.com/leptonai/gpud/components/query/log/filter"

	"github.com/nxadm/tail"
)

const DefaultBufferSize = 2000

type Config struct {
	Query query_config.Config `json:"query"`

	BufferSize int `json:"buffer_size"`

	File     string     `json:"file"`
	Commands [][]string `json:"commands"`

	// For each interval, execute the scanning operation
	// based on the following config (rather than polling).
	// This is to backtrack the old log messages.
	Scan *Scan `json:"scan,omitempty"`

	// "OR" conditions to select logs.
	// An event is generated if any of the filters match.
	// Useful for explicit blacklisting "error" logs
	// (e.g., GPU error messages in dmesg).
	SelectFilters []*query_log_filter.Filter `json:"select_filters"`
	// "AND" conditions to select logs.
	// An event is generated if all of the filters do not match.
	// Useful for explicit whitelisting logs and catch all other
	// (e.g., good healthy log messages).
	RejectFilters []*query_log_filter.Filter `json:"reject_filters"`

	DB       *sql.DB        `json:"-"`
	SeekInfo *tail.SeekInfo `json:"seek_info,omitempty"`

	// Used to commit the last seek info to disk.
	SeekInfoSyncer func(ctx context.Context, file string, seekInfo tail.SeekInfo) `json:"-"`
}

// For each interval, execute the scanning operation
// based on the following config (rather than polling).
// This is to backtrack the old log messages.
type Scan struct {
	File        string     `json:"file"`
	Commands    [][]string `json:"commands"`
	LinesToTail int        `json:"lines_to_tail"`
}

func (cfg *Config) Validate() error {
	if cfg.File == "" && len(cfg.Commands) == 0 {
		return errors.New("file or commands must be set")
	}
	if cfg.Scan != nil {
		if cfg.Scan.File == "" && len(cfg.Scan.Commands) == 0 {
			return errors.New("file or commands must be set for scan")
		}
	}
	if len(cfg.SelectFilters) > 0 && len(cfg.RejectFilters) > 0 {
		return errors.New("cannot have both select and reject filters")
	}
	return nil
}

func (cfg *Config) SetDefaultsIfNotSet() {
	cfg.Query.SetDefaultsIfNotSet()

	if cfg.BufferSize == 0 {
		cfg.BufferSize = DefaultBufferSize
	}
	if cfg.Query.QueueSize < cfg.BufferSize {
		cfg.Query.QueueSize = cfg.BufferSize
	}
}

func ParseConfig(b any) (*Config, error) {
	raw, err := json.Marshal(b)
	if err != nil {
		return nil, err
	}
	cfg := new(Config)
	err = json.Unmarshal(raw, cfg)
	if err != nil {
		return nil, err
	}
	return cfg, nil
}
