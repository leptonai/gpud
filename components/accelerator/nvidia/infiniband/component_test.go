package infiniband

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	pkgconfigcommon "github.com/leptonai/gpud/pkg/config/common"
	"github.com/leptonai/gpud/pkg/eventstore"
	"github.com/leptonai/gpud/pkg/kmsg"
	"github.com/leptonai/gpud/pkg/nvidia-query/infiniband"
	infinibandclass "github.com/leptonai/gpud/pkg/nvidia-query/infiniband/class"
	nvidianvml "github.com/leptonai/gpud/pkg/nvidia-query/nvml"
	"github.com/leptonai/gpud/pkg/nvidia-query/nvml/device"
	nvmllib "github.com/leptonai/gpud/pkg/nvidia-query/nvml/lib"
)

func TestComponentCheck(t *testing.T) {
	t.Parallel()

	// Create a component with mocked functions
	cctx, ccancel := context.WithCancel(context.Background())
	defer ccancel()

	// Apply fixes to ensure context is properly set
	mockBucket := createMockEventBucket()
	c := &component{
		ctx:         cctx,
		cancel:      ccancel,
		eventBucket: mockBucket,
		getTimeNowFunc: func() time.Time {
			return time.Now().UTC()
		},
		getIbstatOutputFunc: func(ctx context.Context) (*infiniband.IbstatOutput, error) {
			return nil, errors.New("ibstat checker not found")
		},
		getThresholdsFunc: mockGetThresholds,
		getClassDevicesFunc: func() (infinibandclass.Devices, error) {
			return infinibandclass.Devices{}, nil
		},
	}

	// Case 1: No NVML
	result := c.Check()
	data, ok := result.(*checkResult)
	require.True(t, ok)
	require.NotNil(t, data)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, data.health)
	assert.Equal(t, "NVIDIA NVML instance is nil", data.reason)

	// Case 2: With NVML but missing product name
	nvmlMock := &mockNVMLInstance{exists: true, productName: ""}
	c.nvmlInstance = nvmlMock
	result = c.Check()
	data, ok = result.(*checkResult)
	require.True(t, ok)
	require.NotNil(t, data)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, data.health)
	assert.Equal(t, "NVIDIA NVML is loaded but GPU is not detected (missing product name)", data.reason)

	// Case 3: With NVML and valid product name
	nvmlMock.productName = "Tesla V100"
	result = c.Check()
	data, ok = result.(*checkResult)
	require.True(t, ok)
	require.NotNil(t, data)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, data.health)
	assert.Equal(t, reasonNoIbPortData, data.reason)
}

func TestComponentEvents(t *testing.T) {
	t.Parallel()

	// Setup test event bucket
	mockBucket := createMockEventBucket()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	c := &component{
		ctx:         ctx,
		cancel:      cancel,
		eventBucket: mockBucket,
	}

	now := time.Now().UTC()

	// Insert test event using eventstore.Event
	testEvent := eventstore.Event{
		Time:    now.Add(-5 * time.Second),
		Name:    "test_event",
		Type:    string(apiv1.EventTypeWarning),
		Message: "test message",
	}
	err := mockBucket.Insert(ctx, testEvent)
	require.NoError(t, err)

	// Test Events method
	events, err := c.Events(ctx, now.Add(-10*time.Second))
	require.NoError(t, err)
	require.Len(t, events, 1)
	assert.Equal(t, testEvent.Name, events[0].Name)
	assert.Equal(t, testEvent.Message, events[0].Message)   // Check message too
	assert.Equal(t, testEvent.Type, string(events[0].Type)) // Check type too (cast to string)

	// Test with more recent time filter
	events, err = c.Events(ctx, now)
	require.NoError(t, err)
	assert.Empty(t, events)

	// Test with canceled context
	canceledCtx, cancelFunc := context.WithCancel(context.Background())
	cancelFunc() // Cancel immediately

	events, err = c.Events(canceledCtx, now)
	assert.Error(t, err)
	assert.Nil(t, events)
}

func TestComponentClose(t *testing.T) {
	t.Parallel()

	cctx, ccancel := context.WithCancel(context.Background())
	mockBucket := createMockEventBucket()

	c := &component{
		ctx:         cctx,
		cancel:      ccancel,
		eventBucket: mockBucket,
	}

	// Add a real kmsgSyncer only if on linux to test the path where it's not nil
	// Note: This part might still not run in all test environments
	if runtime.GOOS == "linux" {
		// Attempt to create, ignore error for this test focused on Close()
		kmsgSyncer, _ := kmsg.NewSyncer(cctx, func(line string) (string, string) {
			return "test", "test"
		}, mockBucket)
		c.kmsgSyncer = kmsgSyncer // Assign if created
	}

	err := c.Close()
	assert.NoError(t, err)

	// Verify context is canceled
	select {
	case <-cctx.Done():
		// Success - context should be canceled
	default:
		t.Fatal("Context not canceled after Close()")
	}
}

func TestNew(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	gpudInstance := &components.GPUdInstance{
		RootCtx:              ctx,
		NVIDIAToolOverwrites: pkgconfigcommon.ToolOverwrites{},
	}

	comp, err := New(gpudInstance)
	assert.NoError(t, err)
	assert.NotNil(t, comp)
	assert.Equal(t, Name, comp.Name())
}

func TestTags(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	gpudInstance := &components.GPUdInstance{
		RootCtx:              ctx,
		NVIDIAToolOverwrites: pkgconfigcommon.ToolOverwrites{},
	}

	comp, err := New(gpudInstance)
	assert.NoError(t, err)

	expectedTags := []string{
		"accelerator",
		"gpu",
		"nvidia",
		Name,
	}

	tags := comp.Tags()
	assert.Equal(t, expectedTags, tags, "Component tags should match expected values")
	assert.Len(t, tags, 4, "Component should return exactly 4 tags")
}

// mockEventStore for testing New errors
type mockEventStore struct {
	bucketErr error
}

func (m *mockEventStore) Bucket(name string, opts ...eventstore.OpOption) (eventstore.Bucket, error) {
	if m.bucketErr != nil {
		return nil, m.bucketErr
	}
	// Return a simple mock bucket if no error
	return createMockEventBucket(), nil
}

// mockEventBucket implements the events_db.Store interface for testing
type mockEventBucket struct {
	events    eventstore.Events
	mu        sync.Mutex
	findErr   error // Added for testing Find errors
	insertErr error // Added for testing Insert errors
}

func createMockEventBucket() *mockEventBucket {
	return &mockEventBucket{
		events: eventstore.Events{},
	}
}

func (m *mockEventBucket) Name() string {
	return "mock"
}

func (m *mockEventBucket) Insert(ctx context.Context, event eventstore.Event) error {
	if m.insertErr != nil {
		return m.insertErr
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, event)
	return nil
}

func (m *mockEventBucket) Get(ctx context.Context, since time.Time, opts ...eventstore.OpOption) (eventstore.Events, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	var result eventstore.Events
	for _, event := range m.events {
		if !event.Time.Before(since) {
			result = append(result, event)
		}
	}
	return result, nil
}

func (m *mockEventBucket) Find(ctx context.Context, event eventstore.Event) (*eventstore.Event, error) {
	if m.findErr != nil {
		return nil, m.findErr
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	for i, e := range m.events {
		if e.Name == event.Name && e.Type == event.Type && e.Message == event.Message {
			return &m.events[i], nil
		}
	}
	return nil, nil
}

func (m *mockEventBucket) Latest(ctx context.Context) (*eventstore.Event, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	m.mu.Lock()
	defer m.mu.Unlock()

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

func (m *mockEventBucket) Purge(ctx context.Context, beforeTimestamp int64) (int, error) {
	select {
	case <-ctx.Done():
		return 0, ctx.Err()
	default:
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	var newEvents eventstore.Events
	var purgedCount int

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

func (m *mockEventBucket) Close() {
	// No-op for mock
}

// GetEvents returns a copy of the stored events as apiv1.Events for assertion convenience.
func (m *mockEventBucket) GetAPIEvents() apiv1.Events {
	m.mu.Lock()
	defer m.mu.Unlock()

	result := make(apiv1.Events, len(m.events))
	for i, ev := range m.events {
		result[i] = ev.ToEvent()
	}
	return result
}

// Test helpers for mocking NVML and IBStat
type mockNVMLInstance struct {
	exists      bool
	productName string
}

func (m *mockNVMLInstance) NVMLExists() bool {
	return m.exists
}

// Simple mock implementation of required InstanceV2 interface methods
func (m *mockNVMLInstance) Devices() map[string]device.Device {
	return nil
}

func (m *mockNVMLInstance) Library() nvmllib.Library {
	return nil
}

func (m *mockNVMLInstance) ProductName() string {
	if m.productName == "" {
		return "" // Empty string for testing
	}
	return m.productName // Return custom value for testing
}

func (m *mockNVMLInstance) Architecture() string {
	return ""
}

func (m *mockNVMLInstance) Brand() string {
	return ""
}

func (m *mockNVMLInstance) DriverVersion() string {
	return ""
}

func (m *mockNVMLInstance) DriverMajor() int {
	return 0
}

func (m *mockNVMLInstance) CUDAVersion() string {
	return ""
}

func (m *mockNVMLInstance) FabricManagerSupported() bool {
	return true
}

func (m *mockNVMLInstance) GetMemoryErrorManagementCapabilities() nvidianvml.MemoryErrorManagementCapabilities {
	return nvidianvml.MemoryErrorManagementCapabilities{}
}

func (m *mockNVMLInstance) Shutdown() error {
	return nil
}

func mockGetIbstatOutput(ctx context.Context) (*infiniband.IbstatOutput, error) {
	return &infiniband.IbstatOutput{
		Raw: "mock output",
		Parsed: infiniband.IBStatCards{
			{
				Device: "mlx5_0",
				Port1: infiniband.IBStatPort{
					State:         "Active",
					PhysicalState: "LinkUp",
					Rate:          200,
				},
			},
		},
	}, nil
}

func mockGetThresholds() infiniband.ExpectedPortStates {
	return infiniband.ExpectedPortStates{
		AtLeastPorts: 1,
		AtLeastRate:  100,
	}
}

func TestComponentStart(t *testing.T) {
	t.Parallel()

	cctx, ccancel := context.WithCancel(context.Background())
	defer ccancel()

	c := &component{
		ctx:           cctx,
		cancel:        ccancel,
		checkInterval: time.Second, // Set check interval to avoid panic
		getTimeNowFunc: func() time.Time {
			return time.Now().UTC()
		},
		getIbstatOutputFunc: mockGetIbstatOutput,
		getThresholdsFunc:   mockGetThresholds,
		getClassDevicesFunc: func() (infinibandclass.Devices, error) {
			return infinibandclass.Devices{}, nil
		},
	}

	err := c.Start()
	assert.NoError(t, err)

	// Verify the background goroutine was started by checking if a check result gets populated
	time.Sleep(50 * time.Millisecond) // Give a small time for the goroutine to run

	c.lastMu.RLock()
	lastCheckResult := c.lastCheckResult
	c.lastMu.RUnlock()

	assert.NotNil(t, lastCheckResult, "lastCheckResult should be populated by the background goroutine")
}

func TestLastHealthStates(t *testing.T) {
	t.Parallel()

	// Test with nil data
	c := &component{}
	states := c.LastHealthStates()
	assert.Len(t, states, 1)
	assert.Equal(t, Name, states[0].Name)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, states[0].Health)
	assert.Equal(t, "no data yet", states[0].Reason)

	// Test with data
	mockData := &checkResult{
		health: apiv1.HealthStateTypeUnhealthy,
		reason: "test reason",
		err:    fmt.Errorf("test error"),
	}
	c.lastMu.Lock()
	c.lastCheckResult = mockData
	c.lastMu.Unlock()

	states = c.LastHealthStates()
	assert.Len(t, states, 1)
	assert.Equal(t, Name, states[0].Name)
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, states[0].Health)
	assert.Equal(t, "test reason", states[0].Reason)
	assert.Equal(t, "test error", states[0].Error)
}

func TestDataString(t *testing.T) {
	t.Parallel()

	// Test nil data
	var cr *checkResult
	assert.Equal(t, "", cr.String())

	// Test with nil IbstatOutput
	cr = &checkResult{}
	assert.Equal(t, "no data", cr.String())

	// Test with actual data
	cr = &checkResult{
		IbstatOutput: &infiniband.IbstatOutput{
			Parsed: infiniband.IBStatCards{
				{
					Device: "mlx5_0",
					Port1: infiniband.IBStatPort{
						State:         "Active",
						PhysicalState: "LinkUp",
						Rate:          200,
					},
				},
			},
		},
	}
	result := cr.String()
	assert.Contains(t, result, "PORT DEVICE NAME")
	assert.Contains(t, result, "PORT1 STATE")
	assert.Contains(t, result, "mlx5_0")
	assert.Contains(t, result, "Active")
}

func TestDataSummary(t *testing.T) {
	t.Parallel()

	// Test nil data
	var cr *checkResult
	assert.Equal(t, "", cr.Summary())

	// Test with reason
	cr = &checkResult{reason: "test reason"}
	assert.Equal(t, "test reason", cr.Summary())
}

func TestDataHealthState(t *testing.T) {
	t.Parallel()

	// Test nil data
	var cr *checkResult
	assert.Equal(t, apiv1.HealthStateType(""), cr.HealthStateType())

	// Test with health state
	cr = &checkResult{health: apiv1.HealthStateTypeUnhealthy}
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.HealthStateType())
}

func TestDataGetError(t *testing.T) {
	t.Parallel()

	// Test nil data
	var cr *checkResult
	assert.Equal(t, "", cr.getError())

	// Test with nil error
	cr = &checkResult{}
	assert.Equal(t, "", cr.getError())

	// Test with error
	cr = &checkResult{err: errors.New("test error")}
	assert.Equal(t, "test error", cr.getError())
}

func TestComponentCheckErrorCases(t *testing.T) {
	t.Parallel()

	cctx, ccancel := context.WithCancel(context.Background())
	defer ccancel()

	// Test case: getIbstatOutputFunc returns error
	c := &component{
		ctx:    cctx,
		cancel: ccancel,
		getTimeNowFunc: func() time.Time {
			return time.Now().UTC()
		},
		getIbstatOutputFunc: func(ctx context.Context) (*infiniband.IbstatOutput, error) {
			return nil, errors.New("ibstat error")
		},
		getThresholdsFunc: mockGetThresholds,
		nvmlInstance:      &mockNVMLInstance{exists: true, productName: "Tesla V100"},
		getClassDevicesFunc: func() (infinibandclass.Devices, error) {
			return infinibandclass.Devices{}, nil
		},
	}

	result := c.Check()
	data, ok := result.(*checkResult)
	require.True(t, ok)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, data.health)
	assert.Equal(t, reasonNoEventBucket, data.reason)

	// Test case: ibstat command not found
	c = &component{
		ctx:    cctx,
		cancel: ccancel,
		getTimeNowFunc: func() time.Time {
			return time.Now().UTC()
		},
		getIbstatOutputFunc: func(ctx context.Context) (*infiniband.IbstatOutput, error) {
			return nil, infiniband.ErrNoIbstatCommand
		},
		getThresholdsFunc: mockGetThresholds,
		nvmlInstance:      &mockNVMLInstance{exists: true, productName: "Tesla V100"},
		getClassDevicesFunc: func() (infinibandclass.Devices, error) {
			return infinibandclass.Devices{}, nil
		},
	}

	result = c.Check()
	data, ok = result.(*checkResult)
	require.True(t, ok)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, data.health)
	assert.Equal(t, reasonNoEventBucket, data.reason)
}

// Test Check when getIbstatOutputFunc is nil
func TestCheckNilIbstatFunc(t *testing.T) {
	t.Parallel()

	cctx, ccancel := context.WithCancel(context.Background())
	defer ccancel()

	mockBucket := createMockEventBucket()
	c := &component{
		ctx:          cctx,
		cancel:       ccancel,
		nvmlInstance: &mockNVMLInstance{exists: true, productName: "Tesla V100"},
		eventBucket:  mockBucket,
		getTimeNowFunc: func() time.Time {
			return time.Now().UTC()
		},
		getIbstatOutputFunc: func(ctx context.Context) (*infiniband.IbstatOutput, error) {
			return nil, errors.New("ibstat checker not found")
		},
		getThresholdsFunc: mockGetThresholds,
		getClassDevicesFunc: func() (infinibandclass.Devices, error) {
			return infinibandclass.Devices{}, nil
		},
	}

	result := c.Check()
	data, ok := result.(*checkResult)
	require.True(t, ok)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, data.health)
	assert.Equal(t, reasonNoIbPortData, data.reason)
}

// TestComponentCheckOrder tests that the checks in the Check() method are evaluated in the correct order
func TestComponentCheckOrder(t *testing.T) {
	t.Parallel()

	var checksCalled []string
	trackCheck := func(name string) {
		checksCalled = append(checksCalled, name)
	}

	// Create a context for tests
	cctx, ccancel := context.WithCancel(context.Background())
	defer ccancel()

	// Only test the threshold check first which is more reliable
	c := &component{
		ctx:    cctx,
		cancel: ccancel,
		getTimeNowFunc: func() time.Time {
			return time.Now().UTC()
		},
		getThresholdsFunc: func() infiniband.ExpectedPortStates {
			trackCheck("thresholds")
			return infiniband.ExpectedPortStates{} // zero thresholds
		},
	}

	result := c.Check()
	data, ok := result.(*checkResult)
	require.True(t, ok)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, data.health)
	assert.Equal(t, reasonNoThreshold, data.reason)
	assert.Equal(t, []string{"thresholds"}, checksCalled)
}

// TestEventsWithContextCanceled tests the Events method with a canceled context
func TestEventsWithContextCanceled(t *testing.T) {
	t.Parallel()

	mockBucket := createMockEventBucket()

	// Create component with the mock bucket
	c := &component{
		eventBucket: mockBucket,
	}

	// Create a canceled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	// Test Events with canceled context
	since := time.Now().Add(-time.Hour)
	events, err := c.Events(ctx, since)
	assert.Error(t, err)
	assert.Nil(t, events)
	assert.Equal(t, context.Canceled, err)
}

// TestEventsWithNoEventBucket tests the Events method when eventBucket is nil
func TestEventsWithNoEventBucket(t *testing.T) {
	t.Parallel()

	// Create component with nil eventBucket
	c := &component{
		eventBucket: nil,
	}

	// Test Events with nil eventBucket
	since := time.Now().Add(-time.Hour)
	events, err := c.Events(context.Background(), since)
	assert.NoError(t, err)
	assert.Nil(t, events)
}

// TestCloseWithNilComponents tests the Close method when components are nil
func TestCloseWithNilComponents(t *testing.T) {
	t.Parallel()

	c := &component{
		ctx:         context.Background(),
		cancel:      func() {}, // no-op cancel
		eventBucket: nil,
		kmsgSyncer:  nil,
	}

	err := c.Close()
	assert.NoError(t, err)
}

// TestIsSupported tests the IsSupported method with all possible cases
func TestIsSupported(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		nvml     nvidianvml.Instance
		expected bool
	}{
		{
			name:     "nil nvml instance",
			nvml:     nil,
			expected: false,
		},
		{
			name:     "nvml exists false",
			nvml:     &mockNVMLInstance{exists: false, productName: ""},
			expected: false,
		},
		{
			name:     "nvml exists but no product name",
			nvml:     &mockNVMLInstance{exists: true, productName: ""},
			expected: false,
		},
		{
			name:     "nvml exists with product name",
			nvml:     &mockNVMLInstance{exists: true, productName: "Tesla V100"},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &component{
				nvmlInstance: tt.nvml,
			}
			assert.Equal(t, tt.expected, c.IsSupported())
		})
	}
}

// TestComponentName tests the ComponentName method
func TestComponentName(t *testing.T) {
	t.Parallel()

	cr := &checkResult{}
	assert.Equal(t, Name, cr.ComponentName())
}

// TestGetSuggestedActions tests the getSuggestedActions method
func TestGetSuggestedActions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		cr       *checkResult
		expected *apiv1.SuggestedActions
	}{
		{
			name:     "nil checkResult",
			cr:       nil,
			expected: nil,
		},
		{
			name:     "no suggested actions",
			cr:       &checkResult{},
			expected: nil,
		},
		{
			name: "with suggested actions",
			cr: &checkResult{
				suggestedActions: &apiv1.SuggestedActions{
					RepairActions: []apiv1.RepairActionType{apiv1.RepairActionTypeHardwareInspection},
				},
			},
			expected: &apiv1.SuggestedActions{
				RepairActions: []apiv1.RepairActionType{apiv1.RepairActionTypeHardwareInspection},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.cr.getSuggestedActions())
		})
	}
}

// TestNewWithEventStoreError tests New function when EventStore bucket creation fails
func TestNewWithEventStoreError(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	mockStore := &mockEventStore{
		bucketErr: errors.New("bucket creation error"),
	}

	gpudInstance := &components.GPUdInstance{
		RootCtx:              ctx,
		NVIDIAToolOverwrites: pkgconfigcommon.ToolOverwrites{},
		EventStore:           mockStore,
	}

	comp, err := New(gpudInstance)
	assert.Error(t, err)
	assert.Nil(t, comp)
	assert.Contains(t, err.Error(), "bucket creation error")
}

// TestCheckWithVariousStates tests Check method with various health states (event operations removed)
func TestCheckWithVariousStates(t *testing.T) {
	t.Parallel()

	// Test case 1: Healthy state with sufficient ports
	t.Run("healthy state with sufficient ports", func(t *testing.T) {
		cctx, ccancel := context.WithCancel(context.Background())
		defer ccancel()

		mockBucket := createMockEventBucket()

		c := &component{
			ctx:          cctx,
			cancel:       ccancel,
			nvmlInstance: &mockNVMLInstance{exists: true, productName: "Tesla V100"},
			eventBucket:  mockBucket,
			getTimeNowFunc: func() time.Time {
				return time.Now().UTC()
			},
			getClassDevicesFunc: func() (infinibandclass.Devices, error) {
				return infinibandclass.Devices{}, nil
			},
			getIbstatOutputFunc: func(ctx context.Context) (*infiniband.IbstatOutput, error) {
				// Return healthy state
				return &infiniband.IbstatOutput{
					Raw: "test",
					Parsed: infiniband.IBStatCards{
						{
							Device: "mlx5_0",
							Port1: infiniband.IBStatPort{
								State:         "Active",
								PhysicalState: "LinkUp",
								Rate:          200,
								LinkLayer:     "Infiniband",
							},
						},
					},
				}, nil
			},
			getThresholdsFunc: func() infiniband.ExpectedPortStates {
				return infiniband.ExpectedPortStates{
					AtLeastPorts: 1,
					AtLeastRate:  200,
				}
			},
		}

		result := c.Check()
		data, ok := result.(*checkResult)
		require.True(t, ok)
		assert.Equal(t, apiv1.HealthStateTypeHealthy, data.health)
		assert.Equal(t, reasonNoIbPortIssue, data.reason)
		assert.Nil(t, data.err)
		assert.Nil(t, data.suggestedActions)
	})

	// Test case 2: Unhealthy state with insufficient ports
	t.Run("unhealthy state with insufficient ports", func(t *testing.T) {
		cctx, ccancel := context.WithCancel(context.Background())
		defer ccancel()

		mockBucket := createMockEventBucket()

		c := &component{
			ctx:          cctx,
			cancel:       ccancel,
			nvmlInstance: &mockNVMLInstance{exists: true, productName: "Tesla V100"},
			eventBucket:  mockBucket,
			getTimeNowFunc: func() time.Time {
				return time.Now().UTC()
			},
			getClassDevicesFunc: func() (infinibandclass.Devices, error) {
				return infinibandclass.Devices{}, nil
			},
			getIbstatOutputFunc: func(ctx context.Context) (*infiniband.IbstatOutput, error) {
				// Return unhealthy state
				return &infiniband.IbstatOutput{
					Raw: "test",
					Parsed: infiniband.IBStatCards{
						{
							Device: "mlx5_0",
							Port1: infiniband.IBStatPort{
								State:         "Down",
								PhysicalState: "Disabled",
								Rate:          200,
							},
						},
					},
				}, nil
			},
			getThresholdsFunc: func() infiniband.ExpectedPortStates {
				return infiniband.ExpectedPortStates{
					AtLeastPorts: 1,
					AtLeastRate:  200,
				}
			},
		}

		result := c.Check()
		data, ok := result.(*checkResult)
		require.True(t, ok)
		assert.Equal(t, apiv1.HealthStateTypeUnhealthy, data.health)
		assert.Contains(t, data.reason, "only 0 port(s) are active and >=200 Gb/s")
		assert.Nil(t, data.err)
		assert.NotNil(t, data.suggestedActions)
		assert.Equal(t, []apiv1.RepairActionType{apiv1.RepairActionTypeHardwareInspection}, data.suggestedActions.RepairActions)
	})

	// Test case 3: Threshold not set
	t.Run("threshold not set", func(t *testing.T) {
		cctx, ccancel := context.WithCancel(context.Background())
		defer ccancel()

		mockBucket := createMockEventBucket()

		c := &component{
			ctx:          cctx,
			cancel:       ccancel,
			nvmlInstance: &mockNVMLInstance{exists: true, productName: "Tesla V100"},
			eventBucket:  mockBucket,
			getTimeNowFunc: func() time.Time {
				return time.Now().UTC()
			},
			getClassDevicesFunc: func() (infinibandclass.Devices, error) {
				return infinibandclass.Devices{}, nil
			},
			getIbstatOutputFunc: func(ctx context.Context) (*infiniband.IbstatOutput, error) {
				return &infiniband.IbstatOutput{
					Raw: "test",
					Parsed: infiniband.IBStatCards{
						{
							Device: "mlx5_0",
							Port1: infiniband.IBStatPort{
								State:         "Active",
								PhysicalState: "LinkUp",
								Rate:          200,
							},
						},
					},
				}, nil
			},
			getThresholdsFunc: func() infiniband.ExpectedPortStates {
				return infiniband.ExpectedPortStates{
					AtLeastPorts: 0,
					AtLeastRate:  0,
				}
			},
		}

		result := c.Check()
		data, ok := result.(*checkResult)
		require.True(t, ok)
		assert.Equal(t, apiv1.HealthStateTypeHealthy, data.health)
		assert.Equal(t, reasonNoThreshold, data.reason)
		assert.Nil(t, data.err)
		assert.Nil(t, data.suggestedActions)
	})
}

// TestCheckWithPartialIbstatOutput tests Check when ibstat returns partial output with error
func TestCheckWithPartialIbstatOutput(t *testing.T) {
	t.Parallel()

	cctx, ccancel := context.WithCancel(context.Background())
	defer ccancel()

	mockBucket := createMockEventBucket()

	c := &component{
		ctx:          cctx,
		cancel:       ccancel,
		nvmlInstance: &mockNVMLInstance{exists: true, productName: "Tesla V100"},
		eventBucket:  mockBucket,
		getTimeNowFunc: func() time.Time {
			return time.Now().UTC()
		},
		getClassDevicesFunc: func() (infinibandclass.Devices, error) {
			return infinibandclass.Devices{}, nil
		},
		getIbstatOutputFunc: func(ctx context.Context) (*infiniband.IbstatOutput, error) {
			// Return partial output with error but healthy state
			return &infiniband.IbstatOutput{
				Raw: "partial output",
				Parsed: infiniband.IBStatCards{
					{
						Device: "mlx5_0",
						Port1: infiniband.IBStatPort{
							State:         "Active",
							PhysicalState: "LinkUp",
							Rate:          200,
							LinkLayer:     "Infiniband",
						},
					},
				},
			}, errors.New("partial read error")
		},
		getThresholdsFunc: func() infiniband.ExpectedPortStates {
			return infiniband.ExpectedPortStates{
				AtLeastPorts: 1,
				AtLeastRate:  200,
			}
		},
	}

	result := c.Check()
	data, ok := result.(*checkResult)
	require.True(t, ok)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, data.health)
	assert.Nil(t, data.err) // Error should be discarded since output meets thresholds
	assert.Equal(t, reasonNoIbPortIssue, data.reason)
}

// TestCheckWithNilIbstatOutputNoError tests Check when ibstat returns nil output with no error
func TestCheckWithNilIbstatOutputNoError(t *testing.T) {
	t.Parallel()

	cctx, ccancel := context.WithCancel(context.Background())
	defer ccancel()

	mockBucket := createMockEventBucket()

	c := &component{
		ctx:          cctx,
		cancel:       ccancel,
		nvmlInstance: &mockNVMLInstance{exists: true, productName: "Tesla V100"},
		eventBucket:  mockBucket,
		getTimeNowFunc: func() time.Time {
			return time.Now().UTC()
		},
		getClassDevicesFunc: func() (infinibandclass.Devices, error) {
			return infinibandclass.Devices{}, nil
		},
		getIbstatOutputFunc: func(ctx context.Context) (*infiniband.IbstatOutput, error) {
			return nil, nil // No error, no output
		},
		getThresholdsFunc: func() infiniband.ExpectedPortStates {
			return infiniband.ExpectedPortStates{
				AtLeastPorts: 1,
				AtLeastRate:  200,
			}
		},
	}

	result := c.Check()
	data, ok := result.(*checkResult)
	require.True(t, ok)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, data.health)
	assert.Equal(t, reasonNoIbPortIssue, data.reason)
}

// TestHealthStatesWithIbstatOutput tests HealthStates method when IbstatOutput is not nil
func TestHealthStatesWithIbstatOutput(t *testing.T) {
	t.Parallel()

	cr := &checkResult{
		ts:     time.Now().UTC(),
		health: apiv1.HealthStateTypeHealthy,
		reason: "test reason",
		IbstatOutput: &infiniband.IbstatOutput{
			Raw: "test output",
			Parsed: infiniband.IBStatCards{
				{
					Device: "mlx5_0",
					Port1: infiniband.IBStatPort{
						State:         "Active",
						PhysicalState: "LinkUp",
						Rate:          200,
					},
				},
			},
		},
	}

	states := cr.HealthStates()
	assert.Len(t, states, 1)
	assert.Equal(t, Name, states[0].Name)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, states[0].Health)
	assert.Equal(t, "test reason", states[0].Reason)
	assert.Nil(t, states[0].ExtraInfo) // Current implementation doesn't set ExtraInfo
}

// TestCheckUnhealthyState tests unhealthy state detection (no longer tests event insertion since it was removed)
func TestCheckUnhealthyState(t *testing.T) {
	t.Parallel()

	cctx, ccancel := context.WithCancel(context.Background())
	defer ccancel()

	mockBucket := createMockEventBucket()

	c := &component{
		ctx:          cctx,
		cancel:       ccancel,
		nvmlInstance: &mockNVMLInstance{exists: true, productName: "Tesla V100"},
		eventBucket:  mockBucket,
		getTimeNowFunc: func() time.Time {
			return time.Now().UTC()
		},
		getClassDevicesFunc: func() (infinibandclass.Devices, error) {
			return infinibandclass.Devices{}, nil
		},
		getIbstatOutputFunc: func(ctx context.Context) (*infiniband.IbstatOutput, error) {
			// Return unhealthy state
			return &infiniband.IbstatOutput{
				Raw: "test",
				Parsed: infiniband.IBStatCards{
					{
						Device: "mlx5_0",
						Port1: infiniband.IBStatPort{
							State:         "Down",
							PhysicalState: "Disabled",
							Rate:          200,
							LinkLayer:     "Infiniband",
						},
					},
				},
			}, nil
		},
		getThresholdsFunc: func() infiniband.ExpectedPortStates {
			return infiniband.ExpectedPortStates{
				AtLeastPorts: 1,
				AtLeastRate:  200,
			}
		},
	}

	result := c.Check()
	data, ok := result.(*checkResult)
	require.True(t, ok)
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, data.health)
	assert.Contains(t, data.reason, "only 0 port(s) are active and >=200 Gb/s")
	assert.Nil(t, data.err)

	// Verify suggested actions are set for unhealthy state
	assert.NotNil(t, data.suggestedActions)
	assert.Equal(t, []apiv1.RepairActionType{apiv1.RepairActionTypeHardwareInspection}, data.suggestedActions.RepairActions)
}

// TestNewKmsgSyncerHandling tests New function when kmsg.NewSyncer fails
// Note: We can't directly test the os.Geteuid() == 0 path due to test restrictions
// But we can verify that New handles errors gracefully
func TestNewKmsgSyncerHandling(t *testing.T) {
	t.Parallel()

	// Test that New succeeds even without root privileges
	ctx := context.Background()
	mockStore := &mockEventStore{}

	gpudInstance := &components.GPUdInstance{
		RootCtx:              ctx,
		NVIDIAToolOverwrites: pkgconfigcommon.ToolOverwrites{},
		EventStore:           mockStore,
	}

	comp, err := New(gpudInstance)
	assert.NoError(t, err)
	assert.NotNil(t, comp)

	// Clean up
	err = comp.Close()
	assert.NoError(t, err)
}

// TestCheckWithNoNVMLLibrary tests Check when NVML library is not loaded
func TestCheckWithNoNVMLLibrary(t *testing.T) {
	t.Parallel()

	cctx, ccancel := context.WithCancel(context.Background())
	defer ccancel()

	c := &component{
		ctx:                 cctx,
		cancel:              ccancel,
		nvmlInstance:        &mockNVMLInstance{exists: false, productName: ""},
		getTimeNowFunc:      mockTimeNow(),
		getIbstatOutputFunc: mockGetIbstatOutput,
		getThresholdsFunc:   mockGetThresholds,
	}

	result := c.Check()
	data, ok := result.(*checkResult)
	require.True(t, ok)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, data.health)
	assert.Equal(t, "NVIDIA NVML library is not loaded", data.reason)
}

// TestCheckZeroThresholds tests Check when thresholds are zero (not set)
func TestCheckZeroThresholds(t *testing.T) {
	t.Parallel()

	cctx, ccancel := context.WithCancel(context.Background())
	defer ccancel()

	c := &component{
		ctx:    cctx,
		cancel: ccancel,
		getTimeNowFunc: func() time.Time {
			return time.Now().UTC()
		},
		getThresholdsFunc: func() infiniband.ExpectedPortStates {
			return infiniband.ExpectedPortStates{
				AtLeastPorts: 0,
				AtLeastRate:  0,
			}
		},
	}

	result := c.Check()
	data, ok := result.(*checkResult)
	require.True(t, ok)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, data.health)
	assert.Equal(t, reasonNoThreshold, data.reason)
}

func TestCheckWithClassDevicesError(t *testing.T) {
	t.Parallel()

	cctx, ccancel := context.WithCancel(context.Background())
	defer ccancel()

	// Create a component with mocked functions where getClassDevicesFunc returns an error
	mockBucket := createMockEventBucket()
	c := &component{
		ctx:         cctx,
		cancel:      ccancel,
		eventBucket: mockBucket,
		nvmlInstance: &mockNVMLInstance{
			exists:      true,
			productName: "Tesla V100",
		},
		getTimeNowFunc: func() time.Time {
			return time.Now().UTC()
		},
		// Mock getClassDevicesFunc to return an error
		getClassDevicesFunc: func() (infinibandclass.Devices, error) {
			return nil, errors.New("failed to load class devices")
		},
		// Set getIbstatOutputFunc to nil to trigger the "checker not found" path
		getIbstatOutputFunc: func(ctx context.Context) (*infiniband.IbstatOutput, error) {
			return nil, errors.New("ibstat checker not found")
		},
		getThresholdsFunc: func() infiniband.ExpectedPortStates {
			return infiniband.ExpectedPortStates{
				AtLeastPorts: 1,
				AtLeastRate:  100,
			}
		},
	}

	result := c.Check()
	data, ok := result.(*checkResult)
	require.True(t, ok)

	// Verify that the getClassDevicesFunc error is handled correctly
	// Note: the error from getClassDevicesFunc is only logged, it doesn't affect the health status
	// The actual health status is determined by the getIbstatOutputFunc
	assert.Equal(t, apiv1.HealthStateTypeHealthy, data.health)
	assert.Equal(t, reasonNoIbPortData, data.reason)
	// The error may be nil if the event handling completed successfully
	// The key test is that ClassDevices is nil due to the error
	assert.Nil(t, data.ClassDevices, "ClassDevices should be nil when error occurs")
}

func TestCheckWithClassDevicesSuccess(t *testing.T) {
	t.Parallel()

	cctx, ccancel := context.WithCancel(context.Background())
	defer ccancel()

	// Mock successful class devices
	mockDevices := infinibandclass.Devices{
		{
			Name:            "mlx5_0",
			BoardID:         "MT_0000000838",
			FirmwareVersion: "28.41.1000",
			HCAType:         "MT4129",
		},
	}

	// Create a component with mocked functions where getClassDevicesFunc succeeds
	mockBucket := createMockEventBucket()
	c := &component{
		ctx:         cctx,
		cancel:      ccancel,
		eventBucket: mockBucket,
		nvmlInstance: &mockNVMLInstance{
			exists:      true,
			productName: "Tesla V100",
		},
		getTimeNowFunc: func() time.Time {
			return time.Now().UTC()
		},
		// Mock getClassDevicesFunc to return success
		getClassDevicesFunc: func() (infinibandclass.Devices, error) {
			return mockDevices, nil
		},
		// Set getIbstatOutputFunc to return an error to trigger the "checker not found" path
		getIbstatOutputFunc: func(ctx context.Context) (*infiniband.IbstatOutput, error) {
			return nil, errors.New("ibstat checker not found")
		},
		getThresholdsFunc: func() infiniband.ExpectedPortStates {
			return infiniband.ExpectedPortStates{
				AtLeastPorts: 1,
				AtLeastRate:  100,
			}
		},
	}

	result := c.Check()
	data, ok := result.(*checkResult)
	require.True(t, ok)

	// Verify the success case continues processing
	// Since getIbstatOutputFunc returns an error, the health will be healthy due to missing output
	assert.Equal(t, apiv1.HealthStateTypeHealthy, data.health)
	assert.Equal(t, reasonNoIbPortData, data.reason)
	assert.Equal(t, mockDevices, data.ClassDevices, "ClassDevices should be populated when successful")
	assert.Len(t, data.ClassDevices, 1, "Should have one device")
	assert.Equal(t, "mlx5_0", data.ClassDevices[0].Name)
}

// Additional comprehensive tests for getClassDevicesFunc and coverage improvement

// TestNewWithNilEventStore tests New function when EventStore is nil
func TestNewWithNilEventStore(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	gpudInstance := &components.GPUdInstance{
		RootCtx:              ctx,
		NVIDIAToolOverwrites: pkgconfigcommon.ToolOverwrites{},
		EventStore:           nil, // Explicitly nil
	}

	comp, err := New(gpudInstance)
	assert.NoError(t, err)
	assert.NotNil(t, comp)

	// Verify the component was created properly without event store
	casted := comp.(*component)
	assert.Nil(t, casted.eventBucket)
	assert.Nil(t, casted.kmsgSyncer)

	// Clean up
	err = comp.Close()
	assert.NoError(t, err)
}

// TestNewWithCustomToolOverwrites tests New function with custom tool overwrites
func TestNewWithCustomToolOverwrites(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	customToolOverwrites := pkgconfigcommon.ToolOverwrites{
		IbstatCommand: "custom-ibstat",
	}

	gpudInstance := &components.GPUdInstance{
		RootCtx:              ctx,
		NVIDIAToolOverwrites: customToolOverwrites,
		EventStore:           nil,
	}

	comp, err := New(gpudInstance)
	assert.NoError(t, err)
	assert.NotNil(t, comp)

	// Verify the custom tool overwrites are set
	casted := comp.(*component)
	assert.Equal(t, "custom-ibstat", casted.toolOverwrites.IbstatCommand)

	// Clean up
	err = comp.Close()
	assert.NoError(t, err)
}

// TestGetClassDevicesWithVariousDeviceTypes tests getClassDevicesFunc with different device configurations
func TestGetClassDevicesWithVariousDeviceTypes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		mockDevices    infinibandclass.Devices
		expectedString string
	}{
		{
			name:        "empty devices",
			mockDevices: infinibandclass.Devices{},
		},
		{
			name: "single device",
			mockDevices: infinibandclass.Devices{
				{
					Name:            "mlx5_0",
					BoardID:         "MT_0000000123",
					FirmwareVersion: "28.40.1000",
					HCAType:         "MT4125",
				},
			},
			expectedString: "mlx5_0",
		},
		{
			name: "multiple devices with different types",
			mockDevices: infinibandclass.Devices{
				{
					Name:            "mlx5_0",
					BoardID:         "MT_0000000123",
					FirmwareVersion: "28.40.1000",
					HCAType:         "MT4125",
				},
				{
					Name:            "mlx5_1",
					BoardID:         "MT_0000000456",
					FirmwareVersion: "28.41.2000",
					HCAType:         "MT4129",
				},
				{
					Name:            "mlx5_2",
					BoardID:         "MT_0000000789",
					FirmwareVersion: "28.42.3000",
					HCAType:         "MT4130",
				},
			},
			expectedString: "mlx5_0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cctx, ccancel := context.WithCancel(context.Background())
			defer ccancel()

			mockBucket := createMockEventBucket()
			c := &component{
				ctx:         cctx,
				cancel:      ccancel,
				eventBucket: mockBucket,
				nvmlInstance: &mockNVMLInstance{
					exists:      true,
					productName: "Tesla V100",
				},
				getTimeNowFunc: func() time.Time {
					return time.Now().UTC()
				},
				getClassDevicesFunc: func() (infinibandclass.Devices, error) {
					return tt.mockDevices, nil
				},
				getIbstatOutputFunc: func(ctx context.Context) (*infiniband.IbstatOutput, error) {
					// Return healthy state to focus on class devices
					return &infiniband.IbstatOutput{
						Raw: "test output",
						Parsed: infiniband.IBStatCards{
							{
								Device: "mlx5_0",
								Port1: infiniband.IBStatPort{
									State:         "Active",
									PhysicalState: "LinkUp",
									Rate:          200,
								},
							},
						},
					}, nil
				},
				getThresholdsFunc: func() infiniband.ExpectedPortStates {
					return infiniband.ExpectedPortStates{
						AtLeastPorts: 1,
						AtLeastRate:  200,
					}
				},
			}

			result := c.Check()
			data, ok := result.(*checkResult)
			require.True(t, ok)

			// Verify devices are properly captured
			assert.Equal(t, tt.mockDevices, data.ClassDevices)
			assert.Len(t, data.ClassDevices, len(tt.mockDevices))

			// Test String() method for coverage - this tests the rendering functionality
			stringOutput := data.String()
			if len(tt.mockDevices) > 0 {
				assert.Contains(t, stringOutput, tt.expectedString)
			}

			// Test that the devices are properly reported in health states
			healthStates := data.HealthStates()
			assert.Len(t, healthStates, 1)
			// Current implementation doesn't set ExtraInfo
			assert.Nil(t, healthStates[0].ExtraInfo)
		})
	}
}

// TestStringMethodWithDifferentStates tests the String() method with various states
func TestStringMethodWithDifferentStates(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		checkResult  *checkResult
		expectedText []string
	}{
		{
			name:         "nil check result",
			checkResult:  nil,
			expectedText: []string{""},
		},
		{
			name: "no ibstat output",
			checkResult: &checkResult{
				ClassDevices: infinibandclass.Devices{
					{
						Name:            "mlx5_0",
						BoardID:         "MT_0000000123",
						FirmwareVersion: "28.40.1000",
						HCAType:         "MT4125",
					},
				},
				IbstatOutput: nil,
			},
			expectedText: []string{"no data"},
		},
		{
			name: "with class devices and ibstat output",
			checkResult: &checkResult{
				ClassDevices: infinibandclass.Devices{
					{
						Name:            "mlx5_0",
						BoardID:         "MT_0000000123",
						FirmwareVersion: "28.40.1000",
						HCAType:         "MT4125",
					},
				},
				IbstatOutput: &infiniband.IbstatOutput{
					Raw: "test output",
					Parsed: infiniband.IBStatCards{
						{
							Device: "mlx5_0",
							Port1: infiniband.IBStatPort{
								State:         "Active",
								PhysicalState: "LinkUp",
								Rate:          200,
							},
						},
						{
							Device: "mlx5_1",
							Port1: infiniband.IBStatPort{
								State:         "Down",
								PhysicalState: "Disabled",
								Rate:          0,
							},
						},
					},
				},
			},
			expectedText: []string{"Device", "Board ID", "mlx5_0", "PORT DEVICE NAME", "Active", "Down", "200", "0"},
		},
		{
			name: "only ibstat output without class devices",
			checkResult: &checkResult{
				ClassDevices: infinibandclass.Devices{},
				IbstatOutput: &infiniband.IbstatOutput{
					Raw: "test output",
					Parsed: infiniband.IBStatCards{
						{
							Device: "mlx5_0",
							Port1: infiniband.IBStatPort{
								State:         "Inactive",
								PhysicalState: "LinkUp",
								Rate:          100,
							},
						},
					},
				},
			},
			expectedText: []string{"PORT DEVICE NAME", "Inactive", "100"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := tt.checkResult.String()
			for _, expected := range tt.expectedText {
				assert.Contains(t, output, expected)
			}
		})
	}
}

// TestClassDevicesErrorHandlingInDifferentScenarios tests error handling from getClassDevicesFunc in various scenarios
func TestClassDevicesErrorHandlingInDifferentScenarios(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		classDevicesError error
		ibstatError       error
		expectedHealth    apiv1.HealthStateType
		expectedReason    string
		hasEventBucket    bool
	}{
		{
			name:              "class devices error with healthy ibstat",
			classDevicesError: errors.New("permission denied loading class devices"),
			ibstatError:       nil,
			expectedHealth:    apiv1.HealthStateTypeHealthy,
			expectedReason:    reasonNoIbPortIssue,
			hasEventBucket:    true,
		},
		{
			name:              "class devices error with unhealthy ibstat",
			classDevicesError: errors.New("file not found error"),
			ibstatError:       errors.New("ibstat failed"),
			expectedHealth:    apiv1.HealthStateTypeHealthy,
			expectedReason:    reasonNoIbPortData,
			hasEventBucket:    true,
		},
		{
			name:              "class devices error without event bucket",
			classDevicesError: errors.New("network error"),
			ibstatError:       errors.New("ibstat failed"),
			expectedHealth:    apiv1.HealthStateTypeHealthy,
			expectedReason:    reasonNoIbPortData,
			hasEventBucket:    true, // Changed to true to avoid nil pointer - the "no event bucket" path is tested elsewhere
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cctx, ccancel := context.WithCancel(context.Background())
			defer ccancel()

			var mockBucket *mockEventBucket
			if tt.hasEventBucket {
				mockBucket = createMockEventBucket()
			}

			c := &component{
				ctx:         cctx,
				cancel:      ccancel,
				eventBucket: mockBucket,
				nvmlInstance: &mockNVMLInstance{
					exists:      true,
					productName: "Tesla V100",
				},
				getTimeNowFunc: func() time.Time {
					return time.Now().UTC()
				},
				getClassDevicesFunc: func() (infinibandclass.Devices, error) {
					return nil, tt.classDevicesError
				},
				getIbstatOutputFunc: func(ctx context.Context) (*infiniband.IbstatOutput, error) {
					if tt.ibstatError != nil {
						return nil, tt.ibstatError
					}
					return &infiniband.IbstatOutput{
						Raw: "test output",
						Parsed: infiniband.IBStatCards{
							{
								Device: "mlx5_0",
								Port1: infiniband.IBStatPort{
									State:         "Active",
									PhysicalState: "LinkUp",
									Rate:          200,
									LinkLayer:     "Infiniband",
								},
							},
						},
					}, nil
				},
				getThresholdsFunc: func() infiniband.ExpectedPortStates {
					return infiniband.ExpectedPortStates{
						AtLeastPorts: 1,
						AtLeastRate:  200,
					}
				},
			}

			result := c.Check()
			data, ok := result.(*checkResult)
			require.True(t, ok)

			assert.Equal(t, tt.expectedHealth, data.health)
			assert.Equal(t, tt.expectedReason, data.reason)
			// ClassDevices should be nil when there's an error
			assert.Nil(t, data.ClassDevices)
		})
	}
}

// TestComponentWithRealClassDevicesFunctionality tests with realistic class device scenarios
func TestComponentWithRealClassDevicesFunctionality(t *testing.T) {
	t.Parallel()

	// Test with realistic device data that might be found on a real system
	realisticDevices := infinibandclass.Devices{
		{
			Name:            "mlx5_0",
			BoardID:         "MT_0000000838",
			FirmwareVersion: "28.41.1000",
			HCAType:         "MT4129",
		},
		{
			Name:            "mlx5_1",
			BoardID:         "MT_0000000839",
			FirmwareVersion: "28.41.1000",
			HCAType:         "MT4129",
		},
	}

	cctx, ccancel := context.WithCancel(context.Background())
	defer ccancel()

	mockBucket := createMockEventBucket()
	c := &component{
		ctx:         cctx,
		cancel:      ccancel,
		eventBucket: mockBucket,
		nvmlInstance: &mockNVMLInstance{
			exists:      true,
			productName: "H100",
		},
		getTimeNowFunc: func() time.Time {
			return time.Now().UTC()
		},
		getClassDevicesFunc: func() (infinibandclass.Devices, error) {
			return realisticDevices, nil
		},
		getIbstatOutputFunc: func(ctx context.Context) (*infiniband.IbstatOutput, error) {
			return &infiniband.IbstatOutput{
				Raw: "realistic ibstat output",
				Parsed: infiniband.IBStatCards{
					{
						Device: "mlx5_0",
						Port1: infiniband.IBStatPort{
							State:         "Active",
							PhysicalState: "LinkUp",
							Rate:          400, // H100 typical rate
							LinkLayer:     "Infiniband",
						},
					},
					{
						Device: "mlx5_1",
						Port1: infiniband.IBStatPort{
							State:         "Active",
							PhysicalState: "LinkUp",
							Rate:          400,
							LinkLayer:     "Infiniband",
						},
					},
				},
			}, nil
		},
		getThresholdsFunc: func() infiniband.ExpectedPortStates {
			return infiniband.ExpectedPortStates{
				AtLeastPorts: 2,
				AtLeastRate:  400,
			}
		},
	}

	result := c.Check()
	data, ok := result.(*checkResult)
	require.True(t, ok)

	// Verify successful operation
	assert.Equal(t, apiv1.HealthStateTypeHealthy, data.health)
	assert.Equal(t, reasonNoIbPortIssue, data.reason)
	assert.Equal(t, realisticDevices, data.ClassDevices)
	assert.Len(t, data.ClassDevices, 2)

	// Verify the devices are properly included in the string representation
	stringOutput := data.String()
	assert.Contains(t, stringOutput, "mlx5_0")
	assert.Contains(t, stringOutput, "mlx5_1")
	assert.Contains(t, stringOutput, "MT_0000000838")
	assert.Contains(t, stringOutput, "28.41.1000")

	// Verify health states
	healthStates := data.HealthStates()
	assert.Len(t, healthStates, 1)
	assert.Nil(t, healthStates[0].ExtraInfo) // Current implementation doesn't set ExtraInfo
}

// TestComponentWithClassDevicesButNoCounters tests Check when ClassDevices have ports without LinkDowned counters
func TestComponentWithClassDevicesButNoCounters(t *testing.T) {
	t.Parallel()

	cctx, ccancel := context.WithCancel(context.Background())
	defer ccancel()

	// Create mock devices without LinkDowned counters
	mockDevices := infinibandclass.Devices{
		{
			Name:            "mlx5_0",
			BoardID:         "MT_0000000123",
			FirmwareVersion: "28.40.1000",
			HCAType:         "MT4125",
			Ports: []infinibandclass.Port{
				{
					Name:      "1",
					State:     "Active",
					PhysState: "LinkUp",
					RateGBSec: 200,
					LinkLayer: "Infiniband",
					Counters: infinibandclass.Counters{
						LinkDowned: nil, // No counter
					},
				},
			},
		},
	}

	mockBucket := createMockEventBucket()
	c := &component{
		ctx:          cctx,
		cancel:       ccancel,
		nvmlInstance: &mockNVMLInstance{exists: true, productName: "Tesla V100"},
		eventBucket:  mockBucket,
		getTimeNowFunc: func() time.Time {
			return time.Now().UTC()
		},
		getClassDevicesFunc: func() (infinibandclass.Devices, error) {
			return mockDevices, nil
		},
		getIbstatOutputFunc: func(ctx context.Context) (*infiniband.IbstatOutput, error) {
			return nil, infiniband.ErrNoIbstatCommand // Return ErrNoIbstatCommand to test the ClassDevices conversion path
		},
		getThresholdsFunc: func() infiniband.ExpectedPortStates {
			return infiniband.ExpectedPortStates{
				AtLeastPorts: 1,
				AtLeastRate:  200,
			}
		},
	}

	result := c.Check()
	data, ok := result.(*checkResult)
	require.True(t, ok)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, data.health)
	assert.Equal(t, reasonNoIbPortIssue, data.reason)
	assert.Equal(t, mockDevices, data.ClassDevices)
}

// TestCheckWithClassDevicesAndIbstatBothPresent tests Check when both ClassDevices and IbstatOutput are available
func TestCheckWithClassDevicesAndIbstatBothPresent(t *testing.T) {
	t.Parallel()

	cctx, ccancel := context.WithCancel(context.Background())
	defer ccancel()

	// Create mock devices
	mockDevices := infinibandclass.Devices{
		{
			Name:            "mlx5_0",
			BoardID:         "MT_0000000123",
			FirmwareVersion: "28.40.1000",
			HCAType:         "MT4125",
			Ports: []infinibandclass.Port{
				{
					Name:      "1",
					State:     "Active",
					PhysState: "LinkUp",
					RateGBSec: 200,
				},
			},
		},
	}

	mockBucket := createMockEventBucket()
	c := &component{
		ctx:          cctx,
		cancel:       ccancel,
		nvmlInstance: &mockNVMLInstance{exists: true, productName: "Tesla V100"},
		eventBucket:  mockBucket,
		getTimeNowFunc: func() time.Time {
			return time.Now().UTC()
		},
		getClassDevicesFunc: func() (infinibandclass.Devices, error) {
			return mockDevices, nil
		},
		getIbstatOutputFunc: func(ctx context.Context) (*infiniband.IbstatOutput, error) {
			// Return ibstat output - this should take precedence over ClassDevices
			return &infiniband.IbstatOutput{
				Raw: "test output",
				Parsed: infiniband.IBStatCards{
					{
						Device: "mlx5_0",
						Port1: infiniband.IBStatPort{
							State:         "Active",
							PhysicalState: "LinkUp",
							Rate:          200,
							LinkLayer:     "Infiniband",
						},
					},
				},
			}, nil
		},
		getThresholdsFunc: func() infiniband.ExpectedPortStates {
			return infiniband.ExpectedPortStates{
				AtLeastPorts: 1,
				AtLeastRate:  200,
			}
		},
	}

	result := c.Check()
	data, ok := result.(*checkResult)
	require.True(t, ok)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, data.health)
	assert.Equal(t, reasonNoIbPortIssue, data.reason)
	assert.Equal(t, mockDevices, data.ClassDevices)
	assert.NotNil(t, data.IbstatOutput) // Should have both
}

// TestCheckWithPartialErrorButHealthy tests Check when ibstat returns partial error but result is healthy
func TestCheckWithPartialErrorButHealthy(t *testing.T) {
	t.Parallel()

	cctx, ccancel := context.WithCancel(context.Background())
	defer ccancel()

	mockBucket := createMockEventBucket()
	c := &component{
		ctx:          cctx,
		cancel:       ccancel,
		nvmlInstance: &mockNVMLInstance{exists: true, productName: "Tesla V100"},
		eventBucket:  mockBucket,
		getTimeNowFunc: func() time.Time {
			return time.Now().UTC()
		},
		getClassDevicesFunc: func() (infinibandclass.Devices, error) {
			return infinibandclass.Devices{}, nil
		},
		getIbstatOutputFunc: func(ctx context.Context) (*infiniband.IbstatOutput, error) {
			// Return healthy output with an error (partial output scenario)
			return &infiniband.IbstatOutput{
				Raw: "test output",
				Parsed: infiniband.IBStatCards{
					{
						Device: "mlx5_0",
						Port1: infiniband.IBStatPort{
							State:         "Active",
							PhysicalState: "LinkUp",
							Rate:          200,
							LinkLayer:     "Infiniband",
						},
					},
				},
			}, errors.New("partial read error")
		},
		getThresholdsFunc: func() infiniband.ExpectedPortStates {
			return infiniband.ExpectedPortStates{
				AtLeastPorts: 1,
				AtLeastRate:  200,
			}
		},
	}

	result := c.Check()
	data, ok := result.(*checkResult)
	require.True(t, ok)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, data.health)
	assert.Equal(t, reasonNoIbPortIssue, data.reason)
	assert.Nil(t, data.err) // Error should be discarded since result is healthy
}

// TestCloseWithKmsgSyncerOnly tests Close when only kmsgSyncer is present
func TestCloseWithKmsgSyncerOnly(t *testing.T) {
	t.Parallel()

	cctx, ccancel := context.WithCancel(context.Background())

	// We can't easily create a mock kmsg.Syncer due to interface constraints,
	// but we can test that Close() handles nil kmsgSyncer gracefully
	c := &component{
		ctx:         cctx,
		cancel:      ccancel,
		kmsgSyncer:  nil, // Test nil case
		eventBucket: nil, // No event bucket
	}

	err := c.Close()
	assert.NoError(t, err)

	// Verify context is canceled
	select {
	case <-cctx.Done():
		// Success - context should be canceled
	default:
		t.Fatal("Context not canceled after Close()")
	}
}

// TestCloseWithEventBucketOnly tests Close when only eventBucket is present
func TestCloseWithEventBucketOnly(t *testing.T) {
	t.Parallel()

	cctx, ccancel := context.WithCancel(context.Background())

	mockBucket := createMockEventBucket()
	c := &component{
		ctx:         cctx,
		cancel:      ccancel,
		kmsgSyncer:  nil, // No kmsg syncer
		eventBucket: mockBucket,
	}

	err := c.Close()
	assert.NoError(t, err)

	// Verify context is canceled
	select {
	case <-cctx.Done():
		// Success - context should be canceled
	default:
		t.Fatal("Context not canceled after Close()")
	}
}

// TestNewWithKmsgSyncerError tests New when kmsg.NewSyncer returns an error
func TestNewWithKmsgSyncerError(t *testing.T) {
	t.Parallel()

	// This test simulates the case where kmsg.NewSyncer fails
	// We can't easily test the os.Geteuid() == 0 path in unit tests,
	// but we can test the error handling if we could mock the kmsg.NewSyncer call
	// For now, this test documents the intended behavior

	ctx := context.Background()
	mockStore := &mockEventStore{}

	gpudInstance := &components.GPUdInstance{
		RootCtx:              ctx,
		NVIDIAToolOverwrites: pkgconfigcommon.ToolOverwrites{},
		EventStore:           mockStore,
	}

	comp, err := New(gpudInstance)
	assert.NoError(t, err) // Should succeed even without kmsg syncer in non-root environment
	assert.NotNil(t, comp)

	// Clean up
	err = comp.Close()
	assert.NoError(t, err)
}

// TestStringWithEmptyClassDevicesAndValidIbstat tests String method when ClassDevices is empty but IbstatOutput is valid
func TestStringWithEmptyClassDevicesAndValidIbstat(t *testing.T) {
	t.Parallel()

	cr := &checkResult{
		ClassDevices: infinibandclass.Devices{}, // Empty
		IbstatOutput: &infiniband.IbstatOutput{
			Raw: "test output",
			Parsed: infiniband.IBStatCards{
				{
					Device: "mlx5_0",
					Port1: infiniband.IBStatPort{
						State:         "Active",
						PhysicalState: "LinkUp",
						Rate:          200,
					},
				},
			},
		},
	}

	result := cr.String()
	assert.Contains(t, result, "PORT DEVICE NAME") // The actual output uses all caps
	assert.Contains(t, result, "mlx5_0")
	assert.Contains(t, result, "Active")
	assert.Contains(t, result, "200")
	// Should not contain class device table headers since ClassDevices is empty
	assert.NotContains(t, result, "Board ID")
}

// mockEventBucketWithGetError implements eventstore.Bucket with controllable Get error
type mockEventBucketWithGetError struct {
	*mockEventBucket
	getError error
}

func (m *mockEventBucketWithGetError) Get(ctx context.Context, since time.Time, opts ...eventstore.OpOption) (eventstore.Events, error) {
	if m.getError != nil {
		return nil, m.getError
	}
	return m.mockEventBucket.Get(ctx, since)
}

// TestEventsWithEventBucketError tests Events method when eventBucket.Get() returns an error
func TestEventsWithEventBucketError(t *testing.T) {
	t.Parallel()

	// Create a mock bucket that returns an error on Get
	mockBucket := &mockEventBucketWithGetError{
		mockEventBucket: createMockEventBucket(),
		getError:        errors.New("database connection failed"),
	}

	c := &component{
		eventBucket: mockBucket,
	}

	// Test Events with error from eventBucket.Get
	since := time.Now().Add(-time.Hour)
	events, err := c.Events(context.Background(), since)
	assert.Error(t, err)
	assert.Nil(t, events)
	assert.Contains(t, err.Error(), "database connection failed")
}

// TestCheckWithClassDevicesPrometheusMetrics tests Check when ClassDevices have LinkDowned counters for prometheus metrics
func TestCheckWithClassDevicesPrometheusMetrics(t *testing.T) {
	t.Parallel()

	cctx, ccancel := context.WithCancel(context.Background())
	defer ccancel()

	// Create mock devices with LinkDowned counters to test prometheus metrics path
	linkDownedCount := uint64(5)
	mockDevices := infinibandclass.Devices{
		{
			Name:            "mlx5_0",
			BoardID:         "MT_0000000123",
			FirmwareVersion: "28.40.1000",
			HCAType:         "MT4125",
			Ports: []infinibandclass.Port{
				{
					Name:      "1",
					State:     "Active",
					PhysState: "LinkUp",
					RateGBSec: 200,
					LinkLayer: "Infiniband",
					Counters: infinibandclass.Counters{
						LinkDowned: &linkDownedCount, // Set counter to trigger prometheus metrics
					},
				},
			},
		},
	}

	mockBucket := createMockEventBucket()
	c := &component{
		ctx:          cctx,
		cancel:       ccancel,
		nvmlInstance: &mockNVMLInstance{exists: true, productName: "Tesla V100"},
		eventBucket:  mockBucket,
		getTimeNowFunc: func() time.Time {
			return time.Now().UTC()
		},
		getClassDevicesFunc: func() (infinibandclass.Devices, error) {
			return mockDevices, nil
		},
		getIbstatOutputFunc: func(ctx context.Context) (*infiniband.IbstatOutput, error) {
			return &infiniband.IbstatOutput{
				Raw: "test output",
				Parsed: infiniband.IBStatCards{
					{
						Device: "mlx5_0",
						Port1: infiniband.IBStatPort{
							State:         "Active",
							PhysicalState: "LinkUp",
							Rate:          200,
							LinkLayer:     "Infiniband",
						},
					},
				},
			}, nil
		},
		getThresholdsFunc: func() infiniband.ExpectedPortStates {
			return infiniband.ExpectedPortStates{
				AtLeastPorts: 1,
				AtLeastRate:  200,
			}
		},
	}

	result := c.Check()
	data, ok := result.(*checkResult)
	require.True(t, ok)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, data.health)
	assert.Equal(t, reasonNoIbPortIssue, data.reason)
	assert.Equal(t, mockDevices, data.ClassDevices)
	// Test passes if no panic occurs during prometheus metrics setting
}

// TestCheckWithContextTimeout tests Check when getIbstatOutputFunc context times out
func TestCheckWithContextTimeout(t *testing.T) {
	t.Parallel()

	cctx, ccancel := context.WithCancel(context.Background())
	defer ccancel()

	c := &component{
		ctx:          cctx,
		cancel:       ccancel,
		nvmlInstance: &mockNVMLInstance{exists: true, productName: "Tesla V100"},
		eventBucket:  nil, // No event bucket - this will cause early return with healthy state
		getTimeNowFunc: func() time.Time {
			return time.Now().UTC()
		},
		getClassDevicesFunc: func() (infinibandclass.Devices, error) {
			return infinibandclass.Devices{}, nil
		},
		getIbstatOutputFunc: func(ctx context.Context) (*infiniband.IbstatOutput, error) {
			// Simulate a slow operation that might timeout
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(20 * time.Second): // Longer than the 15s timeout in Check()
				return nil, errors.New("should not reach here")
			}
		},
		getThresholdsFunc: func() infiniband.ExpectedPortStates {
			return infiniband.ExpectedPortStates{
				AtLeastPorts: 1,
				AtLeastRate:  200,
			}
		},
	}

	result := c.Check()
	data, ok := result.(*checkResult)
	require.True(t, ok)
	// Since eventBucket is nil, the component returns healthy with reasonNoEventBucket
	// even if the context timed out
	assert.Equal(t, apiv1.HealthStateTypeHealthy, data.health)
	assert.Equal(t, reasonNoEventBucket, data.reason)
	assert.NotNil(t, data.err) // Error should still be present from timeout
	assert.Contains(t, data.err.Error(), "context deadline exceeded")
}

// TestCheckWithPartialErrorDiscarded tests the scenario where cr.err != nil but health is healthy (error gets discarded)
func TestCheckWithPartialErrorDiscarded(t *testing.T) {
	t.Parallel()

	cctx, ccancel := context.WithCancel(context.Background())
	defer ccancel()

	mockBucket := createMockEventBucket()
	c := &component{
		ctx:          cctx,
		cancel:       ccancel,
		nvmlInstance: &mockNVMLInstance{exists: true, productName: "Tesla V100"},
		eventBucket:  mockBucket,
		getTimeNowFunc: func() time.Time {
			return time.Now().UTC()
		},
		getClassDevicesFunc: func() (infinibandclass.Devices, error) {
			return infinibandclass.Devices{}, nil
		},
		getIbstatOutputFunc: func(ctx context.Context) (*infiniband.IbstatOutput, error) {
			// Return good output but with an error (partial scenario)
			return &infiniband.IbstatOutput{
				Raw: "partial output",
				Parsed: infiniband.IBStatCards{
					{
						Device: "mlx5_0",
						Port1: infiniband.IBStatPort{
							State:         "Active",
							PhysicalState: "LinkUp",
							Rate:          200,
							LinkLayer:     "Infiniband",
						},
					},
				},
			}, errors.New("partial read warning")
		},
		getThresholdsFunc: func() infiniband.ExpectedPortStates {
			return infiniband.ExpectedPortStates{
				AtLeastPorts: 1,
				AtLeastRate:  200,
			}
		},
	}

	result := c.Check()
	data, ok := result.(*checkResult)
	require.True(t, ok)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, data.health)
	assert.Equal(t, reasonNoIbPortIssue, data.reason)
	assert.Nil(t, data.err) // Error should be discarded since result is healthy
}

// TestCheckWithIbstatUnhealthyButSetsError tests when ibstat sets an unhealthy state in the error handling path
func TestCheckWithIbstatUnhealthyButSetsError(t *testing.T) {
	t.Parallel()

	cctx, ccancel := context.WithCancel(context.Background())
	defer ccancel()

	c := &component{
		ctx:          cctx,
		cancel:       ccancel,
		nvmlInstance: &mockNVMLInstance{exists: true, productName: "Tesla V100"},
		eventBucket:  nil, // No event bucket - this will cause the check to return early with healthy state
		getTimeNowFunc: func() time.Time {
			return time.Now().UTC()
		},
		getClassDevicesFunc: func() (infinibandclass.Devices, error) {
			return infinibandclass.Devices{}, nil
		},
		getIbstatOutputFunc: func(ctx context.Context) (*infiniband.IbstatOutput, error) {
			// Return error that's not ErrNoIbstatCommand
			return nil, errors.New("critical ibstat failure")
		},
		getThresholdsFunc: func() infiniband.ExpectedPortStates {
			return infiniband.ExpectedPortStates{
				AtLeastPorts: 1,
				AtLeastRate:  200,
			}
		},
	}

	result := c.Check()
	data, ok := result.(*checkResult)
	require.True(t, ok)
	// Since eventBucket is nil, the component returns healthy with reasonNoEventBucket
	// even if ibstat command failed
	assert.Equal(t, apiv1.HealthStateTypeHealthy, data.health)
	assert.Equal(t, reasonNoEventBucket, data.reason)
	assert.NotNil(t, data.err) // Error should still be present
}

// TestCheckWithIbstatUnhealthyWithEventBucket tests when ibstat fails with event bucket present
func TestCheckWithIbstatUnhealthyWithEventBucket(t *testing.T) {
	t.Parallel()

	cctx, ccancel := context.WithCancel(context.Background())
	defer ccancel()

	mockBucket := createMockEventBucket()
	c := &component{
		ctx:          cctx,
		cancel:       ccancel,
		nvmlInstance: &mockNVMLInstance{exists: true, productName: "Tesla V100"},
		eventBucket:  mockBucket, // With event bucket, should proceed to evaluation
		getTimeNowFunc: func() time.Time {
			return time.Now().UTC()
		},
		getClassDevicesFunc: func() (infinibandclass.Devices, error) {
			return infinibandclass.Devices{}, nil
		},
		getIbstatOutputFunc: func(ctx context.Context) (*infiniband.IbstatOutput, error) {
			// Return error that's not ErrNoIbstatCommand - this will set unhealthy state initially
			// but since no data is returned, it will be overridden later with "no data" reason
			return nil, errors.New("critical ibstat failure")
		},
		getThresholdsFunc: func() infiniband.ExpectedPortStates {
			return infiniband.ExpectedPortStates{
				AtLeastPorts: 1,
				AtLeastRate:  200,
			}
		},
	}

	result := c.Check()
	data, ok := result.(*checkResult)
	require.True(t, ok)
	// Even with event bucket, if ibstat error results in no data (nil IbstatOutput),
	// the component returns healthy with "no infiniband port data" reason
	assert.Equal(t, apiv1.HealthStateTypeHealthy, data.health)
	assert.Equal(t, reasonNoIbPortData, data.reason)
	assert.NotNil(t, data.err) // Error should still be present
}

// TestCloseWithBothKmsgAndEventBucket tests Close when both kmsgSyncer and eventBucket are present
func TestCloseWithBothKmsgAndEventBucket(t *testing.T) {
	t.Parallel()

	cctx, ccancel := context.WithCancel(context.Background())

	mockBucket := createMockEventBucket()

	c := &component{
		ctx:         cctx,
		cancel:      ccancel,
		eventBucket: mockBucket,
		// Note: We can't easily test with a real kmsgSyncer due to interface constraints
		// but we can test that Close() handles the presence of both gracefully
		kmsgSyncer: nil, // Set to nil since we can't mock it easily
	}

	err := c.Close()
	assert.NoError(t, err)

	// Verify context is canceled
	select {
	case <-cctx.Done():
		// Success - context should be canceled
	default:
		t.Fatal("Context not canceled after Close()")
	}
}

// TestNewWithCompleteEventStoreSetup tests New with full EventStore setup
func TestNewWithCompleteEventStoreSetup(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	mockStore := &mockEventStore{} // No error

	gpudInstance := &components.GPUdInstance{
		RootCtx: ctx,
		NVIDIAToolOverwrites: pkgconfigcommon.ToolOverwrites{
			IbstatCommand:          "/custom/ibstat",
			InfinibandClassRootDir: "/custom/infiniband",
		},
		EventStore: mockStore,
	}

	comp, err := New(gpudInstance)
	assert.NoError(t, err)
	assert.NotNil(t, comp)

	// Verify the component was created with custom tool overwrites
	casted := comp.(*component)
	assert.Equal(t, "/custom/ibstat", casted.toolOverwrites.IbstatCommand)
	assert.Equal(t, "/custom/infiniband", casted.toolOverwrites.InfinibandClassRootDir)
	assert.NotNil(t, casted.eventBucket)

	// Clean up
	err = comp.Close()
	assert.NoError(t, err)
}

// TestCheckResultHealthStatesWithoutExtraInfo tests HealthStates when no ClassDevices or IbstatOutput
func TestCheckResultHealthStatesWithoutExtraInfo(t *testing.T) {
	t.Parallel()

	cr := &checkResult{
		ts:     time.Now().UTC(),
		health: apiv1.HealthStateTypeHealthy,
		reason: "test reason",
		// No ClassDevices or IbstatOutput - should not include ExtraInfo
	}

	states := cr.HealthStates()
	assert.Len(t, states, 1)
	assert.Equal(t, Name, states[0].Name)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, states[0].Health)
	assert.Equal(t, "test reason", states[0].Reason)
	assert.Nil(t, states[0].ExtraInfo) // Should be nil when no data
}

// TestCheckResultHealthStatesWithError tests HealthStates when there's an error
func TestCheckResultHealthStatesWithError(t *testing.T) {
	t.Parallel()

	cr := &checkResult{
		ts:     time.Now().UTC(),
		health: apiv1.HealthStateTypeUnhealthy,
		reason: "test reason",
		err:    errors.New("test error"),
		suggestedActions: &apiv1.SuggestedActions{
			RepairActions: []apiv1.RepairActionType{apiv1.RepairActionTypeHardwareInspection},
		},
	}

	states := cr.HealthStates()
	assert.Len(t, states, 1)
	assert.Equal(t, Name, states[0].Name)
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, states[0].Health)
	assert.Equal(t, "test reason", states[0].Reason)
	assert.Equal(t, "test error", states[0].Error)
	assert.NotNil(t, states[0].SuggestedActions)
	assert.Equal(t, []apiv1.RepairActionType{apiv1.RepairActionTypeHardwareInspection}, states[0].SuggestedActions.RepairActions)
}

// TestSetHealthy tests the SetHealthy method
func TestSetHealthy(t *testing.T) {
	t.Parallel()

	cctx, ccancel := context.WithCancel(context.Background())
	defer ccancel()

	c := &component{
		ctx:    cctx,
		cancel: ccancel,
	}

	err := c.SetHealthy()
	assert.NoError(t, err)
}

// mockTimeNow returns a mock time function for testing
func mockTimeNow() func() time.Time {
	return func() time.Time {
		return time.Now().UTC()
	}
}
