package config

import (
	"context"
	"fmt"
	stdos "os"
	"path/filepath"
	"runtime"
	"time"

	nvidia_clock_speed "github.com/leptonai/gpud/components/accelerator/nvidia/clock-speed"
	nvidia_ecc "github.com/leptonai/gpud/components/accelerator/nvidia/ecc"
	nvidia_gpm "github.com/leptonai/gpud/components/accelerator/nvidia/gpm"
	nvidia_gsp_firmware_mode "github.com/leptonai/gpud/components/accelerator/nvidia/gsp-firmware-mode"
	nvidia_hw_slowdown "github.com/leptonai/gpud/components/accelerator/nvidia/hw-slowdown"
	nvidia_infiniband "github.com/leptonai/gpud/components/accelerator/nvidia/infiniband"
	nvidia_info "github.com/leptonai/gpud/components/accelerator/nvidia/info"
	nvidia_memory "github.com/leptonai/gpud/components/accelerator/nvidia/memory"
	nvidia_nccl "github.com/leptonai/gpud/components/accelerator/nvidia/nccl"
	nvidia_nvlink "github.com/leptonai/gpud/components/accelerator/nvidia/nvlink"
	nvidia_peermem "github.com/leptonai/gpud/components/accelerator/nvidia/peermem"
	nvidia_persistence_mode "github.com/leptonai/gpud/components/accelerator/nvidia/persistence-mode"
	nvidia_power "github.com/leptonai/gpud/components/accelerator/nvidia/power"
	nvidia_processes "github.com/leptonai/gpud/components/accelerator/nvidia/processes"
	nvidia_remapped_rows "github.com/leptonai/gpud/components/accelerator/nvidia/remapped-rows"
	nvidia_component_sxid "github.com/leptonai/gpud/components/accelerator/nvidia/sxid"
	nvidia_temperature "github.com/leptonai/gpud/components/accelerator/nvidia/temperature"
	nvidia_utilization "github.com/leptonai/gpud/components/accelerator/nvidia/utilization"
	nvidia_component_xid "github.com/leptonai/gpud/components/accelerator/nvidia/xid"
	containerd_pod "github.com/leptonai/gpud/components/containerd/pod"
	"github.com/leptonai/gpud/components/cpu"
	"github.com/leptonai/gpud/components/disk"
	docker_container "github.com/leptonai/gpud/components/docker/container"
	"github.com/leptonai/gpud/components/fd"
	"github.com/leptonai/gpud/components/fuse"
	"github.com/leptonai/gpud/components/info"
	kernel_module "github.com/leptonai/gpud/components/kernel-module"
	kubelet_pod "github.com/leptonai/gpud/components/kubelet/pod"
	"github.com/leptonai/gpud/components/library"
	"github.com/leptonai/gpud/components/memory"
	network_latency "github.com/leptonai/gpud/components/network/latency"
	"github.com/leptonai/gpud/components/os"
	component_pci "github.com/leptonai/gpud/components/pci"
	"github.com/leptonai/gpud/components/tailscale"
	nvidia_common "github.com/leptonai/gpud/pkg/config/common"
	"github.com/leptonai/gpud/pkg/log"
	nvidia_query "github.com/leptonai/gpud/pkg/nvidia-query"
	nvidia_query_nvml "github.com/leptonai/gpud/pkg/nvidia-query/nvml"
	"github.com/leptonai/gpud/version"

	"github.com/mitchellh/go-homedir"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

	if runtime.GOOS == "linux" && nvidiaInstalled {
		driverVersion, err := nvidia_query_nvml.GetDriverVersion()
		if err != nil {
			return nil, err
		}
		major, _, _, err := nvidia_query_nvml.ParseDriverVersion(driverVersion)
		if err != nil {
			return nil, err
		}

		log.Logger.Debugw("auto-detected nvidia -- configuring nvidia components")

		if nvidia_query_nvml.ClockEventsSupportedVersion(major) {
			clockEventsSupported, err := nvidia_query_nvml.ClockEventsSupported()
			if err == nil {
				if clockEventsSupported {
					log.Logger.Infow("auto-detected clock events supported")
					cfg.Components[nvidia_hw_slowdown.Name] = nil
				} else {
					log.Logger.Infow("auto-detected clock events not supported -- skipping", "driverVersion", driverVersion)
				}
			} else {
				log.Logger.Warnw("failed to check clock events supported or not", "error", err)
			}
		} else {
			log.Logger.Warnw("old nvidia driver -- skipping clock events in the default config, see https://github.com/NVIDIA/go-nvml/pull/123", "version", driverVersion)
		}

		cfg.Components[nvidia_ecc.Name] = nil
		cfg.Components[nvidia_component_xid.Name] = nil
		cfg.Components[nvidia_component_sxid.Name] = nil
		cfg.Components[nvidia_info.Name] = nil

		cfg.Components[nvidia_clock_speed.Name] = nil
		cfg.Components[nvidia_memory.Name] = nil

		gpmSupported, err := nvidia_query_nvml.GPMSupported()
		if err == nil {
			if gpmSupported {
				log.Logger.Infow("auto-detected gpm supported")
				cfg.Components[nvidia_gpm.Name] = nil
			} else {
				log.Logger.Infow("auto-detected gpm not supported -- skipping", "error", err)
			}
		} else {
			log.Logger.Warnw("failed to check gpm supported or not", "error", err)
		}

		cfg.Components[nvidia_nvlink.Name] = nil
		cfg.Components[nvidia_power.Name] = nil
		cfg.Components[nvidia_temperature.Name] = nil
		cfg.Components[nvidia_utilization.Name] = nil
		cfg.Components[nvidia_processes.Name] = nil
		cfg.Components[nvidia_remapped_rows.Name] = nil
		cfg.Components[library.Name] = library.Config{
			Libraries:  nvidia_query.DefaultNVIDIALibraries,
			SearchDirs: nvidia_query.DefaultNVIDIALibrariesSearchDirs,
		}

		// optional
		cfg.Components[nvidia_infiniband.Name] = nil
		cfg.Components[nvidia_nccl.Name] = nil
		cfg.Components[nvidia_peermem.Name] = nil
		cfg.Components[nvidia_persistence_mode.Name] = nil
		cfg.Components[nvidia_gsp_firmware_mode.Name] = nil
	} else {
		log.Logger.Debugw("auto-detect nvidia not supported -- skipping", "os", runtime.GOOS)
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

func DefaultConfigFile() (string, error) {
	dir, err := setupDefaultDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "gpud.yaml"), nil
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
