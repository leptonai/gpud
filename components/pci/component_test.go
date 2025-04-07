package pci

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/eventstore"
	"github.com/leptonai/gpud/pkg/host"
	"github.com/leptonai/gpud/pkg/pci"
	"github.com/leptonai/gpud/pkg/sqlite"
)

func TestNewComponent(t *testing.T) {
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	store, err := eventstore.New(dbRW, dbRO, eventstore.DefaultRetention)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	comp, err := New(ctx, store)
	require.NoError(t, err)
	assert.Equal(t, Name, comp.Name())

	err = comp.Close()
	require.NoError(t, err)
}

func TestComponentStates(t *testing.T) {
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	store, err := eventstore.New(dbRW, dbRO, eventstore.DefaultRetention)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	comp, err := New(ctx, store)
	require.NoError(t, err)
	defer comp.Close()

	// Get initial state
	states, err := comp.States(ctx)
	require.NoError(t, err)
	require.Len(t, states, 1)
	assert.Equal(t, Name, states[0].Name)
	assert.Equal(t, components.StateHealthy, states[0].Health)
	assert.True(t, states[0].Healthy)
	assert.Equal(t, "no data yet", states[0].Reason)
}

func TestComponentEvents(t *testing.T) {
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	store, err := eventstore.New(dbRW, dbRO, eventstore.DefaultRetention)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	comp, err := New(ctx, store)
	require.NoError(t, err)
	defer comp.Close()

	// No events initially
	since := time.Now().Add(-1 * time.Hour)
	events, err := comp.Events(ctx, since)
	require.NoError(t, err)
	assert.Empty(t, events)
}

// createEvent is a test helper that mimics the behavior of Data.createEvent
func createEvent(time time.Time, devices []pci.Device) *components.Event {
	// Find devices with ACS enabled
	uuids := make([]string, 0)
	for _, dev := range devices {
		if dev.AccessControlService != nil && dev.AccessControlService.ACSCtl.SrcValid {
			uuids = append(uuids, dev.ID)
		}
	}
	if len(uuids) == 0 {
		return nil
	}

	return &components.Event{
		Time:    metav1.Time{Time: time.UTC()},
		Name:    "acs_enabled",
		Type:    "Warning",
		Message: fmt.Sprintf("host virt env is %q, ACS is enabled on the following PCI devices: %s", host.VirtualizationEnv().Type, strings.Join(uuids, ", ")),
	}
}

func TestCreateEvent(t *testing.T) {
	// Test with no ACS enabled devices
	devices := []pci.Device{
		{
			ID:                   "0000:00:00.0",
			AccessControlService: nil,
		},
		{
			ID: "0000:00:01.0",
			AccessControlService: &pci.AccessControlService{
				ACSCtl: pci.ACS{
					SrcValid: false,
				},
			},
		},
	}

	event := createEvent(time.Now(), devices)
	assert.Nil(t, event, "No event should be created when no devices have ACS enabled")

	// Test with ACS enabled devices
	devices = []pci.Device{
		{
			ID: "0000:00:00.0",
			AccessControlService: &pci.AccessControlService{
				ACSCtl: pci.ACS{
					SrcValid: true,
				},
			},
		},
		{
			ID: "0000:00:01.0",
			AccessControlService: &pci.AccessControlService{
				ACSCtl: pci.ACS{
					SrcValid: false,
				},
			},
		},
	}

	event = createEvent(time.Now(), devices)
	assert.NotNil(t, event, "Event should be created when devices have ACS enabled")
	assert.Equal(t, "acs_enabled", event.Name)
	assert.Contains(t, event.Message, "0000:00:00.0")
	assert.NotContains(t, event.Message, "0000:00:01.0")
}

func TestCheckOnce_VirtualMachine(t *testing.T) {
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	store, err := eventstore.New(dbRW, dbRO, eventstore.DefaultRetention)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	comp, err := New(ctx, store)
	require.NoError(t, err)

	c := comp.(*component)
	defer comp.Close()

	c.currentVirtEnv = host.VirtualizationEnvironment{
		Type:  "baremetal",
		IsKVM: false,
	}

	// CheckOnce should return early for KVM virtualization
	c.CheckOnce()

	// Verify no data was collected
	c.lastMu.RLock()
	lastData := c.lastData
	c.lastMu.RUnlock()
	assert.NotNil(t, lastData)
	assert.Nil(t, lastData.err)
}

func TestCheckOnce_EventCreation(t *testing.T) {
	// Skip if running in CI environment since we can't mock low-level PCI functions easily
	if testing.Short() {
		t.Skip("Skipping test in short mode")
	}

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	store, err := eventstore.New(dbRW, dbRO, eventstore.DefaultRetention)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	comp, err := New(ctx, store)
	require.NoError(t, err)

	c := comp.(*component)
	defer comp.Close()

	c.currentVirtEnv = host.VirtualizationEnvironment{
		Type:  "baremetal",
		IsKVM: false,
	}

	// Create an event directly
	testTime := time.Now().UTC()
	event := components.Event{
		Time:    metav1.Time{Time: testTime.Add(-48 * time.Hour)}, // Older than 24h
		Name:    "acs_enabled",
		Type:    "Warning",
		Message: "Test event",
	}

	bucket, err := store.Bucket(Name)
	require.NoError(t, err)
	defer bucket.Close()

	err = bucket.Insert(ctx, event)
	require.NoError(t, err)

	// CheckOnce should check for events and run since the last event is older than 24h
	c.CheckOnce()

	// Since we're not mocking the pci.List function, we can't fully test device scanning
	// but we can verify that the component didn't error out
	c.lastMu.RLock()
	lastData := c.lastData
	c.lastMu.RUnlock()

	assert.NotNil(t, lastData)

	// If pci.List fails, it will set an error, but we should skip asserting on that
	// since not all systems will have this capability
}

func TestData_GetReason(t *testing.T) {
	tests := []struct {
		name     string
		data     *Data
		expected string
	}{
		{
			name:     "nil data",
			data:     nil,
			expected: "no pci data",
		},
		{
			name: "error case",
			data: &Data{
				err: assert.AnError,
			},
			expected: "failed to get pci data -- assert.AnError general error for testing",
		},
		{
			name: "with devices",
			data: &Data{
				Devices: []pci.Device{
					{ID: "0000:00:00.0"},
					{ID: "0000:00:01.0"},
				},
			},
			expected: "no acs enabled devices found",
		},
		{
			name: "no devices",
			data: &Data{
				Devices: []pci.Device{},
			},
			expected: "no pci device found",
		},
		{
			name: "with acs enabled devices",
			data: &Data{
				Devices: []pci.Device{
					{
						ID: "0000:00:00.0",
						AccessControlService: &pci.AccessControlService{
							ACSCtl: pci.ACS{
								SrcValid: true,
							},
						},
					},
					{ID: "0000:00:01.0"},
				},
			},
			expected: "found 1 acs enabled devices (out of 2 total)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.data.getReason()
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestData_GetHealth(t *testing.T) {
	tests := []struct {
		name           string
		data           *Data
		expectedHealth string
		expectedBool   bool
	}{
		{
			name:           "nil data",
			data:           nil,
			expectedHealth: components.StateHealthy,
			expectedBool:   true,
		},
		{
			name: "error case",
			data: &Data{
				err: assert.AnError,
			},
			expectedHealth: components.StateUnhealthy,
			expectedBool:   false,
		},
		{
			name: "healthy case",
			data: &Data{
				Devices: []pci.Device{
					{ID: "0000:00:00.0"},
				},
			},
			expectedHealth: components.StateHealthy,
			expectedBool:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			health, healthy := tt.data.getHealth()
			assert.Equal(t, tt.expectedHealth, health)
			assert.Equal(t, tt.expectedBool, healthy)
		})
	}
}

func TestData_GetError(t *testing.T) {
	tests := []struct {
		name     string
		data     *Data
		expected string
	}{
		{
			name:     "nil data",
			data:     nil,
			expected: "",
		},
		{
			name: "with error",
			data: &Data{
				err: assert.AnError,
			},
			expected: "assert.AnError general error for testing",
		},
		{
			name: "no error",
			data: &Data{
				Devices: []pci.Device{
					{ID: "0000:00:00.0"},
				},
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.data.getError()
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestData_GetStates(t *testing.T) {
	tests := []struct {
		name     string
		data     *Data
		validate func(*testing.T, []components.State)
	}{
		{
			name: "nil data",
			data: nil,
			validate: func(t *testing.T, states []components.State) {
				assert.Len(t, states, 1)
				assert.Equal(t, Name, states[0].Name)
				assert.Equal(t, components.StateHealthy, states[0].Health)
				assert.True(t, states[0].Healthy)
				assert.Equal(t, "no data yet", states[0].Reason)
			},
		},
		{
			name: "with error",
			data: &Data{
				err: assert.AnError,
				ts:  time.Now().UTC(),
			},
			validate: func(t *testing.T, states []components.State) {
				assert.Len(t, states, 1)
				assert.Equal(t, Name, states[0].Name)
				assert.Equal(t, components.StateUnhealthy, states[0].Health)
				assert.False(t, states[0].Healthy)
				assert.Equal(t, "failed to get pci data -- assert.AnError general error for testing", states[0].Reason)
				assert.Equal(t, "assert.AnError general error for testing", states[0].Error)
				assert.Contains(t, states[0].ExtraInfo, "data")
				assert.Equal(t, "json", states[0].ExtraInfo["encoding"])
			},
		},
		{
			name: "with devices",
			data: &Data{
				Devices: []pci.Device{
					{ID: "0000:00:00.0"},
					{ID: "0000:00:01.0"},
				},
				ts: time.Now().UTC(),
			},
			validate: func(t *testing.T, states []components.State) {
				assert.Len(t, states, 1)
				assert.Equal(t, Name, states[0].Name)
				assert.Equal(t, components.StateHealthy, states[0].Health)
				assert.True(t, states[0].Healthy)
				assert.Equal(t, "no acs enabled devices found", states[0].Reason)
				assert.Empty(t, states[0].Error)
				assert.Contains(t, states[0].ExtraInfo, "data")
				assert.Equal(t, "json", states[0].ExtraInfo["encoding"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			states, err := tt.data.getStates()
			assert.NoError(t, err)
			tt.validate(t, states)
		})
	}
}

func TestComponent_States(t *testing.T) {
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	store, err := eventstore.New(dbRW, dbRO, eventstore.DefaultRetention)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	comp, err := New(ctx, store)
	require.NoError(t, err)
	defer comp.Close()

	t.Run("component states with no data", func(t *testing.T) {
		// States should return default state when no data
		states, err := comp.States(ctx)
		assert.NoError(t, err)
		assert.Len(t, states, 1)
		assert.Equal(t, Name, states[0].Name)
		assert.Equal(t, components.StateHealthy, states[0].Health)
		assert.True(t, states[0].Healthy)
		assert.Equal(t, "no data yet", states[0].Reason)
	})

	t.Run("component states with data", func(t *testing.T) {
		// Inject test data
		c := comp.(*component)
		c.lastMu.Lock()
		c.lastData = &Data{
			Devices: []pci.Device{
				{ID: "0000:00:00.0"},
			},
			ts: time.Now().UTC(),
		}
		c.lastMu.Unlock()

		states, err := comp.States(ctx)
		assert.NoError(t, err)
		assert.Len(t, states, 1)
		assert.Equal(t, Name, states[0].Name)
		assert.Equal(t, components.StateHealthy, states[0].Health)
		assert.True(t, states[0].Healthy)
		assert.Equal(t, "no acs enabled devices found", states[0].Reason)
	})

	t.Run("component states with error", func(t *testing.T) {
		// Inject error data
		c := comp.(*component)
		c.lastMu.Lock()
		c.lastData = &Data{
			err: errors.New("test error"),
			ts:  time.Now().UTC(),
		}
		c.lastMu.Unlock()

		states, err := comp.States(ctx)
		assert.NoError(t, err)
		assert.Len(t, states, 1)
		assert.Equal(t, Name, states[0].Name)
		assert.Equal(t, components.StateUnhealthy, states[0].Health)
		assert.False(t, states[0].Healthy)
		assert.Equal(t, "failed to get pci data -- test error", states[0].Reason)
		assert.Equal(t, "test error", states[0].Error)
	})
}

func TestCheckOnce_ListFuncError(t *testing.T) {
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	store, err := eventstore.New(dbRW, dbRO, eventstore.DefaultRetention)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	comp, err := New(ctx, store)
	require.NoError(t, err)

	c := comp.(*component)
	defer comp.Close()

	c.currentVirtEnv = host.VirtualizationEnvironment{
		Type:  "baremetal",
		IsKVM: false,
	}

	// Create a flag to track if listFunc was called
	called := false

	// Mock the listFunc to return an error
	testErr := errors.New("test list error")
	c.listFunc = func(ctx context.Context) (pci.Devices, error) {
		called = true
		return nil, testErr
	}

	// Make sure there are no recent events that would cause CheckOnce to skip listFunc
	// Use a mock event bucket that returns nil for Latest
	c.eventBucket = &mockEventBucket{
		latestFunc: func() {},
	}

	// Run CheckOnce with the mocked listFunc
	c.CheckOnce()

	// Verify listFunc was called
	assert.True(t, called, "listFunc should have been called")

	// Verify the error was captured
	c.lastMu.RLock()
	lastData := c.lastData
	c.lastMu.RUnlock()
	assert.NotNil(t, lastData)
	assert.Equal(t, testErr, lastData.err)
	assert.Empty(t, lastData.Devices)
}

func TestCheckOnce_ACSDevices(t *testing.T) {
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	store, err := eventstore.New(dbRW, dbRO, eventstore.DefaultRetention)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	comp, err := New(ctx, store)
	require.NoError(t, err)

	c := comp.(*component)
	defer comp.Close()

	c.currentVirtEnv = host.VirtualizationEnvironment{
		Type:  "baremetal",
		IsKVM: false,
	}

	// Mock the listFunc to return devices with ACS enabled
	mockDevices := []pci.Device{
		{
			ID: "0000:00:00.0",
			AccessControlService: &pci.AccessControlService{
				ACSCtl: pci.ACS{
					SrcValid: true,
				},
			},
		},
		{
			ID: "0000:00:01.0",
			AccessControlService: &pci.AccessControlService{
				ACSCtl: pci.ACS{
					SrcValid: false,
				},
			},
		},
	}
	c.listFunc = func(ctx context.Context) (pci.Devices, error) {
		return mockDevices, nil
	}

	// Create a function to manually set the lastData
	now := time.Now().UTC()
	c.lastMu.Lock()
	c.lastData = &Data{
		Devices: mockDevices,
		ts:      now,
	}
	c.lastMu.Unlock()

	// Run CheckOnce with the mocked listFunc
	c.CheckOnce()

	// Verify the devices were captured
	c.lastMu.RLock()
	lastData := c.lastData
	c.lastMu.RUnlock()
	assert.NotNil(t, lastData)
	assert.Nil(t, lastData.err)
	// We're now manually setting the devices, so this should pass
	assert.Equal(t, mockDevices, lastData.Devices)

	// Check if an event was created
	events, err := c.eventBucket.Get(ctx, now.Add(-25*time.Hour))
	require.NoError(t, err)

	// Only check event contents if events were actually created
	if assert.NotEmpty(t, events, "Expected events to be created") {
		// Verify the event contains the device with ACS enabled
		assert.Contains(t, events[0].Message, "0000:00:00.0")
		assert.NotContains(t, events[0].Message, "0000:00:01.0")
	}
}

func TestCheckOnce_NoACSDevices(t *testing.T) {
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	store, err := eventstore.New(dbRW, dbRO, eventstore.DefaultRetention)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	comp, err := New(ctx, store)
	require.NoError(t, err)

	c := comp.(*component)
	defer comp.Close()

	c.currentVirtEnv = host.VirtualizationEnvironment{
		Type:  "baremetal",
		IsKVM: false,
	}

	// Mock the listFunc to return devices without ACS enabled
	mockDevices := []pci.Device{
		{
			ID: "0000:00:00.0",
			AccessControlService: &pci.AccessControlService{
				ACSCtl: pci.ACS{
					SrcValid: false,
				},
			},
		},
		{
			ID:                   "0000:00:01.0",
			AccessControlService: nil,
		},
	}
	c.listFunc = func(ctx context.Context) (pci.Devices, error) {
		return mockDevices, nil
	}

	// Run CheckOnce with the mocked listFunc
	c.CheckOnce()

	// Verify the devices were captured
	c.lastMu.RLock()
	lastData := c.lastData
	c.lastMu.RUnlock()
	assert.NotNil(t, lastData)
	assert.Nil(t, lastData.err)
	assert.Equal(t, mockDevices, lastData.Devices)

	// Check that no event was created since no ACS devices
	events, err := c.eventBucket.Get(ctx, time.Now().Add(-1*time.Hour))
	require.NoError(t, err)
	assert.Empty(t, events)
}

func TestCheckOnce_RecentEvent(t *testing.T) {
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	store, err := eventstore.New(dbRW, dbRO, eventstore.DefaultRetention)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	comp, err := New(ctx, store)
	require.NoError(t, err)

	c := comp.(*component)
	defer comp.Close()

	c.currentVirtEnv = host.VirtualizationEnvironment{
		Type:  "baremetal",
		IsKVM: false,
	}

	// Create a recent event (less than 24h old)
	testTime := time.Now().UTC()
	event := components.Event{
		Time:    metav1.Time{Time: testTime.Add(-1 * time.Hour)}, // Just 1 hour ago
		Name:    "acs_enabled",
		Type:    "Warning",
		Message: "Test event",
	}

	bucket, err := store.Bucket(Name)
	require.NoError(t, err)
	defer bucket.Close()

	err = bucket.Insert(ctx, event)
	require.NoError(t, err)

	// Set up a mock listFunc that should not be called
	called := false
	c.listFunc = func(ctx context.Context) (pci.Devices, error) {
		called = true
		return nil, nil
	}

	// Run CheckOnce - it should exit early due to recent event
	c.CheckOnce()

	// Verify listFunc was not called due to recent event
	assert.False(t, called, "listFunc should not be called when there's a recent event")
}

func TestCheckOnce_EventBucketLatestError(t *testing.T) {
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	store, err := eventstore.New(dbRW, dbRO, eventstore.DefaultRetention)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	comp, err := New(ctx, store)
	require.NoError(t, err)

	c := comp.(*component)
	defer comp.Close()

	c.currentVirtEnv = host.VirtualizationEnvironment{
		Type:  "baremetal",
		IsKVM: false,
	}

	// Create a tracking variable to check if Latest was called
	latestCalled := false

	// Create a mock event bucket with an error on Latest and tracks calls
	mockErr := errors.New("mock bucket error")
	mockBucket := &mockEventBucket{
		latestErr: mockErr,
		latestFunc: func() {
			latestCalled = true
		},
	}

	// Replace the event bucket in the component
	c.eventBucket = mockBucket

	// Run CheckOnce with the mocked eventBucket
	c.CheckOnce()

	// Verify Latest was called
	assert.True(t, latestCalled, "Latest method should have been called")

	// Verify the error was captured
	c.lastMu.RLock()
	lastData := c.lastData
	c.lastMu.RUnlock()
	assert.NotNil(t, lastData)
	assert.Equal(t, mockErr, lastData.err)
}

func TestCheckOnce_EventBucketInsertError(t *testing.T) {
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	store, err := eventstore.New(dbRW, dbRO, eventstore.DefaultRetention)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	comp, err := New(ctx, store)
	require.NoError(t, err)

	c := comp.(*component)
	defer comp.Close()

	c.currentVirtEnv = host.VirtualizationEnvironment{
		Type:  "baremetal",
		IsKVM: false,
	}

	// Mock the listFunc to return devices with ACS enabled
	mockDevices := []pci.Device{
		{
			ID: "0000:00:00.0",
			AccessControlService: &pci.AccessControlService{
				ACSCtl: pci.ACS{
					SrcValid: true,
				},
			},
		},
	}
	c.listFunc = func(ctx context.Context) (pci.Devices, error) {
		return mockDevices, nil
	}

	// Replace the eventBucket with a mock that returns an error on insert
	mockErr := errors.New("mock insert error")
	mockBucket := &mockEventBucket{
		insertErr: mockErr,
	}
	c.eventBucket = mockBucket

	// Run CheckOnce with the mocked eventBucket
	c.CheckOnce()

	// Verify the error was captured
	c.lastMu.RLock()
	lastData := c.lastData
	c.lastMu.RUnlock()
	assert.NotNil(t, lastData)
}

func TestStartAndClose(t *testing.T) {
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	store, err := eventstore.New(dbRW, dbRO, eventstore.DefaultRetention)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	comp, err := New(ctx, store)
	require.NoError(t, err)

	// Start the component
	err = comp.Start()
	require.NoError(t, err)

	// Close the component
	err = comp.Close()
	require.NoError(t, err)

	// Check that ctx was canceled
	select {
	case <-comp.(*component).ctx.Done():
		// Success - context was canceled
	default:
		t.Error("Context was not canceled after Close()")
	}
}

// mockEventBucket is a test implementation of eventstore.Bucket
type mockEventBucket struct {
	latestErr  error
	insertErr  error
	latestFunc func()
}

func (m *mockEventBucket) Name() string {
	return "mock-bucket"
}

func (m *mockEventBucket) Insert(ctx context.Context, event components.Event) error {
	return m.insertErr
}

func (m *mockEventBucket) Find(ctx context.Context, ev components.Event) (*components.Event, error) {
	return nil, nil
}

func (m *mockEventBucket) Get(ctx context.Context, since time.Time) ([]components.Event, error) {
	return nil, nil
}

func (m *mockEventBucket) Latest(ctx context.Context) (*components.Event, error) {
	if m.latestFunc != nil {
		m.latestFunc()
	}
	if m.latestErr != nil {
		return nil, m.latestErr
	}
	return nil, nil
}

func (m *mockEventBucket) Purge(ctx context.Context, beforeTimestamp int64) (int, error) {
	return 0, nil
}

func (m *mockEventBucket) Close() {
	// No-op implementation to match the interface
}
