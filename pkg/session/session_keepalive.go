package session

import (
	"context"
	"net/http/cookiejar"
	"time"

	"github.com/leptonai/gpud/pkg/log"
)

func (s *Session) keepAlive() {
	// Start the initial connection immediately
	firstConnection := true

	for {
		select {
		case <-s.ctx.Done():
			log.Logger.Debug("session keep alive: closing keep alive")
			return
		default:
			// CRITICAL: Reconnection delay to prevent race conditions
			//
			// Without this delay, rapid connection failures would create multiple
			// overlapping reader/writer goroutines that all write to the same channels,
			// causing "reader channel full" errors and making GPUd unresponsive.
			//
			// The 3-second delay ensures:
			// - Previous connections have time to fully clean up
			// - We don't overwhelm the control plane with rapid reconnection attempts
			// - Only one set of reader/writer goroutines exists at a time
			if !firstConnection {
				select {
				case <-s.ctx.Done():
					return
				case <-s.timeAfterFunc(3 * time.Second):
					log.Logger.Debug("session keep alive: attempting reconnection after delay")
				}
			}
			firstConnection = false

			readerExit := make(chan any)
			writerExit := make(chan any)

			// CLEANUP: Ensure previous connections are fully terminated
			//
			// This cleanup is essential to prevent the "reader channel full" bug:
			// - s.closer.Close() signals any existing reader/writer goroutines to exit
			// - The sleep gives them time to process the signal and clean up
			// - We drain stale messages to prevent them from blocking new connections
			//
			// Without this cleanup, old goroutines could still be writing to channels
			// when new ones start, causing race conditions and channel overflow
			if s.closer != nil {
				s.closer.Close()

				// Give old goroutines time to detect closer signal and exit
				s.timeSleepFunc(100 * time.Millisecond)

				// Drain any stale messages left in the reader channel
				s.drainReaderChannel()
			}

			s.closer = &closeOnce{closer: make(chan any)}
			ctx, cancel := context.WithCancel(s.ctx) // create local context derived from session context
			// DO NOT CHANGE OR REMOVE THIS COOKIE JAR, DEPEND ON IT FOR STICKY SESSION
			jar, _ := cookiejar.New(nil)

			// DO NOT CHANGE OR REMOVE THIS SERVER HEALTH CHECK, DEPEND ON IT FOR STICKY SESSION
			// TODO: we can remove it once we migrate to gpud-gateway
			if err := s.checkServerHealthFunc(ctx, jar, ""); err != nil {
				log.Logger.Errorf("session keep alive: error checking server health: %v", err)
				cancel()
				continue
			}

			go s.startReaderFunc(ctx, readerExit, jar)
			go s.startWriterFunc(ctx, writerExit, jar)

			// CRITICAL: We must handle EITHER reader or writer exiting first to prevent deadlock
			//
			// Why we use select instead of waiting for reader then writer:
			// 1. Reader can fail first: EOF, network errors, decode errors
			// 2. Writer can fail first: connection broken during send, pipe closed
			// 3. If we always waited for reader first (<-readerExit), we'd deadlock if writer exits first
			//
			// How this prevents deadlock:
			// - Whichever goroutine exits first (reader OR writer) triggers the cleanup
			// - We immediately cancel the context to signal the other goroutine to exit
			// - Then we wait for the other goroutine to finish cleanup
			// - This ensures both goroutines are fully terminated before reconnecting
			//
			// This pattern guarantees:
			// - No deadlock: We handle both possible exit orders
			// - Clean shutdown: Both goroutines exit before we create new ones
			// - No goroutine leaks: Context cancellation ensures termination
			select {
			case <-readerExit:
				log.Logger.Debug("session reader: reader exited first")
				cancel()     // Signal writer to exit
				<-writerExit // Wait for writer cleanup
				log.Logger.Debug("session writer: writer exited after cancellation")
			case <-writerExit:
				log.Logger.Debug("session writer: writer exited first")
				cancel()     // Signal reader to exit
				<-readerExit // Wait for reader cleanup
				log.Logger.Debug("session reader: reader exited after cancellation")
			}
		}
	}
}
