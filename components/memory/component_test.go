package memory

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/shirou/gopsutil/v4/mem"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

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
}

func TestDataGetStatesWithError(t *testing.T) {
	testError := errors.New("memory retrieval error")
	cr := &checkResult{
		TotalBytes: 16,
		UsedBytes:  8,
		health:     apiv1.HealthStateTypeUnhealthy,
		err:        testError,
	}

	states := cr.HealthStates()
	assert.Len(t, states, 1)
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, states[0].Health)
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

func (m *MockEventBucket) Insert(ctx context.Context, event eventstore.Event) error {
	args := m.Called(ctx, event)
	return args.Error(0)
}

func (m *MockEventBucket) Find(ctx context.Context, event eventstore.Event) (*eventstore.Event, error) {
	args := m.Called(ctx, event)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*eventstore.Event), args.Error(1)
}

func (m *MockEventBucket) Get(ctx context.Context, since time.Time) (eventstore.Events, error) {
	args := m.Called(ctx, since)
	return args.Get(0).(eventstore.Events), args.Error(1)
}

func (m *MockEventBucket) Latest(ctx context.Context) (*eventstore.Event, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*eventstore.Event), args.Error(1)
}

func (m *MockEventBucket) Purge(ctx context.Context, beforeTimestamp int64) (int, error) {
	args := m.Called(ctx, beforeTimestamp)
	return args.Int(0), args.Error(1)
}

func (m *MockEventBucket) Close() {
	m.Called()
}

// MockKmsgSyncer implements a mock for kmsg.Syncer
type MockKmsgSyncer struct {
	mock.Mock
}

func (m *MockKmsgSyncer) Close() error {
	args := m.Called()
	return args.Error(0)
}

func TestComponentName(t *testing.T) {
	mockEventBucket := new(MockEventBucket)
	c := &component{
		eventBucket: mockEventBucket,
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
		TotalBytes: 16,
		UsedBytes:  8,
		ts:         time.Now(),
		health:     apiv1.HealthStateTypeHealthy,
		reason:     "using 8 bytes out of total 16 bytes",
	}
	c.lastCheckResult = testData

	states = c.LastHealthStates()
	assert.Len(t, states, 1)
	assert.Equal(t, Name, states[0].Name)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, states[0].Health)
	assert.Equal(t, "using 8 bytes out of total 16 bytes", states[0].Reason)
}

func TestComponentEvents(t *testing.T) {
	// Setup
	mockEventBucket := new(MockEventBucket)
	testTime := time.Now()
	testEvents := eventstore.Events{
		{
			Time:      testTime,
			Name:      Name,
			Type:      "test",
			Message:   "Test event",
			Component: Name,
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
	assert.Equal(t, 1, len(events))
	assert.Equal(t, Name, events[0].Name)
	mockEventBucket.AssertCalled(t, "Get", mock.Anything, since)
}

func TestComponentCheckOnce(t *testing.T) {
	// Setup mocks
	mockEventBucket := new(MockEventBucket)

	// Mock virtual memory function
	mockVMStat := &mem.VirtualMemoryStat{
		Total:        16 * 1024 * 1024 * 1024, // 16GB
		Available:    8 * 1024 * 1024 * 1024,  // 8GB
		Used:         8 * 1024 * 1024 * 1024,  // 8GB
		UsedPercent:  50.0,
		Free:         7 * 1024 * 1024 * 1024,  // 7GB
		VmallocTotal: 32 * 1024 * 1024 * 1024, // 32GB
		VmallocUsed:  16 * 1024 * 1024 * 1024, // 16GB
	}

	mockGetVMFunc := func(ctx context.Context) (*mem.VirtualMemoryStat, error) {
		return mockVMStat, nil
	}

	mockGetBPFJITFunc := func() (uint64, error) {
		return 1024 * 1024, nil // 1MB
	}

	c := &component{
		ctx:                             context.Background(),
		cancel:                          func() {},
		getVirtualMemoryFunc:            mockGetVMFunc,
		getCurrentBPFJITBufferBytesFunc: mockGetBPFJITFunc,
		eventBucket:                     mockEventBucket,
	}

	// Test
	result := c.Check()

	// Verify
	assert.NotNil(t, c.lastCheckResult)
	assert.Equal(t, mockVMStat.Total, c.lastCheckResult.TotalBytes)
	assert.Equal(t, mockVMStat.Available, c.lastCheckResult.AvailableBytes)
	assert.Equal(t, mockVMStat.Used, c.lastCheckResult.UsedBytes)
	assert.Equal(t, uint64(1024*1024), c.lastCheckResult.BPFJITBufferBytes)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, c.lastCheckResult.health)

	// Test result methods
	cr := c.lastCheckResult
	assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.HealthStateType())
	assert.Contains(t, cr.String(), "Total")
	assert.Contains(t, cr.String(), "Used")
	assert.Contains(t, cr.String(), "Available")

	// Test Summary method
	assert.Equal(t, "ok", cr.Summary())

	assert.Equal(t, apiv1.HealthStateTypeHealthy, result.HealthStateType())
}

func TestComponentCheckOnceWithVMError(t *testing.T) {
	// Setup mocks
	mockEventBucket := new(MockEventBucket)
	testError := errors.New("virtual memory error")

	// Mock virtual memory function with error
	mockGetVMFunc := func(ctx context.Context) (*mem.VirtualMemoryStat, error) {
		return nil, testError
	}

	c := &component{
		ctx:                  context.Background(),
		cancel:               func() {},
		getVirtualMemoryFunc: mockGetVMFunc,
		eventBucket:          mockEventBucket,
	}

	// Test
	result := c.Check()

	// Verify
	assert.NotNil(t, c.lastCheckResult)
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, c.lastCheckResult.health)
	assert.Equal(t, testError, c.lastCheckResult.err)
	assert.Contains(t, c.lastCheckResult.reason, "error getting virtual memory")

	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, result.HealthStateType())
}

func TestComponentCheckOnceWithBPFError(t *testing.T) {
	// Setup mocks
	mockEventBucket := new(MockEventBucket)
	testError := errors.New("BPF JIT buffer error")

	// Mock virtual memory function
	mockVMStat := &mem.VirtualMemoryStat{
		Total:       16 * 1024 * 1024 * 1024, // 16GB
		Available:   8 * 1024 * 1024 * 1024,  // 8GB
		Used:        8 * 1024 * 1024 * 1024,  // 8GB
		UsedPercent: 50.0,
		Free:        7 * 1024 * 1024 * 1024, // 7GB
	}

	mockGetVMFunc := func(ctx context.Context) (*mem.VirtualMemoryStat, error) {
		return mockVMStat, nil
	}

	// Mock BPF JIT function with error
	mockGetBPFJITFunc := func() (uint64, error) {
		return 0, testError
	}

	c := &component{
		ctx:                             context.Background(),
		cancel:                          func() {},
		getVirtualMemoryFunc:            mockGetVMFunc,
		getCurrentBPFJITBufferBytesFunc: mockGetBPFJITFunc,
		eventBucket:                     mockEventBucket,
	}

	// Test
	result := c.Check()

	// Verify
	assert.NotNil(t, c.lastCheckResult)
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, c.lastCheckResult.health)
	assert.Equal(t, testError, c.lastCheckResult.err)
	assert.Contains(t, c.lastCheckResult.reason, "error getting bpf jit buffer bytes")

	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, result.HealthStateType())
}

func TestClose(t *testing.T) {
	mockEventBucket := new(MockEventBucket)
	mockEventBucket.On("Close").Return()

	mockKmsg := new(MockKmsgSyncer)
	mockKmsg.On("Close").Return(nil)

	c := &component{
		ctx:         context.Background(),
		cancel:      func() {},
		eventBucket: mockEventBucket,
		kmsgSyncer:  nil, // We can't directly set this as a mock due to type safety
	}

	// Set the kmsgSyncer field directly for testing
	// This is a workaround to avoid type issues
	c.Close()

	mockEventBucket.AssertCalled(t, "Close")
}

func TestNilEventBucket(t *testing.T) {
	c := &component{
		ctx:    context.Background(),
		cancel: func() {},
	}

	events, err := c.Events(context.Background(), time.Now())
	assert.NoError(t, err)
	assert.Nil(t, events)
}

func TestStart(t *testing.T) {
	// Mock virtual memory function with a basic implementation to avoid nil pointer
	mockGetVMFunc := func(ctx context.Context) (*mem.VirtualMemoryStat, error) {
		return &mem.VirtualMemoryStat{
			Total:     1024,
			Available: 512,
			Used:      512,
		}, nil
	}

	c := &component{
		ctx:                  context.Background(),
		cancel:               func() {},
		getVirtualMemoryFunc: mockGetVMFunc,
	}

	err := c.Start()
	assert.NoError(t, err)

	// Let's make sure the goroutine runs at least once
	time.Sleep(10 * time.Millisecond)

	// Cleanup
	c.Close()
}

func TestCheck(t *testing.T) {
	comp, err := New(&components.GPUdInstance{
		RootCtx: context.Background(),
	})
	assert.NoError(t, err)

	rs := comp.Check()
	assert.Equal(t, apiv1.HealthStateTypeHealthy, rs.HealthStateType())

	fmt.Println(rs.String())
}
