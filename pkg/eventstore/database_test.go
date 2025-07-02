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
	"golang.org/x/time/rate"

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
		rate.NewLimiter(rate.Inf, 0), // no rate limiting for tests
		false,                        // rateLimitNoWait parameter
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

func TestRateLimitBlocking(t *testing.T) {
	t.Parallel()

	testTableName := "test_table_rate_limit_blocking"

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	store, err := New(dbRW, dbRO, 0)
	assert.NoError(t, err)

	// Configure rate limiting: 2 events per second (blocking by default)
	bucket, err := store.Bucket(testTableName, WithIngestRateLimit(2, time.Second))
	assert.NoError(t, err)
	defer bucket.Close()

	event := Event{
		Time:      time.Now().UTC(),
		Name:      "test_event",
		Type:      string(apiv1.EventTypeWarning),
		Message:   "test message for rate limiting",
		ExtraInfo: map[string]string{"test": "blocking"},
	}

	// First 2 events should succeed immediately
	start := time.Now()
	assert.NoError(t, bucket.Insert(ctx, event))
	assert.NoError(t, bucket.Insert(ctx, event))

	// Third event should block and succeed after rate limit window
	assert.NoError(t, bucket.Insert(ctx, event))
	elapsed := time.Since(start)

	// Should have taken at least 500ms (rate limit window)
	assert.True(t, elapsed >= 400*time.Millisecond, "Expected blocking behavior to wait for rate limit, took %v", elapsed)
}

func TestRateLimitNonBlocking(t *testing.T) {
	t.Parallel()

	testTableName := "test_table_rate_limit_nonblocking"

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	store, err := New(dbRW, dbRO, 0)
	assert.NoError(t, err)

	// Configure rate limiting: 2 events per second with no-wait option
	bucket, err := store.Bucket(testTableName,
		WithIngestRateLimit(2, time.Second),
		WithIngestRateLimitNoWait())
	assert.NoError(t, err)
	defer bucket.Close()

	event := Event{
		Time:      time.Now().UTC(),
		Name:      "test_event",
		Type:      string(apiv1.EventTypeWarning),
		Message:   "test message for rate limiting",
		ExtraInfo: map[string]string{"test": "nonblocking"},
	}

	// First 2 events should succeed immediately
	start := time.Now()
	assert.NoError(t, bucket.Insert(ctx, event))
	assert.NoError(t, bucket.Insert(ctx, event))

	// Third event should fail immediately with ErrRateLimitExceeded
	err = bucket.Insert(ctx, event)
	elapsed := time.Since(start)

	assert.Error(t, err)
	assert.Equal(t, ErrRateLimitExceeded, err)
	// Should return immediately, not block
	assert.True(t, elapsed < 100*time.Millisecond, "Expected immediate failure, took %v", elapsed)
}

func TestRateLimitNonBlockingConcurrent(t *testing.T) {
	t.Parallel()

	testTableName := "test_table_rate_limit_concurrent"

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	store, err := New(dbRW, dbRO, 0)
	assert.NoError(t, err)

	// Configure rate limiting: 5 events per second with no-wait
	bucket, err := store.Bucket(testTableName,
		WithIngestRateLimit(5, time.Second),
		WithIngestRateLimitNoWait())
	assert.NoError(t, err)
	defer bucket.Close()

	event := Event{
		Time:      time.Now().UTC(),
		Name:      "concurrent_test",
		Type:      string(apiv1.EventTypeInfo),
		Message:   "concurrent rate limit test",
		ExtraInfo: map[string]string{"test": "concurrent"},
	}

	const numGoroutines = 10
	const eventsPerGoroutine = 3

	var wg sync.WaitGroup
	var mu sync.Mutex
	var successCount, errorCount int

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < eventsPerGoroutine; j++ {
				err := bucket.Insert(ctx, event)
				mu.Lock()
				if err == nil {
					successCount++
				} else if err == ErrRateLimitExceeded {
					errorCount++
				} else {
					t.Errorf("Unexpected error: %v", err)
				}
				mu.Unlock()
			}
		}()
	}

	wg.Wait()

	// With 5 events/second limit and 30 total events, we should have some successes and some rate limit errors
	assert.True(t, successCount > 0, "Expected some successful inserts")
	assert.True(t, errorCount > 0, "Expected some rate limit errors")
	assert.Equal(t, numGoroutines*eventsPerGoroutine, successCount+errorCount, "Total events should match")
	t.Logf("Successful inserts: %d, Rate limited: %d", successCount, errorCount)
}

func TestRateLimitBlockingBehavior(t *testing.T) {
	t.Parallel()

	testTableName := "test_table_rate_limit_behavior"

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	store, err := New(dbRW, dbRO, 0)
	assert.NoError(t, err)

	// Configure rate limiting: 1 event per second (blocking)
	bucket, err := store.Bucket(testTableName, WithIngestRateLimit(1, time.Second))
	assert.NoError(t, err)
	defer bucket.Close()

	event := Event{
		Time:      time.Now().UTC(),
		Name:      "behavior_test",
		Type:      string(apiv1.EventTypeInfo),
		Message:   "testing blocking behavior",
		ExtraInfo: map[string]string{"test": "behavior"},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// First event should succeed immediately (using the burst token)
	start := time.Now()
	assert.NoError(t, bucket.Insert(ctx, event))

	// Second event should block and succeed after ~1 second
	assert.NoError(t, bucket.Insert(ctx, event))
	elapsed := time.Since(start)

	// Should have taken at least 900ms due to rate limiting
	assert.True(t, elapsed >= 900*time.Millisecond,
		"Expected blocking behavior to wait for rate limit, took %v", elapsed)
	assert.True(t, elapsed < 2*time.Second,
		"Expected to complete within reasonable time, took %v", elapsed)
}

func TestNoRateLimit(t *testing.T) {
	t.Parallel()

	testTableName := "test_table_no_rate_limit"

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	store, err := New(dbRW, dbRO, 0)
	assert.NoError(t, err)

	// Create bucket without rate limiting
	bucket, err := store.Bucket(testTableName)
	assert.NoError(t, err)
	defer bucket.Close()

	event := Event{
		Time:      time.Now().UTC(),
		Name:      "no_limit_test",
		Type:      string(apiv1.EventTypeInfo),
		Message:   "testing without rate limit",
		ExtraInfo: map[string]string{"test": "unlimited"},
	}

	// Should be able to insert many events quickly without rate limiting
	start := time.Now()
	for i := 0; i < 100; i++ {
		assert.NoError(t, bucket.Insert(ctx, event))
	}
	elapsed := time.Since(start)

	// Should complete very quickly without rate limiting
	assert.True(t, elapsed < 1*time.Second, "Expected fast insertion without rate limiting, took %v", elapsed)
}

func TestRateLimitBlockingWithAlreadyCanceledContext(t *testing.T) {
	t.Parallel()

	testTableName := "test_table_canceled_context"

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	store, err := New(dbRW, dbRO, 0)
	assert.NoError(t, err)

	// Configure rate limiting: 1 event per second (blocking)
	bucket, err := store.Bucket(testTableName, WithIngestRateLimit(1, time.Second))
	assert.NoError(t, err)
	defer bucket.Close()

	event := Event{
		Time:      time.Now().UTC(),
		Name:      "canceled_context_test",
		Type:      string(apiv1.EventTypeWarning),
		Message:   "testing with already canceled context",
		ExtraInfo: map[string]string{"test": "canceled"},
	}

	// First event should succeed with valid context
	ctx := context.Background()
	assert.NoError(t, bucket.Insert(ctx, event))

	// Create an already-canceled context
	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	// Second event should fail immediately with canceled context
	start := time.Now()
	err = bucket.Insert(canceledCtx, event)
	elapsed := time.Since(start)

	assert.Error(t, err)
	assert.Equal(t, context.Canceled, err)
	// Should return immediately, not wait for rate limit
	assert.True(t, elapsed < 50*time.Millisecond,
		"Expected immediate failure with canceled context, took %v", elapsed)
}

func TestRateLimitBlockingWithContextCanceledDuringWait(t *testing.T) {
	t.Parallel()

	testTableName := "test_table_cancel_during_wait"

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	store, err := New(dbRW, dbRO, 0)
	assert.NoError(t, err)

	// Configure rate limiting: 2 events per second (blocking)
	bucket, err := store.Bucket(testTableName, WithIngestRateLimit(2, time.Second))
	assert.NoError(t, err)
	defer bucket.Close()

	event := Event{
		Time:      time.Now().UTC(),
		Name:      "cancel_during_wait_test",
		Type:      string(apiv1.EventTypeInfo),
		Message:   "testing context cancellation during rate limit wait",
		ExtraInfo: map[string]string{"test": "cancel-during-wait"},
	}

	// Use up the burst of 2 events
	ctx := context.Background()
	assert.NoError(t, bucket.Insert(ctx, event))
	assert.NoError(t, bucket.Insert(ctx, event))

	// Create a cancellable context
	cancelCtx, cancel := context.WithCancel(context.Background())

	// Start a goroutine to cancel the context after a short delay
	go func() {
		time.Sleep(200 * time.Millisecond)
		cancel()
	}()

	// Third event should block and then fail when context is canceled
	start := time.Now()
	err = bucket.Insert(cancelCtx, event)
	elapsed := time.Since(start)

	assert.Error(t, err)
	assert.Equal(t, context.Canceled, err)
	// Should fail around 200ms when context is canceled
	assert.True(t, elapsed >= 150*time.Millisecond && elapsed <= 350*time.Millisecond,
		"Expected cancellation around 200ms, took %v", elapsed)
}

func TestRateLimitBlockingWithDeadlineExceeded(t *testing.T) {
	t.Parallel()

	testTableName := "test_table_deadline_exceeded"

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	store, err := New(dbRW, dbRO, 0)
	assert.NoError(t, err)

	// Configure rate limiting: 1 event per 2 seconds (very slow)
	bucket, err := store.Bucket(testTableName, WithIngestRateLimit(1, 2*time.Second))
	assert.NoError(t, err)
	defer bucket.Close()

	event := Event{
		Time:      time.Now().UTC(),
		Name:      "deadline_test",
		Type:      string(apiv1.EventTypeCritical),
		Message:   "testing deadline exceeded",
		ExtraInfo: map[string]string{"test": "deadline"},
	}

	// First event uses the burst token
	ctx := context.Background()
	assert.NoError(t, bucket.Insert(ctx, event))

	// Second event with a deadline that will expire before rate limit allows
	deadlineCtx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	start := time.Now()
	err = bucket.Insert(deadlineCtx, event)
	elapsed := time.Since(start)

	assert.Error(t, err)
	// The rate limiter may return its own error when it detects context deadline will be exceeded
	assert.True(t, err == context.DeadlineExceeded || strings.Contains(err.Error(), "exceed context deadline"),
		"Expected deadline-related error, got %v", err)
	// Rate limiter may detect deadline will be exceeded before waiting, so it could be fast
	assert.True(t, elapsed <= 400*time.Millisecond,
		"Expected reasonable timing, took %v", elapsed)
}

func TestRateLimitNonBlockingWithCanceledContext(t *testing.T) {
	t.Parallel()

	testTableName := "test_table_nonblocking_canceled"

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	store, err := New(dbRW, dbRO, 0)
	assert.NoError(t, err)

	// Configure rate limiting with no-wait option
	bucket, err := store.Bucket(testTableName,
		WithIngestRateLimit(1, time.Second),
		WithIngestRateLimitNoWait())
	assert.NoError(t, err)
	defer bucket.Close()

	event := Event{
		Time:      time.Now().UTC(),
		Name:      "nonblocking_canceled_test",
		Type:      string(apiv1.EventTypeInfo),
		Message:   "testing non-blocking with canceled context",
		ExtraInfo: map[string]string{"test": "nonblocking-canceled"},
	}

	// First event should succeed
	ctx := context.Background()
	assert.NoError(t, bucket.Insert(ctx, event))

	// Create a canceled context
	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel()

	// Second event should fail with rate limit (not context error)
	// because non-blocking checks rate limit before context
	err = bucket.Insert(canceledCtx, event)
	assert.Error(t, err)
	assert.Equal(t, ErrRateLimitExceeded, err)
}

func TestRateLimitBlockingContextPropagation(t *testing.T) {
	t.Parallel()

	testTableName := "test_table_context_propagation"

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	store, err := New(dbRW, dbRO, 0)
	assert.NoError(t, err)

	// Configure rate limiting
	bucket, err := store.Bucket(testTableName, WithIngestRateLimit(1, time.Second))
	assert.NoError(t, err)
	defer bucket.Close()

	event := Event{
		Time:      time.Now().UTC(),
		Name:      "context_propagation_test",
		Type:      string(apiv1.EventTypeWarning),
		Message:   "testing context propagation",
		ExtraInfo: map[string]string{"test": "propagation"},
	}

	// Create a parent context with value
	type ctxKey string
	const testKey ctxKey = "test-key"
	parentCtx := context.WithValue(context.Background(), testKey, "test-value")

	// Use up the burst token
	assert.NoError(t, bucket.Insert(parentCtx, event))

	// Create child context with timeout
	childCtx, cancel := context.WithTimeout(parentCtx, 300*time.Millisecond)
	defer cancel()

	// Verify context value is propagated
	assert.Equal(t, "test-value", childCtx.Value(testKey))

	// Insert should fail with deadline exceeded
	start := time.Now()
	err = bucket.Insert(childCtx, event)
	elapsed := time.Since(start)

	assert.Error(t, err)
	// The rate limiter may return its own error when it detects context deadline will be exceeded
	assert.True(t, err == context.DeadlineExceeded || strings.Contains(err.Error(), "exceed context deadline"),
		"Expected deadline-related error, got %v", err)
	// Rate limiter may detect deadline will be exceeded before waiting, so it could be fast
	assert.True(t, elapsed <= 400*time.Millisecond,
		"Expected reasonable timing, took %v", elapsed)
}

func TestRateLimitMultipleContextCancellations(t *testing.T) {
	t.Parallel()

	testTableName := "test_table_multiple_cancellations"

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	store, err := New(dbRW, dbRO, 0)
	assert.NoError(t, err)

	// Configure rate limiting: 3 events per second
	bucket, err := store.Bucket(testTableName, WithIngestRateLimit(3, time.Second))
	assert.NoError(t, err)
	defer bucket.Close()

	event := Event{
		Time:      time.Now().UTC(),
		Name:      "multiple_cancel_test",
		Type:      string(apiv1.EventTypeInfo),
		Message:   "testing multiple context cancellations",
		ExtraInfo: map[string]string{"test": "multiple"},
	}

	// Use up the burst of 3 events
	ctx := context.Background()
	assert.NoError(t, bucket.Insert(ctx, event))
	assert.NoError(t, bucket.Insert(ctx, event))
	assert.NoError(t, bucket.Insert(ctx, event))

	// Test multiple concurrent inserts with different context cancellation timings
	var wg sync.WaitGroup
	results := make([]error, 3)
	timings := []time.Duration{100 * time.Millisecond, 200 * time.Millisecond, 300 * time.Millisecond}

	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), timings[idx])
			defer cancel()
			results[idx] = bucket.Insert(ctx, event)
		}(i)
	}

	wg.Wait()

	// All should have failed with context-related errors
	for i, err := range results {
		assert.Error(t, err, "Insert %d should have failed", i)
		// Rate limiter may return its own error message when detecting context issues
		assert.True(t, err == context.DeadlineExceeded || err == context.Canceled ||
			strings.Contains(err.Error(), "exceed context deadline"),
			"Insert %d should have context-related error, got %v", i, err)
	}
}
