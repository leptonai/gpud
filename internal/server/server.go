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
	nvidia_clock "github.com/leptonai/gpud/components/accelerator/nvidia/clock"
	nvidia_clockspeed "github.com/leptonai/gpud/components/accelerator/nvidia/clock-speed"
	nvidia_ecc "github.com/leptonai/gpud/components/accelerator/nvidia/ecc"
	nvidia_error "github.com/leptonai/gpud/components/accelerator/nvidia/error"
	nvidia_error_sxid "github.com/leptonai/gpud/components/accelerator/nvidia/error/sxid"
	nvidia_component_error_sxid_id "github.com/leptonai/gpud/components/accelerator/nvidia/error/sxid/id"
	nvidia_error_xid "github.com/leptonai/gpud/components/accelerator/nvidia/error/xid"
	nvidia_component_error_xid_id "github.com/leptonai/gpud/components/accelerator/nvidia/error/xid/id"
	nvidia_fabric_manager "github.com/leptonai/gpud/components/accelerator/nvidia/fabric-manager"
	nvidia_gpm "github.com/leptonai/gpud/components/accelerator/nvidia/gpm"
	nvidia_gsp_firmware_mode "github.com/leptonai/gpud/components/accelerator/nvidia/gsp-firmware-mode"
	nvidia_gsp_firmware_mode_id "github.com/leptonai/gpud/components/accelerator/nvidia/gsp-firmware-mode/id"
	nvidia_infiniband "github.com/leptonai/gpud/components/accelerator/nvidia/infiniband"
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
	"github.com/leptonai/gpud/components/metrics"
	components_metrics_state "github.com/leptonai/gpud/components/metrics/state"
	network_latency "github.com/leptonai/gpud/components/network/latency"
	"github.com/leptonai/gpud/components/os"
	power_supply "github.com/leptonai/gpud/components/power-supply"
	query_config "github.com/leptonai/gpud/components/query/config"
	query_log_config "github.com/leptonai/gpud/components/query/log/config"
	query_log_state "github.com/leptonai/gpud/components/query/log/state"
	"github.com/leptonai/gpud/components/state"
	component_systemd "github.com/leptonai/gpud/components/systemd"
	"github.com/leptonai/gpud/components/tailscale"
	gpud_config "github.com/leptonai/gpud/config"
	lepconfig "github.com/leptonai/gpud/config"
	_ "github.com/leptonai/gpud/docs/apis"
	"github.com/leptonai/gpud/internal/login"
	"github.com/leptonai/gpud/internal/session"
	"github.com/leptonai/gpud/log"
	"github.com/leptonai/gpud/manager"
)

// Server is the gpud main daemon
type Server struct {
	db                    *sql.DB
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
	db, err := state.Open(stateFile)
	if err != nil {
		return nil, fmt.Errorf("failed to open state file: %w", err)
	}
	fifoPath, err := lepconfig.DefaultFifoFile()
	if err != nil {
		return nil, fmt.Errorf("failed to get fifo path: %w", err)
	}
	s := &Server{
		db:                 db,
		fifoPath:           fifoPath,
		enableAutoUpdate:   config.EnableAutoUpdate,
		autoUpdateExitCode: config.AutoUpdateExitCode,
	}
	defer func() {
		if retErr != nil {
			s.Stop()
		}
	}()

	if err := state.CreateTable(ctx, db); err != nil {
		return nil, fmt.Errorf("failed to create table: %w", err)
	}
	if err := state.CreateAPIVersionTable(ctx, db); err != nil {
		return nil, fmt.Errorf("failed to create api version table: %w", err)
	}
	ver, err := state.UpdateAPIVersionIfNotExists(ctx, db, "v1")
	if err != nil {
		return nil, fmt.Errorf("failed to update api version: %w", err)
	}
	log.Logger.Infow("api version", "version", ver)
	if ver != "v1" {
		return nil, fmt.Errorf("api version mismatch: %s (only supports v1)", ver)
	}

	if err := components_metrics_state.CreateTable(ctx, db, components_metrics_state.DefaultTableName); err != nil {
		return nil, fmt.Errorf("failed to create metrics table: %w", err)
	}
	if err := query_log_state.CreateTable(ctx, db); err != nil {
		return nil, fmt.Errorf("failed to create query log state table: %w", err)
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
				purged, err := components_metrics_state.Purge(ctx, db, components_metrics_state.DefaultTableName, before)
				if err != nil {
					log.Logger.Warnw("failed to purge metrics", "error", err)
				} else {
					log.Logger.Debugw("purged metrics", "purged", purged)
				}
			}
		}
	}()

	defaultStateCfg := query_config.State{DB: db}
	defaultQueryCfg := query_config.Config{State: &defaultStateCfg}
	defaultLogCfg := query_log_config.Config{
		Query: defaultQueryCfg,
		DB:    db,
		SeekInfoSyncer: func(ctx context.Context, file string, seekInfo tail.SeekInfo) {
			if err := query_log_state.Insert(ctx, db, file, seekInfo.Offset, int64(seekInfo.Whence)); err != nil {
				log.Logger.Errorw("failed to sync seek info", "error", err)
			}
		},
	}

	if err := checkDependencies(config); err != nil {
		return nil, fmt.Errorf("dependency check failed: %w", err)
	}

	allComponents := make([]components.Component, 0)
	if _, ok := config.Components[os.Name]; !ok {
		allComponents = append(allComponents, os.New(ctx, os.Config{Query: defaultQueryCfg}))
	}

	for k, configValue := range config.Components {
		switch k {
		case cpu.Name:
			cfg := cpu.Config{Query: defaultQueryCfg}
			if configValue != nil {
				parsed, err := cpu.ParseConfig(configValue, db)
				if err != nil {
					return nil, fmt.Errorf("failed to parse component %s config: %w", k, err)
				}
				cfg = *parsed
			}
			if err := cfg.Validate(); err != nil {
				return nil, fmt.Errorf("failed to validate component %s config: %w", k, err)
			}
			allComponents = append(allComponents, cpu.New(ctx, cfg))

		case disk.Name:
			cfg := disk.Config{Query: defaultQueryCfg}
			if configValue != nil {
				parsed, err := disk.ParseConfig(configValue, db)
				if err != nil {
					return nil, fmt.Errorf("failed to parse component %s config: %w", k, err)
				}
				cfg = *parsed
			}
			if err := cfg.Validate(); err != nil {
				return nil, fmt.Errorf("failed to validate component %s config: %w", k, err)
			}
			allComponents = append(allComponents, disk.New(ctx, cfg))

		case dmesg.Name:
			cfg := dmesg.Config{Log: defaultLogCfg}
			if configValue != nil {
				parsed, err := dmesg.ParseConfig(configValue, db)
				if err != nil {
					return nil, fmt.Errorf("failed to parse component %s config: %w", k, err)
				}
				cfg = *parsed
			}
			if err := cfg.Validate(); err != nil {
				return nil, fmt.Errorf("failed to validate component %s config: %w", k, err)
			}

			// nvidia_error_xid cannot be used without dmesg
			nvrmXidFilterFound := false
			if _, ok := config.Components[nvidia_component_error_xid_id.Name]; ok {
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

			// nvidia_error_sxid cannot be used without dmesg
			nvswitchSXidFilterFound := false
			if _, ok := config.Components[nvidia_component_error_sxid_id.Name]; ok {
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

			c, err := dmesg.New(ctx, cfg)
			if err != nil {
				return nil, fmt.Errorf("failed to create component %s: %w", k, err)
			}
			allComponents = append(allComponents, c)

		case fd.Name:
			cfg := fd.Config{Query: defaultQueryCfg, ThresholdLimit: fd.DefaultThresholdLimit}
			if configValue != nil {
				parsed, err := fd.ParseConfig(configValue, db)
				if err != nil {
					return nil, fmt.Errorf("failed to parse component %s config: %w", k, err)
				}
				cfg = *parsed
			}
			if err := cfg.Validate(); err != nil {
				return nil, fmt.Errorf("failed to validate component %s config: %w", k, err)
			}
			allComponents = append(allComponents, fd.New(ctx, cfg))

		case file.Name:
			if configValue != nil {
				filesToCheck, ok := configValue.([]string)
				if !ok {
					return nil, fmt.Errorf("failed to parse component %s config: %w", k, err)
				}
				allComponents = append(allComponents, file.New(filesToCheck))
			}

		case library.Name:
			if configValue != nil {
				libCfg, ok := configValue.(library.Config)
				if !ok {
					return nil, fmt.Errorf("failed to parse component %s config: %w", k, err)
				}
				allComponents = append(allComponents, library.New(libCfg))
			}

		case info.Name:
			allComponents = append(allComponents, info.New(config.Annotations))

		case memory.Name:
			cfg := memory.Config{Query: defaultQueryCfg}
			if configValue != nil {
				parsed, err := memory.ParseConfig(configValue, db)
				if err != nil {
					return nil, fmt.Errorf("failed to parse component %s config: %w", k, err)
				}
				cfg = *parsed
			}
			if err := cfg.Validate(); err != nil {
				return nil, fmt.Errorf("failed to validate component %s config: %w", k, err)
			}
			allComponents = append(allComponents, memory.New(ctx, cfg))

		case os.Name:
			cfg := os.Config{Query: defaultQueryCfg}
			if configValue != nil {
				parsed, err := os.ParseConfig(configValue, db)
				if err != nil {
					return nil, fmt.Errorf("failed to parse component %s config: %w", k, err)
				}
				cfg = *parsed
			}
			if err := cfg.Validate(); err != nil {
				return nil, fmt.Errorf("failed to validate component %s config: %w", k, err)
			}
			allComponents = append(allComponents, os.New(ctx, cfg))

		case power_supply.Name:
			cfg := power_supply.Config{Query: defaultQueryCfg}
			if configValue != nil {
				parsed, err := power_supply.ParseConfig(configValue, db)
				if err != nil {
					return nil, fmt.Errorf("failed to parse component %s config: %w", k, err)
				}
				cfg = *parsed
			}
			allComponents = append(allComponents, power_supply.New(ctx, cfg))

		case component_systemd.Name:
			cfg := component_systemd.Config{Query: defaultQueryCfg}
			if configValue != nil {
				parsed, err := component_systemd.ParseConfig(configValue, db)
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

		case tailscale.Name:
			cfg := tailscale.Config{Query: defaultQueryCfg}
			if configValue != nil {
				parsed, err := tailscale.ParseConfig(configValue, db)
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
			cfg := nvidia_info.Config{Query: defaultQueryCfg}
			if configValue != nil {
				parsed, err := nvidia_info.ParseConfig(configValue, db)
				if err != nil {
					return nil, fmt.Errorf("failed to parse component %s config: %w", k, err)
				}
				cfg = *parsed
			}
			if err := cfg.Validate(); err != nil {
				return nil, fmt.Errorf("failed to validate component %s config: %w", k, err)
			}
			allComponents = append(allComponents, nvidia_info.New(ctx, cfg))

		case nvidia_badenvs_id.Name:
			cfg := nvidia_badenvs.Config{Query: defaultQueryCfg}
			if configValue != nil {
				parsed, err := nvidia_badenvs.ParseConfig(configValue, db)
				if err != nil {
					return nil, fmt.Errorf("failed to parse component %s config: %w", k, err)
				}
				cfg = *parsed
			}
			if err := cfg.Validate(); err != nil {
				return nil, fmt.Errorf("failed to validate component %s config: %w", k, err)
			}
			allComponents = append(allComponents, nvidia_badenvs.New(ctx, cfg))

		case nvidia_error.Name:
			cfg := nvidia_error.Config{Query: defaultQueryCfg}
			if configValue != nil {
				parsed, err := nvidia_error.ParseConfig(configValue, db)
				if err != nil {
					return nil, fmt.Errorf("failed to parse component %s config: %w", k, err)
				}
				cfg = *parsed
			}
			if err := cfg.Validate(); err != nil {
				return nil, fmt.Errorf("failed to validate component %s config: %w", k, err)
			}
			allComponents = append(allComponents, nvidia_error.New(ctx, cfg))

		case nvidia_component_error_xid_id.Name:
			cfg := nvidia_error_xid.Config{Query: defaultQueryCfg}
			if configValue != nil {
				parsed, err := nvidia_error_xid.ParseConfig(configValue, db)
				if err != nil {
					return nil, fmt.Errorf("failed to parse component %s config: %w", k, err)
				}
				cfg = *parsed
			}
			if err := cfg.Validate(); err != nil {
				return nil, fmt.Errorf("failed to validate component %s config: %w", k, err)
			}
			allComponents = append(allComponents, nvidia_error_xid.New(ctx, cfg))

		case nvidia_component_error_sxid_id.Name:
			allComponents = append(allComponents, nvidia_error_sxid.New())

		case nvidia_clock.Name:
			cfg := nvidia_clock.Config{Query: defaultQueryCfg}
			if configValue != nil {
				parsed, err := nvidia_clock.ParseConfig(configValue, db)
				if err != nil {
					return nil, fmt.Errorf("failed to parse component %s config: %w", k, err)
				}
				cfg = *parsed
			}
			if err := cfg.Validate(); err != nil {
				return nil, fmt.Errorf("failed to validate component %s config: %w", k, err)
			}
			allComponents = append(allComponents, nvidia_clock.New(ctx, cfg))

		case nvidia_clockspeed.Name:
			cfg := nvidia_clockspeed.Config{Query: defaultQueryCfg}
			if configValue != nil {
				parsed, err := nvidia_clockspeed.ParseConfig(configValue, db)
				if err != nil {
					return nil, fmt.Errorf("failed to parse component %s config: %w", k, err)
				}
				cfg = *parsed
			}
			if err := cfg.Validate(); err != nil {
				return nil, fmt.Errorf("failed to validate component %s config: %w", k, err)
			}
			allComponents = append(allComponents, nvidia_clockspeed.New(ctx, cfg))

		case nvidia_ecc.Name:
			cfg := nvidia_ecc.Config{Query: defaultQueryCfg}
			if configValue != nil {
				parsed, err := nvidia_ecc.ParseConfig(configValue, db)
				if err != nil {
					return nil, fmt.Errorf("failed to parse component %s config: %w", k, err)
				}
				cfg = *parsed
			}
			if err := cfg.Validate(); err != nil {
				return nil, fmt.Errorf("failed to validate component %s config: %w", k, err)
			}
			allComponents = append(allComponents, nvidia_ecc.New(ctx, cfg))

		case nvidia_memory.Name:
			cfg := nvidia_memory.Config{Query: defaultQueryCfg}
			if configValue != nil {
				parsed, err := nvidia_memory.ParseConfig(configValue, db)
				if err != nil {
					return nil, fmt.Errorf("failed to parse component %s config: %w", k, err)
				}
				cfg = *parsed
			}
			if err := cfg.Validate(); err != nil {
				return nil, fmt.Errorf("failed to validate component %s config: %w", k, err)
			}
			allComponents = append(allComponents, nvidia_memory.New(ctx, cfg))

		case nvidia_gpm.Name:
			cfg := nvidia_gpm.Config{Query: defaultQueryCfg}
			if configValue != nil {
				parsed, err := nvidia_gpm.ParseConfig(configValue, db)
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
			cfg := nvidia_nvlink.Config{Query: defaultQueryCfg}
			if configValue != nil {
				parsed, err := nvidia_nvlink.ParseConfig(configValue, db)
				if err != nil {
					return nil, fmt.Errorf("failed to parse component %s config: %w", k, err)
				}
				cfg = *parsed
			}
			if err := cfg.Validate(); err != nil {
				return nil, fmt.Errorf("failed to validate component %s config: %w", k, err)
			}
			allComponents = append(allComponents, nvidia_nvlink.New(ctx, cfg))

		case nvidia_power.Name:
			cfg := nvidia_power.Config{Query: defaultQueryCfg}
			if configValue != nil {
				parsed, err := nvidia_power.ParseConfig(configValue, db)
				if err != nil {
					return nil, fmt.Errorf("failed to parse component %s config: %w", k, err)
				}
				cfg = *parsed
			}
			if err := cfg.Validate(); err != nil {
				return nil, fmt.Errorf("failed to validate component %s config: %w", k, err)
			}
			allComponents = append(allComponents, nvidia_power.New(ctx, cfg))

		case nvidia_temperature.Name:
			cfg := nvidia_temperature.Config{Query: defaultQueryCfg}
			if configValue != nil {
				parsed, err := nvidia_temperature.ParseConfig(configValue, db)
				if err != nil {
					return nil, fmt.Errorf("failed to parse component %s config: %w", k, err)
				}
				cfg = *parsed
			}
			if err := cfg.Validate(); err != nil {
				return nil, fmt.Errorf("failed to validate component %s config: %w", k, err)
			}
			allComponents = append(allComponents, nvidia_temperature.New(ctx, cfg))

		case nvidia_utilization.Name:
			cfg := nvidia_utilization.Config{Query: defaultQueryCfg}
			if configValue != nil {
				parsed, err := nvidia_utilization.ParseConfig(configValue, db)
				if err != nil {
					return nil, fmt.Errorf("failed to parse component %s config: %w", k, err)
				}
				cfg = *parsed
			}
			if err := cfg.Validate(); err != nil {
				return nil, fmt.Errorf("failed to validate component %s config: %w", k, err)
			}
			allComponents = append(allComponents, nvidia_utilization.New(ctx, cfg))

		case nvidia_processes.Name:
			cfg := nvidia_processes.Config{Query: defaultQueryCfg}
			if configValue != nil {
				parsed, err := nvidia_processes.ParseConfig(configValue, db)
				if err != nil {
					return nil, fmt.Errorf("failed to parse component %s config: %w", k, err)
				}
				cfg = *parsed
			}
			if err := cfg.Validate(); err != nil {
				return nil, fmt.Errorf("failed to validate component %s config: %w", k, err)
			}
			allComponents = append(allComponents, nvidia_processes.New(ctx, cfg))

		case nvidia_remapped_rows.Name:
			cfg := nvidia_remapped_rows.Config{Query: defaultQueryCfg}
			if configValue != nil {
				parsed, err := nvidia_remapped_rows.ParseConfig(configValue, db)
				if err != nil {
					return nil, fmt.Errorf("failed to parse component %s config: %w", k, err)
				}
				cfg = *parsed
			}
			if err := cfg.Validate(); err != nil {
				return nil, fmt.Errorf("failed to validate component %s config: %w", k, err)
			}
			allComponents = append(allComponents, nvidia_remapped_rows.New(ctx, cfg))

		case nvidia_fabric_manager.Name:
			cfg := nvidia_fabric_manager.Config{Query: defaultQueryCfg, Log: nvidia_fabric_manager.DefaultLogConfig()}
			if configValue != nil {
				parsed, err := nvidia_fabric_manager.ParseConfig(configValue, db)
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
			cfg := nvidia_gsp_firmware_mode.Config{Query: defaultQueryCfg}
			if configValue != nil {
				parsed, err := nvidia_gsp_firmware_mode.ParseConfig(configValue, db)
				if err != nil {
					return nil, fmt.Errorf("failed to parse component %s config: %w", k, err)
				}
				cfg = *parsed
			}
			if err := cfg.Validate(); err != nil {
				return nil, fmt.Errorf("failed to validate component %s config: %w", k, err)
			}
			allComponents = append(allComponents, nvidia_gsp_firmware_mode.New(ctx, cfg))

		case nvidia_infiniband.Name:
			cfg := nvidia_infiniband.Config{Query: defaultQueryCfg}
			if configValue != nil {
				parsed, err := nvidia_infiniband.ParseConfig(configValue, db)
				if err != nil {
					return nil, fmt.Errorf("failed to parse component %s config: %w", k, err)
				}
				cfg = *parsed
			}
			if err := cfg.Validate(); err != nil {
				return nil, fmt.Errorf("failed to validate component %s config: %w", k, err)
			}
			allComponents = append(allComponents, nvidia_infiniband.New(ctx, cfg))

		case nvidia_peermem_id.Name:
			cfg := nvidia_peermem.Config{Query: defaultQueryCfg}
			if configValue != nil {
				parsed, err := nvidia_peermem.ParseConfig(configValue, db)
				if err != nil {
					return nil, fmt.Errorf("failed to parse component %s config: %w", k, err)
				}
				cfg = *parsed
			}
			if err := cfg.Validate(); err != nil {
				return nil, fmt.Errorf("failed to validate component %s config: %w", k, err)
			}
			allComponents = append(allComponents, nvidia_peermem.New(ctx, cfg))

		case nvidia_persistence_mode_id.Name:
			cfg := nvidia_persistence_mode.Config{Query: defaultQueryCfg}
			if configValue != nil {
				parsed, err := nvidia_persistence_mode.ParseConfig(configValue, db)
				if err != nil {
					return nil, fmt.Errorf("failed to parse component %s config: %w", k, err)
				}
				cfg = *parsed
			}
			if err := cfg.Validate(); err != nil {
				return nil, fmt.Errorf("failed to validate component %s config: %w", k, err)
			}
			allComponents = append(allComponents, nvidia_persistence_mode.New(ctx, cfg))

		case nvidia_nccl_id.Name:
			cfg := nvidia_nccl.Config{Query: defaultQueryCfg}
			if configValue != nil {
				parsed, err := nvidia_nccl.ParseConfig(configValue, db)
				if err != nil {
					return nil, fmt.Errorf("failed to parse component %s config: %w", k, err)
				}
				cfg = *parsed
			}
			if err := cfg.Validate(); err != nil {
				return nil, fmt.Errorf("failed to validate component %s config: %w", k, err)
			}
			allComponents = append(allComponents, nvidia_nccl.New(ctx, cfg))

		case containerd_pod.Name:
			cfg := containerd_pod.Config{Query: defaultQueryCfg}
			if configValue != nil {
				parsed, err := containerd_pod.ParseConfig(configValue, db)
				if err != nil {
					return nil, fmt.Errorf("failed to parse component %s config: %w", k, err)
				}
				cfg = *parsed
			}
			if err := cfg.Validate(); err != nil {
				return nil, fmt.Errorf("failed to validate component %s config: %w", k, err)
			}
			allComponents = append(allComponents, containerd_pod.New(ctx, cfg))

		case docker_container.Name:
			cfg := docker_container.Config{Query: defaultQueryCfg}
			if configValue != nil {
				parsed, err := docker_container.ParseConfig(configValue, db)
				if err != nil {
					return nil, fmt.Errorf("failed to parse component %s config: %w", k, err)
				}
				cfg = *parsed
			}
			if err := cfg.Validate(); err != nil {
				return nil, fmt.Errorf("failed to validate component %s config: %w", k, err)
			}
			allComponents = append(allComponents, docker_container.New(ctx, cfg))

		case k8s_pod.Name:
			cfg := k8s_pod.Config{Query: defaultQueryCfg}
			if configValue != nil {
				parsed, err := k8s_pod.ParseConfig(configValue, db)
				if err != nil {
					return nil, fmt.Errorf("failed to parse component %s config: %w", k, err)
				}
				cfg = *parsed
			}
			if err := cfg.Validate(); err != nil {
				return nil, fmt.Errorf("failed to validate component %s config: %w", k, err)
			}
			allComponents = append(allComponents, k8s_pod.New(ctx, cfg))

		case network_latency.Name:
			cfg := network_latency.Config{
				Query:                      defaultQueryCfg,
				GlobalMillisecondThreshold: network_latency.DefaultGlobalMillisecondThreshold,
			}
			if configValue != nil {
				parsed, err := network_latency.ParseConfig(configValue, db)
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

	promReg := prometheus.NewRegistry()

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

			log.Logger.Infow("components status",
				"inflight_components", total,
				"evaluated_healthy_states", healthy,
				"evaluated_unhealthy_states", unhealthy,
				"data_collect_success", getSuccess,
				"data_collect_failed", getFailed,
			)
		}
	}()

	if config.RetentionPeriod.Duration > 0 {
		go func() {
			ticker := time.NewTicker(1) // only first run is 1-ns wait
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					ticker.Reset(config.RetentionPeriod.Duration)
				}

				if err := state.Compact(ctx, db); err != nil {
					log.Logger.Errorw("failed to compact state database", "error", err)
				}
				if err := state.RecordMetrics(ctx, db); err != nil {
					log.Logger.Errorw("failed to record metrics", "error", err)
				}
			}
		}()
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
				if err := prov.RegisterCollectors(promReg, db, components_metrics_state.DefaultTableName); err != nil {
					return nil, fmt.Errorf("failed to register metrics for component %s: %w", c.Name(), err)
				}
			} else {
				log.Logger.Debugw("component does not implement components.PromRegisterer", "component", c.Name())
			}
		} else {
			log.Logger.Debugw("component does not implement interface{ Unwrap() interface{} }", "component", c.Name())
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

	uid, _, err := state.CreateMachineIDIfNotExist(ctx, db, cliUID)
	if err != nil {
		return nil, fmt.Errorf("failed to create machine uid: %w", err)
	}
	s.uid = uid
	if err = state.UpdateComponents(ctx, db, uid, strings.Join(componentNames, ",")); err != nil {
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
					containerd_pod.Name,
					docker_container.Name,
					k8s_pod.Name,
				} {
					if _, ok := componentSet[name]; ok {
						continue
					}

					if cc, exists := lepconfig.DefaultContainerdComponent(ctx); exists {
						ccfg := containerd_pod.Config{Query: defaultQueryCfg}
						if cc != nil {
							parsed, err := containerd_pod.ParseConfig(cc, db)
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
							parsed, err := docker_container.ParseConfig(cc, db)
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
							parsed, err := k8s_pod.ParseConfig(cc, db)
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
							if err := prov.RegisterCollectors(promReg, db, components_metrics_state.DefaultTableName); err != nil {
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
				if err = state.UpdateComponents(ctx, db, s.uid, strings.Join(newComponentNames, ",")); err != nil {
					log.Logger.Errorw("failed to update components", "error", err)
				}

				ghler.componentNamesMu.Lock()
				ghler.componentNames = newComponentNames
				ghler.componentNamesMu.Unlock()
			}
		}()
	}

	go s.updateToken(ctx, db, uid, endpoint)

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

	if err = login.Gossip(endpoint, uid, config.Address); err != nil {
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
	log.Logger.Debugw("closed db", "error", s.db.Close())

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
