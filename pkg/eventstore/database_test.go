package eventstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/pkg/sqlite"
)

func Test_defaultTableName(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple name",
			input:    "test",
			expected: fmt.Sprintf("components_test_events_%s", schemaVersion),
		},
		{
			name:     "name with spaces",
			input:    "test component",
			expected: fmt.Sprintf("components_test_component_events_%s", schemaVersion),
		},
		{
			name:     "name with hyphens",
			input:    "test-component",
			expected: fmt.Sprintf("components_test_component_events_%s", schemaVersion),
		},
		{
			name:     "mixed case",
			input:    "TestComponent",
			expected: fmt.Sprintf("components_testcomponent_events_%s", schemaVersion),
		},
		{
			name:     "complex name",
			input:    "Test Component-Name",
			expected: fmt.Sprintf("components_test_component_name_events_%s", schemaVersion),
		},
		{
			name:     "empty string",
			input:    "",
			expected: fmt.Sprintf("components__events_%s", schemaVersion),
		},
		{
			name:     "multiple spaces and hyphens",
			input:    "test  component--name",
			expected: fmt.Sprintf("components_test_component_name_events_%s", schemaVersion),
		},
	}

	for _, tc := range testCases {
		tc := tc // capture range variable
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result := defaultTableName(tc.input)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestTableInsertsReads(t *testing.T) {
	t.Parallel()

	testTableName := "test_table"

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	store, err := New(dbRW, dbRO, 0)
	assert.NoError(t, err)
	bucket, err := store.Bucket(testTableName)
	assert.NoError(t, err)
	defer bucket.Close()

	first := time.Now().UTC()

	events := Events{}
	eventsN := 10
	for i := 0; i < eventsN; i++ {
		events = append(events, Event{
			Time:    first.Add(time.Duration(i) * time.Second),
			Name:    "kmsg",
			Type:    string(apiv1.EventTypeWarning),
			Message: fmt.Sprintf("OOM event %d occurred", i),
		})
	}

	for _, ev := range events {
		assert.NoError(t, bucket.Insert(ctx, ev))
	}

	events, err = bucket.Get(ctx, first.Add(-30*time.Second))
	assert.NoError(t, err)
	assert.Equal(t, eventsN, len(events))

	// make sure timestamp is in descending order
	for i := 1; i < len(events); i++ {
		assert.Greater(t, events[i-1].Time.Unix(), events[i].Time.Unix(), "timestamps should be in descending order")
		// Since events are returned in descending order (newest first),
		// the message index should be eventsN - (i + 1) for the current event
		expectedMsg := fmt.Sprintf("OOM event %d occurred", eventsN-(i+1))
		assert.Equal(t, expectedMsg, events[i].Message, "messages should match in descending order")
	}
}

func TestGetEventsTimeRange(t *testing.T) {
	t.Parallel()

	testTableName := "test_table"

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	store, err := New(dbRW, dbRO, 0)
	assert.NoError(t, err)
	bucket, err := store.Bucket(testTableName)
	assert.NoError(t, err)
	defer bucket.Close()

	baseTime := time.Now().UTC()
	events := Events{
		{
			Time: baseTime.Add(-10 * time.Minute),
			Name: "kmsg",
			Type: string(apiv1.EventTypeWarning),
		},
		{
			Time: baseTime.Add(-5 * time.Minute),
			Name: "kmsg",
			Type: string(apiv1.EventTypeWarning),
		},
		{
			Time: baseTime,
			Name: "kmsg",
			Type: string(apiv1.EventTypeWarning),
		},
	}

	for _, ev := range events {
		assert.NoError(t, bucket.Insert(ctx, ev))
	}

	// Test getting all events
	allEvents, err := bucket.Get(ctx, baseTime.Add(-15*time.Minute))
	assert.NoError(t, err)
	assert.Equal(t, 3, len(allEvents))

	// Test getting recent events only
	recentEvents, err := bucket.Get(ctx, baseTime.Add(-2*time.Minute))
	assert.NoError(t, err)
	assert.Equal(t, 1, len(recentEvents))
}

func TestEmptyResults(t *testing.T) {
	t.Parallel()

	testTableName := "test_table"

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	store, err := New(dbRW, dbRO, 0)
	assert.NoError(t, err)
	bucket, err := store.Bucket(testTableName)
	assert.NoError(t, err)
	defer bucket.Close()

	// Test getting events from empty table
	events, err := bucket.Get(ctx, time.Now().Add(-1*time.Hour))
	assert.NoError(t, err)
	assert.Nil(t, events)
}

func TestMultipleEventTypes(t *testing.T) {
	t.Parallel()

	testTableName := "test_table"

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	store, err := New(dbRW, dbRO, 0)
	assert.NoError(t, err)
	bucket, err := store.Bucket(testTableName)
	assert.NoError(t, err)
	defer bucket.Close()

	baseTime := time.Now().UTC()
	events := Events{
		{
			Time: baseTime,
			Name: "kmsg",
			Type: string(apiv1.EventTypeWarning),
		},
		{
			Time: baseTime.Add(1 * time.Second),
			Name: "syslog",
			Type: string(apiv1.EventTypeWarning),
		},
		{
			Time: baseTime.Add(2 * time.Second),
			Name: "kmsg",
			Type: string(apiv1.EventTypeWarning),
		},
	}

	for _, ev := range events {
		assert.NoError(t, bucket.Insert(ctx, ev))
	}

	// Get all events
	results, err := bucket.Get(ctx, baseTime.Add(-1*time.Second))
	assert.NoError(t, err)
	assert.Equal(t, 3, len(results))

	// Verify events are in descending order
	assert.Equal(t, string(apiv1.EventTypeWarning), results[0].Type)
	assert.Equal(t, string(apiv1.EventTypeWarning), results[1].Type)
	assert.Equal(t, string(apiv1.EventTypeWarning), results[2].Type)
}

func TestPurgePartial(t *testing.T) {
	t.Parallel()

	testTableName := "test_table"

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	store, err := New(dbRW, dbRO, 0)
	assert.NoError(t, err)
	bucket, err := store.Bucket(testTableName)
	assert.NoError(t, err)
	defer bucket.Close()

	baseTime := time.Now().UTC()
	events := Events{
		{
			Time:      baseTime.Add(-10 * time.Minute),
			Name:      "kmsg",
			Type:      string(apiv1.EventTypeWarning),
			ExtraInfo: map[string]string{"id": "old_event"},
		},
		{
			Time:      baseTime,
			Name:      "kmsg",
			Type:      string(apiv1.EventTypeWarning),
			ExtraInfo: map[string]string{"id": "new_event"},
		},
	}

	for _, ev := range events {
		assert.NoError(t, bucket.Insert(ctx, ev))
	}

	// Purge only old events
	deleted, err := bucket.Purge(ctx, baseTime.Add(-5*time.Minute).Unix())
	assert.NoError(t, err)
	assert.Equal(t, 1, deleted)

	// Verify only recent event remains
	remaining, err := bucket.Get(ctx, baseTime.Add(-15*time.Minute))
	assert.NoError(t, err)
	assert.Equal(t, 1, len(remaining))
	extraInfoJSON, err := json.Marshal(remaining[0].ExtraInfo)
	assert.NoError(t, err)
	assert.Equal(t, `{"id":"new_event"}`, string(extraInfoJSON))

	// Try to find old event by ExtraInfo
	oldEvent := Event{
		Time:      baseTime.Add(-10 * time.Minute),
		Name:      "test",
		Type:      string(apiv1.EventTypeWarning),
		ExtraInfo: map[string]string{"id": "old_event"},
	}
	found, err := bucket.Find(ctx, oldEvent)
	assert.NoError(t, err)
	assert.Nil(t, found, "Old event should not be found after purge")
}

func TestFindEvent(t *testing.T) {
	t.Parallel()

	testTableName := "test_table"

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	store, err := New(dbRW, dbRO, 0)
	assert.NoError(t, err)
	bucket, err := store.Bucket(testTableName)
	assert.NoError(t, err)
	defer bucket.Close()

	baseTime := time.Now().UTC()
	testEvent := Event{
		Time:      baseTime.Add(-10 * time.Minute),
		Name:      "kmsg",
		Type:      string(apiv1.EventTypeWarning),
		ExtraInfo: map[string]string{"a": "b"},
	}

	// Test finding non-existent event
	found, err := bucket.Find(ctx, testEvent)
	assert.NoError(t, err)
	assert.Nil(t, found)

	// Insert and find the event
	assert.NoError(t, bucket.Insert(ctx, testEvent))

	found, err = bucket.Find(ctx, testEvent)
	assert.NoError(t, err)
	assert.NotNil(t, found)
	assert.Equal(t, testEvent.Time.Unix(), found.Time.Unix())
	assert.Equal(t, testEvent.Name, found.Name)
	assert.Equal(t, testEvent.Type, found.Type)
	assert.Equal(t, testEvent.ExtraInfo, found.ExtraInfo)
}

func TestFindEventPartialMatch(t *testing.T) {
	t.Parallel()

	testTableName := "test_table"

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	store, err := New(dbRW, dbRO, 0)
	assert.NoError(t, err)
	bucket, err := store.Bucket(testTableName)
	assert.NoError(t, err)
	defer bucket.Close()

	baseTime := time.Now().UTC()
	testEvent := Event{
		Time:      baseTime,
		Name:      "kmsg",
		Type:      string(apiv1.EventTypeWarning),
		ExtraInfo: map[string]string{"a": "b"},
	}
	assert.NoError(t, bucket.Insert(ctx, testEvent))

	// Test finding with matching timestamp/source/type but different details
	testEvent.ExtraInfo["a"] = "c"
	searchEvent := Event{
		Time:      testEvent.Time,
		Name:      testEvent.Name,
		Type:      testEvent.Type,
		ExtraInfo: testEvent.ExtraInfo,
	}

	found, err := bucket.Find(ctx, searchEvent)
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

	store, err := New(dbRW, dbRO, 0)
	assert.NoError(t, err)
	bucket, err := store.Bucket(testTableName)
	assert.NoError(t, err)
	defer bucket.Close()

	baseTime := time.Now().UTC()
	events := Events{
		{
			Time:      baseTime,
			Name:      "kmsg",
			Type:      string(apiv1.EventTypeWarning),
			ExtraInfo: map[string]string{"a": "b", "c": "d"},
		},
		{
			Time:      baseTime,
			Name:      "kmsg",
			Type:      string(apiv1.EventTypeWarning),
			ExtraInfo: map[string]string{"a": "b"},
		},
	}

	// Insert multiple events with same timestamp/source/type
	for _, ev := range events {
		assert.NoError(t, bucket.Insert(ctx, ev))
	}

	// Search should return the first matching event
	searchEvent := Event{
		Time:      baseTime,
		Name:      "kmsg",
		Type:      string(apiv1.EventTypeWarning),
		ExtraInfo: map[string]string{"a": "b"},
	}

	found, err := bucket.Find(ctx, searchEvent)
	assert.NoError(t, err)
	assert.NotNil(t, found)
}

func TestEventWithIDs(t *testing.T) {
	t.Parallel()

	testTableName := "test_table"

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	store, err := New(dbRW, dbRO, 0)
	assert.NoError(t, err)
	bucket, err := store.Bucket(testTableName)
	assert.NoError(t, err)
	defer bucket.Close()

	baseTime := time.Now().UTC()
	event := Event{
		Time: baseTime,
		Name: "nvidia-smi",
		Type: string(apiv1.EventTypeWarning),
		ExtraInfo: map[string]string{
			"xid":      "123",
			"gpu_uuid": "gpu-123",
		},
	}

	// Test insert and find with ExtraInfo
	err = bucket.Insert(ctx, event)
	assert.NoError(t, err)

	found, err := bucket.Find(ctx, event)
	assert.NoError(t, err)
	assert.NotNil(t, found)
	assert.Equal(t, event.ExtraInfo, found.ExtraInfo)

	// Test find with partial ExtraInfo match
	partialEvent := Event{
		Time: event.Time,
		Name: event.Name,
		Type: event.Type,
	}

	found, err = bucket.Find(ctx, partialEvent)
	assert.NoError(t, err)
	assert.Nil(t, found, "Should not find event with different ExtraInfo")

	// Test find with different ExtraInfo
	differentEvent := Event{
		Time: event.Time,
		Name: event.Name,
		Type: event.Type,
		ExtraInfo: map[string]string{
			"xid":      "different",
			"gpu_uuid": "different-gpu",
		},
	}

	found, err = bucket.Find(ctx, differentEvent)
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

	store, err := New(dbRW, dbRO, 0)
	assert.NoError(t, err)
	bucket, err := store.Bucket(testTableName)
	assert.NoError(t, err)
	defer bucket.Close()

	baseTime := time.Now().UTC()
	event := Event{
		Time:      baseTime,
		Name:      "kmsg",
		Type:      string(apiv1.EventTypeWarning),
		ExtraInfo: map[string]string{},
	}

	// Test insert and find with null ExtraInfo
	err = bucket.Insert(ctx, event)
	assert.NoError(t, err)

	found, err := bucket.Find(ctx, event)
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

	store, err := New(dbRW, dbRO, 0)
	assert.NoError(t, err)
	bucket, err := store.Bucket(testTableName)
	assert.NoError(t, err)
	defer bucket.Close()

	baseTime := time.Now().UTC()
	events := Events{
		{
			Time:      baseTime.Add(-10 * time.Minute),
			Name:      "test",
			Type:      string(apiv1.EventTypeWarning),
			ExtraInfo: map[string]string{"id": "old_event"},
		},
		{
			Time:      baseTime,
			Name:      "test",
			Type:      string(apiv1.EventTypeWarning),
			ExtraInfo: map[string]string{"id": "new_event"},
		},
	}

	for _, event := range events {
		err = bucket.Insert(ctx, event)
		assert.NoError(t, err)
	}

	// purge old events
	deleted, err := bucket.Purge(ctx, baseTime.Add(-5*time.Minute).Unix())
	assert.NoError(t, err)
	assert.Equal(t, 1, deleted)

	// Verify only new event remains with correct ExtraInfo
	remaining, err := bucket.Get(ctx, baseTime.Add(-15*time.Minute))
	assert.NoError(t, err)
	assert.Equal(t, 1, len(remaining))
	extraInfoJSON, err := json.Marshal(remaining[0].ExtraInfo)
	assert.NoError(t, err)
	assert.Equal(t, `{"id":"new_event"}`, string(extraInfoJSON))

	// Try to find old event by ExtraInfo
	oldEvent := Event{
		Time:      baseTime.Add(-10 * time.Minute),
		Name:      "test",
		Type:      string(apiv1.EventTypeWarning),
		ExtraInfo: map[string]string{"id": "old_event"},
	}
	found, err := bucket.Find(ctx, oldEvent)
	assert.NoError(t, err)
	assert.Nil(t, found, "Old event should not be found after purge")
}

func TestInvalidTableName(t *testing.T) {
	t.Parallel()

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	// Test with invalid table name
	store, err := New(dbRW, dbRO, 0)
	assert.NoError(t, err)
	_, err = store.Bucket("invalid;table;name")
	assert.Error(t, err)
}

func TestContextCancellation(t *testing.T) {
	t.Parallel()

	testTableName := "test_table"

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	store, err := New(dbRW, dbRO, 0)
	assert.NoError(t, err)
	bucket, err := store.Bucket(testTableName)
	assert.NoError(t, err)
	defer bucket.Close()

	// Test with canceled context
	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	event := Event{
		Time: time.Now().UTC(),
		Name: "test",
		Type: string(apiv1.EventTypeWarning),
	}

	err = bucket.Insert(canceledCtx, event)
	assert.Error(t, err)

	_, err = bucket.Find(canceledCtx, event)
	assert.Error(t, err)

	_, err = bucket.Get(canceledCtx, time.Now().Add(-1*time.Hour))
	assert.Error(t, err)
}

func TestConcurrentAccess(t *testing.T) {
	t.Parallel()

	testTableName := "test_table"

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	store, err := New(dbRW, dbRO, 0)
	assert.NoError(t, err)
	bucket, err := store.Bucket(testTableName)
	assert.NoError(t, err)
	defer bucket.Close()

	baseTime := time.Now().UTC()
	eventCount := 100
	done := make(chan bool)

	// Concurrent inserts
	go func() {
		for i := 0; i < eventCount; i++ {
			event := Event{
				Time: baseTime.Add(time.Duration(i) * time.Second),
				Name: "concurrent",
				Type: string(apiv1.EventTypeWarning),
			}
			assert.NoError(t, bucket.Insert(ctx, event))
		}
		done <- true
	}()

	// Concurrent reads
	go func() {
		for i := 0; i < eventCount; i++ {
			_, err := bucket.Get(ctx, baseTime.Add(-1*time.Hour))
			assert.NoError(t, err)
		}
		done <- true
	}()

	// Wait for both goroutines to complete
	<-done
	<-done

	// Verify final count
	events, err := bucket.Get(ctx, baseTime.Add(-1*time.Hour))
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

	store, err := New(dbRW, dbRO, 0)
	assert.NoError(t, err)
	bucket, err := store.Bucket(testTableName)
	assert.NoError(t, err)
	defer bucket.Close()

	events := Events{
		{
			Time:      time.Now().UTC(),
			Name:      "test;source",
			Type:      string(apiv1.EventTypeWarning),
			Message:   "message with special chars: !@#$%^&*()",
			ExtraInfo: map[string]string{"special chars": "!@#$%^&*()"},
		},
		{
			Time:      time.Now().UTC(),
			Name:      "unicode_source_🔥",
			Type:      string(apiv1.EventTypeWarning),
			Message:   "unicode message: 你好",
			ExtraInfo: map[string]string{"unicode info": "你好"},
		},
	}

	// Test insert and retrieval of events with special characters
	for _, event := range events {
		err = bucket.Insert(ctx, event)
		assert.NoError(t, err)

		found, err := bucket.Find(ctx, event)
		assert.NoError(t, err)
		assert.NotNil(t, found)
		assert.Equal(t, event.Name, found.Name)
		assert.Equal(t, event.Type, found.Type)
		assert.Equal(t, event.Message, found.Message)
		assert.Equal(t, event.ExtraInfo, found.ExtraInfo)
	}
}

func TestLargeEventDetails(t *testing.T) {
	t.Parallel()

	testTableName := "test_table"

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	store, err := New(dbRW, dbRO, 0)
	assert.NoError(t, err)
	bucket, err := store.Bucket(testTableName)
	assert.NoError(t, err)
	defer bucket.Close()

	// Create a large event detail string (100KB)
	largeDetail := make([]byte, 100*1024)
	for i := range largeDetail {
		largeDetail[i] = byte('a' + (i % 26))
	}

	event := Event{
		Time: time.Now().UTC(),
		Name: "test",
		Type: string(apiv1.EventTypeWarning),
	}

	err = bucket.Insert(ctx, event)
	assert.NoError(t, err)

	found, err := bucket.Find(ctx, event)
	assert.NoError(t, err)
	assert.NotNil(t, found)
}

func TestTimestampBoundaries(t *testing.T) {
	t.Parallel()

	testTableName := "test_table"

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	store, err := New(dbRW, dbRO, 0)
	assert.NoError(t, err)
	bucket, err := store.Bucket(testTableName)
	assert.NoError(t, err)
	defer bucket.Close()

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
			Time: time.Unix(ts, 0),
			Name: "test",
			Type: string(apiv1.EventTypeWarning),
		}

		err = bucket.Insert(ctx, event)
		assert.NoError(t, err)

		found, err := bucket.Find(ctx, event)
		assert.NoError(t, err)
		assert.NotNil(t, found)
		assert.Equal(t, ts, found.Time.Unix())
	}

	// Test retrieval with various time ranges
	events, err := bucket.Get(ctx, time.Unix(-(1<<63), 0)) // Get all events
	assert.NoError(t, err)
	assert.Equal(t, len(timestamps), len(events))

	events, err = bucket.Get(ctx, time.Unix(1<<63-1, 0)) // Future time
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

	store, err := New(dbRW, dbRO, 0)
	assert.NoError(t, err)
	bucket, err := store.Bucket(testTableName)
	assert.NoError(t, err)
	defer bucket.Close()

	baseTime := time.Now().UTC()
	eventCount := 100
	done := make(chan bool)

	// Concurrent inserts
	go func() {
		for i := 0; i < eventCount; i++ {
			event := Event{
				Time:      baseTime.Add(time.Duration(i) * time.Second),
				Name:      "concurrent",
				Type:      string(apiv1.EventTypeWarning),
				ExtraInfo: map[string]string{fmt.Sprintf("info_%d", i): fmt.Sprintf("Concurrent event %d", i)},
			}
			assert.NoError(t, bucket.Insert(ctx, event))
		}
		done <- true
	}()

	// Concurrent reads
	go func() {
		for i := 0; i < eventCount; i++ {
			event := Event{
				Time:      baseTime.Add(time.Duration(i) * time.Second),
				Name:      "concurrent",
				Type:      string(apiv1.EventTypeWarning),
				ExtraInfo: map[string]string{fmt.Sprintf("info_%d", i): fmt.Sprintf("Concurrent event %d", i)},
			}
			found, err := bucket.Find(ctx, event)
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
	events, err := bucket.Get(ctx, baseTime.Add(-1*time.Hour))
	assert.NoError(t, err)
	assert.Equal(t, eventCount, len(events))

	// Verify each event has unique info
	infoMap := make(map[string]bool)
	for _, event := range events {
		// Convert the entire ExtraInfo map to a string for comparison
		infoStr := fmt.Sprintf("%v", event.ExtraInfo)
		assert.False(t, infoMap[infoStr], "Duplicate extra info found")
		infoMap[infoStr] = true
	}
}

func TestEventMessage(t *testing.T) {
	t.Parallel()

	testTableName := "test_table"

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	store, err := New(dbRW, dbRO, 0)
	assert.NoError(t, err)
	bucket, err := store.Bucket(testTableName)
	assert.NoError(t, err)
	defer bucket.Close()

	baseTime := time.Now().UTC()
	events := Events{
		{
			Time:    baseTime,
			Name:    "test",
			Type:    string(apiv1.EventTypeWarning),
			Message: "Test message with normal text",
		},
		{
			Time:    baseTime.Add(1 * time.Second),
			Name:    "test",
			Type:    string(apiv1.EventTypeWarning),
			Message: "", // Empty message
		},
		{
			Time:    baseTime.Add(2 * time.Second),
			Name:    "test",
			Type:    string(apiv1.EventTypeWarning),
			Message: "Message with special chars: !@#$%^&*()",
		},
		{
			Time:    baseTime.Add(3 * time.Second),
			Name:    "test",
			Type:    string(apiv1.EventTypeWarning),
			Message: "Unicode message: 你好世界",
		},
	}

	// Test insert and retrieval
	for _, event := range events {
		err = bucket.Insert(ctx, event)
		assert.NoError(t, err)

		found, err := bucket.Find(ctx, event)
		assert.NoError(t, err)
		assert.NotNil(t, found)
		assert.Equal(t, event.Message, found.Message)
	}

	// Test finding with message as part of search criteria
	searchEvent := Event{
		Time:    baseTime,
		Name:    "test",
		Type:    string(apiv1.EventTypeWarning),
		Message: "Test message with normal text",
	}
	found, err := bucket.Find(ctx, searchEvent)
	assert.NoError(t, err)
	assert.NotNil(t, found)
	assert.Equal(t, searchEvent.Message, found.Message)

	// Test finding with empty message
	emptyMessageEvent := Event{
		Time:    baseTime.Add(1 * time.Second),
		Name:    "test",
		Type:    string(apiv1.EventTypeWarning),
		Message: "",
	}
	found, err = bucket.Find(ctx, emptyMessageEvent)
	assert.NoError(t, err)
	assert.NotNil(t, found)
	assert.Equal(t, "", found.Message)

	// Test finding with non-matching message
	nonMatchingEvent := Event{
		Time:    baseTime,
		Name:    "test",
		Type:    string(apiv1.EventTypeWarning),
		Message: "Non-matching message",
	}
	found, err = bucket.Find(ctx, nonMatchingEvent)
	assert.NoError(t, err)
	assert.Nil(t, found)

	// Test getting all events and verify messages
	allEvents, err := bucket.Get(ctx, baseTime.Add(-1*time.Second))
	assert.NoError(t, err)
	assert.Equal(t, len(events), len(allEvents))

	// Verify messages are preserved in descending timestamp order
	for i, event := range allEvents {
		expectedMsg := events[len(events)-1-i].Message
		assert.Equal(t, expectedMsg, event.Message)
	}
}

func TestInvalidJSONHandling(t *testing.T) {
	t.Parallel()

	testTableName := "test_table"

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	store, err := New(dbRW, dbRO, 0)
	assert.NoError(t, err)
	bucket, err := store.Bucket(testTableName)
	assert.NoError(t, err)
	defer bucket.Close()

	// Insert a valid event first
	baseTime := time.Now().UTC()
	event := Event{
		Time:      baseTime,
		Name:      "test",
		Type:      string(apiv1.EventTypeWarning),
		ExtraInfo: map[string]string{"key": "value"},
	}
	err = bucket.Insert(ctx, event)
	assert.NoError(t, err)

	// Manually insert invalid JSON into the database
	_, err = dbRW.ExecContext(ctx, fmt.Sprintf(`
		INSERT INTO %s (timestamp, name, type, extra_info)
		VALUES (?, ?, ?, ?)`,
		bucket.Name()),
		baseTime.Add(time.Second).Unix(),
		"test",
		string(apiv1.EventTypeWarning),
		"{invalid_json", // Invalid JSON for ExtraInfo
	)
	assert.NoError(t, err)

	// Try to retrieve the events - should get error for invalid JSON
	_, err = bucket.Get(ctx, baseTime.Add(-time.Hour))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to unmarshal")
}

func TestLongEventFields(t *testing.T) {
	t.Parallel()

	testTableName := "test_table"

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	store, err := New(dbRW, dbRO, 0)
	assert.NoError(t, err)
	bucket, err := store.Bucket(testTableName)
	assert.NoError(t, err)
	defer bucket.Close()

	// Create very long strings for various fields
	longString := strings.Repeat("a", 10000)
	longMap := make(map[string]string)
	for i := 0; i < 100; i++ {
		longMap[fmt.Sprintf("key_%d", i)] = longString
	}

	event := Event{
		Time:      time.Now().UTC(),
		Name:      longString,
		Type:      string(apiv1.EventTypeWarning),
		Message:   longString,
		ExtraInfo: longMap,
	}

	// Test insert and retrieval of event with very long fields
	err = bucket.Insert(ctx, event)
	assert.NoError(t, err)

	found, err := bucket.Find(ctx, event)
	assert.NoError(t, err)
	assert.NotNil(t, found)
	assert.Equal(t, event.Name, found.Name)
	assert.Equal(t, event.Message, found.Message)
	assert.Equal(t, event.ExtraInfo, found.ExtraInfo)
}

func TestConcurrentTableCreation(t *testing.T) {
	t.Parallel()

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	// Try to create multiple stores with the same table name concurrently
	tableName := "concurrent_table"
	concurrency := 10
	var wg sync.WaitGroup
	stores := make([]Store, concurrency)
	errors := make([]error, concurrency)

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			store, err := New(dbRW, dbRO, 0)
			assert.NoError(t, err)
			bucket, err := store.Bucket(tableName)
			assert.NoError(t, err)
			defer bucket.Close()

			stores[index] = store
			errors[index] = err
		}(i)
	}

	wg.Wait()

	// Verify that all attempts either succeeded or failed gracefully
	successCount := 0
	for i := 0; i < concurrency; i++ {
		if errors[i] == nil {
			successCount++
			assert.NotNil(t, stores[i])
		}
	}
	assert.Greater(t, successCount, 0)
}

func TestEventTypeValidation(t *testing.T) {
	t.Parallel()

	testTableName := "test_table"

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	store, err := New(dbRW, dbRO, 0)
	assert.NoError(t, err)
	bucket, err := store.Bucket(testTableName)
	assert.NoError(t, err)
	defer bucket.Close()

	// Test all valid event types
	validTypes := []apiv1.EventType{
		apiv1.EventTypeWarning,
		apiv1.EventTypeInfo,
		apiv1.EventTypeCritical,
		apiv1.EventTypeFatal,
		apiv1.EventTypeUnknown,
	}

	baseTime := time.Now().UTC()
	for i, eventType := range validTypes {
		event := Event{
			Time:    baseTime.Add(time.Duration(i) * time.Second),
			Name:    "test",
			Type:    string(eventType),
			Message: fmt.Sprintf("Test message for %s", eventType),
		}

		err = bucket.Insert(ctx, event)
		assert.NoError(t, err)

		found, err := bucket.Find(ctx, event)
		assert.NoError(t, err)
		assert.NotNil(t, found)
		assert.Equal(t, string(eventType), found.Type)
	}

	// Verify all events can be retrieved
	events, err := bucket.Get(ctx, baseTime.Add(-time.Hour))
	assert.NoError(t, err)
	assert.Equal(t, len(validTypes), len(events))
}

func TestRetentionPurge(t *testing.T) {
	t.Parallel()

	testTableName := "test_table"

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	bucket, err := newTable(
		dbRW,
		dbRO,
		testTableName,
		10*time.Second,
		// much shorter than the retention period
		// to make tests less flaky
		50*time.Millisecond,
	)
	assert.NoError(t, err)
	defer bucket.Close()

	baseTime := time.Now().UTC()
	events := Events{
		{
			Time:      baseTime.Add(-15 * time.Second),
			Name:      "test",
			Type:      string(apiv1.EventTypeWarning),
			ExtraInfo: map[string]string{"id": "old_event"},
		},
		{
			Time:      baseTime.Add(-5 * time.Second),
			Name:      "test",
			Type:      string(apiv1.EventTypeWarning),
			ExtraInfo: map[string]string{"id": "new_event"},
		},
	}

	for _, event := range events {
		err = bucket.Insert(ctx, event)
		assert.NoError(t, err)
	}

	time.Sleep(time.Second)

	remaining, err := bucket.Get(ctx, baseTime.Add(-20*time.Second))
	assert.NoError(t, err)
	assert.Equal(t, 1, len(remaining))
	assert.Equal(t, "new_event", remaining[0].ExtraInfo["id"])
}

func TestLatest(t *testing.T) {
	t.Parallel()

	testTableName := "test_table"

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	store, err := New(dbRW, dbRO, 0)
	assert.NoError(t, err)
	bucket, err := store.Bucket(testTableName)
	assert.NoError(t, err)
	defer bucket.Close()

	// Test with empty store
	latestEvent, err := bucket.Latest(ctx)
	assert.NoError(t, err)
	assert.Nil(t, latestEvent, "Latest should return nil for empty store")

	// Insert events with different timestamps
	baseTime := time.Now().UTC()
	events := Events{
		{
			Time:    baseTime.Add(-10 * time.Second),
			Name:    "test",
			Type:    string(apiv1.EventTypeWarning),
			Message: "old event",
			ExtraInfo: map[string]string{
				"id": "event1",
			},
		},
		{
			Time:    baseTime,
			Name:    "test",
			Type:    string(apiv1.EventTypeInfo),
			Message: "latest event",
			ExtraInfo: map[string]string{
				"id": "event2",
			},
		},
		{
			Time:    baseTime.Add(-5 * time.Second),
			Name:    "test",
			Type:    string(apiv1.EventTypeCritical),
			Message: "middle event",
			ExtraInfo: map[string]string{
				"id": "event3",
			},
		},
	}

	// Insert events in random order
	for _, event := range events {
		err = bucket.Insert(ctx, event)
		assert.NoError(t, err)
	}

	// Get latest event
	latestEvent, err = bucket.Latest(ctx)
	assert.NoError(t, err)
	assert.NotNil(t, latestEvent)

	// Verify it's the event with the most recent timestamp
	assert.Equal(t, baseTime.Unix(), latestEvent.Time.Unix())
	assert.Equal(t, "latest event", latestEvent.Message)
	assert.Equal(t, string(apiv1.EventTypeInfo), latestEvent.Type)
	assert.Equal(t, "event2", latestEvent.ExtraInfo["id"])

	// Test with canceled context
	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately
	_, err = bucket.Latest(canceledCtx)
	assert.Error(t, err)

	// Test after purging all events
	deleted, err := bucket.Purge(ctx, baseTime.Add(time.Hour).Unix())
	assert.NoError(t, err)
	assert.Equal(t, 3, deleted)

	latestEvent, err = bucket.Latest(ctx)
	assert.NoError(t, err)
	assert.Nil(t, latestEvent, "Latest should return nil after purging all events")
}

func TestCompareEvent(t *testing.T) {
	tests := []struct {
		name     string
		eventA   Event
		eventB   Event
		expected bool
	}{
		{
			name: "same events",
			eventA: Event{
				ExtraInfo: map[string]string{
					"key1": "value1",
					"key2": "value2",
				},
			},
			eventB: Event{
				ExtraInfo: map[string]string{
					"key1": "value1",
					"key2": "value2",
				},
			},
			expected: true,
		},
		{
			name: "different key-value pairs",
			eventA: Event{
				ExtraInfo: map[string]string{
					"key1": "value1",
					"key2": "value2",
				},
			},
			eventB: Event{
				ExtraInfo: map[string]string{
					"key1": "value1",
					"key2": "different_value",
				},
			},
			expected: false,
		},
		{
			name: "eventB missing key",
			eventA: Event{
				ExtraInfo: map[string]string{
					"key1": "value1",
					"key2": "value2",
				},
			},
			eventB: Event{
				ExtraInfo: map[string]string{
					"key1": "value1",
				},
			},
			expected: false,
		},
		{
			name: "eventA missing key",
			eventA: Event{
				ExtraInfo: map[string]string{
					"key1": "value1",
				},
			},
			eventB: Event{
				ExtraInfo: map[string]string{
					"key1": "value1",
					"key2": "value2",
				},
			},
			expected: false,
		},
		{
			name: "empty events",
			eventA: Event{
				ExtraInfo: map[string]string{},
			},
			eventB: Event{
				ExtraInfo: map[string]string{},
			},
			expected: true,
		},
		{
			name: "one empty event",
			eventA: Event{
				ExtraInfo: map[string]string{},
			},
			eventB: Event{
				ExtraInfo: map[string]string{
					"key1": "value1",
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := compareEvent(tt.eventA, tt.eventB)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestUnmarshalIfValid(t *testing.T) {
	t.Parallel()

	type testStruct struct {
		Key   string `json:"key"`
		Value int    `json:"value"`
	}

	tests := []struct {
		name          string
		data          sql.NullString
		expectedError bool
		expectedObj   *testStruct
	}{
		{
			name:          "invalid SQL string",
			data:          sql.NullString{Valid: false},
			expectedError: false,
			expectedObj:   nil,
		},
		{
			name:          "empty string",
			data:          sql.NullString{String: "", Valid: true},
			expectedError: false,
			expectedObj:   nil,
		},
		{
			name:          "null string",
			data:          sql.NullString{String: "null", Valid: true},
			expectedError: false,
			expectedObj:   nil,
		},
		{
			name:          "not starting with {",
			data:          sql.NullString{String: "[1, 2, 3]", Valid: true},
			expectedError: true,
			expectedObj:   nil,
		},
		{
			name:          "valid JSON",
			data:          sql.NullString{String: `{"key":"test","value":123}`, Valid: true},
			expectedError: false,
			expectedObj:   &testStruct{Key: "test", Value: 123},
		},
		{
			name:          "invalid JSON format",
			data:          sql.NullString{String: `{"key":"test","value":"not-an-int"}`, Valid: true},
			expectedError: true,
			expectedObj:   nil,
		},
		{
			name:          "malformed JSON",
			data:          sql.NullString{String: `{"key":"test", unclosed}`, Valid: true},
			expectedError: true,
			expectedObj:   nil,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var result *testStruct
			if tt.expectedObj != nil {
				result = &testStruct{}
			}

			err := unmarshalIfValid(tt.data, &result)

			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			if tt.expectedObj != nil {
				assert.Equal(t, tt.expectedObj, result)
			} else if !tt.expectedError {
				assert.Nil(t, result)
			}
		})
	}
}

// TestReadWithOptions tests the Read method with various options
func TestReadWithOptions(t *testing.T) {
	t.Parallel()

	testTableName := "test_table"

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	storeV2, err := NewV2(dbRW, dbRO, 0)
	assert.NoError(t, err)
	bucketV2, err := storeV2.BucketV2(testTableName)
	assert.NoError(t, err)
	defer bucketV2.Close()

	baseTime := time.Now().UTC()
	events := Events{
		{
			Time:    baseTime.Add(-10 * time.Minute),
			Name:    "event_type_a",
			Type:    string(apiv1.EventTypeWarning),
			Message: "Old event A",
			ExtraInfo: map[string]string{
				"id": "1",
			},
		},
		{
			Time:    baseTime.Add(-5 * time.Minute),
			Name:    "event_type_b",
			Type:    string(apiv1.EventTypeInfo),
			Message: "Middle event B",
			ExtraInfo: map[string]string{
				"id": "2",
			},
		},
		{
			Time:    baseTime,
			Name:    "event_type_a",
			Type:    string(apiv1.EventTypeCritical),
			Message: "Recent event A",
			ExtraInfo: map[string]string{
				"id": "3",
			},
		},
	}

	// Insert events
	for _, event := range events {
		err = bucketV2.Insert(ctx, event)
		assert.NoError(t, err)
	}

	// Test with WithSince option
	t.Run("with_since", func(t *testing.T) {
		result, err := bucketV2.Read(ctx, WithSince(baseTime.Add(-7*time.Minute)))
		assert.NoError(t, err)
		assert.Equal(t, 2, len(result), "Should return 2 recent events")
		// Results should be in descending order
		assert.Equal(t, "3", result[0].ExtraInfo["id"])
		assert.Equal(t, "2", result[1].ExtraInfo["id"])
	})

	// Test with WithName option
	t.Run("with_name", func(t *testing.T) {
		result, err := bucketV2.Read(ctx, WithName("event_type_a"))
		assert.NoError(t, err)
		assert.Equal(t, 2, len(result), "Should return 2 events with name event_type_a")
		// Both should be event_type_a, in descending order
		assert.Equal(t, "event_type_a", result[0].Name)
		assert.Equal(t, "event_type_a", result[1].Name)
		assert.Equal(t, "3", result[0].ExtraInfo["id"])
		assert.Equal(t, "1", result[1].ExtraInfo["id"])
	})

	// Test with both WithSince and WithName
	t.Run("with_since_and_name", func(t *testing.T) {
		result, err := bucketV2.Read(ctx,
			WithSince(baseTime.Add(-7*time.Minute)),
			WithName("event_type_a"))
		assert.NoError(t, err)
		assert.Equal(t, 1, len(result), "Should return 1 recent event_type_a")
		assert.Equal(t, "event_type_a", result[0].Name)
		assert.Equal(t, "3", result[0].ExtraInfo["id"])
	})

	// Test with no options (returns all events)
	t.Run("no_options", func(t *testing.T) {
		result, err := bucketV2.Read(ctx)
		assert.NoError(t, err)
		assert.Equal(t, 3, len(result), "Should return all 3 events")
		// Verify descending order
		assert.Equal(t, "3", result[0].ExtraInfo["id"])
		assert.Equal(t, "2", result[1].ExtraInfo["id"])
		assert.Equal(t, "1", result[2].ExtraInfo["id"])
	})

	// Test with canceled context
	t.Run("canceled_context", func(t *testing.T) {
		canceledCtx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately
		_, err := bucketV2.Read(canceledCtx, WithSince(baseTime))
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "context canceled")
	})
}

// TestReadEventFiltering tests the Read method's filtering capabilities
func TestReadEventFiltering(t *testing.T) {
	t.Parallel()

	testTableName := "test_table"

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	storeV2, err := NewV2(dbRW, dbRO, 0)
	assert.NoError(t, err)
	bucketV2, err := storeV2.BucketV2(testTableName)
	assert.NoError(t, err)
	defer bucketV2.Close()

	baseTime := time.Now().UTC()

	// Insert various event types
	eventNames := []string{"kmsg", "syslog", "dmesg", "kmsg", "syslog"}
	for i, name := range eventNames {
		event := Event{
			Time:    baseTime.Add(time.Duration(i) * time.Second),
			Name:    name,
			Type:    string(apiv1.EventTypeWarning),
			Message: fmt.Sprintf("Event %d", i),
			ExtraInfo: map[string]string{
				"index": fmt.Sprintf("%d", i),
			},
		}
		err = bucketV2.Insert(ctx, event)
		assert.NoError(t, err)
	}

	// Test filtering by name "kmsg"
	t.Run("filter_kmsg", func(t *testing.T) {
		result, err := bucketV2.Read(ctx, WithName("kmsg"))
		assert.NoError(t, err)
		assert.Equal(t, 2, len(result), "Should return 2 kmsg events")
		for _, event := range result {
			assert.Equal(t, "kmsg", event.Name)
		}
		// Verify descending order (latest first)
		assert.Equal(t, "3", result[0].ExtraInfo["index"])
		assert.Equal(t, "0", result[1].ExtraInfo["index"])
	})

	// Test filtering by name "syslog"
	t.Run("filter_syslog", func(t *testing.T) {
		result, err := bucketV2.Read(ctx, WithName("syslog"))
		assert.NoError(t, err)
		assert.Equal(t, 2, len(result), "Should return 2 syslog events")
		for _, event := range result {
			assert.Equal(t, "syslog", event.Name)
		}
		// Verify descending order
		assert.Equal(t, "4", result[0].ExtraInfo["index"])
		assert.Equal(t, "1", result[1].ExtraInfo["index"])
	})

	// Test filtering by name "dmesg"
	t.Run("filter_dmesg", func(t *testing.T) {
		result, err := bucketV2.Read(ctx, WithName("dmesg"))
		assert.NoError(t, err)
		assert.Equal(t, 1, len(result), "Should return 1 dmesg event")
		assert.Equal(t, "dmesg", result[0].Name)
		assert.Equal(t, "2", result[0].ExtraInfo["index"])
	})

	// Test filtering with non-existent name
	t.Run("filter_nonexistent", func(t *testing.T) {
		result, err := bucketV2.Read(ctx, WithName("nonexistent"))
		assert.NoError(t, err)
		assert.Nil(t, result, "Should return nil for non-existent event name")
	})

	// Test combined filtering
	t.Run("filter_combined", func(t *testing.T) {
		result, err := bucketV2.Read(ctx,
			WithSince(baseTime.Add(2*time.Second)),
			WithName("syslog"))
		assert.NoError(t, err)
		assert.Equal(t, 1, len(result), "Should return 1 recent syslog event")
		assert.Equal(t, "syslog", result[0].Name)
		assert.Equal(t, "4", result[0].ExtraInfo["index"])
	})
}

// TestReadEmptyResults tests the Read method with empty results
func TestReadEmptyResults(t *testing.T) {
	t.Parallel()

	testTableName := "test_table"

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	storeV2, err := NewV2(dbRW, dbRO, 0)
	assert.NoError(t, err)
	bucketV2, err := storeV2.BucketV2(testTableName)
	assert.NoError(t, err)
	defer bucketV2.Close()

	// Test reading from empty table
	t.Run("empty_table", func(t *testing.T) {
		result, err := bucketV2.Read(ctx)
		assert.NoError(t, err)
		assert.Nil(t, result, "Should return nil for empty table")
	})

	// Insert an event
	baseTime := time.Now().UTC()
	event := Event{
		Time:    baseTime,
		Name:    "test_event",
		Type:    string(apiv1.EventTypeWarning),
		Message: "Test event",
	}
	err = bucketV2.Insert(ctx, event)
	assert.NoError(t, err)

	// Test reading with future timestamp
	t.Run("future_timestamp", func(t *testing.T) {
		result, err := bucketV2.Read(ctx, WithSince(baseTime.Add(time.Hour)))
		assert.NoError(t, err)
		assert.Nil(t, result, "Should return nil for future timestamp")
	})

	// Test reading with non-existent event name
	t.Run("nonexistent_name", func(t *testing.T) {
		result, err := bucketV2.Read(ctx, WithName("nonexistent"))
		assert.NoError(t, err)
		assert.Nil(t, result, "Should return nil for non-existent event name")
	})

	// Test reading with both filters that match nothing
	t.Run("no_match_combined", func(t *testing.T) {
		result, err := bucketV2.Read(ctx,
			WithSince(baseTime.Add(time.Hour)),
			WithName("test_event"))
		assert.NoError(t, err)
		assert.Nil(t, result, "Should return nil when no events match both filters")
	})
}

// TestReadConcurrency tests concurrent reads with different filters
func TestReadConcurrency(t *testing.T) {
	t.Parallel()

	testTableName := "test_table"

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	storeV2, err := NewV2(dbRW, dbRO, 0)
	assert.NoError(t, err)
	bucketV2, err := storeV2.BucketV2(testTableName)
	assert.NoError(t, err)
	defer bucketV2.Close()

	baseTime := time.Now().UTC()
	eventCount := 100

	// Insert events with different names
	for i := 0; i < eventCount; i++ {
		name := "event_a"
		if i%2 == 0 {
			name = "event_b"
		}
		event := Event{
			Time:    baseTime.Add(time.Duration(i) * time.Second),
			Name:    name,
			Type:    string(apiv1.EventTypeWarning),
			Message: fmt.Sprintf("Event %d", i),
			ExtraInfo: map[string]string{
				"id": fmt.Sprintf("%d", i),
			},
		}
		err = bucketV2.Insert(ctx, event)
		assert.NoError(t, err)
	}

	var wg sync.WaitGroup
	errors := make(chan error, 10)

	// Concurrent reads with different filters
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()

			var result Events
			var err error

			switch index % 4 {
			case 0:
				// Read all events
				result, err = bucketV2.Read(ctx)
				if err == nil && len(result) != eventCount {
					errors <- fmt.Errorf("expected %d events, got %d", eventCount, len(result))
				}
			case 1:
				// Read with name filter
				result, err = bucketV2.Read(ctx, WithName("event_a"))
				if err == nil && len(result) != 50 {
					errors <- fmt.Errorf("expected 50 event_a, got %d", len(result))
				}
			case 2:
				// Read with time filter - events after 50 seconds (50-99)
				result, err = bucketV2.Read(ctx, WithSince(baseTime.Add(50*time.Second)))
				if err == nil && len(result) != 49 {
					errors <- fmt.Errorf("expected 49 recent events, got %d", len(result))
				}
			case 3:
				// Read with combined filters
				result, err = bucketV2.Read(ctx,
					WithSince(baseTime.Add(50*time.Second)),
					WithName("event_b"))
				// Events 51-99 (49 total), only even numbered ones (event_b): 52,54,56...98 = 24
				expectedCount := 24
				if err == nil && len(result) != expectedCount {
					errors <- fmt.Errorf("expected %d filtered events, got %d", expectedCount, len(result))
				}
			}

			if err != nil {
				errors <- err
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	// Check for any errors
	for err := range errors {
		assert.NoError(t, err)
	}
}

// TestBucketWithInvalidOptions tests error handling in Bucket creation
func TestBucketWithInvalidOptions(t *testing.T) {
	t.Parallel()

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	store, err := New(dbRW, dbRO, 0)
	assert.NoError(t, err)

	// Test with invalid option that causes applyOpts to fail
	// Since OpOption is not exposed in this test, we can't directly test this
	// But we can test the disablePurge path
	bucket, err := store.Bucket("test_table", WithDisablePurge())
	assert.NoError(t, err)
	assert.NotNil(t, bucket)
	defer bucket.Close()
}

// TestReadWithInvalidOptions tests error handling for Read with bad options
func TestReadWithInvalidOptions(t *testing.T) {
	t.Parallel()

	testTableName := "test_table"

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	storeV2, err := NewV2(dbRW, dbRO, 0)
	assert.NoError(t, err)
	bucketV2, err := storeV2.BucketV2(testTableName)
	assert.NoError(t, err)
	defer bucketV2.Close()

	// Since we can't easily create an invalid OpOption,
	// we test the normal operation path
	result, err := bucketV2.Read(ctx)
	assert.NoError(t, err)
	assert.Nil(t, result) // empty table returns nil
}

// TestFindEventWithMessage tests findEvent with message parameter
func TestFindEventWithMessage(t *testing.T) {
	t.Parallel()

	testTableName := "test_table"

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	store, err := New(dbRW, dbRO, 0)
	assert.NoError(t, err)
	bucket, err := store.Bucket(testTableName)
	assert.NoError(t, err)
	defer bucket.Close()

	baseTime := time.Now().UTC()

	// Insert events with different messages
	event1 := Event{
		Time:    baseTime,
		Name:    "test",
		Type:    string(apiv1.EventTypeWarning),
		Message: "First message",
	}
	event2 := Event{
		Time:    baseTime,
		Name:    "test",
		Type:    string(apiv1.EventTypeWarning),
		Message: "Second message",
	}

	err = bucket.Insert(ctx, event1)
	assert.NoError(t, err)
	err = bucket.Insert(ctx, event2)
	assert.NoError(t, err)

	// Find with specific message
	found, err := bucket.Find(ctx, event1)
	assert.NoError(t, err)
	assert.NotNil(t, found)
	assert.Equal(t, "First message", found.Message)

	// Find with different message
	found, err = bucket.Find(ctx, event2)
	assert.NoError(t, err)
	assert.NotNil(t, found)
	assert.Equal(t, "Second message", found.Message)
}

// TestRunPurgeErrors tests error handling in runPurge
func TestRunPurgeErrors(t *testing.T) {
	t.Parallel()

	testTableName := "test_table"

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	// Create a table with very short retention and purge interval
	bucket, err := newTable(
		dbRW,
		dbRO,
		testTableName,
		2*time.Second,        // Very short retention
		100*time.Millisecond, // Very short interval
	)
	assert.NoError(t, err)

	// Insert an event
	ctx := context.Background()
	event := Event{
		Time: time.Now().UTC().Add(-10 * time.Second), // Old event
		Name: "test",
		Type: string(apiv1.EventTypeWarning),
	}
	err = bucket.Insert(ctx, event)
	assert.NoError(t, err)

	// Wait for purge to run
	time.Sleep(300 * time.Millisecond)

	// Close the bucket to stop the purge goroutine
	bucket.Close()

	// Verify event was purged
	events, err := bucket.Get(ctx, time.Now().Add(-time.Hour))
	assert.NoError(t, err)
	assert.Empty(t, events)
}
