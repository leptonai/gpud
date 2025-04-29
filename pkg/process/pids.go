package process

import (
	"context"
	"os/exec"
	"strings"

	"github.com/leptonai/gpud/pkg/log"

	procs "github.com/shirou/gopsutil/v4/process"
)

func CheckRunningByPid(ctx context.Context, processName string) bool {
	log.Logger.Debugw("checking if process is running", "processName", processName)
	err := exec.CommandContext(ctx, "pidof", processName).Run()
	if err != nil {
		log.Logger.Debugw("failed to check -- assuming process is not running", "error", err)
	}
	return err == nil
}

// CountRunningPids returns the number of running pids.
func CountRunningPids() (uint64, error) {
	return countRunningPidsImpl(procs.Pids)
}

// countRunningPidsImpl is the implementation of CountRunningPids that takes a function
// to get the PIDs, making it easier to test.
func countRunningPidsImpl(getPids func() ([]int32, error)) (uint64, error) {
	pids, err := getPids()
	if err != nil {
		return 0, err
	}
	return uint64(len(pids)), nil
}

// ProcessStatus represents the read-only status of a process.
// Derived from "github.com/shirou/gopsutil/v4/process.Process" struct.
// ref. https://pkg.go.dev/github.com/shirou/gopsutil/v4@v4.25.3/process#Process
type ProcessStatus interface {
	Name() (string, error)
	Status() ([]string, error)
}

// CountProcessesByStatus counts all processes by its process status.
func CountProcessesByStatus(ctx context.Context) (map[string][]ProcessStatus, error) {
	return countProcessesByStatus(ctx, func(ctx context.Context) ([]ProcessStatus, error) {
		procs, err := procs.ProcessesWithContext(ctx)
		if err != nil {
			return nil, err
		}
		ps := make([]ProcessStatus, len(procs))
		for i, p := range procs {
			ps[i] = p
		}
		return ps, nil
	})
}

// countProcessesByStatus counts all processes by its process status.
func countProcessesByStatus(ctx context.Context, listProcessFunc func(ctx context.Context) ([]ProcessStatus, error)) (map[string][]ProcessStatus, error) {
	processes, err := listProcessFunc(ctx)
	if err != nil {
		return nil, err
	}
	if len(processes) == 0 {
		return nil, nil
	}

	all := make(map[string][]ProcessStatus)
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
			name, _ := p.Name()
			log.Logger.Warnw("no status found", "name", name)
			continue
		}
		s := status[0]

		prev, ok := all[s]
		if !ok {
			all[s] = []ProcessStatus{p}
		} else {
			all[s] = append(prev, p)
		}
	}

	return all, nil
}
