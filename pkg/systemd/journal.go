package systemd

import (
	"bufio"
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

	proc, err := process.New([][]string{{cmd}}, process.WithRunAsBashScript())
	if err != nil {
		return "", err
	}
	if err := proc.Start(ctx); err != nil {
		return "", err
	}
	rd := proc.StdoutReader()

	scanner := bufio.NewScanner(rd)
	lines := make([]string, 0, 10)
	for scanner.Scan() { // returns false at the end of the output
		line := scanner.Text()
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		lines = append(lines, line)

		select {
		case err = <-proc.Wait():
			if err != nil {
				log.Logger.Warnw("lsmod return error", "error", err)
			}
		default:
		}
	}
	if serr := scanner.Err(); serr != nil {
		// process already dead, thus ignore
		// e.g., "read |0: file already closed"
		if !strings.Contains(serr.Error(), "file already closed") {
			return "", serr
		}
	}
	if err != nil {
		return "", err
	}
	if perr := proc.Abort(ctx); perr != nil {
		return "", err
	}

	return strings.Join(lines, "\n"), nil
}
