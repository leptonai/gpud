//go:build linux

package power

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
		assert.NotNil(t, tc.getPowerFunc)
		assert.NotNil(t, tc.getTimeNowFunc)
	})
}

// TestComponent_IsSupported tests IsSupported method with various conditions
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

// TestCheck_InitError tests Check when NVML has an initialization error
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

// TestCheck_MissingProductName tests Check when product name is empty
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

// TestCheck_NilNVMLInstance tests Check with nil NVML instance
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

// TestCheck_NVMLNotExists tests Check when NVML library is not loaded
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

// TestCheck_GPULostError_WithMockey tests Check when getPower returns ErrGPULost
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

		getPowerFunc := func(uuid string, dev device.Device) (Power, error) {
			return Power{}, nvmlerrors.ErrGPULost
		}

		comp := createMockPowerComponent(ctx, mockNvml, getPowerFunc).(*component)
		result := comp.Check()
		cr, ok := result.(*checkResult)
		require.True(t, ok)

		assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
		assert.Equal(t, nvmlerrors.ErrGPULost.Error(), cr.reason)
		assert.NotNil(t, cr.suggestedActions)
		assert.Contains(t, cr.suggestedActions.RepairActions, apiv1.RepairActionTypeRebootSystem)
	})
}

// TestCheck_GPURequiresResetError_WithMockey tests Check when getPower returns ErrGPURequiresReset
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

		getPowerFunc := func(uuid string, dev device.Device) (Power, error) {
			return Power{}, nvmlerrors.ErrGPURequiresReset
		}

		comp := createMockPowerComponent(ctx, mockNvml, getPowerFunc).(*component)
		result := comp.Check()
		cr, ok := result.(*checkResult)
		require.True(t, ok)

		assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
		assert.Equal(t, nvmlerrors.ErrGPURequiresReset.Error(), cr.reason)
		assert.NotNil(t, cr.suggestedActions)
		assert.Contains(t, cr.suggestedActions.RepairActions, apiv1.RepairActionTypeRebootSystem)
	})
}

// TestCheck_GetUsedPercentGPULost_WithMockey tests Check when GetUsedPercent returns error due to GPU lost
func TestCheck_GetUsedPercentGPULost_WithMockey(t *testing.T) {
	mockey.PatchConvey("Check with GetUsedPercent GPU lost error", t, func() {
		ctx := context.Background()

		uuid := "gpu-uuid-percent-lost"
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

		// Return power with invalid UsedPercent format to trigger GetUsedPercent error
		getPowerFunc := func(uuid string, dev device.Device) (Power, error) {
			return Power{
				UUID:                    uuid,
				UsageMilliWatts:         150000,
				EnforcedLimitMilliWatts: 250000,
				UsedPercent:             "invalid", // Will cause ParseFloat to fail
			}, nil
		}

		comp := createMockPowerComponent(ctx, mockNvml, getPowerFunc).(*component)
		result := comp.Check()
		cr, ok := result.(*checkResult)
		require.True(t, ok)

		assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
		assert.Equal(t, "error getting used percent", cr.reason)
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

		getPowerFunc := func(uuid string, dev device.Device) (Power, error) {
			return Power{
				UUID:                    uuid,
				UsageMilliWatts:         150000,
				EnforcedLimitMilliWatts: 250000,
				UsedPercent:             "60.00",
			}, nil
		}

		comp := createMockPowerComponent(ctx, mockNvml, getPowerFunc).(*component)
		result := comp.Check()
		cr, ok := result.(*checkResult)
		require.True(t, ok)

		assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
		assert.Contains(t, cr.reason, "2 GPU(s) were checked")
		assert.Len(t, cr.Powers, 2)
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

		getPowerFunc := func(uuid string, dev device.Device) (Power, error) {
			return Power{
				UUID:                    uuid,
				UsageMilliWatts:         150000,
				EnforcedLimitMilliWatts: 250000,
				UsedPercent:             "60.00",
			}, nil
		}

		comp := createMockPowerComponent(ctx, mockNvml, getPowerFunc).(*component)

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
		getPowerFunc := func(uuid string, dev device.Device) (Power, error) {
			return Power{}, nvmlerrors.ErrGPURequiresReset
		}

		comp := createMockPowerComponent(ctx, mockNvml, getPowerFunc).(*component)
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

	t.Run("empty Powers", func(t *testing.T) {
		cr := &checkResult{}
		assert.Equal(t, "no data", cr.String())
	})

	t.Run("with Powers", func(t *testing.T) {
		cr := &checkResult{
			Powers: []Power{
				{
					UUID:                    "gpu-1",
					BusID:                   "0000:01:00.0",
					UsageMilliWatts:         150000,
					EnforcedLimitMilliWatts: 250000,
					UsedPercent:             "60.00",
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

// TestCheckResult_HealthStates_WithExtraInfo_WithMockey tests HealthStates with Powers
func TestCheckResult_HealthStates_WithExtraInfo_WithMockey(t *testing.T) {
	cr := &checkResult{
		ts:     time.Now(),
		health: apiv1.HealthStateTypeHealthy,
		reason: "all good",
		Powers: []Power{
			{UUID: "gpu-1", UsageMilliWatts: 150000, EnforcedLimitMilliWatts: 250000},
		},
	}

	states := cr.HealthStates()
	require.Len(t, states, 1)
	assert.NotEmpty(t, states[0].ExtraInfo)
	assert.Contains(t, states[0].ExtraInfo["data"], "gpu-1")
}

// TestGetPower_GPULost_WithMockey tests GetPower function with GPU lost error on power usage
func TestGetPower_GPULost_WithMockey(t *testing.T) {
	mockey.PatchConvey("GetPower with GPU lost on power usage", t, func() {
		mockDevice := &mock.Device{
			GetPowerUsageFunc: func() (uint32, nvml.Return) {
				return 0, nvml.ERROR_GPU_IS_LOST
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "GPU-TEST", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		_, err := GetPower("GPU-TEST", dev)
		assert.Error(t, err)
		assert.True(t, errors.Is(err, nvmlerrors.ErrGPULost))
	})
}

// TestGetPower_GPURequiresReset_WithMockey tests GetPower with GPU requires reset error
func TestGetPower_GPURequiresReset_WithMockey(t *testing.T) {
	mockey.PatchConvey("GetPower with GPU requires reset", t, func() {
		mockDevice := &mock.Device{
			GetPowerUsageFunc: func() (uint32, nvml.Return) {
				return 0, nvml.ERROR_RESET_REQUIRED
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "GPU-TEST", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		_, err := GetPower("GPU-TEST", dev)
		assert.Error(t, err)
		assert.True(t, errors.Is(err, nvmlerrors.ErrGPURequiresReset))
	})
}

// TestGetPower_EnforcedLimitGPULost_WithMockey tests GetPower with GPU lost on enforced limit
func TestGetPower_EnforcedLimitGPULost_WithMockey(t *testing.T) {
	mockey.PatchConvey("GetPower with GPU lost on enforced limit", t, func() {
		mockDevice := &mock.Device{
			GetPowerUsageFunc: func() (uint32, nvml.Return) {
				return 100, nvml.SUCCESS
			},
			GetEnforcedPowerLimitFunc: func() (uint32, nvml.Return) {
				return 0, nvml.ERROR_GPU_IS_LOST
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "GPU-TEST", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		_, err := GetPower("GPU-TEST", dev)
		assert.Error(t, err)
		assert.True(t, errors.Is(err, nvmlerrors.ErrGPULost))
	})
}

// TestGetPower_EnforcedLimitRequiresReset_WithMockey tests GetPower with GPU requires reset on enforced limit
func TestGetPower_EnforcedLimitRequiresReset_WithMockey(t *testing.T) {
	mockey.PatchConvey("GetPower with GPU requires reset on enforced limit", t, func() {
		mockDevice := &mock.Device{
			GetPowerUsageFunc: func() (uint32, nvml.Return) {
				return 100, nvml.SUCCESS
			},
			GetEnforcedPowerLimitFunc: func() (uint32, nvml.Return) {
				return 0, nvml.ERROR_RESET_REQUIRED
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "GPU-TEST", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		_, err := GetPower("GPU-TEST", dev)
		assert.Error(t, err)
		assert.True(t, errors.Is(err, nvmlerrors.ErrGPURequiresReset))
	})
}

// TestGetPower_ManagementLimitGPULost_WithMockey tests GetPower with GPU lost on management limit
func TestGetPower_ManagementLimitGPULost_WithMockey(t *testing.T) {
	mockey.PatchConvey("GetPower with GPU lost on management limit", t, func() {
		mockDevice := &mock.Device{
			GetPowerUsageFunc: func() (uint32, nvml.Return) {
				return 100, nvml.SUCCESS
			},
			GetEnforcedPowerLimitFunc: func() (uint32, nvml.Return) {
				return 200, nvml.SUCCESS
			},
			GetPowerManagementLimitFunc: func() (uint32, nvml.Return) {
				return 0, nvml.ERROR_GPU_IS_LOST
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "GPU-TEST", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		_, err := GetPower("GPU-TEST", dev)
		assert.Error(t, err)
		assert.True(t, errors.Is(err, nvmlerrors.ErrGPULost))
	})
}

// TestGetPower_ManagementLimitRequiresReset_WithMockey tests GetPower with GPU requires reset on management limit
func TestGetPower_ManagementLimitRequiresReset_WithMockey(t *testing.T) {
	mockey.PatchConvey("GetPower with GPU requires reset on management limit", t, func() {
		mockDevice := &mock.Device{
			GetPowerUsageFunc: func() (uint32, nvml.Return) {
				return 100, nvml.SUCCESS
			},
			GetEnforcedPowerLimitFunc: func() (uint32, nvml.Return) {
				return 200, nvml.SUCCESS
			},
			GetPowerManagementLimitFunc: func() (uint32, nvml.Return) {
				return 0, nvml.ERROR_RESET_REQUIRED
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "GPU-TEST", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		_, err := GetPower("GPU-TEST", dev)
		assert.Error(t, err)
		assert.True(t, errors.Is(err, nvmlerrors.ErrGPURequiresReset))
	})
}

// TestGetPower_NotSupported_WithMockey tests GetPower when power features are not supported
func TestGetPower_NotSupported_WithMockey(t *testing.T) {
	mockey.PatchConvey("GetPower with not supported features", t, func() {
		mockDevice := &mock.Device{
			GetPowerUsageFunc: func() (uint32, nvml.Return) {
				return 0, nvml.ERROR_NOT_SUPPORTED
			},
			GetEnforcedPowerLimitFunc: func() (uint32, nvml.Return) {
				return 0, nvml.ERROR_NOT_SUPPORTED
			},
			GetPowerManagementLimitFunc: func() (uint32, nvml.Return) {
				return 0, nvml.ERROR_NOT_SUPPORTED
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "GPU-TEST", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		power, err := GetPower("GPU-TEST", dev)
		assert.NoError(t, err)
		assert.False(t, power.GetPowerUsageSupported)
		assert.False(t, power.GetPowerLimitSupported)
		assert.False(t, power.GetPowerManagementLimitSupported)
		assert.Equal(t, "0.0", power.UsedPercent) // When total is 0, UsedPercent is "0.0"
	})
}

// TestGetPower_UsedPercentCalculation_WithMockey tests UsedPercent calculation
func TestGetPower_UsedPercentCalculation_WithMockey(t *testing.T) {
	testCases := []struct {
		name                    string
		usageMilliWatts         uint32
		enforcedLimitMilliWatts uint32
		managementLimitMW       uint32
		expectedPercent         string
	}{
		{
			name:                    "50% usage",
			usageMilliWatts:         100000,
			enforcedLimitMilliWatts: 200000,
			managementLimitMW:       250000,
			expectedPercent:         "50.00",
		},
		{
			name:                    "100% usage",
			usageMilliWatts:         200000,
			enforcedLimitMilliWatts: 200000,
			managementLimitMW:       200000,
			expectedPercent:         "100.00",
		},
		{
			name:                    "0% usage",
			usageMilliWatts:         0,
			enforcedLimitMilliWatts: 200000,
			managementLimitMW:       200000,
			expectedPercent:         "0.00",
		},
		{
			name:                    "fallback to management limit when enforced is 0",
			usageMilliWatts:         100000,
			enforcedLimitMilliWatts: 0,
			managementLimitMW:       200000,
			expectedPercent:         "50.00",
		},
		{
			name:                    "0 when both limits are 0",
			usageMilliWatts:         100000,
			enforcedLimitMilliWatts: 0,
			managementLimitMW:       0,
			expectedPercent:         "0.0",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockey.PatchConvey(tc.name, t, func() {
				mockDevice := &mock.Device{
					GetPowerUsageFunc: func() (uint32, nvml.Return) {
						return tc.usageMilliWatts, nvml.SUCCESS
					},
					GetEnforcedPowerLimitFunc: func() (uint32, nvml.Return) {
						return tc.enforcedLimitMilliWatts, nvml.SUCCESS
					},
					GetPowerManagementLimitFunc: func() (uint32, nvml.Return) {
						return tc.managementLimitMW, nvml.SUCCESS
					},
					GetUUIDFunc: func() (string, nvml.Return) {
						return "GPU-TEST", nvml.SUCCESS
					},
				}

				dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

				power, err := GetPower("GPU-TEST", dev)
				assert.NoError(t, err)
				assert.Equal(t, tc.expectedPercent, power.UsedPercent)
			})
		})
	}
}

// TestGetPower_UnknownError_WithMockey tests GetPower with unknown NVML error
func TestGetPower_UnknownError_WithMockey(t *testing.T) {
	mockey.PatchConvey("GetPower with unknown error on power usage", t, func() {
		mockDevice := &mock.Device{
			GetPowerUsageFunc: func() (uint32, nvml.Return) {
				return 0, nvml.ERROR_UNKNOWN
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "GPU-TEST", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		_, err := GetPower("GPU-TEST", dev)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get device power usage")
	})
}

// TestGetPower_EnforcedLimitUnknownError_WithMockey tests GetPower with unknown error on enforced limit
func TestGetPower_EnforcedLimitUnknownError_WithMockey(t *testing.T) {
	mockey.PatchConvey("GetPower with unknown error on enforced limit", t, func() {
		mockDevice := &mock.Device{
			GetPowerUsageFunc: func() (uint32, nvml.Return) {
				return 100, nvml.SUCCESS
			},
			GetEnforcedPowerLimitFunc: func() (uint32, nvml.Return) {
				return 0, nvml.ERROR_UNKNOWN
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "GPU-TEST", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		_, err := GetPower("GPU-TEST", dev)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get device power limit")
	})
}

// TestGetPower_ManagementLimitUnknownError_WithMockey tests GetPower with unknown error on management limit
func TestGetPower_ManagementLimitUnknownError_WithMockey(t *testing.T) {
	mockey.PatchConvey("GetPower with unknown error on management limit", t, func() {
		mockDevice := &mock.Device{
			GetPowerUsageFunc: func() (uint32, nvml.Return) {
				return 100, nvml.SUCCESS
			},
			GetEnforcedPowerLimitFunc: func() (uint32, nvml.Return) {
				return 200, nvml.SUCCESS
			},
			GetPowerManagementLimitFunc: func() (uint32, nvml.Return) {
				return 0, nvml.ERROR_UNKNOWN
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "GPU-TEST", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		_, err := GetPower("GPU-TEST", dev)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get device power management limit")
	})
}

// TestGetUsedPercent_WithMockey tests GetUsedPercent method
func TestGetUsedPercent_WithMockey(t *testing.T) {
	testCases := []struct {
		name        string
		usedPercent string
		expected    float64
		expectError bool
	}{
		{
			name:        "valid percent",
			usedPercent: "75.50",
			expected:    75.50,
			expectError: false,
		},
		{
			name:        "zero percent",
			usedPercent: "0.0",
			expected:    0.0,
			expectError: false,
		},
		{
			name:        "100 percent",
			usedPercent: "100.00",
			expected:    100.00,
			expectError: false,
		},
		{
			name:        "invalid percent",
			usedPercent: "invalid",
			expectError: true,
		},
		{
			name:        "empty percent",
			usedPercent: "",
			expectError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			power := Power{UsedPercent: tc.usedPercent}
			result, err := power.GetUsedPercent()

			if tc.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.expected, result)
			}
		})
	}
}

// Helper function to create mock power component
func createMockPowerComponent(
	ctx context.Context,
	mockNVMLInstance *customMockNVMLInstanceForMockey,
	getPowerFunc func(uuid string, dev device.Device) (Power, error),
) components.Component {
	cctx, cancel := context.WithCancel(ctx)
	return &component{
		ctx:    cctx,
		cancel: cancel,
		getTimeNowFunc: func() time.Time {
			return time.Now().UTC()
		},
		nvmlInstance: mockNVMLInstance,
		getPowerFunc: getPowerFunc,
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

		checkCalled := make(chan bool, 1)
		getPowerFunc := func(uuid string, dev device.Device) (Power, error) {
			select {
			case checkCalled <- true:
			default:
			}
			return Power{
				UUID:                    uuid,
				UsageMilliWatts:         100000,
				EnforcedLimitMilliWatts: 200000,
				UsedPercent:             "50.00",
			}, nil
		}

		comp := createMockPowerComponent(ctx, mockNvml, getPowerFunc).(*component)
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

// TestCheck_GetUsedPercentGPURequiresReset_WithMockey tests Check when GetUsedPercent triggers GPU requires reset
func TestCheck_GetUsedPercentGPURequiresReset_WithMockey(t *testing.T) {
	mockey.PatchConvey("Check with GetUsedPercent GPU requires reset", t, func() {
		ctx := context.Background()

		uuid := "gpu-uuid-percent-reset"
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

		// Return power with invalid UsedPercent that will trigger an error wrapped with ErrGPURequiresReset
		// This simulates a case where parsing fails and we check for specific GPU errors
		callCount := 0
		getPowerFunc := func(uuid string, dev device.Device) (Power, error) {
			callCount++
			return Power{
				UUID:                    uuid,
				UsageMilliWatts:         150000,
				EnforcedLimitMilliWatts: 250000,
				UsedPercent:             "invalid", // Will cause ParseFloat to fail
			}, nil
		}

		comp := createMockPowerComponent(ctx, mockNvml, getPowerFunc).(*component)
		result := comp.Check()
		cr, ok := result.(*checkResult)
		require.True(t, ok)

		assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
		assert.Equal(t, "error getting used percent", cr.reason)
	})
}

// TestGetPower_SuccessWithAllSupported_WithMockey tests GetPower with all features supported
func TestGetPower_SuccessWithAllSupported_WithMockey(t *testing.T) {
	mockey.PatchConvey("GetPower with all features supported", t, func() {
		mockDevice := &mock.Device{
			GetPowerUsageFunc: func() (uint32, nvml.Return) {
				return 150000, nvml.SUCCESS
			},
			GetEnforcedPowerLimitFunc: func() (uint32, nvml.Return) {
				return 250000, nvml.SUCCESS
			},
			GetPowerManagementLimitFunc: func() (uint32, nvml.Return) {
				return 300000, nvml.SUCCESS
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "GPU-TEST", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "0000:01:00.0")

		power, err := GetPower("GPU-TEST", dev)
		assert.NoError(t, err)
		assert.Equal(t, "GPU-TEST", power.UUID)
		assert.Equal(t, "0000:01:00.0", power.BusID)
		assert.Equal(t, uint32(150000), power.UsageMilliWatts)
		assert.Equal(t, uint32(250000), power.EnforcedLimitMilliWatts)
		assert.Equal(t, uint32(300000), power.ManagementLimitMilliWatts)
		assert.Equal(t, "60.00", power.UsedPercent) // 150000/250000 * 100 = 60.00
	})
}

// TestGetPower_EnforcedLimitNotSupported_UsesManagement_WithMockey tests fallback to management limit
func TestGetPower_EnforcedLimitNotSupported_UsesManagement_WithMockey(t *testing.T) {
	mockey.PatchConvey("GetPower fallback to management limit when enforced not supported", t, func() {
		mockDevice := &mock.Device{
			GetPowerUsageFunc: func() (uint32, nvml.Return) {
				return 100000, nvml.SUCCESS
			},
			GetEnforcedPowerLimitFunc: func() (uint32, nvml.Return) {
				return 0, nvml.ERROR_NOT_SUPPORTED
			},
			GetPowerManagementLimitFunc: func() (uint32, nvml.Return) {
				return 200000, nvml.SUCCESS
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "GPU-TEST", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		power, err := GetPower("GPU-TEST", dev)
		assert.NoError(t, err)
		assert.False(t, power.GetPowerLimitSupported)
		assert.Equal(t, uint32(200000), power.ManagementLimitMilliWatts)
		assert.Equal(t, "50.00", power.UsedPercent) // 100000/200000 * 100 = 50.00 (fallback to management)
	})
}

// TestGetPower_ManagementLimitNotSupported_WithMockey tests when management limit is not supported
func TestGetPower_ManagementLimitNotSupported_WithMockey(t *testing.T) {
	mockey.PatchConvey("GetPower when management limit not supported", t, func() {
		mockDevice := &mock.Device{
			GetPowerUsageFunc: func() (uint32, nvml.Return) {
				return 100000, nvml.SUCCESS
			},
			GetEnforcedPowerLimitFunc: func() (uint32, nvml.Return) {
				return 200000, nvml.SUCCESS
			},
			GetPowerManagementLimitFunc: func() (uint32, nvml.Return) {
				return 0, nvml.ERROR_NOT_SUPPORTED
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "GPU-TEST", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		power, err := GetPower("GPU-TEST", dev)
		assert.NoError(t, err)
		assert.False(t, power.GetPowerManagementLimitSupported)
		assert.Equal(t, uint32(200000), power.EnforcedLimitMilliWatts)
		assert.Equal(t, "50.00", power.UsedPercent) // 100000/200000 * 100 = 50.00
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

// TestCheckResult_HealthStates_WithSuggestedActions_WithMockey tests HealthStates with suggested actions
func TestCheckResult_HealthStates_WithSuggestedActions_WithMockey(t *testing.T) {
	mockey.PatchConvey("HealthStates with suggested actions", t, func() {
		cr := &checkResult{
			ts:     time.Now(),
			health: apiv1.HealthStateTypeUnhealthy,
			reason: "GPU requires reset",
			err:    nvmlerrors.ErrGPURequiresReset,
			suggestedActions: &apiv1.SuggestedActions{
				Description: "GPU requires reset",
				RepairActions: []apiv1.RepairActionType{
					apiv1.RepairActionTypeRebootSystem,
				},
			},
		}

		states := cr.HealthStates()
		require.Len(t, states, 1)
		assert.Equal(t, Name, states[0].Component)
		assert.Equal(t, Name, states[0].Name)
		assert.Equal(t, apiv1.HealthStateTypeUnhealthy, states[0].Health)
		assert.Equal(t, "GPU requires reset", states[0].Reason)
		assert.Equal(t, nvmlerrors.ErrGPURequiresReset.Error(), states[0].Error)
		assert.NotNil(t, states[0].SuggestedActions)
		assert.Equal(t, "GPU requires reset", states[0].SuggestedActions.Description)
		assert.Contains(t, states[0].SuggestedActions.RepairActions, apiv1.RepairActionTypeRebootSystem)
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
		assert.Equal(t, "accelerator-nvidia-power", comp.Name())
	})
}

// TestCheck_GenericPowerError_WithMockey tests Check with a generic power error
func TestCheck_GenericPowerError_WithMockey(t *testing.T) {
	mockey.PatchConvey("Check with generic power error", t, func() {
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

		genericErr := errors.New("some generic power error")
		getPowerFunc := func(uuid string, dev device.Device) (Power, error) {
			return Power{}, genericErr
		}

		comp := createMockPowerComponent(ctx, mockNvml, getPowerFunc).(*component)
		result := comp.Check()
		cr, ok := result.(*checkResult)
		require.True(t, ok)

		assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
		assert.Equal(t, "error getting power", cr.reason)
		assert.Equal(t, genericErr, cr.err)
		assert.Nil(t, cr.suggestedActions) // No suggested actions for generic errors
	})
}

// TestCheck_NoDevices_WithMockey tests Check with no devices
func TestCheck_NoDevices_WithMockey(t *testing.T) {
	mockey.PatchConvey("Check with no devices", t, func() {
		ctx := context.Background()

		mockNvml := &customMockNVMLInstanceForMockey{
			devs:        map[string]device.Device{},
			nvmlExists:  true,
			productName: "NVIDIA H100",
		}

		comp := createMockPowerComponent(ctx, mockNvml, nil).(*component)
		result := comp.Check()
		cr, ok := result.(*checkResult)
		require.True(t, ok)

		assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
		assert.Contains(t, cr.reason, "0 GPU(s) were checked")
		assert.Empty(t, cr.Powers)
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

// TestCheckResult_String_WithMultiplePowers_WithMockey tests String with multiple power readings
func TestCheckResult_String_WithMultiplePowers_WithMockey(t *testing.T) {
	mockey.PatchConvey("String with multiple powers", t, func() {
		cr := &checkResult{
			Powers: []Power{
				{
					UUID:                    "gpu-1",
					BusID:                   "0000:01:00.0",
					UsageMilliWatts:         150000,
					EnforcedLimitMilliWatts: 250000,
					UsedPercent:             "60.00",
				},
				{
					UUID:                    "gpu-2",
					BusID:                   "0000:02:00.0",
					UsageMilliWatts:         200000,
					EnforcedLimitMilliWatts: 300000,
					UsedPercent:             "66.67",
				},
			},
		}
		result := cr.String()
		assert.Contains(t, result, "gpu-1")
		assert.Contains(t, result, "gpu-2")
		assert.Contains(t, result, "0000:01:00.0")
		assert.Contains(t, result, "0000:02:00.0")
		assert.Contains(t, result, "60.00")
		assert.Contains(t, result, "66.67")
	})
}

// TestGetPower_BothLimitsZero_WithMockey tests UsedPercent when both limits are zero
func TestGetPower_BothLimitsZero_WithMockey(t *testing.T) {
	mockey.PatchConvey("GetPower with both limits zero", t, func() {
		mockDevice := &mock.Device{
			GetPowerUsageFunc: func() (uint32, nvml.Return) {
				return 100000, nvml.SUCCESS
			},
			GetEnforcedPowerLimitFunc: func() (uint32, nvml.Return) {
				return 0, nvml.SUCCESS
			},
			GetPowerManagementLimitFunc: func() (uint32, nvml.Return) {
				return 0, nvml.SUCCESS
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "GPU-TEST", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		power, err := GetPower("GPU-TEST", dev)
		assert.NoError(t, err)
		assert.Equal(t, "0.0", power.UsedPercent) // When total is 0, UsedPercent is "0.0"
	})
}

// TestGetPower_OverUsage_WithMockey tests UsedPercent when usage exceeds limit
func TestGetPower_OverUsage_WithMockey(t *testing.T) {
	mockey.PatchConvey("GetPower with over usage", t, func() {
		mockDevice := &mock.Device{
			GetPowerUsageFunc: func() (uint32, nvml.Return) {
				return 300000, nvml.SUCCESS // Over the limit
			},
			GetEnforcedPowerLimitFunc: func() (uint32, nvml.Return) {
				return 200000, nvml.SUCCESS
			},
			GetPowerManagementLimitFunc: func() (uint32, nvml.Return) {
				return 250000, nvml.SUCCESS
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "GPU-TEST", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		power, err := GetPower("GPU-TEST", dev)
		assert.NoError(t, err)
		assert.Equal(t, "150.00", power.UsedPercent) // 300000/200000 * 100 = 150.00
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

		mockNvml := &customMockNVMLInstanceForMockey{
			devs:        devs,
			nvmlExists:  true,
			productName: "NVIDIA H100",
		}

		callCount := 0
		getPowerFunc := func(uuid string, dev device.Device) (Power, error) {
			callCount++
			// First call fails (map iteration order is random, so we fail on the first call)
			if callCount == 1 {
				return Power{}, nvmlerrors.ErrGPULost
			}
			return Power{
				UUID:                    uuid,
				UsageMilliWatts:         150000,
				EnforcedLimitMilliWatts: 250000,
				UsedPercent:             "60.00",
			}, nil
		}

		comp := createMockPowerComponent(ctx, mockNvml, getPowerFunc).(*component)
		result := comp.Check()
		cr, ok := result.(*checkResult)
		require.True(t, ok)

		// Should be unhealthy because first GPU failed
		assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
		assert.Equal(t, nvmlerrors.ErrGPULost.Error(), cr.reason)
	})
}

// TestCheckResult_getError_NilError_WithMockey tests getError with nil error
func TestCheckResult_getError_NilError_WithMockey(t *testing.T) {
	mockey.PatchConvey("getError with nil error", t, func() {
		cr := &checkResult{
			health: apiv1.HealthStateTypeHealthy,
			reason: "all good",
			err:    nil,
		}
		assert.Equal(t, "", cr.getError())
	})
}

// TestCheckResult_getError_WithError_WithMockey tests getError with an error
func TestCheckResult_getError_WithError_WithMockey(t *testing.T) {
	mockey.PatchConvey("getError with error", t, func() {
		cr := &checkResult{
			health: apiv1.HealthStateTypeUnhealthy,
			reason: "error occurred",
			err:    errors.New("test error message"),
		}
		assert.Equal(t, "test error message", cr.getError())
	})
}

// TestCheckResult_HealthStates_NoExtraInfo_WithMockey tests HealthStates without ExtraInfo
func TestCheckResult_HealthStates_NoExtraInfo_WithMockey(t *testing.T) {
	mockey.PatchConvey("HealthStates no extra info", t, func() {
		cr := &checkResult{
			ts:     time.Now(),
			health: apiv1.HealthStateTypeHealthy,
			reason: "healthy with no data",
			Powers: nil,
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

		mockInst := &customMockNVMLInstanceForMockey{
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
			getPowerFunc: func(uuid string, dev device.Device) (Power, error) {
				return Power{}, nil
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

		mockNvml := &customMockNVMLInstanceForMockey{
			devs:        devs,
			nvmlExists:  true,
			productName: "NVIDIA H100",
		}

		getPowerFunc := func(uuid string, dev device.Device) (Power, error) {
			return Power{
				UUID:                    uuid,
				UsageMilliWatts:         150000,
				EnforcedLimitMilliWatts: 250000,
				UsedPercent:             "60.00",
			}, nil
		}

		comp := createMockPowerComponent(ctx, mockNvml, getPowerFunc).(*component)

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

// TestGetPower_Success_AllFieldsPopulated_WithMockey tests GetPower populates all fields correctly
func TestGetPower_Success_AllFieldsPopulated_WithMockey(t *testing.T) {
	mockey.PatchConvey("GetPower populates all fields", t, func() {
		mockDevice := &mock.Device{
			GetPowerUsageFunc: func() (uint32, nvml.Return) {
				return 175000, nvml.SUCCESS
			},
			GetEnforcedPowerLimitFunc: func() (uint32, nvml.Return) {
				return 350000, nvml.SUCCESS
			},
			GetPowerManagementLimitFunc: func() (uint32, nvml.Return) {
				return 400000, nvml.SUCCESS
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "GPU-TEST-FULL", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "Ampere", "NVIDIA", "12.0", "0000:81:00.0")

		power, err := GetPower("GPU-TEST-FULL", dev)
		assert.NoError(t, err)
		assert.Equal(t, "GPU-TEST-FULL", power.UUID)
		assert.Equal(t, "0000:81:00.0", power.BusID)
		assert.Equal(t, uint32(175000), power.UsageMilliWatts)
		assert.Equal(t, uint32(350000), power.EnforcedLimitMilliWatts)
		assert.Equal(t, uint32(400000), power.ManagementLimitMilliWatts)
		assert.True(t, power.GetPowerUsageSupported)
		assert.True(t, power.GetPowerLimitSupported)
		assert.True(t, power.GetPowerManagementLimitSupported)
		assert.Equal(t, "50.00", power.UsedPercent) // 175000/350000 * 100 = 50.00
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

// TestCheck_GetPowerReturnsGenericError_WithMockey tests Check with a non-GPU-specific error
func TestCheck_GetPowerReturnsGenericError_WithMockey(t *testing.T) {
	mockey.PatchConvey("Check with generic getPower error", t, func() {
		ctx := context.Background()

		uuid := "gpu-uuid-generic"
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

		genericErr := errors.New("generic power retrieval error")
		getPowerFunc := func(uuid string, dev device.Device) (Power, error) {
			return Power{}, genericErr
		}

		comp := createMockPowerComponent(ctx, mockNvml, getPowerFunc).(*component)
		result := comp.Check()
		cr, ok := result.(*checkResult)
		require.True(t, ok)

		assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
		assert.Equal(t, "error getting power", cr.reason)
		assert.Equal(t, genericErr, cr.err)
		// No suggested actions for generic errors
		assert.Nil(t, cr.suggestedActions)
	})
}

// TestGetPower_UsageExactlyAtLimit_WithMockey tests GetPower when usage equals limit
func TestGetPower_UsageExactlyAtLimit_WithMockey(t *testing.T) {
	mockey.PatchConvey("GetPower with usage exactly at limit", t, func() {
		mockDevice := &mock.Device{
			GetPowerUsageFunc: func() (uint32, nvml.Return) {
				return 250000, nvml.SUCCESS
			},
			GetEnforcedPowerLimitFunc: func() (uint32, nvml.Return) {
				return 250000, nvml.SUCCESS
			},
			GetPowerManagementLimitFunc: func() (uint32, nvml.Return) {
				return 300000, nvml.SUCCESS
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "GPU-TEST", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		power, err := GetPower("GPU-TEST", dev)
		assert.NoError(t, err)
		assert.Equal(t, "100.00", power.UsedPercent)
	})
}

// TestGetPower_VerySmallUsage_WithMockey tests GetPower with very small power usage
func TestGetPower_VerySmallUsage_WithMockey(t *testing.T) {
	mockey.PatchConvey("GetPower with very small usage", t, func() {
		mockDevice := &mock.Device{
			GetPowerUsageFunc: func() (uint32, nvml.Return) {
				return 1, nvml.SUCCESS
			},
			GetEnforcedPowerLimitFunc: func() (uint32, nvml.Return) {
				return 1000000, nvml.SUCCESS
			},
			GetPowerManagementLimitFunc: func() (uint32, nvml.Return) {
				return 1000000, nvml.SUCCESS
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "GPU-TEST", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		power, err := GetPower("GPU-TEST", dev)
		assert.NoError(t, err)
		assert.Equal(t, "0.00", power.UsedPercent) // 1/1000000 * 100 = 0.0001 rounds to 0.00
	})
}
