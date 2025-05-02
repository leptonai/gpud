package peermem

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/NVIDIA/go-nvlib/pkg/nvlib/device"
	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	nvidianvml "github.com/leptonai/gpud/pkg/nvidia-query/nvml"
	nvmllib "github.com/leptonai/gpud/pkg/nvidia-query/nvml/lib"
	querypeermem "github.com/leptonai/gpud/pkg/nvidia-query/peermem"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// mockNVMLInstance is a mock implementation of nvidianvml.Instance
type mockNVMLInstance struct {
	exists bool
}

func (m *mockNVMLInstance) NVMLExists() bool {
	return m.exists
}

func (m *mockNVMLInstance) Library() nvmllib.Library {
	return nil
}

func (m *mockNVMLInstance) Devices() map[string]device.Device {
	return nil
}

func (m *mockNVMLInstance) ProductName() string {
	return "test"
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

// mockEventBucket implements eventstore.Bucket
type mockEventBucket struct {
	events map[time.Time]apiv1.Events
	closed bool
	name   string
}

func newMockEventBucket() *mockEventBucket {
	return &mockEventBucket{
		events: make(map[time.Time]apiv1.Events),
		name:   "mock-bucket",
	}
}

func (m *mockEventBucket) Name() string {
	return m.name
}

func (m *mockEventBucket) Insert(ctx context.Context, ev apiv1.Event) error {
	if m.closed {
		return errors.New("bucket is closed")
	}

	now := time.Now()
	if m.events[now] == nil {
		m.events[now] = make(apiv1.Events, 0)
	}
	m.events[now] = append(m.events[now], ev)
	return nil
}

func (m *mockEventBucket) Find(ctx context.Context, ev apiv1.Event) (*apiv1.Event, error) {
	if m.closed {
		return nil, errors.New("bucket is closed")
	}

	for _, events := range m.events {
		for _, event := range events {
			if event.Name == ev.Name && event.Component == ev.Component {
				return &event, nil
			}
		}
	}
	return nil, nil
}

func (m *mockEventBucket) Get(ctx context.Context, since time.Time) (apiv1.Events, error) {
	if m.closed {
		return nil, errors.New("bucket is closed")
	}

	var result apiv1.Events
	for t, events := range m.events {
		if t.After(since) || t.Equal(since) {
			result = append(result, events...)
		}
	}
	return result, nil
}

func (m *mockEventBucket) Latest(ctx context.Context) (*apiv1.Event, error) {
	if m.closed {
		return nil, errors.New("bucket is closed")
	}

	var latest *apiv1.Event
	var latestTime time.Time

	for t, events := range m.events {
		if latest == nil || t.After(latestTime) {
			if len(events) > 0 {
				latestEvent := events[0]
				latest = &latestEvent
				latestTime = t
			}
		}
	}

	return latest, nil
}

func (m *mockEventBucket) Purge(ctx context.Context, beforeTimestamp int64) (int, error) {
	if m.closed {
		return 0, errors.New("bucket is closed")
	}

	beforeTime := time.Unix(beforeTimestamp, 0)
	count := 0

	for t, events := range m.events {
		if t.Before(beforeTime) {
			count += len(events)
			delete(m.events, t)
		}
	}

	return count, nil
}

func (m *mockEventBucket) Close() {
	m.closed = true
}

// mockPeermemChecker mocks the CheckLsmodPeermemModule function
type mockPeermemChecker struct {
	output *querypeermem.LsmodPeermemModuleOutput
	err    error
}

func (m *mockPeermemChecker) Check(ctx context.Context) (*querypeermem.LsmodPeermemModuleOutput, error) {
	return m.output, m.err
}

func TestComponentName(t *testing.T) {
	c := &component{}
	assert.Equal(t, Name, c.Name())
}

func TestNewComponent(t *testing.T) {
	// Test creating a component with nil NVML instance
	gpudInstance := &components.GPUdInstance{
		RootCtx:          context.Background(),
		LoadNVMLInstance: nil,
	}

	comp, err := New(gpudInstance)
	assert.NoError(t, err)
	assert.NotNil(t, comp)

	// Test creating a component with NVML instance
	gpudInstance = &components.GPUdInstance{
		RootCtx:          context.Background(),
		LoadNVMLInstance: &mockNVMLInstance{exists: true},
	}

	comp, err = New(gpudInstance)
	assert.NoError(t, err)
	assert.NotNil(t, comp)
}

func TestCheckWithNoNVML(t *testing.T) {
	c := &component{
		ctx:    context.Background(),
		cancel: func() {},
	}

	result := c.Check()
	assert.Equal(t, apiv1.HealthStateTypeHealthy, result.HealthStateType())
	assert.Contains(t, result.Summary(), "NVIDIA NVML instance is nil")
}

func TestCheckWithNVML(t *testing.T) {
	// Test with successful peermem check
	mockChecker := &mockPeermemChecker{
		output: &querypeermem.LsmodPeermemModuleOutput{
			Raw:                      "ib_core 123456 1 nvidia_peermem",
			IbcoreUsingPeermemModule: true,
		},
		err: nil,
	}

	c := &component{
		ctx:                         context.Background(),
		cancel:                      func() {},
		loadNVML:                    &mockNVMLInstance{exists: true},
		checkLsmodPeermemModuleFunc: mockChecker.Check,
	}

	result := c.Check()
	assert.Equal(t, apiv1.HealthStateTypeHealthy, result.HealthStateType())
	assert.Contains(t, result.Summary(), "ibcore successfully loaded peermem module")

	// Test with unsuccessful peermem check (module not loaded)
	mockChecker = &mockPeermemChecker{
		output: &querypeermem.LsmodPeermemModuleOutput{
			Raw:                      "ib_core 123456 1",
			IbcoreUsingPeermemModule: false,
		},
		err: nil,
	}

	c.checkLsmodPeermemModuleFunc = mockChecker.Check

	result = c.Check()
	assert.Equal(t, apiv1.HealthStateTypeHealthy, result.HealthStateType())
	assert.Contains(t, result.Summary(), "ibcore is not using peermem module")

	// Test with error during peermem check
	mockChecker = &mockPeermemChecker{
		output: nil,
		err:    errors.New("command failed"),
	}

	c.checkLsmodPeermemModuleFunc = mockChecker.Check

	result = c.Check()
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, result.HealthStateType())
	assert.Contains(t, result.Summary(), "error checking peermem")
}

func TestLastHealthStates(t *testing.T) {
	c := &component{
		ctx:    context.Background(),
		cancel: func() {},
		lastMu: sync.RWMutex{},
	}

	// When lastCheckResult is nil
	states := c.LastHealthStates()
	assert.Len(t, states, 1)
	assert.Equal(t, Name, states[0].Name)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, states[0].Health)
	assert.Equal(t, "no data yet", states[0].Reason)

	// When lastCheckResult is healthy
	c.lastCheckResult = &checkResult{
		health: apiv1.HealthStateTypeHealthy,
		reason: "all good",
		PeerMemModuleOutput: &querypeermem.LsmodPeermemModuleOutput{
			IbcoreUsingPeermemModule: true,
		},
	}

	states = c.LastHealthStates()
	assert.Len(t, states, 1)
	assert.Equal(t, Name, states[0].Name)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, states[0].Health)
	assert.Equal(t, "all good", states[0].Reason)

	// When lastCheckResult has error
	c.lastCheckResult = &checkResult{
		health: apiv1.HealthStateTypeUnhealthy,
		reason: "error occurred",
		err:    errors.New("something went wrong"),
	}

	states = c.LastHealthStates()
	assert.Len(t, states, 1)
	assert.Equal(t, Name, states[0].Name)
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, states[0].Health)
	assert.Equal(t, "error occurred", states[0].Reason)
	assert.Equal(t, "something went wrong", states[0].Error)
}

func TestEvents(t *testing.T) {
	mockBucket := newMockEventBucket()

	c := &component{
		ctx:         context.Background(),
		cancel:      func() {},
		eventBucket: mockBucket,
	}

	// Test when eventBucket is nil
	c.eventBucket = nil
	events, err := c.Events(context.Background(), time.Now().Add(-time.Hour))
	assert.NoError(t, err)
	assert.Nil(t, events)

	// Test when eventBucket is not nil
	c.eventBucket = mockBucket

	// Add some events
	since := time.Now().Add(-time.Hour)
	mockEvent := apiv1.Event{
		Name:    "test-event",
		Time:    metav1.Time{Time: time.Now()},
		Message: "test message",
	}
	_ = mockBucket.Insert(context.Background(), mockEvent)

	events, err = c.Events(context.Background(), since)
	assert.NoError(t, err)
	assert.Len(t, events, 1)
	assert.Equal(t, "test-event", events[0].Name)
}

func TestDataMethods(t *testing.T) {
	// Test with nil data
	var cr *checkResult
	assert.Equal(t, "", cr.String())
	assert.Equal(t, "", cr.Summary())
	assert.Equal(t, apiv1.HealthStateType(""), cr.HealthStateType())
	assert.Equal(t, "", cr.getError())

	// Test without peer mem module output
	cr = &checkResult{
		health: apiv1.HealthStateTypeHealthy,
		reason: "test reason",
	}
	assert.Equal(t, "no data", cr.String())
	assert.Equal(t, "test reason", cr.Summary())
	assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.HealthStateType())

	// Test with peer mem module output (module loaded)
	cr = &checkResult{
		PeerMemModuleOutput: &querypeermem.LsmodPeermemModuleOutput{
			IbcoreUsingPeermemModule: true,
		},
		health: apiv1.HealthStateTypeHealthy,
		reason: "test reason",
	}
	assert.Equal(t, "ibcore using peermem module: true", cr.String())
	assert.Equal(t, "test reason", cr.Summary())
	assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.HealthStateType())

	// Test with peer mem module output (module not loaded)
	cr = &checkResult{
		PeerMemModuleOutput: &querypeermem.LsmodPeermemModuleOutput{
			IbcoreUsingPeermemModule: false,
		},
		health: apiv1.HealthStateTypeHealthy,
		reason: "test reason",
	}
	assert.Equal(t, "ibcore using peermem module: false", cr.String())

	// Test with error
	cr = &checkResult{
		err:    errors.New("test error"),
		health: apiv1.HealthStateTypeUnhealthy,
		reason: "error reason",
	}
	assert.Equal(t, "test error", cr.getError())
}

func TestClose(t *testing.T) {
	mockBucket := newMockEventBucket()

	c := &component{
		ctx:         context.Background(),
		cancel:      func() {},
		eventBucket: mockBucket,
	}

	err := c.Close()
	assert.NoError(t, err)
	assert.True(t, mockBucket.closed)
}

func TestStart(t *testing.T) {
	c := &component{
		ctx:    context.Background(),
		cancel: func() {},
	}

	err := c.Start()
	assert.NoError(t, err)

	// This is just to ensure the goroutine has a chance to start
	time.Sleep(50 * time.Millisecond)
}
