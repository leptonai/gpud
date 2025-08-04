package disk

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/pkg/disk"
	"github.com/leptonai/gpud/pkg/eventstore"
)

// TestDiskFailureResolutionAfterReboot tests that when disk failures occur before a reboot
// but are not present after the reboot, the component correctly reports healthy state
// with no error messages or suggested actions.
func TestDiskFailureResolutionAfterReboot(t *testing.T) {
	now := time.Now()

	// Helper function to create a test component with mocked dependencies
	createTestComponentWithEvents := func(diskEvents eventstore.Events, rebootEvents eventstore.Events) (*component, *mockEventBucket, *mockRebootEventStore) {
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		// Create mock event bucket
		mockBucket := &mockEventBucket{}
		// Use mock.MatchedBy to match any time argument
		mockBucket.On("Get", ctx, mock.MatchedBy(func(t time.Time) bool { return true })).Return(diskEvents, nil)
		mockBucket.On("Close").Return()

		// Create mock reboot event store
		mockRebootStore := &mockRebootEventStore{
			events: rebootEvents,
		}

		// Create component with mocks
		c := &component{
			ctx:                     ctx,
			cancel:                  cancel,
			retryInterval:           defaultRetryInterval,
			lookbackPeriod:          defaultLookbackPeriod,
			eventBucket:             mockBucket,
			rebootEventStore:        mockRebootStore,
			mountPointsToTrackUsage: map[string]struct{}{},
			getTimeNowFunc: func() time.Time {
				return now
			},
			getExt4PartitionsFunc: func(ctx context.Context) (disk.Partitions, error) {
				return disk.Partitions{}, nil
			},
			getNFSPartitionsFunc: func(ctx context.Context) (disk.Partitions, error) {
				return disk.Partitions{}, nil
			},
		}

		return c, mockBucket, mockRebootStore
	}

	// Test case: RAID array failure resolved after reboot
	t.Run("RAID array failure resolved after reboot", func(t *testing.T) {
		// Disk failure event occurred before reboot
		diskEvents := eventstore.Events{
			{
				Component: Name,
				Time:      now.Add(-2 * time.Hour), // Failure 2 hours ago
				Name:      eventRAIDArrayFailure,
				Type:      string(apiv1.EventTypeWarning),
				Message:   "RAID array failure detected",
			},
		}

		// Reboot occurred after the failure
		rebootEvents := eventstore.Events{
			{
				Time:    now.Add(-1 * time.Hour), // Reboot 1 hour ago (after failure)
				Name:    "reboot",
				Type:    string(apiv1.EventTypeWarning),
				Message: "system reboot detected",
			},
		}

		c, mockBucket, _ := createTestComponentWithEvents(diskEvents, rebootEvents)
		defer func() {
			c.Close()
			mockBucket.AssertExpectations(t)
		}()

		// Run check
		result := c.Check()
		cr := result.(*checkResult)

		// Verify the component reports healthy state
		assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
		assert.Equal(t, "ok", cr.reason)   // No error message
		assert.Nil(t, cr.suggestedActions) // No suggested actions
		assert.Nil(t, cr.err)
	})

	// Test case: Filesystem read-only error resolved after reboot
	t.Run("Filesystem read-only error resolved after reboot", func(t *testing.T) {
		diskEvents := eventstore.Events{
			{
				Component: Name,
				Time:      now.Add(-3 * time.Hour),
				Name:      eventFilesystemReadOnly,
				Type:      string(apiv1.EventTypeWarning),
				Message:   "Filesystem remounted read-only",
			},
		}

		rebootEvents := eventstore.Events{
			{
				Time:    now.Add(-2 * time.Hour),
				Name:    "reboot",
				Type:    string(apiv1.EventTypeWarning),
				Message: "system reboot detected",
			},
		}

		c, mockBucket, _ := createTestComponentWithEvents(diskEvents, rebootEvents)
		defer func() {
			c.Close()
			mockBucket.AssertExpectations(t)
		}()

		result := c.Check()
		cr := result.(*checkResult)

		assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
		assert.Equal(t, "ok", cr.reason)
		assert.Nil(t, cr.suggestedActions)
	})

	// Test case: NVMe path failure resolved after reboot
	t.Run("NVMe path failure resolved after reboot", func(t *testing.T) {
		diskEvents := eventstore.Events{
			{
				Component: Name,
				Time:      now.Add(-4 * time.Hour),
				Name:      eventNVMePathFailure,
				Type:      string(apiv1.EventTypeWarning),
				Message:   "NVMe path failure detected",
			},
		}

		rebootEvents := eventstore.Events{
			{
				Time:    now.Add(-3 * time.Hour),
				Name:    "reboot",
				Type:    string(apiv1.EventTypeWarning),
				Message: "system reboot detected",
			},
		}

		c, mockBucket, _ := createTestComponentWithEvents(diskEvents, rebootEvents)
		defer func() {
			c.Close()
			mockBucket.AssertExpectations(t)
		}()

		result := c.Check()
		cr := result.(*checkResult)

		assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
		assert.Equal(t, "ok", cr.reason)
		assert.Nil(t, cr.suggestedActions)
	})

	// Test case: NVMe timeout resolved after reboot
	t.Run("NVMe timeout resolved after reboot", func(t *testing.T) {
		diskEvents := eventstore.Events{
			{
				Component: Name,
				Time:      now.Add(-5 * time.Hour),
				Name:      eventNVMeTimeout,
				Type:      string(apiv1.EventTypeWarning),
				Message:   "NVME controller timeout detected",
			},
		}

		rebootEvents := eventstore.Events{
			{
				Time:    now.Add(-4 * time.Hour),
				Name:    "reboot",
				Type:    string(apiv1.EventTypeWarning),
				Message: "system reboot detected",
			},
		}

		c, mockBucket, _ := createTestComponentWithEvents(diskEvents, rebootEvents)
		defer func() {
			c.Close()
			mockBucket.AssertExpectations(t)
		}()

		result := c.Check()
		cr := result.(*checkResult)

		assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
		assert.Equal(t, "ok", cr.reason)
		assert.Nil(t, cr.suggestedActions)
	})

	// Test case: NVMe device disabled error resolved after reboot
	t.Run("NVMe device disabled error resolved after reboot", func(t *testing.T) {
		diskEvents := eventstore.Events{
			{
				Component: Name,
				Time:      now.Add(-6 * time.Hour),
				Name:      eventNVMeDeviceDisabled,
				Type:      string(apiv1.EventTypeWarning),
				Message:   "NVME device disabled after reset failure",
			},
		}

		rebootEvents := eventstore.Events{
			{
				Time:    now.Add(-5 * time.Hour),
				Name:    "reboot",
				Type:    string(apiv1.EventTypeWarning),
				Message: "system reboot detected",
			},
		}

		c, mockBucket, _ := createTestComponentWithEvents(diskEvents, rebootEvents)
		defer func() {
			c.Close()
			mockBucket.AssertExpectations(t)
		}()

		result := c.Check()
		cr := result.(*checkResult)

		assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
		assert.Equal(t, "ok", cr.reason)
		assert.Nil(t, cr.suggestedActions)
	})

	// Test case: I/O beyond device boundaries error resolved after reboot
	t.Run("I/O beyond device boundaries error resolved after reboot", func(t *testing.T) {
		diskEvents := eventstore.Events{
			{
				Component: Name,
				Time:      now.Add(-7 * time.Hour),
				Name:      eventBeyondEndOfDevice,
				Type:      string(apiv1.EventTypeWarning),
				Message:   "I/O beyond device boundaries detected",
			},
		}

		rebootEvents := eventstore.Events{
			{
				Time:    now.Add(-6 * time.Hour),
				Name:    "reboot",
				Type:    string(apiv1.EventTypeWarning),
				Message: "system reboot detected",
			},
		}

		c, mockBucket, _ := createTestComponentWithEvents(diskEvents, rebootEvents)
		defer func() {
			c.Close()
			mockBucket.AssertExpectations(t)
		}()

		result := c.Check()
		cr := result.(*checkResult)

		assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
		assert.Equal(t, "ok", cr.reason)
		assert.Nil(t, cr.suggestedActions)
	})

	// Test case: Buffer I/O error resolved after reboot
	t.Run("Buffer I/O error resolved after reboot", func(t *testing.T) {
		diskEvents := eventstore.Events{
			{
				Component: Name,
				Time:      now.Add(-8 * time.Hour),
				Name:      eventBufferIOError,
				Type:      string(apiv1.EventTypeWarning),
				Message:   "Buffer I/O error detected on device",
			},
		}

		rebootEvents := eventstore.Events{
			{
				Time:    now.Add(-7 * time.Hour),
				Name:    "reboot",
				Type:    string(apiv1.EventTypeWarning),
				Message: "system reboot detected",
			},
		}

		c, mockBucket, _ := createTestComponentWithEvents(diskEvents, rebootEvents)
		defer func() {
			c.Close()
			mockBucket.AssertExpectations(t)
		}()

		result := c.Check()
		cr := result.(*checkResult)

		assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
		assert.Equal(t, "ok", cr.reason)
		assert.Nil(t, cr.suggestedActions)
	})

	// Test case: Multiple different failure types all resolved after reboot
	t.Run("Multiple failure types resolved after reboot", func(t *testing.T) {
		diskEvents := eventstore.Events{
			{
				Component: Name,
				Time:      now.Add(-5 * time.Hour),
				Name:      eventRAIDArrayFailure,
				Type:      string(apiv1.EventTypeWarning),
				Message:   "RAID array failure detected",
			},
			{
				Component: Name,
				Time:      now.Add(-4*time.Hour - 30*time.Minute),
				Name:      eventFilesystemReadOnly,
				Type:      string(apiv1.EventTypeWarning),
				Message:   "Filesystem remounted read-only",
			},
			{
				Component: Name,
				Time:      now.Add(-4 * time.Hour),
				Name:      eventNVMePathFailure,
				Type:      string(apiv1.EventTypeWarning),
				Message:   "NVMe path failure detected",
			},
		}

		rebootEvents := eventstore.Events{
			{
				Time:    now.Add(-3 * time.Hour),
				Name:    "reboot",
				Type:    string(apiv1.EventTypeWarning),
				Message: "system reboot detected",
			},
		}

		c, mockBucket, _ := createTestComponentWithEvents(diskEvents, rebootEvents)
		defer func() {
			c.Close()
			mockBucket.AssertExpectations(t)
		}()

		result := c.Check()
		cr := result.(*checkResult)

		assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
		assert.Equal(t, "ok", cr.reason)
		assert.Nil(t, cr.suggestedActions)
	})

	// Test case: Mixed scenario - some failures resolved, one persists after reboot
	t.Run("Mixed scenario - some resolved, one persists", func(t *testing.T) {
		diskEvents := eventstore.Events{
			// These occurred before reboot
			{
				Component: Name,
				Time:      now.Add(-5 * time.Hour),
				Name:      eventRAIDArrayFailure,
				Type:      string(apiv1.EventTypeWarning),
				Message:   "RAID array failure detected",
			},
			{
				Component: Name,
				Time:      now.Add(-4 * time.Hour),
				Name:      eventFilesystemReadOnly,
				Type:      string(apiv1.EventTypeWarning),
				Message:   "Filesystem remounted read-only",
			},
			// This one occurred after reboot (persists)
			{
				Component: Name,
				Time:      now.Add(-30 * time.Minute),
				Name:      eventNVMePathFailure,
				Type:      string(apiv1.EventTypeWarning),
				Message:   "NVMe path failure detected",
			},
		}

		rebootEvents := eventstore.Events{
			{
				Time:    now.Add(-3 * time.Hour),
				Name:    "reboot",
				Type:    string(apiv1.EventTypeWarning),
				Message: "system reboot detected",
			},
		}

		c, mockBucket, _ := createTestComponentWithEvents(diskEvents, rebootEvents)
		defer func() {
			c.Close()
			mockBucket.AssertExpectations(t)
		}()

		result := c.Check()
		cr := result.(*checkResult)

		// Should be unhealthy because NVMe failure persists
		assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
		assert.Contains(t, cr.reason, "NVMe device has no available path, I/O failing")
		assert.NotNil(t, cr.suggestedActions)
		assert.Contains(t, cr.suggestedActions.RepairActions, apiv1.RepairActionTypeRebootSystem)

		// Should NOT contain the resolved failures
		assert.NotContains(t, cr.reason, "RAID array has failed due to disk failure")
		assert.NotContains(t, cr.reason, "filesystem remounted as read-only due to errors")
	})

	// Test case: No disk events - verify healthy state
	t.Run("No disk events", func(t *testing.T) {
		diskEvents := eventstore.Events{} // No disk failure events
		rebootEvents := eventstore.Events{}

		c, mockBucket, _ := createTestComponentWithEvents(diskEvents, rebootEvents)
		defer func() {
			c.Close()
			mockBucket.AssertExpectations(t)
		}()

		result := c.Check()
		cr := result.(*checkResult)

		assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
		assert.Equal(t, "ok", cr.reason)
		assert.Nil(t, cr.suggestedActions)
	})
}

// TestFailureReasonMapCleanup verifies that the failureReasons map is properly
// cleaned up when failures are resolved after reboot
func TestFailureReasonMapCleanup(t *testing.T) {
	now := time.Now()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	// Create a component with all failure types before reboot
	allFailureEvents := eventstore.Events{
		{
			Component: Name,
			Time:      now.Add(-7 * time.Hour),
			Name:      eventRAIDArrayFailure,
			Type:      string(apiv1.EventTypeWarning),
			Message:   "RAID array failure detected",
		},
		{
			Component: Name,
			Time:      now.Add(-6*time.Hour - 50*time.Minute),
			Name:      eventFilesystemReadOnly,
			Type:      string(apiv1.EventTypeWarning),
			Message:   "Filesystem remounted read-only",
		},
		{
			Component: Name,
			Time:      now.Add(-6*time.Hour - 40*time.Minute),
			Name:      eventNVMePathFailure,
			Type:      string(apiv1.EventTypeWarning),
			Message:   "NVMe path failure detected",
		},
		{
			Component: Name,
			Time:      now.Add(-6*time.Hour - 30*time.Minute),
			Name:      eventNVMeTimeout,
			Type:      string(apiv1.EventTypeWarning),
			Message:   "NVME controller timeout detected",
		},
		{
			Component: Name,
			Time:      now.Add(-6*time.Hour - 20*time.Minute),
			Name:      eventNVMeDeviceDisabled,
			Type:      string(apiv1.EventTypeWarning),
			Message:   "NVME device disabled after reset failure",
		},
		{
			Component: Name,
			Time:      now.Add(-6*time.Hour - 10*time.Minute),
			Name:      eventBeyondEndOfDevice,
			Type:      string(apiv1.EventTypeWarning),
			Message:   "I/O beyond device boundaries detected",
		},
		{
			Component: Name,
			Time:      now.Add(-6 * time.Hour),
			Name:      eventBufferIOError,
			Type:      string(apiv1.EventTypeWarning),
			Message:   "Buffer I/O error detected on device",
		},
	}

	rebootEvents := eventstore.Events{
		{
			Time:    now.Add(-5 * time.Hour),
			Name:    "reboot",
			Type:    string(apiv1.EventTypeWarning),
			Message: "system reboot detected",
		},
	}

	mockBucket := &mockEventBucket{}
	mockBucket.On("Get", ctx, mock.MatchedBy(func(t time.Time) bool { return true })).Return(allFailureEvents, nil)
	mockBucket.On("Close").Return()

	mockRebootStore := &mockRebootEventStore{
		events: rebootEvents,
	}

	c := &component{
		ctx:                     ctx,
		cancel:                  cancel,
		retryInterval:           defaultRetryInterval,
		lookbackPeriod:          defaultLookbackPeriod,
		eventBucket:             mockBucket,
		rebootEventStore:        mockRebootStore,
		mountPointsToTrackUsage: map[string]struct{}{},
		getTimeNowFunc: func() time.Time {
			return now
		},
		getExt4PartitionsFunc: func(ctx context.Context) (disk.Partitions, error) {
			return disk.Partitions{}, nil
		},
		getNFSPartitionsFunc: func(ctx context.Context) (disk.Partitions, error) {
			return disk.Partitions{}, nil
		},
	}

	result := c.Check()
	cr := result.(*checkResult)

	// All failures should be resolved after reboot
	assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
	assert.Equal(t, "ok", cr.reason)
	assert.Nil(t, cr.suggestedActions)

	// Verify no failure reasons remain in the reason string
	failureMessages := []string{
		"RAID array failure detected",
		"Filesystem remounted read-only",
		"NVMe path failure detected",
		"NVME controller timeout detected",
		"NVME device disabled after reset failure",
		"I/O beyond device boundaries detected",
		"Buffer I/O error detected on device",
	}

	for _, msg := range failureMessages {
		assert.NotContains(t, cr.reason, msg, "Failure reason should be cleaned up: %s", msg)
	}

	c.Close()
	mockBucket.AssertExpectations(t)
}
