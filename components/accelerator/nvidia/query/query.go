// Package query implements various NVIDIA-related system queries.
// All interactions with NVIDIA data sources are implemented under the query packages.
package query

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	metrics_clock "github.com/leptonai/gpud/components/accelerator/nvidia/query/metrics/clock"
	metrics_clockspeed "github.com/leptonai/gpud/components/accelerator/nvidia/query/metrics/clock-speed"
	metrics_ecc "github.com/leptonai/gpud/components/accelerator/nvidia/query/metrics/ecc"
	metrics_memory "github.com/leptonai/gpud/components/accelerator/nvidia/query/metrics/memory"
	metrics_nvlink "github.com/leptonai/gpud/components/accelerator/nvidia/query/metrics/nvlink"
	metrics_power "github.com/leptonai/gpud/components/accelerator/nvidia/query/metrics/power"
	metrics_processes "github.com/leptonai/gpud/components/accelerator/nvidia/query/metrics/processes"
	metrics_remapped_rows "github.com/leptonai/gpud/components/accelerator/nvidia/query/metrics/remapped-rows"
	metrics_temperature "github.com/leptonai/gpud/components/accelerator/nvidia/query/metrics/temperature"
	metrics_utilization "github.com/leptonai/gpud/components/accelerator/nvidia/query/metrics/utilization"
	"github.com/leptonai/gpud/components/accelerator/nvidia/query/nvml"
	"github.com/leptonai/gpud/components/query"
	query_config "github.com/leptonai/gpud/components/query/config"
	"github.com/leptonai/gpud/components/systemd"
	"github.com/leptonai/gpud/log"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"
)

var DefaultPoller = query.New(
	"shared-nvidia-poller",
	query_config.Config{
		Interval:  metav1.Duration{Duration: query_config.DefaultPollInterval},
		QueueSize: query_config.DefaultQueueSize,
		State: &query_config.State{
			Retention: metav1.Duration{Duration: query_config.DefaultStateRetention},
		},
	},
	Get,
)

var (
	getSuccessOnceCloseOnce sync.Once
	getSuccessOnce          = make(chan any)
)

func GetSuccessOnce() <-chan any {
	return getSuccessOnce
}

// Get all nvidia component queries.
func Get(ctx context.Context) (output any, err error) {
	if err := nvml.StartDefaultInstance(ctx); err != nil {
		return nil, err
	}

	o := &Output{
		SMIExists:             SMIExists(),
		PersistencedExists:    PersistencedExists(),
		PersistencedRunning:   PersistencedRunning(),
		FabricManagerExists:   FabricManagerExists(),
		InfinibandClassExists: InfinibandClassExists(),
		IbstatExists:          IbstatExists(),
	}

	o.GPUDeviceCount, err = CountAllDevicesFromDevDir()
	if err != nil {
		log.Logger.Warnw("failed to count gpu devices", "error", err)
	}

	defer func() {
		getSuccessOnceCloseOnce.Do(func() {
			close(getSuccessOnce)
		})
	}()

	if o.SMIExists {
		// call this with a timeout, as a broken GPU may block the command.
		cctx, ccancel := context.WithTimeout(ctx, time.Minute)
		o.SMI, err = GetSMIOutput(cctx)
		ccancel()
		if err != nil {
			o.SMIQueryErrors = append(o.SMIQueryErrors, err.Error())
		}
		if o.SMI != nil && o.SMI.SummaryFailure != nil {
			o.SMIQueryErrors = append(o.SMIQueryErrors, o.SMI.SummaryFailure.Error())
		}
	}

	for k, desc := range nvml.BAD_CUDA_ENV_KEYS {
		if os.Getenv(k) == "1" {
			if o.BadEnvVarsForCUDA == nil {
				o.BadEnvVarsForCUDA = make(map[string]string)
			}
			o.BadEnvVarsForCUDA[k] = desc
		}
	}

	if o.FabricManagerExists {
		cctx, ccancel := context.WithTimeout(ctx, 30*time.Second)
		ver, err := CheckFabricManagerVersion(cctx)
		ccancel()
		if err != nil {
			o.FabricManagerErrors = append(o.FabricManagerErrors, fmt.Sprintf("failed to check fabric manager version: %v", err))
		}

		if err := systemd.ConnectDbus(); err != nil {
			log.Logger.Warnw("failed to connect to dbus", "error", err)

			o.FabricManagerErrors = append(o.FabricManagerErrors, fmt.Sprintf("failed to connect to dbus: %v", err))
		} else {
			active := false
			defaultConn := systemd.GetDefaultDbusConn()

			if defaultConn != nil {
				cctx, ccancel := context.WithTimeout(ctx, 30*time.Second)
				var err error
				active, err = CheckFabricManagerActive(cctx, defaultConn)
				ccancel()
				if err != nil {
					o.FabricManagerErrors = append(o.FabricManagerErrors, fmt.Sprintf("failed to check fabric manager active: %v", err))
				}
			} else {
				o.FabricManagerErrors = append(o.FabricManagerErrors, "systemd dbus connection not available")
			}

			cctx, ccancel = context.WithTimeout(ctx, 30*time.Second)
			journalOut, err := GetLatestFabricManagerOutput(cctx)
			ccancel()
			if err != nil {
				o.FabricManagerErrors = append(o.FabricManagerErrors, fmt.Sprintf("failed to get fabric manager journal output: %v", err))
			}

			o.FabricManager = &FabricManagerOutput{
				Version:       ver,
				Active:        active,
				JournalOutput: journalOut,
			}
		}
	}

	if o.InfinibandClassExists && o.IbstatExists {
		cctx, ccancel := context.WithTimeout(ctx, 30*time.Second)
		o.Ibstat, err = RunIbstat(cctx)
		ccancel()
		if err != nil {
			if o.Ibstat == nil {
				o.Ibstat = &IbstatOutput{}
			}
			o.Ibstat.Errors = append(o.Ibstat.Errors, err.Error())
		}
	}

	cctx, ccancel := context.WithTimeout(ctx, 30*time.Second)
	o.LsmodPeermem, err = CheckLsmodPeermemModule(cctx)
	ccancel()
	if err != nil {
		o.LsmodPeermemErrors = append(o.LsmodPeermemErrors, err.Error())
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-nvml.DefaultInstanceReady():
		log.Logger.Debugw("default nvml instance ready")
	}

	// TODO
	// this may timeout when the GPU is broken
	// e.g.,
	// "nvAssertOkFailedNoLog: Assertion failed: Call timed out [NV_ERR_TIMEOUT]"
	o.NVML, err = nvml.DefaultInstance().Get()
	if err != nil {
		log.Logger.Warnw("nvml get failed", "error", err)
		o.NVMLErrors = append(o.NVMLErrors, err.Error())
	} else {
		now := time.Now().UTC()
		nowUnix := float64(now.Unix())

		metrics_clock.SetLastUpdateUnixSeconds(nowUnix)
		metrics_clockspeed.SetLastUpdateUnixSeconds(nowUnix)
		metrics_ecc.SetLastUpdateUnixSeconds(nowUnix)
		metrics_memory.SetLastUpdateUnixSeconds(nowUnix)
		metrics_nvlink.SetLastUpdateUnixSeconds(nowUnix)
		metrics_power.SetLastUpdateUnixSeconds(nowUnix)
		metrics_temperature.SetLastUpdateUnixSeconds(nowUnix)
		metrics_utilization.SetLastUpdateUnixSeconds(nowUnix)
		metrics_processes.SetLastUpdateUnixSeconds(nowUnix)
		metrics_remapped_rows.SetLastUpdateUnixSeconds(nowUnix)

		for _, dev := range o.NVML.DeviceInfos {
			log.Logger.Debugw("setting metrics for device", "uuid", dev.UUID, "bus", dev.BusID, "device", dev.DeviceID, "minorNumber", dev.MinorNumberID)

			if dev.ClockEvents != nil {
				if err := metrics_clock.SetHWSlowdown(ctx, dev.UUID, dev.ClockEvents.HWSlowdown, now); err != nil {
					return nil, err
				}
				if err := metrics_clock.SetHWSlowdownThermal(ctx, dev.UUID, dev.ClockEvents.HWSlowdownThermal, now); err != nil {
					return nil, err
				}
				if err := metrics_clock.SetHWSlowdownPowerBrake(ctx, dev.UUID, dev.ClockEvents.HWSlowdownPowerBrake, now); err != nil {
					return nil, err
				}
			}

			if err := metrics_clockspeed.SetGraphicsMHz(ctx, dev.UUID, dev.ClockSpeed.GraphicsMHz, now); err != nil {
				return nil, err
			}
			if err := metrics_clockspeed.SetMemoryMHz(ctx, dev.UUID, dev.ClockSpeed.MemoryMHz, now); err != nil {
				return nil, err
			}

			if err := metrics_ecc.SetAggregateTotalCorrected(ctx, dev.UUID, float64(dev.ECCErrors.Aggregate.Total.Corrected), now); err != nil {
				return nil, err
			}
			if err := metrics_ecc.SetAggregateTotalUncorrected(ctx, dev.UUID, float64(dev.ECCErrors.Aggregate.Total.Uncorrected), now); err != nil {
				return nil, err
			}
			if err := metrics_ecc.SetVolatileTotalCorrected(ctx, dev.UUID, float64(dev.ECCErrors.Volatile.Total.Corrected), now); err != nil {
				return nil, err
			}
			if err := metrics_ecc.SetVolatileTotalUncorrected(ctx, dev.UUID, float64(dev.ECCErrors.Volatile.Total.Uncorrected), now); err != nil {
				return nil, err
			}

			if err := metrics_memory.SetTotalBytes(ctx, dev.UUID, float64(dev.Memory.TotalBytes), now); err != nil {
				return nil, err
			}
			metrics_memory.SetReservedBytes(dev.UUID, float64(dev.Memory.ReservedBytes))
			if err := metrics_memory.SetUsedBytes(ctx, dev.UUID, float64(dev.Memory.UsedBytes), now); err != nil {
				return nil, err
			}
			metrics_memory.SetFreeBytes(dev.UUID, float64(dev.Memory.FreeBytes))
			usedPercent, err := dev.Memory.GetUsedPercent()
			if err != nil {
				o.NVMLErrors = append(o.NVMLErrors, err.Error())
			} else {
				if err := metrics_memory.SetUsedPercent(ctx, dev.UUID, usedPercent, now); err != nil {
					return nil, err
				}
			}

			if err := metrics_nvlink.SetFeatureEnabled(ctx, dev.UUID, dev.NVLink.States.AllFeatureEnabled(), now); err != nil {
				return nil, err
			}
			if err := metrics_nvlink.SetReplayErrors(ctx, dev.UUID, dev.NVLink.States.TotalRelayErrors(), now); err != nil {
				return nil, err
			}
			if err := metrics_nvlink.SetRecoveryErrors(ctx, dev.UUID, dev.NVLink.States.TotalRecoveryErrors(), now); err != nil {
				return nil, err
			}
			if err := metrics_nvlink.SetCRCErrors(ctx, dev.UUID, dev.NVLink.States.TotalCRCErrors(), now); err != nil {
				return nil, err
			}
			if err := metrics_nvlink.SetRxBytes(ctx, dev.UUID, float64(dev.NVLink.States.TotalThroughputRawRxBytes()), now); err != nil {
				return nil, err
			}
			if err := metrics_nvlink.SetTxBytes(ctx, dev.UUID, float64(dev.NVLink.States.TotalThroughputRawTxBytes()), now); err != nil {
				return nil, err
			}

			if err := metrics_power.SetUsageMilliWatts(ctx, dev.UUID, float64(dev.Power.UsageMilliWatts), now); err != nil {
				return nil, err
			}
			if err := metrics_power.SetEnforcedLimitMilliWatts(ctx, dev.UUID, float64(dev.Power.EnforcedLimitMilliWatts), now); err != nil {
				return nil, err
			}
			usedPercent, err = dev.Power.GetUsedPercent()
			if err != nil {
				o.NVMLErrors = append(o.NVMLErrors, err.Error())
			} else {
				if err := metrics_power.SetUsedPercent(ctx, dev.UUID, usedPercent, now); err != nil {
					return nil, err
				}
			}

			if err := metrics_temperature.SetCurrentCelsius(ctx, dev.UUID, float64(dev.Temperature.CurrentCelsiusGPUCore), now); err != nil {
				return nil, err
			}
			if err := metrics_temperature.SetThresholdSlowdownCelsius(ctx, dev.UUID, float64(dev.Temperature.ThresholdCelsiusSlowdown), now); err != nil {
				return nil, err
			}
			usedPercent, err = dev.Temperature.GetUsedPercentSlowdown()
			if err != nil {
				o.NVMLErrors = append(o.NVMLErrors, err.Error())
			} else {
				if err := metrics_temperature.SetSlowdownUsedPercent(ctx, dev.UUID, usedPercent, now); err != nil {
					return nil, err
				}
			}

			if err := metrics_utilization.SetGPUUtilPercent(ctx, dev.UUID, dev.Utilization.GPUUsedPercent, now); err != nil {
				return nil, err
			}
			if err := metrics_utilization.SetMemoryUtilPercent(ctx, dev.UUID, dev.Utilization.MemoryUsedPercent, now); err != nil {
				return nil, err
			}

			if err := metrics_processes.SetRunningProcessesTotal(ctx, dev.UUID, len(dev.Processes.RunningProcesses), now); err != nil {
				return nil, err
			}

			if err := metrics_remapped_rows.SetRemappedDueToUncorrectableErrors(ctx, dev.UUID, uint32(dev.RemappedRows.RemappedDueToCorrectableErrors), now); err != nil {
				return nil, err
			}
			if err := metrics_remapped_rows.SetRemappingPending(ctx, dev.UUID, dev.RemappedRows.RemappingPending, now); err != nil {
				return nil, err
			}
			if err := metrics_remapped_rows.SetRemappingFailed(ctx, dev.UUID, dev.RemappedRows.RemappingFailed, now); err != nil {
				return nil, err
			}
		}
	}

	o.MemoryErrorManagementCapabilities = GetMemoryErrorManagementCapabilities(o.GPUProductName())

	return o, nil
}

const (
	StateKeyGPUProductName      = "gpu_product_name"
	StateKeySMIExists           = "smi_exists"
	StateKeyFabricManagerExists = "fabric_manager_exists"
	StateKeyIbstatExists        = "ibstat_exists"
)

type Output struct {
	// GPU device count from the /dev directory.
	GPUDeviceCount int `json:"gpu_device_count"`

	SMIExists      bool       `json:"smi_exists"`
	SMI            *SMIOutput `json:"smi,omitempty"`
	SMIQueryErrors []string   `json:"smi_query_errors,omitempty"`

	// BadEnvVarsForCUDA is a map of environment variables that are known to hurt CUDA.
	// that is set globally for the host.
	// This implements "DCGM_FR_BAD_CUDA_ENV" logic in DCGM.
	BadEnvVarsForCUDA map[string]string `json:"bad_env_vars_for_cuda,omitempty"`

	PersistencedExists  bool `json:"persistenced_exists"`
	PersistencedRunning bool `json:"persistenced_running"`

	FabricManagerExists bool                 `json:"fabric_manager_exists"`
	FabricManager       *FabricManagerOutput `json:"fabric_manager,omitempty"`
	FabricManagerErrors []string             `json:"fabric_manager_errors,omitempty"`

	InfinibandClassExists bool          `json:"infiniband_class_exists"`
	IbstatExists          bool          `json:"ibstat_exists"`
	Ibstat                *IbstatOutput `json:"ibstat,omitempty"`

	LsmodPeermem       *LsmodPeermemModuleOutput `json:"lsmod_peermem,omitempty"`
	LsmodPeermemErrors []string                  `json:"lsmod_peermem_errors,omitempty"`

	NVML       *nvml.Output `json:"nvml,omitempty"`
	NVMLErrors []string     `json:"nvml_errors,omitempty"`

	MemoryErrorManagementCapabilities MemoryErrorManagementCapabilities `json:"memory_error_management_capabilities,omitempty"`
}

func (o *Output) YAML() ([]byte, error) {
	return yaml.Marshal(o)
}

func (o *Output) GPUCount() int {
	if o == nil {
		return 0
	}
	if o.SMI == nil {
		return 0
	}

	cnts := o.SMI.AttachedGPUs

	// in case of "nvidia-smi" failure
	if cnts == 0 && o.NVML != nil && len(o.NVML.DeviceInfos) > 0 {
		cnts = len(o.NVML.DeviceInfos)
	}

	return cnts
}

func (o *Output) GPUCountFromNVML() int {
	if o == nil {
		return 0
	}
	if o.NVML == nil {
		return 0
	}
	return len(o.NVML.DeviceInfos)
}

func (o *Output) GPUProductName() string {
	if o == nil || o.SMI == nil || len(o.SMI.GPUs) == 0 {
		return ""
	}
	if o.SMI.GPUs[0].ProductName != "" {
		return o.SMI.GPUs[0].ProductName
	}
	if o.NVML != nil && len(o.NVML.DeviceInfos) > 0 {
		return o.NVML.DeviceInfos[0].Name
	}
	return ""
}

// This is the same product name in nvidia-smi outputs.
// ref. https://developer.nvidia.com/management-library-nvml
func (o *Output) GPUProductNameFromNVML() string {
	if o == nil {
		return ""
	}
	if o.NVML != nil && len(o.NVML.DeviceInfos) > 0 {
		return o.NVML.DeviceInfos[0].Name
	}
	return ""
}

const (
	inProgress  = "\033[33m⌛\033[0m"
	checkMark   = "\033[32m✔\033[0m"
	warningSign = "\033[31m✘\033[0m"
)

func (o *Output) PrintInfo(debug bool) {
	if len(o.SMIQueryErrors) > 0 {
		fmt.Printf("%s nvidia-smi check failed with %d error(s)\n", warningSign, len(o.SMIQueryErrors))
		for _, err := range o.SMIQueryErrors {
			fmt.Println(err)
		}
	} else {
		fmt.Printf("%s successfully checked nvidia-smi\n", checkMark)
	}

	fmt.Printf("%s GPU device count '%d' (from /dev)\n", checkMark, o.GPUDeviceCount)
	fmt.Printf("%s GPU count '%d' (from NVML)\n", checkMark, o.GPUCountFromNVML())
	fmt.Printf("%s GPU product name '%s' (from NVML)\n", checkMark, o.GPUProductNameFromNVML())

	if len(o.BadEnvVarsForCUDA) > 0 {
		for k, v := range o.BadEnvVarsForCUDA {
			fmt.Printf("%s bad cuda env var: %s=%s\n", warningSign, k, v)
		}
	} else {
		fmt.Printf("%s successfully checked bad cuda env vars (none found)\n", checkMark)
	}

	if o.SMI != nil {
		if errs := o.SMI.FindGPUErrs(); len(errs) > 0 {
			fmt.Printf("%s scanned nvidia-smi -- found %d error(s)\n", warningSign, len(errs))
			for _, err := range errs {
				fmt.Println(err)
			}
		} else {
			fmt.Printf("%s scanned nvidia-smi -- found no error\n", checkMark)
		}

		if errs := o.SMI.FindHWSlowdownErrs(); len(errs) > 0 {
			fmt.Printf("%s scanned nvidia-smi -- found %d hardware slowdown error(s)\n", warningSign, len(errs))
			for _, err := range errs {
				fmt.Println(err)
			}
		} else {
			fmt.Printf("%s scanned nvidia-smi -- found no hardware slowdown error\n", checkMark)
		}
	}

	if len(o.FabricManagerErrors) > 0 {
		fmt.Printf("%s fabric manager check failed with %d error(s)\n", warningSign, len(o.FabricManagerErrors))
		for _, err := range o.FabricManagerErrors {
			fmt.Println(err)
		}
	} else {
		fmt.Printf("%s successfully checked fabric manager\n", checkMark)
	}

	if o.InfinibandClassExists && o.IbstatExists {
		if o.Ibstat != nil && len(o.Ibstat.Errors) > 0 {
			fmt.Printf("%s ibstat check failed with %d error(s)\n", warningSign, len(o.Ibstat.Errors))
			for _, err := range o.Ibstat.Errors {
				fmt.Println(err)
			}
		} else {
			fmt.Printf("%s successfully checked ibstat\n", checkMark)
		}
	} else {
		fmt.Printf("%s skipped ibstat check (infiniband class not found or ibstat not found)\n", checkMark)
	}

	if len(o.LsmodPeermemErrors) > 0 {
		fmt.Printf("%s lsmod peermem check failed with %d error(s)\n", warningSign, len(o.LsmodPeermemErrors))
		for _, err := range o.LsmodPeermemErrors {
			fmt.Println(err)
		}
	} else {
		fmt.Printf("%s successfully checked lsmod peermem\n", checkMark)
	}

	if len(o.NVMLErrors) > 0 {
		fmt.Printf("%s nvml check failed with %d error(s)\n", warningSign, len(o.NVMLErrors))
		for _, err := range o.NVMLErrors {
			fmt.Println(err)
		}
	} else {
		fmt.Printf("%s successfully checked nvml\n", checkMark)
	}

	if o.NVML != nil {
		if len(o.NVML.DeviceInfos) > 0 {
			fmt.Printf("%s name: %s (NVML)\n", checkMark, o.NVML.DeviceInfos[0].Name)
		}

		for _, dev := range o.NVML.DeviceInfos {
			fmt.Printf("\n\n##################\nNVML %s\n\n", dev.UUID)

			if dev.GSPFirmwareMode.Enabled {
				fmt.Printf("%s NVML GSP firmware mode is enabled\n", checkMark)
			} else {
				fmt.Printf("%s NVML GSP firmware mode is disabled\n", warningSign)
			}

			// ref. https://docs.nvidia.com/deploy/driver-persistence/index.html
			if dev.PersistenceMode.Enabled {
				fmt.Printf("%s NVML persistence mode is enabled (nvidia-persistenced running %v)\n", checkMark, o.PersistencedRunning)
			} else {
				fmt.Printf("%s NVML persistence mode is disabled (nvidia-persistenced running %v)\n", warningSign, o.PersistencedRunning)
			}

			if dev.ClockEvents != nil {
				if dev.ClockEvents.HWSlowdown || dev.ClockEvents.HWSlowdownThermal || dev.ClockEvents.HWSlowdownPowerBrake {
					fmt.Printf("%s NVML found hw slowdown error(s)\n", warningSign)
					yb, err := dev.ClockEvents.YAML()
					if err != nil {
						log.Logger.Warnw("failed to marshal clock events", "error", err)
					} else {
						fmt.Printf("clock events:\n%s\n\n", string(yb))
					}
				} else {
					fmt.Printf("%s NVML found no hw slowdown error\n", checkMark)
				}
			}

			uncorrectedErrs := dev.ECCErrors.Volatile.FindUncorrectedErrs()
			if len(uncorrectedErrs) > 0 {
				fmt.Printf("%s NVML found %d ecc volatile uncorrected error(s)\n", warningSign, len(uncorrectedErrs))
				yb, err := dev.ECCErrors.YAML()
				if err != nil {
					log.Logger.Warnw("failed to marshal ecc errors", "error", err)
				} else {
					fmt.Printf("ecc errors:\n%s\n\n", string(yb))
				}
			} else {
				fmt.Printf("%s NVML found no ecc volatile uncorrected error\n", checkMark)
			}

			if len(dev.Processes.RunningProcesses) > 0 {
				fmt.Printf("%s NVML found %d running process\n", checkMark, len(dev.Processes.RunningProcesses))
				yb, err := dev.Processes.YAML()
				if err != nil {
					log.Logger.Warnw("failed to marshal processes", "error", err)
				} else {
					fmt.Printf("\n%s\n\n", string(yb))
				}
			} else {
				fmt.Printf("%s NVML found no running process\n", checkMark)
			}
		}
	}

	if debug {
		copied := *o
		if copied.Ibstat != nil {
			copied.Ibstat.Raw = ""
		}
		if copied.SMI != nil {
			copied.SMI.Summary = ""
			copied.SMI.Raw = ""
		}
		yb, err := copied.YAML()
		if err != nil {
			log.Logger.Warnw("failed to marshal output", "error", err)
		} else {
			fmt.Printf("\n\n##################\nfull nvidia query output\n\n")
			fmt.Println(string(yb))
		}
	}
}
