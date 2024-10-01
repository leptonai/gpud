//go:build linux
// +build linux

package fd

import (
	"os"
	"strconv"
	"strings"
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
