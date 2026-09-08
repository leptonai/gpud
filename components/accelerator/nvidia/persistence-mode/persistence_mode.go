package persistencemode

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/NVIDIA/go-nvml/pkg/nvml"

	nvmlerrors "github.com/leptonai/gpud/pkg/nvidia/errors"
	"github.com/leptonai/gpud/pkg/nvidia/nvml/device"
)

// PersistenceMode is the persistence mode of the device.
// Implements "DCGM_FR_PERSISTENCE_MODE" in DCGM.
// ref. https://github.com/NVIDIA/DCGM/blob/903d745504f50153be8293f8566346f9de3b3c93/nvvs/plugin_src/software/Software.cpp#L526-L553
//
// Persistence mode controls whether the NVIDIA driver stays loaded when no active clients are connected to the GPU.
// ref. https://developer.nvidia.com/management-library-nvml
//
// Once all clients have closed the device file, the GPU state will be unloaded unless persistence mode is enabled.
// ref. https://docs.nvidia.com/deploy/driver-persistence/index.html
//
// NVIDIA Persistence Daemon provides a more robust implementation of persistence mode on Linux.
// ref. https://docs.nvidia.com/deploy/driver-persistence/index.html#usage
//
// nvidia-smi transparently uses nvidia-persistenced's per-GPU RPC interface
// when the daemon is running, and otherwise falls back to legacy persistence.
type PersistenceMode struct {
	UUID  string `json:"uuid"`
	BusID string `json:"bus_id"`
	// Enabled is the effective persistence state. It normally comes directly
	// from NVML; on daemon-managed systems where NVML reports disabled, gpud
	// verifies that nvidia-persistenced holds the GPU device open.
	Enabled bool `json:"enabled"`
	// Supported is true if the persistence mode is supported by the device.
	Supported bool `json:"supported"`
	// NVMLReportedEnabled preserves the raw nvmlDeviceGetPersistenceMode
	// result when daemon device handles determined the effective state.
	NVMLReportedEnabled *bool `json:"nvml_reported_enabled,omitempty"`
	// EffectiveStateSource is "nvidia-persistenced" when Enabled was verified
	// from the daemon's open GPU device handles rather than taken from NVML.
	EffectiveStateSource string `json:"effective_state_source,omitempty"`
	minorNumber          int
}

// GetPersistenceMode returns the persistence mode for a device.
func GetPersistenceMode(uuid string, dev device.Device) (PersistenceMode, error) {
	mode := PersistenceMode{
		UUID:      uuid,
		BusID:     dev.PCIBusID(),
		Supported: true,
	}
	minorNumber, ret := dev.GetMinorNumber()
	if ret == nvml.SUCCESS {
		mode.minorNumber = minorNumber
	} else {
		mode.minorNumber = -1
	}

	// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlDeviceQueries.html#group__nvmlDeviceQueries_1g1224ad7b15d7407bebfff034ec094c6b
	pm, ret := dev.GetPersistenceMode()
	if nvmlerrors.IsNotSupportError(ret) {
		mode.Supported = false
		return mode, nil
	}
	if nvmlerrors.IsGPULostError(ret) {
		return mode, nvmlerrors.ErrGPULost
	}
	if nvmlerrors.IsGPURequiresReset(ret) {
		return mode, nvmlerrors.ErrGPURequiresReset
	}
	// not a "not supported" error, not a success return, thus return an error here
	if ret != nvml.SUCCESS {
		return mode, fmt.Errorf("failed to get device persistence mode: %v", nvml.ErrorString(ret))
	}
	mode.Enabled = pm == nvml.FEATURE_ENABLED

	return mode, nil
}

// daemonPersistenceModes returns the GPU minor numbers that a live
// nvidia-persistenced process currently holds open. NVIDIA's daemon
// implementation opens a GPU device when persistence is enabled and closes
// it when disabled; those handles are the mechanism that keeps driver state
// loaded. Reading them from proc therefore observes current per-GPU state
// without executing nvidia-smi or inferring state from startup arguments.
func daemonPersistenceModes(procRoot string) (map[int]bool, error) {
	entries, err := os.ReadDir(procRoot)
	if err != nil {
		return nil, err
	}
	modes := make(map[int]bool)
	foundDaemon := false
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if _, err := strconv.Atoi(entry.Name()); err != nil {
			continue
		}
		pidRoot := filepath.Join(procRoot, entry.Name())
		cmdline, err := os.ReadFile(filepath.Join(pidRoot, "cmdline"))
		if err != nil || filepath.Base(strings.SplitN(string(cmdline), "\x00", 2)[0]) != "nvidia-persistenced" {
			continue
		}
		foundDaemon = true
		fds, err := os.ReadDir(filepath.Join(pidRoot, "fd"))
		if err != nil {
			return nil, fmt.Errorf("failed to read nvidia-persistenced file descriptors for pid %s: %w", entry.Name(), err)
		}
		for _, fd := range fds {
			target, err := os.Readlink(filepath.Join(pidRoot, "fd", fd.Name()))
			if err != nil {
				return nil, fmt.Errorf("failed to read nvidia-persistenced file descriptor %s for pid %s: %w", fd.Name(), entry.Name(), err)
			}
			target = strings.TrimSuffix(target, " (deleted)")
			if !strings.HasPrefix(target, "/dev/nvidia") {
				continue
			}
			name := filepath.Base(target)
			minor, err := strconv.Atoi(strings.TrimPrefix(name, "nvidia"))
			if err == nil && name == "nvidia"+strconv.Itoa(minor) {
				modes[minor] = true
			}
		}
	}
	if foundDaemon {
		return modes, nil
	}
	return nil, fmt.Errorf("nvidia-persistenced process not found")
}
