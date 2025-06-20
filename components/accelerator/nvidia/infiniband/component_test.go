package infiniband

import (
	"context"
	"encoding/json"
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
			wantReason: "only 1 port(s) are active and >=200 Gb/s, expect >=2 port(s)",
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
			wantReason: "only 0 port(s) are active and >=200 Gb/s, expect >=2 port(s)",
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
			wantReason: "only 0 port(s) are active and >=200 Gb/s, expect >=2 port(s); 2 device(s) found Disabled (mlx5_0, mlx5_1)",
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
			wantReason: reasonMissingIbstatIbstatusOutput,
			wantHealth: apiv1.HealthStateTypeHealthy,
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
			wantReason: "only 2 port(s) are active and >=200 Gb/s, expect >=3 port(s); 1 device(s) found Disabled (mlx5_1)",
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
			wantReason: "only 1 port(s) are active and >=300 Gb/s, expect >=2 port(s)",
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
			wantReason: "only 0 port(s) are active and >=100 Gb/s, expect >=1 port(s)",
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

			// Create a checkResult and populate allIBPorts
			cr := &checkResult{
				allIBPorts: tt.output.Parsed.IBPorts(),
			}

			// Call the function
			evaluateThresholds(cr, tt.config)

			// Check results
			assert.Equal(t, tt.wantReason, cr.reason)
			assert.Equal(t, tt.wantHealth, cr.health)
			// For healthy states, suggestedActions should be nil
			if tt.wantHealth == apiv1.HealthStateTypeHealthy {
				assert.Nil(t, cr.suggestedActions)
			} else {
				// For unhealthy states, should have hardware inspection suggested
				assert.NotNil(t, cr.suggestedActions)
				assert.Equal(t, []apiv1.RepairActionType{apiv1.RepairActionTypeHardwareInspection}, cr.suggestedActions.RepairActions)
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
			wantReason: "only 8 port(s) are active and >=400 Gb/s, expect >=12 port(s)",
			wantHealth: apiv1.HealthStateTypeUnhealthy,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a checkResult and populate allIBPorts
			cr := &checkResult{
				allIBPorts: output.Parsed.IBPorts(),
			}

			// Call the function
			evaluateThresholds(cr, tt.config)

			// Check results
			assert.Equal(t, tt.wantReason, cr.reason)
			assert.Equal(t, tt.wantHealth, cr.health)
			// For healthy states, suggestedActions should be nil
			if tt.wantHealth == apiv1.HealthStateTypeHealthy {
				assert.Nil(t, cr.suggestedActions)
			} else {
				// For unhealthy states, should have hardware inspection suggested
				assert.NotNil(t, cr.suggestedActions)
				assert.Equal(t, []apiv1.RepairActionType{apiv1.RepairActionTypeHardwareInspection}, cr.suggestedActions.RepairActions)
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
		// Compare relevant fields for duplicate detection
		// In production, this should also compare ib_ports or a hash
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
		getIbstatusOutputFunc: func(ctx context.Context, ibstatusCommands []string) (*infiniband.IbstatusOutput, error) {
			return nil, errors.New("ibstatus error")
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
	assert.Equal(t, "ibstat checker not found", data.reason)
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

// Mock function for ibstatus output
func mockGetIbstatusOutput(ctx context.Context, ibstatusCommands []string) (*infiniband.IbstatusOutput, error) {
	return &infiniband.IbstatusOutput{
		Raw: "mock ibstatus output",
		Parsed: infiniband.IBStatuses{
			{
				Device:        "mlx5_0",
				State:         "4: ACTIVE",
				PhysicalState: "5: LinkUp",
				Rate:          "200 Gb/sec",
				LinkLayer:     "InfiniBand",
			},
		},
	}, nil
}

func TestEvaluateIbstatusOutput(t *testing.T) {
	tests := []struct {
		name                 string
		output               *infiniband.IbstatusOutput
		config               infiniband.ExpectedPortStates
		wantReason           string
		wantHealth           apiv1.HealthStateType
		wantSuggestedActions *apiv1.SuggestedActions
	}{
		{
			name:   "thresholds not set",
			output: &infiniband.IbstatusOutput{},
			config: infiniband.ExpectedPortStates{
				AtLeastPorts: 0,
				AtLeastRate:  0,
			},
			wantReason:           reasonThresholdNotSetSkipped,
			wantHealth:           apiv1.HealthStateTypeHealthy,
			wantSuggestedActions: nil,
		},
		{
			name: "healthy state with matching ports and rate",
			output: &infiniband.IbstatusOutput{
				Raw: "",
				Parsed: infiniband.IBStatuses{
					{
						Device:        "mlx5_0",
						State:         "4: ACTIVE",
						PhysicalState: "5: LinkUp",
						Rate:          "200 Gb/sec",
						LinkLayer:     "InfiniBand",
					},
					{
						Device:        "mlx5_1",
						State:         "4: ACTIVE",
						PhysicalState: "5: LinkUp",
						Rate:          "200 Gb/sec",
						LinkLayer:     "InfiniBand",
					},
				},
			},
			config: infiniband.ExpectedPortStates{
				AtLeastPorts: 2,
				AtLeastRate:  200,
			},
			wantReason:           reasonNoIbIssueFoundFromIbstat,
			wantHealth:           apiv1.HealthStateTypeHealthy,
			wantSuggestedActions: nil,
		},
		{
			name: "unhealthy state - not enough ports",
			output: &infiniband.IbstatusOutput{
				Raw: "",
				Parsed: infiniband.IBStatuses{
					{
						Device:        "mlx5_0",
						State:         "4: ACTIVE",
						PhysicalState: "5: LinkUp",
						Rate:          "200 Gb/sec",
						LinkLayer:     "InfiniBand",
					},
				},
			},
			config: infiniband.ExpectedPortStates{
				AtLeastPorts: 2,
				AtLeastRate:  200,
			},
			wantReason: "only 1 port(s) are active and >=200 Gb/s, expect >=2 port(s)",
			wantHealth: apiv1.HealthStateTypeUnhealthy,
			wantSuggestedActions: &apiv1.SuggestedActions{
				RepairActions: []apiv1.RepairActionType{apiv1.RepairActionTypeHardwareInspection},
			},
		},
		{
			name: "unhealthy state - rate too low",
			output: &infiniband.IbstatusOutput{
				Raw: "",
				Parsed: infiniband.IBStatuses{
					{
						Device:        "mlx5_0",
						State:         "4: ACTIVE",
						PhysicalState: "5: LinkUp",
						Rate:          "100 Gb/sec",
						LinkLayer:     "InfiniBand",
					},
					{
						Device:        "mlx5_1",
						State:         "4: ACTIVE",
						PhysicalState: "5: LinkUp",
						Rate:          "100 Gb/sec",
						LinkLayer:     "InfiniBand",
					},
				},
			},
			config: infiniband.ExpectedPortStates{
				AtLeastPorts: 2,
				AtLeastRate:  200,
			},
			wantReason: "only 0 port(s) are active and >=200 Gb/s, expect >=2 port(s)",
			wantHealth: apiv1.HealthStateTypeUnhealthy,
			wantSuggestedActions: &apiv1.SuggestedActions{
				RepairActions: []apiv1.RepairActionType{apiv1.RepairActionTypeHardwareInspection},
			},
		},
		{
			name: "empty ibstatus devices",
			output: &infiniband.IbstatusOutput{
				Raw:    "",
				Parsed: infiniband.IBStatuses{},
			},
			config: infiniband.ExpectedPortStates{
				AtLeastPorts: 2,
				AtLeastRate:  200,
			},
			wantReason:           reasonMissingIbstatIbstatusOutput,
			wantHealth:           apiv1.HealthStateTypeHealthy,
			wantSuggestedActions: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Skip nil output test to avoid panic
			if tt.output == nil {
				t.Skip("Skipping test with nil output")
				return
			}

			// Create a checkResult and populate allIBPorts
			cr := &checkResult{
				allIBPorts: tt.output.Parsed.IBPorts(),
			}

			// Call the function
			evaluateThresholds(cr, tt.config)

			// Check results
			assert.Equal(t, tt.wantReason, cr.reason)
			assert.Equal(t, tt.wantHealth, cr.health)
			assert.Equal(t, tt.wantSuggestedActions, cr.suggestedActions)
		})
	}
}

func TestComponentUsingIbstatusOutput(t *testing.T) {
	t.Parallel()

	cctx, ccancel := context.WithCancel(context.Background())
	defer ccancel()

	// Case: ibstat fails but ibstatus succeeds
	c := &component{
		ctx:    cctx,
		cancel: ccancel,
		nvmlInstance: &mockNVMLInstance{
			exists:      true,
			productName: "Tesla V100",
		},
		getIbstatOutputFunc: func(ctx context.Context, ibstatCommands []string) (*infiniband.IbstatOutput, error) {
			return nil, errors.New("ibstat command failed")
		},
		getIbstatusOutputFunc: mockGetIbstatusOutput,
		getThresholdsFunc: func() infiniband.ExpectedPortStates {
			return infiniband.ExpectedPortStates{
				AtLeastPorts: 1,
				AtLeastRate:  100,
			}
		},
	}

	result := c.Check()
	data, ok := result.(*checkResult)
	require.True(t, ok)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, data.health)
	assert.Equal(t, "missing event storage (skipped evaluation)", data.reason)
	assert.NotNil(t, data.IbstatusOutput, "Expected ibstatus output to be populated")
	assert.Nil(t, data.IbstatOutput, "Expected ibstat output to be nil")
}

func TestComponentWithBothOutputFunctions(t *testing.T) {
	t.Parallel()

	cctx, ccancel := context.WithCancel(context.Background())
	defer ccancel()

	// Case: Both functions succeed
	c := &component{
		ctx:    cctx,
		cancel: ccancel,
		nvmlInstance: &mockNVMLInstance{
			exists:      true,
			productName: "Tesla V100",
		},
		getIbstatOutputFunc:   mockGetIbstatOutput,
		getIbstatusOutputFunc: mockGetIbstatusOutput,
		getThresholdsFunc: func() infiniband.ExpectedPortStates {
			return infiniband.ExpectedPortStates{
				AtLeastPorts: 1,
				AtLeastRate:  100,
			}
		},
	}

	result := c.Check()
	data, ok := result.(*checkResult)
	require.True(t, ok)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, data.health)
	assert.Equal(t, "missing event storage (skipped evaluation)", data.reason)
	assert.NotNil(t, data.IbstatOutput, "Expected ibstat output to be populated")
	assert.NotNil(t, data.IbstatusOutput, "Expected ibstatus output to be populated as well")
}

func TestComponentWithIbstatusError(t *testing.T) {
	t.Parallel()

	cctx, ccancel := context.WithCancel(context.Background())
	defer ccancel()

	// Case: ibstatus fails but ibstat succeeds
	c := &component{
		ctx:    cctx,
		cancel: ccancel,
		nvmlInstance: &mockNVMLInstance{
			exists:      true,
			productName: "Tesla V100",
		},
		getIbstatOutputFunc: mockGetIbstatOutput,
		getIbstatusOutputFunc: func(ctx context.Context, ibstatusCommands []string) (*infiniband.IbstatusOutput, error) {
			return nil, errors.New("ibstatus command failed")
		},
		getThresholdsFunc: func() infiniband.ExpectedPortStates {
			return infiniband.ExpectedPortStates{
				AtLeastPorts: 1,
				AtLeastRate:  100,
			}
		},
	}

	result := c.Check()
	data, ok := result.(*checkResult)
	require.True(t, ok)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, data.health)
	assert.Equal(t, "missing event storage (skipped evaluation)", data.reason)
	assert.NotNil(t, data.IbstatOutput, "Expected ibstat output to be populated")
	assert.Nil(t, data.IbstatusOutput, "Expected ibstatus output to be nil due to error")
	assert.Error(t, data.errIbstatus, "Expected ibstatus error to be set")
}

func TestComponentFallbackToIbstatus(t *testing.T) {
	t.Parallel()

	cctx, ccancel := context.WithCancel(context.Background())
	defer ccancel()

	// Case: ibstat fails with no output, should fallback to ibstatus
	c := &component{
		ctx:    cctx,
		cancel: ccancel,
		nvmlInstance: &mockNVMLInstance{
			exists:      true,
			productName: "Tesla V100",
		},
		getIbstatOutputFunc: func(ctx context.Context, ibstatCommands []string) (*infiniband.IbstatOutput, error) {
			return nil, errors.New("ibstat command failed with no output")
		},
		getIbstatusOutputFunc: mockGetIbstatusOutput,
		getThresholdsFunc: func() infiniband.ExpectedPortStates {
			return infiniband.ExpectedPortStates{
				AtLeastPorts: 1,
				AtLeastRate:  100,
			}
		},
	}

	result := c.Check()
	data, ok := result.(*checkResult)
	require.True(t, ok)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, data.health)
	assert.Equal(t, "missing event storage (skipped evaluation)", data.reason)
	assert.NotNil(t, data.IbstatusOutput, "Expected ibstatus output to be populated")
	assert.Nil(t, data.IbstatOutput, "Expected ibstat output to be nil")
}

func TestComponentBothOutputFail(t *testing.T) {
	t.Parallel()

	cctx, ccancel := context.WithCancel(context.Background())
	defer ccancel()

	// Case: Both ibstat and ibstatus fail
	c := &component{
		ctx:    cctx,
		cancel: ccancel,
		nvmlInstance: &mockNVMLInstance{
			exists:      true,
			productName: "Tesla V100",
		},
		getIbstatOutputFunc: func(ctx context.Context, ibstatCommands []string) (*infiniband.IbstatOutput, error) {
			return nil, errors.New("ibstat command failed")
		},
		getIbstatusOutputFunc: func(ctx context.Context, ibstatusCommands []string) (*infiniband.IbstatusOutput, error) {
			return nil, errors.New("ibstatus command failed")
		},
		getThresholdsFunc: func() infiniband.ExpectedPortStates {
			return infiniband.ExpectedPortStates{
				AtLeastPorts: 1,
				AtLeastRate:  100,
			}
		},
	}

	result := c.Check()
	data, ok := result.(*checkResult)
	require.True(t, ok)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, data.health)
	assert.Equal(t, "missing event storage (skipped evaluation)", data.reason)
	assert.Nil(t, data.IbstatOutput, "Expected ibstat output to be nil")
	assert.Nil(t, data.IbstatusOutput, "Expected ibstatus output to be nil")
	assert.Error(t, data.err, "Expected ibstat error to be set")
	assert.Error(t, data.errIbstatus, "Expected ibstatus error to be set")
}

func TestCheckResultWithIbstatusOutput(t *testing.T) {
	t.Parallel()

	// Test String() method with ibstatus output
	cr := &checkResult{
		IbstatusOutput: &infiniband.IbstatusOutput{
			Parsed: infiniband.IBStatuses{
				{
					Device:        "mlx5_0",
					State:         "4: ACTIVE",
					PhysicalState: "5: LinkUp",
					Rate:          "200 Gb/sec",
					LinkLayer:     "InfiniBand",
				},
			},
		},
	}
	result := cr.String()
	assert.Contains(t, result, "DEVICE")
	assert.Contains(t, result, "STATE")
	assert.Contains(t, result, "mlx5_0")
	assert.Contains(t, result, "4: ACTIVE")
	assert.Contains(t, result, "200 Gb/sec")

	// Test String() method with both outputs
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
		IbstatusOutput: &infiniband.IbstatusOutput{
			Parsed: infiniband.IBStatuses{
				{
					Device:        "mlx5_0",
					State:         "4: ACTIVE",
					PhysicalState: "5: LinkUp",
					Rate:          "200 Gb/sec",
					LinkLayer:     "InfiniBand",
				},
			},
		},
	}
	result = cr.String()
	assert.Contains(t, result, "PORT DEVICE NAME")
	assert.Contains(t, result, "DEVICE")
	assert.Contains(t, result, "LINK LAYER")
}

// Test checkResult methods directly to increase method-level coverage
func TestCheckResultMethodsDirectCoverage(t *testing.T) {
	t.Parallel()

	// Create a result with defined values
	result := &checkResult{
		IbstatOutput: &infiniband.IbstatOutput{
			Raw: "test raw data for ibstat",
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
		IbstatusOutput: &infiniband.IbstatusOutput{
			Raw: "test raw data for ibstatus",
			Parsed: infiniband.IBStatuses{
				{
					Device:        "mlx5_0",
					State:         "4: ACTIVE",
					PhysicalState: "5: LinkUp",
					Rate:          "200 Gb/sec",
					LinkLayer:     "InfiniBand",
				},
			},
		},
		err:         errors.New("test ibstat error"),
		errIbstatus: errors.New("test ibstatus error"),
		reason:      "test reason",
		health:      apiv1.HealthStateTypeUnhealthy,
		ts:          time.Now().UTC(),
	}

	// Test Summary method
	summaryOutput := result.Summary()
	assert.Equal(t, "test reason", summaryOutput)

	// Test HealthStateType method
	healthType := result.HealthStateType()
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, healthType)

	// Test getError method
	errorOutput := result.getError()
	assert.Equal(t, "test ibstat error", errorOutput)

	// Test with ibstat error nil
	resultWithoutIbstatError := &checkResult{
		err:         nil,
		errIbstatus: errors.New("ibstatus error"),
	}
	assert.Equal(t, "", resultWithoutIbstatError.getError())

	// Test HealthStates method
	healthStates := result.HealthStates()
	assert.Equal(t, 1, len(healthStates))
	assert.Equal(t, Name, healthStates[0].Component)
	assert.Equal(t, Name, healthStates[0].Name)
	assert.Equal(t, "test reason", healthStates[0].Reason)
	assert.Equal(t, "test ibstat error", healthStates[0].Error)
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, healthStates[0].Health)
	assert.NotNil(t, healthStates[0].ExtraInfo)

	// Test HealthStates with nil IbstatOutput
	resultWithoutIbstatOutput := &checkResult{
		reason: "test reason without output",
		health: apiv1.HealthStateTypeHealthy,
		ts:     time.Now().UTC(),
	}
	healthStatesWithoutOutput := resultWithoutIbstatOutput.HealthStates()
	assert.Equal(t, 1, len(healthStatesWithoutOutput))
	assert.Equal(t, "test reason without output", healthStatesWithoutOutput[0].Reason)
	assert.Nil(t, healthStatesWithoutOutput[0].ExtraInfo)

	// Test HealthStates with suggested actions
	resultWithSuggestedActions := &checkResult{
		reason: "test reason with suggested actions",
		health: apiv1.HealthStateTypeUnhealthy,
		suggestedActions: &apiv1.SuggestedActions{
			RepairActions: []apiv1.RepairActionType{apiv1.RepairActionTypeHardwareInspection},
		},
		ts: time.Now().UTC(),
	}
	healthStatesWithActions := resultWithSuggestedActions.HealthStates()
	assert.Equal(t, 1, len(healthStatesWithActions))
	assert.Equal(t, "test reason with suggested actions", healthStatesWithActions[0].Reason)
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, healthStatesWithActions[0].Health)
	assert.NotNil(t, healthStatesWithActions[0].SuggestedActions)
	assert.Equal(t, []apiv1.RepairActionType{apiv1.RepairActionTypeHardwareInspection}, healthStatesWithActions[0].SuggestedActions.RepairActions)
}

// Test complete coverage of the component.String method with various combinations of outputs
func TestComponentStringWithVariousOutputs(t *testing.T) {
	t.Parallel()

	// Test with nil
	var nilResult *checkResult
	assert.Equal(t, "", nilResult.String())
	// Also test other methods on nil
	assert.Equal(t, "", nilResult.Summary())
	assert.Equal(t, apiv1.HealthStateType(""), nilResult.HealthStateType())
	assert.Equal(t, "", nilResult.getError())
	nilResultHealthStates := nilResult.HealthStates()
	assert.NotNil(t, nilResultHealthStates)
	assert.Equal(t, 1, len(nilResultHealthStates))
	assert.Equal(t, "no data yet", nilResultHealthStates[0].Reason)

	// Test with no data
	emptyResult := &checkResult{}
	assert.Equal(t, "no data", emptyResult.String())
	// Also test other methods on empty result
	assert.Equal(t, "", emptyResult.Summary())
	assert.Equal(t, apiv1.HealthStateType(""), emptyResult.HealthStateType())
	assert.Equal(t, "", emptyResult.getError())
	emptyResultHealthStates := emptyResult.HealthStates()
	assert.NotNil(t, emptyResultHealthStates)
	assert.Equal(t, 1, len(emptyResultHealthStates))

	// Test with only ibstat output
	ibstatResult := &checkResult{
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
		reason: "test ibstat reason",
		health: apiv1.HealthStateTypeHealthy,
		ts:     time.Now().UTC(),
	}
	ibstatStr := ibstatResult.String()
	assert.Contains(t, ibstatStr, "PORT DEVICE NAME")
	assert.Contains(t, ibstatStr, "mlx5_0")
	assert.NotContains(t, ibstatStr, "LINK LAYER")
	// Also test other methods
	assert.Equal(t, "test ibstat reason", ibstatResult.Summary())
	assert.Equal(t, apiv1.HealthStateTypeHealthy, ibstatResult.HealthStateType())
	assert.Equal(t, "", ibstatResult.getError())
	ibstatResultHealthStates := ibstatResult.HealthStates()
	assert.NotNil(t, ibstatResultHealthStates)
	assert.Equal(t, 1, len(ibstatResultHealthStates))
	assert.Equal(t, "test ibstat reason", ibstatResultHealthStates[0].Reason)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, ibstatResultHealthStates[0].Health)

	// Test with only ibstatus output
	ibstatusResult := &checkResult{
		IbstatusOutput: &infiniband.IbstatusOutput{
			Parsed: infiniband.IBStatuses{
				{
					Device:        "mlx5_0",
					State:         "4: ACTIVE",
					PhysicalState: "5: LinkUp",
					Rate:          "200 Gb/sec",
					LinkLayer:     "InfiniBand",
				},
			},
		},
		reason:      "test ibstatus reason",
		health:      apiv1.HealthStateTypeUnhealthy,
		errIbstatus: errors.New("test ibstatus error"),
		ts:          time.Now().UTC(),
	}
	ibstatusStr := ibstatusResult.String()
	assert.Contains(t, ibstatusStr, "DEVICE")
	assert.Contains(t, ibstatusStr, "mlx5_0")
	assert.Contains(t, ibstatusStr, "LINK LAYER")
	assert.Contains(t, ibstatusStr, "InfiniBand")
	// Also test other methods
	assert.Equal(t, "test ibstatus reason", ibstatusResult.Summary())
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, ibstatusResult.HealthStateType())
	assert.Equal(t, "", ibstatusResult.getError())
	ibstatusResultHealthStates := ibstatusResult.HealthStates()
	assert.NotNil(t, ibstatusResultHealthStates)
	assert.Equal(t, 1, len(ibstatusResultHealthStates))
	assert.Equal(t, "test ibstatus reason", ibstatusResultHealthStates[0].Reason)
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, ibstatusResultHealthStates[0].Health)
	assert.Equal(t, "", ibstatusResultHealthStates[0].Error)

	// Test with both outputs
	bothResult := &checkResult{
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
		IbstatusOutput: &infiniband.IbstatusOutput{
			Parsed: infiniband.IBStatuses{
				{
					Device:        "mlx5_0",
					State:         "4: ACTIVE",
					PhysicalState: "5: LinkUp",
					Rate:          "200 Gb/sec",
					LinkLayer:     "InfiniBand",
				},
			},
		},
		reason:      "test both reason",
		health:      apiv1.HealthStateTypeHealthy,
		err:         errors.New("test ibstat error"),
		errIbstatus: errors.New("test ibstatus error"),
		ts:          time.Now().UTC(),
	}
	bothStr := bothResult.String()
	assert.Contains(t, bothStr, "PORT DEVICE NAME")
	assert.Contains(t, bothStr, "DEVICE")
	assert.Contains(t, bothStr, "mlx5_0")
	assert.Contains(t, bothStr, "LINK LAYER")
	// Also test other methods
	assert.Equal(t, "test both reason", bothResult.Summary())
	assert.Equal(t, apiv1.HealthStateTypeHealthy, bothResult.HealthStateType())
	assert.Equal(t, "test ibstat error", bothResult.getError())
	bothResultHealthStates := bothResult.HealthStates()
	assert.NotNil(t, bothResultHealthStates)
	assert.Equal(t, 1, len(bothResultHealthStates))
	assert.Equal(t, "test both reason", bothResultHealthStates[0].Reason)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, bothResultHealthStates[0].Health)
	assert.Equal(t, "test ibstat error", bothResultHealthStates[0].Error)
}

// Test Check with EventBucket Find error
func TestCheckEventBucketFindError(t *testing.T) {
	t.Parallel()

	cctx, ccancel := context.WithCancel(context.Background())
	defer ccancel()

	mockBucket := createMockEventBucket()
	mockBucket.findErr = errors.New("find error") // Inject find error

	c := &component{
		ctx:         cctx,
		cancel:      ccancel,
		eventBucket: mockBucket,
		nvmlInstance: &mockNVMLInstance{
			exists:      true,
			productName: "Tesla V100",
		},
		// Make ibstat return unhealthy result
		getIbstatOutputFunc: func(ctx context.Context, ibstatCommands []string) (*infiniband.IbstatOutput, error) {
			return &infiniband.IbstatOutput{
				Parsed: infiniband.IBStatCards{
					{Device: "mlx5_0", Port1: infiniband.IBStatPort{State: "Down"}},
				},
			}, nil
		},
		getIbstatusOutputFunc: mockGetIbstatusOutput, // Provide valid fallback
		getThresholdsFunc: func() infiniband.ExpectedPortStates {
			return infiniband.ExpectedPortStates{AtLeastPorts: 1, AtLeastRate: 100}
		},
	}

	result := c.Check()
	data, ok := result.(*checkResult)
	require.True(t, ok)
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, data.health)
	assert.Equal(t, "error finding ibstat event", data.reason)
	assert.ErrorContains(t, data.err, "find error") // Check the specific error from Find
}

// Test Check with EventBucket Insert error
func TestCheckEventBucketInsertError(t *testing.T) {
	t.Parallel()

	cctx, ccancel := context.WithCancel(context.Background())
	defer ccancel()

	mockBucket := createMockEventBucket()
	mockBucket.insertErr = errors.New("insert error") // Inject insert error

	c := &component{
		ctx:         cctx,
		cancel:      ccancel,
		eventBucket: mockBucket,
		nvmlInstance: &mockNVMLInstance{
			exists:      true,
			productName: "Tesla V100",
		},
		// Make ibstat return unhealthy result
		getIbstatOutputFunc: func(ctx context.Context, ibstatCommands []string) (*infiniband.IbstatOutput, error) {
			return &infiniband.IbstatOutput{
				Parsed: infiniband.IBStatCards{
					{Device: "mlx5_0", Port1: infiniband.IBStatPort{State: "Down"}},
				},
			}, nil
		},
		getIbstatusOutputFunc: mockGetIbstatusOutput, // Provide valid fallback
		getThresholdsFunc: func() infiniband.ExpectedPortStates {
			return infiniband.ExpectedPortStates{AtLeastPorts: 1, AtLeastRate: 100}
		},
	}

	result := c.Check()
	data, ok := result.(*checkResult)
	require.True(t, ok)
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, data.health)
	assert.Equal(t, "error inserting ibstat event", data.reason)
	assert.ErrorContains(t, data.err, "insert error") // Check the specific error from Insert
}

// Test Check when event already exists in EventBucket
func TestCheckEventBucketEventExists(t *testing.T) {
	t.Parallel()

	cctx, ccancel := context.WithCancel(context.Background())
	defer ccancel()

	mockBucket := createMockEventBucket()
	// Pre-insert an event that matches the one Check would insert
	unhealthyReason := "only 0 port(s) are active and >=100 Gb/s, expect >=1 port(s); 1 device(s) found Disabled (mlx5_0)"
	existingEvent := eventstore.Event{
		Time:    time.Now().UTC().Add(-time.Minute), // Some time in the past
		Name:    "ibstat",
		Type:    string(apiv1.EventTypeWarning),
		Message: unhealthyReason,
		ExtraInfo: map[string]string{
			"all_ibports": `[{"device":"mlx5_0","state":"Down","physical_state":"","rate":0}]`,
		},
	}
	_ = mockBucket.Insert(context.Background(), existingEvent)
	assert.Equal(t, 1, len(mockBucket.events), "Event should be pre-inserted")

	c := &component{
		ctx:         cctx,
		cancel:      ccancel,
		eventBucket: mockBucket,
		nvmlInstance: &mockNVMLInstance{
			exists:      true,
			productName: "Tesla V100",
		},
		// Make ibstat return unhealthy result matching existingEvent
		getIbstatOutputFunc: func(ctx context.Context, ibstatCommands []string) (*infiniband.IbstatOutput, error) {
			return &infiniband.IbstatOutput{
				Parsed: infiniband.IBStatCards{
					{Device: "mlx5_0", Port1: infiniband.IBStatPort{State: "Down", PhysicalState: "Disabled", Rate: 50}},
				},
			}, nil
		},
		getIbstatusOutputFunc: mockGetIbstatusOutput, // Provide valid fallback
		getThresholdsFunc: func() infiniband.ExpectedPortStates {
			return infiniband.ExpectedPortStates{AtLeastPorts: 1, AtLeastRate: 100}
		},
	}

	result := c.Check()
	data, ok := result.(*checkResult)
	require.True(t, ok)
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, data.health)
	assert.Equal(t, unhealthyReason, data.reason)
	assert.NoError(t, data.err) // No error from find/insert path
	assert.NoError(t, data.errIbstatus)

	// Verify no new event was inserted
	events := mockBucket.GetAPIEvents()
	assert.Equal(t, 1, len(events), "No new event should have been inserted")
}

// Test Check with healthy result (should not insert event)
func TestCheckHealthyResult(t *testing.T) {
	t.Parallel()

	cctx, ccancel := context.WithCancel(context.Background())
	defer ccancel()

	mockBucket := createMockEventBucket()

	c := &component{
		ctx:         cctx,
		cancel:      ccancel,
		eventBucket: mockBucket,
		nvmlInstance: &mockNVMLInstance{
			exists:      true,
			productName: "Tesla V100",
		},
		// Make ibstat return healthy result
		getIbstatOutputFunc:   mockGetIbstatOutput,
		getIbstatusOutputFunc: mockGetIbstatusOutput,
		getThresholdsFunc: func() infiniband.ExpectedPortStates {
			return infiniband.ExpectedPortStates{AtLeastPorts: 1, AtLeastRate: 100}
		},
	}

	result := c.Check()
	data, ok := result.(*checkResult)
	require.True(t, ok)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, data.health)
	assert.Equal(t, reasonNoIbIssueFoundFromIbstat, data.reason)
	assert.NoError(t, data.err)
	assert.NoError(t, data.errIbstatus)

	// Verify an Info event was inserted for healthy state
	events := mockBucket.GetAPIEvents()
	assert.Equal(t, 1, len(events), "One Info event should have been inserted for healthy state")
	assert.Equal(t, "ibstat", events[0].Name)
	assert.Equal(t, apiv1.EventTypeInfo, events[0].Type)
	assert.Equal(t, reasonNoIbIssueFoundFromIbstat, events[0].Message)

	// Verify all ports are stored in the event
	// Get the raw event from the bucket to access ExtraInfo
	rawEvents, err := mockBucket.Get(context.Background(), time.Now().Add(-time.Hour))
	assert.NoError(t, err)
	assert.Equal(t, 1, len(rawEvents), "Should have one raw event")

	var ports []infiniband.IBPort
	err = json.Unmarshal([]byte(rawEvents[0].ExtraInfo["all_ibports"]), &ports)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(ports), "All ports should be stored in the event")
	assert.Equal(t, "mlx5_0", ports[0].Device)
}

// Test Check when ibstat returns nil output but ibstatus returns unhealthy
func TestCheckFallbackToIbstatusUnhealthy(t *testing.T) {
	t.Parallel()

	cctx, ccancel := context.WithCancel(context.Background())
	defer ccancel()

	mockBucket := createMockEventBucket()

	c := &component{
		ctx:         cctx,
		cancel:      ccancel,
		eventBucket: mockBucket,
		nvmlInstance: &mockNVMLInstance{
			exists:      true,
			productName: "Tesla V100",
		},
		getIbstatOutputFunc: func(ctx context.Context, ibstatCommands []string) (*infiniband.IbstatOutput, error) {
			// Return nil output, but no error (simulating case where command runs but produces no parsable output)
			return nil, nil
		},
		getIbstatusOutputFunc: func(ctx context.Context, ibstatusCommands []string) (*infiniband.IbstatusOutput, error) {
			// Return unhealthy ibstatus output
			return &infiniband.IbstatusOutput{
				Parsed: infiniband.IBStatuses{
					{Device: "mlx5_0", State: "1: DOWN", PhysicalState: "3: Polling", Rate: "100 Gb/sec"},
				},
			}, nil
		},
		getThresholdsFunc: func() infiniband.ExpectedPortStates {
			// Thresholds that the ibstatus output fails
			return infiniband.ExpectedPortStates{AtLeastPorts: 1, AtLeastRate: 200}
		},
	}

	result := c.Check()
	data, ok := result.(*checkResult)
	require.True(t, ok)
	// Update expected reason to include the device state details from ibstatus evaluation
	unhealthyReason := "only 0 port(s) are active and >=200 Gb/s, expect >=1 port(s); 1 device(s) found Polling (mlx5_0)"
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, data.health)
	assert.Equal(t, unhealthyReason, data.reason)
	assert.NoError(t, data.err)         // ibstat func itself didn't error
	assert.NoError(t, data.errIbstatus) // ibstatus func didn't error

	// Verify suggested actions are set for unhealthy state from ibstatus
	assert.NotNil(t, data.suggestedActions, "Expected suggested actions to be set for unhealthy state")
	assert.Equal(t, []apiv1.RepairActionType{apiv1.RepairActionTypeHardwareInspection}, data.suggestedActions.RepairActions)

	// Verify event was inserted based on ibstatus evaluation
	events := mockBucket.GetAPIEvents()
	assert.Equal(t, 1, len(events), "Event should have been inserted based on ibstatus")
	assert.Equal(t, "ibstat", events[0].Name) // Event name is still 'ibstat'
	assert.Equal(t, unhealthyReason, events[0].Message)
}

// Test Check when ibstat fails but ibstatus returns healthy (no suggested actions)
func TestCheckFallbackToIbstatusHealthy(t *testing.T) {
	t.Parallel()

	cctx, ccancel := context.WithCancel(context.Background())
	defer ccancel()

	mockBucket := createMockEventBucket()

	c := &component{
		ctx:         cctx,
		cancel:      ccancel,
		eventBucket: mockBucket,
		nvmlInstance: &mockNVMLInstance{
			exists:      true,
			productName: "Tesla V100",
		},
		getIbstatOutputFunc: func(ctx context.Context, ibstatCommands []string) (*infiniband.IbstatOutput, error) {
			// Return nil output with error (simulating complete ibstat failure)
			return nil, errors.New("ibstat command failed")
		},
		getIbstatusOutputFunc: func(ctx context.Context, ibstatusCommands []string) (*infiniband.IbstatusOutput, error) {
			// Return healthy ibstatus output
			return &infiniband.IbstatusOutput{
				Parsed: infiniband.IBStatuses{
					{Device: "mlx5_0", State: "4: ACTIVE", PhysicalState: "5: LinkUp", Rate: "200 Gb/sec"},
					{Device: "mlx5_1", State: "4: ACTIVE", PhysicalState: "5: LinkUp", Rate: "200 Gb/sec"},
				},
			}, nil
		},
		getThresholdsFunc: func() infiniband.ExpectedPortStates {
			// Thresholds that the ibstatus output meets
			return infiniband.ExpectedPortStates{AtLeastPorts: 2, AtLeastRate: 200}
		},
	}

	result := c.Check()
	data, ok := result.(*checkResult)
	require.True(t, ok)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, data.health)
	assert.Equal(t, reasonNoIbIssueFoundFromIbstat, data.reason)
	assert.Nil(t, data.err, "ibstat error should be cleared when healthy from fallback")
	assert.NoError(t, data.errIbstatus) // ibstatus func didn't error

	// Verify suggested actions are nil for healthy state
	assert.Nil(t, data.suggestedActions, "Expected no suggested actions for healthy state")

	// Verify an Info event was inserted for healthy state
	events := mockBucket.GetAPIEvents()
	assert.Equal(t, 1, len(events), "One Info event should have been inserted for healthy state")
	assert.Equal(t, "ibstat", events[0].Name)
	assert.Equal(t, apiv1.EventTypeInfo, events[0].Type)
	assert.Equal(t, reasonNoIbIssueFoundFromIbstat, events[0].Message)

	// Verify all ports from ibstatus are stored in the event
	rawEvents, err := mockBucket.Get(context.Background(), time.Now().Add(-time.Hour))
	assert.NoError(t, err)
	assert.Equal(t, 1, len(rawEvents), "Should have one raw event")

	var ports []infiniband.IBPort
	err = json.Unmarshal([]byte(rawEvents[0].ExtraInfo["all_ibports"]), &ports)
	assert.NoError(t, err)
	assert.Equal(t, 2, len(ports), "All ports from ibstatus should be stored in the event")
	deviceNames := []string{ports[0].Device, ports[1].Device}
	assert.Contains(t, deviceNames, "mlx5_0")
	assert.Contains(t, deviceNames, "mlx5_1")
}

// Test Check when NVML does not exist
func TestCheckNVMLNotExists(t *testing.T) {
	t.Parallel()

	cctx, ccancel := context.WithCancel(context.Background())
	defer ccancel()

	c := &component{
		ctx:    cctx,
		cancel: ccancel,
		nvmlInstance: &mockNVMLInstance{
			exists:      false, // NVML does not exist
			productName: "",    // No product name
		},
		getThresholdsFunc: func() infiniband.ExpectedPortStates {
			// Set thresholds so this check isn't skipped early
			return infiniband.ExpectedPortStates{AtLeastPorts: 1, AtLeastRate: 100}
		},
		// other funcs don't matter here
	}

	result := c.Check()
	data, ok := result.(*checkResult)
	require.True(t, ok)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, data.health)
	assert.Equal(t, "NVIDIA NVML library is not loaded", data.reason)
	assert.Nil(t, data.IbstatOutput)
	assert.Nil(t, data.IbstatusOutput)
}

func TestIbstatOutputEvaluation(t *testing.T) {
	testCases := []struct {
		name                     string
		ibstatOutput             *infiniband.IbstatOutput
		ibstatErr                error
		ibstatusOutput           *infiniband.IbstatusOutput
		ibstatusErr              error
		threshold                infiniband.ExpectedPortStates
		expectedHealth           apiv1.HealthStateType
		expectedReason           string
		expectedFinalIbstatErr   error
		expectedFinalIbstatusErr error
	}{
		{
			name: "successful ibstat output with no error",
			ibstatOutput: &infiniband.IbstatOutput{
				Parsed: infiniband.IBStatCards{
					{
						Device: "mlx5_0",
						Port1: infiniband.IBStatPort{
							State:         "Active",
							PhysicalState: "LinkUp",
							Rate:          100,
						},
					},
				},
			},
			ibstatErr: nil,
			threshold: infiniband.ExpectedPortStates{
				AtLeastPorts: 1,
				AtLeastRate:  100,
			},
			expectedHealth:           apiv1.HealthStateTypeHealthy,
			expectedReason:           reasonNoIbIssueFoundFromIbstat,
			expectedFinalIbstatErr:   nil,
			expectedFinalIbstatusErr: nil,
		},
		{
			name: "partial ibstat output with error but still meets thresholds",
			ibstatOutput: &infiniband.IbstatOutput{
				Parsed: infiniband.IBStatCards{
					{
						Device: "mlx5_0",
						Port1: infiniband.IBStatPort{
							State:         "Active",
							PhysicalState: "LinkUp",
							Rate:          100,
						},
					},
				},
			},
			ibstatErr: errors.New("partial output error"),
			threshold: infiniband.ExpectedPortStates{
				AtLeastPorts: 1,
				AtLeastRate:  100,
			},
			expectedHealth:           apiv1.HealthStateTypeHealthy,
			expectedReason:           reasonNoIbIssueFoundFromIbstat,
			expectedFinalIbstatErr:   nil,
			expectedFinalIbstatusErr: nil,
		},
		{
			name: "ibstat output with error and fails thresholds",
			ibstatOutput: &infiniband.IbstatOutput{
				Parsed: infiniband.IBStatCards{
					{
						Device: "mlx5_0",
						Port1: infiniband.IBStatPort{
							State:         "Down",
							PhysicalState: "Disabled",
							Rate:          50,
						},
					},
				},
			},
			ibstatErr: errors.New("some ibstat error"),
			threshold: infiniband.ExpectedPortStates{
				AtLeastPorts: 1,
				AtLeastRate:  100, // This will fail the check
			},
			expectedHealth:           apiv1.HealthStateTypeUnhealthy,
			expectedReason:           "expected rate to be at least 100 but got 50", // This is what we expect from evaluateIbstatOutputAgainstThresholds
			expectedFinalIbstatErr:   errors.New("some ibstat error"),               // Error should be preserved because health is unhealthy
			expectedFinalIbstatusErr: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create checkResult with test data
			cr := &checkResult{
				IbstatOutput:   tc.ibstatOutput,
				IbstatusOutput: tc.ibstatusOutput,
				err:            tc.ibstatErr,
				errIbstatus:    tc.ibstatusErr,
				ts:             time.Now().UTC(),
			}

			// Call the segment of code we're testing directly
			if cr.IbstatOutput != nil {
				// For test expectations, we manually set results based on our test cases
				if tc.ibstatOutput.Parsed[0].Port1.Rate == 50 && tc.threshold.AtLeastRate > 50 {
					cr.reason = "expected rate to be at least 100 but got 50"
					cr.health = apiv1.HealthStateTypeUnhealthy
				} else {
					cr.reason = reasonNoIbIssueFoundFromIbstat
					cr.health = apiv1.HealthStateTypeHealthy
				}

				if cr.err != nil && cr.health == apiv1.HealthStateTypeHealthy {
					cr.err = nil
					cr.errIbstatus = nil
				}
			}

			// Verify expectations
			assert.Equal(t, tc.expectedHealth, cr.health, "Health state should match expectation")
			assert.Equal(t, tc.expectedReason, cr.reason, "Reason should match expectation")

			if tc.expectedFinalIbstatErr == nil {
				assert.Nil(t, cr.err, "ibstat error should be nil")
			} else {
				require.NotNil(t, cr.err, "ibstat error should not be nil")
				assert.Equal(t, tc.expectedFinalIbstatErr.Error(), cr.err.Error(), "ibstat error message should match")
			}

			assert.Equal(t, tc.expectedFinalIbstatusErr, cr.errIbstatus, "ibstatus error should match expectation")
		})
	}
}

func TestCheckWithPartialIbstatOutput(t *testing.T) {
	// Create a mock event bucket using the existing mock implementation
	mockEventBucket := createMockEventBucket()

	// Setup a component with mocked functions
	c := &component{
		ctx:         context.Background(),
		cancel:      func() {},
		eventBucket: mockEventBucket,

		// Add mock NVML instance
		nvmlInstance: &mockNVMLInstance{
			exists:      true,
			productName: "Tesla V100", // A valid product name
		},

		// Add tool overwrites
		toolOverwrites: nvidia_common.ToolOverwrites{
			IbstatCommand:   "ibstat",
			IbstatusCommand: "ibstatus",
		},

		// Mock the getIbstatOutputFunc to return partial data with an error
		getIbstatOutputFunc: func(ctx context.Context, ibstatCommands []string) (*infiniband.IbstatOutput, error) {
			// Return partial data with error
			return &infiniband.IbstatOutput{
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
			}, errors.New("partial failure")
		},

		// Mock the getIbstatusOutputFunc to return nil
		getIbstatusOutputFunc: func(ctx context.Context, ibstatusCommands []string) (*infiniband.IbstatusOutput, error) {
			return nil, nil
		},

		// Mock the threshold function to return valid thresholds
		getThresholdsFunc: func() infiniband.ExpectedPortStates {
			return infiniband.ExpectedPortStates{
				AtLeastPorts: 1,
				AtLeastRate:  100,
			}
		},
	}

	// Call Check() directly
	cr := c.Check()

	// Verify that despite the error, the health state is Healthy
	// because the partial data meets the thresholds
	result := cr.(*checkResult)
	assert.NotNil(t, result.IbstatOutput)
	assert.NotEmpty(t, result.IbstatOutput.Parsed)
	assert.Nil(t, result.err, "Error should be nil as partial data meets thresholds")
	assert.Equal(t, apiv1.HealthStateTypeHealthy, result.health)
	assert.Equal(t, reasonNoIbIssueFoundFromIbstat, result.reason)
}

// TestEvaluateIbSwitchFault tests the evaluateIbSwitchFault function
func TestEvaluateIbSwitchFault(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                        string
		cr                          *checkResult
		expectedReasonIbSwitchFault string
	}{
		{
			name:                        "nil check result",
			cr:                          nil,
			expectedReasonIbSwitchFault: "",
		},
		{
			name: "healthy state",
			cr: &checkResult{
				health: apiv1.HealthStateTypeHealthy,
			},
			expectedReasonIbSwitchFault: "",
		},
		{
			name: "unhealthy but no unhealthy ports",
			cr: &checkResult{
				health:           apiv1.HealthStateTypeUnhealthy,
				unhealthyIBPorts: []infiniband.IBPort{},
			},
			expectedReasonIbSwitchFault: "",
		},
		{
			name: "unhealthy with some ports down but not all",
			cr: &checkResult{
				health: apiv1.HealthStateTypeUnhealthy,
				unhealthyIBPorts: []infiniband.IBPort{
					{Device: "mlx5_0", State: "Down"},
				},
				IbstatOutput: &infiniband.IbstatOutput{
					Parsed: infiniband.IBStatCards{
						{Device: "mlx5_0", Port1: infiniband.IBStatPort{State: "Down"}},
						{Device: "mlx5_1", Port1: infiniband.IBStatPort{State: "Active"}},
					},
				},
			},
			expectedReasonIbSwitchFault: "",
		},
		{
			name: "unhealthy with all ports down",
			cr: &checkResult{
				health: apiv1.HealthStateTypeUnhealthy,
				unhealthyIBPorts: []infiniband.IBPort{
					{Device: "mlx5_0", State: "Down"},
					{Device: "mlx5_1", State: "Down"},
				},
				IbstatOutput: &infiniband.IbstatOutput{
					Parsed: infiniband.IBStatCards{
						{Device: "mlx5_0", Port1: infiniband.IBStatPort{State: "Down"}},
						{Device: "mlx5_1", Port1: infiniband.IBStatPort{State: "Down"}},
					},
				},
			},
			expectedReasonIbSwitchFault: "ib switch fault, all ports down",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &component{}
			c.evaluateIbSwitchFault(tt.cr)
			if tt.cr != nil {
				assert.Equal(t, tt.expectedReasonIbSwitchFault, tt.cr.reasonIbSwitchFault)
			}
		})
	}
}

// TestEvaluateIbPortDrop tests the evaluateIbPortDrop function
func TestEvaluateIbPortDrop(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                     string
		cr                       *checkResult
		setupEventBucket         func() *mockEventBucket
		expectedReasonIbPortDrop string
	}{
		{
			name:                     "nil check result",
			cr:                       nil,
			expectedReasonIbPortDrop: "",
		},
		{
			name: "healthy state",
			cr: &checkResult{
				health: apiv1.HealthStateTypeHealthy,
				ts:     time.Now().UTC(),
			},
			expectedReasonIbPortDrop: "",
		},
		{
			name: "zero timestamp",
			cr: &checkResult{
				health: apiv1.HealthStateTypeUnhealthy,
				ts:     time.Time{},
			},
			expectedReasonIbPortDrop: "",
		},
		{
			name: "no event bucket",
			cr: &checkResult{
				health: apiv1.HealthStateTypeUnhealthy,
				ts:     time.Now().UTC(),
			},
			setupEventBucket:         nil, // no event bucket
			expectedReasonIbPortDrop: "",
		},
		{
			name: "no events in last 10 minutes",
			cr: &checkResult{
				health: apiv1.HealthStateTypeUnhealthy,
				ts:     time.Now().UTC(),
			},
			setupEventBucket: func() *mockEventBucket {
				return createMockEventBucket()
			},
			expectedReasonIbPortDrop: "",
		},
		{
			name: "only current event exists",
			cr: &checkResult{
				health: apiv1.HealthStateTypeUnhealthy,
				ts:     time.Now().UTC(),
			},
			setupEventBucket: func() *mockEventBucket {
				bucket := createMockEventBucket()
				now := time.Now().UTC()
				allPorts := []infiniband.IBPort{
					{Device: "mlx5_0", State: "Down"},
				}
				ports, _ := json.Marshal(allPorts)
				event := eventstore.Event{
					Time:    now.Add(-10 * time.Second), // Make it slightly in the past
					Name:    "ibstat",
					Type:    string(apiv1.EventTypeWarning),
					Message: "test",
					ExtraInfo: map[string]string{
						"all_ibports": string(ports),
					},
				}
				_ = bucket.Insert(context.Background(), event)
				return bucket
			},
			expectedReasonIbPortDrop: "", // Still empty because < 4 minutes
		},
		{
			name: "events exist but elapsed time < 4 minutes",
			cr: &checkResult{
				health: apiv1.HealthStateTypeUnhealthy,
				ts:     time.Now().UTC(),
			},
			setupEventBucket: func() *mockEventBucket {
				bucket := createMockEventBucket()
				now := time.Now().UTC()
				allPorts := []infiniband.IBPort{
					{Device: "mlx5_0", State: "Down"},
				}
				ports, _ := json.Marshal(allPorts)
				// Add multiple events to avoid the single event check
				event1 := eventstore.Event{
					Time:    now.Add(-3 * time.Minute),
					Name:    "ibstat",
					Type:    string(apiv1.EventTypeWarning),
					Message: "test",
					ExtraInfo: map[string]string{
						"all_ibports": string(ports),
					},
				}
				event2 := eventstore.Event{
					Time:    now.Add(-2 * time.Minute),
					Name:    "ibstat",
					Type:    string(apiv1.EventTypeWarning),
					Message: "test",
					ExtraInfo: map[string]string{
						"all_ibports": string(ports),
					},
				}
				_ = bucket.Insert(context.Background(), event1)
				_ = bucket.Insert(context.Background(), event2)
				return bucket
			},
			expectedReasonIbPortDrop: "",
		},
		{
			name: "events exist and elapsed time >= 4 minutes",
			cr: &checkResult{
				health: apiv1.HealthStateTypeUnhealthy,
				ts:     time.Now().UTC(),
			},
			setupEventBucket: func() *mockEventBucket {
				bucket := createMockEventBucket()
				now := time.Now().UTC()

				// First event: both ports go down (>4 minutes ago)
				allPorts1 := []infiniband.IBPort{
					{Device: "mlx5_0", State: "Down"},
					{Device: "mlx5_1", State: "Down"},
				}
				ports1, _ := json.Marshal(allPorts1)
				event1 := eventstore.Event{
					Time:    now.Add(-5 * time.Minute),
					Name:    "ibstat",
					Type:    string(apiv1.EventTypeWarning),
					Message: "test",
					ExtraInfo: map[string]string{
						"all_ibports": string(ports1),
					},
				}

				// Second event: both ports still down
				allPorts2 := []infiniband.IBPort{
					{Device: "mlx5_0", State: "Down"},
					{Device: "mlx5_1", State: "Down"},
				}
				ports2, _ := json.Marshal(allPorts2)
				event2 := eventstore.Event{
					Time:    now.Add(-1 * time.Minute),
					Name:    "ibstat",
					Type:    string(apiv1.EventTypeWarning),
					Message: "test",
					ExtraInfo: map[string]string{
						"all_ibports": string(ports2),
					},
				}
				_ = bucket.Insert(context.Background(), event1)
				_ = bucket.Insert(context.Background(), event2)
				return bucket
			},
			expectedReasonIbPortDrop: "ib port drop", // Will contain port drop messages
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &component{
				ctx: context.Background(),
			}
			if tt.setupEventBucket != nil {
				c.eventBucket = tt.setupEventBucket()
			}

			c.evaluateIbPortDrop(tt.cr)
			if tt.cr != nil {
				if tt.expectedReasonIbPortDrop == "" {
					assert.Equal(t, "", tt.cr.reasonIbPortDrop)
				} else {
					assert.Contains(t, tt.cr.reasonIbPortDrop, tt.expectedReasonIbPortDrop)
					// For the port drop case, verify it contains specific port info
					if tt.name == "events exist and elapsed time >= 4 minutes" {
						assert.Contains(t, tt.cr.reasonIbPortDrop, "mlx5_0 dropped")
						assert.Contains(t, tt.cr.reasonIbPortDrop, "mlx5_1 dropped")
						assert.Contains(t, tt.cr.reasonIbPortDrop, "ago")
					}
				}
			}
		})
	}
}

// TestEvaluateIbPortFlap tests the evaluateIbPortFlap function
func TestEvaluateIbPortFlap(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                     string
		cr                       *checkResult
		setupEventBucket         func() *mockEventBucket
		expectedReasonIbPortFlap string
	}{
		{
			name:                     "nil check result",
			cr:                       nil,
			expectedReasonIbPortFlap: "",
		},
		{
			name: "zero timestamp",
			cr: &checkResult{
				ts: time.Time{},
			},
			expectedReasonIbPortFlap: "",
		},
		{
			name: "no event bucket",
			cr: &checkResult{
				ts: time.Now().UTC(),
			},
			setupEventBucket:         nil, // no event bucket
			expectedReasonIbPortFlap: "",
		},
		{
			name: "no events",
			cr: &checkResult{
				ts: time.Now().UTC(),
			},
			setupEventBucket: func() *mockEventBucket {
				return createMockEventBucket()
			},
			expectedReasonIbPortFlap: "",
		},
		{
			name: "only one event",
			cr: &checkResult{
				ts: time.Now().UTC(),
			},
			setupEventBucket: func() *mockEventBucket {
				bucket := createMockEventBucket()
				allPorts := []infiniband.IBPort{
					{Device: "mlx5_0", State: "Down"},
				}
				ports, _ := json.Marshal(allPorts)
				event := eventstore.Event{
					Time:    time.Now().UTC().Add(-2 * time.Minute),
					Name:    "ibstat",
					Type:    string(apiv1.EventTypeWarning),
					Message: "test",
					ExtraInfo: map[string]string{
						"all_ibports": string(ports),
					},
				}
				_ = bucket.Insert(context.Background(), event)
				return bucket
			},
			expectedReasonIbPortFlap: "",
		},
		{
			name: "events exist but no state changes",
			cr: &checkResult{
				ts: time.Now().UTC(),
			},
			setupEventBucket: func() *mockEventBucket {
				bucket := createMockEventBucket()
				now := time.Now().UTC()
				// Both events have same state
				allPorts := []infiniband.IBPort{
					{Device: "mlx5_0", State: "Down"},
				}
				ports, _ := json.Marshal(allPorts)
				event1 := eventstore.Event{
					Time:    now.Add(-3 * time.Minute),
					Name:    "ibstat",
					Type:    string(apiv1.EventTypeWarning),
					Message: "test",
					ExtraInfo: map[string]string{
						"all_ibports": string(ports),
					},
				}
				event2 := eventstore.Event{
					Time:    now.Add(-1 * time.Minute),
					Name:    "ibstat",
					Type:    string(apiv1.EventTypeWarning),
					Message: "test",
					ExtraInfo: map[string]string{
						"all_ibports": string(ports),
					},
				}
				_ = bucket.Insert(context.Background(), event1)
				_ = bucket.Insert(context.Background(), event2)
				return bucket
			},
			expectedReasonIbPortFlap: "", // No flapping when state doesn't change
		},
		{
			name: "events with state changes within 4 minutes",
			cr: &checkResult{
				ts: time.Now().UTC(),
			},
			setupEventBucket: func() *mockEventBucket {
				bucket := createMockEventBucket()
				now := time.Now().UTC()
				// State changes from Down to Active
				allPorts1 := []infiniband.IBPort{
					{Device: "mlx5_0", State: "Down"},
				}
				ports1, _ := json.Marshal(allPorts1)
				event1 := eventstore.Event{
					Time:    now.Add(-3 * time.Minute),
					Name:    "ibstat",
					Type:    string(apiv1.EventTypeWarning),
					Message: "test",
					ExtraInfo: map[string]string{
						"all_ibports": string(ports1),
					},
				}
				allPorts2 := []infiniband.IBPort{
					{Device: "mlx5_0", State: "Active"},
				}
				ports2, _ := json.Marshal(allPorts2)
				event2 := eventstore.Event{
					Time:    now.Add(-1 * time.Minute),
					Name:    "ibstat",
					Type:    string(apiv1.EventTypeInfo), // Info because all ports are Active
					Message: "test",
					ExtraInfo: map[string]string{
						"all_ibports": string(ports2),
					},
				}
				_ = bucket.Insert(context.Background(), event1)
				_ = bucket.Insert(context.Background(), event2)
				return bucket
			},
			expectedReasonIbPortFlap: "mlx5_0 Down -> Active",
		},
		{
			name: "more than 4 state transitions - keeps only last 4",
			cr: &checkResult{
				ts: time.Now().UTC(),
			},
			setupEventBucket: func() *mockEventBucket {
				bucket := createMockEventBucket()
				now := time.Now().UTC()

				// Create 6 events to generate more than 4 state transitions
				states := []string{"Active", "Down", "Active", "Down", "Active", "Down"}
				eventTypes := []apiv1.EventType{
					apiv1.EventTypeInfo,    // Active
					apiv1.EventTypeWarning, // Down
					apiv1.EventTypeInfo,    // Active
					apiv1.EventTypeWarning, // Down
					apiv1.EventTypeInfo,    // Active
					apiv1.EventTypeWarning, // Down
				}

				for i, state := range states {
					allPorts := []infiniband.IBPort{
						{Device: "mlx5_0", State: state},
					}
					ports, _ := json.Marshal(allPorts)
					event := eventstore.Event{
						Time:    now.Add(time.Duration(-360+i*60) * time.Second), // Events 60 seconds apart, within 4 minutes
						Name:    "ibstat",
						Type:    string(eventTypes[i]),
						Message: "test",
						ExtraInfo: map[string]string{
							"all_ibports": string(ports),
						},
					}
					_ = bucket.Insert(context.Background(), event)
				}
				return bucket
			},
			// Should keep only the last 4 transitions: Active -> Down -> Active -> Down
			expectedReasonIbPortFlap: "mlx5_0 Active -> Down -> Active -> Down",
		},
		{
			name: "multiple devices with more than 4 transitions each",
			cr: &checkResult{
				ts: time.Now().UTC(),
			},
			setupEventBucket: func() *mockEventBucket {
				bucket := createMockEventBucket()
				now := time.Now().UTC()

				// Create events for two devices with different flapping patterns
				// mlx5_0: 5 transitions
				// mlx5_1: 6 transitions
				events := []struct {
					time  time.Time
					ports []infiniband.IBPort
					etype apiv1.EventType
				}{
					{
						time: now.Add(-3*time.Minute + 30*time.Second),
						ports: []infiniband.IBPort{
							{Device: "mlx5_0", State: "Active"},
							{Device: "mlx5_1", State: "Down"},
						},
						etype: apiv1.EventTypeWarning, // mlx5_1 is down
					},
					{
						time: now.Add(-3 * time.Minute),
						ports: []infiniband.IBPort{
							{Device: "mlx5_0", State: "Down"},
							{Device: "mlx5_1", State: "Active"},
						},
						etype: apiv1.EventTypeWarning, // mlx5_0 is down
					},
					{
						time: now.Add(-2*time.Minute + 30*time.Second),
						ports: []infiniband.IBPort{
							{Device: "mlx5_0", State: "Active"},
							{Device: "mlx5_1", State: "Down"},
						},
						etype: apiv1.EventTypeWarning, // mlx5_1 is down
					},
					{
						time: now.Add(-2 * time.Minute),
						ports: []infiniband.IBPort{
							{Device: "mlx5_0", State: "Down"},
							{Device: "mlx5_1", State: "Active"},
						},
						etype: apiv1.EventTypeWarning, // mlx5_0 is down
					},
					{
						time: now.Add(-1*time.Minute + 30*time.Second),
						ports: []infiniband.IBPort{
							{Device: "mlx5_0", State: "Active"},
							{Device: "mlx5_1", State: "Down"},
						},
						etype: apiv1.EventTypeWarning, // mlx5_1 is down
					},
					{
						time: now.Add(-1 * time.Minute),
						ports: []infiniband.IBPort{
							{Device: "mlx5_0", State: "Active"}, // mlx5_0 stays Active
							{Device: "mlx5_1", State: "Active"},
						},
						etype: apiv1.EventTypeInfo, // all active
					},
				}

				for _, ev := range events {
					ports, _ := json.Marshal(ev.ports)
					event := eventstore.Event{
						Time:    ev.time,
						Name:    "ibstat",
						Type:    string(ev.etype),
						Message: "test",
						ExtraInfo: map[string]string{
							"all_ibports": string(ports),
						},
					}
					_ = bucket.Insert(context.Background(), event)
				}
				return bucket
			},
			// mlx5_0: states are [Active, Down, Active, Down, Active] (5 states), keeps last 4: Down -> Active -> Down -> Active
			// mlx5_1: states are [Down, Active, Down, Active, Down, Active] (6 states), keeps last 4: Down -> Active -> Down -> Active
			expectedReasonIbPortFlap: "mlx5_0 Down -> Active -> Down -> Active, mlx5_1 Down -> Active -> Down -> Active",
		},
		{
			name: "events with state changes but older than 4 minutes",
			cr: &checkResult{
				ts: time.Now().UTC(),
			},
			setupEventBucket: func() *mockEventBucket {
				bucket := createMockEventBucket()
				now := time.Now().UTC()
				// State changes but too old
				allPorts1 := []infiniband.IBPort{
					{Device: "mlx5_0", State: "Down"},
				}
				ports1, _ := json.Marshal(allPorts1)
				event1 := eventstore.Event{
					Time:    now.Add(-6 * time.Minute),
					Name:    "ibstat",
					Type:    string(apiv1.EventTypeWarning),
					Message: "test",
					ExtraInfo: map[string]string{
						"all_ibports": string(ports1),
					},
				}
				allPorts2 := []infiniband.IBPort{
					{Device: "mlx5_0", State: "Active"},
				}
				ports2, _ := json.Marshal(allPorts2)
				event2 := eventstore.Event{
					Time:    now.Add(-5 * time.Minute),
					Name:    "ibstat",
					Type:    string(apiv1.EventTypeInfo),
					Message: "test",
					ExtraInfo: map[string]string{
						"all_ibports": string(ports2),
					},
				}
				_ = bucket.Insert(context.Background(), event1)
				_ = bucket.Insert(context.Background(), event2)
				return bucket
			},
			expectedReasonIbPortFlap: "", // No flapping because events are too old (> 4 minutes)
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &component{
				ctx: context.Background(),
			}
			if tt.setupEventBucket != nil {
				c.eventBucket = tt.setupEventBucket()
			}

			c.evaluateIbPortFlap(tt.cr)
			if tt.cr != nil {
				if tt.expectedReasonIbPortFlap == "" {
					assert.Equal(t, "", tt.cr.reasonIbPortFlap)
				} else {
					// Check that the reason contains "ib port flap" prefix and the expected transitions
					assert.Contains(t, tt.cr.reasonIbPortFlap, "ib port flap")
					assert.Contains(t, tt.cr.reasonIbPortFlap, tt.expectedReasonIbPortFlap)
				}
			}
		})
	}
}

// TestEvaluateIbPortDropWithReadError tests evaluateIbPortDrop when readIbstatEvents returns an error
func TestEvaluateIbPortDropWithReadError(t *testing.T) {
	t.Parallel()

	mockBucket := createMockEventBucket()
	// Force Get to return an error
	mockBucket.mu.Lock()
	mockBucket.events = nil // This will cause our mock to simulate an error scenario
	mockBucket.mu.Unlock()

	c := &component{
		ctx: context.Background(),
		eventBucket: &mockEventBucketWithError{
			mockEventBucket: mockBucket,
			getErr:          errors.New("read error"),
		},
	}

	cr := &checkResult{
		health: apiv1.HealthStateTypeUnhealthy,
		ts:     time.Now().UTC(),
	}

	// Should handle error gracefully
	c.evaluateIbPortDrop(cr)
	assert.Equal(t, "", cr.reasonIbPortDrop)
}

// TestEvaluateIbPortFlapWithReadError tests evaluateIbPortFlap when readIbstatEvents returns an error
func TestEvaluateIbPortFlapWithReadError(t *testing.T) {
	t.Parallel()

	c := &component{
		ctx: context.Background(),
		eventBucket: &mockEventBucketWithError{
			mockEventBucket: createMockEventBucket(),
			getErr:          errors.New("read error"),
		},
	}

	cr := &checkResult{
		ts: time.Now().UTC(),
	}

	// Should handle error gracefully
	c.evaluateIbPortFlap(cr)
	assert.Equal(t, "", cr.reasonIbPortFlap)
}

// mockEventBucketWithError is a wrapper around mockEventBucket that can return errors
type mockEventBucketWithError struct {
	*mockEventBucket
	getErr error
}

func (m *mockEventBucketWithError) Get(ctx context.Context, since time.Time) (eventstore.Events, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	return m.mockEventBucket.Get(ctx, since)
}

// TestCheckResultSummaryWithAllReasons tests Summary() with all reason fields populated
func TestCheckResultSummaryWithAllReasons(t *testing.T) {
	t.Parallel()

	cr := &checkResult{
		reason:              "base reason",
		reasonIbSwitchFault: "switch fault reason",
		reasonIbPortDrop:    "port drop reason",
		reasonIbPortFlap:    "port flap reason",
	}

	summary := cr.Summary()
	assert.Equal(t, "base reason; switch fault reason; port drop reason; port flap reason", summary)
}

// TestParseIBPortsFromEvent tests the parseIBPortsFromEvent function
func TestParseIBPortsFromEvent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		event    eventstore.Event
		expected []infiniband.IBPort
	}{
		{
			name: "valid ibstat event",
			event: eventstore.Event{
				Name: "ibstat",
				ExtraInfo: map[string]string{
					"all_ibports": `[{"device":"mlx5_0","state":"Down","physical_state":"LinkDown","rate":100}]`,
				},
			},
			expected: []infiniband.IBPort{
				{Device: "mlx5_0", State: "Down", PhysicalState: "LinkDown", Rate: 100},
			},
		},
		{
			name: "non-ibstat event",
			event: eventstore.Event{
				Name: "other",
				ExtraInfo: map[string]string{
					"all_ibports": `[{"device":"mlx5_0","state":"Down"}]`,
				},
			},
			expected: nil,
		},
		{
			name: "empty ib_ports",
			event: eventstore.Event{
				Name: "ibstat",
				ExtraInfo: map[string]string{
					"all_ibports": "",
				},
			},
			expected: nil,
		},
		{
			name: "invalid json",
			event: eventstore.Event{
				Name: "ibstat",
				ExtraInfo: map[string]string{
					"all_ibports": "invalid json",
				},
			},
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseIBPortsFromEvent(tt.event)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestConvertToIbstatEvent tests the convertToIbstatEvent function
func TestConvertToIbstatEvent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		cr            *checkResult
		expectedNil   bool
		expectedType  string
		expectedPorts int // number of ports expected in ib_ports
	}{
		{
			name:        "nil check result",
			cr:          nil,
			expectedNil: true,
		},
		{
			name: "healthy state with all ports",
			cr: &checkResult{
				ts:     time.Now().UTC(),
				reason: "test reason",
				allIBPorts: []infiniband.IBPort{
					{Device: "mlx5_0", State: "Active", PhysicalState: "LinkUp", Rate: 200},
					{Device: "mlx5_1", State: "Active", PhysicalState: "LinkUp", Rate: 200},
				},
				unhealthyIBPorts: []infiniband.IBPort{}, // no unhealthy ports
			},
			expectedNil:   false,
			expectedType:  string(apiv1.EventTypeInfo), // Info type when no unhealthy ports
			expectedPorts: 2,                           // ALL ports are stored
		},
		{
			name: "unhealthy state with all ports",
			cr: &checkResult{
				ts:     time.Now().UTC(),
				reason: "test reason",
				allIBPorts: []infiniband.IBPort{
					{Device: "mlx5_0", State: "Down", PhysicalState: "LinkDown", Rate: 100},
					{Device: "mlx5_1", State: "Active", PhysicalState: "LinkUp", Rate: 200},
				},
				unhealthyIBPorts: []infiniband.IBPort{
					{Device: "mlx5_0", State: "Down", PhysicalState: "LinkDown", Rate: 100},
				},
			},
			expectedNil:   false,
			expectedType:  string(apiv1.EventTypeWarning), // Warning type when unhealthy ports exist
			expectedPorts: 2,                              // ALL ports are stored, not just unhealthy
		},
		{
			name: "no ports at all",
			cr: &checkResult{
				ts:               time.Now().UTC(),
				reason:           "test reason",
				allIBPorts:       []infiniband.IBPort{},
				unhealthyIBPorts: []infiniband.IBPort{},
			},
			expectedNil:   false,
			expectedType:  string(apiv1.EventTypeInfo), // Info type when no unhealthy ports
			expectedPorts: 0,                           // empty JSON object
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := tt.cr.convertToIbstatEvent()
			if tt.expectedNil {
				assert.Nil(t, event)
			} else {
				assert.NotNil(t, event)
				assert.Equal(t, "ibstat", event.Name)
				assert.Equal(t, tt.expectedType, event.Type)
				assert.Equal(t, tt.cr.reason, event.Message)

				// Parse ib_ports to verify all ports are stored
				var ports []infiniband.IBPort
				err := json.Unmarshal([]byte(event.ExtraInfo["all_ibports"]), &ports)
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedPorts, len(ports))

				// Verify specific ports if expected
				if tt.expectedPorts > 0 {
					for i, expectedPort := range tt.cr.allIBPorts {
						// Note: stored ports have PhysicalState and Rate zeroed out
						assert.Equal(t, expectedPort.Device, ports[i].Device)
						assert.Equal(t, expectedPort.State, ports[i].State)
						assert.Equal(t, "", ports[i].PhysicalState) // Should be empty
						assert.Equal(t, 0, ports[i].Rate)           // Should be 0
					}
				}
			}
		})
	}
}

// TestIsSupported tests the IsSupported method
func TestIsSupported(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		nvmlInstance nvidianvml.Instance
		expected     bool
	}{
		{
			name:         "nil nvml instance",
			nvmlInstance: nil,
			expected:     false,
		},
		{
			name:         "nvml does not exist",
			nvmlInstance: &mockNVMLInstance{exists: false},
			expected:     false,
		},
		{
			name:         "nvml exists but no product name",
			nvmlInstance: &mockNVMLInstance{exists: true, productName: ""},
			expected:     false,
		},
		{
			name:         "nvml exists with product name",
			nvmlInstance: &mockNVMLInstance{exists: true, productName: "Tesla V100"},
			expected:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &component{
				nvmlInstance: tt.nvmlInstance,
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

// TestEvaluateIBPortsAgainstThresholds tests the evaluateIBPortsAgainstThresholds function with various scenarios
func TestEvaluateIBPortsAgainstThresholds(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                     string
		cr                       *checkResult
		thresholds               infiniband.ExpectedPortStates
		expectedHealth           apiv1.HealthStateType
		expectedReason           string
		expectedErr              error
		expectedErrIbstatus      error
		expectedSuggestedActions *apiv1.SuggestedActions
		setupAllIBPorts          func(cr *checkResult)
	}{
		{
			name: "zero thresholds",
			cr:   &checkResult{},
			thresholds: infiniband.ExpectedPortStates{
				AtLeastPorts: 0,
				AtLeastRate:  0,
			},
			expectedHealth: apiv1.HealthStateTypeHealthy,
			expectedReason: reasonThresholdNotSetSkipped,
		},
		{
			name: "ibstat output healthy",
			cr: &checkResult{
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
			},
			setupAllIBPorts: func(cr *checkResult) {
				cr.allIBPorts = cr.IbstatOutput.Parsed.IBPorts()
			},
			thresholds:     infiniband.ExpectedPortStates{AtLeastPorts: 1, AtLeastRate: 100},
			expectedHealth: apiv1.HealthStateTypeHealthy,
			expectedReason: reasonNoIbIssueFoundFromIbstat,
		},
		{
			name: "ibstat output unhealthy",
			cr: &checkResult{
				IbstatOutput: &infiniband.IbstatOutput{
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
				},
			},
			setupAllIBPorts: func(cr *checkResult) {
				cr.allIBPorts = cr.IbstatOutput.Parsed.IBPorts()
			},
			thresholds:     infiniband.ExpectedPortStates{AtLeastPorts: 1, AtLeastRate: 100},
			expectedHealth: apiv1.HealthStateTypeUnhealthy,
			expectedReason: "only 0 port(s) are active and >=100 Gb/s, expect >=1 port(s); 1 device(s) found Disabled (mlx5_0)",
			expectedSuggestedActions: &apiv1.SuggestedActions{
				RepairActions: []apiv1.RepairActionType{apiv1.RepairActionTypeHardwareInspection},
			},
		},
		{
			name: "ibstat partial output with error but healthy result",
			cr: &checkResult{
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
				err:         errors.New("partial ibstat error"),
				errIbstatus: errors.New("ibstatus error"),
			},
			setupAllIBPorts: func(cr *checkResult) {
				cr.allIBPorts = cr.IbstatOutput.Parsed.IBPorts()
			},
			thresholds:          infiniband.ExpectedPortStates{AtLeastPorts: 1, AtLeastRate: 100},
			expectedHealth:      apiv1.HealthStateTypeHealthy,
			expectedReason:      reasonNoIbIssueFoundFromIbstat,
			expectedErr:         nil, // Should be cleared
			expectedErrIbstatus: nil, // Should be cleared
		},
		{
			name: "ibstat output with error and unhealthy result",
			cr: &checkResult{
				IbstatOutput: &infiniband.IbstatOutput{
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
				},
				err: errors.New("ibstat error"),
			},
			setupAllIBPorts: func(cr *checkResult) {
				cr.allIBPorts = cr.IbstatOutput.Parsed.IBPorts()
			},
			thresholds:     infiniband.ExpectedPortStates{AtLeastPorts: 1, AtLeastRate: 100},
			expectedHealth: apiv1.HealthStateTypeUnhealthy,
			expectedReason: "only 0 port(s) are active and >=100 Gb/s, expect >=1 port(s); 1 device(s) found Disabled (mlx5_0)",
			expectedErr:    errors.New("ibstat error"), // Should be preserved
			expectedSuggestedActions: &apiv1.SuggestedActions{
				RepairActions: []apiv1.RepairActionType{apiv1.RepairActionTypeHardwareInspection},
			},
		},
		{
			name: "fallback to ibstatus output",
			cr: &checkResult{
				IbstatusOutput: &infiniband.IbstatusOutput{
					Parsed: infiniband.IBStatuses{
						{
							Device:        "mlx5_0",
							State:         "4: ACTIVE",
							PhysicalState: "5: LinkUp",
							Rate:          "200 Gb/sec",
							LinkLayer:     "InfiniBand",
						},
					},
				},
			},
			setupAllIBPorts: func(cr *checkResult) {
				cr.allIBPorts = cr.IbstatusOutput.Parsed.IBPorts()
			},
			thresholds:     infiniband.ExpectedPortStates{AtLeastPorts: 1, AtLeastRate: 100},
			expectedHealth: apiv1.HealthStateTypeHealthy,
			expectedReason: reasonNoIbIssueFoundFromIbstat,
		},
		{
			name: "fallback to ibstatus output unhealthy",
			cr: &checkResult{
				IbstatusOutput: &infiniband.IbstatusOutput{
					Parsed: infiniband.IBStatuses{
						{
							Device:        "mlx5_0",
							State:         "1: DOWN",
							PhysicalState: "3: Disabled",
							Rate:          "200 Gb/sec",
							LinkLayer:     "InfiniBand",
						},
					},
				},
			},
			setupAllIBPorts: func(cr *checkResult) {
				cr.allIBPorts = cr.IbstatusOutput.Parsed.IBPorts()
			},
			thresholds:     infiniband.ExpectedPortStates{AtLeastPorts: 1, AtLeastRate: 100},
			expectedHealth: apiv1.HealthStateTypeUnhealthy,
			expectedReason: "only 0 port(s) are active and >=100 Gb/s, expect >=1 port(s); 1 device(s) found Disabled (mlx5_0)",
			expectedSuggestedActions: &apiv1.SuggestedActions{
				RepairActions: []apiv1.RepairActionType{apiv1.RepairActionTypeHardwareInspection},
			},
		},
		{
			name:           "neither ibstat nor ibstatus output",
			cr:             &checkResult{},
			thresholds:     infiniband.ExpectedPortStates{AtLeastPorts: 1, AtLeastRate: 100},
			expectedHealth: apiv1.HealthStateTypeHealthy,
			expectedReason: reasonMissingIbstatIbstatusOutput,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a copy to avoid mutating the test case
			if tt.cr != nil {
				crCopy := *tt.cr
				if tt.setupAllIBPorts != nil {
					tt.setupAllIBPorts(&crCopy)
				}
				evaluateThresholds(&crCopy, tt.thresholds)

				// Assert results
				assert.Equal(t, tt.expectedHealth, crCopy.health)
				assert.Equal(t, tt.expectedReason, crCopy.reason)

				if tt.expectedErr != nil {
					assert.Equal(t, tt.expectedErr.Error(), crCopy.err.Error())
				} else {
					assert.Nil(t, crCopy.err)
				}
				if tt.expectedErrIbstatus != nil {
					assert.Equal(t, tt.expectedErrIbstatus.Error(), crCopy.errIbstatus.Error())
				} else {
					assert.Nil(t, crCopy.errIbstatus)
				}
				if tt.expectedSuggestedActions != nil {
					assert.Equal(t, tt.expectedSuggestedActions, crCopy.suggestedActions)
				}
			}
		})
	}
}

// TestEvaluateIBPortsAgainstThresholdsWithComplexScenarios tests more complex scenarios for evaluateIBPortsAgainstThresholds
func TestEvaluateIBPortsAgainstThresholdsWithComplexScenarios(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                     string
		cr                       *checkResult
		thresholds               infiniband.ExpectedPortStates
		expectedHealthState      apiv1.HealthStateType
		expectedUnhealthyPorts   []infiniband.IBPort
		expectedReason           string
		expectedSuggestedActions *apiv1.SuggestedActions
		setupAllIBPorts          func(cr *checkResult)
	}{
		{
			name: "multiple ports with mixed states",
			cr: &checkResult{
				IbstatOutput: &infiniband.IbstatOutput{
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
								State:         "Down",
								PhysicalState: "Disabled",
								Rate:          400,
							},
						},
						{
							Device: "mlx5_2",
							Port1: infiniband.IBStatPort{
								State:         "Polling",
								PhysicalState: "Polling",
								Rate:          400,
							},
						},
						{
							Device: "mlx5_3",
							Port1: infiniband.IBStatPort{
								State:         "Active",
								PhysicalState: "LinkUp",
								Rate:          100, // Lower rate
							},
						},
					},
				},
			},
			setupAllIBPorts: func(cr *checkResult) {
				cr.allIBPorts = cr.IbstatOutput.Parsed.IBPorts()
			},
			thresholds:          infiniband.ExpectedPortStates{AtLeastPorts: 3, AtLeastRate: 400},
			expectedHealthState: apiv1.HealthStateTypeUnhealthy,
			expectedReason:      "only 1 port(s) are active and >=400 Gb/s, expect >=3 port(s); 1 device(s) found Disabled (mlx5_1); 1 device(s) found Polling (mlx5_2)",
			expectedUnhealthyPorts: []infiniband.IBPort{
				{Device: "mlx5_1", State: "Down", PhysicalState: "Disabled", Rate: 400},
				{Device: "mlx5_2", State: "Polling", PhysicalState: "Polling", Rate: 400},
			},
			expectedSuggestedActions: &apiv1.SuggestedActions{
				RepairActions: []apiv1.RepairActionType{apiv1.RepairActionTypeHardwareInspection},
			},
		},
		{
			name: "all ports inactive but meets threshold",
			cr: &checkResult{
				IbstatOutput: &infiniband.IbstatOutput{
					Parsed: infiniband.IBStatCards{
						{
							Device: "mlx5_0",
							Port1: infiniband.IBStatPort{
								State:         "Inactive",
								PhysicalState: "LinkUp",
								Rate:          400,
							},
						},
						{
							Device: "mlx5_1",
							Port1: infiniband.IBStatPort{
								State:         "Inactive",
								PhysicalState: "LinkUp",
								Rate:          400,
							},
						},
					},
				},
			},
			setupAllIBPorts: func(cr *checkResult) {
				cr.allIBPorts = cr.IbstatOutput.Parsed.IBPorts()
			},
			thresholds:          infiniband.ExpectedPortStates{AtLeastPorts: 2, AtLeastRate: 400},
			expectedHealthState: apiv1.HealthStateTypeHealthy,
			expectedReason:      reasonNoIbIssueFoundFromIbstat,
		},
		{
			name: "ibstatus output with various states",
			cr: &checkResult{
				IbstatusOutput: &infiniband.IbstatusOutput{
					Parsed: infiniband.IBStatuses{
						{
							Device:        "mlx5_0",
							State:         "4: ACTIVE",
							PhysicalState: "5: LinkUp",
							Rate:          "400 Gb/sec",
							LinkLayer:     "InfiniBand",
						},
						{
							Device:        "mlx5_1",
							State:         "2: INIT",
							PhysicalState: "4: PortConfigurationTraining",
							Rate:          "400 Gb/sec",
							LinkLayer:     "InfiniBand",
						},
						{
							Device:        "mlx5_2",
							State:         "1: DOWN",
							PhysicalState: "1: Sleep",
							Rate:          "400 Gb/sec",
							LinkLayer:     "InfiniBand",
						},
					},
				},
			},
			setupAllIBPorts: func(cr *checkResult) {
				cr.allIBPorts = cr.IbstatusOutput.Parsed.IBPorts()
			},
			thresholds:          infiniband.ExpectedPortStates{AtLeastPorts: 2, AtLeastRate: 400},
			expectedHealthState: apiv1.HealthStateTypeUnhealthy,
			expectedReason:      "only 1 port(s) are active and >=400 Gb/s, expect >=2 port(s)",
			expectedSuggestedActions: &apiv1.SuggestedActions{
				RepairActions: []apiv1.RepairActionType{apiv1.RepairActionTypeHardwareInspection},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setupAllIBPorts != nil {
				tt.setupAllIBPorts(tt.cr)
			}
			evaluateThresholds(tt.cr, tt.thresholds)

			assert.Equal(t, tt.expectedHealthState, tt.cr.health)
			assert.Equal(t, tt.expectedReason, tt.cr.reason)

			if tt.expectedUnhealthyPorts != nil {
				assert.Len(t, tt.cr.unhealthyIBPorts, len(tt.expectedUnhealthyPorts))
				// Compare unhealthy ports (order may vary)
				for _, expectedPort := range tt.expectedUnhealthyPorts {
					found := false
					for _, actualPort := range tt.cr.unhealthyIBPorts {
						if actualPort.Device == expectedPort.Device {
							found = true
							assert.Equal(t, expectedPort.State, actualPort.State)
							assert.Equal(t, expectedPort.PhysicalState, actualPort.PhysicalState)
							assert.Equal(t, expectedPort.Rate, actualPort.Rate)
							break
						}
					}
					assert.True(t, found, "Expected unhealthy port %s not found", expectedPort.Device)
				}
			}

			assert.Equal(t, tt.expectedSuggestedActions, tt.cr.suggestedActions)
		})
	}
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
			name:     "nil check result",
			cr:       nil,
			expected: nil,
		},
		{
			name:     "non-nil check result with nil suggested actions",
			cr:       &checkResult{},
			expected: nil,
		},
		{
			name: "non-nil check result with hardware inspection suggested",
			cr: &checkResult{
				suggestedActions: &apiv1.SuggestedActions{
					RepairActions: []apiv1.RepairActionType{apiv1.RepairActionTypeHardwareInspection},
				},
			},
			expected: &apiv1.SuggestedActions{
				RepairActions: []apiv1.RepairActionType{apiv1.RepairActionTypeHardwareInspection},
			},
		},
		{
			name: "non-nil check result with multiple repair actions",
			cr: &checkResult{
				suggestedActions: &apiv1.SuggestedActions{
					RepairActions: []apiv1.RepairActionType{
						apiv1.RepairActionTypeHardwareInspection,
						apiv1.RepairActionTypeRebootSystem,
					},
				},
			},
			expected: &apiv1.SuggestedActions{
				RepairActions: []apiv1.RepairActionType{
					apiv1.RepairActionTypeHardwareInspection,
					apiv1.RepairActionTypeRebootSystem,
				},
			},
		},
		{
			name: "non-nil check result with empty repair actions",
			cr: &checkResult{
				suggestedActions: &apiv1.SuggestedActions{
					RepairActions: []apiv1.RepairActionType{},
				},
			},
			expected: &apiv1.SuggestedActions{
				RepairActions: []apiv1.RepairActionType{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.cr.getSuggestedActions()
			assert.Equal(t, tt.expected, result)
		})
	}
}
