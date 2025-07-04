package cpu

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/load"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/pkg/eventstore"
)

// mockEventBucket implements a mock for eventstore.Bucket
type mockEventBucket struct {
	mock.Mock
}

func (m *mockEventBucket) Name() string {
	args := m.Called()
	return args.String(0)
}

func (m *mockEventBucket) Insert(ctx context.Context, event eventstore.Event) error {
	args := m.Called(ctx, event)
	return args.Error(0)
}

func (m *mockEventBucket) Find(ctx context.Context, event eventstore.Event) (*eventstore.Event, error) {
	args := m.Called(ctx, event)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*eventstore.Event), args.Error(1)
}

func (m *mockEventBucket) Get(ctx context.Context, since time.Time, opts ...eventstore.OpOption) (eventstore.Events, error) {
	args := m.Called(ctx, since)
	return args.Get(0).(eventstore.Events), args.Error(1)
}

func (m *mockEventBucket) Latest(ctx context.Context) (*eventstore.Event, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*eventstore.Event), args.Error(1)
}

func (m *mockEventBucket) Purge(ctx context.Context, beforeTimestamp int64) (int, error) {
	args := m.Called(ctx, beforeTimestamp)
	return args.Int(0), args.Error(1)
}

func (m *mockEventBucket) Close() {
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

// MockGPUdInstance implements a mock for components.GPUdInstance
type MockGPUdInstance struct {
	RootCtx    context.Context
	EventStore eventstore.Store
}

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
	testError := errors.New("CPU usage retrieval error")
	cr := &checkResult{
		ts:     time.Now(),
		err:    testError,
		health: apiv1.HealthStateTypeUnhealthy,
		reason: "error calculating CPU usage",
	}

	states := cr.HealthStates()
	assert.Len(t, states, 1)
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, states[0].Health)
	assert.Equal(t, testError.Error(), states[0].Error)
	assert.Equal(t, cr.reason, states[0].Reason)
}

func TestComponentName(t *testing.T) {
	mockEventBucket := new(mockEventBucket)
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
	mockEventBucket := new(mockEventBucket)

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
		ts:     time.Now(),
		health: apiv1.HealthStateTypeHealthy,
		reason: "arch: x86_64, cpu: 0, family: 6, model: 60, model_name: Intel(R) Core(TM) i7-4710MQ CPU @ 2.50GHz",
	}
	c.lastMu.Lock()
	c.lastCheckResult = testData
	c.lastMu.Unlock()

	states = c.LastHealthStates()
	assert.Len(t, states, 1)
	assert.Equal(t, Name, states[0].Name)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, states[0].Health)
	assert.Equal(t, testData.reason, states[0].Reason)
}

func TestComponentEvents(t *testing.T) {
	// Setup
	mockEventBucket := new(mockEventBucket)
	testTime := time.Now().UTC()
	// Use eventstore.Event for mock data
	testStoreEvent := eventstore.Event{
		Component: Name,
		Time:      testTime,
		Name:      Name,                        // Event name, often same as component
		Type:      string(apiv1.EventTypeInfo), // Store type as string
		Message:   "Test event",
		ExtraInfo: nil,
	}
	testStoreEvents := eventstore.Events{testStoreEvent}

	mockEventBucket.On("Get", mock.Anything, mock.Anything).Return(testStoreEvents, nil)

	c := &component{
		ctx:         context.Background(),
		cancel:      func() {},
		eventBucket: mockEventBucket,
	}

	// Test
	since := time.Now().Add(-time.Hour)
	// Call Events, which gets eventstore.Events and converts them
	apiEvents, err := c.Events(context.Background(), since)

	assert.NoError(t, err)
	// Assert based on the converted apiv1.Events
	assert.Len(t, apiEvents, 1)
	assert.Equal(t, Name, apiEvents[0].Component)
	assert.Equal(t, "Test event", apiEvents[0].Message)
	assert.Equal(t, metav1.Time{Time: testTime}, apiEvents[0].Time)
	assert.Equal(t, apiv1.EventTypeInfo, apiEvents[0].Type)
	assert.Equal(t, Name, apiEvents[0].Name)
	mockEventBucket.AssertCalled(t, "Get", mock.Anything, since)
}

func TestComponentCheckOnceSuccess(t *testing.T) {
	// Setup mocks
	mockEventBucket := new(mockEventBucket)
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
	}

	// Test
	_ = c.Check()

	// Verify
	assert.NotNil(t, c.lastCheckResult)
	assert.Contains(t, c.lastCheckResult.Usage.UsedPercent, "45.")
	assert.Equal(t, "1.50", c.lastCheckResult.Usage.LoadAvg1Min)
	assert.Equal(t, "1.25", c.lastCheckResult.Usage.LoadAvg5Min)
	assert.Equal(t, "1.10", c.lastCheckResult.Usage.LoadAvg15Min)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, c.lastCheckResult.health)
	assert.Contains(t, c.lastCheckResult.reason, "ok")
}

func TestComponentCheckOnceWithCPUUsageError(t *testing.T) {
	// Setup mocks
	mockEventBucket := new(mockEventBucket)
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
	}

	// Test
	_ = c.Check()

	// Verify
	assert.NotNil(t, c.lastCheckResult)
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, c.lastCheckResult.health)
	assert.Equal(t, testError, c.lastCheckResult.err)
	assert.Contains(t, c.lastCheckResult.reason, "error calculating CPU usage")
}

func TestComponentCheckOnceWithLoadAvgError(t *testing.T) {
	// Setup mocks
	mockEventBucket := new(mockEventBucket)
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
	}

	// Test
	_ = c.Check()

	// Verify
	assert.NotNil(t, c.lastCheckResult)
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, c.lastCheckResult.health)
	assert.Equal(t, testError, c.lastCheckResult.err)
	assert.Contains(t, c.lastCheckResult.reason, "error calculating load average")
}

func TestComponentClose(t *testing.T) {
	// Setup mocks
	mockEventBucket := new(mockEventBucket)

	mockEventBucket.On("Close").Return()

	// Create component with mocks
	ctx, cancel := context.WithCancel(context.Background())
	c := &component{
		ctx:    ctx,
		cancel: cancel,

		eventBucket: mockEventBucket,
	}

	// Test Close method
	err := c.Close()
	assert.NoError(t, err)

	// Verify mocks were called
	mockEventBucket.AssertCalled(t, "Close")

	// Verify context was canceled
	select {
	case <-ctx.Done():
		// Good, context was canceled
	default:
		t.Error("Context was not canceled during Close()")
	}
}

func TestComponentCheckOnceWithGetUsedPctError(t *testing.T) {
	// Setup mocks
	mockEventBucket := new(mockEventBucket)

	testError := errors.New("CPU percent calculation error")

	// Mock functions
	mockGetTimeStatFunc := func(ctx context.Context) (cpu.TimesStat, error) {
		return cpu.TimesStat{}, nil
	}

	mockGetUsedPctFunc := func(ctx context.Context) (float64, error) {
		return 0, testError
	}

	// Setup component with mocks
	c := &component{
		ctx:    context.Background(),
		cancel: func() {},

		getTimeStatFunc:     mockGetTimeStatFunc,
		getUsedPctFunc:      mockGetUsedPctFunc,
		getPrevTimeStatFunc: func() *cpu.TimesStat { return nil },

		eventBucket: mockEventBucket,
	}

	// Test
	_ = c.Check()

	// Verify
	assert.NotNil(t, c.lastCheckResult)
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, c.lastCheckResult.health)
	assert.Equal(t, testError, c.lastCheckResult.err)
	assert.Contains(t, c.lastCheckResult.reason, "error calculating CPU usage")
}

func TestComponentStart(t *testing.T) {
	// Create component
	ctx, cancel := context.WithCancel(context.Background())
	c := &component{
		ctx:    ctx,
		cancel: cancel,

		getTimeStatFunc: func(ctx context.Context) (cpu.TimesStat, error) {
			return cpu.TimesStat{}, nil
		},
		getUsedPctFunc: func(ctx context.Context) (float64, error) {
			return 0, nil
		},
		getLoadAvgStatFunc: func(ctx context.Context) (*load.AvgStat, error) {
			return &load.AvgStat{}, nil
		},

		setPrevTimeStatFunc: func(cpu.TimesStat) {},
		getPrevTimeStatFunc: func() *cpu.TimesStat { return nil },
	}

	// Test Start method
	err := c.Start()
	assert.NoError(t, err)

	// Sleep briefly to allow goroutine to start
	time.Sleep(10 * time.Millisecond)

	// Clean up
	cancel()
}

func TestComponentGetError(t *testing.T) {
	// Test nil data
	var cr *checkResult
	err := cr.getError()
	assert.Equal(t, "", err)

	// Test with nil error
	cr = &checkResult{}
	err = cr.getError()
	assert.Equal(t, "", err)

	// Test with error
	testError := errors.New("test error")
	cr = &checkResult{err: testError}
	err = cr.getError()
	assert.Equal(t, testError.Error(), err)
}

func TestComponentDataExtraInfo(t *testing.T) {
	// Create test data
	testData := &checkResult{
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
		ts:     time.Now(),
		health: apiv1.HealthStateTypeHealthy,
		reason: "test reason",
	}

	// Get states
	states := testData.HealthStates()

	// Verify
	assert.Len(t, states, 1)
	assert.NotNil(t, states[0].ExtraInfo)
	assert.Contains(t, states[0].ExtraInfo, "data")

	// Verify JSON contains expected data
	jsonData := states[0].ExtraInfo["data"]
	assert.Contains(t, jsonData, testData.Usage.UsedPercent)
	assert.Contains(t, jsonData, testData.Usage.LoadAvg1Min)
}

func TestComponentEventsError(t *testing.T) {
	// Setup mock
	mockEventBucket := new(mockEventBucket)
	testError := errors.New("events retrieval error")
	mockEventBucket.On("Get", mock.Anything, mock.Anything).Return(eventstore.Events(nil), testError)

	c := &component{
		ctx:         context.Background(),
		cancel:      func() {},
		eventBucket: mockEventBucket,
	}

	// Test with error
	events, err := c.Events(context.Background(), time.Now().Add(-time.Hour))

	// Verify
	assert.Error(t, err)
	assert.Equal(t, testError, err)
	assert.Empty(t, events)
}

func TestCalculateCPUUsageEdgeCases(t *testing.T) {
	// Test edge case 1: Zero idle time
	t.Run("zero idle time", func(t *testing.T) {
		prevStat := &cpu.TimesStat{
			User:   100,
			System: 50,
			Idle:   0, // Zero idle time
		}
		curStat := cpu.TimesStat{
			User:   150,
			System: 75,
			Idle:   0, // Still zero idle time
		}

		result := calculateCPUUsage(prevStat, curStat, 0)
		assert.GreaterOrEqual(t, result, 0.0)
		assert.LessOrEqual(t, result, 100.0)
	})

	// Test edge case 2: Identical stats (no change)
	t.Run("identical stats", func(t *testing.T) {
		stat := cpu.TimesStat{
			User:   100,
			System: 50,
			Idle:   200,
		}

		result := calculateCPUUsage(&stat, stat, 0)
		assert.Equal(t, 0.0, result)
	})

	// Test edge case 3: Very high CPU usage
	t.Run("very high CPU usage", func(t *testing.T) {
		prevStat := &cpu.TimesStat{
			User:   100,
			System: 50,
			Idle:   10000,
		}
		curStat := cpu.TimesStat{
			User:   9000,  // Huge increase in user time
			System: 1000,  // Huge increase in system time
			Idle:   10001, // Almost no increase in idle time
		}

		result := calculateCPUUsage(prevStat, curStat, 0)
		assert.GreaterOrEqual(t, result, 0.0)
		assert.LessOrEqual(t, result, 100.0)
		assert.InDelta(t, 100.0, result, 1.0)
	})
}

func TestDataMarshalingInStates(t *testing.T) {
	// Test data with unusual values
	testData := &checkResult{
		Cores: Cores{
			Logical: 999999, // Very high core count
		},
		Usage: Usage{
			UsedPercent:  "999999.99", // Very high CPU usage
			LoadAvg1Min:  "999999.99", // Very high load average
			LoadAvg5Min:  "999999.99",
			LoadAvg15Min: "999999.99",
			usedPercent:  999999.99,
		},
		ts:     time.Now(),
		health: apiv1.HealthStateTypeHealthy,
		reason: "test reason with special characters: +!@#$%^&*()_",
	}

	// Get states
	states := testData.HealthStates()

	// Verify no errors in marshaling
	assert.Len(t, states, 1)
	assert.NotNil(t, states[0].ExtraInfo)

	// JSON data should contain all the unusual values
	jsonData := states[0].ExtraInfo["data"]
	assert.Contains(t, jsonData, "999999.99")
	assert.Contains(t, jsonData, "999999") // Core count
}

func TestCheckHealthState(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Create a properly initialized test component to check health state
	c := &component{
		ctx:    ctx,
		cancel: cancel,
		getTimeStatFunc: func(ctx context.Context) (cpu.TimesStat, error) {
			return cpu.TimesStat{
				CPU:    "cpu-total",
				User:   1000,
				System: 500,
				Idle:   8000,
			}, nil
		},
		getUsedPctFunc: func(ctx context.Context) (float64, error) {
			return 25.5, nil
		},
		getLoadAvgStatFunc: func(ctx context.Context) (*load.AvgStat, error) {
			return &load.AvgStat{
				Load1:  1.5,
				Load5:  1.25,
				Load15: 1.1,
			}, nil
		},
		setPrevTimeStatFunc: func(cpu.TimesStat) {},
		getPrevTimeStatFunc: func() *cpu.TimesStat {
			return &cpu.TimesStat{
				CPU:    "cpu-total",
				User:   900,
				System: 450,
				Idle:   7500,
			}
		},
	}

	// Use the Check method directly which returns CheckResult
	rs := c.Check()
	assert.NotNil(t, rs)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, rs.HealthStateType())

	fmt.Println(rs.String())

	b, err := json.Marshal(rs)
	assert.NoError(t, err)
	fmt.Println(string(b))
}

func TestComponentEventsNilBucket(t *testing.T) {
	c := &component{
		ctx:         context.Background(),
		cancel:      func() {},
		eventBucket: nil, // Explicitly nil
	}

	events, err := c.Events(context.Background(), time.Now())
	assert.NoError(t, err)
	assert.Nil(t, events)
}

func TestComponentCloseNilSyncer(t *testing.T) {
	// Setup mocks
	mockEventBucket := new(mockEventBucket)
	mockEventBucket.On("Close").Return()

	// Create component with mocks, kmsgSyncer is nil
	ctx, cancel := context.WithCancel(context.Background())
	c := &component{
		ctx:         ctx,
		cancel:      cancel,
		eventBucket: mockEventBucket,
		kmsgSyncer:  nil, // Explicitly nil
	}

	// Test Close method
	err := c.Close()
	assert.NoError(t, err)

	// Verify mocks were called
	mockEventBucket.AssertCalled(t, "Close")

	// Verify context was canceled
	select {
	case <-ctx.Done():
		// Good, context was canceled
	default:
		t.Error("Context was not canceled during Close()")
	}
}

func TestCheckResultMethodsWithNilReceiver(t *testing.T) {
	var cr *checkResult

	assert.NotPanics(t, func() {
		str := cr.String()
		assert.Equal(t, "", str, "String() on nil receiver")
	})

	assert.NotPanics(t, func() {
		summary := cr.Summary()
		assert.Equal(t, "", summary, "Summary() on nil receiver")
	})

	assert.NotPanics(t, func() {
		health := cr.HealthStateType()
		assert.Equal(t, apiv1.HealthStateType(""), health, "HealthStateType() on nil receiver")
	})

	assert.NotPanics(t, func() {
		errStr := cr.getError()
		assert.Equal(t, "", errStr, "getError() on nil receiver")
	})
}

func TestCheckResultSummaryNotNil(t *testing.T) {
	cr := &checkResult{
		reason: "test reason",
	}
	summary := cr.Summary()
	assert.Equal(t, "test reason", summary)
}
