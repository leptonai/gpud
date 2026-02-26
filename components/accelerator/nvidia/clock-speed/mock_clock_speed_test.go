//go:build linux

package clockspeed

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/NVIDIA/go-nvml/pkg/nvml/mock"
	"github.com/bytedance/mockey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	nvmlerrors "github.com/leptonai/gpud/pkg/nvidia/errors"
	"github.com/leptonai/gpud/pkg/nvidia/nvml/device"
	"github.com/leptonai/gpud/pkg/nvidia/nvml/lib"
	"github.com/leptonai/gpud/pkg/nvidia/nvml/testutil"
	nvidiaproduct "github.com/leptonai/gpud/pkg/nvidia/product"
)

// customMockNVMLInstanceCS implements the nvml.Instance interface for testing with customizable behavior
type customMockNVMLInstanceCS struct {
	devs        map[string]device.Device
	nvmlExists  bool
	productName string
	initError   error
}

func (m *customMockNVMLInstanceCS) Devices() map[string]device.Device { return m.devs }
func (m *customMockNVMLInstanceCS) FabricManagerSupported() bool      { return true }
func (m *customMockNVMLInstanceCS) FabricStateSupported() bool        { return false }
func (m *customMockNVMLInstanceCS) GetMemoryErrorManagementCapabilities() nvidiaproduct.MemoryErrorManagementCapabilities {
	return nvidiaproduct.MemoryErrorManagementCapabilities{}
}
func (m *customMockNVMLInstanceCS) ProductName() string   { return m.productName }
func (m *customMockNVMLInstanceCS) Architecture() string  { return "" }
func (m *customMockNVMLInstanceCS) Brand() string         { return "" }
func (m *customMockNVMLInstanceCS) DriverVersion() string { return "" }
func (m *customMockNVMLInstanceCS) DriverMajor() int      { return 0 }
func (m *customMockNVMLInstanceCS) CUDAVersion() string   { return "" }
func (m *customMockNVMLInstanceCS) NVMLExists() bool      { return m.nvmlExists }
func (m *customMockNVMLInstanceCS) Library() lib.Library  { return nil }
func (m *customMockNVMLInstanceCS) Shutdown() error       { return nil }
func (m *customMockNVMLInstanceCS) InitError() error      { return m.initError }

// createMockClockSpeedComponent creates a component with mocked functions for testing
func createMockClockSpeedComponent(
	ctx context.Context,
	nvmlInstance *customMockNVMLInstanceCS,
	getClockSpeedFunc func(uuid string, dev device.Device) (ClockSpeed, error),
) *component {
	cctx, cancel := context.WithCancel(ctx)

	return &component{
		ctx:    cctx,
		cancel: cancel,
		getTimeNowFunc: func() time.Time {
			return time.Now().UTC()
		},
		nvmlInstance:      nvmlInstance,
		getClockSpeedFunc: getClockSpeedFunc,
	}
}

// TestNew_WithMockey tests the New function using mockey for isolation
func TestNew_WithMockey(t *testing.T) {
	mockey.PatchConvey("New component creation", t, func() {
		ctx := context.Background()
		mockInstance := &customMockNVMLInstanceCS{
			devs:        map[string]device.Device{},
			nvmlExists:  true,
			productName: "NVIDIA H100",
		}

		gpudInstance := &components.GPUdInstance{
			RootCtx:      ctx,
			NVMLInstance: mockInstance,
		}

		c, err := New(gpudInstance)

		assert.NoError(t, err)
		assert.NotNil(t, c)
		assert.Equal(t, Name, c.Name())

		tc, ok := c.(*component)
		require.True(t, ok)
		assert.NotNil(t, tc.getClockSpeedFunc)
		assert.NotNil(t, tc.getTimeNowFunc)
	})
}

// TestComponent_IsSupported_WithMockey tests IsSupported method with various conditions
func TestComponent_IsSupported_WithMockey(t *testing.T) {
	testCases := []struct {
		name         string
		setupNilNVML bool
		nvmlExists   bool
		productName  string
		expected     bool
	}{
		{
			name:         "nil NVML instance returns false",
			setupNilNVML: true,
			expected:     false,
		},
		{
			name:        "NVML not loaded returns false",
			nvmlExists:  false,
			productName: "NVIDIA H100",
			expected:    false,
		},
		{
			name:        "no product name returns false",
			nvmlExists:  true,
			productName: "",
			expected:    false,
		},
		{
			name:        "NVML loaded with product name returns true",
			nvmlExists:  true,
			productName: "NVIDIA H100",
			expected:    true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockey.PatchConvey(tc.name, t, func() {
				ctx := context.Background()
				cctx, cancel := context.WithCancel(ctx)
				defer cancel()

				var comp *component
				if tc.setupNilNVML {
					comp = &component{
						ctx:          cctx,
						cancel:       cancel,
						nvmlInstance: nil,
					}
				} else {
					customMock := &customMockNVMLInstanceCS{
						devs:        map[string]device.Device{},
						nvmlExists:  tc.nvmlExists,
						productName: tc.productName,
					}
					comp = &component{
						ctx:          cctx,
						cancel:       cancel,
						nvmlInstance: customMock,
					}
				}

				result := comp.IsSupported()
				assert.Equal(t, tc.expected, result)
			})
		})
	}
}

// TestCheck_InitError_WithMockey tests Check when NVML has an initialization error
func TestCheck_InitError_WithMockey(t *testing.T) {
	mockey.PatchConvey("Check with NVML init error", t, func() {
		ctx := context.Background()

		initErr := errors.New("error getting device handle for index '0': Unknown Error")
		mockInst := &customMockNVMLInstanceCS{
			devs:        map[string]device.Device{},
			nvmlExists:  true,
			productName: "NVIDIA H100",
			initError:   initErr,
		}

		comp := createMockClockSpeedComponent(ctx, mockInst, nil)

		result := comp.Check()
		cr, ok := result.(*checkResult)
		require.True(t, ok)

		assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
		assert.Contains(t, cr.reason, "NVML initialization error")
		assert.NotNil(t, cr.suggestedActions)
		assert.Contains(t, cr.suggestedActions.RepairActions, apiv1.RepairActionTypeRebootSystem)
	})
}

// TestCheck_MissingProductName_WithMockey tests Check when product name is empty
func TestCheck_MissingProductName_WithMockey(t *testing.T) {
	mockey.PatchConvey("Check with missing product name", t, func() {
		ctx := context.Background()

		mockInst := &customMockNVMLInstanceCS{
			devs:        map[string]device.Device{},
			nvmlExists:  true,
			productName: "",
		}

		comp := createMockClockSpeedComponent(ctx, mockInst, nil)

		result := comp.Check()
		cr, ok := result.(*checkResult)
		require.True(t, ok)

		assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
		assert.Contains(t, cr.reason, "missing product name")
	})
}

// TestCheck_NilNVMLInstance_WithMockey tests Check with nil NVML instance
func TestCheck_NilNVMLInstance_WithMockey(t *testing.T) {
	mockey.PatchConvey("Check with nil NVML instance", t, func() {
		ctx := context.Background()
		cctx, cancel := context.WithCancel(ctx)
		defer cancel()

		comp := &component{
			ctx:    cctx,
			cancel: cancel,
			getTimeNowFunc: func() time.Time {
				return time.Now().UTC()
			},
			nvmlInstance: nil,
		}

		result := comp.Check()
		cr, ok := result.(*checkResult)
		require.True(t, ok)

		assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
		assert.Contains(t, cr.reason, "NVIDIA NVML instance is nil")
	})
}

// TestCheck_NVMLNotExists_WithMockey tests Check when NVML library is not loaded
func TestCheck_NVMLNotExists_WithMockey(t *testing.T) {
	mockey.PatchConvey("Check with NVML not exists", t, func() {
		ctx := context.Background()

		mockInst := &customMockNVMLInstanceCS{
			devs:        map[string]device.Device{},
			nvmlExists:  false,
			productName: "NVIDIA H100",
		}

		comp := createMockClockSpeedComponent(ctx, mockInst, nil)

		result := comp.Check()
		cr, ok := result.(*checkResult)
		require.True(t, ok)

		assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
		assert.Contains(t, cr.reason, "NVIDIA NVML library is not loaded")
	})
}

// TestCheck_GPULostError_WithMockey tests Check when getClockSpeed returns ErrGPULost
func TestCheck_GPULostError_WithMockey(t *testing.T) {
	mockey.PatchConvey("Check with GPU lost error", t, func() {
		ctx := context.Background()

		uuid := "gpu-uuid-lost"
		mockDeviceObj := &mock.Device{
			GetUUIDFunc: func() (string, nvml.Return) {
				return uuid, nvml.SUCCESS
			},
		}
		mockDev := testutil.NewMockDevice(mockDeviceObj, "test-arch", "test-brand", "test-cuda", "test-pci")

		devs := map[string]device.Device{
			uuid: mockDev,
		}

		mockNvml := &customMockNVMLInstanceCS{
			devs:        devs,
			nvmlExists:  true,
			productName: "NVIDIA H100",
		}

		getClockSpeedFunc := func(uuid string, dev device.Device) (ClockSpeed, error) {
			return ClockSpeed{}, nvmlerrors.ErrGPULost
		}

		comp := createMockClockSpeedComponent(ctx, mockNvml, getClockSpeedFunc)
		result := comp.Check()
		cr, ok := result.(*checkResult)
		require.True(t, ok)

		assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
		assert.Equal(t, nvmlerrors.ErrGPULost.Error(), cr.reason)
		assert.NotNil(t, cr.suggestedActions)
		assert.Contains(t, cr.suggestedActions.RepairActions, apiv1.RepairActionTypeRebootSystem)
	})
}

// TestCheck_GPURequiresResetError_WithMockey tests Check when getClockSpeed returns ErrGPURequiresReset
func TestCheck_GPURequiresResetError_WithMockey(t *testing.T) {
	mockey.PatchConvey("Check with GPU requires reset error", t, func() {
		ctx := context.Background()

		uuid := "gpu-uuid-reset"
		mockDeviceObj := &mock.Device{
			GetUUIDFunc: func() (string, nvml.Return) {
				return uuid, nvml.SUCCESS
			},
		}
		mockDev := testutil.NewMockDevice(mockDeviceObj, "test-arch", "test-brand", "test-cuda", "test-pci")

		devs := map[string]device.Device{
			uuid: mockDev,
		}

		mockNvml := &customMockNVMLInstanceCS{
			devs:        devs,
			nvmlExists:  true,
			productName: "NVIDIA H100",
		}

		getClockSpeedFunc := func(uuid string, dev device.Device) (ClockSpeed, error) {
			return ClockSpeed{}, nvmlerrors.ErrGPURequiresReset
		}

		comp := createMockClockSpeedComponent(ctx, mockNvml, getClockSpeedFunc)
		result := comp.Check()
		cr, ok := result.(*checkResult)
		require.True(t, ok)

		assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
		assert.Equal(t, nvmlerrors.ErrGPURequiresReset.Error(), cr.reason)
		assert.NotNil(t, cr.suggestedActions)
		assert.Contains(t, cr.suggestedActions.RepairActions, apiv1.RepairActionTypeRebootSystem)
	})
}

// TestCheck_GenericClockSpeedError_WithMockey tests Check with a generic clock speed error
func TestCheck_GenericClockSpeedError_WithMockey(t *testing.T) {
	mockey.PatchConvey("Check with generic clock speed error", t, func() {
		ctx := context.Background()

		uuid := "gpu-uuid-generic-error"
		mockDeviceObj := &mock.Device{
			GetUUIDFunc: func() (string, nvml.Return) {
				return uuid, nvml.SUCCESS
			},
		}
		mockDev := testutil.NewMockDevice(mockDeviceObj, "test-arch", "test-brand", "test-cuda", "test-pci")

		devs := map[string]device.Device{
			uuid: mockDev,
		}

		mockNvml := &customMockNVMLInstanceCS{
			devs:        devs,
			nvmlExists:  true,
			productName: "NVIDIA H100",
		}

		genericErr := errors.New("some generic clock speed error")
		getClockSpeedFunc := func(uuid string, dev device.Device) (ClockSpeed, error) {
			return ClockSpeed{}, genericErr
		}

		comp := createMockClockSpeedComponent(ctx, mockNvml, getClockSpeedFunc)
		result := comp.Check()
		cr, ok := result.(*checkResult)
		require.True(t, ok)

		assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
		assert.Equal(t, "error getting clock speed", cr.reason)
		assert.Equal(t, genericErr, cr.err)
		// No suggested actions for generic errors
		assert.Nil(t, cr.suggestedActions)
	})
}

// TestCheck_MultipleGPUs_WithMockey tests Check with multiple GPUs
func TestCheck_MultipleGPUs_WithMockey(t *testing.T) {
	mockey.PatchConvey("Check with multiple GPUs", t, func() {
		ctx := context.Background()

		uuid1 := "gpu-uuid-1"
		uuid2 := "gpu-uuid-2"

		mockDeviceObj1 := &mock.Device{
			GetUUIDFunc: func() (string, nvml.Return) {
				return uuid1, nvml.SUCCESS
			},
		}
		mockDev1 := testutil.NewMockDevice(mockDeviceObj1, "test-arch", "test-brand", "test-cuda", "test-pci-1")

		mockDeviceObj2 := &mock.Device{
			GetUUIDFunc: func() (string, nvml.Return) {
				return uuid2, nvml.SUCCESS
			},
		}
		mockDev2 := testutil.NewMockDevice(mockDeviceObj2, "test-arch", "test-brand", "test-cuda", "test-pci-2")

		devs := map[string]device.Device{
			uuid1: mockDev1,
			uuid2: mockDev2,
		}

		mockNvml := &customMockNVMLInstanceCS{
			devs:        devs,
			nvmlExists:  true,
			productName: "NVIDIA H100",
		}

		getClockSpeedFunc := func(uuid string, dev device.Device) (ClockSpeed, error) {
			return ClockSpeed{
				UUID:                   uuid,
				GraphicsMHz:            1500,
				MemoryMHz:              5000,
				ClockGraphicsSupported: true,
				ClockMemorySupported:   true,
			}, nil
		}

		comp := createMockClockSpeedComponent(ctx, mockNvml, getClockSpeedFunc)
		result := comp.Check()
		cr, ok := result.(*checkResult)
		require.True(t, ok)

		assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
		assert.Contains(t, cr.reason, "2 GPU(s) were checked")
		assert.Len(t, cr.ClockSpeeds, 2)
	})
}

// TestCheck_ConcurrentAccess_WithMockey tests concurrent access to Check and LastHealthStates
func TestCheck_ConcurrentAccess_WithMockey(t *testing.T) {
	mockey.PatchConvey("Concurrent Check and LastHealthStates access", t, func() {
		ctx := context.Background()

		uuid := "gpu-uuid-concurrent"
		mockDeviceObj := &mock.Device{
			GetUUIDFunc: func() (string, nvml.Return) {
				return uuid, nvml.SUCCESS
			},
		}
		mockDev := testutil.NewMockDevice(mockDeviceObj, "test-arch", "test-brand", "test-cuda", "test-pci")

		devs := map[string]device.Device{
			uuid: mockDev,
		}

		mockNvml := &customMockNVMLInstanceCS{
			devs:        devs,
			nvmlExists:  true,
			productName: "NVIDIA H100",
		}

		getClockSpeedFunc := func(uuid string, dev device.Device) (ClockSpeed, error) {
			return ClockSpeed{
				UUID:                   uuid,
				GraphicsMHz:            1500,
				MemoryMHz:              5000,
				ClockGraphicsSupported: true,
				ClockMemorySupported:   true,
			}, nil
		}

		comp := createMockClockSpeedComponent(ctx, mockNvml, getClockSpeedFunc)

		// Run concurrent access
		done := make(chan bool, 10)
		for i := 0; i < 5; i++ {
			go func() {
				comp.Check()
				done <- true
			}()
			go func() {
				_ = comp.LastHealthStates()
				done <- true
			}()
		}

		// Wait for all goroutines
		for i := 0; i < 10; i++ {
			<-done
		}

		// Verify final state is consistent
		states := comp.LastHealthStates()
		assert.Len(t, states, 1)
	})
}

// TestCheckResult_Methods_WithMockey tests all checkResult methods
func TestCheckResult_Methods_WithMockey(t *testing.T) {
	t.Run("ComponentName", func(t *testing.T) {
		cr := &checkResult{}
		assert.Equal(t, Name, cr.ComponentName())
	})

	t.Run("String with nil", func(t *testing.T) {
		var cr *checkResult
		assert.Equal(t, "", cr.String())
	})

	t.Run("String with empty data", func(t *testing.T) {
		cr := &checkResult{}
		assert.Equal(t, "no data", cr.String())
	})

	t.Run("String with clock speeds", func(t *testing.T) {
		cr := &checkResult{
			ClockSpeeds: []ClockSpeed{
				{
					UUID:                   "gpu-1",
					BusID:                  "0000:01:00.0",
					GraphicsMHz:            1500,
					MemoryMHz:              5000,
					ClockGraphicsSupported: true,
					ClockMemorySupported:   true,
				},
			},
		}
		result := cr.String()
		assert.Contains(t, result, "gpu-1")
		assert.Contains(t, result, "0000:01:00.0")
		assert.Contains(t, result, "1500 MHz")
		assert.Contains(t, result, "5000 MHz")
	})

	t.Run("Summary with nil", func(t *testing.T) {
		var cr *checkResult
		assert.Equal(t, "", cr.Summary())
	})

	t.Run("Summary with reason", func(t *testing.T) {
		cr := &checkResult{reason: "test reason"}
		assert.Equal(t, "test reason", cr.Summary())
	})

	t.Run("HealthStateType with nil", func(t *testing.T) {
		var cr *checkResult
		assert.Equal(t, apiv1.HealthStateType(""), cr.HealthStateType())
	})

	t.Run("HealthStateType healthy", func(t *testing.T) {
		cr := &checkResult{health: apiv1.HealthStateTypeHealthy}
		assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.HealthStateType())
	})

	t.Run("getError with nil", func(t *testing.T) {
		var cr *checkResult
		assert.Equal(t, "", cr.getError())
	})

	t.Run("getError with nil error", func(t *testing.T) {
		cr := &checkResult{err: nil}
		assert.Equal(t, "", cr.getError())
	})

	t.Run("getError with error", func(t *testing.T) {
		cr := &checkResult{err: errors.New("test error")}
		assert.Equal(t, "test error", cr.getError())
	})

	t.Run("HealthStates with nil", func(t *testing.T) {
		var cr *checkResult
		states := cr.HealthStates()
		require.Len(t, states, 1)
		assert.Equal(t, apiv1.HealthStateTypeHealthy, states[0].Health)
		assert.Equal(t, "no data yet", states[0].Reason)
	})

	t.Run("HealthStates with clock speeds", func(t *testing.T) {
		cr := &checkResult{
			ts:     time.Now(),
			health: apiv1.HealthStateTypeHealthy,
			reason: "all good",
			ClockSpeeds: []ClockSpeed{
				{
					UUID:        "gpu-1",
					GraphicsMHz: 1500,
					MemoryMHz:   5000,
				},
			},
		}

		states := cr.HealthStates()
		require.Len(t, states, 1)
		assert.NotEmpty(t, states[0].ExtraInfo)
		assert.Contains(t, states[0].ExtraInfo["data"], "gpu-1")
	})

	t.Run("HealthStates with suggested actions", func(t *testing.T) {
		cr := &checkResult{
			ts:     time.Now(),
			health: apiv1.HealthStateTypeUnhealthy,
			reason: "GPU lost",
			err:    nvmlerrors.ErrGPULost,
			suggestedActions: &apiv1.SuggestedActions{
				Description: "GPU lost",
				RepairActions: []apiv1.RepairActionType{
					apiv1.RepairActionTypeRebootSystem,
				},
			},
		}

		states := cr.HealthStates()
		require.Len(t, states, 1)
		assert.NotNil(t, states[0].SuggestedActions)
		assert.Contains(t, states[0].SuggestedActions.RepairActions, apiv1.RepairActionTypeRebootSystem)
	})
}

// TestStart_WithMockey tests the Start method
func TestStart_WithMockey(t *testing.T) {
	mockey.PatchConvey("Start method launches goroutine", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		uuid := "gpu-uuid-start"
		mockDeviceObj := &mock.Device{
			GetUUIDFunc: func() (string, nvml.Return) {
				return uuid, nvml.SUCCESS
			},
		}
		mockDev := testutil.NewMockDevice(mockDeviceObj, "test-arch", "test-brand", "test-cuda", "test-pci")

		devs := map[string]device.Device{
			uuid: mockDev,
		}

		mockNvml := &customMockNVMLInstanceCS{
			devs:        devs,
			nvmlExists:  true,
			productName: "NVIDIA H100",
		}

		checkCalled := make(chan bool, 1)
		getClockSpeedFunc := func(uuid string, dev device.Device) (ClockSpeed, error) {
			select {
			case checkCalled <- true:
			default:
			}
			return ClockSpeed{
				UUID:                   uuid,
				GraphicsMHz:            1500,
				MemoryMHz:              5000,
				ClockGraphicsSupported: true,
				ClockMemorySupported:   true,
			}, nil
		}

		comp := createMockClockSpeedComponent(ctx, mockNvml, getClockSpeedFunc)
		err := comp.Start()
		assert.NoError(t, err)

		// Wait for at least one check to be called
		select {
		case <-checkCalled:
			// Success - Check was called
		case <-time.After(2 * time.Second):
			t.Fatal("Check was not called within timeout")
		}
	})
}

// TestClose_WithMockey tests the Close method
func TestClose_WithMockey(t *testing.T) {
	mockey.PatchConvey("Close cancels context", t, func() {
		ctx := context.Background()
		cctx, cancel := context.WithCancel(ctx)

		comp := &component{
			ctx:    cctx,
			cancel: cancel,
		}

		err := comp.Close()
		assert.NoError(t, err)

		// Verify context is canceled
		select {
		case <-comp.ctx.Done():
			// Expected - context is canceled
		default:
			t.Fatal("context was not canceled after Close")
		}
	})
}

// TestEvents_WithMockey tests the Events method
func TestEvents_WithMockey(t *testing.T) {
	mockey.PatchConvey("Events returns nil", t, func() {
		ctx := context.Background()
		cctx, cancel := context.WithCancel(ctx)
		defer cancel()

		comp := &component{
			ctx:    cctx,
			cancel: cancel,
		}

		events, err := comp.Events(ctx, time.Now().Add(-1*time.Hour))
		assert.NoError(t, err)
		assert.Nil(t, events)
	})
}

// TestTags_WithMockey tests the Tags method
func TestTags_WithMockey(t *testing.T) {
	mockey.PatchConvey("Tags returns expected values", t, func() {
		ctx := context.Background()
		cctx, cancel := context.WithCancel(ctx)
		defer cancel()

		comp := &component{
			ctx:    cctx,
			cancel: cancel,
		}

		tags := comp.Tags()
		assert.Equal(t, []string{"accelerator", "gpu", "nvidia", Name}, tags)
		assert.Len(t, tags, 4)
	})
}

// TestName_WithMockey tests the Name method
func TestName_WithMockey(t *testing.T) {
	mockey.PatchConvey("Name returns expected value", t, func() {
		ctx := context.Background()
		cctx, cancel := context.WithCancel(ctx)
		defer cancel()

		comp := &component{
			ctx:    cctx,
			cancel: cancel,
		}

		assert.Equal(t, Name, comp.Name())
		assert.Equal(t, "accelerator-nvidia-clock-speed", comp.Name())
	})
}

// TestLastHealthStates_NoDataYet_WithMockey tests LastHealthStates when no check has been performed
func TestLastHealthStates_NoDataYet_WithMockey(t *testing.T) {
	mockey.PatchConvey("LastHealthStates with no data yet", t, func() {
		ctx := context.Background()
		cctx, cancel := context.WithCancel(ctx)
		defer cancel()

		comp := &component{
			ctx:    cctx,
			cancel: cancel,
		}

		states := comp.LastHealthStates()
		require.Len(t, states, 1)
		assert.Equal(t, apiv1.HealthStateTypeHealthy, states[0].Health)
		assert.Equal(t, "no data yet", states[0].Reason)
		assert.Equal(t, Name, states[0].Component)
		assert.Equal(t, Name, states[0].Name)
	})
}

// TestLastHealthStates_SuggestedActionsPropagate_WithMockey tests suggested actions propagation
func TestLastHealthStates_SuggestedActionsPropagate_WithMockey(t *testing.T) {
	mockey.PatchConvey("Suggested actions propagate to health states", t, func() {
		ctx := context.Background()

		uuid := "gpu-uuid-123"
		mockDeviceObj := &mock.Device{
			GetUUIDFunc: func() (string, nvml.Return) {
				return uuid, nvml.SUCCESS
			},
		}
		mockDev := testutil.NewMockDevice(mockDeviceObj, "test-arch", "test-brand", "test-cuda", "test-pci")

		devs := map[string]device.Device{
			uuid: mockDev,
		}

		mockNvml := &customMockNVMLInstanceCS{
			devs:        devs,
			nvmlExists:  true,
			productName: "NVIDIA H100",
		}

		// Simulate GPU requires reset error
		getClockSpeedFunc := func(uuid string, dev device.Device) (ClockSpeed, error) {
			return ClockSpeed{}, nvmlerrors.ErrGPURequiresReset
		}

		comp := createMockClockSpeedComponent(ctx, mockNvml, getClockSpeedFunc)
		comp.Check()

		states := comp.LastHealthStates()
		require.Len(t, states, 1)
		assert.Equal(t, apiv1.HealthStateTypeUnhealthy, states[0].Health)
		assert.NotNil(t, states[0].SuggestedActions)
		assert.Contains(t, states[0].SuggestedActions.RepairActions, apiv1.RepairActionTypeRebootSystem)
	})
}

// TestCheck_NoDevices_WithMockey tests Check with no devices
func TestCheck_NoDevices_WithMockey(t *testing.T) {
	mockey.PatchConvey("Check with no devices", t, func() {
		ctx := context.Background()

		mockNvml := &customMockNVMLInstanceCS{
			devs:        map[string]device.Device{},
			nvmlExists:  true,
			productName: "NVIDIA H100",
		}

		comp := createMockClockSpeedComponent(ctx, mockNvml, nil)
		result := comp.Check()
		cr, ok := result.(*checkResult)
		require.True(t, ok)

		assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
		assert.Contains(t, cr.reason, "0 GPU(s) were checked")
		assert.Empty(t, cr.ClockSpeeds)
	})
}

// TestCheckResult_HealthStates_NoExtraInfo_WithMockey tests HealthStates without ExtraInfo
func TestCheckResult_HealthStates_NoExtraInfo_WithMockey(t *testing.T) {
	mockey.PatchConvey("HealthStates no extra info", t, func() {
		cr := &checkResult{
			ts:          time.Now(),
			health:      apiv1.HealthStateTypeHealthy,
			reason:      "healthy with no data",
			ClockSpeeds: nil,
		}

		states := cr.HealthStates()
		require.Len(t, states, 1)
		assert.Empty(t, states[0].ExtraInfo)
	})
}

// TestCheck_EmptyDeviceMap_WithMockey tests Check with empty device map (not nil)
func TestCheck_EmptyDeviceMap_WithMockey(t *testing.T) {
	mockey.PatchConvey("Check with empty device map", t, func() {
		ctx := context.Background()
		cctx, cancel := context.WithCancel(ctx)
		defer cancel()

		mockInst := &customMockNVMLInstanceCS{
			devs:        map[string]device.Device{},
			nvmlExists:  true,
			productName: "NVIDIA H100",
		}

		comp := &component{
			ctx:    cctx,
			cancel: cancel,
			getTimeNowFunc: func() time.Time {
				return time.Now().UTC()
			},
			nvmlInstance: mockInst,
			getClockSpeedFunc: func(uuid string, dev device.Device) (ClockSpeed, error) {
				return ClockSpeed{}, nil
			},
		}

		result := comp.Check()
		cr, ok := result.(*checkResult)
		require.True(t, ok)

		assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
		assert.Contains(t, cr.reason, "all 0 GPU(s) were checked")
	})
}

// TestLastHealthStates_RaceCondition_WithMockey tests for race conditions in LastHealthStates
func TestLastHealthStates_RaceCondition_WithMockey(t *testing.T) {
	mockey.PatchConvey("LastHealthStates race condition", t, func() {
		ctx := context.Background()

		uuid := "gpu-uuid-race"
		mockDeviceObj := &mock.Device{
			GetUUIDFunc: func() (string, nvml.Return) {
				return uuid, nvml.SUCCESS
			},
		}
		mockDev := testutil.NewMockDevice(mockDeviceObj, "test-arch", "test-brand", "test-cuda", "test-pci")

		devs := map[string]device.Device{
			uuid: mockDev,
		}

		mockNvml := &customMockNVMLInstanceCS{
			devs:        devs,
			nvmlExists:  true,
			productName: "NVIDIA H100",
		}

		getClockSpeedFunc := func(uuid string, dev device.Device) (ClockSpeed, error) {
			return ClockSpeed{
				UUID:                   uuid,
				GraphicsMHz:            1500,
				MemoryMHz:              5000,
				ClockGraphicsSupported: true,
				ClockMemorySupported:   true,
			}, nil
		}

		comp := createMockClockSpeedComponent(ctx, mockNvml, getClockSpeedFunc)

		// Run multiple concurrent reads and writes
		done := make(chan bool, 40)
		for i := 0; i < 20; i++ {
			go func() {
				comp.Check()
				done <- true
			}()
			go func() {
				_ = comp.LastHealthStates()
				done <- true
			}()
		}

		// Wait for all goroutines
		for i := 0; i < 40; i++ {
			<-done
		}

		// Should not panic and state should be consistent
		states := comp.LastHealthStates()
		assert.Len(t, states, 1)
	})
}

// TestCheckResult_HealthStates_UnhealthyWithError_WithMockey tests HealthStates with error
func TestCheckResult_HealthStates_UnhealthyWithError_WithMockey(t *testing.T) {
	mockey.PatchConvey("HealthStates with unhealthy and error", t, func() {
		testErr := errors.New("test error for health states")
		cr := &checkResult{
			ts:     time.Now(),
			health: apiv1.HealthStateTypeUnhealthy,
			reason: "something went wrong",
			err:    testErr,
		}

		states := cr.HealthStates()
		require.Len(t, states, 1)
		assert.Equal(t, apiv1.HealthStateTypeUnhealthy, states[0].Health)
		assert.Equal(t, "test error for health states", states[0].Error)
		assert.Equal(t, "something went wrong", states[0].Reason)
	})
}

// TestCheck_FirstGPUFailsSecondSucceeds_WithMockey tests Check when first GPU fails
func TestCheck_FirstGPUFailsSecondSucceeds_WithMockey(t *testing.T) {
	mockey.PatchConvey("Check when first GPU fails", t, func() {
		ctx := context.Background()

		uuid1 := "gpu-uuid-fail"
		uuid2 := "gpu-uuid-success"

		mockDeviceObj1 := &mock.Device{
			GetUUIDFunc: func() (string, nvml.Return) {
				return uuid1, nvml.SUCCESS
			},
		}
		mockDev1 := testutil.NewMockDevice(mockDeviceObj1, "test-arch", "test-brand", "test-cuda", "test-pci-1")

		mockDeviceObj2 := &mock.Device{
			GetUUIDFunc: func() (string, nvml.Return) {
				return uuid2, nvml.SUCCESS
			},
		}
		mockDev2 := testutil.NewMockDevice(mockDeviceObj2, "test-arch", "test-brand", "test-cuda", "test-pci-2")

		devs := map[string]device.Device{
			uuid1: mockDev1,
			uuid2: mockDev2,
		}

		mockNvml := &customMockNVMLInstanceCS{
			devs:        devs,
			nvmlExists:  true,
			productName: "NVIDIA H100",
		}

		callCount := 0
		getClockSpeedFunc := func(uuid string, dev device.Device) (ClockSpeed, error) {
			callCount++
			// First call fails (map iteration order is random, so we fail on the first call)
			if callCount == 1 {
				return ClockSpeed{}, nvmlerrors.ErrGPULost
			}
			return ClockSpeed{
				UUID:                   uuid,
				GraphicsMHz:            1500,
				MemoryMHz:              5000,
				ClockGraphicsSupported: true,
				ClockMemorySupported:   true,
			}, nil
		}

		comp := createMockClockSpeedComponent(ctx, mockNvml, getClockSpeedFunc)
		result := comp.Check()
		cr, ok := result.(*checkResult)
		require.True(t, ok)

		// Should be unhealthy because first GPU failed
		assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
		assert.Equal(t, nvmlerrors.ErrGPULost.Error(), cr.reason)
	})
}

// TestGetClockSpeed_GPURequiresReset_WithMockey tests GetClockSpeed with GPU requires reset error
func TestGetClockSpeed_GPURequiresReset_WithMockey(t *testing.T) {
	mockey.PatchConvey("GetClockSpeed with GPU requires reset on graphics", t, func() {
		mockDevice := &mock.Device{
			GetClockInfoFunc: func(clockType nvml.ClockType) (uint32, nvml.Return) {
				if clockType == nvml.CLOCK_GRAPHICS {
					return 0, nvml.ERROR_RESET_REQUIRED
				}
				return 5000, nvml.SUCCESS
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "GPU-TEST", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		_, err := GetClockSpeed("GPU-TEST", dev)
		assert.Error(t, err)
		assert.True(t, errors.Is(err, nvmlerrors.ErrGPURequiresReset))
	})
}

// TestGetClockSpeed_MemoryClockGPURequiresReset_WithMockey tests GetClockSpeed with GPU requires reset on memory clock
func TestGetClockSpeed_MemoryClockGPURequiresReset_WithMockey(t *testing.T) {
	mockey.PatchConvey("GetClockSpeed with GPU requires reset on memory", t, func() {
		mockDevice := &mock.Device{
			GetClockInfoFunc: func(clockType nvml.ClockType) (uint32, nvml.Return) {
				if clockType == nvml.CLOCK_GRAPHICS {
					return 1500, nvml.SUCCESS
				}
				// Memory clock returns reset required
				return 0, nvml.ERROR_RESET_REQUIRED
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "GPU-TEST", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		_, err := GetClockSpeed("GPU-TEST", dev)
		assert.Error(t, err)
		assert.True(t, errors.Is(err, nvmlerrors.ErrGPURequiresReset))
	})
}

// TestGetClockSpeed_MemoryClockGPULost_WithMockey tests GetClockSpeed with GPU lost on memory clock
func TestGetClockSpeed_MemoryClockGPULost_WithMockey(t *testing.T) {
	mockey.PatchConvey("GetClockSpeed with GPU lost on memory", t, func() {
		mockDevice := &mock.Device{
			GetClockInfoFunc: func(clockType nvml.ClockType) (uint32, nvml.Return) {
				if clockType == nvml.CLOCK_GRAPHICS {
					return 1500, nvml.SUCCESS
				}
				// Memory clock returns GPU lost
				return 0, nvml.ERROR_GPU_IS_LOST
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "GPU-TEST", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		_, err := GetClockSpeed("GPU-TEST", dev)
		assert.Error(t, err)
		assert.True(t, errors.Is(err, nvmlerrors.ErrGPULost))
	})
}

// TestGetClockSpeed_GraphicsClockGPULost_WithMockey tests GetClockSpeed with GPU lost on graphics clock
func TestGetClockSpeed_GraphicsClockGPULost_WithMockey(t *testing.T) {
	mockey.PatchConvey("GetClockSpeed with GPU lost on graphics", t, func() {
		mockDevice := &mock.Device{
			GetClockInfoFunc: func(clockType nvml.ClockType) (uint32, nvml.Return) {
				if clockType == nvml.CLOCK_GRAPHICS {
					return 0, nvml.ERROR_GPU_IS_LOST
				}
				return 5000, nvml.SUCCESS
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "GPU-TEST", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		_, err := GetClockSpeed("GPU-TEST", dev)
		assert.Error(t, err)
		assert.True(t, errors.Is(err, nvmlerrors.ErrGPULost))
	})
}

// TestGetClockSpeed_BothNotSupported_WithMockey tests GetClockSpeed when both clocks are not supported
func TestGetClockSpeed_BothNotSupported_WithMockey(t *testing.T) {
	mockey.PatchConvey("GetClockSpeed with both not supported", t, func() {
		mockDevice := &mock.Device{
			GetClockInfoFunc: func(clockType nvml.ClockType) (uint32, nvml.Return) {
				return 0, nvml.ERROR_NOT_SUPPORTED
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "GPU-TEST", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		clockSpeed, err := GetClockSpeed("GPU-TEST", dev)
		assert.NoError(t, err)
		assert.False(t, clockSpeed.ClockGraphicsSupported)
		assert.False(t, clockSpeed.ClockMemorySupported)
		assert.Equal(t, uint32(0), clockSpeed.GraphicsMHz)
		assert.Equal(t, uint32(0), clockSpeed.MemoryMHz)
	})
}

// TestGetClockSpeed_SuccessfulFetch_WithMockey tests GetClockSpeed with successful fetch
func TestGetClockSpeed_SuccessfulFetch_WithMockey(t *testing.T) {
	mockey.PatchConvey("GetClockSpeed successful fetch", t, func() {
		mockDevice := &mock.Device{
			GetClockInfoFunc: func(clockType nvml.ClockType) (uint32, nvml.Return) {
				if clockType == nvml.CLOCK_GRAPHICS {
					return 1800, nvml.SUCCESS
				}
				return 7000, nvml.SUCCESS
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "GPU-TEST", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "0000:81:00.0")

		clockSpeed, err := GetClockSpeed("GPU-TEST", dev)
		assert.NoError(t, err)
		assert.Equal(t, "GPU-TEST", clockSpeed.UUID)
		assert.Equal(t, "0000:81:00.0", clockSpeed.BusID)
		assert.Equal(t, uint32(1800), clockSpeed.GraphicsMHz)
		assert.Equal(t, uint32(7000), clockSpeed.MemoryMHz)
		assert.True(t, clockSpeed.ClockGraphicsSupported)
		assert.True(t, clockSpeed.ClockMemorySupported)
	})
}

// TestGetClockSpeed_UnknownError_WithMockey tests GetClockSpeed with unknown error on graphics clock
func TestGetClockSpeed_UnknownError_WithMockey(t *testing.T) {
	mockey.PatchConvey("GetClockSpeed with unknown error on graphics", t, func() {
		mockDevice := &mock.Device{
			GetClockInfoFunc: func(clockType nvml.ClockType) (uint32, nvml.Return) {
				if clockType == nvml.CLOCK_GRAPHICS {
					return 0, nvml.ERROR_UNKNOWN
				}
				return 5000, nvml.SUCCESS
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "GPU-TEST", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		_, err := GetClockSpeed("GPU-TEST", dev)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get device clock info for nvml.CLOCK_GRAPHICS")
	})
}

// TestGetClockSpeed_MemoryUnknownError_WithMockey tests GetClockSpeed with unknown error on memory clock
func TestGetClockSpeed_MemoryUnknownError_WithMockey(t *testing.T) {
	mockey.PatchConvey("GetClockSpeed with unknown error on memory", t, func() {
		mockDevice := &mock.Device{
			GetClockInfoFunc: func(clockType nvml.ClockType) (uint32, nvml.Return) {
				if clockType == nvml.CLOCK_GRAPHICS {
					return 1500, nvml.SUCCESS
				}
				return 0, nvml.ERROR_UNKNOWN
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "GPU-TEST", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		_, err := GetClockSpeed("GPU-TEST", dev)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get device clock info for nvml.CLOCK_MEM")
	})
}

// TestCheckResult_String_WithMultipleClockSpeeds_WithMockey tests String with multiple clock speeds
func TestCheckResult_String_WithMultipleClockSpeeds_WithMockey(t *testing.T) {
	mockey.PatchConvey("String with multiple clock speeds", t, func() {
		cr := &checkResult{
			ClockSpeeds: []ClockSpeed{
				{
					UUID:                   "gpu-1",
					BusID:                  "0000:01:00.0",
					GraphicsMHz:            1500,
					MemoryMHz:              5000,
					ClockGraphicsSupported: true,
					ClockMemorySupported:   true,
				},
				{
					UUID:                   "gpu-2",
					BusID:                  "0000:02:00.0",
					GraphicsMHz:            1800,
					MemoryMHz:              6000,
					ClockGraphicsSupported: true,
					ClockMemorySupported:   true,
				},
			},
		}
		result := cr.String()
		assert.Contains(t, result, "gpu-1")
		assert.Contains(t, result, "gpu-2")
		assert.Contains(t, result, "0000:01:00.0")
		assert.Contains(t, result, "0000:02:00.0")
		assert.Contains(t, result, "1500 MHz")
		assert.Contains(t, result, "1800 MHz")
		assert.Contains(t, result, "5000 MHz")
		assert.Contains(t, result, "6000 MHz")
	})
}

// TestCheckResult_getError_WithMockey tests getError method with various cases
func TestCheckResult_getError_WithMockey(t *testing.T) {
	testCases := []struct {
		name     string
		cr       *checkResult
		expected string
	}{
		{
			name:     "nil checkResult",
			cr:       nil,
			expected: "",
		},
		{
			name:     "nil error",
			cr:       &checkResult{err: nil},
			expected: "",
		},
		{
			name:     "with error",
			cr:       &checkResult{err: errors.New("test error message")},
			expected: "test error message",
		},
		{
			name:     "with GPU lost error",
			cr:       &checkResult{err: nvmlerrors.ErrGPULost},
			expected: nvmlerrors.ErrGPULost.Error(),
		},
		{
			name:     "with GPU requires reset error",
			cr:       &checkResult{err: nvmlerrors.ErrGPURequiresReset},
			expected: nvmlerrors.ErrGPURequiresReset.Error(),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.cr.getError()
			assert.Equal(t, tc.expected, result)
		})
	}
}

// TestGetClockSpeed_GraphicsNotSupportedMemorySupported_WithMockey tests GetClockSpeed when graphics is not supported but memory is
func TestGetClockSpeed_GraphicsNotSupportedMemorySupported_WithMockey(t *testing.T) {
	mockey.PatchConvey("GetClockSpeed with graphics not supported memory supported", t, func() {
		mockDevice := &mock.Device{
			GetClockInfoFunc: func(clockType nvml.ClockType) (uint32, nvml.Return) {
				if clockType == nvml.CLOCK_GRAPHICS {
					return 0, nvml.ERROR_NOT_SUPPORTED
				}
				return 5000, nvml.SUCCESS
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "GPU-TEST", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		clockSpeed, err := GetClockSpeed("GPU-TEST", dev)
		assert.NoError(t, err)
		assert.False(t, clockSpeed.ClockGraphicsSupported)
		assert.True(t, clockSpeed.ClockMemorySupported)
		assert.Equal(t, uint32(0), clockSpeed.GraphicsMHz)
		assert.Equal(t, uint32(5000), clockSpeed.MemoryMHz)
	})
}

// TestGetClockSpeed_GraphicsSupportedMemoryNotSupported_WithMockey tests GetClockSpeed when graphics is supported but memory is not
func TestGetClockSpeed_GraphicsSupportedMemoryNotSupported_WithMockey(t *testing.T) {
	mockey.PatchConvey("GetClockSpeed with graphics supported memory not supported", t, func() {
		mockDevice := &mock.Device{
			GetClockInfoFunc: func(clockType nvml.ClockType) (uint32, nvml.Return) {
				if clockType == nvml.CLOCK_GRAPHICS {
					return 1500, nvml.SUCCESS
				}
				return 0, nvml.ERROR_NOT_SUPPORTED
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "GPU-TEST", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		clockSpeed, err := GetClockSpeed("GPU-TEST", dev)
		assert.NoError(t, err)
		assert.True(t, clockSpeed.ClockGraphicsSupported)
		assert.False(t, clockSpeed.ClockMemorySupported)
		assert.Equal(t, uint32(1500), clockSpeed.GraphicsMHz)
		assert.Equal(t, uint32(0), clockSpeed.MemoryMHz)
	})
}
