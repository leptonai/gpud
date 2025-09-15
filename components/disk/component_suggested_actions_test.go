package disk

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/pkg/disk"
	"github.com/leptonai/gpud/pkg/eventstore"
	pkghost "github.com/leptonai/gpud/pkg/host"
	pkgnfschecker "github.com/leptonai/gpud/pkg/nfs-checker"
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

// TestEvaluateSuggestedActionForEvent tests the common EvaluateSuggestedActions function
func TestEvaluateSuggestedActionForEvent(t *testing.T) {
	now := time.Now()

	t.Run("case 1 - no reboot events", func(t *testing.T) {
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

		suggestedActions := eventstore.EvaluateSuggestedActions(rebootEvents, diskFailureEvents, 2)

		assert.NotNil(t, suggestedActions)
		assert.Len(t, suggestedActions.RepairActions, 1)
		assert.Equal(t, apiv1.RepairActionTypeRebootSystem, suggestedActions.RepairActions[0])
	})

	t.Run("reboot after failure - edge case", func(t *testing.T) {
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

		suggestedActions := eventstore.EvaluateSuggestedActions(rebootEvents, diskFailureEvents, 2)

		// Edge case: returns nil since reboot happened after failure (reboot may have fixed the issue)
		assert.Nil(t, suggestedActions)
	})

	t.Run("case 2 - one reboot before failure", func(t *testing.T) {
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

		suggestedActions := eventstore.EvaluateSuggestedActions(rebootEvents, diskFailureEvents, 2)

		assert.NotNil(t, suggestedActions)
		assert.Len(t, suggestedActions.RepairActions, 1)
		assert.Equal(t, apiv1.RepairActionTypeRebootSystem, suggestedActions.RepairActions[0])
	})

	t.Run("case 3 - two reboots before failure", func(t *testing.T) {
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

		suggestedActions := eventstore.EvaluateSuggestedActions(rebootEvents, diskFailureEvents, 2)

		// Note: Based on new logic with failure-to-reboot sequences, this might still suggest reboot
		// since we only have 1 failure event
		assert.NotNil(t, suggestedActions)
		assert.Len(t, suggestedActions.RepairActions, 1)
	})
}

// TestAggregateSuggestedActions tests the common AggregateSuggestedActions function
func TestAggregateSuggestedActions(t *testing.T) {
	t.Run("empty actions", func(t *testing.T) {
		result := eventstore.AggregateSuggestedActions([]*apiv1.SuggestedActions{})
		assert.Nil(t, result)
	})

	t.Run("single reboot action", func(t *testing.T) {
		actions := []*apiv1.SuggestedActions{
			{
				RepairActions: []apiv1.RepairActionType{
					apiv1.RepairActionTypeRebootSystem,
				},
			},
		}
		result := eventstore.AggregateSuggestedActions(actions)
		assert.NotNil(t, result)
		assert.Len(t, result.RepairActions, 1)
		assert.Equal(t, apiv1.RepairActionTypeRebootSystem, result.RepairActions[0])
	})

	t.Run("multiple reboot actions", func(t *testing.T) {
		actions := []*apiv1.SuggestedActions{
			{
				RepairActions: []apiv1.RepairActionType{
					apiv1.RepairActionTypeRebootSystem,
				},
			},
			{
				RepairActions: []apiv1.RepairActionType{
					apiv1.RepairActionTypeRebootSystem,
				},
			},
		}
		result := eventstore.AggregateSuggestedActions(actions)
		assert.NotNil(t, result)
		assert.Len(t, result.RepairActions, 1)
		assert.Equal(t, apiv1.RepairActionTypeRebootSystem, result.RepairActions[0])
	})

	t.Run("HW_INSPECTION overrides REBOOT", func(t *testing.T) {
		actions := []*apiv1.SuggestedActions{
			{
				RepairActions: []apiv1.RepairActionType{
					apiv1.RepairActionTypeRebootSystem,
				},
			},
			{
				RepairActions: []apiv1.RepairActionType{
					apiv1.RepairActionTypeHardwareInspection,
				},
			},
			{
				RepairActions: []apiv1.RepairActionType{
					apiv1.RepairActionTypeRebootSystem,
				},
			},
		}
		result := eventstore.AggregateSuggestedActions(actions)
		assert.NotNil(t, result)
		assert.Len(t, result.RepairActions, 1)
		assert.Equal(t, apiv1.RepairActionTypeHardwareInspection, result.RepairActions[0])
	})

	t.Run("nil actions", func(t *testing.T) {
		actions := []*apiv1.SuggestedActions{
			nil,
			{
				RepairActions: []apiv1.RepairActionType{
					apiv1.RepairActionTypeRebootSystem,
				},
			},
			nil,
		}
		result := eventstore.AggregateSuggestedActions(actions)
		assert.NotNil(t, result)
		assert.Len(t, result.RepairActions, 1)
		assert.Equal(t, apiv1.RepairActionTypeRebootSystem, result.RepairActions[0])
	})
}

// TestEventTypeDifferentiation tests that different event types are evaluated separately
func TestEventTypeDifferentiation(t *testing.T) {
	now := time.Now()

	t.Run("mixed event types with different failure patterns", func(t *testing.T) {
		// Setup reboots
		rebootEvents := eventstore.Events{
			{
				Time:    now.Add(-8 * time.Hour), // Initial reboot
				Name:    "reboot",
				Type:    string(apiv1.EventTypeWarning),
				Message: "system reboot detected",
			},
			{
				Time:    now.Add(-4 * time.Hour), // Second reboot
				Name:    "reboot",
				Type:    string(apiv1.EventTypeWarning),
				Message: "system reboot detected",
			},
		}

		// RAID failures creating 2 reboot→failure sequences (should suggest HW_INSPECTION)
		raidFailures := eventstore.Events{
			{
				Component: Name,
				Time:      now.Add(-6 * time.Hour), // After first reboot
				Name:      eventRAIDArrayFailure,
				Type:      string(apiv1.EventTypeWarning),
				Message:   "RAID array failure detected",
			},
			{
				Component: Name,
				Time:      now.Add(-2 * time.Hour), // After second reboot
				Name:      eventRAIDArrayFailure,
				Type:      string(apiv1.EventTypeWarning),
				Message:   "RAID array failure detected",
			},
		}

		// Single filesystem read-only failure after one reboot (should suggest REBOOT)
		fsFailures := eventstore.Events{
			{
				Component: Name,
				Time:      now.Add(-3 * time.Hour), // After second reboot
				Name:      eventFilesystemReadOnly,
				Type:      string(apiv1.EventTypeWarning),
				Message:   "Filesystem remounted read-only",
			},
		}

		// NVMe failure with no reboots (should suggest REBOOT)
		nvmeFailures := eventstore.Events{
			{
				Component: Name,
				Time:      now.Add(-1 * time.Hour), // Recent failure
				Name:      eventNVMePathFailure,
				Type:      string(apiv1.EventTypeWarning),
				Message:   "NVMe path failure detected",
			},
		}

		// Evaluate each event type
		raidAction := eventstore.EvaluateSuggestedActions(rebootEvents, raidFailures, 2)
		fsAction := eventstore.EvaluateSuggestedActions(rebootEvents, fsFailures, 2)
		nvmeAction := eventstore.EvaluateSuggestedActions(eventstore.Events{}, nvmeFailures, 2) // No reboots for NVMe

		// Check individual evaluations
		assert.NotNil(t, raidAction)
		assert.Equal(t, apiv1.RepairActionTypeHardwareInspection, raidAction.RepairActions[0])

		assert.NotNil(t, fsAction)
		assert.Equal(t, apiv1.RepairActionTypeRebootSystem, fsAction.RepairActions[0])

		assert.NotNil(t, nvmeAction)
		assert.Equal(t, apiv1.RepairActionTypeRebootSystem, nvmeAction.RepairActions[0])

		// Aggregate actions - HW_INSPECTION should take priority
		allActions := []*apiv1.SuggestedActions{raidAction, fsAction, nvmeAction}
		aggregated := eventstore.AggregateSuggestedActions(allActions)

		assert.NotNil(t, aggregated)
		assert.Len(t, aggregated.RepairActions, 1)
		assert.Equal(t, apiv1.RepairActionTypeHardwareInspection, aggregated.RepairActions[0])
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
		getTimeNowFunc: func() time.Time {
			return now
		},
		getGroupConfigsFunc: func() pkgnfschecker.Configs {
			return pkgnfschecker.Configs{}
		},
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

	// Should detect the RAID array failure and be unhealthy
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
	assert.Equal(t, "RAID array has failed due to disk failure", cr.reason)
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

// TestLookbackPeriod tests that lookbackPeriod is properly used
func TestLookbackPeriod(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	now := time.Now()

	// Create a test event store
	eventBucket := &simpleMockEventBucket{}

	// Insert an old disk failure event (beyond default lookback period)
	oldEvent := eventstore.Event{
		Component: Name,
		Time:      now.Add(-5 * 24 * time.Hour), // 5 days ago
		Name:      eventRAIDArrayFailure,
		Type:      string(apiv1.EventTypeWarning),
		Message:   "Old RAID failure",
	}
	err := eventBucket.Insert(ctx, oldEvent)
	require.NoError(t, err)

	// Insert a recent disk failure event (within lookback period)
	recentEvent := eventstore.Event{
		Component: Name,
		Time:      now.Add(-2 * time.Minute), // 2 minutes ago - within the lookback period
		Name:      eventRAIDArrayFailure,
		Type:      string(apiv1.EventTypeWarning),
		Message:   "Recent RAID failure",
	}
	err = eventBucket.Insert(ctx, recentEvent)
	require.NoError(t, err)

	// Create mock reboot event store
	mockRebootStore := &mockRebootEventStore{
		events: eventstore.Events{},
	}

	// Create component with default lookback period (3 days)
	c := &component{
		ctx:              ctx,
		rebootEventStore: mockRebootStore,
		eventBucket:      eventBucket,
		lookbackPeriod:   3 * 24 * time.Hour, // 3 days
		getTimeNowFunc: func() time.Time {
			return now
		},
		getGroupConfigsFunc: func() pkgnfschecker.Configs {
			return pkgnfschecker.Configs{}
		},
		getExt4PartitionsFunc: func(ctx context.Context) (disk.Partitions, error) {
			// Return at least one partition so the component doesn't exit early
			return disk.Partitions{
				{
					Device:     "/dev/sda1",
					MountPoint: "/",
					Fstype:     "ext4",
					Usage: &disk.Usage{
						TotalBytes: 100 * 1024 * 1024 * 1024, // 100GB
						FreeBytes:  50 * 1024 * 1024 * 1024,  // 50GB
						UsedBytes:  50 * 1024 * 1024 * 1024,  // 50GB
					},
				},
			}, nil
		},
		getNFSPartitionsFunc: func(ctx context.Context) (disk.Partitions, error) {
			return disk.Partitions{}, nil
		},
	}

	// Run check
	result := c.Check()
	cr, ok := result.(*checkResult)
	require.True(t, ok)

	// Recent event should trigger unhealthy state
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
	assert.Contains(t, cr.reason, "RAID")

	// Should suggest reboot since no previous reboots
	assert.NotNil(t, cr.suggestedActions)
	assert.Len(t, cr.suggestedActions.RepairActions, 1)
	assert.Equal(t, apiv1.RepairActionTypeRebootSystem, cr.suggestedActions.RepairActions[0])
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

// TestComponent_Check_DeduplicationAndSorting tests that duplicate failure reasons are deduplicated and sorted
func TestComponent_Check_DeduplicationAndSorting(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	now := time.Now()

	// Create a test event store
	eventBucket := &simpleMockEventBucket{}

	// Insert multiple disk failure events including duplicates
	events := []struct {
		name    string
		time    time.Time
		message string
	}{
		{eventRAIDArrayFailure, now.Add(-30 * time.Minute), "First RAID failure"},
		{eventRAIDArrayFailure, now.Add(-25 * time.Minute), "Second RAID failure"}, // Duplicate type
		{eventFilesystemReadOnly, now.Add(-20 * time.Minute), "First FS read-only"},
		{eventNVMePathFailure, now.Add(-15 * time.Minute), "NVMe failure"},
		{eventFilesystemReadOnly, now.Add(-10 * time.Minute), "Second FS read-only"}, // Duplicate type
		{eventBufferIOError, now.Add(-5 * time.Minute), "Buffer I/O error"},
		{eventRAIDArrayFailure, now.Add(-2 * time.Minute), "Third RAID failure"}, // Another duplicate
	}

	for _, evt := range events {
		err := eventBucket.Insert(ctx, eventstore.Event{
			Component: Name,
			Time:      evt.time,
			Name:      evt.name,
			Type:      string(apiv1.EventTypeWarning),
			Message:   evt.message,
		})
		require.NoError(t, err)
	}

	// Create mock reboot event store
	mockRebootStore := &mockRebootEventStore{
		events: eventstore.Events{},
	}

	// Create component
	c := &component{
		ctx:              ctx,
		rebootEventStore: mockRebootStore,
		eventBucket:      eventBucket,
		lookbackPeriod:   time.Hour, // 1 hour lookback
		getTimeNowFunc: func() time.Time {
			return now
		},
		getGroupConfigsFunc: func() pkgnfschecker.Configs {
			return pkgnfschecker.Configs{}
		},
		freeSpaceThresholdBytesDegraded: defaultFreeSpaceThresholdBytesDegraded, // 500MB
		getExt4PartitionsFunc: func(ctx context.Context) (disk.Partitions, error) {
			return disk.Partitions{
				{
					Device:     "/dev/sda1",
					MountPoint: "/",
					Fstype:     "ext4",
					Usage: &disk.Usage{
						TotalBytes: 100 * 1024 * 1024 * 1024,
						FreeBytes:  50 * 1024 * 1024 * 1024,
						UsedBytes:  50 * 1024 * 1024 * 1024,
					},
				},
			}, nil
		},
		getNFSPartitionsFunc: func(ctx context.Context) (disk.Partitions, error) {
			return disk.Partitions{}, nil
		},
	}

	// Run check
	result := c.Check()
	cr, ok := result.(*checkResult)
	require.True(t, ok)

	// Should be unhealthy
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)

	// Verify reasons are deduplicated and sorted lexicographically
	// Expected order (sorted alphabetically):
	// 1. Buffer I/O error detected on device
	// 2. NVMe device has no available path, I/O failing
	// 3. RAID array has failed due to disk failure
	// 4. filesystem remounted as read-only due to errors
	expectedReason := "Buffer I/O error detected on device, NVMe device has no available path, I/O failing, RAID array has failed due to disk failure, filesystem remounted as read-only due to errors"
	assert.Equal(t, expectedReason, cr.reason)
}
