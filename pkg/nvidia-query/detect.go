package query

import (
	"context"
	"fmt"
	"strings"

	"github.com/leptonai/gpud/pkg/file"
	"github.com/leptonai/gpud/pkg/log"
	"github.com/leptonai/gpud/pkg/nvidia-query/nvml"
	"github.com/leptonai/gpud/pkg/process"
)

// Returns true if the local machine has NVIDIA GPUs installed.
func GPUsInstalled(ctx context.Context) (bool, error) {
	// now that nvidia-smi installed,
	// check the NVIDIA GPU presence via PCI bus
	pciDevices, err := ListNVIDIAPCIs(ctx)
	if err != nil {
		return false, err
	}
	if len(pciDevices) == 0 {
		return false, nil
	}
	log.Logger.Infow("nvidia PCI devices found", "devices", len(pciDevices))

	// now that we have the NVIDIA PCI devices,
	// call NVML C-based API for NVML API
	gpuDeviceName, err := nvml.LoadGPUDeviceName()
	if err != nil {
		if IsErrDeviceHandleUnknownError(err) {
			log.Logger.Warnw("nvidia device handler failed for unknown error -- likely GPU has fallen off the bus or other Xid error", "error", err)
			return true, nil
		}
		return false, err
	}
	log.Logger.Infow("detected nvidia gpu", "gpuDeviceName", gpuDeviceName)

	return gpuDeviceName != "", nil
}

// Lists all PCI devices that are compatible with NVIDIA.
func ListNVIDIAPCIs(ctx context.Context) ([]string, error) {
	lspciPath, err := file.LocateExecutable("lspci")
	if err != nil {
		return nil, nil
	}

	p, err := process.New(
		process.WithCommand(lspciPath),
		process.WithRunAsBashScript(),
	)
	if err != nil {
		return nil, err
	}

	if err := p.Start(ctx); err != nil {
		return nil, err
	}
	defer func() {
		if err := p.Close(ctx); err != nil {
			log.Logger.Warnw("failed to abort command", "err", err)
		}
	}()

	lines := make([]string, 0)
	if err := process.Read(
		ctx,
		p,
		process.WithReadStdout(),
		process.WithReadStderr(),
		process.WithProcessLine(func(line string) {
			// e.g.,
			// 01:00.0 VGA compatible controller: NVIDIA Corporation Device 2684 (rev a1)
			// 01:00.1 Audio device: NVIDIA Corporation Device 22ba (rev a1)
			if strings.Contains(line, "NVIDIA") {
				lines = append(lines, line)
			}
		}),
		process.WithWaitForCmd(),
	); err != nil {
		return nil, fmt.Errorf("failed to read lspci output: %w\n\noutput:\n%s", err, strings.Join(lines, "\n"))
	}
	return lines, nil
}
