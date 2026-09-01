package nvsentinel

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/leptonai/gpud/pkg/nvsentinel/proto/datamodels"
)

// shortSocketPath returns a short unix socket path. t.TempDir() paths exceed
// the 104-character sun_path limit on macOS once the test name is included.
func shortSocketPath(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "nvs")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return filepath.Join(dir, "nvsentinel.sock")
}

// newTestClient dials the receiver the same way the NVSentinel platform
// connector gRPC sink connector does: an insecure client on a unix target.
func newTestClient(t *testing.T, socketPath string) datamodels.PlatformConnectorClient {
	t.Helper()
	conn, err := grpc.NewClient(
		"unix://"+socketPath,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	return datamodels.NewPlatformConnectorClient(conn)
}

func TestNewRequiresAbsolutePath(t *testing.T) {
	_, err := New("relative/path.sock")
	require.Error(t, err)

	_, err = New("")
	require.Error(t, err)
}

func TestSourceReceivesEventsOverUnixSocket(t *testing.T) {
	socketPath := shortSocketPath(t)

	src, err := New(socketPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = src.Close() })

	events, unsubscribe := src.Subscribe()
	defer unsubscribe()

	client := newTestClient(t, socketPath)

	sent := &datamodels.HealthEvent{
		Version:            1,
		Agent:              "syslog-health-monitor",
		ComponentClass:     "GPU",
		CheckName:          "SysLogsXIDError",
		IsFatal:            true,
		Message:            "Xid 79",
		RecommendedAction:  datamodels.RecommendedAction_RESTART_BM,
		ErrorCode:          []string{"79"},
		EntitiesImpacted:   []*datamodels.Entity{{EntityType: "GPU", EntityValue: "GPU-abc"}},
		GeneratedTimestamp: timestamppb.Now(),
		NodeName:           "node-1",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err = client.HealthEventOccurredV1(ctx, &datamodels.HealthEvents{Version: 1, Events: []*datamodels.HealthEvent{sent}})
	require.NoError(t, err)

	select {
	case ev := <-events:
		assert.Equal(t, "SysLogsXIDError", ev.CheckName)
		assert.Equal(t, "GPU", ev.ComponentClass)
		assert.Equal(t, []string{"79"}, ev.ErrorCodes)
		assert.False(t, src.LastReceived().IsZero())
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for forwarded event")
	}
}

func TestSourceCovers(t *testing.T) {
	socketPath := shortSocketPath(t)

	src, err := New(socketPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = src.Close() })

	client := newTestClient(t, socketPath)

	matchXid79 := func(ev HealthEvent) bool {
		return ev.CheckName == "SysLogsXIDError" && len(ev.ErrorCodes) > 0 && ev.ErrorCodes[0] == "79"
	}

	// Nothing received yet: no coverage.
	assert.False(t, src.Covers(2*time.Minute, matchXid79))

	old := &datamodels.HealthEvent{
		CheckName:          "SysLogsXIDError",
		ComponentClass:     "GPU",
		ErrorCode:          []string{"79"},
		GeneratedTimestamp: timestamppb.New(time.Now().Add(-time.Hour)),
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err = client.HealthEventOccurredV1(ctx, &datamodels.HealthEvents{Version: 1, Events: []*datamodels.HealthEvent{old}})
	require.NoError(t, err)

	// Receiving an event alone must not cover it: coverage represents
	// successful component persistence, reported via RecordCoverage.
	assert.False(t, src.Covers(2*time.Hour, matchXid79))

	// Simulate the component persisting the old event.
	src.RecordCoverage(healthEventFromProto(old, time.Now()))

	// An event older than the window does not cover.
	assert.False(t, src.Covers(2*time.Minute, matchXid79))
	// The same event covers when the window includes it.
	assert.True(t, src.Covers(2*time.Hour, matchXid79))

	fresh := &datamodels.HealthEvent{
		CheckName:          "SysLogsXIDError",
		ComponentClass:     "GPU",
		ErrorCode:          []string{"79"},
		GeneratedTimestamp: timestamppb.Now(),
	}
	_, err = client.HealthEventOccurredV1(ctx, &datamodels.HealthEvents{Version: 1, Events: []*datamodels.HealthEvent{fresh}})
	require.NoError(t, err)

	// The fresh event is received but not yet persisted: still no new
	// coverage within the short window (the old event is outside it).
	assert.False(t, src.Covers(2*time.Minute, matchXid79))

	// Once the component persists the fresh event, it covers.
	src.RecordCoverage(healthEventFromProto(fresh, time.Now()))
	assert.True(t, src.Covers(2*time.Minute, matchXid79))
	// A non-matching predicate stays uncovered.
	assert.False(t, src.Covers(2*time.Minute, func(ev HealthEvent) bool { return ev.CheckName == "SysLogsSXIDError" }))
}

func TestSourceRecordCoverageIgnoresHealthyEvents(t *testing.T) {
	socketPath := shortSocketPath(t)

	src, err := New(socketPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = src.Close() })

	// A healthy event carries no new data point: it must never suppress
	// native detection of a later unhealthy incident.
	src.RecordCoverage(HealthEvent{
		CheckName:          "SysLogsXIDError",
		ComponentClass:     "GPU",
		ErrorCodes:         []string{"79"},
		IsHealthy:          true,
		GeneratedTimestamp: time.Now(),
	})

	assert.False(t, src.Covers(2*time.Minute, func(ev HealthEvent) bool {
		return ev.CheckName == "SysLogsXIDError"
	}))
}

func TestSourceRecordCoverageAfterClose(t *testing.T) {
	socketPath := shortSocketPath(t)

	src, err := New(socketPath)
	require.NoError(t, err)
	require.NoError(t, src.Close())

	// Coverage reported after Close is dropped without panic.
	src.RecordCoverage(HealthEvent{
		CheckName:          "SysLogsXIDError",
		ErrorCodes:         []string{"79"},
		GeneratedTimestamp: time.Now(),
	})
	assert.False(t, src.Covers(2*time.Minute, func(ev HealthEvent) bool {
		return ev.CheckName == "SysLogsXIDError"
	}))
}

func TestSourceSubscriberUnsubscribeAndClose(t *testing.T) {
	socketPath := shortSocketPath(t)

	src, err := New(socketPath)
	require.NoError(t, err)

	ch, unsubscribe := src.Subscribe()
	unsubscribe()
	// Unsubscribe detaches the listener without closing the channel (the
	// dispatcher is the only sender and may still hold a reference); the
	// channel is closed when the source is closed. Unsubscribe is safe to
	// call twice.
	select {
	case ev := <-ch:
		t.Fatalf("unexpected event after unsubscribe: %+v", ev)
	case <-time.After(100 * time.Millisecond):
	}
	unsubscribe()

	// An event enqueued after unsubscribe is not delivered to the
	// detached listener; it is held for zero subscribers instead.
	s := src.(*source)
	require.NoError(t, s.enqueue(HealthEvent{CheckName: "held"}))
	select {
	case ev := <-ch:
		t.Fatalf("unsubscribed listener received event: %+v", ev)
	case <-time.After(200 * time.Millisecond):
	}

	ch2, _ := src.Subscribe()
	// The held event is delivered to the new subscriber.
	select {
	case ev := <-ch2:
		assert.Equal(t, "held", ev.CheckName)
	case <-time.After(5 * time.Second):
		t.Fatal("held event was not delivered to the new subscriber")
	}

	require.NoError(t, src.Close())
	_, open := <-ch2
	assert.False(t, open)
}

func TestSourceRecentIndexIsCapped(t *testing.T) {
	socketPath := shortSocketPath(t)

	src, err := New(socketPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = src.Close() })

	s := src.(*source)
	for i := 0; i < maxRecentEvents+100; i++ {
		s.RecordCoverage(HealthEvent{
			CheckName:          "SysLogsXIDError",
			ErrorCodes:         []string{fmt.Sprintf("%d", i)},
			GeneratedTimestamp: time.Now(),
		})
	}

	s.mu.Lock()
	got := len(s.recent)
	s.mu.Unlock()
	assert.Equal(t, maxRecentEvents, got)
}
func TestSourceSubscribeAfterClose(t *testing.T) {
	socketPath := shortSocketPath(t)

	src, err := New(socketPath)
	require.NoError(t, err)
	require.NoError(t, src.Close())

	// Subscribing after Close returns an already-closed channel.
	ch, unsub := src.Subscribe()
	defer unsub()
	_, open := <-ch
	assert.False(t, open)
}

// TestSourceSubscriberChannelFullBlocksDelivery verifies the delivery
// guarantee behind the acknowledged receive queue: a full subscriber
// channel no longer drops the event. The dispatcher waits for the
// subscriber to drain and then delivers.
func TestSourceSubscriberChannelFullBlocksDelivery(t *testing.T) {
	socketPath := shortSocketPath(t)

	src, err := New(socketPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = src.Close() })

	ch, unsubscribe := src.Subscribe()
	defer unsubscribe()

	// Fill the subscriber channel directly so the dispatcher blocks on the
	// next delivery.
	s := src.(*source)
	s.mu.Lock()
	require.Len(t, s.subs, 1)
	sub := s.subs[0]
	s.mu.Unlock()
	for i := 0; i < subscriberBufferSize; i++ {
		sub.ch <- HealthEvent{CheckName: "filler"}
	}

	// Enqueue one more event. The dispatcher accepts it into a subscriber
	// send that cannot complete while the channel is full.
	require.NoError(t, s.enqueue(HealthEvent{CheckName: "blocked"}))

	// The event must not be dropped: nothing new arrives while the
	// subscriber is stuck. The single read below frees one buffer slot,
	// which the blocked dispatcher send then fills.
	select {
	case ev := <-ch:
		assert.Equal(t, "filler", ev.CheckName, "should receive buffered filler while blocked")
	case <-time.After(time.Second):
		t.Fatal("channel should have buffered events")
	}

	// Drain the remaining buffered fillers; the last event out must be
	// the blocked one the dispatcher held onto.
	for i := 0; i < subscriberBufferSize-1; i++ {
		ev := <-ch
		assert.Equal(t, "filler", ev.CheckName)
	}
	select {
	case ev := <-ch:
		assert.Equal(t, "blocked", ev.CheckName)
	case <-time.After(5 * time.Second):
		t.Fatal("dispatcher did not deliver the event after the subscriber drained")
	}
}

// TestSourceQueueFullFailsRPC verifies that a full receive queue fails the
// RPC with Unavailable instead of acknowledging an undeliverable event,
// so the NVSentinel gRPC sink keeps the queue item and retries.
func TestSourceQueueFullFailsRPC(t *testing.T) {
	// Build a source with a tiny queue directly; the socket setup in New
	// is not needed to exercise the handler's enqueue path.
	s := &source{
		queue:    make(chan HealthEvent, 1),
		subs:     make(map[int]*subscription),
		subAdded: make(chan struct{}),
	}
	srv := &platformConnectorServer{src: s}

	// The first event fits the queue and is acknowledged.
	_, err := srv.HealthEventOccurredV1(context.Background(), &datamodels.HealthEvents{
		Version: 1,
		Events:  []*datamodels.HealthEvent{{CheckName: "first"}},
	})
	require.NoError(t, err)

	// The second event does not fit and must fail the RPC as Unavailable.
	_, err = srv.HealthEventOccurredV1(context.Background(), &datamodels.HealthEvents{
		Version: 1,
		Events:  []*datamodels.HealthEvent{{CheckName: "second"}},
	})
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Unavailable, st.Code())
}

// TestSourceHoldsEventUntilFirstSubscriber verifies that events received
// before any subscriber registers are not dropped: they stay in the
// acknowledged receive queue and are delivered once a subscriber appears
// (for example, while components are still starting).
func TestSourceHoldsEventUntilFirstSubscriber(t *testing.T) {
	socketPath := shortSocketPath(t)

	src, err := New(socketPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = src.Close() })

	client := newTestClient(t, socketPath)

	// No subscribers yet: the RPC still succeeds because the event is
	// acknowledged into the receive queue.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err = client.HealthEventOccurredV1(ctx, &datamodels.HealthEvents{
		Version: 1,
		Events: []*datamodels.HealthEvent{{
			CheckName:          "SysLogsXIDError",
			ComponentClass:     "GPU",
			ErrorCode:          []string{"79"},
			GeneratedTimestamp: timestamppb.Now(),
		}},
	})
	require.NoError(t, err)

	// The event is delivered once a subscriber registers.
	events, unsubscribe := src.Subscribe()
	defer unsubscribe()
	select {
	case ev := <-events:
		assert.Equal(t, "SysLogsXIDError", ev.CheckName)
	case <-time.After(5 * time.Second):
		t.Fatal("event held for zero subscribers was not delivered after Subscribe")
	}
}

// TestSourceConcurrentBroadcastUnsubscribeAndClose exercises the subscriber
// lifetime race: the dispatcher must never send on a channel that
// unsubscribe or Close already detached or closed. Run with -race.
func TestSourceConcurrentBroadcastUnsubscribeAndClose(t *testing.T) {
	socketPath := shortSocketPath(t)

	src, err := New(socketPath)
	require.NoError(t, err)

	s := src.(*source)
	done := make(chan struct{})

	// Enqueue events continuously.
	go func() {
		defer close(done)
		for i := 0; i < 1000; i++ {
			_ = s.enqueue(HealthEvent{CheckName: "SysLogsXIDError"})
		}
	}()

	// Subscribe and unsubscribe in a loop while the dispatch is in flight.
	for i := 0; i < 100; i++ {
		_, unsubscribe := src.Subscribe()
		unsubscribe()
	}

	<-done
	// Close races with any residual dispatch; must not panic.
	require.NoError(t, src.Close())
}

func TestSourceCloseIsIdempotent(t *testing.T) {
	socketPath := shortSocketPath(t)

	src, err := New(socketPath)
	require.NoError(t, err)

	require.NoError(t, src.Close())
	// Second Close must not panic or error (grpc.Server.Stop and lis.Close tolerate double-close).
	require.NoError(t, src.Close())
}

func TestSourceRecordUpdatesLastReceived(t *testing.T) {
	socketPath := shortSocketPath(t)

	src, err := New(socketPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = src.Close() })

	s := src.(*source)
	assert.True(t, s.LastReceived().IsZero())

	require.NoError(t, s.enqueue(HealthEvent{CheckName: "test"}))
	assert.False(t, s.LastReceived().IsZero())
}

func TestSourceCoversEmptyRecent(t *testing.T) {
	socketPath := shortSocketPath(t)

	src, err := New(socketPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = src.Close() })

	// No events recorded: Covers always returns false.
	assert.False(t, src.Covers(time.Hour, func(HealthEvent) bool { return true }))
}
func TestNewSocketDirNotCreatable(t *testing.T) {
	// MkdirAll fails when a path component is a regular file.
	blockingFile := filepath.Join(t.TempDir(), "not-a-dir")
	require.NoError(t, os.WriteFile(blockingFile, []byte("x"), 0o644))

	_, err := New(filepath.Join(blockingFile, "sub", "nvsentinel.sock"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create nvsentinel socket directory")
}

func TestNewStaleSocketRemovalError(t *testing.T) {
	// os.Remove fails when the socket path is a non-empty directory.
	dir := t.TempDir()
	socketPath := filepath.Join(dir, "nvsentinel.sock")
	require.NoError(t, os.MkdirAll(socketPath, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(socketPath, "occupant"), []byte("x"), 0o644))

	_, err := New(socketPath)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to remove stale nvsentinel socket")
}

func TestNewListenError(t *testing.T) {
	// Listen fails when the socket directory is not writable.
	dir := t.TempDir()
	socketPath := filepath.Join(dir, "nvsentinel.sock")
	require.NoError(t, os.Chmod(dir, 0o555))
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })

	_, err := New(socketPath)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to listen on nvsentinel socket")
}
