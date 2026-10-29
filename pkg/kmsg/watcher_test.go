package kmsg

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	kmsgsource "github.com/dsx-ai-factory/health-validation/collect/source/kmsg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Fixture support for the deduper and event-syncer tests. Production records are
// parsed by health-validation and converted through sensor observations.
func parseLine(bootTime time.Time, line string) (*Message, error) {
	record, err := kmsgsource.Parse(bootTime, line)
	if err != nil {
		return nil, err
	}
	return &Message{Timestamp: metav1.NewTime(record.Timestamp), Priority: record.Priority, SequenceNumber: int(record.Sequence), Message: record.Body}, nil
}

func Test_DescribeTimestamp(t *testing.T) {
	// Reference time for all tests
	referenceTime := time.Date(2023, 1, 15, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name           string
		messageTime    time.Time
		referenceTime  time.Time
		expectedOutput string
	}{
		{
			name:           "exactly the same time",
			messageTime:    referenceTime,
			referenceTime:  referenceTime,
			expectedOutput: "now",
		},
		{
			name:           "1 minute ago",
			messageTime:    referenceTime.Add(-1 * time.Minute),
			referenceTime:  referenceTime,
			expectedOutput: "1 minute ago",
		},
		{
			name:           "5 minutes ago",
			messageTime:    referenceTime.Add(-5 * time.Minute),
			referenceTime:  referenceTime,
			expectedOutput: "5 minutes ago",
		},
		{
			name:           "1 hour ago",
			messageTime:    referenceTime.Add(-1 * time.Hour),
			referenceTime:  referenceTime,
			expectedOutput: "1 hour ago",
		},
		{
			name:           "1 day ago",
			messageTime:    referenceTime.Add(-24 * time.Hour),
			referenceTime:  referenceTime,
			expectedOutput: "1 day ago",
		},
		{
			name:           "1 minute in future",
			messageTime:    referenceTime.Add(1 * time.Minute),
			referenceTime:  referenceTime,
			expectedOutput: "1 minute from now",
		},
		{
			name:           "5 minutes in future",
			messageTime:    referenceTime.Add(5 * time.Minute),
			referenceTime:  referenceTime,
			expectedOutput: "5 minutes from now",
		},
		{
			name:           "1 hour in future",
			messageTime:    referenceTime.Add(1 * time.Hour),
			referenceTime:  referenceTime,
			expectedOutput: "1 hour from now",
		},
		{
			name:           "1 day in future",
			messageTime:    referenceTime.Add(24 * time.Hour),
			referenceTime:  referenceTime,
			expectedOutput: "1 day from now",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := Message{
				Timestamp: metav1.NewTime(tt.messageTime),
			}
			result := msg.DescribeTimestamp(tt.referenceTime)
			assert.Equal(t, tt.expectedOutput, result)
		})
	}
}

type fixtureStream struct {
	mu      sync.Mutex
	records []kmsgsource.Message
	err     error
	reads   atomic.Int32
	closed  atomic.Bool
}

func (s *fixtureStream) Read(context.Context) ([]kmsgsource.Message, error) {
	s.reads.Add(1)
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]kmsgsource.Message(nil), s.records...), s.err
}

func (s *fixtureStream) Close() error {
	s.closed.Store(true)
	return nil
}

func sourceRecord(sequence int64, body string) kmsgsource.Message {
	return kmsgsource.Message{BootID: "boot-a", Sequence: sequence, Timestamp: time.Unix(1700000000, 0), Priority: 6, Body: body}
}

func receiveMessage(t *testing.T, ch <-chan Message) Message {
	t.Helper()
	select {
	case message, ok := <-ch:
		require.True(t, ok, "watcher closed before expected message")
		return message
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for kernel message")
		return Message{}
	}
}

func TestWatcherWindowCursor(t *testing.T) {
	for _, dedup := range []bool{true, false} {
		t.Run(fmt.Sprintf("dedup=%v", dedup), func(t *testing.T) {
			stream := &fixtureStream{records: []kmsgsource.Message{sourceRecord(0, "same"), sourceRecord(1, "same"), sourceRecord(2, "last")}}
			var opts []OpOption
			if !dedup {
				opts = append(opts, withDisableDedup())
			}
			watcher, err := newWatcher(stream, opts...)
			require.NoError(t, err)
			t.Cleanup(func() { _ = watcher.Close() })
			ch, err := watcher.Watch()
			require.NoError(t, err)
			_, err = watcher.Watch()
			require.ErrorIs(t, err, ErrWatcherAlreadyStarted)
			assert.Equal(t, 0, receiveMessage(t, ch).SequenceNumber)
			if !dedup {
				assert.Equal(t, 1, receiveMessage(t, ch).SequenceNumber)
			}
			assert.Equal(t, 2, receiveMessage(t, ch).SequenceNumber)
			require.Eventually(t, func() bool { return stream.reads.Load() >= 2 }, 5*time.Second, 10*time.Millisecond)
			// Replayed window entries must not escape, even when content dedup is off.
			stream.mu.Lock()
			stream.records = append(stream.records[1:], sourceRecord(3, "new"))
			stream.mu.Unlock()
			assert.Equal(t, 3, receiveMessage(t, ch).SequenceNumber)
			require.NoError(t, watcher.Close())
			select {
			case _, ok := <-ch:
				assert.False(t, ok)
			case <-time.After(5 * time.Second):
				t.Fatal("watcher did not close")
			}
			assert.True(t, stream.closed.Load())
		})
	}
}

func TestWatcherCloseWithoutConsumer(t *testing.T) {
	stream := &fixtureStream{records: []kmsgsource.Message{sourceRecord(1, "first"), sourceRecord(2, "blocked")}}
	watcher, err := newWatcher(stream, withDisableDedup())
	require.NoError(t, err)
	t.Cleanup(func() { _ = watcher.Close() })
	ch := make(chan Message, 1)
	done := make(chan error, 1)
	go func() { done <- watcher.follow(ch) }()
	require.Eventually(t, func() bool { return len(ch) == cap(ch) }, 5*time.Second, 10*time.Millisecond)
	require.NoError(t, watcher.Close())
	// Wait for the producer to stop before draining the full channel.
	select {
	case err := <-done:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(5 * time.Second):
		t.Fatal("watcher remained blocked after Close")
	}
	require.NoError(t, watcher.Close())
}

func TestWatcherClosesOnCollectionFailure(t *testing.T) {
	stream := &fixtureStream{records: []kmsgsource.Message{sourceRecord(7, "retained")}, err: errors.New("reader failed")}
	watcher, err := newWatcher(stream)
	require.NoError(t, err)
	t.Cleanup(func() { _ = watcher.Close() })
	ch, err := watcher.Watch()
	require.NoError(t, err)
	assert.Equal(t, "retained", receiveMessage(t, ch).Message)
	select {
	case _, ok := <-ch:
		assert.False(t, ok)
	case <-time.After(5 * time.Second):
		t.Fatal("watcher did not terminate on collection failure")
	}
	assert.True(t, stream.closed.Load())
}

func TestWatcherCloseBeforeWatch(t *testing.T) {
	watcher, err := newWatcher(&fixtureStream{})
	require.NoError(t, err)
	require.NoError(t, watcher.Close())
	ch, err := watcher.Watch()
	require.ErrorIs(t, err, context.Canceled)
	assert.Nil(t, ch)
}

func TestRecordCursor(t *testing.T) {
	var cursor recordCursor
	for _, tc := range []struct {
		id         recordID
		fresh, gap bool
	}{
		{recordID{"boot-a", 0}, true, false},
		{recordID{"boot-a", 0}, false, false},
		{recordID{"boot-a", 1}, true, false},
		{recordID{"boot-a", 4}, true, true},
		{recordID{"boot-a", 2}, false, false},
		{recordID{"boot-b", 0}, true, false},
		{recordID{"boot-b", 1}, true, false},
	} {
		fresh, err := cursor.advance(tc.id)
		assert.Equal(t, tc.fresh, fresh)
		assert.Equal(t, tc.gap, err != nil)
	}
}

func TestUnsupportedKmsgPath(t *testing.T) {
	t.Setenv("KMSG_FILE_PATH", "/alternate/kmsg")
	_, err := NewWatcher()
	require.ErrorContains(t, err, "KMSG_FILE_PATH")
	_, err = ReadAll(t.Context())
	require.ErrorContains(t, err, "KMSG_FILE_PATH")
}
