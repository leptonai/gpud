package server

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"database/sql"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/http/pprof"
	goOS "os"
	"path"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/gin-contrib/gzip"
	"github.com/gin-gonic/gin"
	"github.com/nxadm/tail"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"

	"github.com/leptonai/gpud/components"
	nvidia_badenvs "github.com/leptonai/gpud/components/accelerator/nvidia/bad-envs"
	nvidia_badenvs_id "github.com/leptonai/gpud/components/accelerator/nvidia/bad-envs/id"
	nvidia_clock_speed "github.com/leptonai/gpud/components/accelerator/nvidia/clock-speed"
	nvidia_clock_speed_id "github.com/leptonai/gpud/components/accelerator/nvidia/clock-speed/id"
	nvidia_common "github.com/leptonai/gpud/components/accelerator/nvidia/common"
	nvidia_ecc "github.com/leptonai/gpud/components/accelerator/nvidia/ecc"
	nvidia_ecc_id "github.com/leptonai/gpud/components/accelerator/nvidia/ecc/id"
	nvidia_error "github.com/leptonai/gpud/components/accelerator/nvidia/error"
	nvidia_error_sxid "github.com/leptonai/gpud/components/accelerator/nvidia/error/sxid"
	nvidia_component_error_sxid_id "github.com/leptonai/gpud/components/accelerator/nvidia/error/sxid/id"
	nvidia_error_xid "github.com/leptonai/gpud/components/accelerator/nvidia/error/xid"
	nvidia_component_error_xid_id "github.com/leptonai/gpud/components/accelerator/nvidia/error/xid/id"
	nvidia_fabric_manager "github.com/leptonai/gpud/components/accelerator/nvidia/fabric-manager"
	nvidia_gpm "github.com/leptonai/gpud/components/accelerator/nvidia/gpm"
	nvidia_gsp_firmware_mode "github.com/leptonai/gpud/components/accelerator/nvidia/gsp-firmware-mode"
	nvidia_gsp_firmware_mode_id "github.com/leptonai/gpud/components/accelerator/nvidia/gsp-firmware-mode/id"
	nvidia_hw_slowdown "github.com/leptonai/gpud/components/accelerator/nvidia/hw-slowdown"
	nvidia_hw_slowdown_id "github.com/leptonai/gpud/components/accelerator/nvidia/hw-slowdown/id"
	nvidia_infiniband "github.com/leptonai/gpud/components/accelerator/nvidia/infiniband"
	nvidia_infiniband_id "github.com/leptonai/gpud/components/accelerator/nvidia/infiniband/id"
	nvidia_info "github.com/leptonai/gpud/components/accelerator/nvidia/info"
	nvidia_memory "github.com/leptonai/gpud/components/accelerator/nvidia/memory"
	nvidia_nccl "github.com/leptonai/gpud/components/accelerator/nvidia/nccl"
	nvidia_nccl_id "github.com/leptonai/gpud/components/accelerator/nvidia/nccl/id"
	nvidia_nvlink "github.com/leptonai/gpud/components/accelerator/nvidia/nvlink"
	nvidia_peermem "github.com/leptonai/gpud/components/accelerator/nvidia/peermem"
	nvidia_peermem_id "github.com/leptonai/gpud/components/accelerator/nvidia/peermem/id"
	nvidia_persistence_mode "github.com/leptonai/gpud/components/accelerator/nvidia/persistence-mode"
	nvidia_persistence_mode_id "github.com/leptonai/gpud/components/accelerator/nvidia/persistence-mode/id"
	nvidia_power "github.com/leptonai/gpud/components/accelerator/nvidia/power"
	nvidia_power_id "github.com/leptonai/gpud/components/accelerator/nvidia/power/id"
	nvidia_processes "github.com/leptonai/gpud/components/accelerator/nvidia/processes"
	nvidia_query "github.com/leptonai/gpud/components/accelerator/nvidia/query"
	nvidia_query_nvml "github.com/leptonai/gpud/components/accelerator/nvidia/query/nvml"
	nvidia_remapped_rows "github.com/leptonai/gpud/components/accelerator/nvidia/remapped-rows"
	nvidia_temperature "github.com/leptonai/gpud/components/accelerator/nvidia/temperature"
	nvidia_utilization "github.com/leptonai/gpud/components/accelerator/nvidia/utilization"
	containerd_pod "github.com/leptonai/gpud/components/containerd/pod"
	containerd_pod_id "github.com/leptonai/gpud/components/containerd/pod/id"
	"github.com/leptonai/gpud/components/cpu"
	cpu_id "github.com/leptonai/gpud/components/cpu/id"
	events_db "github.com/leptonai/gpud/components/db"
	"github.com/leptonai/gpud/components/disk"
	disk_id "github.com/leptonai/gpud/components/disk/id"
	"github.com/leptonai/gpud/components/dmesg"
	docker_container "github.com/leptonai/gpud/components/docker/container"
	docker_container_id "github.com/leptonai/gpud/components/docker/container/id"
	"github.com/leptonai/gpud/components/fd"
	fd_id "github.com/leptonai/gpud/components/fd/id"
	"github.com/leptonai/gpud/components/file"
	file_id "github.com/leptonai/gpud/components/file/id"
	"github.com/leptonai/gpud/components/fuse"
	fuse_id "github.com/leptonai/gpud/components/fuse/id"
	"github.com/leptonai/gpud/components/info"
	info_id "github.com/leptonai/gpud/components/info/id"
	k8s_pod "github.com/leptonai/gpud/components/k8s/pod"
	k8s_pod_id "github.com/leptonai/gpud/components/k8s/pod/id"
	kernel_module "github.com/leptonai/gpud/components/kernel-module"
	kernel_module_id "github.com/leptonai/gpud/components/kernel-module/id"
	"github.com/leptonai/gpud/components/library"
	library_id "github.com/leptonai/gpud/components/library/id"
	"github.com/leptonai/gpud/components/memory"
	memory_id "github.com/leptonai/gpud/components/memory/id"
	"github.com/leptonai/gpud/components/metrics"
	components_metrics_state "github.com/leptonai/gpud/components/metrics/state"
	network_latency "github.com/leptonai/gpud/components/network/latency"
	network_latency_id "github.com/leptonai/gpud/components/network/latency/id"
	"github.com/leptonai/gpud/components/os"
	os_id "github.com/leptonai/gpud/components/os/id"
	"github.com/leptonai/gpud/components/pci"
	pci_id "github.com/leptonai/gpud/components/pci/id"
	power_supply "github.com/leptonai/gpud/components/power-supply"
	power_supply_id "github.com/leptonai/gpud/components/power-supply/id"
	query_config "github.com/leptonai/gpud/components/query/config"
	"github.com/leptonai/gpud/components/query/log/common"
	query_log_config "github.com/leptonai/gpud/components/query/log/config"
	query_log_state "github.com/leptonai/gpud/components/query/log/state"
	"github.com/leptonai/gpud/components/state"
	component_systemd "github.com/leptonai/gpud/components/systemd"
	systemd_id "github.com/leptonai/gpud/components/systemd/id"
	"github.com/leptonai/gpud/components/tailscale"
	tailscale_id "github.com/leptonai/gpud/components/tailscale/id"
	gpud_config "github.com/leptonai/gpud/config"
	lepconfig "github.com/leptonai/gpud/config"
	_ "github.com/leptonai/gpud/docs/apis"
	"github.com/leptonai/gpud/internal/login"
	"github.com/leptonai/gpud/internal/session"
	"github.com/leptonai/gpud/log"
	"github.com/leptonai/gpud/manager"
	"github.com/leptonai/gpud/pkg/sqlite"
)

// Server is the gpud main daemon
type Server struct {
	dbRW *sql.DB
	dbRO *sql.DB

	nvidiaComponentsExist bool
	uid                   string
	fifoPath              string
	fifo                  *goOS.File
	session               *session.Session
	enableAutoUpdate      bool
	autoUpdateExitCode    int
}

func New(ctx context.Context, config *lepconfig.Config, endpoint string, cliUID string, packageManager *manager.Manager, opts ...gpud_config.OpOption) (_ *Server, retErr error) {
	options := &gpud_config.Op{}
	if err := options.ApplyOpts(opts); err != nil {
		return nil, err
	}

	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("failed to validate config: %w", err)
	}

	stateFile := ":memory:"
	if config.State != "" {
		stateFile = config.State
	}
	dbRW, err := sqlite.Open(stateFile)
	if err != nil {
		return nil, fmt.Errorf("failed to open state file (for read-write): %w", err)
	}
	dbRO, err := sqlite.Open(stateFile, sqlite.WithReadOnly(true))
	if err != nil {
		return nil, fmt.Errorf("failed to open state file (for read-only): %w", err)
	}

	promReg := prometheus.NewRegistry()
	if err := sqlite.Register(promReg); err != nil {
		return nil, fmt.Errorf("failed to register sqlite metrics: %w", err)
	}

	fifoPath, err := lepconfig.DefaultFifoFile()
	if err != nil {
		return nil, fmt.Errorf("failed to get fifo path: %w", err)
	}
	s := &Server{
		dbRW: dbRW,
		dbRO: dbRO,

		fifoPath:           fifoPath,
		enableAutoUpdate:   config.EnableAutoUpdate,
		autoUpdateExitCode: config.AutoUpdateExitCode,
	}
	defer func() {
		if retErr != nil {
			s.Stop()
		}
	}()

	nvidiaInstalled, err := nvidia_query.GPUsInstalled(ctx)
	if err != nil {
		return nil, err
	}
	var eventsStoreNvidiaErrorXid events_db.Store
	var eventsStoreNvidiaHWSlowdown events_db.Store
	if runtime.GOOS == "linux" && nvidiaInstalled {
		eventsStoreNvidiaErrorXid, err = events_db.NewStore(
			dbRW,
			dbRO,
			events_db.CreateDefaultTableName(nvidia_component_error_xid_id.Name),
			3*24*time.Hour,
		)
		if err != nil {
			return nil, err
		}
		eventsStoreNvidiaHWSlowdown, err = events_db.NewStore(
			dbRW,
			dbRO,
			events_db.CreateDefaultTableName(nvidia_hw_slowdown_id.Name),
			3*24*time.Hour,
		)
		if err != nil {
			return nil, err
		}
		nvidia_query.SetDefaultPoller(
			nvidia_query.WithDBRW(dbRW), // to deprecate in favor of events store
			nvidia_query.WithDBRO(dbRO), // to deprecate in favor of events store
			nvidia_query.WithXidEventsStore(eventsStoreNvidiaErrorXid),
			nvidia_query.WithHWSlowdownEventsStore(eventsStoreNvidiaHWSlowdown),
			nvidia_query.WithNvidiaSMICommand(options.NvidiaSMICommand),
			nvidia_query.WithNvidiaSMIQueryCommand(options.NvidiaSMIQueryCommand),
			nvidia_query.WithIbstatCommand(options.IbstatCommand),
		)
	}

	if err := state.CreateTableMachineMetadata(ctx, dbRW); err != nil {
		return nil, fmt.Errorf("failed to create table: %w", err)
	}
	if err := state.CreateTableAPIVersion(ctx, dbRW); err != nil {
		return nil, fmt.Errorf("failed to create api version table: %w", err)
	}
	ver, err := state.UpdateAPIVersionIfNotExists(ctx, dbRW, "v1")
	if err != nil {
		return nil, fmt.Errorf("failed to update api version: %w", err)
	}
	log.Logger.Infow("api version", "version", ver)
	if ver != "v1" {
		return nil, fmt.Errorf("api version mismatch: %s (only supports v1)", ver)
	}

	if err := query_log_state.CreateTableLogFileSeekInfo(ctx, dbRW); err != nil {
		return nil, fmt.Errorf("failed to create query log state table: %w", err)
	}

	if err := components_metrics_state.CreateTableMetrics(ctx, dbRW, components_metrics_state.DefaultTableName); err != nil {
		return nil, fmt.Errorf("failed to create metrics table: %w", err)
	}
	go func() {
		dur := config.RetentionPeriod.Duration
		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(dur):
				now := time.Now().UTC()
				before := now.Add(-dur)
				purged, err := components_metrics_state.PurgeMetrics(ctx, dbRW, components_metrics_state.DefaultTableName, before)
				if err != nil {
					log.Logger.Warnw("failed to purge metrics", "error", err)
				} else {
					log.Logger.Debugw("purged metrics", "purged", purged)
				}
			}
		}
	}()

	defaultQueryCfg := query_config.Config{
		State: &query_config.State{
			DBRW: dbRW,
			DBRO: dbRO,
		},
	}
	defaultLogCfg := query_log_config.Config{
		Query: defaultQueryCfg,
		SeekInfoSyncer: func(ctx context.Context, file string, seekInfo tail.SeekInfo) {
			if err := query_log_state.InsertLogFileSeekInfo(ctx, dbRW, file, seekInfo.Offset, int64(seekInfo.Whence)); err != nil {
				log.Logger.Errorw("failed to sync seek info", "error", err)
			}
		},
	}

	if err := checkDependencies(config); err != nil {
		return nil, fmt.Errorf("dependency check failed: %w", err)
	}

	allComponents := make([]components.Component, 0)
	if _, ok := config.Components[os_id.Name]; !ok {
		c, err := os.New(ctx, os.Config{Query: defaultQueryCfg})
		if err != nil {
			return nil, fmt.Errorf("failed to create component %s: %w", os_id.Name, err)
		}
		allComponents = append(allComponents, c)
	}

	for k, configValue := range config.Components {
		switch k {
		case cpu_id.Name:
			cfg := cpu.Config{Query: defaultQueryCfg}
			if configValue != nil {
				parsed, err := cpu.ParseConfig(configValue, dbRW, dbRO)
				if err != nil {
					return nil, fmt.Errorf("failed to parse component %s config: %w", k, err)
				}
				cfg = *parsed
			}
			if err := cfg.Validate(); err != nil {
				return nil, fmt.Errorf("failed to validate component %s config: %w", k, err)
			}
			allComponents = append(allComponents, cpu.New(ctx, cfg))

		case disk_id.Name:
			cfg := disk.Config{Query: defaultQueryCfg}
			if configValue != nil {
				parsed, err := disk.ParseConfig(configValue, dbRW, dbRO)
				if err != nil {
					return nil, fmt.Errorf("failed to parse component %s config: %w", k, err)
				}
				cfg = *parsed
			}
			if err := cfg.Validate(); err != nil {
				return nil, fmt.Errorf("failed to validate component %s config: %w", k, err)
			}
			allComponents = append(allComponents, disk.New(ctx, cfg))

		case fuse_id.Name:
			cfg := fuse.Config{
				Query:                                defaultQueryCfg,
				CongestedPercentAgainstThreshold:     fuse.DefaultCongestedPercentAgainstThreshold,
				MaxBackgroundPercentAgainstThreshold: fuse.DefaultMaxBackgroundPercentAgainstThreshold,
			}
			if configValue != nil {
				parsed, err := fuse.ParseConfig(configValue, dbRW, dbRO)
				if err != nil {
					return nil, fmt.Errorf("failed to parse component %s config: %w", k, err)
				}
				cfg = *parsed
			}
			if err := cfg.Validate(); err != nil {
				return nil, fmt.Errorf("failed to validate component %s config: %w", k, err)
			}
			c, err := fuse.New(ctx, cfg)
			if err != nil {
				return nil, fmt.Errorf("failed to create component %s: %w", k, err)
			}
			allComponents = append(allComponents, c)

		case pci_id.Name:
			cfg := pci.Config{Query: defaultQueryCfg}
			if configValue != nil {
				parsed, err := pci.ParseConfig(configValue, dbRW, dbRO)
				if err != nil {
					return nil, fmt.Errorf("failed to parse component %s config: %w", k, err)
				}
				cfg = *parsed
			}
			c, err := pci.New(ctx, cfg)
			if err != nil {
				return nil, fmt.Errorf("failed to create component %s: %w", k, err)
			}
			allComponents = append(allComponents, c)

		case dmesg.Name:
			// "defaultQueryCfg" here has the db object to write/insert xid/sxid events (write-only, reads are done in individual components)
			cfg := dmesg.Config{Log: defaultLogCfg}
			if configValue != nil {
				parsed, err := dmesg.ParseConfig(configValue, dbRW, dbRO)
				if err != nil {
					return nil, fmt.Errorf("failed to parse component %s config: %w", k, err)
				}

				parsed.Log.SeekInfoSyncer = func(ctx context.Context, file string, seekInfo tail.SeekInfo) {
					if err := query_log_state.InsertLogFileSeekInfo(ctx, dbRW, file, seekInfo.Offset, int64(seekInfo.Whence)); err != nil {
						log.Logger.Errorw("failed to sync seek info", "error", err)
					}
				}

				cfg = *parsed
			}
			if err := cfg.Validate(); err != nil {
				return nil, fmt.Errorf("failed to validate component %s config: %w", k, err)
			}

			if _, ok := config.Components[nvidia_component_error_xid_id.Name]; ok {
				// nvidia_error_xid cannot be used without dmesg
				nvrmXidFilterFound := false

				for _, f := range cfg.Log.SelectFilters {
					if f.Name == dmesg.EventNvidiaNVRMXid {
						nvrmXidFilterFound = true
						break
					}
				}
				if !nvrmXidFilterFound {
					return nil, fmt.Errorf("%q enabled but dmesg config missing %q filter", nvidia_component_error_xid_id.Name, dmesg.EventNvidiaNVRMXid)
				}
			}

			if _, ok := config.Components[nvidia_component_error_sxid_id.Name]; ok {
				// nvidia_error_sxid cannot be used without dmesg
				nvswitchSXidFilterFound := false

				for _, f := range cfg.Log.SelectFilters {
					if f.Name == dmesg.EventNvidiaNVSwitchSXid {
						nvswitchSXidFilterFound = true
						break
					}
				}
				if !nvswitchSXidFilterFound {
					return nil, fmt.Errorf("%q enabled but dmesg config missing %q filter", nvidia_component_error_sxid_id.Name, dmesg.EventNvidiaNVSwitchSXid)
				}
			}

			c, err := dmesg.New(ctx, cfg, func(parsedTime time.Time, line []byte, filter *common.Filter) {})
			if err != nil {
				return nil, fmt.Errorf("failed to create component %s: %w", k, err)
			}
			allComponents = append(allComponents, c)

		case fd_id.Name:
			cfg := fd.Config{
				Query:                         defaultQueryCfg,
				ThresholdAllocatedFileHandles: fd.DefaultThresholdAllocatedFileHandles,
				ThresholdRunningPIDs:          fd.DefaultThresholdRunningPIDs,
			}
			if configValue != nil {
				parsed, err := fd.ParseConfig(configValue, dbRW, dbRO)
				if err != nil {
					return nil, fmt.Errorf("failed to parse component %s config: %w", k, err)
				}
				cfg = *parsed
			}
			if err := cfg.Validate(); err != nil {
				return nil, fmt.Errorf("failed to validate component %s config: %w", k, err)
			}
			allComponents = append(allComponents, fd.New(ctx, cfg))

		case file_id.Name:
			if configValue != nil {
				filesToCheck, ok := configValue.([]string)
				if !ok {
					return nil, fmt.Errorf("failed to parse component %s config: %w", k, err)
				}
				allComponents = append(allComponents, file.New(filesToCheck))
			}

		case kernel_module_id.Name:
			kernelModulesToCheck := []string{}
			if configValue != nil {
				var ok bool
				kernelModulesToCheck, ok = configValue.([]string)
				if !ok {
					return nil, fmt.Errorf("failed to parse component %s config: %w", k, err)
				}
			}
			allComponents = append(allComponents, kernel_module.New(kernelModulesToCheck))

		case library_id.Name:
			if configValue != nil {
				libCfg, ok := configValue.(library.Config)
				if !ok {
					return nil, fmt.Errorf("failed to parse component %s config: %w", k, err)
				}
				allComponents = append(allComponents, library.New(libCfg))
			}

		case info_id.Name:
			allComponents = append(allComponents, info.New(config.Annotations, dbRO, promReg))

		case memory_id.Name:
			cfg := memory.Config{Query: defaultQueryCfg}
			if configValue != nil {
				parsed, err := memory.ParseConfig(configValue, dbRW, dbRO)
				if err != nil {
					return nil, fmt.Errorf("failed to parse component %s config: %w", k, err)
				}
				cfg = *parsed
			}
			if err := cfg.Validate(); err != nil {
				return nil, fmt.Errorf("failed to validate component %s config: %w", k, err)
			}
			c, err := memory.New(ctx, cfg)
			if err != nil {
				return nil, fmt.Errorf("failed to create component %s: %w", k, err)
			}
			allComponents = append(allComponents, c)

		case os_id.Name:
			cfg := os.Config{Query: defaultQueryCfg}
			if configValue != nil {
				parsed, err := os.ParseConfig(configValue, dbRW, dbRO)
				if err != nil {
					return nil, fmt.Errorf("failed to parse component %s config: %w", k, err)
				}
				cfg = *parsed
			}
			if err := cfg.Validate(); err != nil {
				return nil, fmt.Errorf("failed to validate component %s config: %w", k, err)
			}
			c, err := os.New(ctx, cfg)
			if err != nil {
				return nil, fmt.Errorf("failed to create component %s: %w", k, err)
			}
			allComponents = append(allComponents, c)

		case power_supply_id.Name:
			cfg := power_supply.Config{Query: defaultQueryCfg}
			if configValue != nil {
				parsed, err := power_supply.ParseConfig(configValue, dbRW, dbRO)
				if err != nil {
					return nil, fmt.Errorf("failed to parse component %s config: %w", k, err)
				}
				cfg = *parsed
			}
			allComponents = append(allComponents, power_supply.New(ctx, cfg))

		case systemd_id.Name:
			cfg := component_systemd.Config{Query: defaultQueryCfg}
			if configValue != nil {
				parsed, err := component_systemd.ParseConfig(configValue, dbRW, dbRO)
				if err != nil {
					return nil, fmt.Errorf("failed to parse component %s config: %w", k, err)
				}
				cfg = *parsed
			}
			if err := cfg.Validate(); err != nil {
				return nil, fmt.Errorf("failed to validate component %s config: %w", k, err)
			}
			c, err := component_systemd.New(ctx, cfg)
			if err != nil {
				return nil, fmt.Errorf("failed to create component %s: %w", k, err)
			}
			allComponents = append(allComponents, c)

		case tailscale_id.Name:
			cfg := tailscale.Config{Query: defaultQueryCfg}
			if configValue != nil {
				parsed, err := tailscale.ParseConfig(configValue, dbRW, dbRO)
				if err != nil {
					return nil, fmt.Errorf("failed to parse component %s config: %w", k, err)
				}
				cfg = *parsed
			}
			if err := cfg.Validate(); err != nil {
				return nil, fmt.Errorf("failed to validate component %s config: %w", k, err)
			}
			allComponents = append(allComponents, tailscale.New(ctx, cfg))

		case nvidia_info.Name:
			cfg := nvidia_common.Config{Query: defaultQueryCfg, ToolOverwrites: options.ToolOverwrites}
			if configValue != nil {
				parsed, err := nvidia_common.ParseConfig(configValue, dbRW, dbRO)
				if err != nil {
					return nil, fmt.Errorf("failed to parse component %s config: %w", k, err)
				}
				cfg = *parsed
			}
			if err := cfg.Validate(); err != nil {
				return nil, fmt.Errorf("failed to validate component %s config: %w", k, err)
			}
			c, err := nvidia_info.New(ctx, cfg)
			if err != nil {
				return nil, fmt.Errorf("failed to create component %s: %w", k, err)
			}
			allComponents = append(allComponents, c)

		case nvidia_badenvs_id.Name:
			cfg := nvidia_common.Config{Query: defaultQueryCfg, ToolOverwrites: options.ToolOverwrites}
			if configValue != nil {
				parsed, err := nvidia_common.ParseConfig(configValue, dbRW, dbRO)
				if err != nil {
					return nil, fmt.Errorf("failed to parse component %s config: %w", k, err)
				}
				cfg = *parsed
			}
			if err := cfg.Validate(); err != nil {
				return nil, fmt.Errorf("failed to validate component %s config: %w", k, err)
			}
			c, err := nvidia_badenvs.New(ctx, cfg)
			if err != nil {
				return nil, fmt.Errorf("failed to create component %s: %w", k, err)
			}
			allComponents = append(allComponents, c)

		case nvidia_error.Name:
			cfg := nvidia_common.Config{Query: defaultQueryCfg, ToolOverwrites: options.ToolOverwrites}
			if configValue != nil {
				parsed, err := nvidia_common.ParseConfig(configValue, dbRW, dbRO)
				if err != nil {
					return nil, fmt.Errorf("failed to parse component %s config: %w", k, err)
				}
				cfg = *parsed
			}
			if err := cfg.Validate(); err != nil {
				return nil, fmt.Errorf("failed to validate component %s config: %w", k, err)
			}
			c, err := nvidia_error.New(ctx, cfg)
			if err != nil {
				return nil, fmt.Errorf("failed to create component %s: %w", k, err)
			}
			allComponents = append(allComponents, c)

		case nvidia_component_error_xid_id.Name:
			allComponents = append(allComponents, nvidia_error_xid.New(ctx, dbRW, dbRO))

		case nvidia_component_error_sxid_id.Name:
			// db object to read sxid events (read-only, writes are done in poller)
			allComponents = append(allComponents, nvidia_error_sxid.New(ctx, dbRW, dbRO))

		case nvidia_hw_slowdown_id.Name:
			cfg := nvidia_common.Config{Query: defaultQueryCfg, ToolOverwrites: options.ToolOverwrites}
			if configValue != nil {
				parsed, err := nvidia_common.ParseConfig(configValue, dbRW, dbRO)
				if err != nil {
					return nil, fmt.Errorf("failed to parse component %s config: %w", k, err)
				}
				cfg = *parsed
			}
			if err := cfg.Validate(); err != nil {
				return nil, fmt.Errorf("failed to validate component %s config: %w", k, err)
			}
			c, err := nvidia_hw_slowdown.New(ctx, cfg, eventsStoreNvidiaHWSlowdown)
			if err != nil {
				return nil, fmt.Errorf("failed to create component %s: %w", k, err)
			}
			allComponents = append(allComponents, c)

		case nvidia_clock_speed_id.Name:
			cfg := nvidia_common.Config{Query: defaultQueryCfg, ToolOverwrites: options.ToolOverwrites}
			if configValue != nil {
				parsed, err := nvidia_common.ParseConfig(configValue, dbRW, dbRO)
				if err != nil {
					return nil, fmt.Errorf("failed to parse component %s config: %w", k, err)
				}
				cfg = *parsed
			}
			if err := cfg.Validate(); err != nil {
				return nil, fmt.Errorf("failed to validate component %s config: %w", k, err)
			}
			c, err := nvidia_clock_speed.New(ctx, cfg)
			if err != nil {
				return nil, fmt.Errorf("failed to create component %s: %w", k, err)
			}
			allComponents = append(allComponents, c)

		case nvidia_ecc_id.Name:
			cfg := nvidia_common.Config{Query: defaultQueryCfg, ToolOverwrites: options.ToolOverwrites}
			if configValue != nil {
				parsed, err := nvidia_common.ParseConfig(configValue, dbRW, dbRO)
				if err != nil {
					return nil, fmt.Errorf("failed to parse component %s config: %w", k, err)
				}
				cfg = *parsed
			}
			if err := cfg.Validate(); err != nil {
				return nil, fmt.Errorf("failed to validate component %s config: %w", k, err)
			}
			c, err := nvidia_ecc.New(ctx, cfg)
			if err != nil {
				return nil, fmt.Errorf("failed to create component %s: %w", k, err)
			}
			allComponents = append(allComponents, c)

		case nvidia_memory.Name:
			cfg := nvidia_common.Config{Query: defaultQueryCfg, ToolOverwrites: options.ToolOverwrites}
			if configValue != nil {
				parsed, err := nvidia_common.ParseConfig(configValue, dbRW, dbRO)
				if err != nil {
					return nil, fmt.Errorf("failed to parse component %s config: %w", k, err)
				}
				cfg = *parsed
			}
			if err := cfg.Validate(); err != nil {
				return nil, fmt.Errorf("failed to validate component %s config: %w", k, err)
			}
			c, err := nvidia_memory.New(ctx, cfg)
			if err != nil {
				return nil, fmt.Errorf("failed to create component %s: %w", k, err)
			}
			allComponents = append(allComponents, c)

		case nvidia_gpm.Name:
			cfg := nvidia_common.Config{Query: defaultQueryCfg, ToolOverwrites: options.ToolOverwrites}
			if configValue != nil {
				parsed, err := nvidia_common.ParseConfig(configValue, dbRW, dbRO)
				if err != nil {
					return nil, fmt.Errorf("failed to parse component %s config: %w", k, err)
				}
				cfg = *parsed
			}
			if err := cfg.Validate(); err != nil {
				return nil, fmt.Errorf("failed to validate component %s config: %w", k, err)
			}
			allComponents = append(allComponents, nvidia_gpm.New(ctx, cfg))

		case nvidia_nvlink.Name:
			cfg := nvidia_common.Config{Query: defaultQueryCfg, ToolOverwrites: options.ToolOverwrites}
			if configValue != nil {
				parsed, err := nvidia_common.ParseConfig(configValue, dbRW, dbRO)
				if err != nil {
					return nil, fmt.Errorf("failed to parse component %s config: %w", k, err)
				}
				cfg = *parsed
			}
			if err := cfg.Validate(); err != nil {
				return nil, fmt.Errorf("failed to validate component %s config: %w", k, err)
			}
			c, err := nvidia_nvlink.New(ctx, cfg)
			if err != nil {
				return nil, fmt.Errorf("failed to create component %s: %w", k, err)
			}
			allComponents = append(allComponents, c)

		case nvidia_power_id.Name:
			cfg := nvidia_common.Config{Query: defaultQueryCfg, ToolOverwrites: options.ToolOverwrites}
			if configValue != nil {
				parsed, err := nvidia_common.ParseConfig(configValue, dbRW, dbRO)
				if err != nil {
					return nil, fmt.Errorf("failed to parse component %s config: %w", k, err)
				}
				cfg = *parsed
			}
			if err := cfg.Validate(); err != nil {
				return nil, fmt.Errorf("failed to validate component %s config: %w", k, err)
			}
			c, err := nvidia_power.New(ctx, cfg)
			if err != nil {
				return nil, fmt.Errorf("failed to create component %s: %w", k, err)
			}
			allComponents = append(allComponents, c)

		case nvidia_temperature.Name:
			cfg := nvidia_common.Config{Query: defaultQueryCfg, ToolOverwrites: options.ToolOverwrites}
			if configValue != nil {
				parsed, err := nvidia_common.ParseConfig(configValue, dbRW, dbRO)
				if err != nil {
					return nil, fmt.Errorf("failed to parse component %s config: %w", k, err)
				}
				cfg = *parsed
			}
			if err := cfg.Validate(); err != nil {
				return nil, fmt.Errorf("failed to validate component %s config: %w", k, err)
			}
			c, err := nvidia_temperature.New(ctx, cfg)
			if err != nil {
				return nil, fmt.Errorf("failed to create component %s: %w", k, err)
			}
			allComponents = append(allComponents, c)

		case nvidia_utilization.Name:
			cfg := nvidia_common.Config{Query: defaultQueryCfg, ToolOverwrites: options.ToolOverwrites}
			if configValue != nil {
				parsed, err := nvidia_common.ParseConfig(configValue, dbRW, dbRO)
				if err != nil {
					return nil, fmt.Errorf("failed to parse component %s config: %w", k, err)
				}
				cfg = *parsed
			}
			if err := cfg.Validate(); err != nil {
				return nil, fmt.Errorf("failed to validate component %s config: %w", k, err)
			}
			c, err := nvidia_utilization.New(ctx, cfg)
			if err != nil {
				return nil, fmt.Errorf("failed to create component %s: %w", k, err)
			}
			allComponents = append(allComponents, c)

		case nvidia_processes.Name:
			cfg := nvidia_common.Config{Query: defaultQueryCfg, ToolOverwrites: options.ToolOverwrites}
			if configValue != nil {
				parsed, err := nvidia_common.ParseConfig(configValue, dbRW, dbRO)
				if err != nil {
					return nil, fmt.Errorf("failed to parse component %s config: %w", k, err)
				}
				cfg = *parsed
			}
			if err := cfg.Validate(); err != nil {
				return nil, fmt.Errorf("failed to validate component %s config: %w", k, err)
			}
			c, err := nvidia_processes.New(ctx, cfg)
			if err != nil {
				return nil, fmt.Errorf("failed to create component %s: %w", k, err)
			}
			allComponents = append(allComponents, c)

		case nvidia_remapped_rows.Name:
			cfg := nvidia_common.Config{Query: defaultQueryCfg, ToolOverwrites: options.ToolOverwrites}
			if configValue != nil {
				parsed, err := nvidia_common.ParseConfig(configValue, dbRW, dbRO)
				if err != nil {
					return nil, fmt.Errorf("failed to parse component %s config: %w", k, err)
				}
				cfg = *parsed
			}
			if err := cfg.Validate(); err != nil {
				return nil, fmt.Errorf("failed to validate component %s config: %w", k, err)
			}
			c, err := nvidia_remapped_rows.New(ctx, cfg)
			if err != nil {
				return nil, fmt.Errorf("failed to create component %s: %w", k, err)
			}
			allComponents = append(allComponents, c)

		case nvidia_fabric_manager.Name:
			cfg := nvidia_fabric_manager.Config{
				Query:          defaultQueryCfg,
				Log:            nvidia_fabric_manager.DefaultLogConfig(),
				ToolOverwrites: options.ToolOverwrites,
			}
			if configValue != nil {
				parsed, err := nvidia_fabric_manager.ParseConfig(configValue, dbRW, dbRO)
				if err != nil {
					return nil, fmt.Errorf("failed to parse component %s config: %w", k, err)
				}
				cfg = *parsed
			}
			if err := cfg.Validate(); err != nil {
				return nil, fmt.Errorf("failed to validate component %s config: %w", k, err)
			}
			fabricManagerLogComponent, err := nvidia_fabric_manager.New(ctx, cfg)
			if err != nil {
				return nil, fmt.Errorf("failed to create component %s: %w", k, err)
			}
			allComponents = append(allComponents, fabricManagerLogComponent)

		case nvidia_gsp_firmware_mode_id.Name:
			cfg := nvidia_common.Config{Query: defaultQueryCfg, ToolOverwrites: options.ToolOverwrites}
			if configValue != nil {
				parsed, err := nvidia_common.ParseConfig(configValue, dbRW, dbRO)
				if err != nil {
					return nil, fmt.Errorf("failed to parse component %s config: %w", k, err)
				}
				cfg = *parsed
			}
			if err := cfg.Validate(); err != nil {
				return nil, fmt.Errorf("failed to validate component %s config: %w", k, err)
			}
			c, err := nvidia_gsp_firmware_mode.New(ctx, cfg)
			if err != nil {
				return nil, fmt.Errorf("failed to create component %s: %w", k, err)
			}
			allComponents = append(allComponents, c)

		case nvidia_infiniband_id.Name:
			allComponents = append(allComponents, nvidia_infiniband.New(ctx, options.ToolOverwrites))

		case nvidia_peermem_id.Name:
			cfg := nvidia_common.Config{Query: defaultQueryCfg, ToolOverwrites: options.ToolOverwrites}
			if configValue != nil {
				parsed, err := nvidia_common.ParseConfig(configValue, dbRW, dbRO)
				if err != nil {
					return nil, fmt.Errorf("failed to parse component %s config: %w", k, err)
				}
				cfg = *parsed
			}
			if err := cfg.Validate(); err != nil {
				return nil, fmt.Errorf("failed to validate component %s config: %w", k, err)
			}
			c, err := nvidia_peermem.New(ctx, cfg)
			if err != nil {
				return nil, fmt.Errorf("failed to create component %s: %w", k, err)
			}
			allComponents = append(allComponents, c)

		case nvidia_persistence_mode_id.Name:
			cfg := nvidia_common.Config{Query: defaultQueryCfg, ToolOverwrites: options.ToolOverwrites}
			if configValue != nil {
				parsed, err := nvidia_common.ParseConfig(configValue, dbRW, dbRO)
				if err != nil {
					return nil, fmt.Errorf("failed to parse component %s config: %w", k, err)
				}
				cfg = *parsed
			}
			if err := cfg.Validate(); err != nil {
				return nil, fmt.Errorf("failed to validate component %s config: %w", k, err)
			}
			c, err := nvidia_persistence_mode.New(ctx, cfg)
			if err != nil {
				return nil, fmt.Errorf("failed to create component %s: %w", k, err)
			}
			allComponents = append(allComponents, c)

		case nvidia_nccl_id.Name:
			cfg := nvidia_common.Config{Query: defaultQueryCfg, ToolOverwrites: options.ToolOverwrites}
			if configValue != nil {
				parsed, err := nvidia_common.ParseConfig(configValue, dbRW, dbRO)
				if err != nil {
					return nil, fmt.Errorf("failed to parse component %s config: %w", k, err)
				}
				cfg = *parsed
			}
			if err := cfg.Validate(); err != nil {
				return nil, fmt.Errorf("failed to validate component %s config: %w", k, err)
			}
			c, err := nvidia_nccl.New(ctx, cfg)
			if err != nil {
				return nil, fmt.Errorf("failed to create component %s: %w", k, err)
			}
			allComponents = append(allComponents, c)

		case containerd_pod_id.Name:
			cfg := containerd_pod.Config{Query: defaultQueryCfg}
			if configValue != nil {
				parsed, err := containerd_pod.ParseConfig(configValue, dbRW, dbRO)
				if err != nil {
					return nil, fmt.Errorf("failed to parse component %s config: %w", k, err)
				}
				cfg = *parsed
			}
			if err := cfg.Validate(); err != nil {
				return nil, fmt.Errorf("failed to validate component %s config: %w", k, err)
			}
			allComponents = append(allComponents, containerd_pod.New(ctx, cfg))

		case docker_container_id.Name:
			cfg := docker_container.Config{Query: defaultQueryCfg}
			if configValue != nil {
				parsed, err := docker_container.ParseConfig(configValue, dbRW, dbRO)
				if err != nil {
					return nil, fmt.Errorf("failed to parse component %s config: %w", k, err)
				}
				cfg = *parsed
			}
			if err := cfg.Validate(); err != nil {
				return nil, fmt.Errorf("failed to validate component %s config: %w", k, err)
			}
			allComponents = append(allComponents, docker_container.New(ctx, cfg))

		case k8s_pod_id.Name:
			cfg := k8s_pod.Config{Query: defaultQueryCfg}
			if configValue != nil {
				parsed, err := k8s_pod.ParseConfig(configValue, dbRW, dbRO)
				if err != nil {
					return nil, fmt.Errorf("failed to parse component %s config: %w", k, err)
				}
				cfg = *parsed
			}
			if err := cfg.Validate(); err != nil {
				return nil, fmt.Errorf("failed to validate component %s config: %w", k, err)
			}
			allComponents = append(allComponents, k8s_pod.New(ctx, cfg))

		case network_latency_id.Name:
			cfg := network_latency.Config{
				Query:                      defaultQueryCfg,
				GlobalMillisecondThreshold: network_latency.DefaultGlobalMillisecondThreshold,
			}
			if configValue != nil {
				parsed, err := network_latency.ParseConfig(configValue, dbRW, dbRO)
				if err != nil {
					return nil, fmt.Errorf("failed to parse component %s config: %w", k, err)
				}
				cfg = *parsed
			}
			if err := cfg.Validate(); err != nil {
				return nil, fmt.Errorf("failed to validate component %s config: %w", k, err)
			}
			allComponents = append(allComponents, network_latency.New(ctx, cfg))

		default:
			return nil, fmt.Errorf("unknown component %s", k)
		}
	}

	if err := metrics.Register(promReg); err != nil {
		return nil, fmt.Errorf("failed to register metrics: %w", err)
	}
	if err := state.Register(promReg); err != nil {
		return nil, fmt.Errorf("failed to register state metrics: %w", err)
	}
	go func() {
		ticker := time.NewTicker(time.Minute) // only first run is 1-minute wait
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				ticker.Reset(20 * time.Minute)
			}

			total, err := metrics.ReadRegisteredTotal(promReg)
			if err != nil {
				log.Logger.Errorw("failed to get registered total", "error", err)
				continue
			}

			healthy, err := metrics.ReadHealthyTotal(promReg)
			if err != nil {
				log.Logger.Errorw("failed to get registered healthy", "error", err)
				continue
			}

			unhealthy, err := metrics.ReadUnhealthyTotal(promReg)
			if err != nil {
				log.Logger.Errorw("failed to get registered unhealthy", "error", err)
				continue
			}

			getSuccess, err := metrics.ReadGetSuccessTotal(promReg)
			if err != nil {
				log.Logger.Errorw("failed to get success total", "error", err)
				continue
			}

			getFailed, err := metrics.ReadGetFailedTotal(promReg)
			if err != nil {
				log.Logger.Errorw("failed to get failed total", "error", err)
				continue
			}

			log.Logger.Debugw("components status",
				"inflight_components", total,
				"evaluated_healthy_states", healthy,
				"evaluated_unhealthy_states", unhealthy,
				"data_collect_success", getSuccess,
				"data_collect_failed", getFailed,
			)
		}
	}()

	// track metrics every hour
	go func() {
		ticker := time.NewTicker(time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				ticker.Reset(time.Hour)
			}

			if err := state.RecordMetrics(ctx, dbRW); err != nil {
				log.Logger.Errorw("failed to record metrics", "error", err)
			}
		}
	}()

	// compact the state database every retention period
	if config.CompactPeriod.Duration > 0 {
		go func() {
			ticker := time.NewTicker(config.CompactPeriod.Duration)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					ticker.Reset(config.CompactPeriod.Duration)
				}

				if err := sqlite.Compact(ctx, dbRW); err != nil {
					log.Logger.Errorw("failed to compact state database", "error", err)
				}
			}
		}()
	} else {
		log.Logger.Debugw("compact period is not set, skipping compacting")
	}

	for i := range allComponents {
		metrics.SetRegistered(allComponents[i].Name())
		allComponents[i] = metrics.NewWatchableComponent(allComponents[i])
	}

	var componentNames []string
	componentSet := make(map[string]struct{})
	for _, c := range allComponents {
		componentSet[c.Name()] = struct{}{}
		componentNames = append(componentNames, c.Name())
		if strings.Contains(c.Name(), "nvidia") {
			s.nvidiaComponentsExist = true
		}

		// this guarantees no name conflict, thus safe to register handlers by its name
		if err := components.RegisterComponent(c.Name(), c); err != nil {
			log.Logger.Debugw("failed to register component", "name", c.Name(), "error", err)
			continue
		}

		if orig, ok := c.(interface{ Unwrap() interface{} }); ok {
			if prov, ok := orig.Unwrap().(components.PromRegisterer); ok {
				log.Logger.Debugw("registering prometheus collectors", "component", c.Name())
				if err := prov.RegisterCollectors(promReg, dbRW, dbRO, components_metrics_state.DefaultTableName); err != nil {
					return nil, fmt.Errorf("failed to register metrics for component %s: %w", c.Name(), err)
				}
			} else {
				log.Logger.Debugw("component does not implement components.PromRegisterer", "component", c.Name())
			}
		} else {
			log.Logger.Debugw("component does not implement interface{ Unwrap() interface{} }", "component", c.Name())
		}
	}

	for _, c := range allComponents {
		if err = c.Start(); err != nil {
			log.Logger.Errorw("failed to start component", "name", c.Name(), "error", err)
			return nil, fmt.Errorf("failed to start component %s: %w", c.Name(), err)
		}
	}

	// to not start healthz until the initial gpu data is ready
	if s.nvidiaComponentsExist {
		log.Logger.Debugw("waiting for nvml instance to be ready")
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-nvidia_query_nvml.DefaultInstanceReady():
			log.Logger.Debugw("nvml instance is ready")
		}

		log.Logger.Debugw("waiting for first nvidia query to succeed")
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-nvidia_query.GetSuccessOnce():
			log.Logger.Debugw("first nvidia query succeeded")
		}
	}

	uid, err := state.CreateMachineIDIfNotExist(ctx, dbRW, dbRO, cliUID)
	if err != nil {
		return nil, fmt.Errorf("failed to create machine uid: %w", err)
	}
	s.uid = uid
	if err = state.UpdateComponents(ctx, dbRW, uid, strings.Join(componentNames, ",")); err != nil {
		return nil, fmt.Errorf("failed to update components: %w", err)
	}

	// TODO: implement configuration file refresh + apply

	router := gin.Default()
	router.SetHTMLTemplate(rootTmpl)

	cert, err := s.generateSelfSignedCert()
	if err != nil {
		return nil, fmt.Errorf("failed to generate tls cert: %w", err)
	}

	installRootGinMiddlewares(router)
	installCommonGinMiddlewares(router, log.Logger.Desugar())

	v1 := router.Group("/v1")

	// if the request header is set "Accept-Encoding: gzip",
	// the middleware automatically gzip-compresses the response with the response header "Content-Encoding: gzip"
	v1.Use(gzip.Gzip(gzip.DefaultCompression, gzip.WithExcludedPaths([]string{"/update/"})))

	ghler := newGlobalHandler(config, components.GetAllComponents())
	registeredPaths := ghler.registerComponentRoutes(v1)
	for i := range registeredPaths {
		registeredPaths[i].Path = path.Join(v1.BasePath(), registeredPaths[i].Path)
	}

	registeredPaths = append(registeredPaths, componentHandlerDescription{
		Path: "/metrics",
		Desc: "Prometheus metrics",
	})
	promHandler := promhttp.HandlerFor(promReg, promhttp.HandlerOpts{})
	router.GET("/metrics", func(ctx *gin.Context) {
		promHandler.ServeHTTP(ctx.Writer, ctx.Request)
	})

	router.GET(URLPathSwagger, ginSwagger.WrapHandler(swaggerFiles.Handler))
	router.GET(URLPathHealthz, createHealthzHandler())
	registeredPaths = append(registeredPaths, componentHandlerDescription{
		Path: URLPathHealthz,
		Desc: URLPathHealthzDesc,
	})

	admin := router.Group("/admin")

	admin.GET(URLPathConfig, createConfigHandler(config))
	registeredPaths = append(registeredPaths, componentHandlerDescription{
		Path: path.Join("/admin", URLPathConfig),
		Desc: URLPathConfigDesc,
	})
	admin.GET(URLPathPackages, createPackageHandler(packageManager))
	registeredPaths = append(registeredPaths, componentHandlerDescription{
		Path: path.Join("/admin", URLPathPackages),
		Desc: URLPathPackagesDesc,
	})

	if config.Pprof {
		log.Logger.Debugw("registering pprof handlers")
		admin.GET("/pprof/profile", gin.WrapH(http.HandlerFunc(pprof.Profile)))
		admin.GET("/pprof/heap", gin.WrapH(pprof.Handler("heap")))
		admin.GET("/pprof/trace", gin.WrapH(http.HandlerFunc(pprof.Trace)))
	}

	if config.Web != nil && config.Web.Enable {
		router.GET("/", createRootHandler(registeredPaths, *config.Web))

		if config.Web.Enable {
			go func() {
				time.Sleep(2 * time.Second)
				url := "https://" + config.Address
				if !strings.HasPrefix(config.Address, "127.0.0.1") && !strings.HasPrefix(config.Address, "0.0.0.0") && !strings.HasPrefix(config.Address, "localhost") {
					url = "https://localhost" + config.Address
				}
				fmt.Printf("\n\n\n\n\n%s serving %s\n\n\n\n\n", checkMark, url)
			}()
		}
	}

	// refresh components in case containerd, docker, or k8s kubelet starts afterwards
	if config.RefreshComponentsInterval.Duration > 0 {
		go func() {
			ticker := time.NewTicker(config.RefreshComponentsInterval.Duration)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					ticker.Reset(config.RefreshComponentsInterval.Duration)
				}

				componentsToAdd := make([]components.Component, 0)

				// NOTE: systemd unit update still requires gpud restarts
				for _, name := range []string{
					containerd_pod_id.Name,
					docker_container_id.Name,
					k8s_pod_id.Name,
				} {
					if _, ok := componentSet[name]; ok {
						continue
					}

					if cc, exists := lepconfig.DefaultContainerdComponent(ctx); exists {
						ccfg := containerd_pod.Config{Query: defaultQueryCfg}
						if cc != nil {
							parsed, err := containerd_pod.ParseConfig(cc, dbRW, dbRO)
							if err != nil {
								log.Logger.Errorw("failed to parse component %s config: %w", name, err)
								continue
							}
							ccfg = *parsed
						}
						if err := ccfg.Validate(); err != nil {
							log.Logger.Errorw("failed to validate component %s config: %w", name, err)
							continue
						}
						componentsToAdd = append(componentsToAdd, containerd_pod.New(ctx, ccfg))
					}

					if cc, exists := lepconfig.DefaultDockerContainerComponent(ctx, options.DockerIgnoreConnectionErrors); exists {
						ccfg := docker_container.Config{Query: defaultQueryCfg}
						if cc != nil {
							parsed, err := docker_container.ParseConfig(cc, dbRW, dbRO)
							if err != nil {
								log.Logger.Errorw("failed to parse component %s config: %w", name, err)
								continue
							}
							ccfg = *parsed
						}
						if err := ccfg.Validate(); err != nil {
							log.Logger.Errorw("failed to validate component %s config: %w", name, err)
							continue
						}
						componentsToAdd = append(componentsToAdd, docker_container.New(ctx, ccfg))
					}

					if cc, exists := lepconfig.DefaultK8sPodComponent(ctx, options.KubeletIgnoreConnectionErrors); exists {
						ccfg := k8s_pod.Config{Query: defaultQueryCfg}
						if cc != nil {
							parsed, err := k8s_pod.ParseConfig(cc, dbRW, dbRO)
							if err != nil {
								log.Logger.Errorw("failed to parse component %s config: %w", name, err)
								continue
							}
							ccfg = *parsed
						}
						if err := ccfg.Validate(); err != nil {
							log.Logger.Errorw("failed to validate component %s config: %w", name, err)
							continue
						}
						componentsToAdd = append(componentsToAdd, k8s_pod.New(ctx, ccfg))
					}
				}

				if len(componentsToAdd) == 0 {
					continue
				}

				for i := range componentsToAdd {
					if components.IsComponentRegistered(componentsToAdd[i].Name()) {
						continue
					}
					if err := components.RegisterComponent(componentsToAdd[i].Name(), componentsToAdd[i]); err != nil {
						// fails if already registered
						log.Logger.Errorw("failed to register component", "name", componentsToAdd[i].Name(), "error", err)
						continue
					}

					metrics.SetRegistered(componentsToAdd[i].Name())
					componentsToAdd[i] = metrics.NewWatchableComponent(componentsToAdd[i])

					if orig, ok := componentsToAdd[i].(interface{ Unwrap() interface{} }); ok {
						if prov, ok := orig.Unwrap().(components.PromRegisterer); ok {
							log.Logger.Debugw("registering prometheus collectors", "component", componentsToAdd[i].Name())
							if err := prov.RegisterCollectors(promReg, dbRW, dbRO, components_metrics_state.DefaultTableName); err != nil {
								log.Logger.Errorw("failed to register metrics for component", "component", componentsToAdd[i].Name(), "error", err)
							}
						} else {
							log.Logger.Debugw("component does not implement components.PromRegisterer", "component", componentsToAdd[i].Name())
						}
					} else {
						log.Logger.Debugw("component does not implement interface{ Unwrap() interface{} }", "component", componentsToAdd[i].Name())
					}
				}

				newComponentNames := make([]string, len(componentNames))
				copy(newComponentNames, componentNames)
				for _, c := range componentsToAdd {
					newComponentNames = append(newComponentNames, c.Name())
				}
				if err = state.UpdateComponents(ctx, dbRW, s.uid, strings.Join(newComponentNames, ",")); err != nil {
					log.Logger.Errorw("failed to update components", "error", err)
				}

				ghler.componentNamesMu.Lock()
				ghler.componentNames = newComponentNames
				ghler.componentNamesMu.Unlock()
			}
		}()
	}

	go s.updateToken(ctx, dbRW, uid, endpoint)

	go func() {
		srv := &http.Server{
			Addr:    config.Address,
			Handler: router,
			TLSConfig: &tls.Config{
				Certificates: []tls.Certificate{cert},
			},
		}
		log.Logger.Infof("serving %s", config.Address)
		// Start HTTPS server
		err = srv.ListenAndServeTLS("", "")
		if err != nil {
			s.Stop()
			log.Logger.Fatalf("serve %v failure %v", config.Address, err)
		}
	}()

	ghler.componentNamesMu.RLock()
	currComponents := ghler.componentNames
	ghler.componentNamesMu.RUnlock()
	if err = login.Gossip(endpoint, uid, config.Address, currComponents); err != nil {
		log.Logger.Debugf("failed to gossip: %v", err)
	}
	return s, nil
}

const checkMark = "\033[32m✔\033[0m"

func (s *Server) Stop() {
	if s.session != nil {
		s.session.Stop()
	}
	for name, component := range components.GetAllComponents() {
		closer, ok := component.(io.Closer)
		if !ok {
			continue
		}
		if err := closer.Close(); err != nil {
			log.Logger.Errorf("failed to close plugin %v: %v", name, err)
		}
	}

	if cerr := s.dbRW.Close(); cerr != nil {
		log.Logger.Debugw("failed to close read-write db", "error", cerr)
	} else {
		log.Logger.Debugw("successfully closed read-write db")
	}
	if cerr := s.dbRO.Close(); cerr != nil {
		log.Logger.Debugw("failed to close read-only db", "error", cerr)
	} else {
		log.Logger.Debugw("successfully closed read-only db")
	}

	if s.nvidiaComponentsExist {
		serr := nvidia_query_nvml.DefaultInstance().Shutdown()
		if serr != nil {
			log.Logger.Warnw("failed to shutdown NVML", "error", serr)
		}
	}
	if s.fifo != nil {
		if err := s.fifo.Close(); err != nil {
			log.Logger.Errorf("failed to close fifo: %v", err)
		}
	}
	if s.fifoPath != "" {
		if err := goOS.Remove(s.fifoPath); err != nil {
			log.Logger.Errorf("failed to remove fifo: %s", err)
		}
	}
}

func (s *Server) generateSelfSignedCert() (tls.Certificate, error) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, err
	}

	// Create a certificate template
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"Lepton AI"},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	// Create the certificate
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		return tls.Certificate{}, err
	}

	// Encode the certificate and private key to PEM format
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	privDER, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		return tls.Certificate{}, err
	}
	privPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: privDER})

	// Load the certificate
	cert, err := tls.X509KeyPair(certPEM, privPEM)
	if err != nil {
		return tls.Certificate{}, err
	}

	return cert, nil
}

func (s *Server) updateToken(ctx context.Context, db *sql.DB, uid string, endpoint string) {
	var userToken string
	pipePath := s.fifoPath
	if dbToken, err := state.GetLoginInfo(ctx, db, uid); err == nil {
		userToken = dbToken
	}

	if userToken != "" {
		var err error
		s.session, err = session.NewSession(
			ctx,
			fmt.Sprintf("https://%s/api/v1/session", endpoint),
			session.WithMachineID(uid),
			session.WithPipeInterval(3*time.Second),
			session.WithEnableAutoUpdate(s.enableAutoUpdate),
			session.WithAutoUpdateExitCode(s.autoUpdateExitCode),
		)
		if err != nil {
			log.Logger.Errorw("error creating session", "error", err)
		}
	}

	if _, err := goOS.Stat(pipePath); err == nil {
		if err = goOS.Remove(pipePath); err != nil {
			log.Logger.Errorf("error creating pipe: %v", err)
			return
		}
	} else if !goOS.IsNotExist(err) {
		log.Logger.Errorf("error stat pipe: %v", err)
		return
	}

	if err := syscall.Mkfifo(pipePath, 0666); err != nil {
		log.Logger.Errorf("error creating pipe: %v", err)
		return
	}
	for {
		pipe, err := goOS.OpenFile(pipePath, goOS.O_RDONLY, goOS.ModeNamedPipe)
		if err != nil {
			log.Logger.Errorf("error opening named pipe: %v", err)
			return
		}
		buffer := make([]byte, 1024)
		if n, err := pipe.Read(buffer); err != nil {
			log.Logger.Errorf("error reading pipe: %v", err)
		} else {
			userToken = string(buffer[:n])
		}

		pipe.Close()
		if userToken != "" {
			if s.session != nil {
				s.session.Stop()
			}
			s.session, err = session.NewSession(
				ctx,
				fmt.Sprintf("https://%s/api/v1/session", endpoint),
				session.WithMachineID(uid),
				session.WithPipeInterval(3*time.Second),
				session.WithEnableAutoUpdate(s.enableAutoUpdate),
				session.WithAutoUpdateExitCode(s.autoUpdateExitCode),
			)
			if err != nil {
				log.Logger.Errorw("error creating session", "error", err)
			}
		}

		time.Sleep(time.Second)
	}
}

func WriteToken(token string, fifoFile string) error {
	var f *goOS.File
	var err error
	for i := 0; i < 30; i++ {
		if _, err = goOS.Stat(fifoFile); goOS.IsNotExist(err) {
			time.Sleep(1 * time.Second)
			continue
		} else if err != nil {
			return fmt.Errorf("failed to stat fifo file: %w", err)
		}
	}
	if err != nil {
		return fmt.Errorf("server not ready")
	}

	if f, err = goOS.OpenFile(fifoFile, goOS.O_WRONLY, 0600); err != nil {
		return fmt.Errorf("failed to open fifo file: %w", err)
	}
	defer func() {
		if err := f.Close(); err != nil {
			log.Logger.Errorf("failed to close token fifo: %v", err)
		}
	}()

	_, err = f.Write([]byte(token))
	if err != nil {
		return fmt.Errorf("failed to write token: %w", err)
	}
	return nil
}
