package session

import (
	"context"
	"fmt"
	"net/http/cookiejar"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestKeepAliveReconnectionDelay verifies that keepAlive waits 3 seconds before reconnecting
func TestKeepAliveReconnectionDelay(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s := &Session{
		ctx:    ctx,
		reader: make(chan Body, 20),
		writer: make(chan Body, 20),
	}

	// Mock checkServerHealth to always succeed
	s.checkServerHealthFunc = func(ctx context.Context, jar *cookiejar.Jar, token string) error {
		return nil
	}

	// Track when timeAfter is called
	var timeAfterCalled int32
	s.timeAfterFunc = func(d time.Duration) <-chan time.Time {
		atomic.AddInt32(&timeAfterCalled, 1)
		assert.Equal(t, 3*time.Second, d, "Expected 3 second delay")
		ch := make(chan time.Time, 1)
		ch <- time.Now() // Return immediately for testing
		return ch
	}

	// Mock sleep to avoid delays in test
	var sleepCalled int32
	s.timeSleepFunc = func(d time.Duration) {
		atomic.AddInt32(&sleepCalled, 1)
		assert.Equal(t, 100*time.Millisecond, d, "Expected 100ms cleanup sleep")
	}

	// Track reader/writer starts
	var readerStarts int32
	var writerStarts int32

	s.startReaderFunc = func(ctx context.Context, readerExit chan any, jar *cookiejar.Jar) {
		atomic.AddInt32(&readerStarts, 1)
		// Simulate immediate exit to trigger reconnection
		close(readerExit)
	}

	s.startWriterFunc = func(ctx context.Context, writerExit chan any, jar *cookiejar.Jar) {
		atomic.AddInt32(&writerStarts, 1)
		// Simulate exit after reader
		go func() {
			<-ctx.Done()
			close(writerExit)
		}()
	}

	// Run keepAlive in background
	go s.keepAlive()

	// Give it time to run through at least 2 connection attempts
	time.Sleep(200 * time.Millisecond)
	cancel()
	time.Sleep(50 * time.Millisecond)

	// Verify reconnection delay was used (called for second connection attempt)
	assert.GreaterOrEqual(t, atomic.LoadInt32(&timeAfterCalled), int32(1),
		"timeAfter should be called for reconnection delay")

	// Verify cleanup sleep was called
	assert.GreaterOrEqual(t, atomic.LoadInt32(&sleepCalled), int32(1),
		"Sleep should be called for cleanup")

	// Verify reader and writer were started
	assert.GreaterOrEqual(t, atomic.LoadInt32(&readerStarts), int32(1),
		"Reader should be started at least once")
	assert.GreaterOrEqual(t, atomic.LoadInt32(&writerStarts), int32(1),
		"Writer should be started at least once")
}

// TestKeepAliveChannelDraining verifies that stale messages are drained
func TestKeepAliveChannelDraining(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s := &Session{
		ctx:    ctx,
		reader: make(chan Body, 20),
		writer: make(chan Body, 20),
	}

	// Add some messages to reader channel
	for i := 0; i < 5; i++ {
		s.reader <- Body{Data: []byte("test")}
	}

	// Verify channel has messages
	assert.Equal(t, 5, len(s.reader), "Channel should have 5 messages")

	// Drain the channel
	s.drainReaderChannel()

	// Verify channel is empty
	assert.Equal(t, 0, len(s.reader), "Channel should be empty after draining")
}

// TestKeepAliveDeadlockPrevention verifies that either reader or writer can exit first
func TestKeepAliveDeadlockPrevention(t *testing.T) {
	testCases := []struct {
		name        string
		readerFirst bool
	}{
		{"ReaderExitsFirst", true},
		{"WriterExitsFirst", false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			s := &Session{
				ctx:    ctx,
				reader: make(chan Body, 20),
				writer: make(chan Body, 20),
			}

			// Mock checkServerHealth to always succeed
			s.checkServerHealthFunc = func(ctx context.Context, jar *cookiejar.Jar, token string) error {
				return nil
			}

			// Mock time functions to speed up test
			s.timeAfterFunc = func(d time.Duration) <-chan time.Time {
				ch := make(chan time.Time, 1)
				ch <- time.Now()
				return ch
			}
			s.timeSleepFunc = func(d time.Duration) {}

			// Use a channel to signal when both have exited
			bothExited := make(chan bool)

			// Track which exit happened
			var readerExited, writerExited bool
			var mu sync.Mutex
			var firstCall int32

			s.startReaderFunc = func(ctx context.Context, readerExit chan any, jar *cookiejar.Jar) {
				// Only track the first call
				if atomic.AddInt32(&firstCall, 1) > 2 {
					close(readerExit)
					return
				}

				go func() {
					if tc.readerFirst {
						// Reader exits first
						time.Sleep(10 * time.Millisecond)
						close(readerExit)
						mu.Lock()
						readerExited = true
						if writerExited {
							select {
							case bothExited <- true:
							default:
							}
						}
						mu.Unlock()
					} else {
						// Wait for context cancellation
						<-ctx.Done()
						time.Sleep(20 * time.Millisecond)
						close(readerExit)
						mu.Lock()
						readerExited = true
						if writerExited {
							select {
							case bothExited <- true:
							default:
							}
						}
						mu.Unlock()
					}
				}()
			}

			s.startWriterFunc = func(ctx context.Context, writerExit chan any, jar *cookiejar.Jar) {
				// Only track the first call
				if atomic.LoadInt32(&firstCall) > 2 {
					close(writerExit)
					return
				}

				go func() {
					if !tc.readerFirst {
						// Writer exits first
						time.Sleep(10 * time.Millisecond)
						close(writerExit)
						mu.Lock()
						writerExited = true
						if readerExited {
							select {
							case bothExited <- true:
							default:
							}
						}
						mu.Unlock()
					} else {
						// Wait for context cancellation
						<-ctx.Done()
						time.Sleep(20 * time.Millisecond)
						close(writerExit)
						mu.Lock()
						writerExited = true
						if readerExited {
							select {
							case bothExited <- true:
							default:
							}
						}
						mu.Unlock()
					}
				}()
			}

			// Run keepAlive in background
			go s.keepAlive()

			// Wait for both to exit
			select {
			case <-bothExited:
				// Good, both exited
			case <-time.After(1 * time.Second):
				t.Fatal("Timeout waiting for both goroutines to exit")
			}

			cancel()

			// Verify both goroutines exited
			mu.Lock()
			assert.True(t, readerExited, "Reader should have exited")
			assert.True(t, writerExited, "Writer should have exited")
			mu.Unlock()
		})
	}
}

// TestKeepAliveNoRapidReconnection verifies that rapid reconnections don't create overlapping goroutines
func TestKeepAliveNoRapidReconnection(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s := &Session{
		ctx:    ctx,
		reader: make(chan Body, 20),
		writer: make(chan Body, 20),
	}

	// Mock checkServerHealth to always fail (simulate rapid failures)
	healthCheckCount := int32(0)
	s.checkServerHealthFunc = func(ctx context.Context, jar *cookiejar.Jar, token string) error {
		atomic.AddInt32(&healthCheckCount, 1)
		// Fail health checks to test reconnection behavior
		return fmt.Errorf("simulated health check failure")
	}

	// Track active goroutines
	var activeReaders int32
	var activeWriters int32
	var maxConcurrentReaders int32
	var maxConcurrentWriters int32

	// Mock instant time functions
	s.timeAfterFunc = func(d time.Duration) <-chan time.Time {
		ch := make(chan time.Time, 1)
		ch <- time.Now()
		return ch
	}
	s.timeSleepFunc = func(d time.Duration) {}

	s.startReaderFunc = func(ctx context.Context, readerExit chan any, jar *cookiejar.Jar) {
		current := atomic.AddInt32(&activeReaders, 1)
		// Track max concurrent readers
		for {
			max := atomic.LoadInt32(&maxConcurrentReaders)
			if current <= max || atomic.CompareAndSwapInt32(&maxConcurrentReaders, max, current) {
				break
			}
		}

		go func() {
			// Simulate some work
			time.Sleep(50 * time.Millisecond)
			atomic.AddInt32(&activeReaders, -1)
			close(readerExit)
		}()
	}

	s.startWriterFunc = func(ctx context.Context, writerExit chan any, jar *cookiejar.Jar) {
		current := atomic.AddInt32(&activeWriters, 1)
		// Track max concurrent writers
		for {
			max := atomic.LoadInt32(&maxConcurrentWriters)
			if current <= max || atomic.CompareAndSwapInt32(&maxConcurrentWriters, max, current) {
				break
			}
		}

		go func() {
			<-ctx.Done()
			atomic.AddInt32(&activeWriters, -1)
			close(writerExit)
		}()
	}

	// Run keepAlive
	go s.keepAlive()

	// Let it run for a bit
	time.Sleep(300 * time.Millisecond)
	cancel()
	time.Sleep(100 * time.Millisecond)

	// Verify we never had more than 1 concurrent reader/writer
	// (allowing 2 for brief overlap during cleanup)
	assert.LessOrEqual(t, atomic.LoadInt32(&maxConcurrentReaders), int32(2),
		"Should not have more than 2 concurrent readers")
	assert.LessOrEqual(t, atomic.LoadInt32(&maxConcurrentWriters), int32(2),
		"Should not have more than 2 concurrent writers")
}

// TestDrainReaderChannel verifies the drainReaderChannel function
func TestDrainReaderChannel(t *testing.T) {
	s := &Session{
		reader: make(chan Body, 20),
	}

	// Test empty channel
	s.drainReaderChannel()
	assert.Equal(t, 0, len(s.reader), "Empty channel should remain empty")

	// Test channel with messages
	for i := 0; i < 10; i++ {
		s.reader <- Body{Data: []byte("test"), ReqID: string(rune('0' + i))}
	}

	assert.Equal(t, 10, len(s.reader), "Channel should have 10 messages")

	s.drainReaderChannel()

	assert.Equal(t, 0, len(s.reader), "Channel should be empty after draining")

	// Test channel at capacity
	for i := 0; i < 20; i++ {
		s.reader <- Body{Data: []byte("test")}
	}

	assert.Equal(t, 20, len(s.reader), "Channel should be at capacity")

	s.drainReaderChannel()

	assert.Equal(t, 0, len(s.reader), "Full channel should be empty after draining")
}

// TestKeepAliveContextCancellation verifies proper shutdown on context cancellation
func TestKeepAliveContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	s := &Session{
		ctx:    ctx,
		reader: make(chan Body, 20),
		writer: make(chan Body, 20),
	}

	// Mock functions to track calls
	keepAliveExited := make(chan bool)

	// Cancel context immediately
	cancel()

	go func() {
		s.keepAlive()
		close(keepAliveExited)
	}()

	// Wait for keepAlive to exit
	select {
	case <-keepAliveExited:
		// Success - keepAlive exited on context cancellation
	case <-time.After(1 * time.Second):
		t.Fatal("keepAlive did not exit on context cancellation")
	}
}
