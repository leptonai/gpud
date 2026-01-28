package kmsg

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/bytedance/mockey"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/leptonai/gpud/pkg/eventstore"
)

type stubWatcher struct {
	ch       chan Message
	watchErr error
}

func (s *stubWatcher) Watch() (<-chan Message, error) { return s.ch, s.watchErr }
func (s *stubWatcher) Close() error                   { return nil }

type findResult struct {
	ev  *eventstore.Event
	err error
}

type stubBucket struct {
	mu           sync.Mutex
	findResults  []findResult
	insertErrors []error
	inserted     []eventstore.Event
}

func (b *stubBucket) Name() string { return "stub" }

func (b *stubBucket) Insert(ctx context.Context, ev eventstore.Event) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.inserted = append(b.inserted, ev)
	if len(b.insertErrors) == 0 {
		return nil
	}
	err := b.insertErrors[0]
	b.insertErrors = b.insertErrors[1:]
	return err
}

func (b *stubBucket) Find(ctx context.Context, ev eventstore.Event) (*eventstore.Event, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.findResults) == 0 {
		return nil, nil
	}
	res := b.findResults[0]
	b.findResults = b.findResults[1:]
	return res.ev, res.err
}

func (b *stubBucket) Get(ctx context.Context, since time.Time) (eventstore.Events, error) {
	return nil, nil
}

func (b *stubBucket) Latest(ctx context.Context) (*eventstore.Event, error) { return nil, nil }
func (b *stubBucket) Purge(ctx context.Context, beforeTimestamp int64) (int, error) {
	return 0, nil
}
func (b *stubBucket) Close() {}

func (b *stubBucket) insertCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.inserted)
}

func TestNewSyncer_UsesNewWatcher_WithMockey(t *testing.T) {
	mockey.PatchConvey("NewSyncer uses NewWatcher when watcher is nil", t, func() {
		ch := make(chan Message)
		close(ch)
		sw := &stubWatcher{ch: ch}

		mockey.Mock(NewWatcher).To(func(opts ...OpOption) (Watcher, error) {
			return sw, nil
		}).Build()

		bucket := &stubBucket{}
		syncer, err := NewSyncer(context.Background(), func(string) (string, string) { return "", "" }, bucket)
		require.NoError(t, err)
		require.NotNil(t, syncer)
		syncer.Close()
	})
}

func TestNewSyncer_NewWatcherError_WithMockey(t *testing.T) {
	mockey.PatchConvey("NewSyncer returns error when NewWatcher fails", t, func() {
		mockey.Mock(NewWatcher).To(func(opts ...OpOption) (Watcher, error) {
			return nil, errors.New("watcher failed")
		}).Build()

		bucket := &stubBucket{}
		_, err := NewSyncer(context.Background(), func(string) (string, string) { return "", "" }, bucket)
		require.Error(t, err)
	})
}

func TestNewSyncer_WatchError(t *testing.T) {
	ch := make(chan Message)
	sw := &stubWatcher{ch: ch, watchErr: errors.New("watch failed")}
	bucket := &stubBucket{}

	_, err := newSyncer(context.Background(), sw, func(string) (string, string) { return "", "" }, bucket)
	require.Error(t, err)
}

func TestSyncer_SyncBranches(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch := make(chan Message, 4)
	sw := &stubWatcher{ch: ch}
	bucket := &stubBucket{
		findResults: []findResult{
			{ev: nil, err: errors.New("find failed")},
			{ev: &eventstore.Event{}},
			{ev: nil, err: nil},
		},
		insertErrors: []error{nil, errors.New("insert failed")},
	}

	match := func(line string) (string, string) {
		switch line {
		case "skip":
			return "", ""
		case "finderr":
			return "event.finderr", "message-1"
		case "dupe":
			return "event.dupe", "message-2"
		case "inserterr":
			return "event.inserterr", "message-3"
		default:
			return "event.default", line
		}
	}

	syncer, err := newSyncer(ctx, sw, match, bucket)
	require.NoError(t, err)
	defer syncer.Close()

	now := metav1.NewTime(time.Now())
	ch <- Message{Timestamp: now, Message: "skip"}
	ch <- Message{Timestamp: now, Message: "finderr"}
	ch <- Message{Timestamp: now, Message: "dupe"}
	ch <- Message{Timestamp: now, Message: "inserterr"}
	close(ch)

	require.Eventually(t, func() bool {
		return bucket.insertCount() == 2
	}, time.Second, 10*time.Millisecond)
}
