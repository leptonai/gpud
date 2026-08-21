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
	"google.golang.org/grpc/credentials/insecure"
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

	assert.True(t, src.Covers(2*time.Minute, matchXid79))
	// A non-matching predicate stays uncovered.
	assert.False(t, src.Covers(2*time.Minute, func(ev HealthEvent) bool { return ev.CheckName == "SysLogsSXIDError" }))
}

func TestSourceSubscriberUnsubscribeAndClose(t *testing.T) {
	socketPath := shortSocketPath(t)

	src, err := New(socketPath)
	require.NoError(t, err)

	ch, unsubscribe := src.Subscribe()
	unsubscribe()
	// Unsubscribe closes the channel and is safe to call twice.
	_, open := <-ch
	assert.False(t, open)
	unsubscribe()

	ch2, _ := src.Subscribe()
	require.NoError(t, src.Close())
	_, open = <-ch2
	assert.False(t, open)
}

func TestSourceRecentIndexIsCapped(t *testing.T) {
	socketPath := shortSocketPath(t)

	src, err := New(socketPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = src.Close() })

	s := src.(*source)
	for i := 0; i < maxRecentEvents+100; i++ {
		s.record(HealthEvent{
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
