package config

import (
	"context"
	"fmt"
	stdos "os"
	"path/filepath"
	"time"

	os_id "github.com/leptonai/gpud/components/os/id"
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
			//cpu.Name:              nil,
			//disk_id.Name:          disk.DefaultConfig(),
			//fuse_id.Name:          nil,
			//fd.Name:               nil,
			//info.Name:             nil,
			//memory.Name:           nil,
			os_id.Name: nil,
			//kernel_module_id.Name: nil,
		},

		RetentionPeriod: DefaultRetentionPeriod,
		CompactPeriod:   DefaultCompactPeriod,

		Pprof: false,

		Web: &Web{
			Enable:        true,
			Admin:         false,
			RefreshPeriod: DefaultRefreshPeriod,
			SincePeriod:   DefaultRetentionPeriod,
		},

		ToolOverwriteOptions: ToolOverwriteOptions{
			IbstatCommand: options.IbstatCommand,
		},

		EnableAutoUpdate: true,

		DockerIgnoreConnectionErrors:  options.DockerIgnoreConnectionErrors,
		KubeletIgnoreConnectionErrors: options.KubeletIgnoreConnectionErrors,

		FilesToCheck:         options.FilesToCheck,
		KernelModulesToCheck: options.KernelModulesToCheck,

		//NvidiaToolOverwrites: nvidia_common.ToolOverwrites{
		//	IbstatCommand: options.IbstatCommand,
		//},
	}

	//if len(cfg.FilesToCheck) > 0 {
	//	cfg.Components[file_id.Name] = cfg.FilesToCheck
	//}
	//if len(cfg.KernelModulesToCheck) > 0 {
	//	cfg.Components[kernel_module_id.Name] = cfg.KernelModulesToCheck
	//}
	//
	//// regardless of its dependency activeness, we always enable these components
	//// and dynamically checks its activeness
	//cfg.Components[docker_container.Name] = nil
	//cfg.Components[containerd_pod.Name] = nil
	//cfg.Components[kubelet_pod.Name] = nil
	//
	//cfg.Components[network_latency_id.Name] = nil
	//
	//if runtime.GOOS == "linux" {
	//	cfg.Components[component_pci_id.Name] = nil
	//}
	//
	//if runtime.GOOS == "linux" {
	//	if pkd_systemd.SystemdExists() && pkd_systemd.SystemctlExists() {
	//		if err := systemd.CreateDefaultEnvFile(); err != nil {
	//			log.Logger.Debugw("failed to create default systemd env file", "error", err)
	//		}
	//
	//		log.Logger.Debugw("auto-detected systemd -- configuring systemd component")
	//		cfg.Components[component_systemd_id.Name] = component_systemd.DefaultConfig()
	//	}
	//} else {
	//	log.Logger.Debugw("auto-detect systemd not supported -- skipping", "os", runtime.GOOS)
	//}
	//
	//if runtime.GOOS == "linux" {
	//	cfg.Components[tailscale.Name] = nil
	//} else {
	//	log.Logger.Debugw("auto-detect tailscale not supported -- skipping", "os", runtime.GOOS)
	//}
	//
	//nvidiaInstalled, err := nvidia_query.GPUsInstalled(ctx)
	//if err != nil {
	//	return nil, err
	//}
	//
	//if runtime.GOOS == "linux" && nvidiaInstalled {
	//	driverVersion, err := nvidia_query_nvml.GetDriverVersion()
	//	if err != nil {
	//		return nil, err
	//	}
	//	major, _, _, err := nvidia_query_nvml.ParseDriverVersion(driverVersion)
	//	if err != nil {
	//		return nil, err
	//	}
	//
	//	log.Logger.Debugw("auto-detected nvidia -- configuring nvidia components")
	//
	//	if nvidia_query_nvml.ClockEventsSupportedVersion(major) {
	//		clockEventsSupported, err := nvidia_query_nvml.ClockEventsSupported()
	//		if err == nil {
	//			if clockEventsSupported {
	//				log.Logger.Infow("auto-detected clock events supported")
	//				cfg.Components[nvidia_hw_slowdown_id.Name] = nil
	//			} else {
	//				log.Logger.Infow("auto-detected clock events not supported -- skipping", "driverVersion", driverVersion)
	//			}
	//		} else {
	//			log.Logger.Warnw("failed to check clock events supported or not", "error", err)
	//		}
	//	} else {
	//		log.Logger.Warnw("old nvidia driver -- skipping clock events in the default config, see https://github.com/NVIDIA/go-nvml/pull/123", "version", driverVersion)
	//	}
	//
	//	cfg.Components[nvidia_ecc_id.Name] = nil
	//	cfg.Components[nvidia_component_xid.Name] = nil
	//	cfg.Components[nvidia_component_sxid.Name] = nil
	//	cfg.Components[nvidia_info.Name] = nil
	//
	//	cfg.Components[nvidia_clock_speed_id.Name] = nil
	//	cfg.Components[nvidia_memory.Name] = nil
	//
	//gpmSupported, err := nvidia_query_nvml.GPMSupported()
	//if err == nil {
	//	if gpmSupported {
	//		log.Logger.Infow("auto-detected gpm supported")
	//		cfg.Components[nvidia_gpm.Name] = nil
	//	} else {
	//		log.Logger.Infow("auto-detected gpm not supported -- skipping", "error", err)
	//	}
	//} else {
	//	log.Logger.Warnw("failed to check gpm supported or not", "error", err)
	//}
	//
	//	cfg.Components[nvidia_nvlink.Name] = nil
	//	cfg.Components[nvidia_power_id.Name] = nil
	//	cfg.Components[nvidia_temperature.Name] = nil
	//	cfg.Components[nvidia_utilization.Name] = nil
	//	cfg.Components[nvidia_processes.Name] = nil
	//	cfg.Components[nvidia_remapped_rows.Name] = nil
	//	cfg.Components[library_id.Name] = library.Config{
	//		Libraries:  nvidia_query.DefaultNVIDIALibraries,
	//		SearchDirs: nvidia_query.DefaultNVIDIALibrariesSearchDirs,
	//	}
	//
	//	// optional
	//	cfg.Components[nvidia_infiniband_id.Name] = nil
	//
	//	cfg.Components[nvidia_nccl_id.Name] = nil
	//	cfg.Components[nvidia_peermem_id.Name] = nil
	//	cfg.Components[nvidia_persistence_mode_id.Name] = nil
	//	cfg.Components[nvidia_gsp_firmware_mode_id.Name] = nil
	//} else {
	//	log.Logger.Debugw("auto-detect nvidia not supported -- skipping", "os", runtime.GOOS)
	//}

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
