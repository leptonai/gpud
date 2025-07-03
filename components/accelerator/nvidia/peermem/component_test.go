package peermem

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/eventstore"
	nvidianvml "github.com/leptonai/gpud/pkg/nvidia-query/nvml"
	"github.com/leptonai/gpud/pkg/nvidia-query/nvml/device"
	nvmllib "github.com/leptonai/gpud/pkg/nvidia-query/nvml/lib"
	querypeermem "github.com/leptonai/gpud/pkg/nvidia-query/peermem"
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
	events []eventstore.Event
	mu     sync.RWMutex
	closed bool
	name   string
	getErr error
}

func newMockEventBucket() *mockEventBucket {
	return &mockEventBucket{
		events: make([]eventstore.Event, 0),
		name:   "mock-bucket",
	}
}

func (m *mockEventBucket) Name() string {
	return m.name
}

func (m *mockEventBucket) Insert(ctx context.Context, ev eventstore.Event) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return errors.New("bucket is closed")
	}
	m.events = append(m.events, ev)
	return nil
}

func (m *mockEventBucket) Find(ctx context.Context, ev eventstore.Event) (*eventstore.Event, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.closed {
		return nil, errors.New("bucket is closed")
	}

	for i := range m.events {
		if m.events[i].Name == ev.Name && m.events[i].Component == ev.Component {
			return &m.events[i], nil
		}
	}
	return nil, nil
}

func (m *mockEventBucket) Get(ctx context.Context, since time.Time) (eventstore.Events, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.closed {
		return nil, errors.New("bucket is closed")
	}

	if m.getErr != nil {
		return nil, m.getErr
	}

	var result eventstore.Events
	for _, event := range m.events {
		if !event.Time.Before(since) {
			result = append(result, event)
		}
	}
	return result, nil
}

func (m *mockEventBucket) Latest(ctx context.Context) (*eventstore.Event, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.closed {
		return nil, errors.New("bucket is closed")
	}

	if len(m.events) == 0 {
		return nil, nil
	}

	latest := m.events[0]
	for i := 1; i < len(m.events); i++ {
		if m.events[i].Time.After(latest.Time) {
			latest = m.events[i]
		}
	}

	return &latest, nil
}

func (m *mockEventBucket) Purge(ctx context.Context, beforeTimestamp int64) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return 0, errors.New("bucket is closed")
	}

	beforeTime := time.Unix(beforeTimestamp, 0)
	count := 0
	var remainingEvents []eventstore.Event

	for _, event := range m.events {
		if event.Time.Before(beforeTime) {
			count++
		} else {
			remainingEvents = append(remainingEvents, event)
		}
	}
	m.events = remainingEvents

	return count, nil
}

// Close marks the mock bucket as closed.
func (m *mockEventBucket) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
}

// Ensure mockEventBucket satisfies the interface
var _ eventstore.Bucket = (*mockEventBucket)(nil)

// mockPeermemChecker mocks the CheckLsmodPeermemModule function
type mockPeermemChecker struct {
	output *querypeermem.LsmodPeermemModuleOutput
	err    error
}

func (m *mockPeermemChecker) Check(ctx context.Context) (*querypeermem.LsmodPeermemModuleOutput, error) {
	return m.output, m.err
}

// Mock EventStore for testing New error path
type mockErrorEventStore struct{}

func (m *mockErrorEventStore) Bucket(name string, opts ...eventstore.OpOption) (eventstore.Bucket, error) {
	return nil, errors.New("failed to create bucket")
}

func TestComponentName(t *testing.T) {
	c := &component{}
	assert.Equal(t, Name, c.Name())
}

func TestNewComponent(t *testing.T) {
	// Test creating a component with nil NVML instance
	gpudInstance := &components.GPUdInstance{
		RootCtx:      context.Background(),
		NVMLInstance: nil,
	}

	comp, err := New(gpudInstance)
	assert.NoError(t, err)
	assert.NotNil(t, comp)

	// Test creating a component with NVML instance
	gpudInstance = &components.GPUdInstance{
		RootCtx:      context.Background(),
		NVMLInstance: &mockNVMLInstance{exists: true},
	}

	comp, err = New(gpudInstance)
	assert.NoError(t, err)
	assert.NotNil(t, comp)
}

func TestNewComponentError(t *testing.T) {
	// Test creating a component when event store fails to create bucket
	gpudInstance := &components.GPUdInstance{
		RootCtx:      context.Background(),
		NVMLInstance: &mockNVMLInstance{exists: true},
		EventStore:   &mockErrorEventStore{},
	}

	comp, err := New(gpudInstance)

	// When EventStore is provided, bucket creation is always attempted regardless of OS
	assert.Error(t, err)
	assert.Nil(t, comp)
	if err != nil { // Avoid panic on nil err
		assert.Contains(t, err.Error(), "failed to create bucket")
	}
}

func TestCheckWithNoNVML(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c := &component{
		ctx:    ctx,
		cancel: cancel,
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
		nvmlInstance:                &mockNVMLInstance{exists: true},
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
	tempBucket := c.eventBucket
	c.eventBucket = nil
	events, err := c.Events(context.Background(), time.Now().Add(-time.Hour))
	assert.NoError(t, err)
	assert.Nil(t, events)
	c.eventBucket = tempBucket // Restore

	// Test when eventBucket is not nil
	since := time.Now().Add(-time.Hour)
	now := time.Now()
	mockStoreEvent := eventstore.Event{
		Name:      "test-event",
		Time:      now,
		Message:   "test message",
		Component: Name,
		Type:      string(apiv1.EventTypeInfo),
	}
	_ = mockBucket.Insert(context.Background(), mockStoreEvent)

	expectedAPIEvent := apiv1.Event{
		Name:      "test-event",
		Time:      metav1.Time{Time: now},
		Message:   "test message",
		Component: Name,
		Type:      apiv1.EventTypeInfo,
	}

	events, err = c.Events(context.Background(), since)
	assert.NoError(t, err)
	assert.Len(t, events, 1)
	assert.Equal(t, expectedAPIEvent, events[0])

	// Test Get error path
	mockBucket.getErr = errors.New("failed to get events") // Simulate error
	events, err = c.Events(context.Background(), since)
	assert.Error(t, err)
	assert.Nil(t, events)
	assert.Equal(t, "failed to get events", err.Error())
	mockBucket.getErr = nil // Reset error for subsequent tests if any
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

func TestCloseWithNilBucket(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c := &component{
		ctx:         ctx,
		cancel:      cancel,
		eventBucket: nil, // Explicitly set to nil
	}

	err := c.Close()
	assert.NoError(t, err) // Close should not error if bucket is nil
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
