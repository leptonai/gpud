//go:build linux

package utilization

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
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

// TestNew_WithMockey tests the New function using mockey for isolation
func TestNew_WithMockey(t *testing.T) {
	mockey.PatchConvey("New component creation", t, func() {
		ctx := context.Background()
		mockInstance := &mockNVMLInstanceForMockey{
			devicesFunc: func() map[string]device.Device { return nil },
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
		assert.NotNil(t, tc.getUtilizationFunc)
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
					customMock := &customMockNVMLInstanceForMockey{
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
		cctx, cancel := context.WithCancel(ctx)
		defer cancel()

		initErr := errors.New("error getting device handle for index '0': Unknown Error")
		mockInst := &mockNVMLInstanceWithInitErrorForMockey{
			devs:      map[string]device.Device{},
			initError: initErr,
		}

		comp := &component{
			ctx:    cctx,
			cancel: cancel,
			getTimeNowFunc: func() time.Time {
				return time.Now().UTC()
			},
			nvmlInstance: mockInst,
		}

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
		cctx, cancel := context.WithCancel(ctx)
		defer cancel()

		mockInst := &customMockNVMLInstanceForMockey{
			devs:        map[string]device.Device{},
			nvmlExists:  true,
			productName: "",
		}

		comp := &component{
			ctx:    cctx,
			cancel: cancel,
			getTimeNowFunc: func() time.Time {
				return time.Now().UTC()
			},
			nvmlInstance: mockInst,
		}

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
		cctx, cancel := context.WithCancel(ctx)
		defer cancel()

		mockInst := &customMockNVMLInstanceForMockey{
			devs:        map[string]device.Device{},
			nvmlExists:  false,
			productName: "NVIDIA H100",
		}

		comp := &component{
			ctx:    cctx,
			cancel: cancel,
			getTimeNowFunc: func() time.Time {
				return time.Now().UTC()
			},
			nvmlInstance: mockInst,
		}

		result := comp.Check()
		cr, ok := result.(*checkResult)
		require.True(t, ok)

		assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
		assert.Contains(t, cr.reason, "NVIDIA NVML library is not loaded")
	})
}

// TestCheck_GPULostError_WithMockey tests Check when getUtilization returns ErrGPULost
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

		mockNvml := &customMockNVMLInstanceForMockey{
			devs:        devs,
			nvmlExists:  true,
			productName: "NVIDIA H100",
		}

		getUtilizationFunc := func(uuid string, dev device.Device) (Utilization, error) {
			return Utilization{}, nvmlerrors.ErrGPULost
		}

		comp := createMockUtilizationComponent(ctx, mockNvml, getUtilizationFunc).(*component)
		result := comp.Check()
		cr, ok := result.(*checkResult)
		require.True(t, ok)

		assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
		assert.Equal(t, nvmlerrors.ErrGPULost.Error(), cr.reason)
		assert.NotNil(t, cr.suggestedActions)
		assert.Contains(t, cr.suggestedActions.RepairActions, apiv1.RepairActionTypeRebootSystem)
	})
}

// TestCheck_GPURequiresResetError_WithMockey tests Check when getUtilization returns ErrGPURequiresReset
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

		mockNvml := &customMockNVMLInstanceForMockey{
			devs:        devs,
			nvmlExists:  true,
			productName: "NVIDIA H100",
		}

		getUtilizationFunc := func(uuid string, dev device.Device) (Utilization, error) {
			return Utilization{}, nvmlerrors.ErrGPURequiresReset
		}

		comp := createMockUtilizationComponent(ctx, mockNvml, getUtilizationFunc).(*component)
		result := comp.Check()
		cr, ok := result.(*checkResult)
		require.True(t, ok)

		assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
		assert.Equal(t, nvmlerrors.ErrGPURequiresReset.Error(), cr.reason)
		assert.NotNil(t, cr.suggestedActions)
		assert.Contains(t, cr.suggestedActions.RepairActions, apiv1.RepairActionTypeRebootSystem)
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

		mockNvml := &customMockNVMLInstanceForMockey{
			devs:        devs,
			nvmlExists:  true,
			productName: "NVIDIA H100",
		}

		getUtilizationFunc := func(uuid string, dev device.Device) (Utilization, error) {
			return Utilization{
				UUID:              uuid,
				GPUUsedPercent:    75,
				MemoryUsedPercent: 60,
				Supported:         true,
			}, nil
		}

		comp := createMockUtilizationComponent(ctx, mockNvml, getUtilizationFunc).(*component)
		result := comp.Check()
		cr, ok := result.(*checkResult)
		require.True(t, ok)

		assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
		assert.Contains(t, cr.reason, "2 GPU(s) were checked")
		assert.Len(t, cr.Utilizations, 2)
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

		mockNvml := &customMockNVMLInstanceForMockey{
			devs:        devs,
			nvmlExists:  true,
			productName: "NVIDIA H100",
		}

		getUtilizationFunc := func(uuid string, dev device.Device) (Utilization, error) {
			return Utilization{
				UUID:              uuid,
				GPUUsedPercent:    75,
				MemoryUsedPercent: 60,
				Supported:         true,
			}, nil
		}

		comp := createMockUtilizationComponent(ctx, mockNvml, getUtilizationFunc).(*component)

		// Run concurrent access
		var wg sync.WaitGroup
		for i := 0; i < 5; i++ {
			wg.Add(2)
			go func() {
				defer wg.Done()
				comp.Check()
			}()
			go func() {
				defer wg.Done()
				_ = comp.LastHealthStates()
			}()
		}

		wg.Wait()

		// Verify final state is consistent
		states := comp.LastHealthStates()
		assert.Len(t, states, 1)
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

		mockNvml := &customMockNVMLInstanceForMockey{
			devs:        devs,
			nvmlExists:  true,
			productName: "NVIDIA H100",
		}

		// Simulate GPU requires reset error
		getUtilizationFunc := func(uuid string, dev device.Device) (Utilization, error) {
			return Utilization{}, nvmlerrors.ErrGPURequiresReset
		}

		comp := createMockUtilizationComponent(ctx, mockNvml, getUtilizationFunc).(*component)
		comp.Check()

		states := comp.LastHealthStates()
		require.Len(t, states, 1)
		assert.Equal(t, apiv1.HealthStateTypeUnhealthy, states[0].Health)
		assert.NotNil(t, states[0].SuggestedActions)
		assert.Contains(t, states[0].SuggestedActions.RepairActions, apiv1.RepairActionTypeRebootSystem)
	})
}

// TestCheckResult_Methods_WithMockey tests all checkResult methods
func TestCheckResult_Methods_WithMockey(t *testing.T) {
	t.Run("ComponentName", func(t *testing.T) {
		cr := &checkResult{}
		assert.Equal(t, Name, cr.ComponentName())
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

// TestCheckResult_String_WithMockey tests the String method of checkResult
func TestCheckResult_String_WithMockey(t *testing.T) {
	t.Run("nil check result", func(t *testing.T) {
		var cr *checkResult
		assert.Equal(t, "", cr.String())
	})

	t.Run("empty Utilizations", func(t *testing.T) {
		cr := &checkResult{}
		assert.Equal(t, "no data", cr.String())
	})

	t.Run("with Utilizations", func(t *testing.T) {
		cr := &checkResult{
			Utilizations: []Utilization{
				{
					UUID:              "gpu-1",
					BusID:             "0000:01:00.0",
					GPUUsedPercent:    75,
					MemoryUsedPercent: 60,
					Supported:         true,
				},
			},
		}
		result := cr.String()
		assert.Contains(t, result, "gpu-1")
		assert.Contains(t, result, "0000:01:00.0")
	})
}

// TestCheckResult_Summary_WithMockey tests the Summary method
func TestCheckResult_Summary_WithMockey(t *testing.T) {
	t.Run("nil check result", func(t *testing.T) {
		var cr *checkResult
		assert.Equal(t, "", cr.Summary())
	})

	t.Run("with reason", func(t *testing.T) {
		cr := &checkResult{reason: "test reason"}
		assert.Equal(t, "test reason", cr.Summary())
	})
}

// TestCheckResult_HealthStateType_WithMockey tests the HealthStateType method
func TestCheckResult_HealthStateType_WithMockey(t *testing.T) {
	t.Run("nil check result", func(t *testing.T) {
		var cr *checkResult
		assert.Equal(t, apiv1.HealthStateType(""), cr.HealthStateType())
	})

	t.Run("healthy", func(t *testing.T) {
		cr := &checkResult{health: apiv1.HealthStateTypeHealthy}
		assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.HealthStateType())
	})
}

// TestCheckResult_HealthStates_NilResult_WithMockey tests HealthStates with nil checkResult
func TestCheckResult_HealthStates_NilResult_WithMockey(t *testing.T) {
	var cr *checkResult
	states := cr.HealthStates()
	require.Len(t, states, 1)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, states[0].Health)
	assert.Equal(t, "no data yet", states[0].Reason)
}

// TestCheckResult_HealthStates_WithExtraInfo_WithMockey tests HealthStates with Utilizations
func TestCheckResult_HealthStates_WithExtraInfo_WithMockey(t *testing.T) {
	cr := &checkResult{
		ts:     time.Now(),
		health: apiv1.HealthStateTypeHealthy,
		reason: "all good",
		Utilizations: []Utilization{
			{UUID: "gpu-1", GPUUsedPercent: 75, MemoryUsedPercent: 60},
		},
	}

	states := cr.HealthStates()
	require.Len(t, states, 1)
	assert.NotEmpty(t, states[0].ExtraInfo)
	assert.Contains(t, states[0].ExtraInfo["data"], "gpu-1")
}

// TestGetUtilization_GPULost_WithMockey tests GetUtilization function with GPU lost error
func TestGetUtilization_GPULost_WithMockey(t *testing.T) {
	mockey.PatchConvey("GetUtilization with GPU lost", t, func() {
		mockDevice := &mock.Device{
			GetUtilizationRatesFunc: func() (nvml.Utilization, nvml.Return) {
				return nvml.Utilization{}, nvml.ERROR_GPU_IS_LOST
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "GPU-TEST", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		_, err := GetUtilization("GPU-TEST", dev)
		assert.Error(t, err)
		assert.True(t, errors.Is(err, nvmlerrors.ErrGPULost))
	})
}

// TestGetUtilization_GPURequiresReset_WithMockey tests GetUtilization with GPU requires reset error
func TestGetUtilization_GPURequiresReset_WithMockey(t *testing.T) {
	mockey.PatchConvey("GetUtilization with GPU requires reset", t, func() {
		mockDevice := &mock.Device{
			GetUtilizationRatesFunc: func() (nvml.Utilization, nvml.Return) {
				return nvml.Utilization{}, nvml.ERROR_RESET_REQUIRED
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "GPU-TEST", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		_, err := GetUtilization("GPU-TEST", dev)
		assert.Error(t, err)
		assert.True(t, errors.Is(err, nvmlerrors.ErrGPURequiresReset))
	})
}

// TestGetUtilization_NotSupported_WithMockey tests GetUtilization when utilization is not supported
func TestGetUtilization_NotSupported_WithMockey(t *testing.T) {
	mockey.PatchConvey("GetUtilization with not supported", t, func() {
		mockDevice := &mock.Device{
			GetUtilizationRatesFunc: func() (nvml.Utilization, nvml.Return) {
				return nvml.Utilization{}, nvml.ERROR_NOT_SUPPORTED
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "GPU-TEST", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		util, err := GetUtilization("GPU-TEST", dev)
		assert.NoError(t, err)
		assert.False(t, util.Supported)
	})
}

// TestGetUtilization_Success_WithMockey tests GetUtilization with successful response
func TestGetUtilization_Success_WithMockey(t *testing.T) {
	mockey.PatchConvey("GetUtilization success", t, func() {
		mockDevice := &mock.Device{
			GetUtilizationRatesFunc: func() (nvml.Utilization, nvml.Return) {
				return nvml.Utilization{
					Gpu:    75,
					Memory: 60,
				}, nvml.SUCCESS
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "GPU-TEST", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		util, err := GetUtilization("GPU-TEST", dev)
		assert.NoError(t, err)
		assert.True(t, util.Supported)
		assert.Equal(t, uint32(75), util.GPUUsedPercent)
		assert.Equal(t, uint32(60), util.MemoryUsedPercent)
	})
}

// TestGetUtilization_UnknownError_WithMockey tests GetUtilization with unknown NVML error
func TestGetUtilization_UnknownError_WithMockey(t *testing.T) {
	mockey.PatchConvey("GetUtilization with unknown error", t, func() {
		mockDevice := &mock.Device{
			GetUtilizationRatesFunc: func() (nvml.Utilization, nvml.Return) {
				return nvml.Utilization{}, nvml.ERROR_UNKNOWN
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "GPU-TEST", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		_, err := GetUtilization("GPU-TEST", dev)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get device utilization rates")
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

		mockNvml := &customMockNVMLInstanceForMockey{
			devs:        devs,
			nvmlExists:  true,
			productName: "NVIDIA H100",
		}

		var checkCalled atomic.Bool
		getUtilizationFunc := func(uuid string, dev device.Device) (Utilization, error) {
			checkCalled.Store(true)
			return Utilization{
				UUID:              uuid,
				GPUUsedPercent:    50,
				MemoryUsedPercent: 40,
				Supported:         true,
			}, nil
		}

		comp := createMockUtilizationComponent(ctx, mockNvml, getUtilizationFunc).(*component)

		err := comp.Start()
		assert.NoError(t, err)

		// Give the goroutine time to execute Check
		time.Sleep(100 * time.Millisecond)

		assert.True(t, checkCalled.Load(), "Check should have been called")
	})
}

// TestClose_WithMockey tests the Close method
func TestClose_WithMockey(t *testing.T) {
	mockey.PatchConvey("Close method cancels context", t, func() {
		ctx := context.Background()

		mockNvml := &customMockNVMLInstanceForMockey{
			devs:        map[string]device.Device{},
			nvmlExists:  true,
			productName: "NVIDIA H100",
		}

		comp := createMockUtilizationComponent(ctx, mockNvml, nil).(*component)

		err := comp.Close()
		assert.NoError(t, err)

		// Check that context is canceled
		select {
		case <-comp.ctx.Done():
			// Context is properly canceled
		default:
			t.Fatal("component context was not canceled on Close")
		}
	})
}

// TestCheck_GenericError_WithMockey tests Check when getUtilization returns a generic error
func TestCheck_GenericError_WithMockey(t *testing.T) {
	mockey.PatchConvey("Check with generic error", t, func() {
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

		mockNvml := &customMockNVMLInstanceForMockey{
			devs:        devs,
			nvmlExists:  true,
			productName: "NVIDIA H100",
		}

		genericErr := errors.New("some generic error")
		getUtilizationFunc := func(uuid string, dev device.Device) (Utilization, error) {
			return Utilization{}, genericErr
		}

		comp := createMockUtilizationComponent(ctx, mockNvml, getUtilizationFunc).(*component)
		result := comp.Check()
		cr, ok := result.(*checkResult)
		require.True(t, ok)

		assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
		assert.Equal(t, "error getting utilization", cr.reason)
		assert.Equal(t, genericErr, cr.err)
		// No suggested actions for generic errors
		assert.Nil(t, cr.suggestedActions)
	})
}

// TestCheckResult_getError_WithMockey tests the getError method
func TestCheckResult_getError_WithMockey(t *testing.T) {
	t.Run("nil check result", func(t *testing.T) {
		var cr *checkResult
		assert.Equal(t, "", cr.getError())
	})

	t.Run("nil error", func(t *testing.T) {
		cr := &checkResult{err: nil}
		assert.Equal(t, "", cr.getError())
	})

	t.Run("with error", func(t *testing.T) {
		cr := &checkResult{err: errors.New("test error")}
		assert.Equal(t, "test error", cr.getError())
	})
}

// Helper function to create mock utilization component
func createMockUtilizationComponent(
	ctx context.Context,
	mockNVMLInstance *customMockNVMLInstanceForMockey,
	getUtilizationFunc func(uuid string, dev device.Device) (Utilization, error),
) components.Component {
	cctx, cancel := context.WithCancel(ctx)
	return &component{
		ctx:    cctx,
		cancel: cancel,
		getTimeNowFunc: func() time.Time {
			return time.Now().UTC()
		},
		nvmlInstance:       mockNVMLInstance,
		getUtilizationFunc: getUtilizationFunc,
	}
}

// Mock implementations for mockey tests

type mockNVMLInstanceForMockey struct {
	devicesFunc func() map[string]device.Device
}

func (m *mockNVMLInstanceForMockey) Devices() map[string]device.Device {
	if m.devicesFunc != nil {
		return m.devicesFunc()
	}
	return nil
}
func (m *mockNVMLInstanceForMockey) FabricManagerSupported() bool { return true }
func (m *mockNVMLInstanceForMockey) FabricStateSupported() bool   { return false }
func (m *mockNVMLInstanceForMockey) GetMemoryErrorManagementCapabilities() nvidiaproduct.MemoryErrorManagementCapabilities {
	return nvidiaproduct.MemoryErrorManagementCapabilities{}
}
func (m *mockNVMLInstanceForMockey) ProductName() string   { return "Test GPU" }
func (m *mockNVMLInstanceForMockey) Architecture() string  { return "" }
func (m *mockNVMLInstanceForMockey) Brand() string         { return "" }
func (m *mockNVMLInstanceForMockey) DriverVersion() string { return "" }
func (m *mockNVMLInstanceForMockey) DriverMajor() int      { return 0 }
func (m *mockNVMLInstanceForMockey) CUDAVersion() string   { return "" }
func (m *mockNVMLInstanceForMockey) NVMLExists() bool      { return true }
func (m *mockNVMLInstanceForMockey) Library() lib.Library {
	return nil
}
func (m *mockNVMLInstanceForMockey) Shutdown() error  { return nil }
func (m *mockNVMLInstanceForMockey) InitError() error { return nil }

type customMockNVMLInstanceForMockey struct {
	devs        map[string]device.Device
	nvmlExists  bool
	productName string
}

func (m *customMockNVMLInstanceForMockey) Devices() map[string]device.Device { return m.devs }
func (m *customMockNVMLInstanceForMockey) FabricManagerSupported() bool      { return true }
func (m *customMockNVMLInstanceForMockey) FabricStateSupported() bool        { return false }
func (m *customMockNVMLInstanceForMockey) GetMemoryErrorManagementCapabilities() nvidiaproduct.MemoryErrorManagementCapabilities {
	return nvidiaproduct.MemoryErrorManagementCapabilities{}
}
func (m *customMockNVMLInstanceForMockey) ProductName() string   { return m.productName }
func (m *customMockNVMLInstanceForMockey) Architecture() string  { return "" }
func (m *customMockNVMLInstanceForMockey) Brand() string         { return "" }
func (m *customMockNVMLInstanceForMockey) DriverVersion() string { return "" }
func (m *customMockNVMLInstanceForMockey) DriverMajor() int      { return 0 }
func (m *customMockNVMLInstanceForMockey) CUDAVersion() string   { return "" }
func (m *customMockNVMLInstanceForMockey) NVMLExists() bool      { return m.nvmlExists }
func (m *customMockNVMLInstanceForMockey) Library() lib.Library  { return nil }
func (m *customMockNVMLInstanceForMockey) Shutdown() error       { return nil }
func (m *customMockNVMLInstanceForMockey) InitError() error      { return nil }

type mockNVMLInstanceWithInitErrorForMockey struct {
	devs      map[string]device.Device
	initError error
}

func (m *mockNVMLInstanceWithInitErrorForMockey) Devices() map[string]device.Device { return m.devs }
func (m *mockNVMLInstanceWithInitErrorForMockey) FabricManagerSupported() bool      { return true }
func (m *mockNVMLInstanceWithInitErrorForMockey) FabricStateSupported() bool        { return false }
func (m *mockNVMLInstanceWithInitErrorForMockey) GetMemoryErrorManagementCapabilities() nvidiaproduct.MemoryErrorManagementCapabilities {
	return nvidiaproduct.MemoryErrorManagementCapabilities{}
}
func (m *mockNVMLInstanceWithInitErrorForMockey) ProductName() string   { return "NVIDIA H100" }
func (m *mockNVMLInstanceWithInitErrorForMockey) Architecture() string  { return "" }
func (m *mockNVMLInstanceWithInitErrorForMockey) Brand() string         { return "" }
func (m *mockNVMLInstanceWithInitErrorForMockey) DriverVersion() string { return "" }
func (m *mockNVMLInstanceWithInitErrorForMockey) DriverMajor() int      { return 0 }
func (m *mockNVMLInstanceWithInitErrorForMockey) CUDAVersion() string   { return "" }
func (m *mockNVMLInstanceWithInitErrorForMockey) NVMLExists() bool      { return true }
func (m *mockNVMLInstanceWithInitErrorForMockey) Library() lib.Library  { return nil }
func (m *mockNVMLInstanceWithInitErrorForMockey) Shutdown() error       { return nil }
func (m *mockNVMLInstanceWithInitErrorForMockey) InitError() error      { return m.initError }
