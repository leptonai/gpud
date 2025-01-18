//go:build darwin
// +build darwin

package memory

func GetCurrentProcessRSSInBytes() (uint64, error) {
	return 0, nil
}
