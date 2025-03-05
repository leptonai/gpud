//go:build darwin
// +build darwin

package kmsg

func NewWatcher() (Watcher, error) {
	return &emptyWatcher{}, nil
}
