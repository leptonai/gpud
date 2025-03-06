package kmsg

import (
	"os"
	"runtime"

	"github.com/leptonai/gpud/pkg/log"
)

// CreateEventsWatcher creates a new events watcher that will watch the kmsg.
// Experimental.
// TODO: integrate with the events store.
func CreateEventsWatcher(matchFunc func(line string) (eventName string, message string)) (Watcher, error) {
	if os.Getenv("GPUD_EXPERIMENTAL_KMSG_WATCHER") != "true" {
		return nil, nil
	}
	if runtime.GOOS != "linux" {
		log.Logger.Warnw("kmsg watcher is not supported on non-linux systems")
		return nil, nil
	}
	if os.Geteuid() != 0 {
		log.Logger.Warnw("kmsg watcher is not supported for non-root users")
		return nil, nil
	}
	kmsgWatcher, err := NewWatcher()
	if err != nil {
		return nil, err
	}
	kmsgCh := make(chan Message, 1024)
	go func() {
		werr := kmsgWatcher.Watch(kmsgCh)
		if werr != nil {
			log.Logger.Errorw("failed to watch kmsg", "err", werr)
		}
	}()
	go func() {
		for m := range kmsgCh {
			ev, msg := matchFunc(m.Message)
			if msg != "" {
				log.Logger.Infow("[EXPERIMENTAL] kmsg event", "event", ev, "message", msg, "raw", m.Message)
			}
		}
	}()
	return kmsgWatcher, nil
}
