package fuse

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/common"
	"github.com/leptonai/gpud/pkg/eventstore"
	query_config "github.com/leptonai/gpud/pkg/query/config"
	"github.com/leptonai/gpud/pkg/sqlite"

	"github.com/prometheus/client_golang/prometheus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestComponent(t *testing.T) {
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	store, err := eventstore.New(dbRW, dbRO)
	assert.NoError(t, err)

	cfg := Config{
		Query: query_config.Config{
			State: &query_config.State{
				DBRW: dbRW,
				DBRO: dbRO,
			},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	c, err := New(ctx, cfg, store)
	if err != nil {
		t.Fatalf("Failed to create component: %v", err)
	}
	defer c.Close()

	// Test component name
	if c.Name() != "fuse" {
		t.Errorf("Expected component name to be 'fuse', got '%s'", c.Name())
	}

	// Test Start method
	if err := c.Start(); err != nil {
		t.Errorf("Start() returned error: %v", err)
	}

	states, err := c.States(ctx)
	if err != nil {
		t.Fatalf("Failed to get states: %v", err)
	}
	t.Logf("States: %+v", states)

	comp, ok := c.(*component)
	if !ok {
		t.Fatalf("Component is not of type *component")
	}

	now := time.Now().UTC()
	ev := components.Event{
		Time:    metav1.Time{Time: now},
		Name:    "fuse_connections",
		Type:    common.EventTypeCritical,
		Message: "test device",
		ExtraInfo: map[string]string{
			"data":     "{1:2}",
			"encoding": "json",
		},
	}
	if err := comp.eventBucket.Insert(ctx, ev); err != nil {
		t.Fatalf("Failed to insert event: %v", err)
	}

	events, err := comp.Events(ctx, now.Add(-time.Second))
	if err != nil {
		t.Fatalf("Failed to get events: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("Expected 1 event, got %d", len(events))
	}
	if events[0].Name != "fuse_connections" {
		t.Fatalf("Expected event name 'fuse_connections', got '%s'", events[0].Name)
	}

	// Test metrics
	metrics, err := comp.Metrics(ctx, now.Add(-time.Second))
	if err != nil {
		t.Fatalf("Failed to get metrics: %v", err)
	}
	t.Logf("Got %d metrics", len(metrics))

	// Test prometheus registration
	reg := prometheus.NewRegistry()
	if err := comp.RegisterCollectors(reg, dbRW, dbRO, "test_table"); err != nil {
		t.Fatalf("Failed to register collectors: %v", err)
	}

	// Test closing
	if err := c.Close(); err != nil {
		t.Fatalf("Failed to close component: %v", err)
	}

	select {
	case <-comp.ctx.Done():
	case <-time.After(time.Second):
		t.Fatalf("Component context did not close")
	}
}

// TestComponentMetricsWithInvalidTime tests metrics retrieval with invalid time range
func TestComponentMetricsWithInvalidTime(t *testing.T) {
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	store, err := eventstore.New(dbRW, dbRO)
	assert.NoError(t, err)

	cfg := Config{
		Query: query_config.Config{
			State: &query_config.State{
				DBRW: dbRW,
				DBRO: dbRO,
			},
		},
	}

	ctx := context.Background()
	c, err := New(ctx, cfg, store)
	if err != nil {
		t.Fatalf("Failed to create component: %v", err)
	}
	defer c.Close()

	// Test with future time
	futureTime := time.Now().Add(24 * time.Hour)
	metrics, err := c.Metrics(ctx, futureTime)
	if err != nil {
		t.Logf("Expected behavior: got error for future time: %v", err)
	}
	if len(metrics) > 0 {
		t.Errorf("Expected no metrics for future time, got %d", len(metrics))
	}
}
