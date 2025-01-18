//go:build darwin
// +build darwin

package uptime

func GetCurrentProcessStartTimeInUnixTime() (uint64, error) {
	return 0, nil
}
