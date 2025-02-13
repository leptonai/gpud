package log

import "sync"

var (
	globalPollersMu sync.RWMutex

	// maps the file name to its poller
	// assume, the common filters are merged for all components
	globalPollers = make(map[string]Poller)
)

func RegisterPoller(poller Poller) {
	globalPollersMu.Lock()
	defer globalPollersMu.Unlock()

	globalPollers[poller.File()] = poller
}

func GetPoller(fileName string) Poller {
	globalPollersMu.RLock()
	defer globalPollersMu.RUnlock()

	return globalPollers[fileName]
}
