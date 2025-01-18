//go:build windows
// +build windows

package file

func CheckFDLimitSupported() bool {
	return false
}

func CheckFileHandlesSupported() bool {
	return false
}

func GetLimit() (uint64, error) {
	return 0, nil
}

func GetFileHandles() (uint64, uint64, error) {
	return 0, 0, nil
}

func GetUsage() (uint64, error) {
	return 0, nil
}

// Returns the number of allocated file handles for the current process.
// Not implemented for this architecture.
func GetCurrentProcessUsage() (uint64, error) {
	return 0, nil
}
