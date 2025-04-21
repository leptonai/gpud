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
