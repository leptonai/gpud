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

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	nvidia_common "github.com/leptonai/gpud/pkg/config/common"
	"github.com/leptonai/gpud/pkg/eventstore"
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
			wantReason: reasonThresholdNotSetSkipped,
			wantHealth: apiv1.HealthStateTypeHealthy,
		},
		{
			name:   "only rate threshold set",
			output: &infiniband.IbstatOutput{Parsed: infiniband.IBStatCards{}},
			config: infiniband.ExpectedPortStates{
				AtLeastPorts: 0,
				AtLeastRate:  200,
			},
			wantReason: reasonThresholdNotSetSkipped,
			wantHealth: apiv1.HealthStateTypeHealthy,
		},
		{
			name: "healthy state with matching ports and rate",
			output: &infiniband.IbstatOutput{
				Raw: "",
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
			wantReason: reasonNoIbIssueFoundFromIbstat,
			wantHealth: apiv1.HealthStateTypeHealthy,
		},
		{
			name: "unhealthy state - not enough ports",
			output: &infiniband.IbstatOutput{
				Raw: "",
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
						Device: "mlx5_0",
						Port1: infiniband.IBStatPort{
							State:         "Active",
							PhysicalState: "LinkUp",
							Rate:          100,
						},
					},
					{
						Device: "mlx5_1",
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
						Device: "mlx5_0",
						Port1: infiniband.IBStatPort{
							State:         "Down",
							PhysicalState: "Disabled",
							Rate:          200,
						},
					},
					{
						Device: "mlx5_1",
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
						Device: "mlx5_0",
						Port1: infiniband.IBStatPort{
							State:         "Inactive",
							PhysicalState: "LinkUp",
							Rate:          200,
						},
					},
					{
						Device: "mlx5_1",
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
			wantReason: reasonNoIbIssueFoundFromIbstat,
			wantHealth: apiv1.HealthStateTypeHealthy,
		},
		{
			name: "mixed port states",
			output: &infiniband.IbstatOutput{
				Raw: "",
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
							Rate:          200,
						},
					},
					{
						Device: "mlx5_2",
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
						Device: "mlx5_0",
						Port1: infiniband.IBStatPort{
							State:         "Active",
							PhysicalState: "LinkUp",
							Rate:          400,
						},
					},
					{
						Device: "mlx5_1",
						Port1: infiniband.IBStatPort{
							State:         "Active",
							PhysicalState: "LinkUp",
							Rate:          200,
						},
					},
					{
						Device: "mlx5_2",
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
						Device: "mlx5_0",
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

			health, suggestedActions, reason := evaluateIbstatOutputAgainstThresholds(tt.output, tt.config)
			assert.Equal(t, tt.wantReason, reason)
			assert.Equal(t, tt.wantHealth, health)
			// For healthy states, suggestedActions should be nil
			if tt.wantHealth == apiv1.HealthStateTypeHealthy {
				assert.Nil(t, suggestedActions)
			} else {
				// For unhealthy states, should have hardware inspection suggested
				assert.NotNil(t, suggestedActions)
				assert.Equal(t, []apiv1.RepairActionType{apiv1.RepairActionTypeHardwareInspection}, suggestedActions.RepairActions)
			}
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
			wantReason: reasonNoIbIssueFoundFromIbstat,
			wantHealth: apiv1.HealthStateTypeHealthy,
		},
		{
			name: "healthy state - mixed rate ports",
			config: infiniband.ExpectedPortStates{
				AtLeastPorts: 12,  // Total number of ports in test data
				AtLeastRate:  100, // Minimum rate that includes all ports
			},
			wantReason: reasonNoIbIssueFoundFromIbstat,
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
			health, suggestedActions, reason := evaluateIbstatOutputAgainstThresholds(output, tt.config)
			assert.Equal(t, tt.wantReason, reason)
			assert.Equal(t, tt.wantHealth, health)
			// For healthy states, suggestedActions should be nil
			if tt.wantHealth == apiv1.HealthStateTypeHealthy {
				assert.Nil(t, suggestedActions)
			} else {
				// For unhealthy states, should have hardware inspection suggested
				assert.NotNil(t, suggestedActions)
				assert.Equal(t, []apiv1.RepairActionType{apiv1.RepairActionTypeHardwareInspection}, suggestedActions.RepairActions)
			}
		})
	}
}

func TestComponentCheck(t *testing.T) {
	t.Parallel()

	// Create a component with mocked functions
	cctx, ccancel := context.WithCancel(context.Background())
	defer ccancel()

	// Apply fixes to ensure context is properly set
	c := &component{
		ctx:                 cctx,
		cancel:              ccancel,
		getIbstatOutputFunc: nil, // Explicitly set to nil to test this case
		getThresholdsFunc:   mockGetThresholds,
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
	// Check for the actual error message that occurs with nil getIbstatOutputFunc
	assert.Equal(t, "ibstat checker not found", data.reason)
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
		NVIDIAToolOverwrites: nvidia_common.ToolOverwrites{},
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
		NVIDIAToolOverwrites: nvidia_common.ToolOverwrites{},
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

// MockEventStore for testing New errors
type MockEventStore struct {
	bucketErr error
}

func (m *MockEventStore) Bucket(name string, opts ...eventstore.OpOption) (eventstore.Bucket, error) {
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

func (m *mockEventBucket) Get(ctx context.Context, since time.Time) (eventstore.Events, error) {
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

func mockGetIbstatOutput(ctx context.Context, ibstatCommands []string) (*infiniband.IbstatOutput, error) {
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
		getIbstatOutputFunc: func(ctx context.Context, ibstatCommands []string) (*infiniband.IbstatOutput, error) {
			return nil, errors.New("ibstat error")
		},
		getThresholdsFunc: mockGetThresholds,
		nvmlInstance:      &mockNVMLInstance{exists: true, productName: "Tesla V100"},
	}

	result := c.Check()
	data, ok := result.(*checkResult)
	require.True(t, ok)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, data.health)
	assert.Equal(t, "missing event storage (skipped evaluation)", data.reason)

	// Test case: ibstat command not found
	c = &component{
		ctx:    cctx,
		cancel: ccancel,
		getIbstatOutputFunc: func(ctx context.Context, ibstatCommands []string) (*infiniband.IbstatOutput, error) {
			return nil, infiniband.ErrNoIbstatCommand
		},
		getThresholdsFunc: mockGetThresholds,
		nvmlInstance:      &mockNVMLInstance{exists: true, productName: "Tesla V100"},
	}

	result = c.Check()
	data, ok = result.(*checkResult)
	require.True(t, ok)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, data.health)
	assert.Equal(t, "missing event storage (skipped evaluation)", data.reason)
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

// Test Check when getIbstatOutputFunc is nil
func TestCheckNilIbstatFunc(t *testing.T) {
	t.Parallel()

	cctx, ccancel := context.WithCancel(context.Background())
	defer ccancel()

	c := &component{
		ctx:                 cctx,
		cancel:              ccancel,
		nvmlInstance:        &mockNVMLInstance{exists: true, productName: "Tesla V100"},
		getIbstatOutputFunc: nil, // Set to nil explicitly
		getThresholdsFunc:   mockGetThresholds,
	}

	result := c.Check()
	data, ok := result.(*checkResult)
	require.True(t, ok)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, data.health)
	assert.Equal(t, "ibstat checker not found", data.reason)
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
		getThresholdsFunc: func() infiniband.ExpectedPortStates {
			trackCheck("thresholds")
			return infiniband.ExpectedPortStates{} // zero thresholds
		},
	}

	result := c.Check()
	data, ok := result.(*checkResult)
	require.True(t, ok)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, data.health)
	assert.Equal(t, reasonThresholdNotSetSkipped, data.reason)
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
	mockStore := &MockEventStore{
		bucketErr: errors.New("bucket creation error"),
	}

	gpudInstance := &components.GPUdInstance{
		RootCtx:              ctx,
		NVIDIAToolOverwrites: nvidia_common.ToolOverwrites{},
		EventStore:           mockStore,
	}

	comp, err := New(gpudInstance)
	assert.Error(t, err)
	assert.Nil(t, comp)
	assert.Contains(t, err.Error(), "bucket creation error")
}

// TestCheckWithEventBucketOperations tests Check method with event bucket operations
func TestCheckWithEventBucketOperations(t *testing.T) {
	t.Parallel()

	// Test case 1: Find operation fails
	t.Run("find operation fails", func(t *testing.T) {
		cctx, ccancel := context.WithCancel(context.Background())
		defer ccancel()

		mockBucket := createMockEventBucket()
		mockBucket.findErr = errors.New("find error")

		c := &component{
			ctx:          cctx,
			cancel:       ccancel,
			nvmlInstance: &mockNVMLInstance{exists: true, productName: "Tesla V100"},
			eventBucket:  mockBucket,
			getIbstatOutputFunc: func(ctx context.Context, ibstatCommands []string) (*infiniband.IbstatOutput, error) {
				// Return unhealthy state to trigger event operations
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
		assert.Equal(t, "error finding ibstat event", data.reason)
		assert.NotNil(t, data.err)
	})

	// Test case 2: Insert operation fails
	t.Run("insert operation fails", func(t *testing.T) {
		cctx, ccancel := context.WithCancel(context.Background())
		defer ccancel()

		mockBucket := createMockEventBucket()
		mockBucket.insertErr = errors.New("insert error")

		c := &component{
			ctx:          cctx,
			cancel:       ccancel,
			nvmlInstance: &mockNVMLInstance{exists: true, productName: "Tesla V100"},
			eventBucket:  mockBucket,
			getIbstatOutputFunc: func(ctx context.Context, ibstatCommands []string) (*infiniband.IbstatOutput, error) {
				// Return unhealthy state to trigger event operations
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
		assert.Equal(t, "error inserting ibstat event", data.reason)
		assert.NotNil(t, data.err)
	})

	// Test case 3: Event already exists
	t.Run("event already exists", func(t *testing.T) {
		cctx, ccancel := context.WithCancel(context.Background())
		defer ccancel()

		mockBucket := createMockEventBucket()
		// Pre-insert an event to simulate it already exists
		existingEvent := eventstore.Event{
			Time:    time.Now().UTC(),
			Name:    "ibstat",
			Type:    string(apiv1.EventTypeWarning),
			Message: "only 0 ports (>= 200 Gb/s) are active, expect at least 1; 1 device(s) found Disabled (mlx5_0)",
		}
		_ = mockBucket.Insert(cctx, existingEvent)

		c := &component{
			ctx:          cctx,
			cancel:       ccancel,
			nvmlInstance: &mockNVMLInstance{exists: true, productName: "Tesla V100"},
			eventBucket:  mockBucket,
			getIbstatOutputFunc: func(ctx context.Context, ibstatCommands []string) (*infiniband.IbstatOutput, error) {
				// Return unhealthy state to trigger event operations
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
		// Event already exists, so no error
		assert.Nil(t, data.err)
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
		getIbstatOutputFunc: func(ctx context.Context, ibstatCommands []string) (*infiniband.IbstatOutput, error) {
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
	assert.Equal(t, reasonNoIbIssueFoundFromIbstat, data.reason)
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
		getIbstatOutputFunc: func(ctx context.Context, ibstatCommands []string) (*infiniband.IbstatOutput, error) {
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
	assert.Equal(t, reasonMissingIbstatOutput, data.reason)
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
	assert.NotNil(t, states[0].ExtraInfo)
	assert.Contains(t, states[0].ExtraInfo["data"], "ibstat_output")
	assert.Contains(t, states[0].ExtraInfo["data"], "mlx5_0")
}

// TestCheckSuccessfulEventInsertion tests successful unhealthy event insertion
func TestCheckSuccessfulEventInsertion(t *testing.T) {
	t.Parallel()

	cctx, ccancel := context.WithCancel(context.Background())
	defer ccancel()

	mockBucket := createMockEventBucket()

	c := &component{
		ctx:          cctx,
		cancel:       ccancel,
		nvmlInstance: &mockNVMLInstance{exists: true, productName: "Tesla V100"},
		eventBucket:  mockBucket,
		getIbstatOutputFunc: func(ctx context.Context, ibstatCommands []string) (*infiniband.IbstatOutput, error) {
			// Return unhealthy state to trigger event insertion
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
	assert.Contains(t, data.reason, "only 0 ports")
	assert.Nil(t, data.err)

	// Verify event was inserted
	events := mockBucket.GetAPIEvents()
	assert.Len(t, events, 1)
	assert.Equal(t, "ibstat", events[0].Name)
	assert.Equal(t, apiv1.EventTypeWarning, events[0].Type)
}

// TestNewKmsgSyncerHandling tests New function when kmsg.NewSyncer fails
// Note: We can't directly test the os.Geteuid() == 0 path due to test restrictions
// But we can verify that New handles errors gracefully
func TestNewKmsgSyncerHandling(t *testing.T) {
	t.Parallel()

	// Test that New succeeds even without root privileges
	ctx := context.Background()
	mockStore := &MockEventStore{}

	gpudInstance := &components.GPUdInstance{
		RootCtx:              ctx,
		NVIDIAToolOverwrites: nvidia_common.ToolOverwrites{},
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
	assert.Equal(t, reasonThresholdNotSetSkipped, data.reason)
}
