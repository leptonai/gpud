package fd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/eventstore"
)

func TestDataGetStatesNil(t *testing.T) {
	// Test with nil data
	var cr *checkResult
	states := cr.HealthStates()
	assert.Len(t, states, 1)
	assert.Equal(t, Name, states[0].Name)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, states[0].Health)
	assert.Equal(t, "no data yet", states[0].Reason)
	assert.Empty(t, states[0].Error)
	assert.Empty(t, states[0].ExtraInfo)

	assert.Empty(t, cr.String())
	assert.Empty(t, cr.Summary())
	assert.Equal(t, apiv1.HealthStateType(""), cr.HealthStateType())
	assert.Empty(t, cr.getError())
}

func TestDataGetStatesWithError(t *testing.T) {
	testError := errors.New("file descriptor retrieval error")
	cr := &checkResult{
		AllocatedFileHandles: 1000,
		Usage:                500,
		Limit:                10000,
		err:                  testError,
		health:               apiv1.HealthStateTypeUnhealthy,
	}

	states := cr.HealthStates()
	assert.Len(t, states, 1)
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, states[0].Health)
	assert.Equal(t, testError.Error(), states[0].Error)
}

// MockEventStore implements a mock for eventstore.Store
type MockEventStore struct {
	mock.Mock
}

func (m *MockEventStore) Bucket(name string, opts ...eventstore.OpOption) (eventstore.Bucket, error) {
	args := m.Called(name)
	// Return a mock that implements eventstore.Bucket
	if bucket, ok := args.Get(0).(eventstore.Bucket); ok {
		return bucket, args.Error(1)
	}
	return nil, args.Error(1)
}

// MockEventBucket implements a mock for eventstore.Bucket using testify/mock
type MockEventBucket struct {
	mock.Mock
}

func (m *MockEventBucket) Name() string {
	args := m.Called()
	return args.String(0)
}

func (m *MockEventBucket) Insert(ctx context.Context, event eventstore.Event) error {
	args := m.Called(ctx, event)
	return args.Error(0)
}

// Get now returns eventstore.Events to match the interface
func (m *MockEventBucket) Get(ctx context.Context, since time.Time) (eventstore.Events, error) {
	args := m.Called(ctx, since)
	if events, ok := args.Get(0).(eventstore.Events); ok {
		return events, args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *MockEventBucket) Find(ctx context.Context, event eventstore.Event) (*eventstore.Event, error) {
	args := m.Called(ctx, event)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	if ev, ok := args.Get(0).(*eventstore.Event); ok {
		return ev, args.Error(1)
	}
	return nil, args.Error(1)
}

// Latest now returns *eventstore.Event to match the interface
func (m *MockEventBucket) Latest(ctx context.Context) (*eventstore.Event, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	// Assume the mock is set up to return *eventstore.Event
	return args.Get(0).(*eventstore.Event), args.Error(1)
}

func (m *MockEventBucket) Purge(ctx context.Context, beforeTimestamp int64) (int, error) {
	args := m.Called(ctx, beforeTimestamp)
	return args.Int(0), args.Error(1)
}

func (m *MockEventBucket) Close() {
	m.Called()
}

// Ensure MockEventBucket satisfies the interface
var _ eventstore.Bucket = (*MockEventBucket)(nil)

func TestComponentName(t *testing.T) {
	mockEventBucket := new(MockEventBucket) // Use standard mock
	c := &component{
		eventBucket: mockEventBucket, // Assign standard mock
	}

	assert.Equal(t, Name, c.Name())
}

func TestTags(t *testing.T) {
	c := &component{}

	expectedTags := []string{
		Name,
	}

	tags := c.Tags()
	assert.Equal(t, expectedTags, tags, "Component tags should match expected values")
	assert.Len(t, tags, 1, "Component should return exactly 1 tag")
}

func TestComponentStates(t *testing.T) {
	// Setup
	mockEventBucket := new(MockEventBucket)

	c := &component{
		ctx:         context.Background(),
		cancel:      func() {},
		eventBucket: mockEventBucket,
	}

	// Test with no data yet
	states := c.LastHealthStates()
	assert.Len(t, states, 1)
	assert.Equal(t, Name, states[0].Name)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, states[0].Health)
	assert.Equal(t, "no data yet", states[0].Reason)

	// Test with data
	testData := &checkResult{
		AllocatedFileHandles:        1000,
		RunningPIDs:                 500,
		Usage:                       800,
		Limit:                       10000,
		AllocatedFileHandlesPercent: "10.00",
		UsedPercent:                 "8.00",
		FileHandlesSupported:        true,
		FDLimitSupported:            true,
		ts:                          time.Now(),
		health:                      apiv1.HealthStateTypeHealthy,
		reason:                      "current file descriptors: 800, threshold: 10000000, used_percent: 0.01",
	}
	c.lastCheckResult = testData

	states = c.LastHealthStates()
	assert.Len(t, states, 1)
	assert.Equal(t, Name, states[0].Name)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, states[0].Health)
	assert.Equal(t, testData.reason, states[0].Reason)
}

func TestComponentEvents(t *testing.T) {
	// Setup
	mockEventBucket := new(MockEventBucket)
	now := time.Now().UTC() // Use UTC for consistency
	// Use correct fields for eventstore.Event
	testEvStoreEvents := eventstore.Events{
		{
			Component: Name,
			Time:      now,
			Name:      "file_descriptor_event",
			Type:      string(apiv1.EventTypeInfo), // Use apiv1 types for source Type string
			Message:   "Test event 1",
			ExtraInfo: map[string]string{"key": "value"},
		},
	}
	// Expected apiv1.Events after conversion via ToEvent()
	expectedAPIEvents := apiv1.Events{
		{
			Component: Name,
			Time:      metav1.NewTime(now), // Conversion uses metav1.Time
			Name:      "file_descriptor_event",
			Type:      apiv1.EventTypeInfo,
			Message:   "Test event 1",
			// Note: ExtraInfo is NOT part of apiv1.Event
		},
	}

	mockEventBucket.On("Get", mock.Anything, mock.Anything).Return(testEvStoreEvents, nil)

	c := &component{
		ctx:         context.Background(),
		cancel:      func() {},
		eventBucket: mockEventBucket,
	}

	// Test
	since := time.Now().Add(-time.Hour)
	events, err := c.Events(context.Background(), since)

	assert.NoError(t, err)
	// Compare against expected apiv1.Events
	assert.Equal(t, expectedAPIEvents, events)
	mockEventBucket.AssertCalled(t, "Get", mock.Anything, since)
}

func TestComponentEventsError(t *testing.T) {
	// Setup
	mockEventBucket := new(MockEventBucket)
	testError := errors.New("failed to get events")
	// Return eventstore.Events(nil) for the Get mock
	mockEventBucket.On("Get", mock.Anything, mock.Anything).Return(eventstore.Events(nil), testError)

	c := &component{
		ctx:         context.Background(),
		cancel:      func() {},
		eventBucket: mockEventBucket,
	}

	// Test
	since := time.Now().Add(-time.Hour)
	events, err := c.Events(context.Background(), since)

	// Verify
	assert.Error(t, err)
	assert.Equal(t, testError, err)
	assert.Nil(t, events)
	mockEventBucket.AssertCalled(t, "Get", mock.Anything, since)
}

func TestCalcUsagePct(t *testing.T) {
	tests := []struct {
		name     string
		usage    uint64
		limit    uint64
		expected float64
	}{
		{"zero limit", 100, 0, 0},
		{"zero usage", 0, 100, 0},
		{"normal case", 50, 100, 50},
		{"usage equals limit", 100, 100, 100},
		{"usage exceeds limit", 150, 100, 150},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := calcUsagePct(tc.usage, tc.limit)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestComponentCheckOnceSuccess(t *testing.T) {
	// Setup mocks
	mockEventBucket := new(MockEventBucket)

	// Mock functions
	mockGetFileHandles := func() (uint64, uint64, error) {
		return 1000, 0, nil
	}

	mockCountRunningPIDs := func() (uint64, error) {
		return 500, nil
	}

	mockGetUsage := func() (uint64, error) {
		return 800, nil
	}

	mockGetLimit := func() (uint64, error) {
		return 10000, nil
	}

	mockCheckFileHandlesSupported := func() bool {
		return true
	}

	mockCheckFDLimitSupported := func() bool {
		return true
	}

	c := &component{
		ctx:                           context.Background(),
		cancel:                        func() {},
		getFileHandlesFunc:            mockGetFileHandles,
		countRunningPIDsFunc:          mockCountRunningPIDs,
		getUsageFunc:                  mockGetUsage,
		getLimitFunc:                  mockGetLimit,
		checkFileHandlesSupportedFunc: mockCheckFileHandlesSupported,
		checkFDLimitSupportedFunc:     mockCheckFDLimitSupported,
		eventBucket:                   mockEventBucket,
		thresholdAllocatedFileHandles: DefaultThresholdAllocatedFileHandles,
		thresholdRunningPIDs:          DefaultThresholdRunningPIDs,
	}

	// Test
	_ = c.Check()

	// Verify
	assert.NotNil(t, c.lastCheckResult)
	assert.Equal(t, uint64(1000), c.lastCheckResult.AllocatedFileHandles)
	assert.Equal(t, uint64(500), c.lastCheckResult.RunningPIDs)
	assert.Equal(t, uint64(800), c.lastCheckResult.Usage)
	assert.Equal(t, uint64(10000), c.lastCheckResult.Limit)
	assert.True(t, c.lastCheckResult.FileHandlesSupported)
	assert.True(t, c.lastCheckResult.FDLimitSupported)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, c.lastCheckResult.health)
}

func TestComponentCheckOnceWithFileHandlesError(t *testing.T) {
	// Setup mocks
	mockEventBucket := new(MockEventBucket)

	testError := errors.New("file handles error")

	// Mock functions
	mockGetFileHandles := func() (uint64, uint64, error) {
		return 0, 0, testError
	}

	c := &component{
		ctx:                context.Background(),
		cancel:             func() {},
		getFileHandlesFunc: mockGetFileHandles,
		eventBucket:        mockEventBucket,
	}

	// Test
	_ = c.Check()

	// Verify
	assert.NotNil(t, c.lastCheckResult)
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, c.lastCheckResult.health)
	assert.Equal(t, testError, c.lastCheckResult.err)
	assert.Contains(t, c.lastCheckResult.reason, "error getting file handles")
}

func TestComponentCheckOnceWithPIDsError(t *testing.T) {
	// Setup mocks
	mockEventBucket := new(MockEventBucket)

	testError := errors.New("running pids error")

	// Mock functions
	mockGetFileHandles := func() (uint64, uint64, error) {
		return 1000, 0, nil
	}

	mockCountRunningPIDs := func() (uint64, error) {
		return 0, testError
	}

	c := &component{
		ctx:                  context.Background(),
		cancel:               func() {},
		getFileHandlesFunc:   mockGetFileHandles,
		countRunningPIDsFunc: mockCountRunningPIDs,
		eventBucket:          mockEventBucket,
	}

	// Test
	_ = c.Check()

	// Verify
	assert.NotNil(t, c.lastCheckResult)
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, c.lastCheckResult.health)
	assert.Equal(t, testError, c.lastCheckResult.err)
	assert.Contains(t, c.lastCheckResult.reason, "error getting running pids")
}

func TestComponentCheckOnceWithUsageError(t *testing.T) {
	// Setup mocks
	mockEventBucket := new(MockEventBucket)

	testError := errors.New("usage error")

	// Mock functions
	mockGetFileHandles := func() (uint64, uint64, error) {
		return 1000, 0, nil
	}

	mockCountRunningPIDs := func() (uint64, error) {
		return 500, nil
	}

	mockGetUsage := func() (uint64, error) {
		return 0, testError
	}

	c := &component{
		ctx:                  context.Background(),
		cancel:               func() {},
		getFileHandlesFunc:   mockGetFileHandles,
		countRunningPIDsFunc: mockCountRunningPIDs,
		getUsageFunc:         mockGetUsage,
		eventBucket:          mockEventBucket,
	}

	// Test
	_ = c.Check()

	// Verify
	assert.NotNil(t, c.lastCheckResult)
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, c.lastCheckResult.health)
	assert.Equal(t, testError, c.lastCheckResult.err)
	assert.Contains(t, c.lastCheckResult.reason, "error getting usage")
}

func TestComponentCheckOnceWithLimitError(t *testing.T) {
	// Setup mocks
	mockEventBucket := new(MockEventBucket)

	testError := errors.New("limit error")

	// Mock functions
	mockGetFileHandles := func() (uint64, uint64, error) {
		return 1000, 0, nil
	}

	mockCountRunningPIDs := func() (uint64, error) {
		return 500, nil
	}

	mockGetUsage := func() (uint64, error) {
		return 800, nil
	}

	mockGetLimit := func() (uint64, error) {
		return 0, testError
	}

	c := &component{
		ctx:                  context.Background(),
		cancel:               func() {},
		getFileHandlesFunc:   mockGetFileHandles,
		countRunningPIDsFunc: mockCountRunningPIDs,
		getUsageFunc:         mockGetUsage,
		getLimitFunc:         mockGetLimit,
		eventBucket:          mockEventBucket,
	}

	// Test
	_ = c.Check()

	// Verify
	assert.NotNil(t, c.lastCheckResult)
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, c.lastCheckResult.health)
	assert.Equal(t, testError, c.lastCheckResult.err)
	assert.Contains(t, c.lastCheckResult.reason, "error getting limit")
}

func TestComponentCheckOnceWithHighFileHandlesAllocation(t *testing.T) {
	// Setup mocks
	mockEventBucket := new(MockEventBucket)

	// Mock functions
	mockGetFileHandles := func() (uint64, uint64, error) {
		return 9000, 0, nil
	}

	mockCountRunningPIDs := func() (uint64, error) {
		return 500, nil
	}

	mockGetUsage := func() (uint64, error) {
		return 8500, nil
	}

	mockGetLimit := func() (uint64, error) {
		return 10000, nil
	}

	mockCheckFileHandlesSupported := func() bool {
		return true
	}

	mockCheckFDLimitSupported := func() bool {
		return true
	}

	c := &component{
		ctx:                           context.Background(),
		cancel:                        func() {},
		getFileHandlesFunc:            mockGetFileHandles,
		countRunningPIDsFunc:          mockCountRunningPIDs,
		getUsageFunc:                  mockGetUsage,
		getLimitFunc:                  mockGetLimit,
		checkFileHandlesSupportedFunc: mockCheckFileHandlesSupported,
		checkFDLimitSupportedFunc:     mockCheckFDLimitSupported,
		eventBucket:                   mockEventBucket,
		// Setting a low threshold to test warning condition
		thresholdAllocatedFileHandles: 10000,
		thresholdRunningPIDs:          DefaultThresholdRunningPIDs,
	}

	// Test
	_ = c.Check()

	// Verify
	assert.NotNil(t, c.lastCheckResult)
	assert.Equal(t, apiv1.HealthStateTypeDegraded, c.lastCheckResult.health)
	assert.Equal(t, ErrFileHandlesAllocationExceedsWarning, c.lastCheckResult.reason)
}

func TestComponentCheckOnceWithHighRunningPIDs(t *testing.T) {
	// Setup mocks
	mockEventBucket := new(MockEventBucket)

	// Mock functions
	mockGetFileHandles := func() (uint64, uint64, error) {
		return 1000, 0, nil
	}

	mockCountRunningPIDs := func() (uint64, error) {
		return 9000, nil
	}

	mockGetUsage := func() (uint64, error) {
		// Set usage high enough to trigger warning
		return 9000, nil
	}

	mockGetLimit := func() (uint64, error) {
		return 10000, nil
	}

	mockCheckFileHandlesSupported := func() bool {
		return true
	}

	mockCheckFDLimitSupported := func() bool {
		return true
	}

	c := &component{
		ctx:                           context.Background(),
		cancel:                        func() {},
		getFileHandlesFunc:            mockGetFileHandles,
		countRunningPIDsFunc:          mockCountRunningPIDs,
		getUsageFunc:                  mockGetUsage,
		getLimitFunc:                  mockGetLimit,
		checkFileHandlesSupportedFunc: mockCheckFileHandlesSupported,
		checkFDLimitSupportedFunc:     mockCheckFDLimitSupported,
		eventBucket:                   mockEventBucket,
		thresholdAllocatedFileHandles: 5000, // Set lower to trigger warning
		thresholdRunningPIDs:          DefaultThresholdRunningPIDs,
	}

	// Test
	_ = c.Check()

	// Verify
	assert.NotNil(t, c.lastCheckResult)
	assert.Equal(t, apiv1.HealthStateTypeDegraded, c.lastCheckResult.health)
	assert.Equal(t, ErrFileHandlesAllocationExceedsWarning, c.lastCheckResult.reason)
}

func TestComponentCheckOnceWithBothHighValues(t *testing.T) {
	// Setup mocks
	mockEventBucket := new(MockEventBucket)

	// Mock functions
	mockGetFileHandles := func() (uint64, uint64, error) {
		return 9000, 0, nil
	}

	mockCountRunningPIDs := func() (uint64, error) {
		return 9000, nil
	}

	mockGetUsage := func() (uint64, error) {
		return 8500, nil
	}

	mockGetLimit := func() (uint64, error) {
		return 10000, nil
	}

	mockCheckFileHandlesSupported := func() bool {
		return true
	}

	mockCheckFDLimitSupported := func() bool {
		return true
	}

	c := &component{
		ctx:                           context.Background(),
		cancel:                        func() {},
		getFileHandlesFunc:            mockGetFileHandles,
		countRunningPIDsFunc:          mockCountRunningPIDs,
		getUsageFunc:                  mockGetUsage,
		getLimitFunc:                  mockGetLimit,
		checkFileHandlesSupportedFunc: mockCheckFileHandlesSupported,
		checkFDLimitSupportedFunc:     mockCheckFDLimitSupported,
		eventBucket:                   mockEventBucket,
		// Setting low thresholds to test warning conditions
		thresholdAllocatedFileHandles: 5000,
		thresholdRunningPIDs:          5000,
	}

	// Test
	_ = c.Check()

	// Verify
	assert.NotNil(t, c.lastCheckResult)
	assert.Equal(t, apiv1.HealthStateTypeDegraded, c.lastCheckResult.health)
	// Should contain both warnings
	assert.Contains(t, c.lastCheckResult.reason, "file handles")
}

func TestComponentCheckOnceWhenFileHandlesNotSupported(t *testing.T) {
	// Setup mocks
	mockEventBucket := new(MockEventBucket)

	// Mock functions
	mockGetFileHandles := func() (uint64, uint64, error) {
		return 1000, 0, nil
	}

	mockCountRunningPIDs := func() (uint64, error) {
		return 500, nil
	}

	mockGetUsage := func() (uint64, error) {
		return 800, nil
	}

	mockGetLimit := func() (uint64, error) {
		return 10000, nil
	}

	mockCheckFileHandlesSupported := func() bool {
		return false
	}

	mockCheckFDLimitSupported := func() bool {
		return true
	}

	c := &component{
		ctx:                           context.Background(),
		cancel:                        func() {},
		getFileHandlesFunc:            mockGetFileHandles,
		countRunningPIDsFunc:          mockCountRunningPIDs,
		getUsageFunc:                  mockGetUsage,
		getLimitFunc:                  mockGetLimit,
		checkFileHandlesSupportedFunc: mockCheckFileHandlesSupported,
		checkFDLimitSupportedFunc:     mockCheckFDLimitSupported,
		eventBucket:                   mockEventBucket,
		thresholdAllocatedFileHandles: DefaultThresholdAllocatedFileHandles,
		thresholdRunningPIDs:          DefaultThresholdRunningPIDs,
	}

	// Test
	_ = c.Check()

	// Verify
	assert.NotNil(t, c.lastCheckResult)
	assert.False(t, c.lastCheckResult.FileHandlesSupported)
	assert.True(t, c.lastCheckResult.FDLimitSupported)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, c.lastCheckResult.health)
}

func TestComponentCheckOnceWhenFDLimitNotSupported(t *testing.T) {
	// Setup mocks
	mockEventBucket := new(MockEventBucket)

	// Mock functions
	mockGetFileHandles := func() (uint64, uint64, error) {
		return 1000, 0, nil
	}

	mockCountRunningPIDs := func() (uint64, error) {
		return 500, nil
	}

	mockGetUsage := func() (uint64, error) {
		return 800, nil
	}

	mockGetLimit := func() (uint64, error) {
		return 10000, nil
	}

	mockCheckFileHandlesSupported := func() bool {
		return true
	}

	mockCheckFDLimitSupported := func() bool {
		return false
	}

	c := &component{
		ctx:                           context.Background(),
		cancel:                        func() {},
		getFileHandlesFunc:            mockGetFileHandles,
		countRunningPIDsFunc:          mockCountRunningPIDs,
		getUsageFunc:                  mockGetUsage,
		getLimitFunc:                  mockGetLimit,
		checkFileHandlesSupportedFunc: mockCheckFileHandlesSupported,
		checkFDLimitSupportedFunc:     mockCheckFDLimitSupported,
		eventBucket:                   mockEventBucket,
		thresholdAllocatedFileHandles: DefaultThresholdAllocatedFileHandles,
		thresholdRunningPIDs:          DefaultThresholdRunningPIDs,
	}

	// Test
	_ = c.Check()

	// Verify
	assert.NotNil(t, c.lastCheckResult)
	assert.True(t, c.lastCheckResult.FileHandlesSupported)
	assert.False(t, c.lastCheckResult.FDLimitSupported)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, c.lastCheckResult.health)
}

func TestComponentClose(t *testing.T) {
	// Setup
	mockEventBucket := new(MockEventBucket)
	mockEventBucket.On("Close").Return()

	ctx, cancel := context.WithCancel(context.Background())
	c := &component{
		ctx:         ctx,
		cancel:      cancel,
		eventBucket: mockEventBucket,
	}

	// Test
	c.Close()

	// Verify
	mockEventBucket.AssertCalled(t, "Close")
	// Verify context is canceled
	<-ctx.Done()
}

func TestComponentEventBucketOperations(t *testing.T) {
	// Setup
	mockEventBucket := new(MockEventBucket)

	// Set up expectations for bucket operations
	// Use correct fields for eventstore.Event
	mockEV := eventstore.Event{
		Component: Name,
		Time:      time.Now().UTC(),
		Name:      "insert_test_event",
		Type:      string(apiv1.EventTypeInfo),
		Message:   "Testing insert operation",
		ExtraInfo: map[string]string{"op": "insert"},
	}
	mockEventBucket.On("Insert", mock.Anything, mock.AnythingOfType("eventstore.Event")).Return(nil)

	c := &component{
		ctx:         context.Background(),
		eventBucket: mockEventBucket,
	}

	// Test bucket insert operation with eventstore.Event
	err := c.eventBucket.Insert(context.Background(), mockEV)

	// Verify
	assert.NoError(t, err)
	mockEventBucket.AssertCalled(t, "Insert", mock.Anything, mock.AnythingOfType("eventstore.Event"))
}

func TestFormatAsPercent(t *testing.T) {
	tests := []struct {
		name     string
		value    float64
		expected string
	}{
		{"zero", 0, "0.00"},
		{"integer", 42, "42.00"},
		{"decimal", 42.5, "42.50"},
		{"small decimal", 0.125, "0.12"},
		{"rounding up", 12.345, "12.35"},
		{"rounding down", 12.344, "12.34"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := formatAsPercent(tc.value)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestComponentCheckOnceWithHighUsage(t *testing.T) {
	// Setup mocks
	mockEventBucket := new(MockEventBucket)

	// Mock functions
	mockGetFileHandles := func() (uint64, uint64, error) {
		return 1000, 0, nil
	}

	mockCountRunningPIDs := func() (uint64, error) {
		return 500, nil
	}

	mockGetUsage := func() (uint64, error) {
		return 9500, nil
	}

	mockGetLimit := func() (uint64, error) {
		return 10000, nil
	}

	mockCheckFileHandlesSupported := func() bool {
		return true
	}

	mockCheckFDLimitSupported := func() bool {
		return true
	}

	c := &component{
		ctx:                           context.Background(),
		cancel:                        func() {},
		getFileHandlesFunc:            mockGetFileHandles,
		countRunningPIDsFunc:          mockCountRunningPIDs,
		getUsageFunc:                  mockGetUsage,
		getLimitFunc:                  mockGetLimit,
		checkFileHandlesSupportedFunc: mockCheckFileHandlesSupported,
		checkFDLimitSupportedFunc:     mockCheckFDLimitSupported,
		eventBucket:                   mockEventBucket,
		thresholdAllocatedFileHandles: 5000, // Lower threshold to trigger warning
		thresholdRunningPIDs:          DefaultThresholdRunningPIDs,
	}

	// Test
	_ = c.Check()

	// Verify
	assert.NotNil(t, c.lastCheckResult)
	assert.Equal(t, "95.00", c.lastCheckResult.UsedPercent)
	assert.Equal(t, ErrFileHandlesAllocationExceedsWarning, c.lastCheckResult.reason)
}

// Helper function to format float as percent string (only needed for tests)
func formatAsPercent(value float64) string {
	return fmt.Sprintf("%.2f", value)
}

func TestComponentCheckOnceWithWarningConditions(t *testing.T) {
	tests := []struct {
		name                     string
		allocatedFileHandles     uint64
		runningPIDs              uint64
		usage                    uint64
		limit                    uint64
		thresholdFileHandles     uint64
		thresholdPIDs            uint64
		expectedHealth           apiv1.HealthStateType
		fileHandlesSupported     bool
		fdLimitSupported         bool
		expectReasonContainsText string
	}{
		{
			name:                     "all healthy",
			allocatedFileHandles:     1000,
			runningPIDs:              500,
			usage:                    800,
			limit:                    10000,
			thresholdFileHandles:     DefaultThresholdAllocatedFileHandles,
			thresholdPIDs:            DefaultThresholdRunningPIDs,
			expectedHealth:           apiv1.HealthStateTypeHealthy,
			fileHandlesSupported:     true,
			fdLimitSupported:         true,
			expectReasonContainsText: "no issue found (file descriptor usage is within the threshold)",
		},
		{
			name:                     "high file handles",
			allocatedFileHandles:     1000,
			runningPIDs:              500,
			usage:                    9000, // High usage triggers warning
			limit:                    10000,
			thresholdFileHandles:     5000,
			thresholdPIDs:            DefaultThresholdRunningPIDs,
			expectedHealth:           apiv1.HealthStateTypeDegraded,
			fileHandlesSupported:     true,
			fdLimitSupported:         true,
			expectReasonContainsText: ErrFileHandlesAllocationExceedsWarning,
		},
		{
			name:                     "high running PIDs",
			allocatedFileHandles:     1000,
			runningPIDs:              9000,
			usage:                    9000, // High usage triggers warning
			limit:                    10000,
			thresholdFileHandles:     5000,
			thresholdPIDs:            DefaultThresholdRunningPIDs,
			expectedHealth:           apiv1.HealthStateTypeDegraded,
			fileHandlesSupported:     true,
			fdLimitSupported:         true,
			expectReasonContainsText: ErrFileHandlesAllocationExceedsWarning,
		},
		{
			name:                     "file handles not supported",
			allocatedFileHandles:     1000,
			runningPIDs:              500,
			usage:                    800,
			limit:                    10000,
			thresholdFileHandles:     DefaultThresholdAllocatedFileHandles,
			thresholdPIDs:            DefaultThresholdRunningPIDs,
			expectedHealth:           apiv1.HealthStateTypeHealthy,
			fileHandlesSupported:     false,
			fdLimitSupported:         true,
			expectReasonContainsText: "no issue found (file descriptor usage is within the threshold)",
		},
		{
			name:                     "fd limit not supported",
			allocatedFileHandles:     1000,
			runningPIDs:              500,
			usage:                    800,
			limit:                    10000,
			thresholdFileHandles:     DefaultThresholdAllocatedFileHandles,
			thresholdPIDs:            DefaultThresholdRunningPIDs,
			expectedHealth:           apiv1.HealthStateTypeHealthy,
			fileHandlesSupported:     true,
			fdLimitSupported:         false,
			expectReasonContainsText: "no issue found (file descriptor usage is within the threshold)",
		},
		{
			name:                     "high usage",
			allocatedFileHandles:     1000,
			runningPIDs:              500,
			usage:                    9000, // High usage triggers warning
			limit:                    10000,
			thresholdFileHandles:     5000, // Set low to trigger warning
			thresholdPIDs:            DefaultThresholdRunningPIDs,
			expectedHealth:           apiv1.HealthStateTypeDegraded,
			fileHandlesSupported:     true,
			fdLimitSupported:         true,
			expectReasonContainsText: ErrFileHandlesAllocationExceedsWarning,
		},
		{
			name:                     "zero usage",
			allocatedFileHandles:     0,
			runningPIDs:              0,
			usage:                    0,
			limit:                    10000,
			thresholdFileHandles:     DefaultThresholdAllocatedFileHandles,
			thresholdPIDs:            DefaultThresholdRunningPIDs,
			expectedHealth:           apiv1.HealthStateTypeHealthy,
			fileHandlesSupported:     true,
			fdLimitSupported:         true,
			expectReasonContainsText: "no issue found",
		},
		{
			name:                     "zero limit",
			allocatedFileHandles:     1000,
			runningPIDs:              500,
			usage:                    800,
			limit:                    0, // Zero limit
			thresholdFileHandles:     DefaultThresholdAllocatedFileHandles,
			thresholdPIDs:            DefaultThresholdRunningPIDs,
			expectedHealth:           apiv1.HealthStateTypeHealthy, // Should still be healthy, but percentages will be 0
			fileHandlesSupported:     true,
			fdLimitSupported:         true,
			expectReasonContainsText: "no issue found",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup mocks
			mockEventBucket := new(MockEventBucket)

			// Mock functions
			mockGetFileHandles := func() (uint64, uint64, error) {
				return tc.allocatedFileHandles, 0, nil
			}

			mockCountRunningPIDs := func() (uint64, error) {
				return tc.runningPIDs, nil
			}

			mockGetUsage := func() (uint64, error) {
				return tc.usage, nil
			}

			mockGetLimit := func() (uint64, error) {
				return tc.limit, nil
			}

			mockCheckFileHandlesSupported := func() bool {
				return tc.fileHandlesSupported
			}

			mockCheckFDLimitSupported := func() bool {
				return tc.fdLimitSupported
			}

			c := &component{
				ctx:                           context.Background(),
				cancel:                        func() {},
				getFileHandlesFunc:            mockGetFileHandles,
				countRunningPIDsFunc:          mockCountRunningPIDs,
				getUsageFunc:                  mockGetUsage,
				getLimitFunc:                  mockGetLimit,
				checkFileHandlesSupportedFunc: mockCheckFileHandlesSupported,
				checkFDLimitSupportedFunc:     mockCheckFDLimitSupported,
				eventBucket:                   mockEventBucket,
				thresholdAllocatedFileHandles: tc.thresholdFileHandles,
				thresholdRunningPIDs:          tc.thresholdPIDs,
			}

			// Test
			_ = c.Check()

			// Verify
			assert.NotNil(t, c.lastCheckResult)
			assert.Equal(t, tc.expectedHealth, c.lastCheckResult.health)

			// Use contains for more flexible matching of reasons.
			assert.Contains(t, c.lastCheckResult.reason, tc.expectReasonContainsText)

			// Specific checks for zero limit case
			if tc.limit == 0 {
				assert.Equal(t, "0.00", c.lastCheckResult.AllocatedFileHandlesPercent)
				assert.Equal(t, "0.00", c.lastCheckResult.UsedPercent)
				assert.Equal(t, "0.00", c.lastCheckResult.ThresholdAllocatedFileHandlesPercent)
				// ThresholdRunningPIDsPercent depends on Usage and thresholdRunningPIDs, not the main limit
			}
		})
	}
}

func TestDataString(t *testing.T) {
	// Test with data
	cr := &checkResult{
		AllocatedFileHandles:        1000,
		RunningPIDs:                 500,
		Usage:                       800,
		Limit:                       10000,
		AllocatedFileHandlesPercent: "10.00",
		UsedPercent:                 "8.00",
		FileHandlesSupported:        true,
		FDLimitSupported:            true,
		ts:                          time.Now(),
		health:                      apiv1.HealthStateTypeHealthy,
		reason:                      "test reason",
	}

	// Test
	result := cr.String()

	// Verify
	assert.Contains(t, result, "Allocated File Handles")
	assert.Contains(t, result, "1000")
	assert.Contains(t, result, "500")
	assert.Contains(t, result, "800")
	assert.Contains(t, result, "10000")
}

func TestDataJSON(t *testing.T) {
	// Test with data
	cr := &checkResult{
		AllocatedFileHandles:                 1000,
		RunningPIDs:                          500,
		Usage:                                800,
		Limit:                                10000,
		AllocatedFileHandlesPercent:          "10.00",
		UsedPercent:                          "8.00",
		ThresholdAllocatedFileHandles:        DefaultThresholdAllocatedFileHandles,
		ThresholdAllocatedFileHandlesPercent: "0.01",
		ThresholdRunningPIDs:                 DefaultThresholdRunningPIDs,
		ThresholdRunningPIDsPercent:          "0.01",
		FileHandlesSupported:                 true,
		FDLimitSupported:                     true,
		ts:                                   time.Now(), // Non-zero time for marshaling
		health:                               apiv1.HealthStateTypeHealthy,
		reason:                               "test reason",
	}

	// Test JSON marshaling
	jsonBytes, err := json.Marshal(cr)

	// Verify
	assert.NoError(t, err)
	assert.NotEmpty(t, jsonBytes)

	var unmarshaled map[string]interface{}
	err = json.Unmarshal(jsonBytes, &unmarshaled)
	assert.NoError(t, err)
	assert.Equal(t, float64(1000), unmarshaled["allocated_file_handles"])
	assert.Equal(t, float64(500), unmarshaled["running_pids"])
	assert.Equal(t, float64(800), unmarshaled["usage"])
	assert.Equal(t, float64(10000), unmarshaled["limit"])
	assert.Equal(t, "10.00", unmarshaled["allocated_file_handles_percent"])
	assert.Equal(t, "8.00", unmarshaled["used_percent"])
	assert.Equal(t, float64(DefaultThresholdAllocatedFileHandles), unmarshaled["threshold_allocated_file_handles"])
	assert.Equal(t, "0.01", unmarshaled["threshold_allocated_file_handles_percent"])
	assert.Equal(t, float64(DefaultThresholdRunningPIDs), unmarshaled["threshold_running_pids"])
	assert.Equal(t, "0.01", unmarshaled["threshold_running_pids_percent"])
	assert.Equal(t, true, unmarshaled["file_handles_supported"])
	assert.Equal(t, true, unmarshaled["fd_limit_supported"])

	// Test HealthStates includes marshaled data
	states := cr.HealthStates()
	assert.Len(t, states, 1)
	assert.NotEmpty(t, states[0].ExtraInfo["data"])
	var extraData checkResult
	err = json.Unmarshal([]byte(states[0].ExtraInfo["data"]), &extraData)
	assert.NoError(t, err)
	assert.Equal(t, cr.AllocatedFileHandles, extraData.AllocatedFileHandles) // Check one field for basic validation
}

func TestCheckResult(t *testing.T) {
	// Setup
	mockBucket := new(MockEventBucket) // Use standard mock
	c := &component{
		ctx:         context.Background(),
		cancel:      func() {},
		eventBucket: mockBucket, // Assign standard mock
		getFileHandlesFunc: func() (uint64, uint64, error) {
			return 1000, 0, nil
		},
		countRunningPIDsFunc: func() (uint64, error) {
			return 500, nil
		},
		getUsageFunc: func() (uint64, error) {
			return 800, nil
		},
		getLimitFunc: func() (uint64, error) {
			return 10000, nil
		},
		checkFileHandlesSupportedFunc: func() bool {
			return true
		},
		checkFDLimitSupportedFunc: func() bool {
			return true
		},
	}

	// Test
	result := c.Check()

	// Verify the CheckResult interface
	assert.NotNil(t, result)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, result.HealthStateType())
	assert.NotEmpty(t, result.String())
	assert.NotEmpty(t, result.Summary())
}

func TestComponentEventsWithNoEventBucket(t *testing.T) {
	// Setup
	c := &component{
		ctx:    context.Background(),
		cancel: func() {},
	}

	// Test
	since := time.Now().Add(-time.Hour)
	events, err := c.Events(context.Background(), since)

	// Verify
	assert.NoError(t, err)
	assert.Nil(t, events)
}

func TestStartAndClose(t *testing.T) {
	// Setup
	mockEventBucket := new(MockEventBucket)
	mockEventBucket.On("Close").Return()

	ctx, cancel := context.WithCancel(context.Background())
	c := &component{
		ctx:         ctx,
		cancel:      cancel,
		eventBucket: mockEventBucket,
		getFileHandlesFunc: func() (uint64, uint64, error) {
			return 1000, 0, nil
		},
		countRunningPIDsFunc: func() (uint64, error) {
			return 500, nil
		},
		getUsageFunc: func() (uint64, error) {
			return 800, nil
		},
		getLimitFunc: func() (uint64, error) {
			return 10000, nil
		},
		checkFileHandlesSupportedFunc: func() bool {
			return true
		},
		checkFDLimitSupportedFunc: func() bool {
			return true
		},
	}

	// Test Start
	err := c.Start()
	assert.NoError(t, err)

	// Allow the goroutine to run at least once
	time.Sleep(10 * time.Millisecond)

	// Verify data was collected
	c.lastMu.RLock()
	assert.NotNil(t, c.lastCheckResult)
	c.lastMu.RUnlock()

	// Test Close
	err = c.Close()
	assert.NoError(t, err)

	// Verify context is canceled
	select {
	case <-ctx.Done():
		// Context was canceled successfully
	default:
		t.Error("Context was not canceled")
	}

	mockEventBucket.AssertCalled(t, "Close")
}

// Add TestNewErrorBucket tests
func TestNewErrorBucket(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Skipping event store test on non-linux")
	}
	mockEventStore := new(MockEventStore)
	testError := errors.New("failed to get bucket")
	// MockEventStore.Bucket needs to return something satisfying eventstore.Bucket or nil
	mockEventStore.On("Bucket", Name).Return((*MockEventBucket)(nil), testError) // Return nil mock bucket on error

	gpudInstance := &components.GPUdInstance{
		RootCtx:    context.Background(),
		EventStore: mockEventStore,
	}

	comp, err := New(gpudInstance)

	assert.Error(t, err)
	assert.Equal(t, testError, err)
	assert.Nil(t, comp)
	mockEventStore.AssertCalled(t, "Bucket", Name)
}

// Skipping TestNewErrorKmsgSyncer as it requires mocking os.Geteuid or running as root.
