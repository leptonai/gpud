package memory

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/shirou/gopsutil/v4/mem"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/pkg/eventstore"
)

func TestDataGetStatesNil(t *testing.T) {
	// Test with nil data
	var d *Data
	states, err := d.getHealthStates()
	assert.NoError(t, err)
	assert.Len(t, states, 1)
	assert.Equal(t, Name, states[0].Name)
	assert.Equal(t, apiv1.StateTypeHealthy, states[0].Health)
	assert.Equal(t, "no data yet", states[0].Reason)
}

func TestDataGetStatesWithError(t *testing.T) {
	testError := errors.New("memory retrieval error")
	d := &Data{
		TotalBytes: 16,
		UsedBytes:  8,
		health:     apiv1.StateTypeUnhealthy,
		err:        testError,
	}

	states, err := d.getHealthStates()
	assert.NoError(t, err)
	assert.Len(t, states, 1)
	assert.Equal(t, apiv1.StateTypeUnhealthy, states[0].Health)
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

func (m *MockEventBucket) Get(ctx context.Context, since time.Time) (apiv1.Events, error) {
	args := m.Called(ctx, since)
	return args.Get(0).(apiv1.Events), args.Error(1)
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

func TestComponentStates(t *testing.T) {
	// Setup
	mockEventBucket := new(MockEventBucket)

	c := &component{
		ctx:         context.Background(),
		cancel:      func() {},
		eventBucket: mockEventBucket,
	}

	// Test with no data yet
	states, err := c.HealthStates(context.Background())
	assert.NoError(t, err)
	assert.Len(t, states, 1)
	assert.Equal(t, Name, states[0].Name)
	assert.Equal(t, apiv1.StateTypeHealthy, states[0].Health)
	assert.Equal(t, "no data yet", states[0].Reason)

	// Test with data
	testData := &Data{
		TotalBytes: 16,
		UsedBytes:  8,
		ts:         time.Now(),
		health:     apiv1.StateTypeHealthy,
		reason:     "using 8 bytes out of total 16 bytes",
	}
	c.lastData = testData

	states, err = c.HealthStates(context.Background())
	assert.NoError(t, err)
	assert.Len(t, states, 1)
	assert.Equal(t, Name, states[0].Name)
	assert.Equal(t, apiv1.StateTypeHealthy, states[0].Health)
	assert.Equal(t, "using 8 bytes out of total 16 bytes", states[0].Reason)
}

func TestComponentEvents(t *testing.T) {
	// Setup
	mockEventBucket := new(MockEventBucket)
	testTime := metav1.Now()
	testEvents := apiv1.Events{
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
	c.CheckOnce()

	// Verify
	assert.NotNil(t, c.lastData)
	assert.Equal(t, mockVMStat.Total, c.lastData.TotalBytes)
	assert.Equal(t, mockVMStat.Available, c.lastData.AvailableBytes)
	assert.Equal(t, mockVMStat.Used, c.lastData.UsedBytes)
	assert.Equal(t, uint64(1024*1024), c.lastData.BPFJITBufferBytes)
	assert.Equal(t, apiv1.StateTypeHealthy, c.lastData.health)
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
	c.CheckOnce()

	// Verify
	assert.NotNil(t, c.lastData)
	assert.Equal(t, apiv1.StateTypeUnhealthy, c.lastData.health)
	assert.Equal(t, testError, c.lastData.err)
	assert.Contains(t, c.lastData.reason, "failed to get virtual memory")
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
	c.CheckOnce()

	// Verify
	assert.NotNil(t, c.lastData)
	assert.Equal(t, apiv1.StateTypeUnhealthy, c.lastData.health)
	assert.Equal(t, testError, c.lastData.err)
	assert.Contains(t, c.lastData.reason, "failed to get bpf jit buffer bytes")
}

func TestCheckHealthState(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	rs, err := CheckHealthState(ctx)
	assert.NoError(t, err)
	assert.Equal(t, apiv1.StateTypeHealthy, rs.HealthState())

	fmt.Println(rs.String())

	b, err := json.Marshal(rs)
	assert.NoError(t, err)
	fmt.Println(string(b))
}
