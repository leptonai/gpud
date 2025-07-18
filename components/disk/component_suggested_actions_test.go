package disk

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/pkg/disk"
	"github.com/leptonai/gpud/pkg/eventstore"
	pkghost "github.com/leptonai/gpud/pkg/host"
)

// mockRebootEventStore implements pkghost.RebootEventStore for testing
type mockRebootEventStore struct {
	events eventstore.Events
	err    error
}

// Ensure mockRebootEventStore implements pkghost.RebootEventStore
var _ pkghost.RebootEventStore = (*mockRebootEventStore)(nil)

func (m *mockRebootEventStore) RecordReboot(ctx context.Context) error {
	return nil
}

func (m *mockRebootEventStore) GetRebootEvents(ctx context.Context, since time.Time) (eventstore.Events, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.events, nil
}

// TestEvaluateSuggestedActions tests the evaluateSuggestedActions function
func TestEvaluateSuggestedActions(t *testing.T) {
	now := time.Now()

	t.Run("no disk failure events", func(t *testing.T) {
		cr := &checkResult{
			ts: now,
		}
		rebootEvents := eventstore.Events{}
		diskFailureEvents := eventstore.Events{} // Empty

		evaluateSuggestedActions(cr, rebootEvents, diskFailureEvents)

		assert.NotNil(t, cr.err)
		assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
		assert.Equal(t, "no disk failure event found after insert", cr.reason)
		assert.Contains(t, cr.err.Error(), "no disk failure event found after insert")
		assert.Nil(t, cr.suggestedActions)
	})

	t.Run("case 1 - no reboot events", func(t *testing.T) {
		cr := &checkResult{
			ts: now,
		}
		rebootEvents := eventstore.Events{} // No reboots
		diskFailureEvents := eventstore.Events{
			{
				Component: Name,
				Time:      now.Add(-1 * time.Hour),
				Name:      eventRAIDArrayFailure,
				Type:      string(apiv1.EventTypeWarning),
				Message:   "RAID array failure detected",
			},
		}

		evaluateSuggestedActions(cr, rebootEvents, diskFailureEvents)

		assert.Nil(t, cr.err)
		assert.NotNil(t, cr.suggestedActions)
		assert.Len(t, cr.suggestedActions.RepairActions, 1)
		assert.Equal(t, apiv1.RepairActionTypeRebootSystem, cr.suggestedActions.RepairActions[0])
	})

	t.Run("edge case - reboot after failure", func(t *testing.T) {
		cr := &checkResult{
			ts: now,
		}
		rebootEvents := eventstore.Events{
			{
				Time:    now.Add(-30 * time.Minute), // Reboot happened recently
				Name:    "reboot",
				Type:    string(apiv1.EventTypeWarning),
				Message: "system reboot detected",
			},
		}
		diskFailureEvents := eventstore.Events{
			{
				Component: Name,
				Time:      now.Add(-1 * time.Hour), // Failure happened before reboot
				Name:      eventFilesystemReadOnly,
				Type:      string(apiv1.EventTypeWarning),
				Message:   "Filesystem remounted read-only",
			},
		}

		evaluateSuggestedActions(cr, rebootEvents, diskFailureEvents)

		assert.Nil(t, cr.err)
		assert.Nil(t, cr.suggestedActions) // No action when reboot happened after failure
	})

	t.Run("case 2a - one reboot one failure", func(t *testing.T) {
		cr := &checkResult{
			ts: now,
		}
		rebootEvents := eventstore.Events{
			{
				Time:    now.Add(-2 * time.Hour), // Reboot happened first
				Name:    "reboot",
				Type:    string(apiv1.EventTypeWarning),
				Message: "system reboot detected",
			},
		}
		diskFailureEvents := eventstore.Events{
			{
				Component: Name,
				Time:      now.Add(-1 * time.Hour), // Failure after reboot
				Name:      eventNVMePathFailure,
				Type:      string(apiv1.EventTypeWarning),
				Message:   "NVMe path failure detected",
			},
		}

		evaluateSuggestedActions(cr, rebootEvents, diskFailureEvents)

		assert.Nil(t, cr.err)
		assert.NotNil(t, cr.suggestedActions)
		assert.Len(t, cr.suggestedActions.RepairActions, 1)
		assert.Equal(t, apiv1.RepairActionTypeRebootSystem, cr.suggestedActions.RepairActions[0])
	})

	t.Run("case 2b - multiple reboots one failure", func(t *testing.T) {
		cr := &checkResult{
			ts: now,
		}
		rebootEvents := eventstore.Events{
			{
				Time:    now.Add(-4 * time.Hour),
				Name:    "reboot",
				Type:    string(apiv1.EventTypeWarning),
				Message: "system reboot detected",
			},
			{
				Time:    now.Add(-2 * time.Hour),
				Name:    "reboot",
				Type:    string(apiv1.EventTypeWarning),
				Message: "system reboot detected",
			},
		}
		diskFailureEvents := eventstore.Events{
			{
				Component: Name,
				Time:      now.Add(-1 * time.Hour),
				Name:      eventRAIDArrayFailure,
				Type:      string(apiv1.EventTypeWarning),
				Message:   "RAID array failure detected",
			},
		}

		evaluateSuggestedActions(cr, rebootEvents, diskFailureEvents)

		assert.Nil(t, cr.err)
		assert.NotNil(t, cr.suggestedActions)
		assert.Len(t, cr.suggestedActions.RepairActions, 1)
		assert.Equal(t, apiv1.RepairActionTypeRebootSystem, cr.suggestedActions.RepairActions[0])
	})

	t.Run("case 3 - multiple sequences", func(t *testing.T) {
		cr := &checkResult{
			ts: now,
		}
		rebootEvents := eventstore.Events{
			{
				Time:    now.Add(-5 * time.Hour),
				Name:    "reboot",
				Type:    string(apiv1.EventTypeWarning),
				Message: "system reboot detected",
			},
			{
				Time:    now.Add(-3 * time.Hour),
				Name:    "reboot",
				Type:    string(apiv1.EventTypeWarning),
				Message: "system reboot detected",
			},
		}
		diskFailureEvents := eventstore.Events{
			{
				Component: Name,
				Time:      now.Add(-4 * time.Hour), // After first reboot
				Name:      eventFilesystemReadOnly,
				Type:      string(apiv1.EventTypeWarning),
				Message:   "Filesystem remounted read-only",
			},
			{
				Component: Name,
				Time:      now.Add(-2 * time.Hour), // After second reboot
				Name:      eventFilesystemReadOnly,
				Type:      string(apiv1.EventTypeWarning),
				Message:   "Filesystem remounted read-only",
			},
		}

		evaluateSuggestedActions(cr, rebootEvents, diskFailureEvents)

		assert.Nil(t, cr.err)
		assert.NotNil(t, cr.suggestedActions)
		assert.Len(t, cr.suggestedActions.RepairActions, 1)
		assert.Equal(t, apiv1.RepairActionTypeHardwareInspection, cr.suggestedActions.RepairActions[0])
	})

	t.Run("edge case - multiple failures but only one sequence", func(t *testing.T) {
		cr := &checkResult{
			ts: now,
		}
		// Initial reboot before all failures
		rebootEvents := eventstore.Events{
			{
				Time:    now.Add(-48 * time.Hour), // Initial reboot
				Name:    "reboot",
				Type:    string(apiv1.EventTypeWarning),
				Message: "system reboot detected",
			},
		}
		diskFailureEvents := eventstore.Events{
			{
				Component: Name,
				Time:      now.Add(-24 * time.Hour), // Failure 1 after reboot
				Name:      eventNVMePathFailure,
				Type:      string(apiv1.EventTypeWarning),
				Message:   "NVMe path failure detected",
			},
			{
				Component: Name,
				Time:      now.Add(-12 * time.Hour), // Failure 2 after reboot
				Name:      eventRAIDArrayFailure,
				Type:      string(apiv1.EventTypeWarning),
				Message:   "RAID array failure detected",
			},
		}

		evaluateSuggestedActions(cr, rebootEvents, diskFailureEvents)

		assert.Nil(t, cr.err)
		assert.NotNil(t, cr.suggestedActions)
		assert.Len(t, cr.suggestedActions.RepairActions, 1)
		// Should suggest reboot since only one valid sequence
		assert.Equal(t, apiv1.RepairActionTypeRebootSystem, cr.suggestedActions.RepairActions[0])
	})
}

// TestComponent_Check_SuggestedActions tests the integration of suggested actions in Check method
func TestComponent_Check_SuggestedActions(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	now := time.Now()

	// Create a test event store
	eventBucket := &simpleMockEventBucket{}

	// Insert some disk failure events
	err := eventBucket.Insert(ctx, eventstore.Event{
		Component: Name,
		Time:      now.Add(-2 * time.Hour),
		Name:      eventRAIDArrayFailure,
		Type:      string(apiv1.EventTypeWarning),
		Message:   "RAID array failure",
	})
	require.NoError(t, err)

	// Create mock reboot event store
	mockRebootStore := &mockRebootEventStore{
		events: eventstore.Events{
			{
				Time:    now.Add(-3 * time.Hour),
				Name:    "reboot",
				Type:    string(apiv1.EventTypeWarning),
				Message: "system reboot detected",
			},
		},
	}

	// Create component
	c := &component{
		ctx:              ctx,
		rebootEventStore: mockRebootStore,
		eventBucket:      eventBucket,
		lookbackPeriod:   96 * time.Hour,
		getExt4PartitionsFunc: func(ctx context.Context) (disk.Partitions, error) {
			return disk.Partitions{}, nil
		},
		getNFSPartitionsFunc: func(ctx context.Context) (disk.Partitions, error) {
			return disk.Partitions{}, nil
		},
	}

	// Run check
	result := c.Check()
	cr, ok := result.(*checkResult)
	require.True(t, ok)

	// Should be healthy initially (no recent failures)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
	assert.Equal(t, "no ext4/nfs partition found", cr.reason)
}

// TestComponent_Check_NoSpaceLeftNotSuggested tests that eventNoSpaceLeft doesn't trigger reboot
func TestComponent_Check_NoSpaceLeftNotSuggested(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	now := time.Now()

	// Create a test event store
	eventBucket := &simpleMockEventBucket{}

	// Insert a no-space-left event (should NOT trigger reboot suggestion)
	err := eventBucket.Insert(ctx, eventstore.Event{
		Component: Name,
		Time:      now.Add(-1 * time.Minute),
		Name:      eventNoSpaceLeft,
		Type:      string(apiv1.EventTypeWarning),
		Message:   "No space left on device",
	})
	require.NoError(t, err)

	// Create mock reboot event store
	mockRebootStore := &mockRebootEventStore{
		events: eventstore.Events{},
	}

	// Create component
	c := &component{
		ctx:              ctx,
		rebootEventStore: mockRebootStore,
		eventBucket:      eventBucket,
		lookbackPeriod:   96 * time.Hour,
		getExt4PartitionsFunc: func(ctx context.Context) (disk.Partitions, error) {
			return disk.Partitions{}, nil
		},
		getNFSPartitionsFunc: func(ctx context.Context) (disk.Partitions, error) {
			return disk.Partitions{}, nil
		},
	}

	// Run check
	result := c.Check()
	cr, ok := result.(*checkResult)
	require.True(t, ok)

	// Should be healthy and NOT suggest any actions
	assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
	assert.Nil(t, cr.suggestedActions)
}

// TestCheckResult_GetSuggestedActions tests the getSuggestedActions method
func TestCheckResult_GetSuggestedActions(t *testing.T) {
	// Test with nil checkResult
	var nilCR *checkResult
	actions := nilCR.getSuggestedActions()
	assert.Nil(t, actions)

	// Test with no suggested actions
	cr := &checkResult{}
	actions = cr.getSuggestedActions()
	assert.Nil(t, actions)

	// Test with suggested actions
	expectedActions := &apiv1.SuggestedActions{
		RepairActions: []apiv1.RepairActionType{
			apiv1.RepairActionTypeRebootSystem,
			apiv1.RepairActionTypeHardwareInspection,
		},
	}
	cr = &checkResult{
		suggestedActions: expectedActions,
	}
	actions = cr.getSuggestedActions()
	assert.Equal(t, expectedActions, actions)
}

// TestCheckResult_HealthStates_WithSuggestedActions tests HealthStates includes suggested actions
func TestCheckResult_HealthStates_WithSuggestedActions(t *testing.T) {
	testTime := time.Now()
	suggestedActions := &apiv1.SuggestedActions{
		RepairActions: []apiv1.RepairActionType{
			apiv1.RepairActionTypeRebootSystem,
		},
	}

	cr := &checkResult{
		ts:               testTime,
		health:           apiv1.HealthStateTypeUnhealthy,
		reason:           "RAID array failure detected",
		suggestedActions: suggestedActions,
	}

	states := cr.HealthStates()
	assert.Len(t, states, 1)

	state := states[0]
	assert.Equal(t, Name, state.Name)
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, state.Health)
	assert.Equal(t, "RAID array failure detected", state.Reason)
	assert.Equal(t, suggestedActions, state.SuggestedActions)
}

// TestRecordDiskFailureEvent tests the recordDiskFailureEvent method
func TestRecordDiskFailureEvent(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	t.Run("successful insert", func(t *testing.T) {
		eventBucket := &simpleMockEventBucket{}
		c := &component{
			ctx:         ctx,
			eventBucket: eventBucket,
		}

		cr := &checkResult{
			ts: time.Now(),
		}

		err := c.recordDiskFailureEvent(cr, eventRAIDArrayFailure, "RAID array failure")
		assert.NoError(t, err)

		// Verify event was inserted
		events, err := eventBucket.Get(ctx, time.Time{})
		assert.NoError(t, err)
		assert.Len(t, events, 1)
		assert.Equal(t, eventRAIDArrayFailure, events[0].Name)
	})

	t.Run("event already exists", func(t *testing.T) {
		eventBucket := &simpleMockEventBucket{}
		c := &component{
			ctx:         ctx,
			eventBucket: eventBucket,
		}

		cr := &checkResult{
			ts: time.Now(),
		}

		// Insert the same event twice
		err := c.recordDiskFailureEvent(cr, eventFilesystemReadOnly, "Filesystem read-only")
		assert.NoError(t, err)

		err = c.recordDiskFailureEvent(cr, eventFilesystemReadOnly, "Filesystem read-only")
		assert.NoError(t, err) // Should not error when event already exists

		// Verify only one event exists
		events, err := eventBucket.Get(ctx, time.Time{})
		assert.NoError(t, err)
		assert.Len(t, events, 1)
	})

	t.Run("event bucket error", func(t *testing.T) {
		// Create a mock event bucket that returns errors
		mockBucket := &simpleMockEventBucket{
			findErr: errors.New("find error"),
		}

		c := &component{
			ctx:         ctx,
			eventBucket: mockBucket,
		}

		cr := &checkResult{
			ts: time.Now(),
		}

		err := c.recordDiskFailureEvent(cr, eventNVMePathFailure, "NVMe failure")
		assert.Error(t, err)
		assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
		assert.Equal(t, "error finding disk failure event", cr.reason)
		assert.NotNil(t, cr.err)
	})
}

// simpleMockEventBucket is a simple mock implementation of eventstore.Bucket for testing
type simpleMockEventBucket struct {
	findErr   error
	insertErr error
	events    eventstore.Events
}

func (m *simpleMockEventBucket) Name() string {
	return "disk-test-bucket"
}

func (m *simpleMockEventBucket) Insert(ctx context.Context, event eventstore.Event) error {
	if m.insertErr != nil {
		return m.insertErr
	}
	m.events = append(m.events, event)
	return nil
}

func (m *simpleMockEventBucket) Find(ctx context.Context, event eventstore.Event) (*eventstore.Event, error) {
	if m.findErr != nil {
		return nil, m.findErr
	}
	for _, e := range m.events {
		if e.Name == event.Name && e.Component == event.Component {
			return &e, nil
		}
	}
	return nil, nil
}

func (m *simpleMockEventBucket) Get(ctx context.Context, since time.Time) (eventstore.Events, error) {
	var result eventstore.Events
	for _, e := range m.events {
		if e.Time.After(since) || e.Time.Equal(since) {
			result = append(result, e)
		}
	}
	return result, nil
}

func (m *simpleMockEventBucket) Close() {}

func (m *simpleMockEventBucket) Latest(ctx context.Context) (*eventstore.Event, error) {
	if len(m.events) == 0 {
		return nil, nil
	}
	latest := m.events[0]
	for _, e := range m.events {
		if e.Time.After(latest.Time) {
			latest = e
		}
	}
	return &latest, nil
}

func (m *simpleMockEventBucket) Purge(ctx context.Context, beforeTimestamp int64) (int, error) {
	return 0, nil
}
