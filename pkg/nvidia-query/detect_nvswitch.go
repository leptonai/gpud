package query

import (
	"context"
	"fmt"
	"strings"

	"github.com/leptonai/gpud/pkg/file"
	"github.com/leptonai/gpud/pkg/log"
	"github.com/leptonai/gpud/pkg/process"
)

func CountSMINVSwitches(ctx context.Context) ([]string, error) {
	return countSMINVSwitches(ctx, "nvidia-smi nvlink --status")
}

// e.g.,
// "nvidia-smi nvlink --status"
func countSMINVSwitches(ctx context.Context, command string) ([]string, error) {
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
			// e.g.,
			// GPU 7: NVIDIA A100-SXM4-80GB (UUID: GPU-754035b4-4708-efcd-b261-623aea38bcad)
			if strings.Contains(line, "GPU ") && strings.Contains(line, "NVIDIA") && strings.Contains(line, "UUID") {
				lines = append(lines, line)
			}
		}),
		process.WithWaitForCmd(),
	); err != nil {
		return nil, fmt.Errorf("failed to read nvidia-smi nvlink output: %w\n\noutput:\n%s", err, strings.Join(lines, "\n"))
	}
	return lines, nil
}
