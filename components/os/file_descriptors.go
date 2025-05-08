package os

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// FileDescriptors represents the file descriptors of the host.
type FileDescriptors struct {
	// The number of file descriptors currently allocated on the host (not per process).
	AllocatedFileHandles uint64 `json:"allocated_file_handles"`
	// The number of running PIDs returned by https://pkg.go.dev/github.com/shirou/gopsutil/v4/process#Pids.
	RunningPIDs uint64 `json:"running_pids"`
	Usage       uint64 `json:"usage"`
	Limit       uint64 `json:"limit"`

	// AllocatedFileHandlesPercent is the percentage of file descriptors that are currently allocated,
	// based on the current file descriptor limit and the current number of file descriptors allocated on the host (not per process).
	AllocatedFileHandlesPercent string `json:"allocated_file_handles_percent"`
	// UsedPercent is the percentage of file descriptors that are currently in use,
	// based on the current file descriptor limit on the host (not per process).
	UsedPercent string `json:"used_percent"`

	ThresholdAllocatedFileHandles        uint64 `json:"threshold_allocated_file_handles"`
	ThresholdAllocatedFileHandlesPercent string `json:"threshold_allocated_file_handles_percent"`

	ThresholdRunningPIDs        uint64 `json:"threshold_running_pids"`
	ThresholdRunningPIDsPercent string `json:"threshold_running_pids_percent"`

	// Set to true if the file handles are supported.
	FileHandlesSupported bool `json:"file_handles_supported"`
	// Set to true if the file descriptor limit is supported.
	FDLimitSupported bool `json:"fd_limit_supported"`
}

// Retrieves the file descriptor limit from the given path.
// For linux, it's "/proc/sys/fs/file-max".
// ref. https://docs.kernel.org/admin-guide/sysctl/fs.html#file-max-file-nr
func getLimit(path string) (uint64, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	return strconv.ParseUint(strings.TrimSpace(string(data)), 10, 64)
}

// Retrieves the number of allocated file handles.
// ref. https://docs.kernel.org/admin-guide/sysctl/fs.html#file-max-file-nr
func getFileHandles(path string) (uint64, uint64, error) {
	// e.g., "/proc/sys/fs/file-nr"
	// "three values in file-nr denote
	// the number of allocated file handles,
	// the number of allocated but unused file handles,
	// and the maximum number of file handles"
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, 0, err
	}
	fields := strings.Fields(strings.TrimSpace(string(data)))
	if len(fields) != 3 {
		return 0, 0, fmt.Errorf("unexpected number of fields in file-nr: %v", len(fields))
	}
	allocated, err := strconv.ParseUint(fields[0], 10, 64)
	if err != nil {
		return 0, 0, err
	}
	unused, err := strconv.ParseUint(fields[1], 10, 64)
	if err != nil {
		return 0, 0, err
	}
	return allocated, unused, nil
}
