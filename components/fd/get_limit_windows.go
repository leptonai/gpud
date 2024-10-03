//go:build windows
// +build windows

package fd

func checkFDLimitSupported() bool {
	return false
}

func getLimit() (uint64, error) {
	return 0, nil
}

func getUsage() (uint64, error) {
	return 0, nil
}
