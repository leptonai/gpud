// Copyright 2019 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

//go:build linux
// +build linux

package file

import (
	"os"
	"strconv"
	"strings"

	"github.com/prometheus/procfs"
)

const fileMaxLinux = "/proc/sys/fs/file-max"

func CheckFDLimitSupported() bool {
	_, err := os.Stat(fileMaxLinux)
	return err == nil
}

// returns the current file descriptor limit for the host, not for the current process.
// for the current process, use syscall.Getrlimit.
func GetLimit() (uint64, error) {
	data, err := os.ReadFile(fileMaxLinux)
	if err != nil {
		return 0, err
	}
	return strconv.ParseUint(strings.TrimSpace(string(data)), 10, 64)
}

// "process_open_fds" in prometheus collector
// ref. https://github.com/prometheus/client_golang/blob/main/prometheus/process_collector_other.go
// ref. https://pkg.go.dev/github.com/prometheus/procfs
func GetUsage() (uint64, error) {
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
