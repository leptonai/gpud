// Package config provides the log poller configuration.
package config

import (
	"context"
	"encoding/json"
	"errors"

	poller_config "github.com/leptonai/gpud/poller/config"
	poller_log_common "github.com/leptonai/gpud/poller/log/common"

	"github.com/nxadm/tail"
)

const DefaultBufferSize = 2000

type Config struct {
	PollerConfig poller_config.Config `json:"poller_config"`

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
	SelectFilters []*poller_log_common.Filter `json:"select_filters"`
	// "AND" conditions to select logs.
	// An event is generated if all of the filters do not match.
	// Useful for explicit whitelisting logs and catch all other
	// (e.g., good healthy log messages).
	RejectFilters []*poller_log_common.Filter `json:"reject_filters"`

	SeekInfo *tail.SeekInfo `json:"seek_info,omitempty"`

	// Used to commit the last seek info to disk.
	SeekInfoSyncer func(ctx context.Context, file string, seekInfo tail.SeekInfo) `json:"-"`

	// Parse time format
	TimeParseFunc poller_log_common.ExtractTimeFunc `json:"-"`
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
	cfg.PollerConfig.SetDefaultsIfNotSet()

	if cfg.BufferSize == 0 {
		cfg.BufferSize = DefaultBufferSize
	}
	if cfg.PollerConfig.QueueSize < cfg.BufferSize {
		cfg.PollerConfig.QueueSize = cfg.BufferSize
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
