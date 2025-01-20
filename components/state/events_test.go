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

func TestPurgeEvents(t *testing.T) {
	t.Parallel()

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := CreateEventsTable(ctx, dbRW); err != nil {
		t.Fatalf("failed to create table: %v", err)
	}

	now := time.Now()
	events := []Event{
		{
			Timestamp:    now.Add(-2 * time.Hour).Unix(),
			EventType:    "old_event",
			DataSource:   "test_source",
			EventDetails: "old details",
		},
		{
			Timestamp:    now.Add(-1 * time.Hour).Unix(),
			EventType:    "recent_event",
			DataSource:   "test_source",
			EventDetails: "recent details",
		},
		{
			Timestamp:    now.Unix(),
			EventType:    "current_event",
			DataSource:   "test_source",
			EventDetails: "current details",
		},
	}

	for _, event := range events {
		if err := InsertEvent(ctx, dbRW, event); err != nil {
			t.Fatalf("failed to insert event: %v", err)
		}
	}

	// Purge events older than 90 minutes
	purgeTime := now.Add(-90 * time.Minute)
	purged, err := PurgeEvents(ctx, dbRW, WithBefore(purgeTime))
	if err != nil {
		t.Fatalf("failed to purge events: %v", err)
	}
	if purged != 1 {
		t.Fatalf("expected to purge 1 event, purged %d", purged)
	}

	// Verify remaining events
	remaining, err := ReadEvents(ctx, dbRO)
	if err != nil {
		t.Fatalf("failed to read events: %v", err)
	}
	if len(remaining) != 2 {
		t.Fatalf("expected 2 events after purge, got %d", len(remaining))
	}
}

func TestReadEventsWithFilters(t *testing.T) {
	t.Parallel()

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := CreateEventsTable(ctx, dbRW); err != nil {
		t.Fatalf("failed to create table: %v", err)
	}

	now := time.Now()
	events := []Event{
		{
			Timestamp:    now.Unix(),
			EventType:    "error",
			DataSource:   "nvml",
			Target:       "gpu-1",
			EventDetails: "error details",
		},
		{
			Timestamp:    now.Unix(),
			EventType:    "warning",
			DataSource:   "nvml",
			Target:       "gpu-2",
			EventDetails: "warning details",
		},
		{
			Timestamp:    now.Unix(),
			EventType:    "error",
			DataSource:   "dmesg",
			Target:       "gpu-1",
			EventDetails: "dmesg error",
		},
	}

	for _, event := range events {
		if err := InsertEvent(ctx, dbRW, event); err != nil {
			t.Fatalf("failed to insert event: %v", err)
		}
	}

	// Test filtering by event type
	errorEvents, err := ReadEvents(ctx, dbRO, WithEventType("error"))
	if err != nil {
		t.Fatalf("failed to read error events: %v", err)
	}
	if len(errorEvents) != 2 {
		t.Fatalf("expected 2 error events, got %d", len(errorEvents))
	}

	// Test filtering by data source
	nvmlEvents, err := ReadEvents(ctx, dbRO, WithDataSource("nvml"))
	if err != nil {
		t.Fatalf("failed to read nvml events: %v", err)
	}
	if len(nvmlEvents) != 2 {
		t.Fatalf("expected 2 nvml events, got %d", len(nvmlEvents))
	}

	// Test filtering by target
	gpu1Events, err := ReadEvents(ctx, dbRO, WithTarget("gpu-1"))
	if err != nil {
		t.Fatalf("failed to read gpu-1 events: %v", err)
	}
	if len(gpu1Events) != 2 {
		t.Fatalf("expected 2 gpu-1 events, got %d", len(gpu1Events))
	}

	// Test with limit
	limitedEvents, err := ReadEvents(ctx, dbRO, WithLimit(1))
	if err != nil {
		t.Fatalf("failed to read limited events: %v", err)
	}
	if len(limitedEvents) != 1 {
		t.Fatalf("expected 1 event with limit, got %d", len(limitedEvents))
	}
}

func TestFindDuplicateEvents(t *testing.T) {
	t.Parallel()

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := CreateEventsTable(ctx, dbRW); err != nil {
		t.Fatalf("failed to create table: %v", err)
	}

	now := time.Now()
	event := Event{
		Timestamp:    now.Unix(),
		EventType:    "test_event",
		DataSource:   "test_source",
		Target:       "test_target",
		EventDetails: "test details",
	}

	// Insert first event
	if err := InsertEvent(ctx, dbRW, event); err != nil {
		t.Fatalf("failed to insert first event: %v", err)
	}

	// Try to find the same event
	found, err := FindEvent(ctx, dbRO, event)
	if err != nil {
		t.Fatalf("failed to find event: %v", err)
	}
	if !found {
		t.Fatal("expected to find the event")
	}

	// Try to find event with different details
	differentEvent := event
	differentEvent.EventDetails = "different details"
	found, err = FindEvent(ctx, dbRO, differentEvent)
	if err != nil {
		t.Fatalf("failed to find event: %v", err)
	}
	if found {
		t.Fatal("expected not to find event with different details")
	}
}

func TestReadEventsEmptyResult(t *testing.T) {
	t.Parallel()

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := CreateEventsTable(ctx, dbRW); err != nil {
		t.Fatalf("failed to create table: %v", err)
	}

	// Try to read from empty table
	events, err := ReadEvents(ctx, dbRO)
	if err != nil {
		t.Fatalf("failed to read events: %v", err)
	}
	if events != nil {
		t.Fatal("expected nil events from empty table")
	}

	// Try to read with non-matching filter
	events, err = ReadEvents(ctx, dbRO, WithEventType("non_existent"))
	if err != nil {
		t.Fatalf("failed to read events with filter: %v", err)
	}
	if events != nil {
		t.Fatal("expected nil events with non-matching filter")
	}
}
