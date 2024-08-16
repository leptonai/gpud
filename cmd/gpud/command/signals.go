package command

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/leptonai/gpud/internal/server"
	"github.com/leptonai/gpud/log"
	"github.com/leptonai/gpud/pkg/systemd"

	sd "github.com/coreos/go-systemd/v22/daemon"
	"golang.org/x/sys/unix"
)

var handledSignals = []os.Signal{
	unix.SIGTERM,
	unix.SIGINT,
	unix.SIGUSR1,
	unix.SIGPIPE,
}

func handleSignals(ctx context.Context, cancel context.CancelFunc, signals chan os.Signal, serverC chan *server.Server) chan struct{} {
	done := make(chan struct{}, 1)
	go func() {
		var server *server.Server
		for {
			select {
			case s := <-serverC:
				server = s
			case s := <-signals:

				// Do not print message when deailing with SIGPIPE, which may cause
				// nested signals and consume lots of cpu bandwidth.
				if s == unix.SIGPIPE {
					continue
				}

				log.Logger.Debugf("received signal: %v", s)
				switch s {
				case unix.SIGUSR1:
					dumpStacks(true)
				default:
					cancel()

					if systemd.SystemctlExists() {
						if err := notifyStopping(ctx); err != nil {
							log.Logger.Error("notify stopping failed")
						}
					}

					if server == nil {
						close(done)
						return
					}

					server.Stop()
					close(done)
					return
				}
			}
		}
	}()
	return done
}

// notifyReady notifies systemd that the daemon is ready to serve requests
func notifyReady(ctx context.Context) error {
	return sdNotify(ctx, sd.SdNotifyReady)
}

// notifyStopping notifies systemd that the daemon is about to be stopped
func notifyStopping(ctx context.Context) error {
	return sdNotify(ctx, sd.SdNotifyStopping)
}

func sdNotify(ctx context.Context, state string) error {
	notified, err := sd.SdNotify(false, state)
	log.Logger.Debugf("sd notification: %v %v %v", state, notified, err)
	return err
}

func dumpStacks(writeToFile bool) {
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

	if writeToFile {
		// Also write to file to aid gathering diagnostics
		name := filepath.Join(os.TempDir(), fmt.Sprintf("gpud.%d.stacks.log", os.Getpid()))
		f, err := os.Create(name)
		if err != nil {
			return
		}
		defer f.Close()
		_, _ = f.WriteString(string(buf))
		log.Logger.Debugf("goroutine stack dump written to %s", name)
	}
}
