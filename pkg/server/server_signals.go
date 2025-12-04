package server

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"golang.org/x/sys/unix"

	"github.com/leptonai/gpud/pkg/log"
)

type ServerStopper interface {
	Stop()
}

var DefaultSignalsToHandle = []os.Signal{
	unix.SIGTERM,
	unix.SIGINT,
	unix.SIGUSR1,
	unix.SIGPIPE,
}

// HandleSignals handles signals and stops the server.
func HandleSignals(ctx context.Context, cancel context.CancelFunc, signals chan os.Signal, serverC chan ServerStopper, notifyStopping func(ctx context.Context) error) chan struct{} {
	done := make(chan struct{}, 1)
	go func() {
		var srv ServerStopper
		for {
			select {
			case s := <-serverC:
				srv = s

			case s := <-signals:
				// Do not print message when deailing with SIGPIPE, which may cause
				// nested signals and consume lots of cpu bandwidth.
				if s == unix.SIGPIPE {
					continue
				}

				log.Logger.Warnw("received signal -- stopping server and notifying", "signal", s)
				switch s {
				case unix.SIGUSR1:
					file := filepath.Join(os.TempDir(), fmt.Sprintf("gpud.%d.stacks.log", os.Getpid()))
					dumpStacks(file)

				default:
					cancel()

					if err := notifyStopping(ctx); err != nil {
						log.Logger.Errorw("notify stopping failed")
					}

					if srv != nil {
						srv.Stop()
					}

					close(done)
					return
				}
			}
		}
	}()
	return done
}

func dumpStacks(file string) {
	var (
		buf       []byte
		stackSize int
	)
	bufferLen := 16384
	for stackSize == len(buf) {
		buf = make([]byte, bufferLen)
		stackSize = runtime.Stack(buf, true)
		bufferLen *= 2
	}
	buf = buf[:stackSize]
	log.Logger.Debugf("=== BEGIN goroutine stack dump ===\n%s\n=== END goroutine stack dump ===", buf)

	f, err := os.Create(file)
	if err != nil {
		return
	}
	defer func() {
		if cerr := f.Close(); cerr != nil {
			log.Logger.Errorw("failed to close stack trace file", "error", cerr)
		}
	}()

	_, err = f.WriteString(string(buf))
	if err != nil {
		log.Logger.Errorw("failed to write stack trace to file", "error", err)
	} else {
		log.Logger.Debugw("goroutine stack dump written to file", "file", file)
	}
}
