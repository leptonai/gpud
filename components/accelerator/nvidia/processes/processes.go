package processes

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/dustin/go-humanize"
	"github.com/leptonai/gpud/pkg/nvidia-query/nvml/device"
	"github.com/shirou/gopsutil/v4/process"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/leptonai/gpud/pkg/log"
	nvmlerrors "github.com/leptonai/gpud/pkg/nvidia-query/nvml/errors"
)

// Processes represents the current clock events from the nvmlDeviceGetCurrentClocksEventReasons API.
// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlDeviceQueries.html#group__nvmlDeviceQueries_1g7e505374454a0d4fc7339b6c885656d6
// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlDeviceQueries.html#group__nvmlDeviceQueries_1ga115e41a14b747cb334a0e7b49ae1941
// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlClocksEventReasons.html#group__nvmlClocksEventReasons
type Processes struct {
	// Represents the GPU UUID.
	UUID string `json:"uuid"`

	// BusID is the GPU bus ID from the nvml API.
	//  e.g., "0000:0f:00.0"
	BusID string `json:"bus_id"`

	// A list of running processes.
	RunningProcesses []Process `json:"running_processes"`

	// GetComputeRunningProcessesSupported is true if the device supports the getComputeRunningProcesses API.
	GetComputeRunningProcessesSupported bool `json:"get_compute_running_processes_supported"`

	// GetProcessUtilizationSupported is true if the device supports the getProcessUtilization API.
	GetProcessUtilizationSupported bool `json:"get_process_utilization_supported"`
}

type Process struct {
	PID    uint32   `json:"pid"`
	Status []string `json:"status,omitempty"`

	// ZombieStatus is set to true if the process is defunct
	// (terminated but not reaped by its parent).
	ZombieStatus bool `json:"zombie_status,omitempty"`

	// BadEnvVarsForCUDA is a map of environment variables that are known to hurt CUDA
	// that is set for this specific process.
	// Empty if there is no bad environment variable found for this process.
	// This implements "DCGM_FR_BAD_CUDA_ENV" logic in DCGM.
	BadEnvVarsForCUDA map[string]string `json:"bad_env_vars_for_cuda,omitempty"`

	CmdArgs                     []string    `json:"cmd_args,omitempty"`
	CreateTime                  metav1.Time `json:"create_time,omitempty"`
	GPUUsedPercent              uint32      `json:"gpu_used_percent,omitempty"`
	GPUUsedMemoryBytes          uint64      `json:"gpu_used_memory_bytes,omitempty"`
	GPUUsedMemoryBytesHumanized string      `json:"gpu_used_memory_bytes_humanized,omitempty"`
}

func GetProcesses(uuid string, dev device.Device) (Processes, error) {
	return getProcesses(uuid, dev, process.NewProcess)
}

func getProcesses(uuid string, dev device.Device, newProcessFunc func(pid int32) (*process.Process, error)) (Processes, error) {
	procs := Processes{
		UUID:  uuid,
		BusID: dev.PCIBusID(),

		GetComputeRunningProcessesSupported: true,
		GetProcessUtilizationSupported:      true,
	}

	// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlDeviceQueries.html#group__nvmlDeviceQueries_1g34afcba3d32066db223265aa022a6b80
	computeProcs, ret := dev.GetComputeRunningProcesses()
	if nvmlerrors.IsNotSupportError(ret) {
		procs.GetComputeRunningProcessesSupported = false
		return procs, nil
	}
	if nvmlerrors.IsGPULostError(ret) {
		return procs, nvmlerrors.ErrGPULost
	}
	if nvmlerrors.IsGPURequiresReset(ret) {
		return procs, nvmlerrors.ErrGPURequiresReset
	}
	if ret != nvml.SUCCESS { // not a "not supported" error, not a success return, thus return an error here
		return procs, fmt.Errorf("failed to get device compute processes: %v", nvml.ErrorString(ret))
	}

	for _, proc := range computeProcs {
		procObject, err := newProcessFunc(int32(proc.Pid))
		if err != nil {
			// ref. process does not exist
			if errors.Is(err, process.ErrorProcessNotRunning) {
				log.Logger.Debugw("process not running -- skipping", "pid", proc.Pid, "error", err)
				continue
			}
			if nvmlerrors.IsNoSuchFileOrDirectoryError(err) {
				log.Logger.Debugw("process not running -- skipping", "pid", proc.Pid, "error", err)
				continue
			}
			return Processes{}, fmt.Errorf("failed to get process %d: %v", proc.Pid, err)
		}

		args, err := procObject.CmdlineSlice()
		if err != nil {
			if nvmlerrors.IsNoSuchFileOrDirectoryError(err) {
				log.Logger.Debugw("process not running -- skipping", "pid", proc.Pid, "error", err)
				continue
			}
			return Processes{}, fmt.Errorf("failed to get process %d args: %v", proc.Pid, err)
		}
		createTimeUnixMS, err := procObject.CreateTime()
		if err != nil {
			if nvmlerrors.IsNoSuchFileOrDirectoryError(err) {
				log.Logger.Debugw("process not running -- skipping", "pid", proc.Pid, "error", err)
				continue
			}
			return Processes{}, fmt.Errorf("failed to get process %d create time: %v", proc.Pid, err)
		}
		createTime := metav1.Unix(createTimeUnixMS/1000, 0)

		// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlDeviceQueries.html#group__nvmlDeviceQueries_1gb0ea5236f5e69e63bf53684a11c233bd
		memUtil := uint32(0)
		utils, ret := dev.GetProcessUtilization(uint64(proc.Pid))
		if nvmlerrors.IsNotSupportError(ret) {
			procs.GetProcessUtilizationSupported = false
			return procs, nil
		}
		if nvmlerrors.IsNotFoundError(ret) {
			continue
		}
		if nvmlerrors.IsGPULostError(ret) {
			return procs, nvmlerrors.ErrGPULost
		}
		if nvmlerrors.IsGPURequiresReset(ret) {
			return procs, nvmlerrors.ErrGPURequiresReset
		}
		if ret != nvml.SUCCESS { // not a "not supported" error, not a success return, thus return an error here
			return procs, fmt.Errorf("failed to get process %d utilization (%v)", proc.Pid, nvml.ErrorString(ret))
		}

		if len(utils) > 0 {
			// sort by last seen timestamp, so that first is the latest
			sort.Slice(utils, func(i, j int) bool {
				return utils[i].TimeStamp > utils[j].TimeStamp
			})

			// ref. https://docs.nvidia.com/deploy/nvml-api/structnvmlProcessUtilizationSample__t.html#structnvmlProcessUtilizationSample__t
			memUtil = utils[0].MemUtil
		}

		status, err := procObject.Status()
		if err != nil {
			if nvmlerrors.IsNoSuchFileOrDirectoryError(err) {
				continue
			}
			return procs, fmt.Errorf("failed to get process %d status: %v", proc.Pid, err)
		}
		isZombie := false
		for _, s := range status {
			if s == process.Zombie {
				isZombie = true
				break
			}
		}

		envs, err := procObject.Environ()
		if err != nil {
			if nvmlerrors.IsNoSuchFileOrDirectoryError(err) {
				continue
			}
			return procs, fmt.Errorf("failed to get process %d environ: %v", proc.Pid, err)
		}

		badEnvVars := make(map[string]string)
		for _, env := range envs {
			parts := strings.SplitN(env, "=", 2)
			if len(parts) == 2 {
				key, value := parts[0], parts[1]

				// implementing "DCGM_FR_BAD_CUDA_ENV"
				if _, ok := BAD_CUDA_ENV_KEYS[key]; ok {
					badEnvVars[key] = value
				}
			}
		}
		if len(badEnvVars) == 0 {
			badEnvVars = nil
		}

		procs.RunningProcesses = append(procs.RunningProcesses, Process{
			PID: proc.Pid,

			Status:       status,
			ZombieStatus: isZombie,

			BadEnvVarsForCUDA: badEnvVars,

			CmdArgs:    args,
			CreateTime: createTime,

			GPUUsedPercent: memUtil,

			// "Amount of used GPU memory in bytes."
			// ref. https://docs.nvidia.com/deploy/nvml-api/structnvmlProcessInfo__t.html#structnvmlProcessInfo__t
			GPUUsedMemoryBytes:          proc.UsedGpuMemory,
			GPUUsedMemoryBytesHumanized: humanize.IBytes(proc.UsedGpuMemory),
		})
	}

	return procs, nil
}
