package config

import (
	"context"
	"fmt"
	"net"
	stdos "os"
	"path/filepath"
	"runtime"
	"time"

	nvidia_clock "github.com/leptonai/gpud/components/accelerator/nvidia/clock"
	nvidia_clockspeed "github.com/leptonai/gpud/components/accelerator/nvidia/clock-speed"
	nvidia_ecc "github.com/leptonai/gpud/components/accelerator/nvidia/ecc"
	nvidia_error "github.com/leptonai/gpud/components/accelerator/nvidia/error"
	nvidia_component_error_xid_sxid_id "github.com/leptonai/gpud/components/accelerator/nvidia/error-xid-sxid/id"
	nvidia_component_error_sxid_id "github.com/leptonai/gpud/components/accelerator/nvidia/error/sxid/id"
	nvidia_component_error_xid_id "github.com/leptonai/gpud/components/accelerator/nvidia/error/xid/id"
	nvidia_fabric_manager "github.com/leptonai/gpud/components/accelerator/nvidia/fabric-manager"
	nvidia_gpm "github.com/leptonai/gpud/components/accelerator/nvidia/gpm"
	nvidia_gsp_firmware_mode_id "github.com/leptonai/gpud/components/accelerator/nvidia/gsp-firmware-mode/id"
	nvidia_infiniband "github.com/leptonai/gpud/components/accelerator/nvidia/infiniband"
	nvidia_info "github.com/leptonai/gpud/components/accelerator/nvidia/info"
	nvidia_memory "github.com/leptonai/gpud/components/accelerator/nvidia/memory"
	nvidia_nccl_id "github.com/leptonai/gpud/components/accelerator/nvidia/nccl/id"
	nvidia_nvlink "github.com/leptonai/gpud/components/accelerator/nvidia/nvlink"
	nvidia_peermem_id "github.com/leptonai/gpud/components/accelerator/nvidia/peermem/id"
	nvidia_persistence_mode_id "github.com/leptonai/gpud/components/accelerator/nvidia/persistence-mode/id"
	nvidia_power "github.com/leptonai/gpud/components/accelerator/nvidia/power"
	nvidia_processes "github.com/leptonai/gpud/components/accelerator/nvidia/processes"
	nvidia_query "github.com/leptonai/gpud/components/accelerator/nvidia/query"
	nvidia_query_nvml "github.com/leptonai/gpud/components/accelerator/nvidia/query/nvml"
	nvidia_remapped_rows "github.com/leptonai/gpud/components/accelerator/nvidia/remapped-rows"
	nvidia_temperature "github.com/leptonai/gpud/components/accelerator/nvidia/temperature"
	nvidia_utilization "github.com/leptonai/gpud/components/accelerator/nvidia/utilization"
	containerd_pod "github.com/leptonai/gpud/components/containerd/pod"
	"github.com/leptonai/gpud/components/cpu"
	"github.com/leptonai/gpud/components/disk"
	"github.com/leptonai/gpud/components/dmesg"
	docker_container "github.com/leptonai/gpud/components/docker/container"
	"github.com/leptonai/gpud/components/fd"
	"github.com/leptonai/gpud/components/file"
	"github.com/leptonai/gpud/components/info"
	k8s_pod "github.com/leptonai/gpud/components/k8s/pod"
	"github.com/leptonai/gpud/components/library"
	"github.com/leptonai/gpud/components/memory"
	network_latency "github.com/leptonai/gpud/components/network/latency"
	"github.com/leptonai/gpud/components/os"
	power_supply "github.com/leptonai/gpud/components/power-supply"
	query_config "github.com/leptonai/gpud/components/query/config"
	component_systemd "github.com/leptonai/gpud/components/systemd"
	"github.com/leptonai/gpud/components/tailscale"
	"github.com/leptonai/gpud/log"
	pkg_file "github.com/leptonai/gpud/pkg/file"
	pkd_systemd "github.com/leptonai/gpud/pkg/systemd"
	"github.com/leptonai/gpud/systemd"
	"github.com/leptonai/gpud/version"

	"github.com/mitchellh/go-homedir"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	DefaultAPIVersion = "v1"
	DefaultGPUdPort   = 15132
)

var (
	DefaultRefreshPeriod             = metav1.Duration{Duration: time.Minute}
	DefaultRetentionPeriod           = metav1.Duration{Duration: 30 * time.Minute}
	DefaultRefreshComponentsInterval = metav1.Duration{Duration: time.Minute}
)

var (
	DefaultNVIDIALibraries = []string{
		// core library for NVML
		// typically symlinked to "libnvidia-ml.so.1" or "libnvidia-ml.so.535.183.06"
		"libnvidia-ml.so",

		// core library for CUDA support
		// typically symlinked to "libcuda.so.1" or "libcuda.so.535.183.06"
		"libcuda.so",
	}
	DefaultNVIDIALibrariesSearchDirs = []string{
		// ref. https://github.com/NVIDIA/nvidia-container-toolkit/blob/main/internal/lookup/library.go#L33-L62
		"/",
		"/usr/lib64",
		"/usr/lib/x86_64-linux-gnu",
		"/usr/lib/aarch64-linux-gnu",
		"/usr/lib/x86_64-linux-gnu/nvidia/current",
		"/usr/lib/aarch64-linux-gnu/nvidia/current",
		"/lib64",
		"/lib/x86_64-linux-gnu",
		"/lib/aarch64-linux-gnu",
		"/lib/x86_64-linux-gnu/nvidia/current",
		"/lib/aarch64-linux-gnu/nvidia/current",
	}
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
			cpu.Name:    nil,
			disk.Name:   disk.DefaultConfig(),
			fd.Name:     nil,
			info.Name:   nil,
			memory.Name: nil,
			os.Name:     nil,
		},

		RetentionPeriod:           DefaultRetentionPeriod,
		RefreshComponentsInterval: DefaultRefreshComponentsInterval,
		Pprof:                     false,

		Web: &Web{
			Enable:        true,
			Admin:         false,
			RefreshPeriod: DefaultRefreshPeriod,
			SincePeriod:   DefaultRetentionPeriod,
		},

		EnableAutoUpdate: true,
	}

	if len(options.FilesToCheck) > 0 {
		cfg.Components[file.Name] = options.FilesToCheck
	}

	if cc, exists := DefaultDockerContainerComponent(ctx, options.DockerIgnoreConnectionErrors); exists {
		cfg.Components[docker_container.Name] = cc
	}
	if cc, exists := DefaultContainerdComponent(ctx); exists {
		cfg.Components[containerd_pod.Name] = cc
	}
	if cc, exists := DefaultK8sPodComponent(ctx, options.KubeletIgnoreConnectionErrors); exists {
		cfg.Components[k8s_pod.Name] = cc
	}

	if _, err := stdos.Stat(power_supply.DefaultBatteryCapacityFile); err == nil {
		cfg.Components[power_supply.Name] = nil
	}

	cc, exists, err := DefaultDmesgComponent(ctx)
	if err != nil {
		return nil, err
	}
	if exists {
		cfg.Components[dmesg.Name] = cc
	}

	cfg.Components[network_latency.Name] = nil

	if runtime.GOOS == "linux" {
		if pkd_systemd.SystemdExists() && pkd_systemd.SystemctlExists() {
			if err := systemd.CreateDefaultEnvFile(); err != nil {
				log.Logger.Debugw("failed to create default systemd env file", "error", err)
			}

			log.Logger.Debugw("auto-detected systemd -- configuring systemd component")

			systemdCfg := component_systemd.DefaultConfig()

			if active, err := pkd_systemd.IsActive("kubelet"); err == nil && active {
				systemdCfg.Units = append(systemdCfg.Units, "kubelet")
			}

			if active, err := pkd_systemd.IsActive("docker"); err == nil && active {
				systemdCfg.Units = append(systemdCfg.Units, "docker")
			}

			if active, err := pkd_systemd.IsActive("tailscaled"); err == nil && active {
				systemdCfg.Units = append(systemdCfg.Units, "tailscaled")
			}

			cfg.Components[component_systemd.Name] = systemdCfg
		}
	} else {
		log.Logger.Debugw("auto-detect systemd not supported -- skipping", "os", runtime.GOOS)
	}

	if runtime.GOOS == "linux" {
		if tailscale.TailscaleExists() {
			log.Logger.Debugw("auto-detected tailscale -- configuring tailscale component")
			cfg.Components[tailscale.Name] = nil
		}
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
					cfg.Components[nvidia_clock.Name] = nil
				} else {
					log.Logger.Infow("auto-detected clock events not supported -- skipping", "error", err)
				}
			} else {
				log.Logger.Warnw("failed to check clock events supported or not", "error", err)
			}
		} else {
			log.Logger.Warnw("old nvidia driver -- skipping clock events in the default config, see https://github.com/NVIDIA/go-nvml/pull/123", "version", driverVersion)
		}

		cfg.Components[nvidia_ecc.Name] = nil
		cfg.Components[nvidia_error.Name] = nil
		if _, ok := cfg.Components[dmesg.Name]; ok {
			cfg.Components[nvidia_component_error_xid_id.Name] = nil
			cfg.Components[nvidia_component_error_sxid_id.Name] = nil
			cfg.Components[nvidia_component_error_xid_sxid_id.Name] = nil
		}
		cfg.Components[nvidia_info.Name] = nil

		cfg.Components[nvidia_clockspeed.Name] = nil
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
			Libraries:  DefaultNVIDIALibraries,
			SearchDirs: DefaultNVIDIALibrariesSearchDirs,
		}

		// optional
		cfg.Components[nvidia_fabric_manager.Name] = nil
		cfg.Components[nvidia_infiniband.Name] = nil
		cfg.Components[nvidia_nccl_id.Name] = nil
		cfg.Components[nvidia_peermem_id.Name] = nil
		cfg.Components[nvidia_persistence_mode_id.Name] = nil
		cfg.Components[nvidia_gsp_firmware_mode_id.Name] = nil
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

func DefaultContainerdComponent(ctx context.Context) (any, bool) {
	if runtime.GOOS != "linux" {
		log.Logger.Debugw("ignoring default containerd pod checking since it's not linux", "os", runtime.GOOS)
		return nil, false
	}

	p, err := pkg_file.LocateExecutable("containerd")
	if p != "" && err == nil {
		log.Logger.Debugw("containerd found in PATH", "path", p)
		return containerd_pod.Config{
			Query:    query_config.DefaultConfig(),
			Endpoint: containerd_pod.DefaultContainerRuntimeEndpoint,
		}, true
	}
	log.Logger.Debugw("containerd not found in PATH -- fallback to containerd run checks", "error", err)

	containerdSocketExists := false
	containerdRunning := false

	if _, err := stdos.Stat(containerd_pod.DefaultSocketFile); err == nil {
		log.Logger.Debugw("containerd default socket file exists, containerd installed", "file", containerd_pod.DefaultSocketFile)
		containerdSocketExists = true
	} else {
		log.Logger.Debugw("containerd default socket file does not exist, skip containerd check", "file", containerd_pod.DefaultSocketFile, "error", err)
	}

	cctx, ccancel := context.WithTimeout(ctx, 5*time.Second)
	defer ccancel()

	if _, _, conn, err := containerd_pod.Connect(cctx, containerd_pod.DefaultContainerRuntimeEndpoint); err == nil {
		log.Logger.Debugw("containerd default cri endpoint open, containerd running", "endpoint", containerd_pod.DefaultContainerRuntimeEndpoint)
		containerdRunning = true
		_ = conn.Close()
	} else {
		log.Logger.Debugw("containerd default cri endpoint not open, skip containerd checking", "endpoint", containerd_pod.DefaultContainerRuntimeEndpoint, "error", err)
	}

	if containerdSocketExists && containerdRunning {
		log.Logger.Debugw("auto-detected containerd -- configuring containerd pod component")
		return containerd_pod.Config{
			Query:    query_config.DefaultConfig(),
			Endpoint: containerd_pod.DefaultContainerRuntimeEndpoint,
		}, true
	}
	return nil, false
}

func DefaultDockerContainerComponent(ctx context.Context, ignoreConnectionErrors bool) (any, bool) {
	p, err := pkg_file.LocateExecutable("docker")
	if p != "" && err == nil {
		log.Logger.Debugw("docker found in PATH", "path", p)
		return docker_container.Config{
			Query: query_config.DefaultConfig(),
		}, true
	}
	log.Logger.Debugw("docker not found in PATH -- fallback to docker run checks", "error", err)

	if docker_container.IsDockerRunning() {
		log.Logger.Debugw("auto-detected docker -- configuring docker container component")
		return docker_container.Config{
			Query:                  query_config.DefaultConfig(),
			IgnoreConnectionErrors: ignoreConnectionErrors,
		}, true
	}
	return nil, false
}

func DefaultK8sPodComponent(ctx context.Context, ignoreConnectionErrors bool) (any, bool) {
	if runtime.GOOS != "linux" {
		log.Logger.Debugw("ignoring default kubelet checking since it's not linux", "os", runtime.GOOS)
		return nil, false
	}

	p, err := pkg_file.LocateExecutable("kubelet")
	if p != "" && err == nil {
		log.Logger.Debugw("kubelet found in PATH", "path", p)
		return k8s_pod.Config{
			Query:                  query_config.DefaultConfig(),
			Port:                   k8s_pod.DefaultKubeletReadOnlyPort,
			IgnoreConnectionErrors: ignoreConnectionErrors,
		}, true
	}
	log.Logger.Debugw("kubelet not found in PATH -- fallback to kubelet run checks", "error", err)

	// check if the TCP port is open/used
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("localhost:%d", k8s_pod.DefaultKubeletReadOnlyPort), 3*time.Second)
	if err != nil {
		log.Logger.Debugw("tcp port is not open", "port", k8s_pod.DefaultKubeletReadOnlyPort, "error", err)
	} else {
		log.Logger.Debugw("tcp port is open", "port", k8s_pod.DefaultKubeletReadOnlyPort)
		conn.Close()

		kerr := k8s_pod.CheckKubeletReadOnlyPort(ctx, k8s_pod.DefaultKubeletReadOnlyPort)
		// check
		if kerr != nil {
			log.Logger.Debugw("kubelet readonly port is not open", "port", k8s_pod.DefaultKubeletReadOnlyPort, "error", kerr)
		} else {
			log.Logger.Debugw("auto-detected kubelet readonly port -- configuring k8s pod components", "port", k8s_pod.DefaultKubeletReadOnlyPort)

			// "k8s_pod" requires kubelet read-only port
			// assume if kubelet is running, it opens the most common read-only port 10255
			return k8s_pod.Config{
				Query:                  query_config.DefaultConfig(),
				Port:                   k8s_pod.DefaultKubeletReadOnlyPort,
				IgnoreConnectionErrors: ignoreConnectionErrors,
			}, true
		}
	}

	return nil, false
}

func DefaultDmesgComponent(ctx context.Context) (any, bool, error) {
	if runtime.GOOS != "linux" {
		log.Logger.Debugw("ignoring default dmesg since it's not linux", "os", runtime.GOOS)
		return nil, false, nil
	}

	asRoot := stdos.Geteuid() == 0 // running as root
	if !asRoot {
		log.Logger.Debugw("auto-detected dmesg but running as root -- skipping")
		return nil, false, nil
	}

	if dmesg.DmesgExists() {
		log.Logger.Debugw("auto-detected dmesg -- configuring dmesg component")
		cfg, err := dmesg.DefaultConfig(ctx)
		if err != nil {
			return nil, false, err
		}
		return cfg, true, nil
	}

	log.Logger.Debugw("dmesg does not exist -- skipping dmesg component")
	return nil, false, nil
}
