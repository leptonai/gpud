package config

import (
	"context"
	"fmt"
	stdos "os"
	"path/filepath"
	"time"

	"github.com/mitchellh/go-homedir"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	nvidiacommon "github.com/leptonai/gpud/pkg/config/common"
)

const (
	DefaultAPIVersion = "v1"
	DefaultGPUdPort   = 15132
	DefaultDataDir    = "/var/lib/gpud"
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

	dataDir, err := ResolveDataDir(options.DataDir)
	if err != nil {
		return nil, err
	}

	cfg := &Config{
		APIVersion:       DefaultAPIVersion,
		Address:          fmt.Sprintf(":%d", DefaultGPUdPort),
		DataDir:          dataDir,
		RetentionPeriod:  DefaultRetentionPeriod,
		CompactPeriod:    DefaultCompactPeriod,
		Pprof:            false,
		EnableAutoUpdate: true,
		NvidiaToolOverwrites: nvidiacommon.ToolOverwrites{
			InfinibandClassRootDir: options.InfinibandClassRootDir,
		},
		FailureInjector: options.FailureInjector,
	}

	if cfg.State == "" {
		cfg.State = StateFilePath(cfg.DataDir)
	}

	return cfg, nil
}

// ResolveDataDir resolves and validates a data directory path.
// If dataDir is empty or matches DefaultDataDir, it uses platform-specific logic:
//   - For root users (or when /var/lib exists): /var/lib/gpud
//   - For non-root users: $HOME/.gpud
//
// For non-empty custom paths, it ensures the directory exists and is writable.
// The directory is created with 0755 permissions if it doesn't exist.
func ResolveDataDir(dataDir string) (string, error) {
	if dataDir == "" {
		return setupDefaultDir()
	}

	if err := stdos.MkdirAll(dataDir, 0755); err != nil {
		return "", err
	}
	return dataDir, nil
}

func setupDefaultDir() (string, error) {
	asRoot := stdos.Geteuid() == 0 // running as root

	d := DefaultDataDir
	_, err := stdos.Stat(filepath.Dir(d))
	if !asRoot || stdos.IsNotExist(err) {
		homeDir, err := homedir.Dir()
		if err != nil {
			return "", err
		}
		d = filepath.Join(homeDir, ".gpud")
	}

	if _, err := stdos.Stat(d); stdos.IsNotExist(err) {
		if err = stdos.MkdirAll(d, 0755); err != nil {
			return "", err
		}
	}
	return d, nil
}

func DefaultStateFile() (string, error) {
	dir, err := ResolveDataDir("")
	if err != nil {
		return "", err
	}
	return StateFilePath(dir), nil
}

func DefaultFifoFile() (string, error) {
	dir, err := ResolveDataDir("")
	if err != nil {
		return "", err
	}
	return FifoFilePath(dir), nil
}

// StateFilePath returns the state DB file path under the dataDir.
func StateFilePath(dataDir string) string {
	return filepath.Join(dataDir, "gpud.state")
}

// FifoFilePath returns the FIFO pipe path under the dataDir.
func FifoFilePath(dataDir string) string {
	return filepath.Join(dataDir, "gpud.fifo")
}

// PackagesDir returns the packages directory under the dataDir.
func PackagesDir(dataDir string) string {
	return filepath.Join(dataDir, "packages")
}

// VersionFilePath returns the version file path under the dataDir.
func VersionFilePath(dataDir string) string {
	return filepath.Join(dataDir, "target_version")
}
