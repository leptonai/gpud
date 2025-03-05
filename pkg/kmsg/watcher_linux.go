//go:build linux
// +build linux

package kmsg

import (
	"github.com/euank/go-kmsg-parser/v3/kmsgparser"
)

var _ Watcher = (*kmsgParser)(nil)

func NewWatcher() (Watcher, error) {
	parser, err := kmsgparser.NewParser()
	if err != nil {
		return nil, err
	}
	return &kmsgParser{parser: parser}, nil
}

type kmsgParser struct {
	parser kmsgparser.Parser
}

func (k *kmsgParser) Read(ch chan<- Message) error {
	ch2 := make(chan kmsgparser.Message, 1024)
	defer close(ch2)

	errc := make(chan error, 1)
	go func() {
		errc <- k.parser.Parse(ch2)
	}()

	for msg := range ch2 {
		select {
		case ch <- Message{
			Timestamp: msg.Timestamp,
			Message:   msg.Message,
		}:
		case err := <-errc:
			return err
		}

	}
	return nil
}

func (k *kmsgParser) Close() error {
	return k.parser.Close()
}
