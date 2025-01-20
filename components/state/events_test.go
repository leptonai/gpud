package state

import (
	"context"
	"testing"
	"time"

	"github.com/leptonai/gpud/pkg/sqlite"
)

func TestReadEvents(t *testing.T) {
	t.Parallel()

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := CreateEventsTable(ctx, dbRW); err != nil {
		t.Fatalf("failed to create table: %v", err)
	}

	now := time.Now()
	events := []Event{}
	for i := 0; i < 10; i++ {
		events = append(events, Event{
			Timestamp:    now.Add(time.Duration(i) * time.Second).Unix(),
			EventType:    "test_event",
			DataSource:   "test_data_source",
			Target:       "test_target",
			EventDetails: "test_event_details",
		})
	}

	for _, event := range events {
		found, err := FindEvent(ctx, dbRO, event)
		if err != nil {
			t.Fatalf("failed to find event: %v", err)
		}
		if found {
			t.Fatalf("expected event not found")
		}
		if err := InsertEvent(ctx, dbRW, event); err != nil {
			t.Fatalf("failed to insert event: %v", err)
		}
	}

	readEvents, err := ReadEvents(ctx, dbRO, WithEventType("test_event"), WithDataSource("test_data_source"), WithTarget("test_target"), WithSortTimestampAscendingOrder())
	if err != nil {
		t.Fatalf("failed to read events: %v", err)
	}

	if len(readEvents) != len(events) {
		t.Fatalf("expected %d events, got %d", len(events), len(readEvents))
	}

	for i, event := range events {
		if readEvents[i] != event {
			t.Fatalf("expected event %v, got %v", event, readEvents[i])
		}
	}
}
