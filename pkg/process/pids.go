package process

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
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
	PID() int32
	Status() ([]string, error)
}

func getProcessStatus(p *procs.Process) ProcessStatus {
	return &processStatus{Process: p}
}

type processStatus struct {
	*procs.Process
}

func (p *processStatus) PID() int32 {
	return p.Pid
}

// Name returns the process comm. On systems with procfs (Linux, or when
// HOST_PROC points at an alternate procfs root, e.g., /host/proc in debug
// pods) it reads /proc/<pid>/comm only; elsewhere it falls back to the
// platform-native gopsutil implementation.
//
// We deliberately avoid gopsutil's Process.Name() on Linux: for names
// truncated to 15 chars (TASK_COMM_LEN-1) it falls back to reading
// /proc/<pid>/cmdline, which requires mm access (access_remote_vm).
//
// The hazard has a precise precondition: the cmdline read only blocks when
// the target task is stuck in D-state WHILE holding its mmap_lock — i.e., a
// wedged teardown (e.g., the NVIDIA driver's uvm_va_space_destroy path during
// process exit). A plain I/O wait in D-state (e.g., folio_wait_bit_common on
// a suspended device) does not hold that lock, so the cmdline read completes;
// we observed exactly that during the LEP-6029 canary validation
// (2026-08-13), where the fallback returned the full 21-char name
// "nvidia-dstate-blocker" for a D-state process while ps showed the 15-char
// comm "nvidia-dstate-b". We still avoid the path entirely: the organic
// incident shape is precisely the mm-holding teardown, and comm alone
// suffices for detection — the name is kernel-truncated to 15 chars,
// consistent with ps(1).
//
// The procfs-availability check runs per call (one stat per blocked process
// per minute — negligible) so tests/dev hosts can toggle HOST_PROC between
// cases. When procfs exists but the per-pid comm file does not, the process
// exited between enumeration and read; the error is returned to the caller,
// which already tolerates that PID churn (see countProcessesByStatus).
func (p *processStatus) Name() (string, error) {
	if _, err := os.Stat(procFSRoot()); err == nil {
		return ReadComm(p.Pid)
	}
	return p.Process.Name()
}

// procFSRoot returns the procfs root: HOST_PROC if set (as gopsutil honors),
// else /proc.
func procFSRoot() string {
	if v := os.Getenv("HOST_PROC"); v != "" {
		return v
	}
	return "/proc"
}

// ReadComm reads the process name (comm) from <procfs>/<pid>/comm and returns
// it trimmed. It never reads /proc/<pid>/cmdline (which can block on D-state
// tasks holding their mmap_lock). A missing file (PID exited between
// enumeration and read) returns an error; callers must tolerate that churn
// rather than assume the process still exists.
func ReadComm(pid int32) (string, error) {
	b, err := os.ReadFile(filepath.Join(procFSRoot(), strconv.Itoa(int(pid)), "comm"))
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}

// FindProcessByName finds a process by its name.
func FindProcessByName(ctx context.Context, processName string) (ProcessStatus, error) {
	return findProcessByName(ctx, processName, procs.ProcessesWithContext)
}

// findProcessByName finds a process by its name.
func findProcessByName(ctx context.Context, processName string, listProcessFunc func(ctx context.Context) ([]*procs.Process, error)) (ProcessStatus, error) {
	procs, err := listProcessFunc(ctx)
	if err != nil {
		return nil, err
	}
	for _, p := range procs {
		name, err := p.Name()
		if err != nil {
			continue
		}
		if strings.Contains(name, processName) {
			return getProcessStatus(p), nil
		}
	}
	return nil, nil
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
			ps[i] = getProcessStatus(p)
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
