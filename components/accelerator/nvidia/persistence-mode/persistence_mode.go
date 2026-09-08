package persistencemode

import (
	"context"
	"encoding/csv"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/NVIDIA/go-nvml/pkg/nvml"

	"github.com/leptonai/gpud/pkg/nvidia/driverroot"
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
	// verifies the state through nvidia-smi's per-GPU daemon RPC query.
	Enabled bool `json:"enabled"`
	// Supported is true if the persistence mode is supported by the device.
	Supported bool `json:"supported"`
	// NVMLReportedEnabled preserves the raw nvmlDeviceGetPersistenceMode
	// result when nvidia-smi was needed to determine the effective state.
	NVMLReportedEnabled *bool `json:"nvml_reported_enabled,omitempty"`
	// EffectiveStateSource is "nvidia-smi" when Enabled was verified through
	// the persistence daemon RPC rather than taken directly from NVML.
	EffectiveStateSource string `json:"effective_state_source,omitempty"`
}

// GetPersistenceMode returns the persistence mode for a device.
func GetPersistenceMode(uuid string, dev device.Device) (PersistenceMode, error) {
	mode := PersistenceMode{
		UUID:      uuid,
		BusID:     dev.PCIBusID(),
		Supported: true,
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

// queryEffectivePersistenceModes asks nvidia-smi for the effective per-GPU
// state. NVIDIA documents that nvidia-smi uses nvidia-persistenced's RPC
// interface when the daemon is running, so this observes runtime per-device
// changes that cannot be inferred from the daemon process or its startup argv.
//
// The driver-root chroot path is required for GPU Operator installations:
// nvidia-smi and its matching libraries live inside that root. Bare-metal
// installations use nvidia-smi from PATH.
func queryEffectivePersistenceModes(ctx context.Context) (map[string]bool, error) {
	queryCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	args := []string{"--query-gpu=uuid,persistence_mode", "--format=csv,noheader,nounits"}
	var lastErr error
	for _, root := range driverroot.Existing() {
		executable := filepath.Join(root, "usr", "bin", "nvidia-smi")
		if st, err := os.Stat(executable); err != nil || !st.Mode().IsRegular() || st.Mode().Perm()&0o111 == 0 {
			continue
		}
		out, err := runPersistenceModeQuery(queryCtx, "chroot", root, "/usr/bin/nvidia-smi", args[0], args[1])
		if err != nil {
			lastErr = fmt.Errorf("failed to query persistence mode with %s: %w", executable, err)
			continue
		}
		return parsePersistenceModeCSV(out)
	}

	executable, err := exec.LookPath("nvidia-smi")
	if err == nil {
		out, runErr := runPersistenceModeQuery(queryCtx, executable, args...)
		if runErr == nil {
			return parsePersistenceModeCSV(out)
		}
		lastErr = fmt.Errorf("failed to query persistence mode with %s: %w", executable, runErr)
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, fmt.Errorf("nvidia-smi not found in PATH or NVIDIA driver roots")
}

func runPersistenceModeQuery(ctx context.Context, executable string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, executable, args...).Output()
}

func parsePersistenceModeCSV(data []byte) (map[string]bool, error) {
	records, err := csv.NewReader(strings.NewReader(string(data))).ReadAll()
	if err != nil {
		return nil, fmt.Errorf("failed to parse nvidia-smi persistence output: %w", err)
	}
	if len(records) == 0 {
		return nil, fmt.Errorf("nvidia-smi returned no persistence mode rows")
	}
	states := make(map[string]bool, len(records))
	for _, record := range records {
		if len(record) != 2 {
			return nil, fmt.Errorf("unexpected nvidia-smi persistence row with %d fields", len(record))
		}
		uuid := strings.TrimSpace(record[0])
		state := strings.ToLower(strings.TrimSpace(record[1]))
		if uuid == "" || (state != "enabled" && state != "disabled") {
			return nil, fmt.Errorf("unexpected nvidia-smi persistence row %q", record)
		}
		if _, exists := states[uuid]; exists {
			return nil, fmt.Errorf("duplicate nvidia-smi persistence row for GPU %q", uuid)
		}
		states[uuid] = state == "enabled"
	}
	return states, nil
}
