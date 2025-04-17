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

	containerd_pod "github.com/leptonai/gpud/components/containerd/pod"
	"github.com/leptonai/gpud/components/cpu"
	"github.com/leptonai/gpud/components/disk"
	docker_container "github.com/leptonai/gpud/components/docker/container"
	"github.com/leptonai/gpud/components/fd"
	"github.com/leptonai/gpud/components/fuse"
	"github.com/leptonai/gpud/components/info"
	kernel_module "github.com/leptonai/gpud/components/kernel-module"
	kubelet_pod "github.com/leptonai/gpud/components/kubelet/pod"
	"github.com/leptonai/gpud/components/memory"
	network_latency "github.com/leptonai/gpud/components/network/latency"
	"github.com/leptonai/gpud/components/os"
	component_pci "github.com/leptonai/gpud/components/pci"
	"github.com/leptonai/gpud/components/tailscale"
	nvidia_common "github.com/leptonai/gpud/pkg/config/common"
	"github.com/leptonai/gpud/pkg/log"
	nvidia_query "github.com/leptonai/gpud/pkg/nvidia-query"
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

		Address: fmt.Sprintf(":%d", DefaultGPUdPort),

		// default components that work both in mac/linux
		Components: map[string]any{
			cpu.Name:           nil,
			disk.Name:          nil,
			fuse.Name:          nil,
			fd.Name:            nil,
			info.Name:          nil,
			memory.Name:        nil,
			os.Name:            nil,
			kernel_module.Name: nil,
		},

		RetentionPeriod: DefaultRetentionPeriod,
		CompactPeriod:   DefaultCompactPeriod,

		Pprof: false,

		ToolOverwriteOptions: ToolOverwriteOptions{
			IbstatCommand: options.IbstatCommand,
		},

		EnableAutoUpdate: true,

		DockerIgnoreConnectionErrors: options.DockerIgnoreConnectionErrors,

		KernelModulesToCheck: options.KernelModulesToCheck,

		NvidiaToolOverwrites: nvidia_common.ToolOverwrites{
			IbstatCommand: options.IbstatCommand,
		},
	}

	if len(cfg.KernelModulesToCheck) > 0 {
		cfg.Components[kernel_module.Name] = cfg.KernelModulesToCheck
	}

	// regardless of its dependency activeness, we always enable these components
	// and dynamically checks its activeness
	cfg.Components[docker_container.Name] = nil
	cfg.Components[containerd_pod.Name] = nil
	cfg.Components[kubelet_pod.Name] = nil

	cfg.Components[network_latency.Name] = nil

	cfg.Components[component_pci.Name] = nil

	if runtime.GOOS == "linux" {
		cfg.Components[tailscale.Name] = nil
	} else {
		log.Logger.Debugw("auto-detect tailscale not supported -- skipping", "os", runtime.GOOS)
	}

	nvidiaInstalled, err := nvidia_query.GPUsInstalled(ctx)
	if err != nil {
		return nil, err
	}
	_ = nvidiaInstalled

	if cfg.State == "" {
		var err error
		cfg.State, err = DefaultStateFile()
		if err != nil {
			return nil, err
		}
	}

	return cfg, nil
}

const defaultVarLib = "/var/lib/gpud"

func setupDefaultDir() (string, error) {
	asRoot := stdos.Geteuid() == 0 // running as root

	d := defaultVarLib
	_, err := stdos.Stat("/var/lib")
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
