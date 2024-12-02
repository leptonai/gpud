package file

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

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
