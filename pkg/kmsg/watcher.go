package kmsg

import (
	"time"
)

// We cannot direct import "kmsgparser.Parser",
// if we want to support cross-platform compilation.
// Otherwise,
// "github.com/euank/go-kmsg-parser/v3@v3.0.0/kmsgparser/kmsgparser.go:113:22: undefined: syscall.Sysinfo_t"
type Watcher interface {
	Read(ch chan<- Message) error
	Close() error
}

// We cannot direct import "kmsgparser.Message",
// if we want to support cross-platform compilation.
// Otherwise,
// "github.com/euank/go-kmsg-parser/v3@v3.0.0/kmsgparser/kmsgparser.go:113:22: undefined: syscall.Sysinfo_t"
type Message struct {
	Priority       int
	SequenceNumber int
	Timestamp      time.Time
	Message        string
}

type emptyWatcher struct{}

func (e *emptyWatcher) Read(ch chan<- Message) error {
	close(ch)
	return nil
}

func (e *emptyWatcher) Close() error {
	return nil
}
