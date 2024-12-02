package process

import (
	"context"
	"strings"

	"github.com/leptonai/gpud/log"

	procs "github.com/shirou/gopsutil/v4/process"
)

// CountRunningPids returns the number of running pids.
func CountRunningPids() (uint64, error) {
	pids, err := procs.Pids()
	if err != nil {
		return 0, err
	}
	return uint64(len(pids)), nil
}

// CountProcessesByStatus counts all processes by its process status.
func CountProcessesByStatus(ctx context.Context) (map[string][]*procs.Process, error) {
	processes, err := procs.ProcessesWithContext(ctx)
	if err != nil {
		return nil, err
	}
	if len(processes) == 0 {
		return nil, nil
	}

	all := make(map[string][]*procs.Process)
	for _, p := range processes {
		if p == nil {
			continue
		}

		status, err := p.Status()
		if err != nil {
			ee := strings.ToLower(err.Error())

			// e.g., Not Found
			if strings.Contains(ee, "not found") {
				continue
			}

			// e.g., "open /proc/2342816/status: no such file or directory"
			if strings.Contains(ee, "no such file") {
				continue
			}

			log.Logger.Warnw("failed to get status", "error", err)
			continue
		}
		if len(status) < 1 {
			log.Logger.Warnw("no status found", "pid", p.Pid)
			continue
		}
		s := status[0]

		prev, ok := all[s]
		if !ok {
			all[s] = []*procs.Process{p}
		} else {
			all[s] = append(prev, p)
		}
	}

	return all, nil
}
