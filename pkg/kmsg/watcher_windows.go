//go:build windows
// +build windows

package kmsg

func NewWatcher() (Watcher, error) {
	return &emptyWatcher{}, nil
}
