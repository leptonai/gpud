package streamer

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/pkg/eventstore"
)

// mockEventBucket implements eventstore.Bucket for testing
type mockEventBucket struct {
	mu     sync.Mutex
	events []eventstore.Event
	name   string
}

func newMockEventBucket() *mockEventBucket {
	return &mockEventBucket{
		events: make([]eventstore.Event, 0),
		name:   "test-bucket",
	}
}

func (m *mockEventBucket) Name() string {
	return m.name
}

func (m *mockEventBucket) Insert(ctx context.Context, event eventstore.Event) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check for duplicates
	for _, e := range m.events {
		if e.Name == event.Name && e.Time.Equal(event.Time) && e.Message == event.Message {
			return fmt.Errorf("duplicate event")
		}
	}

	m.events = append(m.events, event)
	return nil
}

func (m *mockEventBucket) Find(ctx context.Context, event eventstore.Event) (*eventstore.Event, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, e := range m.events {
		if e.Name == event.Name && e.Time.Equal(event.Time) && e.Message == event.Message {
			return &e, nil
		}
	}
	return nil, nil
}

func (m *mockEventBucket) Get(ctx context.Context, since time.Time) (eventstore.Events, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var result []eventstore.Event
	for _, e := range m.events {
		if e.Time.After(since) || e.Time.Equal(since) {
			result = append(result, e)
		}
	}
	return result, nil
}

func (m *mockEventBucket) Delete(ctx context.Context, id string) error {
	return nil
}

func (m *mockEventBucket) GetN(ctx context.Context, n int) (eventstore.Events, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if n > len(m.events) {
		n = len(m.events)
	}
	result := make([]eventstore.Event, n)
	copy(result, m.events[:n])
	return result, nil
}

func (m *mockEventBucket) Latest(ctx context.Context) (*eventstore.Event, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.events) == 0 {
		return nil, nil
	}

	// Return the latest event (last in slice since we append new events)
	return &m.events[len(m.events)-1], nil
}

func (m *mockEventBucket) Purge(ctx context.Context, beforeTimestamp int64) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	count := 0
	var remaining []eventstore.Event
	for _, e := range m.events {
		if e.Time.Unix() < beforeTimestamp {
			count++
		} else {
			remaining = append(remaining, e)
		}
	}
	m.events = remaining
	return count, nil
}

func (m *mockEventBucket) Close() {
	// No-op for mock
}

// Test match function for fabric manager logs
func testMatchFunc(line string) (eventName string, message string) {
	if strings.Contains(line, "[ERROR]") {
		if strings.Contains(line, "NVSwitch non-fatal error") {
			return "nvswitch_non_fatal_error", line
		}
		return "fabric_manager_error", line
	}
	if strings.Contains(line, "multicast group") && strings.Contains(line, "is allocated") {
		return "multicast_group_allocated", line
	}
	if strings.Contains(line, "multicast group") && strings.Contains(line, "is freed") {
		return "multicast_group_freed", line
	}
	return "", ""
}

func TestProcessorCreation(t *testing.T) {
	t.Run("create processor with valid parameters", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		eventBucket := newMockEventBucket()
		p, err := New(
			ctx,
			[][]string{{"echo", "test"}},
			testMatchFunc,
			parseFabricManagerLog,
			eventBucket,
		)
		require.NoError(t, err)
		require.NotNil(t, p)
		defer p.Close()
	})

	t.Run("create processor with invalid commands", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		eventBucket := newMockEventBucket()
		p, err := New(
			ctx,
			[][]string{}, // Empty commands
			testMatchFunc,
			parseFabricManagerLog,
			eventBucket,
		)
		assert.Error(t, err)
		assert.Nil(t, p)
		assert.Contains(t, err.Error(), "no commands provided")
	})
}

func TestProcessorWatch(t *testing.T) {
	t.Run("process matching log lines", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		eventBucket := newMockEventBucket()
		p, err := New(
			ctx,
			[][]string{
				{"echo", "[Feb 27 2025 15:10:02] [ERROR] [tid 12727] detected NVSwitch non-fatal error 12028"},
				{"echo", "[Feb 25 2025 13:59:45] [INFO] [tid 1808] multicast group 0 is allocated."},
				{"echo", "[Feb 25 2025 13:59:45] [INFO] [tid 1808] multicast group 0 is freed."},
			},
			testMatchFunc,
			parseFabricManagerLog,
			eventBucket,
		)
		require.NoError(t, err)
		require.NotNil(t, p)
		defer p.Close()

		// Give processor time to process
		time.Sleep(500 * time.Millisecond)

		// Check events were inserted
		events, err := eventBucket.Get(ctx, time.Time{})
		require.NoError(t, err)
		assert.Len(t, events, 3)

		// Verify event names
		eventNames := make(map[string]bool)
		for _, e := range events {
			eventNames[e.Name] = true
		}
		assert.True(t, eventNames["nvswitch_non_fatal_error"])
		assert.True(t, eventNames["multicast_group_allocated"])
		assert.True(t, eventNames["multicast_group_freed"])
	})

	t.Run("skip non-matching log lines", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		eventBucket := newMockEventBucket()
		p, err := New(
			ctx,
			[][]string{
				{"echo", "[Feb 25 2025 13:59:45] [INFO] [tid 1803] Received an inband message"},
				{"echo", "[Feb 25 2025 13:59:45] [INFO] Some random info log"},
			},
			testMatchFunc,
			parseFabricManagerLog,
			eventBucket,
		)
		require.NoError(t, err)
		require.NotNil(t, p)
		defer p.Close()

		// Give processor time to process
		time.Sleep(500 * time.Millisecond)

		// Check no events were inserted
		events, err := eventBucket.Get(ctx, time.Time{})
		require.NoError(t, err)
		assert.Len(t, events, 0)
	})

	t.Run("prevent duplicate events", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		eventBucket := newMockEventBucket()

		// Pre-insert an event
		existingEvent := eventstore.Event{
			Time:    time.Date(2025, time.February, 27, 15, 10, 2, 0, time.UTC),
			Type:    string(apiv1.EventTypeWarning),
			Name:    "nvswitch_non_fatal_error",
			Message: "[ERROR] [tid 12727] detected NVSwitch non-fatal error 12028",
		}
		err := eventBucket.Insert(ctx, existingEvent)
		require.NoError(t, err)

		p, err := New(
			ctx,
			[][]string{
				// Same error line that should be deduplicated
				{"echo", "[Feb 27 2025 15:10:02] [ERROR] [tid 12727] detected NVSwitch non-fatal error 12028"},
				// Different error that should be inserted
				{"echo", "[Feb 27 2025 15:10:03] [ERROR] [tid 12727] detected NVSwitch non-fatal error 12029"},
			},
			testMatchFunc,
			parseFabricManagerLog,
			eventBucket,
		)
		require.NoError(t, err)
		require.NotNil(t, p)
		defer p.Close()

		// Give processor time to process
		time.Sleep(500 * time.Millisecond)

		// Check only one new event was inserted
		events, err := eventBucket.Get(ctx, time.Time{})
		require.NoError(t, err)
		assert.Len(t, events, 2) // Original + 1 new
	})

	t.Run("context cancellation stops processing", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())

		eventBucket := newMockEventBucket()
		p, err := New(
			ctx,
			[][]string{
				{"sleep", "2"}, // Long running command
			},
			testMatchFunc,
			parseFabricManagerLog,
			eventBucket,
		)
		require.NoError(t, err)
		require.NotNil(t, p)

		// Cancel context
		cancel()

		// Close should complete without hanging
		done := make(chan struct{})
		go func() {
			p.Close()
			close(done)
		}()

		select {
		case <-done:
			// Success
		case <-time.After(2 * time.Second):
			t.Fatal("Close did not complete after context cancellation")
		}
	})
}

func TestProcessorEvents(t *testing.T) {
	t.Run("get events since timestamp", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		eventBucket := newMockEventBucket()

		// Pre-insert some events
		events := []eventstore.Event{
			{
				Time:    time.Date(2025, time.February, 25, 10, 0, 0, 0, time.UTC),
				Type:    string(apiv1.EventTypeWarning),
				Name:    "event1",
				Message: "message1",
			},
			{
				Time:    time.Date(2025, time.February, 25, 11, 0, 0, 0, time.UTC),
				Type:    string(apiv1.EventTypeWarning),
				Name:    "event2",
				Message: "message2",
			},
			{
				Time:    time.Date(2025, time.February, 25, 12, 0, 0, 0, time.UTC),
				Type:    string(apiv1.EventTypeWarning),
				Name:    "event3",
				Message: "message3",
			},
		}

		for _, e := range events {
			err := eventBucket.Insert(ctx, e)
			require.NoError(t, err)
		}

		p, err := New(
			ctx,
			[][]string{{"echo", "test"}},
			testMatchFunc,
			parseFabricManagerLog,
			eventBucket,
		)
		require.NoError(t, err)
		require.NotNil(t, p)
		defer p.Close()

		// Get events since 11:00
		since := time.Date(2025, time.February, 25, 11, 0, 0, 0, time.UTC)
		gotEvents, err := p.Events(ctx, since)
		require.NoError(t, err)
		assert.Len(t, gotEvents, 2) // Should get event2 and event3

		// Verify we got the right events
		eventNames := make(map[string]bool)
		for _, e := range gotEvents {
			eventNames[e.Name] = true
		}
		assert.True(t, eventNames["event2"])
		assert.True(t, eventNames["event3"])
		assert.False(t, eventNames["event1"])
	})
}

func TestProcessorWithFabricManagerTestData(t *testing.T) {
	t.Run("process fabric manager test log", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// Read the test data file
		testData, err := os.ReadFile("testdata/fabricmanager.log")
		require.NoError(t, err)

		// Create a temporary file
		tmpFile, err := os.CreateTemp("", "fabricmanager_test_*.log")
		require.NoError(t, err)
		defer os.Remove(tmpFile.Name())

		// Write test data to temp file
		_, err = tmpFile.Write(testData)
		require.NoError(t, err)
		tmpFile.Close()

		eventBucket := newMockEventBucket()

		// Use tail -n +1 -f to read from the beginning and follow the file
		// This ensures we read all lines, not just the last few
		p, err := New(
			ctx,
			[][]string{
				{"tail", "-n", "+1", "-f", tmpFile.Name()},
			},
			testMatchFunc,
			parseFabricManagerLog,
			eventBucket,
		)
		require.NoError(t, err)
		require.NotNil(t, p)
		defer p.Close()

		// Wait for processor to process all lines
		// Poll for the NVSwitch error event with timeout
		var events apiv1.Events
		deadline := time.Now().Add(5 * time.Second)
		foundNVSwitchError := false
		allocatedCount := 0
		freedCount := 0

		for time.Now().Before(deadline) {
			events, err = p.Events(ctx, time.Time{})
			require.NoError(t, err)

			// Count events
			allocatedCount = 0
			freedCount = 0
			for _, e := range events {
				switch e.Name {
				case "nvswitch_non_fatal_error":
					foundNVSwitchError = true
					assert.Contains(t, e.Message, "detected NVSwitch non-fatal error 12028")
					expectedTime := time.Date(2025, time.February, 27, 15, 10, 2, 0, time.UTC)
					assert.True(t, e.Time.Time.Equal(expectedTime), "timestamp should match")
				case "multicast_group_allocated":
					allocatedCount++
				case "multicast_group_freed":
					freedCount++
				}
			}

			// Check if we have all expected events
			if foundNVSwitchError && allocatedCount > 0 && freedCount > 0 {
				break
			}

			time.Sleep(100 * time.Millisecond)
		}

		assert.True(t, foundNVSwitchError, "should find NVSwitch error event")
		assert.Greater(t, allocatedCount, 0, "should have multicast allocated events")
		assert.Greater(t, freedCount, 0, "should have multicast freed events")
	})
}

func TestProcessorClose(t *testing.T) {
	t.Run("close processor multiple times", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		eventBucket := newMockEventBucket()
		p, err := New(
			ctx,
			[][]string{{"echo", "test"}},
			testMatchFunc,
			parseFabricManagerLog,
			eventBucket,
		)
		require.NoError(t, err)
		require.NotNil(t, p)

		// Close multiple times should not panic
		p.Close()
		p.Close()
		p.Close()
	})
}

func TestEventTypes(t *testing.T) {
	t.Run("all events should be warning type", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		eventBucket := newMockEventBucket()
		p, err := New(
			ctx,
			[][]string{
				{"echo", "[Feb 27 2025 15:10:02] [ERROR] [tid 12727] detected NVSwitch non-fatal error"},
				{"echo", "[Feb 25 2025 13:59:45] [INFO] [tid 1808] multicast group 0 is allocated."},
			},
			testMatchFunc,
			parseFabricManagerLog,
			eventBucket,
		)
		require.NoError(t, err)
		require.NotNil(t, p)
		defer p.Close()

		// Give processor time to process
		time.Sleep(500 * time.Millisecond)

		events, err := eventBucket.Get(ctx, time.Time{})
		require.NoError(t, err)

		// All events should be of type warning
		for _, e := range events {
			assert.Equal(t, string(apiv1.EventTypeWarning), e.Type)
		}
	})
}
