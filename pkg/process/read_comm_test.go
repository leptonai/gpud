package process

// Validation of the comm-only name reads (LEP-6029) — WHY/HOW/WHAT:
//
// WHY a Linux container: these tests exercise /proc reads, which macOS (the
// dev host) lacks, and the pidof-based TestCheckRunningByPid_SelfProcess only
// passes where pidof exists. The D-state tracker's target environment is
// Linux with procfs, so the behavior must be validated there, not just on the
// dev host.
//
// HOW (not committed — no container machinery is added to the repo): the
// suites were run in a rootless podman golang:1.25 container with the repo
// and module cache mounted read-only:
//
//	podman run --rm --user 1000:1000 -v <repo>:/src:ro \
//	  -v <gomodcache>:/go/pkg/mod:ro -e GOCACHE=/tmp/gocache -w /src \
//	  golang:1.25 go test -race -gcflags="all=-N -l -d=checkptr=0" \
//	  ./components/os/ ./pkg/process/
//
// Non-root matters: as root, the os component's newComponent activates the
// kmsg syncer, which needs /dev/kmsg (absent in containers) and unrelated
// pstore tests fail on that environmental difference, not on product code.
//
// WHAT passed (2026-08-14, linux/arm64, go1.25.13, container): full
// components/os and pkg/process suites with -race, including
// TestCheckRunningByPid_SelfProcess and the comm-only Name() tests against
// the real procfs.

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	procs "github.com/shirou/gopsutil/v4/process"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeFakeProc creates a fake procfs entry HOST_PROC/<pid>/comm.
func writeFakeProc(t *testing.T, pid int32, comm string) string {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, "1234")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "comm"), []byte(comm+"\n"), 0o644))
	return root
}

func TestReadComm(t *testing.T) {
	root := writeFakeProc(t, 1234, "nvidia-smi")
	t.Setenv("HOST_PROC", root)

	name, err := ReadComm(1234)
	require.NoError(t, err)
	assert.Equal(t, "nvidia-smi", name)
}

func TestReadComm_MissingProcess(t *testing.T) {
	t.Setenv("HOST_PROC", t.TempDir())
	_, err := ReadComm(999999)
	require.Error(t, err, "missing proc entry must error (caller tolerates PID churn)")
}

func TestProcessStatusName_ReadsCommOnly(t *testing.T) {
	// a >15-char name would make gopsutil's Name() fall back to reading
	// /proc/<pid>/cmdline, which can block on D-state tasks holding their
	// mmap_lock; our wrapper must read comm only. The fake procfs has a comm
	// file and deliberately NO cmdline file: success proves cmdline is never
	// touched.
	root := writeFakeProc(t, 1234, "nvidia-persisten") // 16 chars, longer than comm's 15-char kernel cap
	t.Setenv("HOST_PROC", root)

	// bypass procs.NewProcess (it probes process existence); only Pid is needed
	ps := getProcessStatus(&procs.Process{Pid: 1234})

	name, err := ps.Name()
	require.NoError(t, err, "must not read /proc/<pid>/cmdline even for long names")
	assert.Equal(t, "nvidia-persisten", name)
}

func TestProcessStatusName_ProcfsRootSetButCommMissing(t *testing.T) {
	// when a procfs root exists (Linux prod, or HOST_PROC set), a missing
	// comm file means the process exited between enumeration and read: the
	// error must propagate (callers tolerate PID churn) and must NOT fall
	// back to the platform-native path (which would reintroduce the cmdline
	// read hazard on Linux).
	t.Setenv("HOST_PROC", t.TempDir()) // procfs root exists, but no <pid>/comm

	ps := getProcessStatus(&procs.Process{Pid: 999999})
	_, err := ps.Name()
	require.Error(t, err, "missing comm with a live procfs root must error, not fall back")
}

func TestProcessStatusName_NoProcfsFallsBackToNative(t *testing.T) {
	// the platform-native fallback exists only for hosts without procfs
	// (e.g., darwin dev machines running tests); on Linux this branch is
	// unreachable because /proc always exists.
	if runtime.GOOS == "linux" {
		t.Skip("cannot simulate a missing procfs on linux")
	}

	// self PID is guaranteed to exist on the host platform
	ps := getProcessStatus(&procs.Process{Pid: int32(os.Getpid())})
	name, err := ps.Name()
	require.NoError(t, err)
	assert.NotEmpty(t, name, "native fallback must return the self process name")
}
