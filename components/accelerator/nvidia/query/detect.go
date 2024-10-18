package query

import (
	"bufio"
	"context"
	"fmt"
	"strings"

	"github.com/leptonai/gpud/log"
	"github.com/leptonai/gpud/pkg/file"
	"github.com/leptonai/gpud/pkg/process"

	"github.com/NVIDIA/go-nvlib/pkg/nvlib/device"
	nvinfo "github.com/NVIDIA/go-nvlib/pkg/nvlib/info"
	"github.com/NVIDIA/go-nvml/pkg/nvml"
)

// Returns true if the local machine has NVIDIA GPUs installed.
func GPUsInstalled(ctx context.Context) (bool, error) {
	smiInstalled := SMIExists()
	if !smiInstalled {
		return false, nil
	}

	// now that nvidia-smi installed,
	// check the NVIDIA GPU presence via PCI bus
	pciDevices, err := ListPCIs(ctx)
	if err != nil {
		return false, err
	}
	if len(pciDevices) == 0 {
		return false, nil
	}

	// now that we have the NVIDIA PCI devices,
	// call NVML C-based API for NVML API
	productName, err := LoadProductName(ctx)
	if err != nil {
		return false, err
	}

	log.Logger.Infow("detected nvidia gpu", "product", productName)
	return true, nil
}

// Loads the product name of the NVIDIA GPU.
func LoadProductName(ctx context.Context) (string, error) {
	nvmlLib := nvml.New()
	if ret := nvmlLib.Init(); ret != nvml.SUCCESS {
		return "", fmt.Errorf("failed to initialize NVML: %v", nvml.ErrorString(ret))
	}

	deviceLib := device.New(nvmlLib)
	infoLib := nvinfo.New(
		nvinfo.WithNvmlLib(nvmlLib),
		nvinfo.WithDeviceLib(deviceLib),
	)

	nvmlExists, nvmlExistsMsg := infoLib.HasNvml()
	if !nvmlExists {
		return "", fmt.Errorf("NVML not found: %s", nvmlExistsMsg)
	}

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
func ListPCIs(ctx context.Context) ([]string, error) {
	lspciPath, err := file.LocateExecutable("lspci")
	if err != nil {
		return nil, nil
	}
	if lspciPath == "" {
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

	lines := make([]string, 0)

	scanner := bufio.NewScanner(p.StdoutReader())
	for scanner.Scan() { // returns false at the end of the output
		line := scanner.Text()

		// e.g.,
		// 01:00.0 VGA compatible controller: NVIDIA Corporation Device 2684 (rev a1)
		// 01:00.1 Audio device: NVIDIA Corporation Device 22ba (rev a1)
		if strings.Contains(line, "NVIDIA") {
			lines = append(lines, line)
		}

		select {
		case err := <-p.Wait():
			if err != nil {
				return nil, err
			}
		default:
		}
	}
	if serr := scanner.Err(); serr != nil {
		// process already dead, thus ignore
		// e.g., "read |0: file already closed"
		if !strings.Contains(serr.Error(), "file already closed") {
			return nil, serr
		}
	}

	select {
	case err := <-p.Wait():
		if err != nil {
			return nil, err
		}
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	return lines, nil
}
