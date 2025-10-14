package query

import (
	"context"
	"fmt"
	"strings"

	"github.com/leptonai/gpud/pkg/file"
	"github.com/leptonai/gpud/pkg/log"
	"github.com/leptonai/gpud/pkg/process"
)

// DeviceVendorID defines the vendor ID of NVIDIA devices.
// e.g.,
// lspci -nn | grep -i "10de.*"
// ref. https://devicehunt.com/view/type/pci/vendor/10DE
const DeviceVendorID = "10de"

// ListPCIGPUs returns all "lspci" lines that represents NVIDIA GPU devices.
func ListPCIGPUs(ctx context.Context) ([]string, error) {
	return listPCIs(ctx, "lspci -nn", isNVIDIAGPUPCI)
}

// 3D controller represents the GPU device itself
// whereas PCI Bridge refers to the PCIe switch/bridge component
// that connects the GPU to the system's PCIe infrastructure
//
// e.g.,
// 000a:00:00.0 Bridge: NVIDIA Corporation Device 1af1 (rev a1)
// 000b:00:00.0 3D controller: NVIDIA Corporation GA100 [A100 SXM4 80GB] (rev a1)
func isNVIDIAGPUPCI(line string) bool {
	return strings.Contains(line, "NVIDIA") && strings.Contains(line, "3D controller")
}

// ListPCINVSwitches returns all "lspci" lines that represents NVIDIA NVSwitch devices.
func ListPCINVSwitches(ctx context.Context) ([]string, error) {
	return listPCIs(ctx, "lspci -nn", isNVIDIANVSwitchPCI)
}

// e.g.,
// 0005:00:00.0 Bridge [0680]: NVIDIA Corporation Device [10de:1af1] (rev a1)
//
// Latest NVIDIA devices such as GB200 uses the following format:
// 0018:00:00.0 PCI bridge [0604]: NVIDIA Corporation Device [10de:22b1]
func isNVIDIANVSwitchPCI(line string) bool {
	line = strings.ToLower(line)
	return strings.Contains(line, "nvidia") && strings.Contains(line, "bridge")
}

func listPCIs(ctx context.Context, command string, matchFunc func(line string) bool) ([]string, error) {
	lspciPath, err := file.LocateExecutable(strings.Split(command, " ")[0])
	if lspciPath == "" || err != nil {
		return nil, fmt.Errorf("failed to locate lspci: %w", err)
	}

	p, err := process.New(
		process.WithCommand(command),
		process.WithRunAsBashScript(),
		process.WithRunBashInline(),
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
			if !strings.Contains(strings.ToLower(line), DeviceVendorID) {
				return
			}

			if matchFunc != nil && matchFunc(line) {
				lines = append(lines, line)
			}
		}),
		process.WithWaitForCmd(),
	); err != nil {
		return nil, fmt.Errorf("failed to read lspci output: %w\n\noutput:\n%s", err, strings.Join(lines, "\n"))
	}
	return lines, nil
}
