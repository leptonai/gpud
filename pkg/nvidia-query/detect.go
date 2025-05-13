package query

import (
	"context"
	"fmt"
	"strings"

	"github.com/leptonai/gpud/pkg/file"
	"github.com/leptonai/gpud/pkg/log"
	"github.com/leptonai/gpud/pkg/process"
)

// Lists all PCI devices that are compatible with NVIDIA.
func ListNVIDIAPCIs(ctx context.Context) ([]string, error) {
	return listNVIDIAPCIs(ctx, "lspci")
}

func listNVIDIAPCIs(ctx context.Context, command string) ([]string, error) {
	lspciPath, err := file.LocateExecutable(strings.Split(command, " ")[0])
	if lspciPath == "" || err != nil {
		return nil, fmt.Errorf("failed to locate lspci: %w", err)
	}

	p, err := process.New(
		process.WithCommand(command),
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
			// 3D controller represents the GPU device itself
			// whereas PCI Bridge refers to the PCIe switch/bridge component
			// that connects the GPU to the system's PCIe infrastructure
			//
			// e.g.,
			// 000a:00:00.0 Bridge: NVIDIA Corporation Device 1af1 (rev a1)
			// 000b:00:00.0 3D controller: NVIDIA Corporation GA100 [A100 SXM4 80GB] (rev a1)
			if strings.Contains(line, "3D controller") && strings.Contains(line, "NVIDIA") {
				lines = append(lines, line)
			}
		}),
		process.WithWaitForCmd(),
	); err != nil {
		return nil, fmt.Errorf("failed to read lspci output: %w\n\noutput:\n%s", err, strings.Join(lines, "\n"))
	}
	return lines, nil
}
