//go:build windows
// +build windows

package uptime

func GetCurrentProcessStartTimeInUnixTime() (uint64, error) {
	return 0, nil
}
