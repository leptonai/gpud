package infiniband

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/eventstore"
	"github.com/leptonai/gpud/pkg/nvidia-query/infiniband"
	infinibandstore "github.com/leptonai/gpud/pkg/nvidia-query/infiniband/store"
)

// TestComponent_SetHealthy tests the SetHealthy method
func TestComponent_SetHealthy(t *testing.T) {
	ctx := context.Background()

	t.Run("with event bucket and ibPortsStore - successful", func(t *testing.T) {
		fixedTime := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)

		mockBucket := createMockEventBucketForSetHealthy()
		mockStore := &mockIBPortsStoreForSetHealthy{}

		c := &component{
			ctx:          ctx,
			eventBucket:  mockBucket,
			ibPortsStore: mockStore,
			getTimeNowFunc: func() time.Time {
				return fixedTime
			},
		}

		err := c.SetHealthy()
		assert.NoError(t, err)

		// Verify tombstone was set
		assert.Equal(t, fixedTime, mockStore.tombstoneTime)

		// Verify purge was called
		assert.Equal(t, 1, mockBucket.purgeCalls)
		assert.Equal(t, fixedTime.Unix(), mockBucket.purgeBeforeTimestamp)
	})

	t.Run("without event bucket", func(t *testing.T) {
		fixedTime := time.Date(2024, 2, 1, 11, 0, 0, 0, time.UTC)

		mockStore := &mockIBPortsStoreForSetHealthy{}

		c := &component{
			ctx:          ctx,
			eventBucket:  nil,
			ibPortsStore: mockStore,
			getTimeNowFunc: func() time.Time {
				return fixedTime
			},
		}

		err := c.SetHealthy()
		assert.NoError(t, err)

		// Verify tombstone was still set
		assert.Equal(t, fixedTime, mockStore.tombstoneTime)
	})

	t.Run("without ibPortsStore", func(t *testing.T) {
		fixedTime := time.Date(2024, 3, 1, 12, 0, 0, 0, time.UTC)

		mockBucket := createMockEventBucketForSetHealthy()

		c := &component{
			ctx:          ctx,
			eventBucket:  mockBucket,
			ibPortsStore: nil,
			getTimeNowFunc: func() time.Time {
				return fixedTime
			},
		}

		err := c.SetHealthy()
		assert.NoError(t, err)

		// Verify purge was called
		assert.Equal(t, 1, mockBucket.purgeCalls)
		assert.Equal(t, fixedTime.Unix(), mockBucket.purgeBeforeTimestamp)
	})

	t.Run("tombstone error is handled gracefully", func(t *testing.T) {
		fixedTime := time.Date(2024, 4, 1, 13, 0, 0, 0, time.UTC)

		mockBucket := createMockEventBucketForSetHealthy()
		mockStore := &mockIBPortsStoreForSetHealthy{
			tombstoneErr: errors.New("tombstone failed"),
		}

		c := &component{
			ctx:          ctx,
			eventBucket:  mockBucket,
			ibPortsStore: mockStore,
			getTimeNowFunc: func() time.Time {
				return fixedTime
			},
		}

		err := c.SetHealthy()
		// Should not return error, just log warning
		assert.NoError(t, err)

		// Verify purge was still called
		assert.Equal(t, 1, mockBucket.purgeCalls)
	})

	t.Run("purge returns error", func(t *testing.T) {
		fixedTime := time.Date(2024, 5, 1, 14, 0, 0, 0, time.UTC)
		purgeErr := errors.New("purge failed")

		mockBucket := createMockEventBucketForSetHealthy()
		mockBucket.purgeErr = purgeErr
		mockStore := &mockIBPortsStoreForSetHealthy{}

		c := &component{
			ctx:          ctx,
			eventBucket:  mockBucket,
			ibPortsStore: mockStore,
			getTimeNowFunc: func() time.Time {
				return fixedTime
			},
		}

		err := c.SetHealthy()
		assert.Error(t, err)
		assert.Equal(t, purgeErr, err)

		// Verify tombstone was still set before error
		assert.Equal(t, fixedTime, mockStore.tombstoneTime)
	})

	t.Run("SetHealthy event already exists - skips insertion", func(t *testing.T) {
		fixedTime := time.Date(2024, 6, 1, 15, 0, 0, 0, time.UTC)

		mockBucket := createMockEventBucketForSetHealthy()
		// Pre-insert a SetHealthy event
		existingEvent := eventstore.Event{
			Time: fixedTime.Add(-1 * time.Hour),
			Name: "SetHealthy",
		}
		err := mockBucket.Insert(ctx, existingEvent)
		assert.NoError(t, err)

		mockStore := &mockIBPortsStoreForSetHealthy{}

		c := &component{
			ctx:          ctx,
			eventBucket:  mockBucket,
			ibPortsStore: mockStore,
			getTimeNowFunc: func() time.Time {
				return fixedTime
			},
		}

		err = c.SetHealthy()
		assert.NoError(t, err)

		// No longer verifying SetHealthy events since insertion was removed
	})

	t.Run("context timeout during operations", func(t *testing.T) {
		fixedTime := time.Date(2024, 9, 1, 18, 0, 0, 0, time.UTC)

		// Create a context that's already canceled
		canceledCtx, cancel := context.WithCancel(ctx)
		cancel()

		mockBucket := createMockEventBucketForSetHealthy()
		mockBucket.purgeErr = context.Canceled
		mockStore := &mockIBPortsStoreForSetHealthy{}

		c := &component{
			ctx:          canceledCtx,
			eventBucket:  mockBucket,
			ibPortsStore: mockStore,
			getTimeNowFunc: func() time.Time {
				return fixedTime
			},
		}

		err := c.SetHealthy()
		assert.Error(t, err)
		assert.Equal(t, context.Canceled, err)
	})
}

// TestComponent_ImplementsHealthSettable verifies the interface implementation
func TestComponent_ImplementsHealthSettable(t *testing.T) {
	// This will fail to compile if component doesn't implement HealthSettable
	var _ components.HealthSettable = &component{}
}

// mockIBPortsStoreForSetHealthy extends the existing mockIBPortsStore for SetHealthy testing
type mockIBPortsStoreForSetHealthy struct {
	events        []infinibandstore.Event
	tombstoneTime time.Time
	tombstoneErr  error
}

func (m *mockIBPortsStoreForSetHealthy) Insert(eventTime time.Time, ibPorts []infiniband.IBPort) error {
	return nil
}

func (m *mockIBPortsStoreForSetHealthy) SetEventType(device string, port uint, timestamp time.Time, eventType string, eventReason string) error {
	return nil
}

func (m *mockIBPortsStoreForSetHealthy) LastEvents(since time.Time) ([]infinibandstore.Event, error) {
	return m.events, nil
}

func (m *mockIBPortsStoreForSetHealthy) Tombstone(timestamp time.Time) error {
	if m.tombstoneErr != nil {
		return m.tombstoneErr
	}
	m.tombstoneTime = timestamp
	return nil
}

func (m *mockIBPortsStoreForSetHealthy) Scan() error {
	return nil
}

// mockEventBucketForSetHealthy extends the existing mockEventBucket for SetHealthy testing
type mockEventBucketForSetHealthy struct {
	events               eventstore.Events
	insertErr            error
	findErr              error
	purgeErr             error
	purgeCalls           int
	purgeBeforeTimestamp int64
}

func createMockEventBucketForSetHealthy() *mockEventBucketForSetHealthy {
	return &mockEventBucketForSetHealthy{
		events: eventstore.Events{},
	}
}

func (m *mockEventBucketForSetHealthy) Name() string {
	return "mock"
}

func (m *mockEventBucketForSetHealthy) Insert(ctx context.Context, event eventstore.Event) error {
	if m.insertErr != nil {
		return m.insertErr
	}
	m.events = append(m.events, event)
	return nil
}

func (m *mockEventBucketForSetHealthy) Find(ctx context.Context, event eventstore.Event) (*eventstore.Event, error) {
	if m.findErr != nil {
		return nil, m.findErr
	}
	for i, e := range m.events {
		if e.Name == event.Name {
			return &m.events[i], nil
		}
	}
	return nil, nil
}

func (m *mockEventBucketForSetHealthy) Get(ctx context.Context, since time.Time) (eventstore.Events, error) {
	var result eventstore.Events
	for _, event := range m.events {
		if !event.Time.Before(since) {
			result = append(result, event)
		}
	}
	return result, nil
}

func (m *mockEventBucketForSetHealthy) Latest(ctx context.Context) (*eventstore.Event, error) {
	if len(m.events) == 0 {
		return nil, nil
	}
	latest := m.events[0]
	for _, e := range m.events[1:] {
		if e.Time.After(latest.Time) {
			latest = e
		}
	}
	return &latest, nil
}

func (m *mockEventBucketForSetHealthy) Purge(ctx context.Context, beforeTimestamp int64) (int, error) {
	if m.purgeErr != nil {
		return 0, m.purgeErr
	}
	m.purgeCalls++
	m.purgeBeforeTimestamp = beforeTimestamp

	var newEvents eventstore.Events
	purgedCount := 0
	for _, event := range m.events {
		if event.Time.Unix() >= beforeTimestamp {
			newEvents = append(newEvents, event)
		} else {
			purgedCount++
		}
	}
	m.events = newEvents
	return purgedCount, nil
}

func (m *mockEventBucketForSetHealthy) Close() {
	// No-op for mock
}

// GetAPIEvents returns events for assertion convenience
func (m *mockEventBucketForSetHealthy) GetAPIEvents() []struct{ Name, Component, Type string } {
	result := make([]struct{ Name, Component, Type string }, len(m.events))
	for i, ev := range m.events {
		result[i] = struct{ Name, Component, Type string }{
			Name:      ev.Name,
			Component: ev.Component,
			Type:      ev.Type,
		}
	}
	return result
}
