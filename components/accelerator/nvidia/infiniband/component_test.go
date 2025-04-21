package infiniband

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/NVIDIA/go-nvlib/pkg/nvlib/device"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	nvidia_common "github.com/leptonai/gpud/pkg/config/common"
	"github.com/leptonai/gpud/pkg/kmsg"
	"github.com/leptonai/gpud/pkg/nvidia-query/infiniband"
	nvidianvml "github.com/leptonai/gpud/pkg/nvidia-query/nvml"
	nvmllib "github.com/leptonai/gpud/pkg/nvidia-query/nvml/lib"
)

func TestEvaluate(t *testing.T) {
	tests := []struct {
		name       string
		output     *infiniband.IbstatOutput
		config     infiniband.ExpectedPortStates
		wantReason string
		wantHealth apiv1.HealthStateType
	}{
		{
			name:   "thresholds not set",
			output: &infiniband.IbstatOutput{},
			config: infiniband.ExpectedPortStates{
				AtLeastPorts: 0,
				AtLeastRate:  0,
			},
			wantReason: reasonThresholdNotSetSkipped,
			wantHealth: apiv1.HealthStateTypeHealthy,
		},
		{
			name:   "only ports threshold set",
			output: &infiniband.IbstatOutput{Parsed: infiniband.IBStatCards{}},
			config: infiniband.ExpectedPortStates{
				AtLeastPorts: 2,
				AtLeastRate:  0,
			},
			wantReason: "only 0 ports (>= 0 Gb/s) are active, expect at least 2",
			wantHealth: apiv1.HealthStateTypeUnhealthy,
		},
		{
			name:   "only rate threshold set",
			output: &infiniband.IbstatOutput{Parsed: infiniband.IBStatCards{}},
			config: infiniband.ExpectedPortStates{
				AtLeastPorts: 0,
				AtLeastRate:  200,
			},
			wantReason: reasonNoIbIssueFound,
			wantHealth: apiv1.HealthStateTypeHealthy,
		},
		{
			name: "healthy state with matching ports and rate",
			output: &infiniband.IbstatOutput{
				Raw: "",
				Parsed: infiniband.IBStatCards{
					{
						Name: "mlx5_0",
						Port1: infiniband.IBStatPort{
							State:         "Active",
							PhysicalState: "LinkUp",
							Rate:          200,
						},
					},
					{
						Name: "mlx5_1",
						Port1: infiniband.IBStatPort{
							State:         "Active",
							PhysicalState: "LinkUp",
							Rate:          200,
						},
					},
				},
			},
			config: infiniband.ExpectedPortStates{
				AtLeastPorts: 2,
				AtLeastRate:  200,
			},
			wantReason: reasonNoIbIssueFound,
			wantHealth: apiv1.HealthStateTypeHealthy,
		},
		{
			name: "unhealthy state - not enough ports",
			output: &infiniband.IbstatOutput{
				Raw: "",
				Parsed: infiniband.IBStatCards{
					{
						Name: "mlx5_0",
						Port1: infiniband.IBStatPort{
							State:         "Active",
							PhysicalState: "LinkUp",
							Rate:          200,
						},
					},
				},
			},
			config: infiniband.ExpectedPortStates{
				AtLeastPorts: 2,
				AtLeastRate:  200,
			},
			wantReason: "only 1 ports (>= 200 Gb/s) are active, expect at least 2",
			wantHealth: apiv1.HealthStateTypeUnhealthy,
		},
		{
			name: "unhealthy state - rate too low",
			output: &infiniband.IbstatOutput{
				Raw: "",
				Parsed: infiniband.IBStatCards{
					{
						Name: "mlx5_0",
						Port1: infiniband.IBStatPort{
							State:         "Active",
							PhysicalState: "LinkUp",
							Rate:          100,
						},
					},
					{
						Name: "mlx5_1",
						Port1: infiniband.IBStatPort{
							State:         "Active",
							PhysicalState: "LinkUp",
							Rate:          100,
						},
					},
				},
			},
			config: infiniband.ExpectedPortStates{
				AtLeastPorts: 2,
				AtLeastRate:  200,
			},
			wantReason: "only 0 ports (>= 200 Gb/s) are active, expect at least 2",
			wantHealth: apiv1.HealthStateTypeUnhealthy,
		},
		{
			name: "unhealthy state - disabled ports",
			output: &infiniband.IbstatOutput{
				Raw: "",
				Parsed: infiniband.IBStatCards{
					{
						Name: "mlx5_0",
						Port1: infiniband.IBStatPort{
							State:         "Down",
							PhysicalState: "Disabled",
							Rate:          200,
						},
					},
					{
						Name: "mlx5_1",
						Port1: infiniband.IBStatPort{
							State:         "Down",
							PhysicalState: "Disabled",
							Rate:          200,
						},
					},
				},
			},
			config: infiniband.ExpectedPortStates{
				AtLeastPorts: 2,
				AtLeastRate:  200,
			},
			wantReason: "only 0 ports (>= 200 Gb/s) are active, expect at least 2; 2 device(s) found Disabled (mlx5_0, mlx5_1)",
			wantHealth: apiv1.HealthStateTypeUnhealthy,
		},
		{
			name: "empty ibstat cards",
			output: &infiniband.IbstatOutput{
				Raw:    "",
				Parsed: infiniband.IBStatCards{},
			},
			config: infiniband.ExpectedPortStates{
				AtLeastPorts: 2,
				AtLeastRate:  200,
			},
			wantReason: "only 0 ports (>= 200 Gb/s) are active, expect at least 2",
			wantHealth: apiv1.HealthStateTypeUnhealthy,
		},
		{
			name: "inactive ports",
			output: &infiniband.IbstatOutput{
				Raw: "",
				Parsed: infiniband.IBStatCards{
					{
						Name: "mlx5_0",
						Port1: infiniband.IBStatPort{
							State:         "Inactive",
							PhysicalState: "LinkUp",
							Rate:          200,
						},
					},
					{
						Name: "mlx5_1",
						Port1: infiniband.IBStatPort{
							State:         "Inactive",
							PhysicalState: "LinkUp",
							Rate:          200,
						},
					},
				},
			},
			config: infiniband.ExpectedPortStates{
				AtLeastPorts: 2,
				AtLeastRate:  200,
			},
			wantReason: reasonNoIbIssueFound,
			wantHealth: apiv1.HealthStateTypeHealthy,
		},
		{
			name: "mixed port states",
			output: &infiniband.IbstatOutput{
				Raw: "",
				Parsed: infiniband.IBStatCards{
					{
						Name: "mlx5_0",
						Port1: infiniband.IBStatPort{
							State:         "Active",
							PhysicalState: "LinkUp",
							Rate:          200,
						},
					},
					{
						Name: "mlx5_1",
						Port1: infiniband.IBStatPort{
							State:         "Down",
							PhysicalState: "Disabled",
							Rate:          200,
						},
					},
					{
						Name: "mlx5_2",
						Port1: infiniband.IBStatPort{
							State:         "Inactive",
							PhysicalState: "LinkUp",
							Rate:          200,
						},
					},
				},
			},
			config: infiniband.ExpectedPortStates{
				AtLeastPorts: 3,
				AtLeastRate:  200,
			},
			wantReason: "only 2 ports (>= 200 Gb/s) are active, expect at least 3; 1 device(s) found Disabled (mlx5_1)",
			wantHealth: apiv1.HealthStateTypeUnhealthy,
		},
		{
			name: "mixed rate values",
			output: &infiniband.IbstatOutput{
				Raw: "",
				Parsed: infiniband.IBStatCards{
					{
						Name: "mlx5_0",
						Port1: infiniband.IBStatPort{
							State:         "Active",
							PhysicalState: "LinkUp",
							Rate:          400,
						},
					},
					{
						Name: "mlx5_1",
						Port1: infiniband.IBStatPort{
							State:         "Active",
							PhysicalState: "LinkUp",
							Rate:          200,
						},
					},
					{
						Name: "mlx5_2",
						Port1: infiniband.IBStatPort{
							State:         "Active",
							PhysicalState: "LinkUp",
							Rate:          100,
						},
					},
				},
			},
			config: infiniband.ExpectedPortStates{
				AtLeastPorts: 2,
				AtLeastRate:  300,
			},
			wantReason: "only 1 ports (>= 300 Gb/s) are active, expect at least 2",
			wantHealth: apiv1.HealthStateTypeUnhealthy,
		},
		{
			name: "zero rate value",
			output: &infiniband.IbstatOutput{
				Raw: "",
				Parsed: infiniband.IBStatCards{
					{
						Name: "mlx5_0",
						Port1: infiniband.IBStatPort{
							State:         "Active",
							PhysicalState: "LinkUp",
							Rate:          0,
						},
					},
				},
			},
			config: infiniband.ExpectedPortStates{
				AtLeastPorts: 1,
				AtLeastRate:  100,
			},
			wantReason: "only 0 ports (>= 100 Gb/s) are active, expect at least 1",
			wantHealth: apiv1.HealthStateTypeUnhealthy,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Skip nil output test to avoid panic
			if tt.output == nil {
				t.Skip("Skipping test with nil output")
				return
			}

			reason, health := evaluateIbstatOutputAgainstThresholds(tt.output, tt.config)
			assert.Equal(t, tt.wantReason, reason)
			assert.Equal(t, tt.wantHealth, health)
		})
	}
}

func TestDefaultExpectedPortStates(t *testing.T) {
	// Test default values
	defaults := GetDefaultExpectedPortStates()
	assert.Equal(t, 0, defaults.AtLeastPorts)
	assert.Equal(t, 0, defaults.AtLeastRate)

	// Test setting new values
	newStates := infiniband.ExpectedPortStates{
		AtLeastPorts: 2,
		AtLeastRate:  200,
	}
	SetDefaultExpectedPortStates(newStates)

	updated := GetDefaultExpectedPortStates()
	assert.Equal(t, newStates.AtLeastPorts, updated.AtLeastPorts)
	assert.Equal(t, newStates.AtLeastRate, updated.AtLeastRate)
}

func TestEvaluateWithTestData(t *testing.T) {
	// Read the test data file
	testDataPath := filepath.Join("testdata", "ibstat.47.0.h100.all.active.1")
	content, err := os.ReadFile(testDataPath)
	require.NoError(t, err, "Failed to read test data file")

	// Parse the test data
	cards, err := infiniband.ParseIBStat(string(content))
	require.NoError(t, err, "Failed to parse ibstat output")

	output := &infiniband.IbstatOutput{
		Raw:    string(content),
		Parsed: cards,
	}

	tests := []struct {
		name       string
		config     infiniband.ExpectedPortStates
		wantReason string
		wantHealth apiv1.HealthStateType
	}{
		{
			name: "healthy state - all H100 ports active at 400Gb/s",
			config: infiniband.ExpectedPortStates{
				AtLeastPorts: 8,   // Number of 400Gb/s ports in the test data
				AtLeastRate:  400, // Expected rate for H100 cards
			},
			wantReason: reasonNoIbIssueFound,
			wantHealth: apiv1.HealthStateTypeHealthy,
		},
		{
			name: "healthy state - mixed rate ports",
			config: infiniband.ExpectedPortStates{
				AtLeastPorts: 12,  // Total number of ports in test data
				AtLeastRate:  100, // Minimum rate that includes all ports
			},
			wantReason: reasonNoIbIssueFound,
			wantHealth: apiv1.HealthStateTypeHealthy,
		},
		{
			name: "unhealthy state - not enough high-rate ports",
			config: infiniband.ExpectedPortStates{
				AtLeastPorts: 12,  // Total number of ports
				AtLeastRate:  400, // Only 8 ports have this rate
			},
			wantReason: "only 8 ports (>= 400 Gb/s) are active, expect at least 12",
			wantHealth: apiv1.HealthStateTypeUnhealthy,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reason, health := evaluateIbstatOutputAgainstThresholds(output, tt.config)
			assert.Equal(t, tt.wantReason, reason)
			assert.Equal(t, tt.wantHealth, health)
		})
	}
}

func TestComponentCheck(t *testing.T) {
	t.Parallel()

	// Create a component with mocked functions
	cctx, ccancel := context.WithCancel(context.Background())
	defer ccancel()

	c := &component{
		ctx:                 cctx,
		cancel:              ccancel,
		getIbstatOutputFunc: mockGetIbstatOutput,
		getThresholdsFunc:   mockGetThresholds,
	}

	// Case 1: No NVML
	result := c.Check()
	data, ok := result.(*checkResult)
	require.True(t, ok)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, data.health)
	assert.Equal(t, "NVIDIA NVML instance is nil", data.reason)

	// Case 2: With NVML
	nvmlMock := &mockNVMLInstance{exists: true}
	c.nvmlInstance = nvmlMock
	result = c.Check()
	data, ok = result.(*checkResult)
	require.True(t, ok)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, data.health)
	assert.NotNil(t, data.IbstatOutput)
}

func TestComponentEvents(t *testing.T) {
	t.Parallel()

	// Setup test event bucket
	mockBucket := NewMockEventBucket()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	c := &component{
		ctx:         ctx,
		cancel:      cancel,
		eventBucket: mockBucket,
	}

	now := time.Now().UTC()

	// Insert test event
	testEvent := apiv1.Event{
		Time:    metav1.Time{Time: now.Add(-5 * time.Second)},
		Name:    "test_event",
		Type:    apiv1.EventTypeWarning,
		Message: "test message",
	}
	err := mockBucket.Insert(ctx, testEvent)
	require.NoError(t, err)

	// Test Events method
	events, err := c.Events(ctx, now.Add(-10*time.Second))
	require.NoError(t, err)
	require.Len(t, events, 1)
	assert.Equal(t, testEvent.Name, events[0].Name)

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
	mockBucket := NewMockEventBucket()

	c := &component{
		ctx:         cctx,
		cancel:      ccancel,
		eventBucket: mockBucket,
	}

	// Only try to create kmsgSyncer on Linux
	if runtime.GOOS == "linux" {
		kmsgSyncer, err := kmsg.NewSyncer(cctx, func(line string) (string, string) {
			return "test", "test"
		}, mockBucket)
		if err == nil {
			c.kmsgSyncer = kmsgSyncer
		}
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

	// Create instance
	instance := &components.GPUdInstance{
		RootCtx:              context.Background(),
		NVIDIAToolOverwrites: nvidia_common.ToolOverwrites{},
	}

	// Test successful creation
	comp, err := New(instance)
	assert.NoError(t, err)
	assert.NotNil(t, comp)
	defer comp.Close()

	assert.Equal(t, Name, comp.Name())
}

// MockEventBucket implements the events_db.Store interface for testing
type MockEventBucket struct {
	events apiv1.Events
	mu     sync.Mutex
}

func NewMockEventBucket() *MockEventBucket {
	return &MockEventBucket{
		events: apiv1.Events{},
	}
}

func (m *MockEventBucket) Name() string {
	return "mock"
}

func (m *MockEventBucket) Insert(ctx context.Context, event apiv1.Event) error {
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

func (m *MockEventBucket) Get(ctx context.Context, since time.Time) (apiv1.Events, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	var result apiv1.Events
	for _, event := range m.events {
		if !event.Time.Time.Before(since) {
			result = append(result, event)
		}
	}
	return result, nil
}

func (m *MockEventBucket) Find(ctx context.Context, event apiv1.Event) (*apiv1.Event, error) {
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

func (m *MockEventBucket) Latest(ctx context.Context) (*apiv1.Event, error) {
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
		if e.Time.Time.After(latest.Time.Time) {
			latest = e
		}
	}
	return &latest, nil
}

func (m *MockEventBucket) Purge(ctx context.Context, beforeTimestamp int64) (int, error) {
	select {
	case <-ctx.Done():
		return 0, ctx.Err()
	default:
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	var newEvents apiv1.Events
	var purgedCount int

	for _, event := range m.events {
		if event.Time.Time.Unix() >= beforeTimestamp {
			newEvents = append(newEvents, event)
		} else {
			purgedCount++
		}
	}

	m.events = newEvents
	return purgedCount, nil
}

func (m *MockEventBucket) Close() {
	// No-op for mock
}

func (m *MockEventBucket) GetEvents() apiv1.Events {
	m.mu.Lock()
	defer m.mu.Unlock()

	result := make(apiv1.Events, len(m.events))
	copy(result, m.events)
	return result
}

// Test helpers for mocking NVML and IBStat
type mockNVMLInstance struct {
	exists bool
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
	return ""
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

func mockGetIbstatOutput(ctx context.Context, ibstatCommands []string) (*infiniband.IbstatOutput, error) {
	return &infiniband.IbstatOutput{
		Raw: "mock output",
		Parsed: infiniband.IBStatCards{
			{
				Name: "mlx5_0",
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
		ctx:                 cctx,
		cancel:              ccancel,
		getIbstatOutputFunc: mockGetIbstatOutput,
		getThresholdsFunc:   mockGetThresholds,
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
					Name: "mlx5_0",
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
	assert.Contains(t, result, "PORT NAME")
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
	assert.Equal(t, apiv1.HealthStateType(""), cr.HealthState())

	// Test with health state
	cr = &checkResult{health: apiv1.HealthStateTypeUnhealthy}
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.HealthState())
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
		getIbstatOutputFunc: func(ctx context.Context, ibstatCommands []string) (*infiniband.IbstatOutput, error) {
			return nil, errors.New("ibstat error")
		},
		getThresholdsFunc: mockGetThresholds,
		nvmlInstance:      &mockNVMLInstance{exists: true},
	}

	result := c.Check()
	data, ok := result.(*checkResult)
	require.True(t, ok)
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, data.health)
	assert.Contains(t, data.reason, "ibstat command failed")

	// Test case: getIbstatOutputFunc returns nil output
	c = &component{
		ctx:    cctx,
		cancel: ccancel,
		getIbstatOutputFunc: func(ctx context.Context, ibstatCommands []string) (*infiniband.IbstatOutput, error) {
			return nil, nil
		},
		getThresholdsFunc: mockGetThresholds,
		nvmlInstance:      &mockNVMLInstance{exists: true},
	}

	result = c.Check()
	data, ok = result.(*checkResult)
	require.True(t, ok)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, data.health)
	assert.Equal(t, reasonMissingIbstatOutput, data.reason)

	// Test case: ibstat command not found
	c = &component{
		ctx:    cctx,
		cancel: ccancel,
		getIbstatOutputFunc: func(ctx context.Context, ibstatCommands []string) (*infiniband.IbstatOutput, error) {
			return nil, infiniband.ErrNoIbstatCommand
		},
		getThresholdsFunc: mockGetThresholds,
		nvmlInstance:      &mockNVMLInstance{exists: true},
	}

	result = c.Check()
	data, ok = result.(*checkResult)
	require.True(t, ok)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, data.health)
	assert.Equal(t, "ibstat command not found", data.reason)
}

func TestComponentCheckEventBucketOperations(t *testing.T) {
	t.Parallel()

	cctx, ccancel := context.WithCancel(context.Background())
	defer ccancel()

	mockBucket := NewMockEventBucket()

	// Test case: successful event insertion
	c := &component{
		ctx:                 cctx,
		cancel:              ccancel,
		eventBucket:         mockBucket,
		nvmlInstance:        &mockNVMLInstance{exists: true},
		getIbstatOutputFunc: mockGetIbstatOutput,
		getThresholdsFunc: func() infiniband.ExpectedPortStates {
			// Return thresholds that will trigger an unhealthy state
			return infiniband.ExpectedPortStates{
				AtLeastPorts: 5,
				AtLeastRate:  400,
			}
		},
	}

	result := c.Check()
	data, ok := result.(*checkResult)
	require.True(t, ok)
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, data.health)

	// Verify that an event was inserted
	events := mockBucket.GetEvents()
	assert.NotEmpty(t, events)
	assert.Equal(t, "ibstat", events[0].Name)
	assert.Equal(t, apiv1.EventTypeWarning, events[0].Type)
}

// Test that DefaultExpectedPortStates are properly maintained across SetDefaultExpectedPortStates calls
func TestDefaultExpectedPortStatesThreadSafety(t *testing.T) {
	t.Parallel()

	// Save original defaults to restore after test
	originalDefaults := GetDefaultExpectedPortStates()
	defer SetDefaultExpectedPortStates(originalDefaults)

	// Set initial test values
	initialTest := infiniband.ExpectedPortStates{
		AtLeastPorts: 1,
		AtLeastRate:  100,
	}
	SetDefaultExpectedPortStates(initialTest)

	// Run concurrent tests
	const numGoroutines = 10
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			testVal := infiniband.ExpectedPortStates{
				AtLeastPorts: id + 1,
				AtLeastRate:  (id + 1) * 100,
			}
			SetDefaultExpectedPortStates(testVal)

			// Verify we can get the values we just set
			current := GetDefaultExpectedPortStates()
			if current.AtLeastPorts != testVal.AtLeastPorts || current.AtLeastRate != testVal.AtLeastRate {
				t.Errorf("Expected %+v, got %+v", testVal, current)
			}
		}(i)
	}

	wg.Wait()

	// Force a final known value that's definitely different from original defaults
	finalValue := infiniband.ExpectedPortStates{
		AtLeastPorts: 42,
		AtLeastRate:  4200,
	}
	SetDefaultExpectedPortStates(finalValue)

	// Verify we get the specific final value we just set
	final := GetDefaultExpectedPortStates()
	assert.Equal(t, finalValue.AtLeastPorts, final.AtLeastPorts)
	assert.Equal(t, finalValue.AtLeastRate, final.AtLeastRate)
}

func TestCheckWithEventErrors(t *testing.T) {
	t.Parallel()

	cctx, ccancel := context.WithCancel(context.Background())
	defer ccancel()

	// Mock event bucket that returns errors
	errorBucket := &mockErrorBucket{
		findError:   errors.New("find error"),
		insertError: errors.New("insert error"),
	}

	// Test case: Find method returns error
	c := &component{
		ctx:                 cctx,
		cancel:              ccancel,
		eventBucket:         errorBucket,
		nvmlInstance:        &mockNVMLInstance{exists: true},
		getIbstatOutputFunc: mockGetIbstatOutput,
		getThresholdsFunc: func() infiniband.ExpectedPortStates {
			return infiniband.ExpectedPortStates{
				AtLeastPorts: 5,
				AtLeastRate:  400,
			}
		},
	}

	result := c.Check()
	data, ok := result.(*checkResult)
	require.True(t, ok)
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, data.health)
	assert.Contains(t, data.reason, "failed to find ibstat event")

	// Test case: Insert method returns error
	errorBucket.findError = nil // Reset find error

	result = c.Check()
	data, ok = result.(*checkResult)
	require.True(t, ok)
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, data.health)
	assert.Contains(t, data.reason, "failed to insert ibstat event")
}

// Mock event bucket that returns errors
type mockErrorBucket struct {
	findError   error
	insertError error
}

func (m *mockErrorBucket) Name() string {
	return "error-mock"
}

func (m *mockErrorBucket) Insert(ctx context.Context, event apiv1.Event) error {
	if m.insertError != nil {
		return m.insertError
	}
	return nil
}

func (m *mockErrorBucket) Get(ctx context.Context, since time.Time) (apiv1.Events, error) {
	return nil, nil
}

func (m *mockErrorBucket) Find(ctx context.Context, event apiv1.Event) (*apiv1.Event, error) {
	if m.findError != nil {
		return nil, m.findError
	}
	return nil, nil
}

func (m *mockErrorBucket) Latest(ctx context.Context) (*apiv1.Event, error) {
	return nil, nil
}

func (m *mockErrorBucket) Purge(ctx context.Context, beforeTimestamp int64) (int, error) {
	return 0, nil
}

func (m *mockErrorBucket) Close() {
	// No-op
}

// Test when an event already exists in the bucket
func TestCheckWithExistingEvent(t *testing.T) {
	t.Parallel()

	cctx, ccancel := context.WithCancel(context.Background())
	defer ccancel()

	// Mock bucket that returns a found event
	mockBucket := &mockFoundEventBucket{}

	c := &component{
		ctx:                 cctx,
		cancel:              ccancel,
		eventBucket:         mockBucket,
		nvmlInstance:        &mockNVMLInstance{exists: true},
		getIbstatOutputFunc: mockGetIbstatOutput,
		getThresholdsFunc: func() infiniband.ExpectedPortStates {
			return infiniband.ExpectedPortStates{
				AtLeastPorts: 5,
				AtLeastRate:  400,
			}
		},
	}

	// When an event is found, it shouldn't try to insert a new one
	result := c.Check()
	data, ok := result.(*checkResult)
	require.True(t, ok)
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, data.health)

	// Verify that no insertion was attempted
	assert.False(t, mockBucket.insertCalled, "Insert should not be called when event already exists")
}

// Mock bucket that always returns a found event
type mockFoundEventBucket struct {
	insertCalled bool
}

func (m *mockFoundEventBucket) Name() string {
	return "found-event-mock"
}

func (m *mockFoundEventBucket) Insert(ctx context.Context, event apiv1.Event) error {
	m.insertCalled = true
	return nil
}

func (m *mockFoundEventBucket) Get(ctx context.Context, since time.Time) (apiv1.Events, error) {
	return nil, nil
}

func (m *mockFoundEventBucket) Find(ctx context.Context, event apiv1.Event) (*apiv1.Event, error) {
	// Always return a found event
	return &apiv1.Event{
		Time:    metav1.Time{Time: time.Now()},
		Name:    event.Name,
		Type:    event.Type,
		Message: event.Message,
	}, nil
}

func (m *mockFoundEventBucket) Latest(ctx context.Context) (*apiv1.Event, error) {
	return nil, nil
}

func (m *mockFoundEventBucket) Purge(ctx context.Context, beforeTimestamp int64) (int, error) {
	return 0, nil
}

func (m *mockFoundEventBucket) Close() {
	// No-op
}

// Test Check when getIbstatOutputFunc is nil
func TestCheckNilIbstatFunc(t *testing.T) {
	t.Parallel()

	cctx, ccancel := context.WithCancel(context.Background())
	defer ccancel()

	c := &component{
		ctx:                 cctx,
		cancel:              ccancel,
		nvmlInstance:        &mockNVMLInstance{exists: true},
		getIbstatOutputFunc: nil, // Set to nil explicitly
		getThresholdsFunc:   mockGetThresholds,
	}

	result := c.Check()
	data, ok := result.(*checkResult)
	require.True(t, ok)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, data.health)
	assert.Equal(t, "ibstat checker not found", data.reason)
}
