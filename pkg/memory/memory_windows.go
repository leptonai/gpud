//go:build windows
// +build windows

package memory

func GetCurrentProcessRSSInBytes() (uint64, error) {
	return 0, nil
}
