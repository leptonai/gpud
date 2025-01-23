package db

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/leptonai/gpud/pkg/sqlite"
	"github.com/stretchr/testify/assert"
)

func TestTableInsertsReads(t *testing.T) {
	t.Parallel()

	testTableName := "test_table"

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	db, err := NewDB(ctx, dbRW, dbRO, testTableName)
	if err != nil {
		t.Fatalf("failed to create db: %v", err)
	}

	first := time.Now().UTC()

	events := []Event{}
	eventsN := 10
	for i := 0; i < eventsN; i++ {
		events = append(events, Event{
			Timestamp:    first.Add(time.Duration(i) * time.Second).Unix(),
			DataSource:   "dmesg",
			EventType:    "memory_oom",
			EventDetails: fmt.Sprintf("oom_reaper: reaped process %d (vector), now anon-rss:0kB, file-rss:0kB, shmem-rss:0", i),
		})
	}

	for _, ev := range events {
		if err := db.Insert(ctx, ev); err != nil {
			t.Fatalf("failed to insert event: %v", err)
		}
	}

	events, err = db.Get(ctx, first.Add(-30*time.Second))
	if err != nil {
		t.Fatalf("failed to get events: %v", err)
	}
	if len(events) != eventsN {
		t.Fatalf("expected %d events, got %d", eventsN, len(events))
	}

	// make sure timestamp is in descending order
	for i := 1; i < len(events); i++ {
		if events[i].Timestamp > events[i-1].Timestamp {
			t.Fatalf("expected timestamp to be in descending order, got %d and %d", events[i].Timestamp, events[i-1].Timestamp)
		}
	}

	deleted, err := db.Purge(ctx, first.Add(time.Duration(eventsN*2)*time.Second).Unix())
	if err != nil {
		t.Fatalf("failed to purge events: %v", err)
	}
	if deleted != eventsN {
		t.Fatalf("expected %d events to be deleted, got %d", eventsN, deleted)
	}
}

func TestGetEventsTimeRange(t *testing.T) {
	t.Parallel()

	testTableName := "test_table"

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	db, err := NewDB(ctx, dbRW, dbRO, testTableName)
	if err != nil {
		t.Fatalf("failed to create db: %v", err)
	}

	baseTime := time.Now().UTC()
	events := []Event{
		{
			Timestamp:    baseTime.Add(-10 * time.Minute).Unix(),
			DataSource:   "dmesg",
			EventType:    "memory_oom",
			EventDetails: "old event",
		},
		{
			Timestamp:    baseTime.Add(-5 * time.Minute).Unix(),
			DataSource:   "dmesg",
			EventType:    "memory_oom",
			EventDetails: "mid event",
		},
		{
			Timestamp:    baseTime.Unix(),
			DataSource:   "dmesg",
			EventType:    "memory_oom",
			EventDetails: "recent event",
		},
	}

	for _, ev := range events {
		if err := db.Insert(ctx, ev); err != nil {
			t.Fatalf("failed to insert event: %v", err)
		}
	}

	// Test getting all events
	allEvents, err := db.Get(ctx, baseTime.Add(-15*time.Minute))
	if err != nil {
		t.Fatalf("failed to get all events: %v", err)
	}
	if len(allEvents) != 3 {
		t.Fatalf("expected 3 events, got %d", len(allEvents))
	}

	// Test getting recent events only
	recentEvents, err := db.Get(ctx, baseTime.Add(-2*time.Minute))
	if err != nil {
		t.Fatalf("failed to get recent events: %v", err)
	}
	if len(recentEvents) != 1 {
		t.Fatalf("expected 1 recent event, got %d", len(recentEvents))
	}
	if recentEvents[0].EventDetails != "recent event" {
		t.Fatalf("expected recent event, got %s", recentEvents[0].EventDetails)
	}
}

func TestEmptyResults(t *testing.T) {
	t.Parallel()

	testTableName := "test_table"

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	db, err := NewDB(ctx, dbRW, dbRO, testTableName)
	if err != nil {
		t.Fatalf("failed to create db: %v", err)
	}

	// Test getting events from empty table
	events, err := db.Get(ctx, time.Now().Add(-1*time.Hour))
	if err != nil {
		t.Fatalf("failed to get events from empty table: %v", err)
	}
	if events != nil {
		t.Fatalf("expected nil events from empty table, got %v", events)
	}

	// Test purging empty table
	deleted, err := db.Purge(ctx, time.Now().Unix())
	if err != nil {
		t.Fatalf("failed to purge empty table: %v", err)
	}
	if deleted != 0 {
		t.Fatalf("expected 0 deleted rows from empty table, got %d", deleted)
	}
}

func TestMultipleEventTypes(t *testing.T) {
	t.Parallel()

	testTableName := "test_table"

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	db, err := NewDB(ctx, dbRW, dbRO, testTableName)
	if err != nil {
		t.Fatalf("failed to create db: %v", err)
	}

	baseTime := time.Now().UTC()
	events := []Event{
		{
			Timestamp:    baseTime.Unix(),
			DataSource:   "dmesg",
			EventType:    "memory_oom",
			EventDetails: "oom event",
		},
		{
			Timestamp:    baseTime.Add(1 * time.Second).Unix(),
			DataSource:   "syslog",
			EventType:    "memory_edac_correctable_errors",
			EventDetails: "edac event",
		},
		{
			Timestamp:    baseTime.Add(2 * time.Second).Unix(),
			DataSource:   "dmesg",
			EventType:    "memory_oom_cgroup",
			EventDetails: "cgroup event",
		},
	}

	for _, ev := range events {
		if err := db.Insert(ctx, ev); err != nil {
			t.Fatalf("failed to insert event: %v", err)
		}
	}

	// Get all events
	results, err := db.Get(ctx, baseTime.Add(-1*time.Second))
	if err != nil {
		t.Fatalf("failed to get events: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 events, got %d", len(results))
	}

	// Verify events are in descending order
	if results[0].EventType != "memory_oom_cgroup" ||
		results[1].EventType != "memory_edac_correctable_errors" ||
		results[2].EventType != "memory_oom" {
		t.Fatal("events not in expected order")
	}
}

func TestPurgePartial(t *testing.T) {
	t.Parallel()

	testTableName := "test_table"

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	db, err := NewDB(ctx, dbRW, dbRO, testTableName)
	if err != nil {
		t.Fatalf("failed to create db: %v", err)
	}

	baseTime := time.Now().UTC()
	events := []Event{
		{
			Timestamp:    baseTime.Add(-10 * time.Minute).Unix(),
			DataSource:   "dmesg",
			EventType:    "memory_oom",
			EventDetails: "old event",
		},
		{
			Timestamp:    baseTime.Unix(),
			DataSource:   "dmesg",
			EventType:    "memory_oom",
			EventDetails: "recent event",
		},
	}

	for _, ev := range events {
		if err := db.Insert(ctx, ev); err != nil {
			t.Fatalf("failed to insert event: %v", err)
		}
	}

	// Purge only old events
	deleted, err := db.Purge(ctx, baseTime.Add(-5*time.Minute).Unix())
	if err != nil {
		t.Fatalf("failed to purge old events: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("expected 1 deleted event, got %d", deleted)
	}

	// Verify only recent event remains
	remaining, err := db.Get(ctx, baseTime.Add(-15*time.Minute))
	if err != nil {
		t.Fatalf("failed to get remaining events: %v", err)
	}
	if len(remaining) != 1 {
		t.Fatalf("expected 1 remaining event, got %d", len(remaining))
	}
	if remaining[0].EventDetails != "recent event" {
		t.Fatalf("expected recent event to remain, got %s", remaining[0].EventDetails)
	}
}

func TestFindEvent(t *testing.T) {
	t.Parallel()

	testTableName := "test_table"

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	db, err := NewDB(ctx, dbRW, dbRO, testTableName)
	if err != nil {
		t.Fatalf("failed to create db: %v", err)
	}

	baseTime := time.Now().UTC()
	testEvent := Event{
		Timestamp:    baseTime.Add(-10 * time.Minute).Unix(),
		DataSource:   "dmesg",
		EventType:    "memory_oom",
		EventDetails: "old event",
	}

	// Test finding non-existent event
	found, err := db.Find(ctx, testEvent)
	if err != nil {
		t.Fatalf("failed to find non-existent event: %v", err)
	}
	if found != nil {
		t.Fatal("expected nil for non-existent event")
	}

	// Insert and find the event
	if err := db.Insert(ctx, testEvent); err != nil {
		t.Fatalf("failed to insert event: %v", err)
	}

	found, err = db.Find(ctx, testEvent)
	if err != nil {
		t.Fatalf("failed to find event: %v", err)
	}
	if found == nil {
		t.Fatal("expected to find event but got nil")
	}
	if found.Timestamp != testEvent.Timestamp ||
		found.DataSource != testEvent.DataSource ||
		found.EventType != testEvent.EventType ||
		found.EventDetails != testEvent.EventDetails {
		t.Fatalf("found event doesn't match: got %+v, want %+v", found, testEvent)
	}
}

func TestFindEventPartialMatch(t *testing.T) {
	t.Parallel()

	testTableName := "test_table"

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	db, err := NewDB(ctx, dbRW, dbRO, testTableName)
	if err != nil {
		t.Fatalf("failed to create db: %v", err)
	}

	baseTime := time.Now().UTC()
	testEvent := Event{
		Timestamp:    baseTime.Unix(),
		DataSource:   "dmesg",
		EventType:    "memory_oom",
		EventDetails: "original details",
	}

	if err := db.Insert(ctx, testEvent); err != nil {
		t.Fatalf("failed to insert event: %v", err)
	}

	// Test finding with matching timestamp/source/type but different details
	searchEvent := Event{
		Timestamp:    testEvent.Timestamp,
		DataSource:   testEvent.DataSource,
		EventType:    testEvent.EventType,
		EventDetails: "different details",
	}

	found, err := db.Find(ctx, searchEvent)
	if err != nil {
		t.Fatalf("failed to find event: %v", err)
	}
	if found == nil {
		t.Fatal("expected to find event but got nil")
	}
	if found.EventDetails != testEvent.EventDetails {
		t.Fatalf("expected original details %q, got %q", testEvent.EventDetails, found.EventDetails)
	}
}

func TestFindEventMultipleMatches(t *testing.T) {
	t.Parallel()

	testTableName := "test_table"

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	db, err := NewDB(ctx, dbRW, dbRO, testTableName)
	if err != nil {
		t.Fatalf("failed to create db: %v", err)
	}

	baseTime := time.Now().UTC()
	events := []Event{
		{
			Timestamp:    baseTime.Unix(),
			DataSource:   "dmesg",
			EventType:    "memory_oom",
			EventDetails: "first event",
		},
		{
			Timestamp:    baseTime.Unix(),
			DataSource:   "dmesg",
			EventType:    "memory_oom",
			EventDetails: "second event",
		},
	}

	// Insert multiple events with same timestamp/source/type
	for _, ev := range events {
		if err := db.Insert(ctx, ev); err != nil {
			t.Fatalf("failed to insert event: %v", err)
		}
	}

	// Search should return the first matching event
	searchEvent := Event{
		Timestamp:  baseTime.Unix(),
		DataSource: "dmesg",
		EventType:  "memory_oom",
	}

	found, err := db.Find(ctx, searchEvent)
	if err != nil {
		t.Fatalf("failed to find event: %v", err)
	}
	if found == nil {
		t.Fatal("expected to find event but got nil")
	}
	// Should match one of the events
	foundMatch := false
	for _, ev := range events {
		if found.EventDetails == ev.EventDetails {
			foundMatch = true
			break
		}
	}
	if !foundMatch {
		t.Fatalf("found event details %q doesn't match any expected events", found.EventDetails)
	}
}

func TestEventWithIDs(t *testing.T) {
	t.Parallel()

	testTableName := "test_table"

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	db, err := NewDB(ctx, dbRW, dbRO, testTableName)
	assert.NoError(t, err)

	baseTime := time.Now().UTC()
	event := Event{
		Timestamp:    baseTime.Unix(),
		DataSource:   "nvidia-smi",
		EventType:    "gpu_error",
		EventID1:     "xid",
		EventID2:     "gpu-123",
		EventDetails: "GPU error details",
	}

	// Test insert and find with IDs
	err = db.Insert(ctx, event)
	assert.NoError(t, err)

	found, err := db.Find(ctx, event)
	assert.NoError(t, err)
	assert.NotNil(t, found)
	assert.Equal(t, event.EventID1, found.EventID1)
	assert.Equal(t, event.EventID2, found.EventID2)

	// Test find with partial ID match
	partialEvent := Event{
		Timestamp:  event.Timestamp,
		DataSource: event.DataSource,
		EventType:  event.EventType,
		EventID1:   event.EventID1,
	}

	found, err = db.Find(ctx, partialEvent)
	assert.NoError(t, err)
	assert.NotNil(t, found)
	assert.Equal(t, event.EventID2, found.EventID2)

	// Test find with different ID
	differentEvent := Event{
		Timestamp:  event.Timestamp,
		DataSource: event.DataSource,
		EventType:  event.EventType,
		EventID1:   "different-xid",
		EventID2:   "different-gpu",
	}

	found, err = db.Find(ctx, differentEvent)
	assert.NoError(t, err)
	assert.Nil(t, found, "Should not find event with different IDs")
}

func TestNullEventIDs(t *testing.T) {
	t.Parallel()

	testTableName := "test_table"

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	db, err := NewDB(ctx, dbRW, dbRO, testTableName)
	assert.NoError(t, err)

	baseTime := time.Now().UTC()
	event := Event{
		Timestamp:    baseTime.Unix(),
		DataSource:   "dmesg",
		EventType:    "system_event",
		EventID1:     "",
		EventID2:     "",
		EventDetails: "Event with null IDs",
	}

	// Test insert and find with null IDs
	err = db.Insert(ctx, event)
	assert.NoError(t, err)

	found, err := db.Find(ctx, event)
	assert.NoError(t, err)
	assert.NotNil(t, found)
	assert.Empty(t, found.EventID1)
	assert.Empty(t, found.EventID2)
}

func TestInvalidTableName(t *testing.T) {
	t.Parallel()

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	// Test with invalid table name
	_, err := NewDB(ctx, dbRW, dbRO, "invalid;table;name")
	assert.Error(t, err)
}

func TestContextCancellation(t *testing.T) {
	t.Parallel()

	testTableName := "test_table"

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	db, err := NewDB(ctx, dbRW, dbRO, testTableName)
	assert.NoError(t, err)

	// Test with canceled context
	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	event := Event{
		Timestamp:    time.Now().UTC().Unix(),
		DataSource:   "test",
		EventType:    "test_event",
		EventDetails: "Test details",
	}

	err = db.Insert(canceledCtx, event)
	assert.Error(t, err)

	_, err = db.Find(canceledCtx, event)
	assert.Error(t, err)

	_, err = db.Get(canceledCtx, time.Now().Add(-1*time.Hour))
	assert.Error(t, err)
}

func TestConcurrentAccess(t *testing.T) {
	t.Parallel()

	testTableName := "test_table"

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	db, err := NewDB(ctx, dbRW, dbRO, testTableName)
	assert.NoError(t, err)

	baseTime := time.Now().UTC()
	eventCount := 100
	done := make(chan bool)

	// Concurrent inserts
	go func() {
		for i := 0; i < eventCount; i++ {
			event := Event{
				Timestamp:    baseTime.Add(time.Duration(i) * time.Second).Unix(),
				DataSource:   "concurrent",
				EventType:    "test_event",
				EventDetails: fmt.Sprintf("Concurrent event %d", i),
			}
			assert.NoError(t, db.Insert(ctx, event))
		}
		done <- true
	}()

	// Concurrent reads
	go func() {
		for i := 0; i < eventCount; i++ {
			_, err := db.Get(ctx, baseTime.Add(-1*time.Hour))
			assert.NoError(t, err)
		}
		done <- true
	}()

	// Wait for both goroutines to complete
	<-done
	<-done

	// Verify final count
	events, err := db.Get(ctx, baseTime.Add(-1*time.Hour))
	assert.NoError(t, err)
	assert.Equal(t, eventCount, len(events))
}

func TestSpecialCharactersInEvents(t *testing.T) {
	t.Parallel()

	testTableName := "test_table"

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	db, err := NewDB(ctx, dbRW, dbRO, testTableName)
	assert.NoError(t, err)

	events := []Event{
		{
			Timestamp:    time.Now().UTC().Unix(),
			DataSource:   "test;source",
			EventType:    "test'type",
			EventID1:     "id\"1",
			EventID2:     "id`2",
			EventDetails: "details with special chars: !@#$%^&*()",
		},
		{
			Timestamp:    time.Now().UTC().Unix(),
			DataSource:   "unicode_source_🔥",
			EventType:    "unicode_type_⚡",
			EventID1:     "unicode_id1_💾",
			EventID2:     "unicode_id2_🚀",
			EventDetails: "unicode details: 你好，世界！",
		},
	}

	// Test insert and retrieval of events with special characters
	for _, event := range events {
		err = db.Insert(ctx, event)
		assert.NoError(t, err)

		found, err := db.Find(ctx, event)
		assert.NoError(t, err)
		assert.NotNil(t, found)
		assert.Equal(t, event.DataSource, found.DataSource)
		assert.Equal(t, event.EventType, found.EventType)
		assert.Equal(t, event.EventID1, found.EventID1)
		assert.Equal(t, event.EventID2, found.EventID2)
		assert.Equal(t, event.EventDetails, found.EventDetails)
	}
}

func TestLargeEventDetails(t *testing.T) {
	t.Parallel()

	testTableName := "test_table"

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	db, err := NewDB(ctx, dbRW, dbRO, testTableName)
	assert.NoError(t, err)

	// Create a large event detail string (100KB)
	largeDetail := make([]byte, 100*1024)
	for i := range largeDetail {
		largeDetail[i] = byte('a' + (i % 26))
	}

	event := Event{
		Timestamp:    time.Now().UTC().Unix(),
		DataSource:   "test",
		EventType:    "large_event",
		EventDetails: string(largeDetail),
	}

	err = db.Insert(ctx, event)
	assert.NoError(t, err)

	found, err := db.Find(ctx, event)
	assert.NoError(t, err)
	assert.NotNil(t, found)
	assert.Equal(t, event.EventDetails, found.EventDetails)
}

func TestTimestampBoundaries(t *testing.T) {
	t.Parallel()

	testTableName := "test_table"

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	db, err := NewDB(ctx, dbRW, dbRO, testTableName)
	assert.NoError(t, err)

	timestamps := []int64{
		0,                  // Unix epoch
		-1,                 // Before Unix epoch
		1 << 32,            // Large timestamp
		-(1 << 31),         // Large negative timestamp
		time.Now().Unix(),  // Current time
		1 << 62,            // Very large timestamp
		-((1 << 62) + 100), // Very large negative timestamp
	}

	for _, ts := range timestamps {
		event := Event{
			Timestamp:    ts,
			DataSource:   "test",
			EventType:    "timestamp_test",
			EventDetails: fmt.Sprintf("timestamp: %d", ts),
		}

		err = db.Insert(ctx, event)
		assert.NoError(t, err)

		found, err := db.Find(ctx, event)
		assert.NoError(t, err)
		assert.NotNil(t, found)
		assert.Equal(t, ts, found.Timestamp)
	}

	// Test retrieval with various time ranges
	events, err := db.Get(ctx, time.Unix(-(1<<63), 0)) // Get all events
	assert.NoError(t, err)
	assert.Equal(t, len(timestamps), len(events))

	events, err = db.Get(ctx, time.Unix(1<<63-1, 0)) // Future time
	assert.NoError(t, err)
	assert.Nil(t, events)
}

func TestConcurrentWritesWithDifferentIDs(t *testing.T) {
	t.Parallel()

	testTableName := "test_table"

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	db, err := NewDB(ctx, dbRW, dbRO, testTableName)
	assert.NoError(t, err)

	baseTime := time.Now().UTC()
	eventCount := 100
	done := make(chan bool)

	// Concurrent inserts with different event IDs
	go func() {
		for i := 0; i < eventCount; i++ {
			event := Event{
				Timestamp:    baseTime.Add(time.Duration(i) * time.Second).Unix(),
				DataSource:   "concurrent",
				EventType:    "test_event",
				EventID1:     fmt.Sprintf("id1_%d", i),
				EventID2:     fmt.Sprintf("id2_%d", i),
				EventDetails: fmt.Sprintf("Concurrent event %d", i),
			}
			assert.NoError(t, db.Insert(ctx, event))
		}
		done <- true
	}()

	// Concurrent reads with event ID filtering
	go func() {
		for i := 0; i < eventCount; i++ {
			event := Event{
				Timestamp:  baseTime.Add(time.Duration(i) * time.Second).Unix(),
				DataSource: "concurrent",
				EventType:  "test_event",
				EventID1:   fmt.Sprintf("id1_%d", i),
				EventID2:   fmt.Sprintf("id2_%d", i),
			}
			found, err := db.Find(ctx, event)
			if err == nil && found != nil {
				assert.Equal(t, event.EventID1, found.EventID1)
				assert.Equal(t, event.EventID2, found.EventID2)
			}
		}
		done <- true
	}()

	// Wait for both goroutines to complete
	<-done
	<-done

	// Verify all events were inserted with correct IDs
	events, err := db.Get(ctx, baseTime.Add(-1*time.Hour))
	assert.NoError(t, err)
	assert.Equal(t, eventCount, len(events))

	// Verify each event has unique IDs
	idMap := make(map[string]bool)
	for _, event := range events {
		id := event.EventID1 + ":" + event.EventID2
		assert.False(t, idMap[id], "Duplicate event IDs found")
		idMap[id] = true
	}
}

func TestPurgeWithEventIDs(t *testing.T) {
	t.Parallel()

	testTableName := "test_table"

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	db, err := NewDB(ctx, dbRW, dbRO, testTableName)
	assert.NoError(t, err)

	baseTime := time.Now().UTC()
	events := []Event{
		{
			Timestamp:    baseTime.Add(-10 * time.Minute).Unix(),
			DataSource:   "test",
			EventType:    "event_type",
			EventID1:     "old_id1",
			EventID2:     "old_id2",
			EventDetails: "old event",
		},
		{
			Timestamp:    baseTime.Unix(),
			DataSource:   "test",
			EventType:    "event_type",
			EventID1:     "new_id1",
			EventID2:     "new_id2",
			EventDetails: "new event",
		},
	}

	for _, event := range events {
		err = db.Insert(ctx, event)
		assert.NoError(t, err)
	}

	// Purge old events
	deleted, err := db.Purge(ctx, baseTime.Add(-5*time.Minute).Unix())
	assert.NoError(t, err)
	assert.Equal(t, 1, deleted)

	// Verify only new event remains with correct IDs
	remaining, err := db.Get(ctx, baseTime.Add(-15*time.Minute))
	assert.NoError(t, err)
	assert.Equal(t, 1, len(remaining))
	assert.Equal(t, "new_id1", remaining[0].EventID1)
	assert.Equal(t, "new_id2", remaining[0].EventID2)

	// Try to find old event by IDs
	oldEvent := Event{
		Timestamp:  baseTime.Add(-10 * time.Minute).Unix(),
		DataSource: "test",
		EventType:  "event_type",
		EventID1:   "old_id1",
		EventID2:   "old_id2",
	}
	found, err := db.Find(ctx, oldEvent)
	assert.NoError(t, err)
	assert.Nil(t, found, "Old event should not be found after purge")
}
