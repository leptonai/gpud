package kmsg

import (
	"errors"
	"sync"
	"testing"
	"time"

	kmsgsource "github.com/dsx-ai-factory/health-validation/collect/source/kmsg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSharedWatchersIndependentDeliveryAndLifetime(t *testing.T) {
	source := &fixtureStream{records: []kmsgsource.Message{sourceRecord(0, "repeat"), sourceRecord(1, "repeat"), sourceRecord(2, "last")}}
	var opens int
	pool := watcherPool{open: func() (stream, error) { opens++; return source, nil }}
	deduped, err := pool.newWatcher()
	require.NoError(t, err)
	t.Cleanup(func() { _ = deduped.Close() })
	raw, err := pool.newWatcher(withDisableDedup())
	require.NoError(t, err)
	t.Cleanup(func() { _ = raw.Close() })
	assert.Equal(t, 1, opens)
	assert.Same(t, deduped.collection, raw.collection)
	dedupedCh, err := deduped.Watch()
	require.NoError(t, err)
	rawCh, err := raw.Watch()
	require.NoError(t, err)
	assert.Equal(t, 0, receiveMessage(t, dedupedCh).SequenceNumber)
	assert.Equal(t, 2, receiveMessage(t, dedupedCh).SequenceNumber)
	for i := range 3 {
		assert.Equal(t, i, receiveMessage(t, rawCh).SequenceNumber)
	}
	require.NoError(t, deduped.Close())
	assert.False(t, source.closed.Load(), "one component must not close its peers' reader")

	// A component that starts later gets the retained window independently.
	late, err := pool.newWatcher(withDisableDedup())
	require.NoError(t, err)
	t.Cleanup(func() { _ = late.Close() })
	lateCh, err := late.Watch()
	require.NoError(t, err)
	for i := range 3 {
		assert.Equal(t, i, receiveMessage(t, lateCh).SequenceNumber)
	}
	assert.Equal(t, 1, opens)

	source.mu.Lock()
	source.records = append(source.records, sourceRecord(3, "new"))
	source.mu.Unlock()
	assert.Equal(t, 3, receiveMessage(t, rawCh).SequenceNumber)
	assert.Equal(t, 3, receiveMessage(t, lateCh).SequenceNumber)
	require.NoError(t, raw.Close())
	assert.False(t, source.closed.Load())
	require.NoError(t, late.Close())
	assert.EqualValues(t, 1, source.closes.Load())
	require.NoError(t, late.Close())
	assert.EqualValues(t, 1, source.closes.Load())
}

func TestSharedWatchersConcurrentCollection(t *testing.T) {
	source := &fixtureStream{records: []kmsgsource.Message{sourceRecord(1, "shared")}}
	pool := watcherPool{open: func() (stream, error) { return source, nil }}
	const count = 32
	watchers := make([]*watcher, count)
	for i := range watchers {
		var err error
		watchers[i], err = pool.newWatcher()
		require.NoError(t, err)
		t.Cleanup(func() { _ = watchers[i].Close() })
	}
	// Prime one collection, then extend its cache lifetime so scheduling delays
	// under the race detector cannot turn this into a timing-sensitive assertion.
	_, err := watchers[0].collection.read()
	require.NoError(t, err)
	watchers[0].collection.nextRead = time.Now().Add(time.Hour)
	var wg sync.WaitGroup
	for _, w := range watchers {
		wg.Go(func() {
			records, err := w.collection.read()
			assert.NoError(t, err)
			assert.Len(t, records, 1)
		})
	}
	wg.Wait()
	assert.EqualValues(t, 1, source.reads.Load(), "components must share the same measurement")
	for _, w := range watchers {
		wg.Go(func() { assert.NoError(t, w.Close()) })
	}
	wg.Wait()
	assert.EqualValues(t, 1, source.closes.Load())
}

func TestSharedCollectionReusesUnchangedWindow(t *testing.T) {
	source := &fixtureStream{}
	w, err := newWatcher(source)
	require.NoError(t, err)
	t.Cleanup(func() { _ = w.Close() })
	c := w.collection
	read := func() ([]Message, error) {
		// No watcher goroutine is running in this test. Advance past the cache
		// interval without sleeping, then exercise collection through the sensor.
		c.nextRead = time.Time{}
		return c.read()
	}
	records, err := read()
	require.NoError(t, err)
	require.Empty(t, records, "startup window may be empty")
	source.records = []kmsgsource.Message{sourceRecord(0, "first")}
	first, err := read()
	require.NoError(t, err)
	require.Len(t, first, 1)
	replayed, err := read()
	require.NoError(t, err)
	require.Len(t, replayed, 1)
	assert.Same(t, &first[0], &replayed[0], "unchanged windows must reuse converted records")
	source.records = append(source.records, sourceRecord(1, "new"))
	updated, err := read()
	require.NoError(t, err)
	require.Len(t, updated, 2)
	assert.Equal(t, "new", updated[1].Message)
	assert.Equal(t, "first", first[0].Message, "older snapshots remain immutable during delivery")
	// A new failure must be reported even when the stream's records are unchanged.
	source.err = errors.New("reader failed")
	partial, err := read()
	require.ErrorContains(t, err, "reader failed")
	assert.Equal(t, updated, partial)
}

func TestSharedWatchersSlowConsumerDoesNotBlockPeer(t *testing.T) {
	source := &fixtureStream{records: []kmsgsource.Message{sourceRecord(1, "one"), sourceRecord(2, "two")}}
	pool := watcherPool{open: func() (stream, error) { return source, nil }}
	slow, err := pool.newWatcher(withDisableDedup())
	require.NoError(t, err)
	t.Cleanup(func() { _ = slow.Close() })
	fast, err := pool.newWatcher(withDisableDedup())
	require.NoError(t, err)
	t.Cleanup(func() { _ = fast.Close() })
	blocked := make(chan Message, 1)
	done := make(chan error, 1)
	go func() { done <- slow.follow(blocked) }()
	require.Eventually(t, func() bool { return len(blocked) == cap(blocked) }, 5*time.Second, time.Millisecond)
	fastCh, err := fast.Watch()
	require.NoError(t, err)
	assert.Equal(t, 1, receiveMessage(t, fastCh).SequenceNumber)
	assert.Equal(t, 2, receiveMessage(t, fastCh).SequenceNumber)
	source.mu.Lock()
	source.records = []kmsgsource.Message{sourceRecord(3, "three")}
	source.mu.Unlock()
	assert.Equal(t, 3, receiveMessage(t, fastCh).SequenceNumber)
	require.NoError(t, slow.Close())
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("slow subscription did not cancel its blocked delivery")
	}
	assert.False(t, source.closed.Load())
}

func TestSharedWatchersFailureAndReopen(t *testing.T) {
	failed := &fixtureStream{records: []kmsgsource.Message{sourceRecord(7, "retained")}, err: errors.New("reader failed")}
	healthy := &fixtureStream{records: []kmsgsource.Message{sourceRecord(8, "recovered")}}
	opens := 0
	pool := watcherPool{open: func() (stream, error) {
		opens++
		if opens == 1 {
			return failed, nil
		}
		return healthy, nil
	}}
	first, err := pool.newWatcher()
	require.NoError(t, err)
	t.Cleanup(func() { _ = first.Close() })
	second, err := pool.newWatcher()
	require.NoError(t, err)
	t.Cleanup(func() { _ = second.Close() })
	for _, w := range []*watcher{first, second} {
		ch, err := w.Watch()
		require.NoError(t, err)
		assert.Equal(t, "retained", receiveMessage(t, ch).Message)
		select {
		case _, ok := <-ch:
			require.False(t, ok)
		case <-time.After(5 * time.Second):
			t.Fatal("subscription did not terminate after partial collection failure")
		}
	}
	assert.EqualValues(t, 1, failed.reads.Load())
	assert.EqualValues(t, 1, failed.closes.Load())
	reopened, err := pool.newWatcher()
	require.NoError(t, err)
	t.Cleanup(func() { _ = reopened.Close() })
	ch, err := reopened.Watch()
	require.NoError(t, err)
	assert.Equal(t, "recovered", receiveMessage(t, ch).Message)
	assert.Equal(t, 2, opens)
}

func TestSharedWatchersBootIdentity(t *testing.T) {
	source := &fixtureStream{records: []kmsgsource.Message{sourceRecord(10, "old boot")}}
	w, err := newWatcher(source, withDisableDedup())
	require.NoError(t, err)
	t.Cleanup(func() { _ = w.Close() })
	ch, err := w.Watch()
	require.NoError(t, err)
	assert.Equal(t, 10, receiveMessage(t, ch).SequenceNumber)
	newBoot := sourceRecord(0, "new boot")
	newBoot.BootID = "boot-b"
	source.mu.Lock()
	source.records = []kmsgsource.Message{newBoot}
	source.mu.Unlock()
	assert.Equal(t, 0, receiveMessage(t, ch).SequenceNumber)
}
