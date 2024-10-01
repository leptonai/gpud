//go:build darwin
// +build darwin

package fd

func checkFDLimitSupported() bool {
	return false
}

// No easy way to get the system-wide file descriptor limit on darwin.
func getLimit() (uint64, error) {
	return 0, nil
}

func getUsage() (uint64, error) {
	return 0, nil
}
