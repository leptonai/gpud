package dmesg

import (
	"regexp"
)

const (
	// e.g.,
	// [...] VFS: file-max limit 1000000 reached
	// [...] VFS: file-max limit <number> reached
	//
	// ref.
	// https://docs.kernel.org/admin-guide/sysctl/fs.html#file-max-file-nr
	RegexVFSFileMaxLimitReached = `VFS: file-max limit \d+ reached`
)

var compiledVFSFileMaxLimitReached = regexp.MustCompile(RegexVFSFileMaxLimitReached)

// Returns true if the line indicates that the file-max limit has been reached.
// ref. https://docs.kernel.org/admin-guide/sysctl/fs.html#file-max-file-nr
func HasVFSFileMaxLimitReached(line string) bool {
	if match := compiledVFSFileMaxLimitReached.FindStringSubmatch(line); match != nil {
		return true
	}
	return false
}
