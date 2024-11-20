//go:build windows
// +build windows

package file

func CheckFDLimitSupported() bool {
	return false
}

func GetLimit() (uint64, error) {
	return 0, nil
}

func GetUsage() (uint64, error) {
	return 0, nil
}
