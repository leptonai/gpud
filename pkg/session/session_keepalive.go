package session

import (
	"context"
	"errors"
	"net/http/cookiejar"

	"github.com/leptonai/gpud/pkg/log"
)

func (s *Session) keepAlive() {
	backoff := reconnectBackoff{}
	if !s.waitReconnectDelay(s.ctx, s.jitter(startupJitterMax)) {
		return
	}

	for {
		select {
		case <-s.ctx.Done():
			log.Logger.Debug("session keep alive: closing keep alive")
			return
		default:
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
				s.sleep(cleanupDrainDelay)

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

				// Persist authentication-related failures from health check to session_states,
				// so that "gpud status" can surface them. Without this, a rejected token
				// causes silent retry loops and "gpud status" never sees the failure.
				// The server may return 401/403 for auth errors, or 500 with
				// "failed to validate token" when the token is invalid.
				var httpErr *healthCheckHTTPError
				if errors.As(err, &httpErr) {
					sig := classifyHealthCheckError(httpErr)
					if sig.authFailure {
						s.persistLoginStatus(ctx, false, sig.reason)
					}
				}

				cancel()
				if s.ctx.Err() != nil {
					return
				}
				sig := classifyHealthCheckError(err)
				delay := backoff.nextDelay(s, sig)
				log.Logger.Debugw("session keep alive: attempting reconnection after delay", "delay", delay.String(), "reason", sig.reason, "retryAfter", sig.retryAfter.String())
				if !s.waitReconnectDelay(s.ctx, delay) {
					return
				}
				continue
			}

			readerExit := make(chan reconnectSignal, 1)
			writerExit := make(chan reconnectSignal, 1)
			connectionStartedAt := s.now()

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
			var firstExit reconnectSignal
			var secondExit reconnectSignal
			select {
			case firstExit = <-readerExit:
				log.Logger.Debug("session reader: reader exited first")
				cancel()                  // Signal writer to exit
				secondExit = <-writerExit // Wait for writer cleanup
				log.Logger.Debug("session writer: writer exited after cancellation")
			case firstExit = <-writerExit:
				log.Logger.Debug("session writer: writer exited first")
				cancel()                  // Signal reader to exit
				secondExit = <-readerExit // Wait for reader cleanup
				log.Logger.Debug("session reader: reader exited after cancellation")
			}
			if s.ctx.Err() != nil {
				return
			}

			if s.now().Sub(connectionStartedAt) >= reconnectStableWindow {
				backoff.reset()
			}
			sig := chooseReconnectSignal(firstExit, secondExit)
			delay := backoff.nextDelay(s, sig)
			log.Logger.Debugw("session keep alive: attempting reconnection after delay", "delay", delay.String(), "reason", sig.reason, "retryAfter", sig.retryAfter.String())
			if !s.waitReconnectDelay(s.ctx, delay) {
				return
			}
		}
	}
}
