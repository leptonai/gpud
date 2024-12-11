package systemd

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"

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

	lines := make([]string, 0, 10)
	if err := process.ReadAllStdout(
		ctx,
		proc,
		process.WithProcessLine(func(line string) {
			s := strings.TrimSpace(line)
			if s == "" {
				return
			}
			lines = append(lines, s)
		}),
		process.WithWaitForCmd(),
	); err != nil {
		return "", err
	}
	if perr := proc.Abort(ctx); perr != nil {
		return "", err
	}

	return strings.Join(lines, "\n"), nil
}
