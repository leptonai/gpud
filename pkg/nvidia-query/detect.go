package query

import (
	"context"
	"fmt"
	"strings"

	"github.com/NVIDIA/go-nvlib/pkg/nvlib/device"
	"github.com/NVIDIA/go-nvml/pkg/nvml"

	"github.com/leptonai/gpud/pkg/file"
	"github.com/leptonai/gpud/pkg/log"
	nvmlquery "github.com/leptonai/gpud/pkg/nvidia-query/nvml"
	"github.com/leptonai/gpud/pkg/process"
)

// Returns true if the local machine has NVIDIA GPUs installed.
func GPUsInstalled(ctx context.Context) (bool, error) {
	smiInstalled := SMIExists()
	if !smiInstalled {
		return false, nil
	}
	log.Logger.Debugw("nvidia-smi installed")

	// now that nvidia-smi installed,
	// check the NVIDIA GPU presence via PCI bus
	pciDevices, err := ListNVIDIAPCIs(ctx)
	if err != nil {
		return false, err
	}
	if len(pciDevices) == 0 {
		return false, nil
	}
	log.Logger.Debugw("nvidia PCI devices found", "devices", len(pciDevices))

	// now that we have the NVIDIA PCI devices,
	// call NVML C-based API for NVML API
	gpuDeviceName, err := LoadGPUDeviceName(ctx)
	if err != nil {
		if IsErrDeviceHandleUnknownError(err) {
			log.Logger.Warnw("nvidia device handler failed for unknown error -- likely GPU has fallen off the bus or other Xid error", "error", err)
			return true, nil
		}
		return false, err
	}
	log.Logger.Debugw("detected nvidia gpu", "gpuDeviceName", gpuDeviceName)

	return true, nil
}

// Loads the product name of the NVIDIA GPU device.
func LoadGPUDeviceName(ctx context.Context) (string, error) {
	nvmlLib := nvmlquery.NewNVML()
	if ret := nvmlLib.Init(); ret != nvml.SUCCESS {
		return "", fmt.Errorf("failed to initialize NVML: %v", nvml.ErrorString(ret))
	}

	deviceLib := device.New(nvmlLib)

	// do not check nvml lib if it is mocked
	infoLib := nvmlquery.NewNVInfo(nvmlLib, deviceLib)
	nvmlExists, nvmlExistsMsg := infoLib.HasNvml()
	if !nvmlExists {
		return "", fmt.Errorf("NVML not found: %s", nvmlExistsMsg)
	}

	// "NVIDIA Xid 79: GPU has fallen off the bus" may fail this syscall with:
	// "error getting device handle for index '6': Unknown Error"
	devices, err := deviceLib.GetDevices()
	if err != nil {
		return "", err
	}

	for _, d := range devices {
		name, ret := d.GetName()
		if ret != nvml.SUCCESS {
			return "", fmt.Errorf("failed to get device name: %v", nvml.ErrorString(ret))
		}
		if name != "" {
			return name, nil
		}
	}

	return "", nil
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
