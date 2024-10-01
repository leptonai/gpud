//go:build linux
// +build linux

package fd

import (
	"os"
	"strconv"
	"strings"

	"github.com/prometheus/procfs"
)

const fileMaxLinux = "/proc/sys/fs/file-max"

func checkFDLimitSupported() bool {
	_, err := os.Stat(fileMaxLinux)
	return err == nil
}

// returns the current file descriptor limit for the host, not for the current process.
// for the current process, use syscall.Getrlimit.
func getLimit() (uint64, error) {
	data, err := os.ReadFile(fileMaxLinux)
	if err != nil {
		return 0, err
	}
	return strconv.ParseUint(strings.TrimSpace(string(data)), 10, 64)
}

// "process_open_fds" in prometheus collector
// ref. https://github.com/prometheus/client_golang/blob/main/prometheus/process_collector_other.go
// ref. https://pkg.go.dev/github.com/prometheus/procfs
func getUsage() (uint64, error) {
	procs, err := procfs.AllProcs()
	if err != nil {
		return 0, err
	}
	total := uint64(0)
	for _, proc := range procs {
		l, err := proc.FileDescriptorsLen()
		if err != nil {
			// If the error is due to the file descriptor being cleaned up and not used anymore,
			// skip to the next process ID.
			if os.IsNotExist(err) ||
				strings.Contains(err.Error(), "no such file or directory") ||

				// e.g., stat /proc/1321147/fd: no such process
				strings.Contains(err.Error(), "no such process") {
				continue
			}

			return 0, err
		}
		total += uint64(l)
	}
	return total, nil
}
