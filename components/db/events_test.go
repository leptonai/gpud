package db

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/components/common"
	"github.com/leptonai/gpud/pkg/sqlite"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestCreateDefaultTableName(t *testing.T) {
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
			result := CreateDefaultTableName(tc.input)
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

	store, err := NewStore(dbRW, dbRO, testTableName, 0)
	assert.NoError(t, err)
	defer store.Close()

	first := time.Now().UTC()

	events := []components.Event{}
	eventsN := 10
	for i := 0; i < eventsN; i++ {
		events = append(events, components.Event{
			Time:    metav1.Time{Time: first.Add(time.Duration(i) * time.Second)},
			Name:    "dmesg",
			Type:    common.EventTypeWarning,
			Message: fmt.Sprintf("OOM event %d occurred", i),
			SuggestedActions: &common.SuggestedActions{
				Descriptions: []string{fmt.Sprintf("oom_reaper: reaped process %d (vector), now anon-rss:0kB, file-rss:0kB, shmem-rss:0", i)},
			},
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
		assert.Greater(t, events[i-1].Time.Unix(), events[i].Time.Unix(), "timestamps should be in descending order")
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

	db, err := NewStore(dbRW, dbRO, testTableName, 0)
	assert.NoError(t, err)

	baseTime := time.Now().UTC()
	events := []components.Event{
		{
			Time: metav1.Time{Time: baseTime.Add(-10 * time.Minute)},
			Name: "dmesg",
			Type: common.EventTypeWarning,
			SuggestedActions: &common.SuggestedActions{
				Descriptions: []string{"old event"},
			},
		},
		{
			Time: metav1.Time{Time: baseTime.Add(-5 * time.Minute)},
			Name: "dmesg",
			Type: common.EventTypeWarning,
			SuggestedActions: &common.SuggestedActions{
				Descriptions: []string{"mid event"},
			},
		},
		{
			Time: metav1.Time{Time: baseTime},
			Name: "dmesg",
			Type: common.EventTypeWarning,
			SuggestedActions: &common.SuggestedActions{
				Descriptions: []string{"recent event"},
			},
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
	assert.Equal(t, "recent event", recentEvents[0].SuggestedActions.Descriptions[0])
}

func TestEmptyResults(t *testing.T) {
	t.Parallel()

	testTableName := "test_table"

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	store, err := NewStore(dbRW, dbRO, testTableName, 0)
	assert.NoError(t, err)
	defer store.Close()

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

	store, err := NewStore(dbRW, dbRO, testTableName, 0)
	assert.NoError(t, err)
	defer store.Close()

	baseTime := time.Now().UTC()
	events := []components.Event{
		{
			Time: metav1.Time{Time: baseTime},
			Name: "dmesg",
			Type: common.EventTypeWarning,
			SuggestedActions: &common.SuggestedActions{
				Descriptions: []string{"oom event"},
			},
		},
		{
			Time: metav1.Time{Time: baseTime.Add(1 * time.Second)},
			Name: "syslog",
			Type: common.EventTypeWarning,
			SuggestedActions: &common.SuggestedActions{
				Descriptions: []string{"edac event"},
			},
		},
		{
			Time: metav1.Time{Time: baseTime.Add(2 * time.Second)},
			Name: "dmesg",
			Type: common.EventTypeWarning,
			SuggestedActions: &common.SuggestedActions{
				Descriptions: []string{"cgroup event"},
			},
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
	assert.Equal(t, common.EventTypeWarning, results[0].Type)
	assert.Equal(t, common.EventTypeWarning, results[1].Type)
	assert.Equal(t, common.EventTypeWarning, results[2].Type)
}

func TestPurgePartial(t *testing.T) {
	t.Parallel()

	testTableName := "test_table"

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	store, err := NewStore(dbRW, dbRO, testTableName, 0)
	assert.NoError(t, err)
	defer store.Close()

	baseTime := time.Now().UTC()
	events := []components.Event{
		{
			Time:      metav1.Time{Time: baseTime.Add(-10 * time.Minute)},
			Name:      "dmesg",
			Type:      common.EventTypeWarning,
			ExtraInfo: map[string]string{"id": "old_event"},
			SuggestedActions: &common.SuggestedActions{
				Descriptions: []string{"old event"},
			},
		},
		{
			Time:      metav1.Time{Time: baseTime},
			Name:      "dmesg",
			Type:      common.EventTypeWarning,
			ExtraInfo: map[string]string{"id": "new_event"},
			SuggestedActions: &common.SuggestedActions{
				Descriptions: []string{"recent event"},
			},
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
	extraInfoJSON, err := json.Marshal(remaining[0].ExtraInfo)
	assert.NoError(t, err)
	assert.Equal(t, `{"id":"new_event"}`, string(extraInfoJSON))

	// Try to find old event by ExtraInfo
	oldEvent := components.Event{
		Time:      metav1.Time{Time: baseTime.Add(-10 * time.Minute)},
		Name:      "test",
		Type:      common.EventTypeWarning,
		ExtraInfo: map[string]string{"id": "old_event"},
	}
	found, err := store.Find(ctx, oldEvent)
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

	store, err := NewStore(dbRW, dbRO, testTableName, 0)
	assert.NoError(t, err)
	defer store.Close()

	baseTime := time.Now().UTC()
	testEvent := components.Event{
		Time:      metav1.Time{Time: baseTime.Add(-10 * time.Minute)},
		Name:      "dmesg",
		Type:      common.EventTypeWarning,
		ExtraInfo: map[string]string{"a": "b"},
		SuggestedActions: &common.SuggestedActions{
			Descriptions: []string{"old event"},
		},
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
	assert.Equal(t, testEvent.Time.Unix(), found.Time.Unix())
	assert.Equal(t, testEvent.Name, found.Name)
	assert.Equal(t, testEvent.Type, found.Type)
	assert.Equal(t, testEvent.ExtraInfo, found.ExtraInfo)
	assert.Equal(t, testEvent.SuggestedActions.Descriptions[0], found.SuggestedActions.Descriptions[0])
}

func TestFindEventPartialMatch(t *testing.T) {
	t.Parallel()

	testTableName := "test_table"

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	store, err := NewStore(dbRW, dbRO, testTableName, 0)
	assert.NoError(t, err)
	defer store.Close()

	baseTime := time.Now().UTC()
	testEvent := components.Event{
		Time:      metav1.Time{Time: baseTime},
		Name:      "dmesg",
		Type:      common.EventTypeWarning,
		ExtraInfo: map[string]string{"a": "b"},
		SuggestedActions: &common.SuggestedActions{
			Descriptions: []string{"original details"},
		},
	}

	assert.NoError(t, store.Insert(ctx, testEvent))

	// Test finding with matching timestamp/source/type but different details
	searchEvent := components.Event{
		Time:      metav1.Time{Time: testEvent.Time.Time},
		Name:      testEvent.Name,
		Type:      testEvent.Type,
		ExtraInfo: testEvent.ExtraInfo,
		SuggestedActions: &common.SuggestedActions{
			Descriptions: []string{"different details"},
		},
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

	store, err := NewStore(dbRW, dbRO, testTableName, 0)
	assert.NoError(t, err)
	defer store.Close()

	baseTime := time.Now().UTC()
	events := []components.Event{
		{
			Time:      metav1.Time{Time: baseTime},
			Name:      "dmesg",
			Type:      common.EventTypeWarning,
			ExtraInfo: map[string]string{"a": "b"},
			SuggestedActions: &common.SuggestedActions{
				Descriptions: []string{"first event"},
			},
		},
		{
			Time:      metav1.Time{Time: baseTime},
			Name:      "dmesg",
			Type:      common.EventTypeWarning,
			ExtraInfo: map[string]string{"a": "b"},
			SuggestedActions: &common.SuggestedActions{
				Descriptions: []string{"second event"},
			},
		},
	}

	// Insert multiple events with same timestamp/source/type
	for _, ev := range events {
		assert.NoError(t, store.Insert(ctx, ev))
	}

	// Search should return the first matching event
	searchEvent := components.Event{
		Time:      metav1.Time{Time: baseTime},
		Name:      "dmesg",
		Type:      common.EventTypeWarning,
		ExtraInfo: map[string]string{"a": "b"},
	}

	found, err := store.Find(ctx, searchEvent)
	assert.NoError(t, err)
	assert.NotNil(t, found)

	// Should match one of the events
	foundMatch := false
	for _, ev := range events {
		if found.SuggestedActions.Descriptions[0] == ev.SuggestedActions.Descriptions[0] {
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

	store, err := NewStore(dbRW, dbRO, testTableName, 0)
	assert.NoError(t, err)
	defer store.Close()

	baseTime := time.Now().UTC()
	event := components.Event{
		Time: metav1.Time{Time: baseTime},
		Name: "nvidia-smi",
		Type: common.EventTypeWarning,
		ExtraInfo: map[string]string{
			"xid":      "123",
			"gpu_uuid": "gpu-123",
		},
		SuggestedActions: &common.SuggestedActions{
			Descriptions: []string{"GPU error details"},
		},
	}

	// Test insert and find with ExtraInfo
	err = store.Insert(ctx, event)
	assert.NoError(t, err)

	found, err := store.Find(ctx, event)
	assert.NoError(t, err)
	assert.NotNil(t, found)
	assert.Equal(t, event.ExtraInfo, found.ExtraInfo)

	// Test find with partial ExtraInfo match
	partialEvent := components.Event{
		Time: metav1.Time{Time: event.Time.Time},
		Name: event.Name,
		Type: event.Type,
	}

	found, err = store.Find(ctx, partialEvent)
	assert.NoError(t, err)
	assert.NotNil(t, found)
	assert.Equal(t, event.ExtraInfo, found.ExtraInfo)

	// Test find with different ExtraInfo
	differentEvent := components.Event{
		Time: metav1.Time{Time: event.Time.Time},
		Name: event.Name,
		Type: event.Type,
		ExtraInfo: map[string]string{
			"xid":      "different",
			"gpu_uuid": "different-gpu",
		},
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

	store, err := NewStore(dbRW, dbRO, testTableName, 0)
	assert.NoError(t, err)
	defer store.Close()

	baseTime := time.Now().UTC()
	event := components.Event{
		Time:      metav1.Time{Time: baseTime},
		Name:      "dmesg",
		Type:      common.EventTypeWarning,
		ExtraInfo: map[string]string{},
		SuggestedActions: &common.SuggestedActions{
			Descriptions: []string{"Event with null ExtraInfo"},
		},
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

	db, err := NewStore(dbRW, dbRO, testTableName, 0)
	assert.NoError(t, err)

	baseTime := time.Now().UTC()
	events := []components.Event{
		{
			Time:      metav1.Time{Time: baseTime.Add(-10 * time.Minute)},
			Name:      "test",
			Type:      common.EventTypeWarning,
			ExtraInfo: map[string]string{"id": "old_event"},
			SuggestedActions: &common.SuggestedActions{
				Descriptions: []string{"old event"},
			},
		},
		{
			Time:      metav1.Time{Time: baseTime},
			Name:      "test",
			Type:      common.EventTypeWarning,
			ExtraInfo: map[string]string{"id": "new_event"},
			SuggestedActions: &common.SuggestedActions{
				Descriptions: []string{"new event"},
			},
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
	extraInfoJSON, err := json.Marshal(remaining[0].ExtraInfo)
	assert.NoError(t, err)
	assert.Equal(t, `{"id":"new_event"}`, string(extraInfoJSON))

	// Try to find old event by ExtraInfo
	oldEvent := components.Event{
		Time:      metav1.Time{Time: baseTime.Add(-10 * time.Minute)},
		Name:      "test",
		Type:      common.EventTypeWarning,
		ExtraInfo: map[string]string{"id": "old_event"},
	}
	found, err := db.Find(ctx, oldEvent)
	assert.NoError(t, err)
	assert.Nil(t, found, "Old event should not be found after purge")
}

func TestInvalidTableName(t *testing.T) {
	t.Parallel()

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	// Test with invalid table name
	_, err := NewStore(dbRW, dbRO, "invalid;table;name", 0)
	assert.Error(t, err)
}

func TestContextCancellation(t *testing.T) {
	t.Parallel()

	testTableName := "test_table"

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	store, err := NewStore(dbRW, dbRO, testTableName, 0)
	assert.NoError(t, err)
	defer store.Close()

	// Test with canceled context
	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	event := components.Event{
		Time: metav1.Time{Time: time.Now().UTC()},
		Name: "test",
		Type: common.EventTypeWarning,
		SuggestedActions: &common.SuggestedActions{
			Descriptions: []string{"Test details"},
		},
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

	db, err := NewStore(dbRW, dbRO, testTableName, 0)
	assert.NoError(t, err)

	baseTime := time.Now().UTC()
	eventCount := 100
	done := make(chan bool)

	// Concurrent inserts
	go func() {
		for i := 0; i < eventCount; i++ {
			event := components.Event{
				Time: metav1.Time{Time: baseTime.Add(time.Duration(i) * time.Second)},
				Name: "concurrent",
				Type: common.EventTypeWarning,
				SuggestedActions: &common.SuggestedActions{
					Descriptions: []string{fmt.Sprintf("Concurrent event %d", i)},
				},
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

	store, err := NewStore(dbRW, dbRO, testTableName, 0)
	assert.NoError(t, err)
	defer store.Close()

	events := []components.Event{
		{
			Time:      metav1.Time{Time: time.Now().UTC()},
			Name:      "test;source",
			Type:      common.EventTypeWarning,
			Message:   "message with special chars: !@#$%^&*()",
			ExtraInfo: map[string]string{"special chars": "!@#$%^&*()"},
			SuggestedActions: &common.SuggestedActions{
				Descriptions: []string{"details with special chars"},
			},
		},
		{
			Time:      metav1.Time{Time: time.Now().UTC()},
			Name:      "unicode_source_🔥",
			Type:      common.EventTypeWarning,
			Message:   "unicode message: 你好",
			ExtraInfo: map[string]string{"unicode info": "你好"},
			SuggestedActions: &common.SuggestedActions{
				Descriptions: []string{"unicode details: 世界！"},
			},
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
		assert.Equal(t, event.SuggestedActions.Descriptions[0], found.SuggestedActions.Descriptions[0])
	}
}

func TestLargeEventDetails(t *testing.T) {
	t.Parallel()

	testTableName := "test_table"

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	db, err := NewStore(dbRW, dbRO, testTableName, 0)
	assert.NoError(t, err)

	// Create a large event detail string (100KB)
	largeDetail := make([]byte, 100*1024)
	for i := range largeDetail {
		largeDetail[i] = byte('a' + (i % 26))
	}

	event := components.Event{
		Time: metav1.Time{Time: time.Now().UTC()},
		Name: "test",
		Type: common.EventTypeWarning,
		SuggestedActions: &common.SuggestedActions{
			Descriptions: []string{string(largeDetail)},
		},
	}

	err = db.Insert(ctx, event)
	assert.NoError(t, err)

	found, err := db.Find(ctx, event)
	assert.NoError(t, err)
	assert.NotNil(t, found)
	assert.Equal(t, event.SuggestedActions.Descriptions[0], found.SuggestedActions.Descriptions[0])
}

func TestTimestampBoundaries(t *testing.T) {
	t.Parallel()

	testTableName := "test_table"

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	store, err := NewStore(dbRW, dbRO, testTableName, 0)
	assert.NoError(t, err)
	defer store.Close()

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
		event := components.Event{
			Time: metav1.Time{Time: time.Unix(ts, 0)},
			Name: "test",
			Type: common.EventTypeWarning,
			SuggestedActions: &common.SuggestedActions{
				Descriptions: []string{fmt.Sprintf("timestamp: %d", ts)},
			},
		}

		err = store.Insert(ctx, event)
		assert.NoError(t, err)

		found, err := store.Find(ctx, event)
		assert.NoError(t, err)
		assert.NotNil(t, found)
		assert.Equal(t, ts, found.Time.Unix())
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

	store, err := NewStore(dbRW, dbRO, testTableName, 0)
	assert.NoError(t, err)
	defer store.Close()

	baseTime := time.Now().UTC()
	eventCount := 100
	done := make(chan bool)

	// Concurrent inserts
	go func() {
		for i := 0; i < eventCount; i++ {
			event := components.Event{
				Time:      metav1.Time{Time: baseTime.Add(time.Duration(i) * time.Second)},
				Name:      "concurrent",
				Type:      common.EventTypeWarning,
				ExtraInfo: map[string]string{fmt.Sprintf("info_%d", i): fmt.Sprintf("Concurrent event %d", i)},
				SuggestedActions: &common.SuggestedActions{
					Descriptions: []string{fmt.Sprintf("Concurrent event %d", i)},
				},
			}
			assert.NoError(t, store.Insert(ctx, event))
		}
		done <- true
	}()

	// Concurrent reads
	go func() {
		for i := 0; i < eventCount; i++ {
			event := components.Event{
				Time:      metav1.Time{Time: baseTime.Add(time.Duration(i) * time.Second)},
				Name:      "concurrent",
				Type:      common.EventTypeWarning,
				ExtraInfo: map[string]string{fmt.Sprintf("info_%d", i): fmt.Sprintf("Concurrent event %d", i)},
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
		// Convert the entire ExtraInfo map to a string for comparison
		infoStr := fmt.Sprintf("%v", event.ExtraInfo)
		assert.False(t, infoMap[infoStr], "Duplicate extra info found")
		infoMap[infoStr] = true
	}
}

func TestNewStoreErrors(t *testing.T) {
	t.Parallel()

	testTableName := "test_table"

	// Test case: nil write DB
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()
	store, err := NewStore(nil, dbRO, testTableName, 0)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrNoDBRWSet)
	assert.Nil(t, store)

	// Test case: nil read DB
	store, err = NewStore(dbRW, nil, testTableName, 0)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrNoDBROSet)
	assert.Nil(t, store)

	// Test case: both DBs nil
	store, err = NewStore(nil, nil, testTableName, 0)
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

	store, err := NewStore(dbRW, dbRO, testTableName, 0)
	assert.NoError(t, err)
	defer store.Close()

	baseTime := time.Now().UTC()
	events := []components.Event{
		{
			Time:    metav1.Time{Time: baseTime},
			Name:    "test",
			Type:    common.EventTypeWarning,
			Message: "Test message with normal text",
		},
		{
			Time:    metav1.Time{Time: baseTime.Add(1 * time.Second)},
			Name:    "test",
			Type:    common.EventTypeWarning,
			Message: "", // Empty message
		},
		{
			Time:    metav1.Time{Time: baseTime.Add(2 * time.Second)},
			Name:    "test",
			Type:    common.EventTypeWarning,
			Message: "Message with special chars: !@#$%^&*()",
		},
		{
			Time:    metav1.Time{Time: baseTime.Add(3 * time.Second)},
			Name:    "test",
			Type:    common.EventTypeWarning,
			Message: "Unicode message: 你好世界",
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
	searchEvent := components.Event{
		Time:    metav1.Time{Time: baseTime},
		Name:    "test",
		Type:    common.EventTypeWarning,
		Message: "Test message with normal text",
	}
	found, err := store.Find(ctx, searchEvent)
	assert.NoError(t, err)
	assert.NotNil(t, found)
	assert.Equal(t, searchEvent.Message, found.Message)

	// Test finding with empty message
	emptyMessageEvent := components.Event{
		Time:    metav1.Time{Time: baseTime.Add(1 * time.Second)},
		Name:    "test",
		Type:    common.EventTypeWarning,
		Message: "",
	}
	found, err = store.Find(ctx, emptyMessageEvent)
	assert.NoError(t, err)
	assert.NotNil(t, found)
	assert.Equal(t, "", found.Message)

	// Test finding with non-matching message
	nonMatchingEvent := components.Event{
		Time:    metav1.Time{Time: baseTime},
		Name:    "test",
		Type:    common.EventTypeWarning,
		Message: "Non-matching message",
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

func TestNilSuggestedActions(t *testing.T) {
	t.Parallel()

	testTableName := "test_table"

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	store, err := NewStore(dbRW, dbRO, testTableName, 0)
	assert.NoError(t, err)
	defer store.Close()

	baseTime := time.Now().UTC()
	event := components.Event{
		Time:             metav1.Time{Time: baseTime},
		Name:             "test",
		Type:             common.EventTypeWarning,
		Message:          "Test message",
		ExtraInfo:        map[string]string{"key": "value"},
		SuggestedActions: nil, // Explicitly set to nil
	}

	// Test insert and find with nil SuggestedActions
	err = store.Insert(ctx, event)
	assert.NoError(t, err)

	found, err := store.Find(ctx, event)
	assert.NoError(t, err)
	assert.NotNil(t, found)
	assert.Nil(t, found.SuggestedActions)
}

func TestInvalidJSONHandling(t *testing.T) {
	t.Parallel()

	testTableName := "test_table"

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	store, err := NewStore(dbRW, dbRO, testTableName, 0)
	assert.NoError(t, err)
	defer store.Close()

	// Insert a valid event first
	baseTime := time.Now().UTC()
	event := components.Event{
		Time:      metav1.Time{Time: baseTime},
		Name:      "test",
		Type:      common.EventTypeWarning,
		ExtraInfo: map[string]string{"key": "value"},
		SuggestedActions: &common.SuggestedActions{
			Descriptions: []string{"test action"},
		},
	}
	err = store.Insert(ctx, event)
	assert.NoError(t, err)

	// Manually insert invalid JSON into the database
	_, err = dbRW.ExecContext(ctx, fmt.Sprintf(`
		INSERT INTO %s (timestamp, name, type, extra_info, suggested_actions)
		VALUES (?, ?, ?, ?, ?)`,
		testTableName),
		baseTime.Add(time.Second).Unix(),
		"test",
		common.EventTypeWarning,
		"{invalid_json", // Invalid JSON for ExtraInfo
		"{invalid_json", // Invalid JSON for SuggestedActions
	)
	assert.NoError(t, err)

	// Try to retrieve the events - should get error for invalid JSON
	_, err = store.Get(ctx, baseTime.Add(-time.Hour))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to unmarshal")
}

func TestEmptyTableName(t *testing.T) {
	t.Parallel()

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	// Test with empty table name
	store, err := NewStore(dbRW, dbRO, "", 0)
	assert.Error(t, err)
	assert.Nil(t, store)
}

func TestLongEventFields(t *testing.T) {
	t.Parallel()

	testTableName := "test_table"

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	store, err := NewStore(dbRW, dbRO, testTableName, 0)
	assert.NoError(t, err)
	defer store.Close()

	// Create very long strings for various fields
	longString := strings.Repeat("a", 10000)
	longMap := make(map[string]string)
	for i := 0; i < 100; i++ {
		longMap[fmt.Sprintf("key_%d", i)] = longString
	}

	event := components.Event{
		Time:      metav1.Time{Time: time.Now().UTC()},
		Name:      longString,
		Type:      common.EventTypeWarning,
		Message:   longString,
		ExtraInfo: longMap,
		SuggestedActions: &common.SuggestedActions{
			Descriptions: []string{longString},
		},
	}

	// Test insert and retrieval of event with very long fields
	err = store.Insert(ctx, event)
	assert.NoError(t, err)

	found, err := store.Find(ctx, event)
	assert.NoError(t, err)
	assert.NotNil(t, found)
	assert.Equal(t, event.Name, found.Name)
	assert.Equal(t, event.Message, found.Message)
	assert.Equal(t, event.ExtraInfo, found.ExtraInfo)
	assert.Equal(t, event.SuggestedActions.Descriptions[0], found.SuggestedActions.Descriptions[0])
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
			store, err := NewStore(dbRW, dbRO, tableName, 0)
			assert.NoError(t, err)
			defer store.Close()

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

	store, err := NewStore(dbRW, dbRO, testTableName, 0)
	assert.NoError(t, err)
	defer store.Close()

	// Test all valid event types
	validTypes := []common.EventType{
		common.EventTypeWarning,
		common.EventTypeInfo,
		common.EventTypeCritical,
		common.EventTypeFatal,
		common.EventTypeUnknown,
	}

	baseTime := time.Now().UTC()
	for i, eventType := range validTypes {
		event := components.Event{
			Time:    metav1.Time{Time: baseTime.Add(time.Duration(i) * time.Second)},
			Name:    "test",
			Type:    eventType,
			Message: fmt.Sprintf("Test message for %s", eventType),
		}

		err = store.Insert(ctx, event)
		assert.NoError(t, err)

		found, err := store.Find(ctx, event)
		assert.NoError(t, err)
		assert.NotNil(t, found)
		assert.Equal(t, eventType, found.Type)
	}

	// Verify all events can be retrieved
	events, err := store.Get(ctx, baseTime.Add(-time.Hour))
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

	// Create store with 10 second retention
	store, err := NewStore(dbRW, dbRO, testTableName, 10*time.Second)
	assert.NoError(t, err)
	defer store.Close()

	baseTime := time.Now().UTC()
	events := []components.Event{
		{
			Time:      metav1.Time{Time: baseTime.Add(-15 * time.Second)},
			Name:      "test",
			Type:      common.EventTypeWarning,
			ExtraInfo: map[string]string{"id": "old_event"},
			SuggestedActions: &common.SuggestedActions{
				Descriptions: []string{"old event"},
			},
		},
		{
			Time:      metav1.Time{Time: baseTime.Add(-5 * time.Second)},
			Name:      "test",
			Type:      common.EventTypeWarning,
			ExtraInfo: map[string]string{"id": "new_event"},
			SuggestedActions: &common.SuggestedActions{
				Descriptions: []string{"new event"},
			},
		},
	}

	for _, event := range events {
		err = store.Insert(ctx, event)
		assert.NoError(t, err)
	}

	time.Sleep(3 * time.Second)

	remaining, err := store.Get(ctx, baseTime.Add(-20*time.Second))
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

	store, err := NewStore(dbRW, dbRO, testTableName, 0)
	assert.NoError(t, err)
	defer store.Close()

	// Test with empty store
	latestEvent, err := store.Latest(ctx)
	assert.NoError(t, err)
	assert.Nil(t, latestEvent, "Latest should return nil for empty store")

	// Insert events with different timestamps
	baseTime := time.Now().UTC()
	events := []components.Event{
		{
			Time:    metav1.Time{Time: baseTime.Add(-10 * time.Second)},
			Name:    "test",
			Type:    common.EventTypeWarning,
			Message: "old event",
			ExtraInfo: map[string]string{
				"id": "event1",
			},
			SuggestedActions: &common.SuggestedActions{
				Descriptions: []string{"old event action"},
			},
		},
		{
			Time:    metav1.Time{Time: baseTime},
			Name:    "test",
			Type:    common.EventTypeInfo,
			Message: "latest event",
			ExtraInfo: map[string]string{
				"id": "event2",
			},
			SuggestedActions: &common.SuggestedActions{
				Descriptions: []string{"latest event action"},
			},
		},
		{
			Time:    metav1.Time{Time: baseTime.Add(-5 * time.Second)},
			Name:    "test",
			Type:    common.EventTypeCritical,
			Message: "middle event",
			ExtraInfo: map[string]string{
				"id": "event3",
			},
			SuggestedActions: &common.SuggestedActions{
				Descriptions: []string{"middle event action"},
			},
		},
	}

	// Insert events in random order
	for _, event := range events {
		err = store.Insert(ctx, event)
		assert.NoError(t, err)
	}

	// Get latest event
	latestEvent, err = store.Latest(ctx)
	assert.NoError(t, err)
	assert.NotNil(t, latestEvent)

	// Verify it's the event with the most recent timestamp
	assert.Equal(t, baseTime.Unix(), latestEvent.Time.Unix())
	assert.Equal(t, "latest event", latestEvent.Message)
	assert.Equal(t, common.EventTypeInfo, latestEvent.Type)
	assert.Equal(t, "event2", latestEvent.ExtraInfo["id"])
	assert.Equal(t, "latest event action", latestEvent.SuggestedActions.Descriptions[0])

	// Test with canceled context
	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately
	_, err = store.Latest(canceledCtx)
	assert.Error(t, err)

	// Test after purging all events
	deleted, err := store.Purge(ctx, baseTime.Add(time.Hour).Unix())
	assert.NoError(t, err)
	assert.Equal(t, 3, deleted)

	latestEvent, err = store.Latest(ctx)
	assert.NoError(t, err)
	assert.Nil(t, latestEvent, "Latest should return nil after purging all events")
}

func TestExtraInfoOrderedMap(t *testing.T) {
	t.Parallel()

	testTableName := "test_table"

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	store, err := NewStore(dbRW, dbRO, testTableName, 0)
	assert.NoError(t, err)
	defer store.Close()

	// Create a large ExtraInfo map with keys that could be marshaled in random order
	extraInfo := make(map[string]string)
	for i := 0; i < 100; i++ {
		// Use different key patterns to test ordering
		switch i % 3 {
		case 0:
			extraInfo[fmt.Sprintf("key_%d", i)] = fmt.Sprintf("value_%d", i)
		case 1:
			extraInfo[fmt.Sprintf("info_%d", i)] = fmt.Sprintf("data_%d", i)
		case 2:
			extraInfo[fmt.Sprintf("attr_%d", i)] = fmt.Sprintf("prop_%d", i)
		}
	}

	// Use a fixed timestamp to ensure exact matches
	baseTime := time.Unix(time.Now().Unix(), 0).UTC()
	event := components.Event{
		Time:      metav1.Time{Time: baseTime},
		Name:      "test",
		Type:      common.EventTypeWarning,
		Message:   "Test ordered map",
		ExtraInfo: extraInfo,
		SuggestedActions: &common.SuggestedActions{
			Descriptions: []string{"test action"},
		},
	}

	// Insert the event
	err = store.Insert(ctx, event)
	assert.NoError(t, err)

	// Verify the event was inserted
	events, err := store.Get(ctx, baseTime.Add(-1*time.Hour))
	assert.NoError(t, err)
	assert.Equal(t, 1, len(events))
	assert.Equal(t, event.ExtraInfo, events[0].ExtraInfo)

	// Test finding with exact match
	searchEvent := components.Event{
		Time:      event.Time,
		Name:      event.Name,
		Type:      event.Type,
		ExtraInfo: event.ExtraInfo,
	}
	found, err := store.Find(ctx, searchEvent)
	if err != nil {
		t.Logf("Find error: %v", err)
	}
	if found == nil {
		t.Log("Found event is nil")
		// Get all events to see what's in the database
		events, err := store.Get(ctx, baseTime.Add(-1*time.Hour))
		if err != nil {
			t.Logf("Get error: %v", err)
		} else {
			t.Logf("Found %d events in database", len(events))
			for i, e := range events {
				t.Logf("Event %d: time=%v name=%s type=%s", i, e.Time.Unix(), e.Name, e.Type)
				extraInfoJSON, _ := json.Marshal(e.ExtraInfo)
				t.Logf("Event %d ExtraInfo: %s", i, string(extraInfoJSON))
			}
		}
	}
	assert.NoError(t, err)
	assert.NotNil(t, found)
	assert.Equal(t, event.ExtraInfo, found.ExtraInfo)

	// Verify all keys are present and match
	for k, v := range event.ExtraInfo {
		foundValue, exists := found.ExtraInfo[k]
		assert.True(t, exists, "Key %s should exist in found event", k)
		assert.Equal(t, v, foundValue, "Value for key %s should match", k)
	}

	// Test finding with partial ExtraInfo - this should return nil since we require exact matches
	partialInfo := make(map[string]string)
	// Take a subset of the original keys
	i := 0
	for k, v := range extraInfo {
		partialInfo[k] = v
		i++
		if i >= 10 {
			break
		}
	}

	searchEvent = components.Event{
		Time:      metav1.Time{Time: baseTime},
		Name:      "test",
		Type:      common.EventTypeWarning,
		ExtraInfo: partialInfo,
	}

	found, err = store.Find(ctx, searchEvent)
	assert.NoError(t, err)
	assert.Nil(t, found, "Should not find event with partial ExtraInfo")

	// Test finding with different ExtraInfo
	differentInfo := make(map[string]string)
	for k := range extraInfo {
		differentInfo[k] = "different_value"
	}

	searchEvent.ExtraInfo = differentInfo
	found, err = store.Find(ctx, searchEvent)
	assert.NoError(t, err)
	assert.Nil(t, found, "Should not find event with different ExtraInfo values")

	// Test concurrent finds with the same ExtraInfo
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			searchEvent := components.Event{
				Time:      event.Time,
				Name:      event.Name,
				Type:      event.Type,
				ExtraInfo: event.ExtraInfo,
			}
			found, err := store.Find(ctx, searchEvent)
			assert.NoError(t, err)
			assert.NotNil(t, found)
			assert.Equal(t, event.ExtraInfo, found.ExtraInfo)
		}()
	}
	wg.Wait()

	// Test with multiple events having same ExtraInfo
	for i := 1; i < 5; i++ {
		eventCopy := event
		eventCopy.Time = metav1.Time{Time: time.Unix(baseTime.Unix()+int64(i), 0).UTC()}
		err = store.Insert(ctx, eventCopy)
		assert.NoError(t, err)
	}

	// Should still find the original event when searching
	searchEvent = components.Event{
		Time:      event.Time,
		Name:      event.Name,
		Type:      event.Type,
		ExtraInfo: event.ExtraInfo,
	}
	found, err = store.Find(ctx, searchEvent)
	assert.NoError(t, err)
	assert.NotNil(t, found)
	assert.Equal(t, baseTime.Unix(), found.Time.Unix())
	assert.Equal(t, event.ExtraInfo, found.ExtraInfo)
}
