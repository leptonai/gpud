package cpu

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/load"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/common"
	"github.com/leptonai/gpud/pkg/eventstore"
)

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

func (m *MockEventBucket) Insert(ctx context.Context, event components.Event) error {
	args := m.Called(ctx, event)
	return args.Error(0)
}

func (m *MockEventBucket) Find(ctx context.Context, event components.Event) (*components.Event, error) {
	args := m.Called(ctx, event)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*components.Event), args.Error(1)
}

func (m *MockEventBucket) Get(ctx context.Context, since time.Time) ([]components.Event, error) {
	args := m.Called(ctx, since)
	return args.Get(0).([]components.Event), args.Error(1)
}

func (m *MockEventBucket) Latest(ctx context.Context) (*components.Event, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*components.Event), args.Error(1)
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

func TestDataGetStatesNil(t *testing.T) {
	// Test with nil data
	var d *Data
	states, err := d.getStates()
	assert.NoError(t, err)
	assert.Len(t, states, 1)
	assert.Equal(t, Name, states[0].Name)
	assert.Equal(t, components.StateHealthy, states[0].Health)
	assert.True(t, states[0].Healthy)
	assert.Equal(t, "no data yet", states[0].Reason)
}

func TestDataGetStatesWithError(t *testing.T) {
	testError := errors.New("CPU usage retrieval error")
	d := &Data{
		ts:      time.Now(),
		err:     testError,
		healthy: false,
		reason:  "error calculating CPU usage",
	}

	states, err := d.getStates()
	assert.NoError(t, err)
	assert.Len(t, states, 1)
	assert.Equal(t, components.StateUnhealthy, states[0].Health)
	assert.False(t, states[0].Healthy)
	assert.Equal(t, testError.Error(), states[0].Error)
	assert.Equal(t, d.reason, states[0].Reason)
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
	assert.Equal(t, components.StateHealthy, states[0].Health)
	assert.True(t, states[0].Healthy)
	assert.Equal(t, "no data yet", states[0].Reason)

	// Test with data
	testData := &Data{
		Info: Info{
			Arch:      "x86_64",
			CPU:       "0",
			Family:    "6",
			Model:     "60",
			ModelName: "Intel(R) Core(TM) i7-4710MQ CPU @ 2.50GHz",
		},
		Cores: Cores{
			Logical: 8,
		},
		Usage: Usage{
			UsedPercent:  "25.50",
			LoadAvg1Min:  "1.50",
			LoadAvg5Min:  "1.25",
			LoadAvg15Min: "1.10",
			usedPercent:  25.5,
		},
		ts:      time.Now(),
		healthy: true,
		reason:  "arch: x86_64, cpu: 0, family: 6, model: 60, model_name: Intel(R) Core(TM) i7-4710MQ CPU @ 2.50GHz",
	}
	c.lastData = testData

	states, err = c.States(context.Background())
	assert.NoError(t, err)
	assert.Len(t, states, 1)
	assert.Equal(t, Name, states[0].Name)
	assert.Equal(t, components.StateHealthy, states[0].Health)
	assert.True(t, states[0].Healthy)
	assert.Equal(t, testData.reason, states[0].Reason)
}

func TestComponentEvents(t *testing.T) {
	// Setup
	mockEventBucket := new(MockEventBucket)
	testTime := metav1.Now()
	testEvents := []components.Event{
		{
			Time:    testTime,
			Name:    Name,
			Type:    common.EventType("test"),
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

func TestComponentCheckOnceSuccess(t *testing.T) {
	// Setup mocks
	mockEventBucket := new(MockEventBucket)
	mockKmsgSyncer := new(MockKmsgSyncer)
	mockKmsgSyncer.On("Close").Return(nil)

	// Mock CPU time stat
	mockPrevTimeStat := &cpu.TimesStat{
		CPU:       "cpu-total",
		User:      1000,
		System:    500,
		Idle:      8000,
		Nice:      100,
		Iowait:    200,
		Irq:       50,
		Softirq:   25,
		Steal:     10,
		Guest:     5,
		GuestNice: 2,
	}

	mockCurTimeStat := cpu.TimesStat{
		CPU:       "cpu-total",
		User:      1100,
		System:    550,
		Idle:      8200,
		Nice:      110,
		Iowait:    210,
		Irq:       55,
		Softirq:   30,
		Steal:     12,
		Guest:     6,
		GuestNice: 3,
	}

	// Mock functions
	mockGetTimeStatFunc := func(ctx context.Context) (cpu.TimesStat, error) {
		return mockCurTimeStat, nil
	}

	mockGetUsedPctFunc := func(ctx context.Context) (float64, error) {
		return 25.5, nil
	}

	mockLoadAvgStat := &load.AvgStat{
		Load1:  1.5,
		Load5:  1.25,
		Load15: 1.1,
	}

	mockGetLoadAvgStatFunc := func(ctx context.Context) (*load.AvgStat, error) {
		return mockLoadAvgStat, nil
	}

	// Setup component with mocks
	c := &component{
		ctx:    context.Background(),
		cancel: func() {},

		getTimeStatFunc:    mockGetTimeStatFunc,
		getUsedPctFunc:     mockGetUsedPctFunc,
		getLoadAvgStatFunc: mockGetLoadAvgStatFunc,

		setPrevTimeStatFunc: setPrevTimeStat,
		getPrevTimeStatFunc: func() *cpu.TimesStat { return mockPrevTimeStat },

		eventBucket: mockEventBucket,

		info: Info{
			Arch:      "x86_64",
			CPU:       "0",
			Family:    "6",
			Model:     "60",
			ModelName: "Intel(R) Core(TM) i7-4710MQ CPU @ 2.50GHz",
		},
		cores: Cores{
			Logical: 8,
		},
	}

	// Test
	c.CheckOnce()

	// Verify
	assert.NotNil(t, c.lastData)
	assert.Contains(t, c.lastData.Usage.UsedPercent, "45.")
	assert.Equal(t, "1.50", c.lastData.Usage.LoadAvg1Min)
	assert.Equal(t, "1.25", c.lastData.Usage.LoadAvg5Min)
	assert.Equal(t, "1.10", c.lastData.Usage.LoadAvg15Min)
	assert.True(t, c.lastData.healthy)
	assert.Equal(t, "arch: x86_64, cpu: 0, family: 6, model: 60, model_name: Intel(R) Core(TM) i7-4710MQ CPU @ 2.50GHz", c.lastData.reason)
}

func TestComponentCheckOnceWithCPUUsageError(t *testing.T) {
	// Setup mocks
	mockEventBucket := new(MockEventBucket)
	mockKmsgSyncer := new(MockKmsgSyncer)
	mockKmsgSyncer.On("Close").Return(nil)

	testError := errors.New("CPU usage calculation error")

	// Mock functions
	mockGetTimeStatFunc := func(ctx context.Context) (cpu.TimesStat, error) {
		return cpu.TimesStat{}, testError
	}

	// Setup component with mocks
	c := &component{
		ctx:    context.Background(),
		cancel: func() {},

		getTimeStatFunc:     mockGetTimeStatFunc,
		getPrevTimeStatFunc: func() *cpu.TimesStat { return nil },

		eventBucket: mockEventBucket,

		info: Info{
			Arch:      "x86_64",
			CPU:       "0",
			Family:    "6",
			Model:     "60",
			ModelName: "Intel(R) Core(TM) i7-4710MQ CPU @ 2.50GHz",
		},
		cores: Cores{
			Logical: 8,
		},
	}

	// Test
	c.CheckOnce()

	// Verify
	assert.NotNil(t, c.lastData)
	assert.False(t, c.lastData.healthy)
	assert.Equal(t, testError, c.lastData.err)
	assert.Contains(t, c.lastData.reason, "error calculating CPU usage")
}

func TestComponentCheckOnceWithLoadAvgError(t *testing.T) {
	// Setup mocks
	mockEventBucket := new(MockEventBucket)
	mockKmsgSyncer := new(MockKmsgSyncer)
	mockKmsgSyncer.On("Close").Return(nil)

	mockPrevTimeStat := &cpu.TimesStat{
		CPU:       "cpu-total",
		User:      1000,
		System:    500,
		Idle:      8000,
		Nice:      100,
		Iowait:    200,
		Irq:       50,
		Softirq:   25,
		Steal:     10,
		Guest:     5,
		GuestNice: 2,
	}

	mockCurTimeStat := cpu.TimesStat{
		CPU:       "cpu-total",
		User:      1100,
		System:    550,
		Idle:      8200,
		Nice:      110,
		Iowait:    210,
		Irq:       55,
		Softirq:   30,
		Steal:     12,
		Guest:     6,
		GuestNice: 3,
	}

	testError := errors.New("load average calculation error")

	// Mock functions
	mockGetTimeStatFunc := func(ctx context.Context) (cpu.TimesStat, error) {
		return mockCurTimeStat, nil
	}

	mockGetUsedPctFunc := func(ctx context.Context) (float64, error) {
		return 25.5, nil
	}

	mockGetLoadAvgStatFunc := func(ctx context.Context) (*load.AvgStat, error) {
		return nil, testError
	}

	// Setup component with mocks
	c := &component{
		ctx:    context.Background(),
		cancel: func() {},

		getTimeStatFunc:    mockGetTimeStatFunc,
		getUsedPctFunc:     mockGetUsedPctFunc,
		getLoadAvgStatFunc: mockGetLoadAvgStatFunc,

		setPrevTimeStatFunc: setPrevTimeStat,
		getPrevTimeStatFunc: func() *cpu.TimesStat { return mockPrevTimeStat },

		eventBucket: mockEventBucket,

		info: Info{
			Arch:      "x86_64",
			CPU:       "0",
			Family:    "6",
			Model:     "60",
			ModelName: "Intel(R) Core(TM) i7-4710MQ CPU @ 2.50GHz",
		},
		cores: Cores{
			Logical: 8,
		},
	}

	// Test
	c.CheckOnce()

	// Verify
	assert.NotNil(t, c.lastData)
	assert.False(t, c.lastData.healthy)
	assert.Equal(t, testError, c.lastData.err)
	assert.Contains(t, c.lastData.reason, "error calculating load average")
}
