package config

import (
	"context"
	"fmt"
	stdos "os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/mitchellh/go-homedir"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	nvidiacommon "github.com/leptonai/gpud/pkg/config/common"
	"github.com/leptonai/gpud/version"
)

const (
	DefaultAPIVersion = "v1"
	DefaultGPUdPort   = 15132
)

var (
	DefaultRefreshPeriod = metav1.Duration{Duration: time.Minute}

	// keep the metrics only for the last 3 hours
	DefaultRetentionPeriod = metav1.Duration{Duration: 3 * time.Hour}

	// compact/vacuum is disruptive to existing queries (including reads)
	// but necessary to keep the state database from growing indefinitely
	// TODO: disabled for now, until we have a better way to detect the performance issue
	DefaultCompactPeriod = metav1.Duration{Duration: 0}
)

func DefaultConfig(ctx context.Context, opts ...OpOption) (*Config, error) {
	options := &Op{}
	if err := options.ApplyOpts(opts); err != nil {
		return nil, err
	}

	cfg := &Config{
		APIVersion: DefaultAPIVersion,
		Annotations: map[string]string{
			"version": version.Version,
		},
		Address:          fmt.Sprintf(":%d", DefaultGPUdPort),
		RetentionPeriod:  DefaultRetentionPeriod,
		CompactPeriod:    DefaultCompactPeriod,
		Pprof:            false,
		EnableAutoUpdate: true,
		NvidiaToolOverwrites: nvidiacommon.ToolOverwrites{
			IbstatCommand:   options.IbstatCommand,
			IbstatusCommand: options.IbstatusCommand,
		},
	}

	if cfg.State == "" {
		var err error
		cfg.State, err = DefaultStateFile()
		if err != nil {
			return nil, err
		}
	}

	return cfg, nil
}

var (
	defaultVarLibLinux  = "/var/lib/gpud"
	defaultVarLibOthers = ""
)

func init() {
	homeDir, err := homedir.Dir()
	if err == nil {
		defaultVarLibOthers = filepath.Join(homeDir, ".gpud")
	}
}

func setupDefaultDir() (string, error) {
	d := defaultVarLibLinux
	if runtime.GOOS != "linux" {
		d = defaultVarLibOthers
	}
	if _, err := stdos.Stat(d); stdos.IsNotExist(err) {
		if err = stdos.MkdirAll(d, 0755); err != nil {
			return "", err
		}
	}
	return d, nil
}

func DefaultStateFile() (string, error) {
	dir, err := setupDefaultDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "gpud.state"), nil
}

func DefaultFifoFile() (string, error) {
	f, err := setupDefaultDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(f, "gpud.fifo"), nil
}
