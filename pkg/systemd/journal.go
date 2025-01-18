package systemd

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/leptonai/gpud/log"
	"github.com/leptonai/gpud/pkg/process"
)

func JournalctlExists() bool {
	p, err := exec.LookPath("journalctl")
	if err != nil {
		return false
	}
	return p != ""
}

// Fetches the latest/recent journal outputs using "journalctl".
// Equivalent to "journalctl -xeu [service name].service --no-pager".
func GetLatestJournalctlOutput(ctx context.Context, svcName string) (string, error) {
	if !JournalctlExists() {
		return "", errors.New("requires journalctl")
	}

	if !strings.HasSuffix(svcName, ".service") {
		svcName = svcName + ".service"
	}
	cmd := fmt.Sprintf("journalctl -xeu %s --no-pager", svcName)

	proc, err := process.New(process.WithCommand(cmd), process.WithRunAsBashScript())
	if err != nil {
		return "", err
	}
	if err := proc.Start(ctx); err != nil {
		return "", err
	}
	defer func() {
		if err := proc.Close(ctx); err != nil {
			log.Logger.Warnw("failed to abort command", "err", err)
		}
	}()

	lines := make([]string, 0, 10)
	if err := process.Read(
		ctx,
		proc,
		process.WithReadStdout(),
		process.WithReadStderr(),
		process.WithProcessLine(func(line string) {
			s := strings.TrimSpace(line)
			if s == "" {
				return
			}
			lines = append(lines, s)
		}),
		process.WithWaitForCmd(),
	); err != nil {
		return "", fmt.Errorf("failed to read journalctl output: %w\n\noutput:\n%s", err, strings.Join(lines, "\n"))
	}

	return strings.Join(lines, "\n"), nil
}
