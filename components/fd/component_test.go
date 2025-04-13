package fd

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/pkg/eventstore"
)

func TestDataGetStatesNil(t *testing.T) {
	// Test with nil data
	var d *Data
	states, err := d.getStates()
	assert.NoError(t, err)
	assert.Len(t, states, 1)
	assert.Equal(t, Name, states[0].Name)
	assert.Equal(t, apiv1.StateTypeHealthy, states[0].Health)
	assert.True(t, states[0].DeprecatedHealthy)
	assert.Equal(t, "no data yet", states[0].Reason)
}

func TestDataGetStatesWithError(t *testing.T) {
	testError := errors.New("file descriptor retrieval error")
	d := &Data{
		AllocatedFileHandles: 1000,
		Usage:                500,
		Limit:                10000,
		err:                  testError,
		healthy:              false,
		health:               apiv1.StateTypeUnhealthy,
	}

	states, err := d.getStates()
	assert.NoError(t, err)
	assert.Len(t, states, 1)
	assert.Equal(t, apiv1.StateTypeUnhealthy, states[0].Health)
	assert.False(t, states[0].DeprecatedHealthy)
	assert.Equal(t, testError.Error(), states[0].Error)
}

// MockEventStore implements a mock for eventstore.Store
type MockEventStore struct {
	mock.Mock
}

func (m *MockEventStore) Bucket(name string, opts ...eventstore.OpOption) (eventstore.Bucket, error) {
	args := m.Called(name)
	return args.Get(0).(eventstore.Bucket), args.Error(1)
}

// MockEventBucket implements a mock for eventstore.Bucket
type MockEventBucket struct {
	mock.Mock
}

func (m *MockEventBucket) Name() string {
	args := m.Called()
	return args.String(0)
}

func (m *MockEventBucket) Insert(ctx context.Context, event apiv1.Event) error {
	args := m.Called(ctx, event)
	return args.Error(0)
}

func (m *MockEventBucket) Find(ctx context.Context, event apiv1.Event) (*apiv1.Event, error) {
	args := m.Called(ctx, event)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*apiv1.Event), args.Error(1)
}

func (m *MockEventBucket) Get(ctx context.Context, since time.Time) ([]apiv1.Event, error) {
	args := m.Called(ctx, since)
	return args.Get(0).([]apiv1.Event), args.Error(1)
}

func (m *MockEventBucket) Latest(ctx context.Context) (*apiv1.Event, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*apiv1.Event), args.Error(1)
}

func (m *MockEventBucket) Purge(ctx context.Context, beforeTimestamp int64) (int, error) {
	args := m.Called(ctx, beforeTimestamp)
	return args.Int(0), args.Error(1)
}

func (m *MockEventBucket) Close() {
	m.Called()
}

func TestComponentName(t *testing.T) {
	mockEventBucket := new(MockEventBucket)
	c := &component{
		eventBucket: mockEventBucket,
	}

	assert.Equal(t, Name, c.Name())
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
	states, err := c.States(context.Background())
	assert.NoError(t, err)
	assert.Len(t, states, 1)
	assert.Equal(t, Name, states[0].Name)
	assert.Equal(t, apiv1.StateTypeHealthy, states[0].Health)
	assert.True(t, states[0].DeprecatedHealthy)
	assert.Equal(t, "no data yet", states[0].Reason)

	// Test with data
	testData := &Data{
		AllocatedFileHandles:        1000,
		RunningPIDs:                 500,
		Usage:                       800,
		Limit:                       10000,
		AllocatedFileHandlesPercent: "10.00",
		UsedPercent:                 "8.00",
		FileHandlesSupported:        true,
		FDLimitSupported:            true,
		ts:                          time.Now(),
		healthy:                     true,
		health:                      apiv1.StateTypeHealthy,
		reason:                      "current file descriptors: 800, threshold: 10000000, used_percent: 0.01",
	}
	c.lastData = testData

	states, err = c.States(context.Background())
	assert.NoError(t, err)
	assert.Len(t, states, 1)
	assert.Equal(t, Name, states[0].Name)
	assert.Equal(t, apiv1.StateTypeHealthy, states[0].Health)
	assert.True(t, states[0].DeprecatedHealthy)
	assert.Equal(t, testData.reason, states[0].Reason)
}

func TestComponentEvents(t *testing.T) {
	// Setup
	mockEventBucket := new(MockEventBucket)
	testTime := metav1.Now()
	testEvents := []apiv1.Event{
		{
			Time:    testTime,
			Name:    Name,
			Type:    apiv1.EventType("test"),
			Message: "Test event",
		},
	}

	mockEventBucket.On("Get", mock.Anything, mock.Anything).Return(testEvents, nil)

	c := &component{
		ctx:         context.Background(),
		cancel:      func() {},
		eventBucket: mockEventBucket,
	}

	// Test
	since := time.Now().Add(-time.Hour)
	events, err := c.Events(context.Background(), since)

	assert.NoError(t, err)
	assert.Equal(t, testEvents, events)
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
	c.CheckOnce()

	// Verify
	assert.NotNil(t, c.lastData)
	assert.Equal(t, uint64(1000), c.lastData.AllocatedFileHandles)
	assert.Equal(t, uint64(500), c.lastData.RunningPIDs)
	assert.Equal(t, uint64(800), c.lastData.Usage)
	assert.Equal(t, uint64(10000), c.lastData.Limit)
	assert.True(t, c.lastData.FileHandlesSupported)
	assert.True(t, c.lastData.FDLimitSupported)
	assert.True(t, c.lastData.healthy)
	assert.Equal(t, apiv1.StateTypeHealthy, c.lastData.health)
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
	c.CheckOnce()

	// Verify
	assert.NotNil(t, c.lastData)
	assert.False(t, c.lastData.healthy)
	assert.Equal(t, apiv1.StateTypeUnhealthy, c.lastData.health)
	assert.Equal(t, testError, c.lastData.err)
	assert.Contains(t, c.lastData.reason, "error getting file handles")
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
	c.CheckOnce()

	// Verify
	assert.NotNil(t, c.lastData)
	assert.False(t, c.lastData.healthy)
	assert.Equal(t, apiv1.StateTypeUnhealthy, c.lastData.health)
	assert.Equal(t, testError, c.lastData.err)
	assert.Contains(t, c.lastData.reason, "error getting running pids")
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
	c.CheckOnce()

	// Verify
	assert.NotNil(t, c.lastData)
	assert.False(t, c.lastData.healthy)
	assert.Equal(t, apiv1.StateTypeUnhealthy, c.lastData.health)
	assert.Equal(t, testError, c.lastData.err)
	assert.Contains(t, c.lastData.reason, "error getting usage")
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
	c.CheckOnce()

	// Verify
	assert.NotNil(t, c.lastData)
	assert.False(t, c.lastData.healthy)
	assert.Equal(t, apiv1.StateTypeUnhealthy, c.lastData.health)
	assert.Equal(t, testError, c.lastData.err)
	assert.Contains(t, c.lastData.reason, "error getting limit")
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
	c.CheckOnce()

	// Verify
	assert.NotNil(t, c.lastData)
	assert.False(t, c.lastData.healthy)
	assert.Equal(t, apiv1.StateTypeDegraded, c.lastData.health)
	assert.Equal(t, ErrFileHandlesAllocationExceedsWarning, c.lastData.reason)
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
	c.CheckOnce()

	// Verify
	assert.NotNil(t, c.lastData)
	assert.False(t, c.lastData.healthy)
	assert.Equal(t, apiv1.StateTypeDegraded, c.lastData.health)
	assert.Equal(t, ErrFileHandlesAllocationExceedsWarning, c.lastData.reason)
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
	c.CheckOnce()

	// Verify
	assert.NotNil(t, c.lastData)
	assert.False(t, c.lastData.healthy)
	assert.Equal(t, apiv1.StateTypeDegraded, c.lastData.health)
	// Should contain both warnings
	assert.Contains(t, c.lastData.reason, "file handles")
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
	c.CheckOnce()

	// Verify
	assert.NotNil(t, c.lastData)
	assert.False(t, c.lastData.FileHandlesSupported)
	assert.True(t, c.lastData.FDLimitSupported)
	assert.True(t, c.lastData.healthy)
	assert.Equal(t, apiv1.StateTypeHealthy, c.lastData.health)
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
	c.CheckOnce()

	// Verify
	assert.NotNil(t, c.lastData)
	assert.True(t, c.lastData.FileHandlesSupported)
	assert.False(t, c.lastData.FDLimitSupported)
	assert.True(t, c.lastData.healthy)
	assert.Equal(t, apiv1.StateTypeHealthy, c.lastData.health)
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
	mockEvent := apiv1.Event{
		Name:    Name,
		Type:    apiv1.EventType("test"),
		Message: "Test event",
	}
	mockEventBucket.On("Insert", mock.Anything, mock.Anything).Return(nil)

	c := &component{
		ctx:         context.Background(),
		eventBucket: mockEventBucket,
	}

	// Test bucket insert operation
	err := c.eventBucket.Insert(context.Background(), mockEvent)

	// Verify
	assert.NoError(t, err)
	mockEventBucket.AssertCalled(t, "Insert", mock.Anything, mock.Anything)
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
	c.CheckOnce()

	// Verify
	assert.NotNil(t, c.lastData)
	assert.Equal(t, "95.00", c.lastData.UsedPercent)
	assert.False(t, c.lastData.healthy)
	assert.Equal(t, ErrFileHandlesAllocationExceedsWarning, c.lastData.reason)
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
		expectedHealth           apiv1.StateType
		expectedHealthy          bool
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
			expectedHealth:           apiv1.StateTypeHealthy,
			expectedHealthy:          true,
			fileHandlesSupported:     true,
			fdLimitSupported:         true,
			expectReasonContainsText: "current file descriptors",
		},
		{
			name:                     "high file handles",
			allocatedFileHandles:     1000,
			runningPIDs:              500,
			usage:                    9000, // High usage triggers warning
			limit:                    10000,
			thresholdFileHandles:     5000,
			thresholdPIDs:            DefaultThresholdRunningPIDs,
			expectedHealth:           apiv1.StateTypeDegraded,
			expectedHealthy:          false,
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
			expectedHealth:           apiv1.StateTypeDegraded,
			expectedHealthy:          false,
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
			expectedHealth:           apiv1.StateTypeHealthy,
			expectedHealthy:          true,
			fileHandlesSupported:     false,
			fdLimitSupported:         true,
			expectReasonContainsText: "current file descriptors",
		},
		{
			name:                     "fd limit not supported",
			allocatedFileHandles:     1000,
			runningPIDs:              500,
			usage:                    800,
			limit:                    10000,
			thresholdFileHandles:     DefaultThresholdAllocatedFileHandles,
			thresholdPIDs:            DefaultThresholdRunningPIDs,
			expectedHealth:           apiv1.StateTypeHealthy,
			expectedHealthy:          true,
			fileHandlesSupported:     true,
			fdLimitSupported:         false,
			expectReasonContainsText: "current file descriptors",
		},
		{
			name:                     "high usage",
			allocatedFileHandles:     1000,
			runningPIDs:              500,
			usage:                    9000, // High usage triggers warning
			limit:                    10000,
			thresholdFileHandles:     5000, // Set low to trigger warning
			thresholdPIDs:            DefaultThresholdRunningPIDs,
			expectedHealth:           apiv1.StateTypeDegraded,
			expectedHealthy:          false,
			fileHandlesSupported:     true,
			fdLimitSupported:         true,
			expectReasonContainsText: ErrFileHandlesAllocationExceedsWarning,
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
			c.CheckOnce()

			// Verify
			assert.NotNil(t, c.lastData)
			assert.Equal(t, tc.expectedHealthy, c.lastData.healthy)
			assert.Equal(t, tc.expectedHealth, c.lastData.health)

			// If the text is the full error message, use exact equality.
			// Otherwise use contains for more flexible matching.
			if tc.expectReasonContainsText == ErrFileHandlesAllocationExceedsWarning {
				assert.Equal(t, tc.expectReasonContainsText, c.lastData.reason)
			} else {
				assert.Contains(t, c.lastData.reason, tc.expectReasonContainsText)
			}
		})
	}
}
