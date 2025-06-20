package gpucounts

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/NVIDIA/go-nvlib/pkg/nvlib/device"
	"github.com/NVIDIA/go-nvml/pkg/nvml/mock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	nvidianvml "github.com/leptonai/gpud/pkg/nvidia-query/nvml"
	nvmllib "github.com/leptonai/gpud/pkg/nvidia-query/nvml/lib"
	"github.com/leptonai/gpud/pkg/nvidia-query/nvml/testutil"
)

// mockNVMLInstance implements the nvidianvml.Instance interface for testing
type mockNVMLInstance struct {
	devices      map[string]device.Device
	nvmlExists   bool
	productName  string
	architecture string
	brand        string
}

func (m *mockNVMLInstance) NVMLExists() bool {
	return m.nvmlExists
}

func (m *mockNVMLInstance) Library() nvmllib.Library {
	return nil
}

func (m *mockNVMLInstance) Devices() map[string]device.Device {
	return m.devices
}

func (m *mockNVMLInstance) ProductName() string {
	return m.productName
}

func (m *mockNVMLInstance) Architecture() string {
	return m.architecture
}

func (m *mockNVMLInstance) Brand() string {
	return m.brand
}

func (m *mockNVMLInstance) DriverVersion() string {
	return "test-driver-version"
}

func (m *mockNVMLInstance) DriverMajor() int {
	return 1
}

func (m *mockNVMLInstance) CUDAVersion() string {
	return "test-cuda-version"
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

// TestNew tests the New function
func TestNew(t *testing.T) {
	ctx := context.Background()
	mockDevices := map[string]device.Device{
		"test-uuid": testutil.NewMockDevice(&mock.Device{}, "test-arch", "test-brand", "1.0", "0000:00:00.0"),
	}
	mockInstance := &mockNVMLInstance{
		devices:     mockDevices,
		nvmlExists:  true,
		productName: "test-product",
	}

	gpudInstance := &components.GPUdInstance{
		RootCtx:      ctx,
		NVMLInstance: mockInstance,
	}
	comp, err := New(gpudInstance)
	require.NoError(t, err)

	c, ok := comp.(*component)
	require.True(t, ok)
	assert.Equal(t, mockInstance, c.nvmlInstance)
	assert.NotNil(t, c.getCountLspci)
	assert.NotNil(t, c.getThresholdsFunc)
	assert.NotNil(t, c.ctx)
	assert.NotNil(t, c.cancel)

	// Test that getCountLspci works correctly
	count, err := c.getCountLspci(ctx)
	// It may fail if lspci is not available on the test system
	if err == nil {
		assert.GreaterOrEqual(t, count, 0)
	}
}

// TestComponent_Name tests the Name method
func TestComponent_Name(t *testing.T) {
	c := &component{}
	assert.Equal(t, Name, c.Name())
}

// TestComponent_Tags tests the Tags method
func TestComponent_Tags(t *testing.T) {
	c := &component{}

	expectedTags := []string{
		"accelerator",
		"gpu",
		"nvidia",
		Name,
	}

	tags := c.Tags()
	assert.Equal(t, expectedTags, tags)
	assert.Len(t, tags, 4)
}

// TestComponent_IsSupported tests the IsSupported method
func TestComponent_IsSupported(t *testing.T) {
	tests := []struct {
		name      string
		component *component
		expected  bool
	}{
		{
			name: "nil nvml instance",
			component: &component{
				nvmlInstance: nil,
			},
			expected: false,
		},
		{
			name: "nvml not exists",
			component: &component{
				nvmlInstance: &mockNVMLInstance{
					nvmlExists: false,
				},
			},
			expected: false,
		},
		{
			name: "empty product name",
			component: &component{
				nvmlInstance: &mockNVMLInstance{
					nvmlExists:  true,
					productName: "",
				},
			},
			expected: false,
		},
		{
			name: "supported",
			component: &component{
				nvmlInstance: &mockNVMLInstance{
					nvmlExists:  true,
					productName: "test-product",
				},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.component.IsSupported()
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestComponent_Start tests the Start method
func TestComponent_Start(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mockDevices := map[string]device.Device{
		"test-uuid": testutil.NewMockDevice(&mock.Device{}, "test-arch", "test-brand", "1.0", "0000:00:00.0"),
	}
	mockInstance := &mockNVMLInstance{
		devices:     mockDevices,
		nvmlExists:  true,
		productName: "test-product",
	}

	c := &component{
		ctx:          ctx,
		cancel:       cancel,
		nvmlInstance: mockInstance,
		getCountLspci: func(ctx context.Context) (int, error) {
			return 1, nil
		},
		getThresholdsFunc: func() ExpectedGPUCounts {
			return ExpectedGPUCounts{Count: 0}
		},
	}

	err := c.Start()
	assert.NoError(t, err)

	// Allow the goroutine time to initialize
	time.Sleep(100 * time.Millisecond)
}

// TestComponent_LastHealthStates tests the LastHealthStates method
func TestComponent_LastHealthStates(t *testing.T) {
	ctx := context.Background()

	// Test when lastCheckResult is nil
	c := &component{
		ctx: ctx,
	}

	states := c.LastHealthStates()
	assert.Len(t, states, 1)
	assert.Equal(t, "no data yet", states[0].Reason)

	// Test with valid data
	c.lastCheckResult = &checkResult{
		ProductName: "test-product",
		CountLspci:  2,
		CountNVML:   2,
		ts:          time.Now(),
		health:      apiv1.HealthStateTypeHealthy,
		reason:      "all good",
	}

	states = c.LastHealthStates()
	assert.Len(t, states, 1)
	assert.Equal(t, "all good", states[0].Reason)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, states[0].Health)
	assert.Equal(t, "test-product", states[0].ExtraInfo["product_name"])
	assert.Equal(t, "2", states[0].ExtraInfo["count_lspci"])
	assert.Equal(t, "2", states[0].ExtraInfo["count_nvml"])
}

// TestComponent_Events tests the Events method
func TestComponent_Events(t *testing.T) {
	ctx := context.Background()
	c := &component{
		ctx: ctx,
	}

	events, err := c.Events(ctx, time.Now().Add(-time.Hour))
	assert.NoError(t, err)
	assert.Nil(t, events)
}

// TestComponent_Close tests the Close method
func TestComponent_Close(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	c := &component{
		ctx:    ctx,
		cancel: cancel,
	}

	err := c.Close()
	assert.NoError(t, err)
}

// TestComponent_Check_NilNVML tests the Check method with nil NVML instance
func TestComponent_Check_NilNVML(t *testing.T) {
	ctx := context.Background()
	c := &component{
		ctx:          ctx,
		nvmlInstance: nil,
		getThresholdsFunc: func() ExpectedGPUCounts {
			return ExpectedGPUCounts{Count: 1}
		},
	}

	result := c.Check()
	cr, ok := result.(*checkResult)
	require.True(t, ok)

	assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
	assert.Equal(t, "NVIDIA NVML instance is nil", cr.reason)
	assert.Nil(t, cr.err)
}

// TestComponent_Check_NVMLNotLoaded tests the Check method when NVML is not loaded
func TestComponent_Check_NVMLNotLoaded(t *testing.T) {
	ctx := context.Background()
	c := &component{
		ctx: ctx,
		nvmlInstance: &mockNVMLInstance{
			nvmlExists: false,
		},
		getThresholdsFunc: func() ExpectedGPUCounts {
			return ExpectedGPUCounts{Count: 1}
		},
	}

	result := c.Check()
	cr, ok := result.(*checkResult)
	require.True(t, ok)

	assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
	assert.Equal(t, "NVIDIA NVML library is not loaded", cr.reason)
	assert.Nil(t, cr.err)
}

// TestComponent_Check_EmptyProductName tests the Check method when product name is empty
func TestComponent_Check_EmptyProductName(t *testing.T) {
	ctx := context.Background()
	c := &component{
		ctx: ctx,
		nvmlInstance: &mockNVMLInstance{
			nvmlExists:  true,
			productName: "",
		},
		getThresholdsFunc: func() ExpectedGPUCounts {
			return ExpectedGPUCounts{Count: 1}
		},
	}

	result := c.Check()
	cr, ok := result.(*checkResult)
	require.True(t, ok)

	assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
	assert.Equal(t, "NVIDIA NVML is loaded but GPU is not detected (missing product name)", cr.reason)
	assert.Nil(t, cr.err)
}

// TestComponent_Check_Success tests the Check method for successful case
func TestComponent_Check_Success(t *testing.T) {
	ctx := context.Background()

	mockDevices := map[string]device.Device{
		"test-uuid-1": testutil.NewMockDevice(&mock.Device{}, "test-arch", "test-brand", "1.0", "0000:00:00.0"),
		"test-uuid-2": testutil.NewMockDevice(&mock.Device{}, "test-arch", "test-brand", "1.0", "0000:00:01.0"),
	}

	c := &component{
		ctx: ctx,
		nvmlInstance: &mockNVMLInstance{
			devices:     mockDevices,
			nvmlExists:  true,
			productName: "test-product",
		},
		getCountLspci: func(ctx context.Context) (int, error) {
			return 2, nil
		},
		getThresholdsFunc: func() ExpectedGPUCounts {
			return ExpectedGPUCounts{Count: 2}
		},
	}

	result := c.Check()
	cr, ok := result.(*checkResult)
	require.True(t, ok)

	assert.Equal(t, "test-product", cr.ProductName)
	assert.Equal(t, 2, cr.CountLspci)
	assert.Equal(t, 2, cr.CountNVML)
	assert.Nil(t, cr.err)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
	assert.Equal(t, "nvidia gpu count matching thresholds (2)", cr.reason)

	// Check that lastCheckResult was updated
	assert.NotNil(t, c.lastCheckResult)
	assert.Equal(t, cr, c.lastCheckResult)
}

// TestComponent_Check_LspciError tests the Check method when lspci fails
func TestComponent_Check_LspciError(t *testing.T) {
	ctx := context.Background()
	testErr := errors.New("lspci error")

	mockDevices := map[string]device.Device{
		"test-uuid": testutil.NewMockDevice(&mock.Device{}, "test-arch", "test-brand", "1.0", "0000:00:00.0"),
	}

	c := &component{
		ctx: ctx,
		nvmlInstance: &mockNVMLInstance{
			devices:     mockDevices,
			nvmlExists:  true,
			productName: "test-product",
		},
		getCountLspci: func(ctx context.Context) (int, error) {
			return 0, testErr
		},
		getThresholdsFunc: func() ExpectedGPUCounts {
			return ExpectedGPUCounts{Count: 1}
		},
	}

	result := c.Check()
	cr, ok := result.(*checkResult)
	require.True(t, ok)

	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
	assert.Equal(t, "error getting count of lspci", cr.reason)
	assert.Equal(t, testErr, cr.err)
}

// TestComponent_Check_ThresholdNotSet tests the Check method when threshold is not set
func TestComponent_Check_ThresholdNotSet(t *testing.T) {
	ctx := context.Background()

	mockDevices := map[string]device.Device{
		"test-uuid": testutil.NewMockDevice(&mock.Device{}, "test-arch", "test-brand", "1.0", "0000:00:00.0"),
	}

	c := &component{
		ctx: ctx,
		nvmlInstance: &mockNVMLInstance{
			devices:     mockDevices,
			nvmlExists:  true,
			productName: "test-product",
		},
		getCountLspci: func(ctx context.Context) (int, error) {
			return 1, nil
		},
		getThresholdsFunc: func() ExpectedGPUCounts {
			return ExpectedGPUCounts{Count: 0} // Zero threshold
		},
	}

	result := c.Check()
	cr, ok := result.(*checkResult)
	require.True(t, ok)

	assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
	assert.Equal(t, reasonThresholdNotSetSkipped, cr.reason)
}

// TestComponent_Check_LspciCountMismatch tests when lspci count doesn't match threshold
func TestComponent_Check_LspciCountMismatch(t *testing.T) {
	ctx := context.Background()

	mockDevices := map[string]device.Device{
		"test-uuid-1": testutil.NewMockDevice(&mock.Device{}, "test-arch", "test-brand", "1.0", "0000:00:00.0"),
		"test-uuid-2": testutil.NewMockDevice(&mock.Device{}, "test-arch", "test-brand", "1.0", "0000:00:01.0"),
	}

	c := &component{
		ctx: ctx,
		nvmlInstance: &mockNVMLInstance{
			devices:     mockDevices,
			nvmlExists:  true,
			productName: "test-product",
		},
		getCountLspci: func(ctx context.Context) (int, error) {
			return 1, nil // lspci reports 1 GPU
		},
		getThresholdsFunc: func() ExpectedGPUCounts {
			return ExpectedGPUCounts{Count: 2} // Expecting 2 GPUs
		},
	}

	result := c.Check()
	cr, ok := result.(*checkResult)
	require.True(t, ok)

	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
	assert.Equal(t, "nvidia gpu count mismatch (lspci 1, expected 2)", cr.reason)
	assert.Nil(t, cr.err)
	assert.Equal(t, 1, cr.CountLspci)
	assert.Equal(t, 2, cr.CountNVML)
}

// TestComponent_Check_NVMLCountMismatch tests when NVML count doesn't match threshold
func TestComponent_Check_NVMLCountMismatch(t *testing.T) {
	ctx := context.Background()

	mockDevices := map[string]device.Device{
		"test-uuid": testutil.NewMockDevice(&mock.Device{}, "test-arch", "test-brand", "1.0", "0000:00:00.0"),
	}

	c := &component{
		ctx: ctx,
		nvmlInstance: &mockNVMLInstance{
			devices:     mockDevices,
			nvmlExists:  true,
			productName: "test-product",
		},
		getCountLspci: func(ctx context.Context) (int, error) {
			return 2, nil // lspci reports 2 GPUs
		},
		getThresholdsFunc: func() ExpectedGPUCounts {
			return ExpectedGPUCounts{Count: 2} // Expecting 2 GPUs
		},
	}

	result := c.Check()
	cr, ok := result.(*checkResult)
	require.True(t, ok)

	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
	assert.Equal(t, "nvidia gpu count mismatch (nvml 1, expected 2)", cr.reason)
	assert.Nil(t, cr.err)
	assert.Equal(t, 2, cr.CountLspci)
	assert.Equal(t, 1, cr.CountNVML) // Only 1 device in mockDevices
}

// TestComponent_Check_NVMLZeroDevices tests when NVML reports zero devices
func TestComponent_Check_NVMLZeroDevices(t *testing.T) {
	ctx := context.Background()

	c := &component{
		ctx: ctx,
		nvmlInstance: &mockNVMLInstance{
			devices:     map[string]device.Device{}, // Empty devices map
			nvmlExists:  true,
			productName: "test-product",
		},
		getCountLspci: func(ctx context.Context) (int, error) {
			return 2, nil // lspci reports 2 GPUs
		},
		getThresholdsFunc: func() ExpectedGPUCounts {
			return ExpectedGPUCounts{Count: 2} // Expecting 2 GPUs
		},
	}

	result := c.Check()
	cr, ok := result.(*checkResult)
	require.True(t, ok)

	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
	assert.Equal(t, "nvidia gpu count mismatch (nvml 0, expected 2)", cr.reason)
	assert.Nil(t, cr.err)
	assert.Equal(t, 2, cr.CountLspci)
	assert.Equal(t, 0, cr.CountNVML)
}

// TestCheckResult_String tests the String method of checkResult
func TestCheckResult_String(t *testing.T) {
	// Test nil checkResult
	var nilCR *checkResult
	str := nilCR.String()
	assert.Empty(t, str)

	// Test empty checkResult
	emptyCR := &checkResult{}
	str = emptyCR.String()
	assert.Equal(t, "no data", str)

	// Test with data
	cr := &checkResult{
		ProductName: "test-product",
		CountLspci:  2,
		CountNVML:   3,
	}

	str = cr.String()
	assert.Contains(t, str, "test-product")
	assert.Contains(t, str, "2")
	assert.Contains(t, str, "3")
}

// TestCheckResult_Summary tests the Summary method of checkResult
func TestCheckResult_Summary(t *testing.T) {
	// Test nil checkResult
	var nilCR *checkResult
	summary := nilCR.Summary()
	assert.Empty(t, summary)

	// Test with reason
	cr := &checkResult{
		reason: "test reason",
	}
	summary = cr.Summary()
	assert.Equal(t, "test reason", summary)
}

// TestCheckResult_HealthStateType tests the HealthStateType method of checkResult
func TestCheckResult_HealthStateType(t *testing.T) {
	// Test nil checkResult
	var nilCR *checkResult
	health := nilCR.HealthStateType()
	assert.Empty(t, health)

	// Test with health state
	cr := &checkResult{
		health: apiv1.HealthStateTypeUnhealthy,
	}
	health = cr.HealthStateType()
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, health)
}

// TestCheckResult_GetSuggestedActions tests the getSuggestedActions method
func TestCheckResult_GetSuggestedActions(t *testing.T) {
	// Test nil checkResult
	var nilCR *checkResult
	actions := nilCR.getSuggestedActions()
	assert.Nil(t, actions)

	// Test with no suggested actions
	cr := &checkResult{}
	actions = cr.getSuggestedActions()
	assert.Nil(t, actions)

	// Test with suggested actions
	expectedActions := &apiv1.SuggestedActions{
		Description:   "Test issue description",
		RepairActions: []apiv1.RepairActionType{apiv1.RepairActionTypeRebootSystem, apiv1.RepairActionTypeHardwareInspection},
	}
	cr = &checkResult{
		suggestedActions: expectedActions,
	}
	actions = cr.getSuggestedActions()
	assert.Equal(t, expectedActions, actions)
}

// TestCheckResult_GetError tests the getError method
func TestCheckResult_GetError(t *testing.T) {
	// Test nil checkResult
	var nilCR *checkResult
	errStr := nilCR.getError()
	assert.Empty(t, errStr)

	// Test with no error
	cr := &checkResult{}
	errStr = cr.getError()
	assert.Empty(t, errStr)

	// Test with error
	testErr := errors.New("test error")
	cr = &checkResult{
		err: testErr,
	}
	errStr = cr.getError()
	assert.Equal(t, "test error", errStr)
}

// TestCheckResult_ComponentName tests the ComponentName method
func TestCheckResult_ComponentName(t *testing.T) {
	cr := &checkResult{}
	assert.Equal(t, Name, cr.ComponentName())
}

// TestCheckResult_HealthStates tests the HealthStates method
func TestCheckResult_HealthStates(t *testing.T) {
	// Test nil checkResult
	var nilCR *checkResult
	states := nilCR.HealthStates()
	assert.Len(t, states, 1)
	assert.Equal(t, Name, states[0].Name)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, states[0].Health)
	assert.Equal(t, "no data yet", states[0].Reason)

	// Test with data
	testTime := time.Now()
	testErr := errors.New("test error")
	suggestedActions := &apiv1.SuggestedActions{
		Description:   "Test issue description",
		RepairActions: []apiv1.RepairActionType{apiv1.RepairActionTypeCheckUserAppAndGPU},
	}

	cr := &checkResult{
		ProductName:      "test-product",
		CountLspci:       2,
		CountNVML:        3,
		ts:               testTime,
		health:           apiv1.HealthStateTypeUnhealthy,
		reason:           "test reason",
		err:              testErr,
		suggestedActions: suggestedActions,
	}

	states = cr.HealthStates()
	assert.Len(t, states, 1)

	state := states[0]
	assert.Equal(t, Name, state.Name)
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, state.Health)
	assert.Equal(t, "test reason", state.Reason)
	assert.Equal(t, "test error", state.Error)
	assert.Equal(t, suggestedActions, state.SuggestedActions)

	// Check ExtraInfo
	assert.Equal(t, "test-product", state.ExtraInfo["product_name"])
	assert.Equal(t, "2", state.ExtraInfo["count_lspci"])
	assert.Equal(t, "3", state.ExtraInfo["count_nvml"])
}

// TestExpectedGPUCounts_IsZero tests the IsZero method from threshold.go
func TestExpectedGPUCounts_IsZero(t *testing.T) {
	// Test nil
	var nilEC *ExpectedGPUCounts
	assert.True(t, nilEC.IsZero())

	// Test zero count
	ec := &ExpectedGPUCounts{Count: 0}
	assert.True(t, ec.IsZero())

	// Test negative count
	ec = &ExpectedGPUCounts{Count: -1}
	assert.True(t, ec.IsZero())

	// Test positive count
	ec = &ExpectedGPUCounts{Count: 1}
	assert.False(t, ec.IsZero())
}

// TestDefaultExpectedGPUCounts tests the Get/Set functions for default thresholds
func TestDefaultExpectedGPUCounts(t *testing.T) {
	// Test initial default
	initial := GetDefaultExpectedGPUCounts()
	assert.Equal(t, 0, initial.Count)

	// Test setting new default
	newCounts := ExpectedGPUCounts{Count: 4}
	SetDefaultExpectedGPUCounts(newCounts)

	retrieved := GetDefaultExpectedGPUCounts()
	assert.Equal(t, 4, retrieved.Count)

	// Reset to original
	SetDefaultExpectedGPUCounts(ExpectedGPUCounts{Count: 0})
}

// TestComponent_Integration tests the full lifecycle of the component
func TestComponent_Integration(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create mock devices
	mockDevices := map[string]device.Device{
		"test-uuid-1": testutil.NewMockDevice(&mock.Device{}, "test-arch", "test-brand", "1.0", "0000:00:00.0"),
		"test-uuid-2": testutil.NewMockDevice(&mock.Device{}, "test-arch", "test-brand", "1.0", "0000:00:01.0"),
	}

	mockInstance := &mockNVMLInstance{
		devices:     mockDevices,
		nvmlExists:  true,
		productName: "test-product",
	}

	// Create GPUd instance
	gpudInstance := &components.GPUdInstance{
		RootCtx:      ctx,
		NVMLInstance: mockInstance,
	}

	// Create component
	comp, err := New(gpudInstance)
	require.NoError(t, err)

	// Test component properties
	assert.Equal(t, Name, comp.Name())
	assert.True(t, comp.IsSupported())
	assert.Len(t, comp.Tags(), 4)

	// Start the component
	err = comp.Start()
	assert.NoError(t, err)

	// Allow the goroutine to run at least one check
	time.Sleep(200 * time.Millisecond)

	// Check health states
	states := comp.LastHealthStates()
	assert.Len(t, states, 1)

	// Ensure the check has run
	if states[0].Reason != "no data yet" {
		assert.Equal(t, apiv1.HealthStateTypeHealthy, states[0].Health)
		assert.Equal(t, reasonThresholdNotSetSkipped, states[0].Reason)
	}

	// Check events (should be nil for this component)
	events, err := comp.Events(ctx, time.Now().Add(-time.Hour))
	assert.NoError(t, err)
	assert.Nil(t, events)

	// Close the component
	err = comp.Close()
	assert.NoError(t, err)
}

// TestComponent_ConcurrentAccess tests concurrent access to the component
func TestComponent_ConcurrentAccess(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mockDevices := map[string]device.Device{
		"test-uuid": testutil.NewMockDevice(&mock.Device{}, "test-arch", "test-brand", "1.0", "0000:00:00.0"),
	}

	c := &component{
		ctx: ctx,
		nvmlInstance: &mockNVMLInstance{
			devices:     mockDevices,
			nvmlExists:  true,
			productName: "test-product",
		},
		getCountLspci: func(ctx context.Context) (int, error) {
			return 1, nil
		},
		getThresholdsFunc: func() ExpectedGPUCounts {
			return ExpectedGPUCounts{Count: 1}
		},
	}

	// Run multiple checks concurrently
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = c.Check()
			_ = c.LastHealthStates()
		}()
	}

	wg.Wait()

	// Verify the final state
	states := c.LastHealthStates()
	assert.Len(t, states, 1)
}

// TestComponent_Check_NilLspciFunc tests when getCountLspci is nil
func TestComponent_Check_NilLspciFunc(t *testing.T) {
	ctx := context.Background()

	mockDevices := map[string]device.Device{
		"test-uuid": testutil.NewMockDevice(&mock.Device{}, "test-arch", "test-brand", "1.0", "0000:00:00.0"),
	}

	c := &component{
		ctx: ctx,
		nvmlInstance: &mockNVMLInstance{
			devices:     mockDevices,
			nvmlExists:  true,
			productName: "test-product",
		},
		getCountLspci: nil, // Explicitly set to nil
		getThresholdsFunc: func() ExpectedGPUCounts {
			return ExpectedGPUCounts{Count: 1}
		},
	}

	result := c.Check()
	cr, ok := result.(*checkResult)
	require.True(t, ok)

	// When getCountLspci is nil, CountLspci will be 0, which doesn't match threshold
	assert.Equal(t, "test-product", cr.ProductName)
	assert.Equal(t, 0, cr.CountLspci) // Should be zero since getCountLspci is nil
	assert.Equal(t, 1, cr.CountNVML)
	assert.Nil(t, cr.err)
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
	assert.Equal(t, "nvidia gpu count mismatch (lspci 0, expected 1)", cr.reason)
}
