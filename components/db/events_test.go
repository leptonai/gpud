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

	store, err := NewStore(ctx, dbRW, dbRO, testTableName)
	assert.NoError(t, err)

	first := time.Now().UTC()

	events := []Event{}
	eventsN := 10
	for i := 0; i < eventsN; i++ {
		events = append(events, Event{
			Timestamp:        first.Add(time.Duration(i) * time.Second).Unix(),
			Name:             "dmesg",
			Type:             "memory_oom",
			Message:          fmt.Sprintf("OOM event %d occurred", i),
			SuggestedActions: fmt.Sprintf("oom_reaper: reaped process %d (vector), now anon-rss:0kB, file-rss:0kB, shmem-rss:0", i),
		})
	}

	for _, ev := range events {
		assert.NoError(t, store.Insert(ctx, ev))
	}

	events, err = store.Get(ctx, first.Add(-30*time.Second))
	assert.NoError(t, err)
	assert.Equal(t, eventsN, len(events))

	// make sure timestamp is in descending order
	for i := 1; i < len(events); i++ {
		assert.Greater(t, events[i-1].Timestamp, events[i].Timestamp, "timestamps should be in descending order")
		// Since events are returned in descending order (newest first),
		// the message index should be eventsN - (i + 1) for the current event
		expectedMsg := fmt.Sprintf("OOM event %d occurred", eventsN-(i+1))
		assert.Equal(t, expectedMsg, events[i].Message, "messages should match in descending order")
	}

	deleted, err := store.Purge(ctx, first.Add(time.Duration(eventsN*2)*time.Second).Unix())
	assert.NoError(t, err)
	assert.Equal(t, eventsN, deleted)
}

func TestGetEventsTimeRange(t *testing.T) {
	t.Parallel()

	testTableName := "test_table"

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	db, err := NewStore(ctx, dbRW, dbRO, testTableName)
	assert.NoError(t, err)

	baseTime := time.Now().UTC()
	events := []Event{
		{
			Timestamp:        baseTime.Add(-10 * time.Minute).Unix(),
			Name:             "dmesg",
			Type:             "memory_oom",
			SuggestedActions: "old event",
		},
		{
			Timestamp:        baseTime.Add(-5 * time.Minute).Unix(),
			Name:             "dmesg",
			Type:             "memory_oom",
			SuggestedActions: "mid event",
		},
		{
			Timestamp:        baseTime.Unix(),
			Name:             "dmesg",
			Type:             "memory_oom",
			SuggestedActions: "recent event",
		},
	}

	for _, ev := range events {
		assert.NoError(t, db.Insert(ctx, ev))
	}

	// Test getting all events
	allEvents, err := db.Get(ctx, baseTime.Add(-15*time.Minute))
	assert.NoError(t, err)
	assert.Equal(t, 3, len(allEvents))

	// Test getting recent events only
	recentEvents, err := db.Get(ctx, baseTime.Add(-2*time.Minute))
	assert.NoError(t, err)
	assert.Equal(t, 1, len(recentEvents))
	assert.Equal(t, "recent event", recentEvents[0].SuggestedActions)
}

func TestEmptyResults(t *testing.T) {
	t.Parallel()

	testTableName := "test_table"

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	store, err := NewStore(ctx, dbRW, dbRO, testTableName)
	assert.NoError(t, err)

	// Test getting events from empty table
	events, err := store.Get(ctx, time.Now().Add(-1*time.Hour))
	assert.NoError(t, err)
	assert.Nil(t, events)

	// Test purging empty table
	deleted, err := store.Purge(ctx, time.Now().Unix())
	assert.NoError(t, err)
	assert.Equal(t, 0, deleted)
}

func TestMultipleEventTypes(t *testing.T) {
	t.Parallel()

	testTableName := "test_table"

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	store, err := NewStore(ctx, dbRW, dbRO, testTableName)
	assert.NoError(t, err)

	baseTime := time.Now().UTC()
	events := []Event{
		{
			Timestamp:        baseTime.Unix(),
			Name:             "dmesg",
			Type:             "memory_oom",
			SuggestedActions: "oom event",
		},
		{
			Timestamp:        baseTime.Add(1 * time.Second).Unix(),
			Name:             "syslog",
			Type:             "memory_edac_correctable_errors",
			SuggestedActions: "edac event",
		},
		{
			Timestamp:        baseTime.Add(2 * time.Second).Unix(),
			Name:             "dmesg",
			Type:             "memory_oom_cgroup",
			SuggestedActions: "cgroup event",
		},
	}

	for _, ev := range events {
		assert.NoError(t, store.Insert(ctx, ev))
	}

	// Get all events
	results, err := store.Get(ctx, baseTime.Add(-1*time.Second))
	assert.NoError(t, err)
	assert.Equal(t, 3, len(results))

	// Verify events are in descending order
	assert.Equal(t, "memory_oom_cgroup", results[0].Type)
	assert.Equal(t, "memory_edac_correctable_errors", results[1].Type)
	assert.Equal(t, "memory_oom", results[2].Type)
}

func TestPurgePartial(t *testing.T) {
	t.Parallel()

	testTableName := "test_table"

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	store, err := NewStore(ctx, dbRW, dbRO, testTableName)
	assert.NoError(t, err)

	baseTime := time.Now().UTC()
	events := []Event{
		{
			Timestamp:        baseTime.Add(-10 * time.Minute).Unix(),
			Name:             "dmesg",
			Type:             "memory_oom",
			ExtraInfo:        `{"a":"b"}`,
			SuggestedActions: "old event",
		},
		{
			Timestamp:        baseTime.Unix(),
			Name:             "dmesg",
			Type:             "memory_oom",
			ExtraInfo:        `{"a":"b"}`,
			SuggestedActions: "recent event",
		},
	}

	for _, ev := range events {
		assert.NoError(t, store.Insert(ctx, ev))
	}

	// Purge only old events
	deleted, err := store.Purge(ctx, baseTime.Add(-5*time.Minute).Unix())
	assert.NoError(t, err)
	assert.Equal(t, 1, deleted)

	// Verify only recent event remains
	remaining, err := store.Get(ctx, baseTime.Add(-15*time.Minute))
	assert.NoError(t, err)
	assert.Equal(t, 1, len(remaining))
	assert.Equal(t, "recent event", remaining[0].SuggestedActions)
}

func TestFindEvent(t *testing.T) {
	t.Parallel()

	testTableName := "test_table"

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	store, err := NewStore(ctx, dbRW, dbRO, testTableName)
	assert.NoError(t, err)

	baseTime := time.Now().UTC()
	testEvent := Event{
		Timestamp:        baseTime.Add(-10 * time.Minute).Unix(),
		Name:             "dmesg",
		Type:             "memory_oom",
		ExtraInfo:        `{"a":"b"}`,
		SuggestedActions: "old event",
	}

	// Test finding non-existent event
	found, err := store.Find(ctx, testEvent)
	assert.NoError(t, err)
	assert.Nil(t, found)

	// Insert and find the event
	assert.NoError(t, store.Insert(ctx, testEvent))

	found, err = store.Find(ctx, testEvent)
	assert.NoError(t, err)
	assert.NotNil(t, found)
	assert.Equal(t, testEvent.Timestamp, found.Timestamp)
	assert.Equal(t, testEvent.Name, found.Name)
	assert.Equal(t, testEvent.Type, found.Type)
	assert.Equal(t, testEvent.ExtraInfo, found.ExtraInfo)
	assert.Equal(t, testEvent.SuggestedActions, found.SuggestedActions)
}

func TestFindEventPartialMatch(t *testing.T) {
	t.Parallel()

	testTableName := "test_table"

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	store, err := NewStore(ctx, dbRW, dbRO, testTableName)
	assert.NoError(t, err)

	baseTime := time.Now().UTC()
	testEvent := Event{
		Timestamp:        baseTime.Unix(),
		Name:             "dmesg",
		Type:             "memory_oom",
		ExtraInfo:        `{"a":"b"}`,
		SuggestedActions: "original details",
	}

	assert.NoError(t, store.Insert(ctx, testEvent))

	// Test finding with matching timestamp/source/type but different details
	searchEvent := Event{
		Timestamp:        testEvent.Timestamp,
		Name:             testEvent.Name,
		Type:             testEvent.Type,
		ExtraInfo:        testEvent.ExtraInfo,
		SuggestedActions: "different details",
	}

	found, err := store.Find(ctx, searchEvent)
	assert.NoError(t, err)
	assert.Nil(t, found)
}

func TestFindEventMultipleMatches(t *testing.T) {
	t.Parallel()

	testTableName := "test_table"

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	store, err := NewStore(ctx, dbRW, dbRO, testTableName)
	assert.NoError(t, err)

	baseTime := time.Now().UTC()
	events := []Event{
		{
			Timestamp:        baseTime.Unix(),
			Name:             "dmesg",
			Type:             "memory_oom",
			ExtraInfo:        `{"a":"b"}`,
			SuggestedActions: "first event",
		},
		{
			Timestamp:        baseTime.Unix(),
			Name:             "dmesg",
			Type:             "memory_oom",
			ExtraInfo:        `{"a":"b"}`,
			SuggestedActions: "second event",
		},
	}

	// Insert multiple events with same timestamp/source/type
	for _, ev := range events {
		assert.NoError(t, store.Insert(ctx, ev))
	}

	// Search should return the first matching event
	searchEvent := Event{
		Timestamp: baseTime.Unix(),
		Name:      "dmesg",
		Type:      "memory_oom",
		ExtraInfo: `{"a":"b"}`,
	}

	found, err := store.Find(ctx, searchEvent)
	assert.NoError(t, err)
	assert.NotNil(t, found)

	// Should match one of the events
	foundMatch := false
	for _, ev := range events {
		if found.SuggestedActions == ev.SuggestedActions {
			foundMatch = true
			break
		}
	}
	assert.True(t, foundMatch, "Found event should match one of the inserted events")
}

func TestEventWithIDs(t *testing.T) {
	t.Parallel()

	testTableName := "test_table"

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	store, err := NewStore(ctx, dbRW, dbRO, testTableName)
	assert.NoError(t, err)

	baseTime := time.Now().UTC()
	event := Event{
		Timestamp:        baseTime.Unix(),
		Name:             "nvidia-smi",
		Type:             "gpu_error",
		ExtraInfo:        `{"xid": "123", "gpu_uuid": "gpu-123"}`,
		SuggestedActions: "GPU error details",
	}

	// Test insert and find with ExtraInfo
	err = store.Insert(ctx, event)
	assert.NoError(t, err)

	found, err := store.Find(ctx, event)
	assert.NoError(t, err)
	assert.NotNil(t, found)
	assert.Equal(t, event.ExtraInfo, found.ExtraInfo)

	// Test find with partial ExtraInfo match
	partialEvent := Event{
		Timestamp: event.Timestamp,
		Name:      event.Name,
		Type:      event.Type,
	}

	found, err = store.Find(ctx, partialEvent)
	assert.NoError(t, err)
	assert.NotNil(t, found)
	assert.Equal(t, event.ExtraInfo, found.ExtraInfo)

	// Test find with different ExtraInfo
	differentEvent := Event{
		Timestamp: event.Timestamp,
		Name:      event.Name,
		Type:      event.Type,
		ExtraInfo: `{"xid": "different", "gpu_uuid": "different-gpu"}`,
	}

	found, err = store.Find(ctx, differentEvent)
	assert.NoError(t, err)
	assert.Nil(t, found, "Should not find event with different ExtraInfo")
}

func TestNullEventIDs(t *testing.T) {
	t.Parallel()

	testTableName := "test_table"

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	store, err := NewStore(ctx, dbRW, dbRO, testTableName)
	assert.NoError(t, err)

	baseTime := time.Now().UTC()
	event := Event{
		Timestamp:        baseTime.Unix(),
		Name:             "dmesg",
		Type:             "system_event",
		ExtraInfo:        "",
		SuggestedActions: "Event with null ExtraInfo",
	}

	// Test insert and find with null ExtraInfo
	err = store.Insert(ctx, event)
	assert.NoError(t, err)

	found, err := store.Find(ctx, event)
	assert.NoError(t, err)
	assert.NotNil(t, found)
	assert.Equal(t, len(found.ExtraInfo), 0)
}

func TestPurgeWithEventIDs(t *testing.T) {
	t.Parallel()

	testTableName := "test_table"

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	db, err := NewStore(ctx, dbRW, dbRO, testTableName)
	assert.NoError(t, err)

	baseTime := time.Now().UTC()
	events := []Event{
		{
			Timestamp:        baseTime.Add(-10 * time.Minute).Unix(),
			Name:             "test",
			Type:             "event_type",
			ExtraInfo:        `{"id": "old_event"}`,
			SuggestedActions: "old event",
		},
		{
			Timestamp:        baseTime.Unix(),
			Name:             "test",
			Type:             "event_type",
			ExtraInfo:        `{"id": "new_event"}`,
			SuggestedActions: "new event",
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

	// Verify only new event remains with correct ExtraInfo
	remaining, err := db.Get(ctx, baseTime.Add(-15*time.Minute))
	assert.NoError(t, err)
	assert.Equal(t, 1, len(remaining))
	assert.Equal(t, `{"id": "new_event"}`, remaining[0].ExtraInfo)

	// Try to find old event by ExtraInfo
	oldEvent := Event{
		Timestamp: baseTime.Add(-10 * time.Minute).Unix(),
		Name:      "test",
		Type:      "event_type",
		ExtraInfo: `{"id": "old_event"}`,
	}
	found, err := db.Find(ctx, oldEvent)
	assert.NoError(t, err)
	assert.Nil(t, found, "Old event should not be found after purge")
}

func TestInvalidTableName(t *testing.T) {
	t.Parallel()

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	// Test with invalid table name
	_, err := NewStore(ctx, dbRW, dbRO, "invalid;table;name")
	assert.Error(t, err)
}

func TestContextCancellation(t *testing.T) {
	t.Parallel()

	testTableName := "test_table"

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	store, err := NewStore(ctx, dbRW, dbRO, testTableName)
	assert.NoError(t, err)

	// Test with canceled context
	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	event := Event{
		Timestamp:        time.Now().UTC().Unix(),
		Name:             "test",
		Type:             "test_event",
		SuggestedActions: "Test details",
	}

	err = store.Insert(canceledCtx, event)
	assert.Error(t, err)

	_, err = store.Find(canceledCtx, event)
	assert.Error(t, err)

	_, err = store.Get(canceledCtx, time.Now().Add(-1*time.Hour))
	assert.Error(t, err)
}

func TestConcurrentAccess(t *testing.T) {
	t.Parallel()

	testTableName := "test_table"

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	db, err := NewStore(ctx, dbRW, dbRO, testTableName)
	assert.NoError(t, err)

	baseTime := time.Now().UTC()
	eventCount := 100
	done := make(chan bool)

	// Concurrent inserts
	go func() {
		for i := 0; i < eventCount; i++ {
			event := Event{
				Timestamp:        baseTime.Add(time.Duration(i) * time.Second).Unix(),
				Name:             "concurrent",
				Type:             "test_event",
				SuggestedActions: fmt.Sprintf("Concurrent event %d", i),
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

	store, err := NewStore(ctx, dbRW, dbRO, testTableName)
	assert.NoError(t, err)

	events := []Event{
		{
			Timestamp:        time.Now().UTC().Unix(),
			Name:             "test;source",
			Type:             "test'type",
			Message:          "message with special chars: !@#$%^&*()",
			ExtraInfo:        "special chars: !@#$%^&*()",
			SuggestedActions: "details with special chars",
		},
		{
			Timestamp:        time.Now().UTC().Unix(),
			Name:             "unicode_source_🔥",
			Type:             "unicode_type_⚡",
			Message:          "unicode message: 你好",
			ExtraInfo:        "unicode info: 你好",
			SuggestedActions: "unicode details: 世界！",
		},
	}

	// Test insert and retrieval of events with special characters
	for _, event := range events {
		err = store.Insert(ctx, event)
		assert.NoError(t, err)

		found, err := store.Find(ctx, event)
		assert.NoError(t, err)
		assert.NotNil(t, found)
		assert.Equal(t, event.Name, found.Name)
		assert.Equal(t, event.Type, found.Type)
		assert.Equal(t, event.Message, found.Message)
		assert.Equal(t, event.ExtraInfo, found.ExtraInfo)
		assert.Equal(t, event.SuggestedActions, found.SuggestedActions)
	}
}

func TestLargeEventDetails(t *testing.T) {
	t.Parallel()

	testTableName := "test_table"

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	db, err := NewStore(ctx, dbRW, dbRO, testTableName)
	assert.NoError(t, err)

	// Create a large event detail string (100KB)
	largeDetail := make([]byte, 100*1024)
	for i := range largeDetail {
		largeDetail[i] = byte('a' + (i % 26))
	}

	event := Event{
		Timestamp:        time.Now().UTC().Unix(),
		Name:             "test",
		Type:             "large_event",
		SuggestedActions: string(largeDetail),
	}

	err = db.Insert(ctx, event)
	assert.NoError(t, err)

	found, err := db.Find(ctx, event)
	assert.NoError(t, err)
	assert.NotNil(t, found)
	assert.Equal(t, event.SuggestedActions, found.SuggestedActions)
}

func TestTimestampBoundaries(t *testing.T) {
	t.Parallel()

	testTableName := "test_table"

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	store, err := NewStore(ctx, dbRW, dbRO, testTableName)
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
			Timestamp:        ts,
			Name:             "test",
			Type:             "timestamp_test",
			SuggestedActions: fmt.Sprintf("timestamp: %d", ts),
		}

		err = store.Insert(ctx, event)
		assert.NoError(t, err)

		found, err := store.Find(ctx, event)
		assert.NoError(t, err)
		assert.NotNil(t, found)
		assert.Equal(t, ts, found.Timestamp)
	}

	// Test retrieval with various time ranges
	events, err := store.Get(ctx, time.Unix(-(1<<63), 0)) // Get all events
	assert.NoError(t, err)
	assert.Equal(t, len(timestamps), len(events))

	events, err = store.Get(ctx, time.Unix(1<<63-1, 0)) // Future time
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

	store, err := NewStore(ctx, dbRW, dbRO, testTableName)
	assert.NoError(t, err)

	baseTime := time.Now().UTC()
	eventCount := 100
	done := make(chan bool)

	// Concurrent inserts
	go func() {
		for i := 0; i < eventCount; i++ {
			event := Event{
				Timestamp:        baseTime.Add(time.Duration(i) * time.Second).Unix(),
				Name:             "concurrent",
				Type:             "test_event",
				ExtraInfo:        fmt.Sprintf("info_%d", i),
				SuggestedActions: fmt.Sprintf("Concurrent event %d", i),
			}
			assert.NoError(t, store.Insert(ctx, event))
		}
		done <- true
	}()

	// Concurrent reads
	go func() {
		for i := 0; i < eventCount; i++ {
			event := Event{
				Timestamp: baseTime.Add(time.Duration(i) * time.Second).Unix(),
				Name:      "concurrent",
				Type:      "test_event",
				ExtraInfo: fmt.Sprintf("info_%d", i),
			}
			found, err := store.Find(ctx, event)
			if err == nil && found != nil {
				assert.Equal(t, event.ExtraInfo, found.ExtraInfo)
			}
		}
		done <- true
	}()

	// Wait for both goroutines to complete
	<-done
	<-done

	// Verify all events were inserted
	events, err := store.Get(ctx, baseTime.Add(-1*time.Hour))
	assert.NoError(t, err)
	assert.Equal(t, eventCount, len(events))

	// Verify each event has unique info
	infoMap := make(map[string]bool)
	for _, event := range events {
		assert.False(t, infoMap[event.ExtraInfo], "Duplicate extra info found")
		infoMap[event.ExtraInfo] = true
	}
}

func TestNewStoreErrors(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	testTableName := "test_table"

	// Test case: nil write DB
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()
	store, err := NewStore(ctx, nil, dbRO, testTableName)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrNoDBRWSet)
	assert.Nil(t, store)

	// Test case: nil read DB
	store, err = NewStore(ctx, dbRW, nil, testTableName)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrNoDBROSet)
	assert.Nil(t, store)

	// Test case: both DBs nil
	store, err = NewStore(ctx, nil, nil, testTableName)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrNoDBRWSet)
	assert.Nil(t, store)
}

func TestEventMessage(t *testing.T) {
	t.Parallel()

	testTableName := "test_table"

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	store, err := NewStore(ctx, dbRW, dbRO, testTableName)
	assert.NoError(t, err)

	baseTime := time.Now().UTC()
	events := []Event{
		{
			Timestamp: baseTime.Unix(),
			Name:      "test",
			Type:      "test_event",
			Message:   "Test message with normal text",
		},
		{
			Timestamp: baseTime.Add(1 * time.Second).Unix(),
			Name:      "test",
			Type:      "test_event",
			Message:   "", // Empty message
		},
		{
			Timestamp: baseTime.Add(2 * time.Second).Unix(),
			Name:      "test",
			Type:      "test_event",
			Message:   "Message with special chars: !@#$%^&*()",
		},
		{
			Timestamp: baseTime.Add(3 * time.Second).Unix(),
			Name:      "test",
			Type:      "test_event",
			Message:   "Unicode message: 你好世界",
		},
	}

	// Test insert and retrieval
	for _, event := range events {
		err = store.Insert(ctx, event)
		assert.NoError(t, err)

		found, err := store.Find(ctx, event)
		assert.NoError(t, err)
		assert.NotNil(t, found)
		assert.Equal(t, event.Message, found.Message)
	}

	// Test finding with message as part of search criteria
	searchEvent := Event{
		Timestamp: baseTime.Unix(),
		Name:      "test",
		Type:      "test_event",
		Message:   "Test message with normal text",
	}
	found, err := store.Find(ctx, searchEvent)
	assert.NoError(t, err)
	assert.NotNil(t, found)
	assert.Equal(t, searchEvent.Message, found.Message)

	// Test finding with empty message
	emptyMessageEvent := Event{
		Timestamp: baseTime.Add(1 * time.Second).Unix(),
		Name:      "test",
		Type:      "test_event",
		Message:   "",
	}
	found, err = store.Find(ctx, emptyMessageEvent)
	assert.NoError(t, err)
	assert.NotNil(t, found)
	assert.Equal(t, "", found.Message)

	// Test finding with non-matching message
	nonMatchingEvent := Event{
		Timestamp: baseTime.Unix(),
		Name:      "test",
		Type:      "test_event",
		Message:   "Non-matching message",
	}
	found, err = store.Find(ctx, nonMatchingEvent)
	assert.NoError(t, err)
	assert.Nil(t, found)

	// Test getting all events and verify messages
	allEvents, err := store.Get(ctx, baseTime.Add(-1*time.Second))
	assert.NoError(t, err)
	assert.Equal(t, len(events), len(allEvents))

	// Verify messages are preserved in descending timestamp order
	for i, event := range allEvents {
		expectedMsg := events[len(events)-1-i].Message
		assert.Equal(t, expectedMsg, event.Message)
	}
}
