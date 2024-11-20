//go:build darwin
// +build darwin

package file

func CheckFDLimitSupported() bool {
	return false
}

// No easy way to get the system-wide file descriptor limit on darwin.
func GetLimit() (uint64, error) {
	return 0, nil
}

// may fail for mac
// e.g.,
// stat /proc: no such file or directory
func GetUsage() (uint64, error) {
	return 0, nil
}
