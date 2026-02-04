//go:build linux

package ecc

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
		mockInstance := &mockNVMLInstance{
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
		assert.NotNil(t, tc.getECCModeEnabledFunc)
		assert.NotNil(t, tc.getECCErrorsFunc)
	})
}

// TestComponent_IsSupported tests IsSupported method with various conditions
func TestComponent_IsSupported(t *testing.T) {
	testCases := []struct {
		name         string
		nvmlInstance func() *mockNVMLInstance
		expected     bool
		setupNilNVML bool
		nvmlExists   bool
		productName  string
	}{
		{
			name:         "nil NVML instance returns false",
			setupNilNVML: true,
			expected:     false,
		},
		{
			name: "NVML not loaded returns false",
			nvmlInstance: func() *mockNVMLInstance {
				return &mockNVMLInstance{
					devicesFunc: func() map[string]device.Device { return nil },
				}
			},
			nvmlExists:  false,
			productName: "NVIDIA H100",
			expected:    false,
		},
		{
			name: "no product name returns false",
			nvmlInstance: func() *mockNVMLInstance {
				return &mockNVMLInstance{
					devicesFunc: func() map[string]device.Device { return nil },
				}
			},
			nvmlExists:  true,
			productName: "",
			expected:    false,
		},
		{
			name: "NVML loaded with product name returns true",
			nvmlInstance: func() *mockNVMLInstance {
				return &mockNVMLInstance{
					devicesFunc: func() map[string]device.Device { return nil },
				}
			},
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
					mockInst := tc.nvmlInstance()
					// Create a custom instance that overrides NVMLExists and ProductName
					customMock := &customMockNVMLInstance{
						devs:        map[string]device.Device{},
						nvmlExists:  tc.nvmlExists,
						productName: tc.productName,
					}
					_ = mockInst // unused but shows the pattern
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

// customMockNVMLInstance with customizable NVMLExists and ProductName
type customMockNVMLInstance struct {
	devs        map[string]device.Device
	nvmlExists  bool
	productName string
}

func (m *customMockNVMLInstance) Devices() map[string]device.Device { return m.devs }
func (m *customMockNVMLInstance) FabricManagerSupported() bool      { return true }
func (m *customMockNVMLInstance) FabricStateSupported() bool        { return false }
func (m *customMockNVMLInstance) GetMemoryErrorManagementCapabilities() nvidiaproduct.MemoryErrorManagementCapabilities {
	return nvidiaproduct.MemoryErrorManagementCapabilities{}
}
func (m *customMockNVMLInstance) ProductName() string   { return m.productName }
func (m *customMockNVMLInstance) Architecture() string  { return "" }
func (m *customMockNVMLInstance) Brand() string         { return "" }
func (m *customMockNVMLInstance) DriverVersion() string { return "" }
func (m *customMockNVMLInstance) DriverMajor() int      { return 0 }
func (m *customMockNVMLInstance) CUDAVersion() string   { return "" }
func (m *customMockNVMLInstance) NVMLExists() bool      { return m.nvmlExists }
func (m *customMockNVMLInstance) Library() lib.Library  { return nil }
func (m *customMockNVMLInstance) Shutdown() error       { return nil }
func (m *customMockNVMLInstance) InitError() error      { return nil }

// TestCheck_InitError tests Check when NVML has an initialization error
func TestCheck_InitError(t *testing.T) {
	mockey.PatchConvey("Check with NVML init error", t, func() {
		ctx := context.Background()
		cctx, cancel := context.WithCancel(ctx)
		defer cancel()

		initErr := errors.New("error getting device handle for index '0': Unknown Error")
		mockInst := &mockNVMLInstanceWithInitError{
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

// mockNVMLInstanceWithInitError returns an init error
type mockNVMLInstanceWithInitError struct {
	devs      map[string]device.Device
	initError error
}

func (m *mockNVMLInstanceWithInitError) Devices() map[string]device.Device { return m.devs }
func (m *mockNVMLInstanceWithInitError) FabricManagerSupported() bool      { return true }
func (m *mockNVMLInstanceWithInitError) FabricStateSupported() bool        { return false }
func (m *mockNVMLInstanceWithInitError) GetMemoryErrorManagementCapabilities() nvidiaproduct.MemoryErrorManagementCapabilities {
	return nvidiaproduct.MemoryErrorManagementCapabilities{}
}
func (m *mockNVMLInstanceWithInitError) ProductName() string   { return "NVIDIA H100" }
func (m *mockNVMLInstanceWithInitError) Architecture() string  { return "" }
func (m *mockNVMLInstanceWithInitError) Brand() string         { return "" }
func (m *mockNVMLInstanceWithInitError) DriverVersion() string { return "" }
func (m *mockNVMLInstanceWithInitError) DriverMajor() int      { return 0 }
func (m *mockNVMLInstanceWithInitError) CUDAVersion() string   { return "" }
func (m *mockNVMLInstanceWithInitError) NVMLExists() bool      { return true }
func (m *mockNVMLInstanceWithInitError) Library() lib.Library  { return nil }
func (m *mockNVMLInstanceWithInitError) Shutdown() error       { return nil }
func (m *mockNVMLInstanceWithInitError) InitError() error      { return m.initError }

// TestCheck_MissingProductName tests Check when product name is empty
func TestCheck_MissingProductName(t *testing.T) {
	mockey.PatchConvey("Check with missing product name", t, func() {
		ctx := context.Background()
		cctx, cancel := context.WithCancel(ctx)
		defer cancel()

		mockInst := &customMockNVMLInstance{
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

// TestGetECCModeEnabled_WithMockey tests GetECCModeEnabled with mocked device
func TestGetECCModeEnabled_WithMockey(t *testing.T) {
	testCases := []struct {
		name          string
		currentECC    nvml.EnableState
		pendingECC    nvml.EnableState
		eccModeRet    nvml.Return
		expectedMode  ECCMode
		expectedErr   error
		expectError   bool
		errorContains string
	}{
		{
			name:       "both enabled",
			currentECC: nvml.FEATURE_ENABLED,
			pendingECC: nvml.FEATURE_ENABLED,
			eccModeRet: nvml.SUCCESS,
			expectedMode: ECCMode{
				UUID:           "test-uuid",
				BusID:          "test-pci",
				EnabledCurrent: true,
				EnabledPending: true,
				Supported:      true,
			},
			expectError: false,
		},
		{
			name:        "GPU lost error",
			currentECC:  nvml.FEATURE_DISABLED,
			pendingECC:  nvml.FEATURE_DISABLED,
			eccModeRet:  nvml.ERROR_GPU_IS_LOST,
			expectedErr: nvmlerrors.ErrGPULost,
			expectError: true,
		},
		{
			name:        "GPU requires reset",
			currentECC:  nvml.FEATURE_DISABLED,
			pendingECC:  nvml.FEATURE_DISABLED,
			eccModeRet:  nvml.ERROR_RESET_REQUIRED,
			expectedErr: nvmlerrors.ErrGPURequiresReset,
			expectError: true,
		},
		{
			name:          "Unknown error",
			currentECC:    nvml.FEATURE_DISABLED,
			pendingECC:    nvml.FEATURE_DISABLED,
			eccModeRet:    nvml.ERROR_UNKNOWN,
			expectError:   true,
			errorContains: "failed to get current/pending ecc mode",
		},
		{
			name:       "Not supported",
			currentECC: nvml.FEATURE_DISABLED,
			pendingECC: nvml.FEATURE_DISABLED,
			eccModeRet: nvml.ERROR_NOT_SUPPORTED,
			expectedMode: ECCMode{
				UUID:           "test-uuid",
				BusID:          "test-pci",
				EnabledCurrent: false,
				EnabledPending: false,
				Supported:      false,
			},
			expectError: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockey.PatchConvey(tc.name, t, func() {
				mockDevice := &mock.Device{
					GetEccModeFunc: func() (nvml.EnableState, nvml.EnableState, nvml.Return) {
						return tc.currentECC, tc.pendingECC, tc.eccModeRet
					},
					GetUUIDFunc: func() (string, nvml.Return) {
						return "test-uuid", nvml.SUCCESS
					},
				}

				dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

				result, err := GetECCModeEnabled("test-uuid", dev)

				if tc.expectError {
					assert.Error(t, err)
					if tc.expectedErr != nil {
						assert.True(t, errors.Is(err, tc.expectedErr))
					}
					if tc.errorContains != "" {
						assert.Contains(t, err.Error(), tc.errorContains)
					}
				} else {
					assert.NoError(t, err)
					assert.Equal(t, tc.expectedMode.EnabledCurrent, result.EnabledCurrent)
					assert.Equal(t, tc.expectedMode.EnabledPending, result.EnabledPending)
					assert.Equal(t, tc.expectedMode.Supported, result.Supported)
				}
			})
		})
	}
}

// TestGetECCErrors_GPURequiresReset tests GetECCErrors with GPU requires reset error
func TestGetECCErrors_GPURequiresReset(t *testing.T) {
	mockey.PatchConvey("GetECCErrors with GPU requires reset", t, func() {
		mockDevice := &mock.Device{
			GetTotalEccErrorsFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType) (uint64, nvml.Return) {
				return 0, nvml.ERROR_RESET_REQUIRED
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		_, err := GetECCErrors("test-uuid", dev, true)

		assert.Error(t, err)
		assert.True(t, errors.Is(err, nvmlerrors.ErrGPURequiresReset))
	})
}

// TestGetECCErrors_VolatileGPULost tests GetECCErrors with GPU lost error on volatile ECC
func TestGetECCErrors_VolatileGPULost(t *testing.T) {
	mockey.PatchConvey("GetECCErrors with volatile GPU lost", t, func() {
		callCount := 0
		mockDevice := &mock.Device{
			GetTotalEccErrorsFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType) (uint64, nvml.Return) {
				callCount++
				// First two calls succeed (aggregate corrected and uncorrected)
				if callCount <= 2 {
					return 0, nvml.SUCCESS
				}
				// Third call (volatile corrected) returns GPU lost
				return 0, nvml.ERROR_GPU_IS_LOST
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		_, err := GetECCErrors("test-uuid", dev, false)

		assert.Error(t, err)
		assert.True(t, errors.Is(err, nvmlerrors.ErrGPULost))
	})
}

// TestGetECCErrors_MemoryErrorCounterGPULost tests memory error counter with GPU lost
func TestGetECCErrors_MemoryErrorCounterGPULost(t *testing.T) {
	mockey.PatchConvey("GetECCErrors memory error counter GPU lost", t, func() {
		mockDevice := &mock.Device{
			GetTotalEccErrorsFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType) (uint64, nvml.Return) {
				return 0, nvml.SUCCESS
			},
			GetMemoryErrorCounterFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType, location nvml.MemoryLocation) (uint64, nvml.Return) {
				return 0, nvml.ERROR_GPU_IS_LOST
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		// With ECC mode enabled, it will try to get memory error counters
		_, err := GetECCErrors("test-uuid", dev, true)

		assert.Error(t, err)
		assert.True(t, errors.Is(err, nvmlerrors.ErrGPULost))
	})
}

// TestCheck_ConcurrentAccess tests concurrent access to Check and LastHealthStates
func TestCheck_ConcurrentAccess(t *testing.T) {
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

		getDevicesFunc := func() map[string]device.Device {
			return devs
		}

		getECCModeEnabledFunc := func(uuid string, dev device.Device) (ECCMode, error) {
			return ECCMode{
				UUID:           uuid,
				EnabledCurrent: true,
				Supported:      true,
			}, nil
		}

		getECCErrorsFunc := func(uuid string, dev device.Device, eccModeEnabled bool) (ECCErrors, error) {
			return ECCErrors{
				UUID:      uuid,
				Supported: true,
			}, nil
		}

		comp := MockECCComponent(ctx, getDevicesFunc, getECCModeEnabledFunc, getECCErrorsFunc).(*component)

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

// TestCheckResult_Methods tests all checkResult methods
func TestCheckResult_Methods(t *testing.T) {
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

// TestLastHealthStates_SuggestedActionsPropagate tests suggested actions propagation
func TestLastHealthStates_SuggestedActionsPropagate(t *testing.T) {
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

		getDevicesFunc := func() map[string]device.Device {
			return devs
		}

		// Simulate GPU requires reset error
		getECCModeEnabledFunc := func(uuid string, dev device.Device) (ECCMode, error) {
			return ECCMode{}, nvmlerrors.ErrGPURequiresReset
		}

		comp := MockECCComponent(ctx, getDevicesFunc, getECCModeEnabledFunc, nil).(*component)
		comp.Check()

		states := comp.LastHealthStates()
		require.Len(t, states, 1)
		assert.Equal(t, apiv1.HealthStateTypeUnhealthy, states[0].Health)
		assert.NotNil(t, states[0].SuggestedActions)
		assert.Contains(t, states[0].SuggestedActions.RepairActions, apiv1.RepairActionTypeRebootSystem)
	})
}

// TestGetECCErrors_AggregateUncorrectedGPULost tests aggregate uncorrected with GPU lost
func TestGetECCErrors_AggregateUncorrectedGPULost(t *testing.T) {
	mockey.PatchConvey("GetECCErrors aggregate uncorrected GPU lost", t, func() {
		callCount := 0
		mockDevice := &mock.Device{
			GetTotalEccErrorsFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType) (uint64, nvml.Return) {
				callCount++
				// First call (aggregate corrected) succeeds
				if callCount == 1 {
					return 0, nvml.SUCCESS
				}
				// Second call (aggregate uncorrected) returns GPU lost
				return 0, nvml.ERROR_GPU_IS_LOST
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		_, err := GetECCErrors("test-uuid", dev, false)

		assert.Error(t, err)
		assert.True(t, errors.Is(err, nvmlerrors.ErrGPULost))
	})
}

// TestGetECCErrors_VolatileUncorrectedGPULost tests volatile uncorrected with GPU lost
func TestGetECCErrors_VolatileUncorrectedGPULost(t *testing.T) {
	mockey.PatchConvey("GetECCErrors volatile uncorrected GPU lost", t, func() {
		callCount := 0
		mockDevice := &mock.Device{
			GetTotalEccErrorsFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType) (uint64, nvml.Return) {
				callCount++
				// First three calls succeed
				if callCount <= 3 {
					return 0, nvml.SUCCESS
				}
				// Fourth call (volatile uncorrected) returns GPU lost
				return 0, nvml.ERROR_GPU_IS_LOST
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		_, err := GetECCErrors("test-uuid", dev, false)

		assert.Error(t, err)
		assert.True(t, errors.Is(err, nvmlerrors.ErrGPULost))
	})
}

// TestGetECCErrors_AllMemoryLocations tests ECC errors collection from all memory locations
func TestGetECCErrors_AllMemoryLocations(t *testing.T) {
	mockey.PatchConvey("GetECCErrors all memory locations", t, func() {
		mockDevice := &mock.Device{
			GetTotalEccErrorsFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType) (uint64, nvml.Return) {
				if errorType == nvml.MEMORY_ERROR_TYPE_CORRECTED {
					return 10, nvml.SUCCESS
				}
				return 5, nvml.SUCCESS
			},
			GetMemoryErrorCounterFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType, location nvml.MemoryLocation) (uint64, nvml.Return) {
				if errorType == nvml.MEMORY_ERROR_TYPE_CORRECTED {
					return 2, nvml.SUCCESS
				}
				return 1, nvml.SUCCESS
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		result, err := GetECCErrors("test-uuid", dev, true)

		assert.NoError(t, err)
		assert.True(t, result.Supported)
		assert.Equal(t, uint64(10), result.Aggregate.Total.Corrected)
		assert.Equal(t, uint64(5), result.Aggregate.Total.Uncorrected)
		assert.Equal(t, uint64(10), result.Volatile.Total.Corrected)
		assert.Equal(t, uint64(5), result.Volatile.Total.Uncorrected)
		// Check memory location counts
		assert.Equal(t, uint64(2), result.Aggregate.L1Cache.Corrected)
		assert.Equal(t, uint64(1), result.Aggregate.L1Cache.Uncorrected)
	})
}

// TestGetECCErrors_ECCModeDisabled tests that memory error counters are skipped when ECC is disabled
func TestGetECCErrors_ECCModeDisabled(t *testing.T) {
	mockey.PatchConvey("GetECCErrors with ECC mode disabled", t, func() {
		memoryErrorCounterCalled := false
		mockDevice := &mock.Device{
			GetTotalEccErrorsFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType) (uint64, nvml.Return) {
				return 0, nvml.SUCCESS
			},
			GetMemoryErrorCounterFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType, location nvml.MemoryLocation) (uint64, nvml.Return) {
				memoryErrorCounterCalled = true
				return 0, nvml.SUCCESS
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		// ECC mode disabled - should skip memory error counter calls
		result, err := GetECCErrors("test-uuid", dev, false)

		assert.NoError(t, err)
		assert.True(t, result.Supported)
		assert.False(t, memoryErrorCounterCalled, "GetMemoryErrorCounter should not be called when ECC mode is disabled")
	})
}

// TestGetECCErrors_MemoryErrorCounterRequiresReset tests memory error counter with GPU requires reset
func TestGetECCErrors_MemoryErrorCounterRequiresReset(t *testing.T) {
	mockey.PatchConvey("GetECCErrors memory error counter requires reset", t, func() {
		mockDevice := &mock.Device{
			GetTotalEccErrorsFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType) (uint64, nvml.Return) {
				return 0, nvml.SUCCESS
			},
			GetMemoryErrorCounterFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType, location nvml.MemoryLocation) (uint64, nvml.Return) {
				return 0, nvml.ERROR_RESET_REQUIRED
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		_, err := GetECCErrors("test-uuid", dev, true)

		assert.Error(t, err)
		assert.True(t, errors.Is(err, nvmlerrors.ErrGPURequiresReset))
	})
}

// TestGetECCErrors_NotSupported tests ECC errors when feature is not supported
func TestGetECCErrors_NotSupported(t *testing.T) {
	mockey.PatchConvey("GetECCErrors not supported", t, func() {
		mockDevice := &mock.Device{
			GetTotalEccErrorsFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType) (uint64, nvml.Return) {
				return 0, nvml.ERROR_NOT_SUPPORTED
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		result, err := GetECCErrors("test-uuid", dev, false)

		assert.NoError(t, err)
		assert.False(t, result.Supported)
	})
}

// TestGetECCErrors_VolatileL2CorrectedNotSupported tests the special case where
// volatile L2 corrected returns NOT_SUPPORTED and is tolerated.
func TestGetECCErrors_VolatileL2CorrectedNotSupported(t *testing.T) {
	mockey.PatchConvey("GetECCErrors volatile L2 corrected not supported", t, func() {
		mockDevice := &mock.Device{
			GetTotalEccErrorsFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType) (uint64, nvml.Return) {
				return 0, nvml.SUCCESS
			},
			GetMemoryErrorCounterFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType, location nvml.MemoryLocation) (uint64, nvml.Return) {
				if counterType == nvml.VOLATILE_ECC && location == nvml.MEMORY_LOCATION_L2_CACHE && errorType == nvml.MEMORY_ERROR_TYPE_CORRECTED {
					return 0, nvml.ERROR_NOT_SUPPORTED
				}
				return 0, nvml.SUCCESS
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		result, err := GetECCErrors("test-uuid", dev, true)

		assert.NoError(t, err)
		assert.True(t, result.Supported)
	})
}

// TestGetECCErrors_VolatileL2CorrectedUnexpectedError tests that unexpected errors
// for volatile L2 corrected are surfaced.
func TestGetECCErrors_VolatileL2CorrectedUnexpectedError(t *testing.T) {
	mockey.PatchConvey("GetECCErrors volatile L2 corrected unexpected error", t, func() {
		mockDevice := &mock.Device{
			GetTotalEccErrorsFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType) (uint64, nvml.Return) {
				return 0, nvml.SUCCESS
			},
			GetMemoryErrorCounterFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType, location nvml.MemoryLocation) (uint64, nvml.Return) {
				if counterType == nvml.VOLATILE_ECC && location == nvml.MEMORY_LOCATION_L2_CACHE && errorType == nvml.MEMORY_ERROR_TYPE_CORRECTED {
					return 0, nvml.ERROR_UNKNOWN
				}
				return 0, nvml.SUCCESS
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		_, err := GetECCErrors("test-uuid", dev, true)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get l2 cache ecc errors")
	})
}

// TestCheck_ECCModeError_WithMockey tests Check when getting ECC mode fails (using mockey)
func TestCheck_ECCModeError_WithMockey(t *testing.T) {
	mockey.PatchConvey("Check with ECC mode error", t, func() {
		ctx := context.Background()

		uuid := "gpu-uuid-mode-error"
		mockDeviceObj := &mock.Device{
			GetUUIDFunc: func() (string, nvml.Return) {
				return uuid, nvml.SUCCESS
			},
		}
		mockDev := testutil.NewMockDevice(mockDeviceObj, "test-arch", "test-brand", "test-cuda", "test-pci")

		devs := map[string]device.Device{
			uuid: mockDev,
		}

		getDevicesFunc := func() map[string]device.Device {
			return devs
		}

		getECCModeEnabledFunc := func(uuid string, dev device.Device) (ECCMode, error) {
			return ECCMode{}, errors.New("unknown ECC mode error")
		}

		comp := MockECCComponent(ctx, getDevicesFunc, getECCModeEnabledFunc, nil).(*component)
		result := comp.Check()
		cr, ok := result.(*checkResult)
		require.True(t, ok)

		assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
		assert.Contains(t, cr.reason, "error getting ECC mode")
	})
}

// TestCheck_ECCErrorsError_WithMockey tests Check when getting ECC errors fails (using mockey)
func TestCheck_ECCErrorsError_WithMockey(t *testing.T) {
	mockey.PatchConvey("Check with ECC errors error", t, func() {
		ctx := context.Background()

		uuid := "gpu-uuid-errors-error"
		mockDeviceObj := &mock.Device{
			GetUUIDFunc: func() (string, nvml.Return) {
				return uuid, nvml.SUCCESS
			},
		}
		mockDev := testutil.NewMockDevice(mockDeviceObj, "test-arch", "test-brand", "test-cuda", "test-pci")

		devs := map[string]device.Device{
			uuid: mockDev,
		}

		getDevicesFunc := func() map[string]device.Device {
			return devs
		}

		getECCModeEnabledFunc := func(uuid string, dev device.Device) (ECCMode, error) {
			return ECCMode{
				UUID:           uuid,
				EnabledCurrent: true,
				Supported:      true,
			}, nil
		}

		getECCErrorsFunc := func(uuid string, dev device.Device, eccModeEnabled bool) (ECCErrors, error) {
			return ECCErrors{}, nvmlerrors.ErrGPULost
		}

		comp := MockECCComponent(ctx, getDevicesFunc, getECCModeEnabledFunc, getECCErrorsFunc).(*component)
		result := comp.Check()
		cr, ok := result.(*checkResult)
		require.True(t, ok)

		assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
		assert.Contains(t, cr.reason, nvmlerrors.ErrGPULost.Error())
		assert.NotNil(t, cr.suggestedActions)
	})
}

// TestCheck_NilNVMLInstance tests Check with nil NVML instance
func TestCheck_NilNVMLInstance(t *testing.T) {
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
func TestCheck_NVMLNotExists(t *testing.T) {
	mockey.PatchConvey("Check with NVML not exists", t, func() {
		ctx := context.Background()
		cctx, cancel := context.WithCancel(ctx)
		defer cancel()

		mockInst := &customMockNVMLInstance{
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

// TestFindUncorrectedErrs tests the FindUncorrectedErrs method
func TestFindUncorrectedErrs(t *testing.T) {
	testCases := []struct {
		name     string
		counts   AllECCErrorCounts
		expected []string
	}{
		{
			name:     "no uncorrected errors",
			counts:   AllECCErrorCounts{},
			expected: nil,
		},
		{
			name: "total uncorrected errors",
			counts: AllECCErrorCounts{
				Total: ECCErrorCounts{Uncorrected: 5},
			},
			expected: []string{"total uncorrected 5 errors"},
		},
		{
			name: "multiple uncorrected errors",
			counts: AllECCErrorCounts{
				Total:   ECCErrorCounts{Uncorrected: 3},
				L1Cache: ECCErrorCounts{Uncorrected: 2},
				DRAM:    ECCErrorCounts{Uncorrected: 1},
			},
			expected: []string{
				"total uncorrected 3 errors",
				"L1 Cache uncorrected 2 errors",
				"DRAM uncorrected 1 errors",
			},
		},
		{
			name: "all memory types with errors",
			counts: AllECCErrorCounts{
				Total:            ECCErrorCounts{Uncorrected: 1},
				L1Cache:          ECCErrorCounts{Uncorrected: 1},
				L2Cache:          ECCErrorCounts{Uncorrected: 1},
				DRAM:             ECCErrorCounts{Uncorrected: 1},
				SRAM:             ECCErrorCounts{Uncorrected: 1},
				GPUDeviceMemory:  ECCErrorCounts{Uncorrected: 1},
				GPUTextureMemory: ECCErrorCounts{Uncorrected: 1},
				SharedMemory:     ECCErrorCounts{Uncorrected: 1},
				GPURegisterFile:  ECCErrorCounts{Uncorrected: 1},
			},
			expected: []string{
				"total uncorrected 1 errors",
				"L1 Cache uncorrected 1 errors",
				"L2 Cache uncorrected 1 errors",
				"DRAM uncorrected 1 errors",
				"SRAM uncorrected 1 errors",
				"GPU device memory uncorrected 1 errors",
				"GPU texture memory uncorrected 1 errors",
				"shared memory uncorrected 1 errors",
				"GPU register file uncorrected 1 errors",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.counts.FindUncorrectedErrs()
			assert.Equal(t, tc.expected, result)
		})
	}
}

// TestCheckResult_String tests the String method of checkResult
func TestCheckResult_String(t *testing.T) {
	t.Run("nil check result", func(t *testing.T) {
		var cr *checkResult
		assert.Equal(t, "", cr.String())
	})

	t.Run("empty ECC modes", func(t *testing.T) {
		cr := &checkResult{}
		assert.Equal(t, "no data", cr.String())
	})

	t.Run("with ECC modes and errors", func(t *testing.T) {
		cr := &checkResult{
			ECCModes: []ECCMode{
				{
					UUID:           "gpu-1",
					BusID:          "0000:01:00.0",
					EnabledCurrent: true,
					EnabledPending: true,
					Supported:      true,
				},
			},
			ECCErrors: []ECCErrors{
				{
					UUID:      "gpu-1",
					BusID:     "0000:01:00.0",
					Supported: true,
				},
			},
		}
		result := cr.String()
		assert.Contains(t, result, "gpu-1")
		assert.Contains(t, result, "0000:01:00.0")
	})
}

// TestCheckResult_Summary tests the Summary method
func TestCheckResult_Summary(t *testing.T) {
	t.Run("nil check result", func(t *testing.T) {
		var cr *checkResult
		assert.Equal(t, "", cr.Summary())
	})

	t.Run("with reason", func(t *testing.T) {
		cr := &checkResult{reason: "test reason"}
		assert.Equal(t, "test reason", cr.Summary())
	})
}

// TestCheckResult_HealthStateType tests the HealthStateType method
func TestCheckResult_HealthStateType(t *testing.T) {
	t.Run("nil check result", func(t *testing.T) {
		var cr *checkResult
		assert.Equal(t, apiv1.HealthStateType(""), cr.HealthStateType())
	})

	t.Run("healthy", func(t *testing.T) {
		cr := &checkResult{health: apiv1.HealthStateTypeHealthy}
		assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.HealthStateType())
	})
}

// TestCheckResult_HealthStates_NilResult tests HealthStates with nil checkResult
func TestCheckResult_HealthStates_NilResult(t *testing.T) {
	var cr *checkResult
	states := cr.HealthStates()
	require.Len(t, states, 1)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, states[0].Health)
	assert.Equal(t, "no data yet", states[0].Reason)
}

// TestCheckResult_HealthStates_WithExtraInfo tests HealthStates with ECCModes and ECCErrors
func TestCheckResult_HealthStates_WithExtraInfo(t *testing.T) {
	cr := &checkResult{
		ts:     time.Now(),
		health: apiv1.HealthStateTypeHealthy,
		reason: "all good",
		ECCModes: []ECCMode{
			{UUID: "gpu-1", EnabledCurrent: true},
		},
		ECCErrors: []ECCErrors{
			{UUID: "gpu-1", Supported: true},
		},
	}

	states := cr.HealthStates()
	require.Len(t, states, 1)
	assert.NotEmpty(t, states[0].ExtraInfo)
	assert.Contains(t, states[0].ExtraInfo["data"], "gpu-1")
}

// TestComponentStart_WithMockey tests the Start method behavior
func TestComponentStart_WithMockey(t *testing.T) {
	mockey.PatchConvey("Start method", t, func() {
		ctx := context.Background()
		cctx, cancel := context.WithCancel(ctx)

		checkCalled := &atomic.Int32{}
		mockInst := &customMockNVMLInstance{
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
			getECCModeEnabledFunc: func(uuid string, dev device.Device) (ECCMode, error) {
				checkCalled.Add(1)
				return ECCMode{}, nil
			},
			getECCErrorsFunc: func(uuid string, dev device.Device, eccModeEnabledCurrent bool) (ECCErrors, error) {
				return ECCErrors{}, nil
			},
		}

		err := comp.Start()
		assert.NoError(t, err)

		// Give the goroutine time to execute
		time.Sleep(50 * time.Millisecond)

		// Close to stop the goroutine
		cancel()
	})
}

// TestComponentClose_WithMockey tests the Close method behavior
func TestComponentClose_WithMockey(t *testing.T) {
	mockey.PatchConvey("Close method", t, func() {
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
			// Good - context is canceled
		default:
			t.Fatal("context should be canceled after Close")
		}
	})
}

// TestComponentEvents_WithMockey tests the Events method
func TestComponentEvents_WithMockey(t *testing.T) {
	mockey.PatchConvey("Events method", t, func() {
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

// TestComponentTags_WithMockey tests the Tags method
func TestComponentTags_WithMockey(t *testing.T) {
	mockey.PatchConvey("Tags method", t, func() {
		comp := &component{}
		tags := comp.Tags()

		assert.Contains(t, tags, "accelerator")
		assert.Contains(t, tags, "gpu")
		assert.Contains(t, tags, "nvidia")
		assert.Contains(t, tags, Name)
		assert.Len(t, tags, 4)
	})
}

// TestGetECCErrors_AggregateUnknownError tests aggregate ECC with unknown error
func TestGetECCErrors_AggregateUnknownError(t *testing.T) {
	mockey.PatchConvey("GetECCErrors aggregate unknown error", t, func() {
		mockDevice := &mock.Device{
			GetTotalEccErrorsFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType) (uint64, nvml.Return) {
				return 0, nvml.ERROR_UNKNOWN
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		_, err := GetECCErrors("test-uuid", dev, false)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get total ecc errors")
	})
}

// TestGetECCErrors_VolatileUnknownError tests volatile ECC with unknown error
func TestGetECCErrors_VolatileUnknownError(t *testing.T) {
	mockey.PatchConvey("GetECCErrors volatile unknown error", t, func() {
		callCount := 0
		mockDevice := &mock.Device{
			GetTotalEccErrorsFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType) (uint64, nvml.Return) {
				callCount++
				// First two calls succeed (aggregate)
				if callCount <= 2 {
					return 0, nvml.SUCCESS
				}
				// Third call (volatile corrected) returns unknown error
				return 0, nvml.ERROR_UNKNOWN
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		_, err := GetECCErrors("test-uuid", dev, false)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get total ecc errors")
	})
}

// TestGetECCErrors_MemoryCounterL1CacheError tests memory error counter L1 cache error
func TestGetECCErrors_MemoryCounterL1CacheError(t *testing.T) {
	mockey.PatchConvey("GetECCErrors memory counter L1 cache error", t, func() {
		mockDevice := &mock.Device{
			GetTotalEccErrorsFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType) (uint64, nvml.Return) {
				return 0, nvml.SUCCESS
			},
			GetMemoryErrorCounterFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType, location nvml.MemoryLocation) (uint64, nvml.Return) {
				if location == nvml.MEMORY_LOCATION_L1_CACHE && errorType == nvml.MEMORY_ERROR_TYPE_UNCORRECTED {
					return 0, nvml.ERROR_UNKNOWN
				}
				return 0, nvml.SUCCESS
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		_, err := GetECCErrors("test-uuid", dev, true)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get l1 cache ecc errors")
	})
}

// TestGetECCErrors_MemoryCounterDRAMError tests memory error counter DRAM error
func TestGetECCErrors_MemoryCounterDRAMError(t *testing.T) {
	mockey.PatchConvey("GetECCErrors memory counter DRAM error", t, func() {
		callCount := 0
		mockDevice := &mock.Device{
			GetTotalEccErrorsFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType) (uint64, nvml.Return) {
				return 0, nvml.SUCCESS
			},
			GetMemoryErrorCounterFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType, location nvml.MemoryLocation) (uint64, nvml.Return) {
				callCount++
				// L1 and L2 cache succeed
				if location == nvml.MEMORY_LOCATION_L1_CACHE || location == nvml.MEMORY_LOCATION_L2_CACHE {
					return 0, nvml.SUCCESS
				}
				// DRAM fails
				if location == nvml.MEMORY_LOCATION_DRAM {
					return 0, nvml.ERROR_UNKNOWN
				}
				return 0, nvml.SUCCESS
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		_, err := GetECCErrors("test-uuid", dev, true)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get dram")
	})
}

// TestGetECCErrors_MemoryCounterSRAMGPULost tests memory error counter SRAM GPU lost
func TestGetECCErrors_MemoryCounterSRAMGPULost(t *testing.T) {
	mockey.PatchConvey("GetECCErrors memory counter SRAM GPU lost", t, func() {
		mockDevice := &mock.Device{
			GetTotalEccErrorsFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType) (uint64, nvml.Return) {
				return 0, nvml.SUCCESS
			},
			GetMemoryErrorCounterFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType, location nvml.MemoryLocation) (uint64, nvml.Return) {
				// L1, L2, DRAM succeed
				if location == nvml.MEMORY_LOCATION_L1_CACHE ||
					location == nvml.MEMORY_LOCATION_L2_CACHE ||
					location == nvml.MEMORY_LOCATION_DRAM {
					return 0, nvml.SUCCESS
				}
				// SRAM returns GPU lost
				if location == nvml.MEMORY_LOCATION_SRAM {
					return 0, nvml.ERROR_GPU_IS_LOST
				}
				return 0, nvml.SUCCESS
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		_, err := GetECCErrors("test-uuid", dev, true)

		assert.Error(t, err)
		assert.True(t, errors.Is(err, nvmlerrors.ErrGPULost))
	})
}

// TestGetECCErrors_MemoryCounterDeviceMemoryNotSupported tests device memory not supported
func TestGetECCErrors_MemoryCounterDeviceMemoryNotSupported(t *testing.T) {
	mockey.PatchConvey("GetECCErrors memory counter device memory not supported", t, func() {
		mockDevice := &mock.Device{
			GetTotalEccErrorsFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType) (uint64, nvml.Return) {
				return 0, nvml.SUCCESS
			},
			GetMemoryErrorCounterFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType, location nvml.MemoryLocation) (uint64, nvml.Return) {
				// Aggregate device memory not supported
				if location == nvml.MEMORY_LOCATION_DEVICE_MEMORY && counterType == nvml.AGGREGATE_ECC {
					return 0, nvml.ERROR_NOT_SUPPORTED
				}
				return 0, nvml.SUCCESS
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		result, err := GetECCErrors("test-uuid", dev, true)

		assert.NoError(t, err)
		assert.False(t, result.Supported)
	})
}

// TestGetECCErrors_MemoryCounterTextureMemoryError tests texture memory error
func TestGetECCErrors_MemoryCounterTextureMemoryError(t *testing.T) {
	mockey.PatchConvey("GetECCErrors memory counter texture memory error", t, func() {
		mockDevice := &mock.Device{
			GetTotalEccErrorsFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType) (uint64, nvml.Return) {
				return 0, nvml.SUCCESS
			},
			GetMemoryErrorCounterFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType, location nvml.MemoryLocation) (uint64, nvml.Return) {
				// Success for L1, L2, DRAM, SRAM, device memory
				if location == nvml.MEMORY_LOCATION_L1_CACHE ||
					location == nvml.MEMORY_LOCATION_L2_CACHE ||
					location == nvml.MEMORY_LOCATION_DRAM ||
					location == nvml.MEMORY_LOCATION_SRAM ||
					location == nvml.MEMORY_LOCATION_DEVICE_MEMORY {
					return 0, nvml.SUCCESS
				}
				// Texture memory error
				if location == nvml.MEMORY_LOCATION_TEXTURE_MEMORY {
					return 0, nvml.ERROR_UNKNOWN
				}
				return 0, nvml.SUCCESS
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		_, err := GetECCErrors("test-uuid", dev, true)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "texture memory")
	})
}

// TestGetECCErrors_VolatileL2CacheGPULost tests volatile L2 cache GPU lost
func TestGetECCErrors_VolatileL2CacheGPULost(t *testing.T) {
	mockey.PatchConvey("GetECCErrors volatile L2 cache GPU lost", t, func() {
		mockDevice := &mock.Device{
			GetTotalEccErrorsFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType) (uint64, nvml.Return) {
				return 0, nvml.SUCCESS
			},
			GetMemoryErrorCounterFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType, location nvml.MemoryLocation) (uint64, nvml.Return) {
				// Aggregate all succeed
				if counterType == nvml.AGGREGATE_ECC {
					return 0, nvml.SUCCESS
				}
				// Volatile L1 succeeds
				if counterType == nvml.VOLATILE_ECC && location == nvml.MEMORY_LOCATION_L1_CACHE {
					return 0, nvml.SUCCESS
				}
				// Volatile L2 corrected returns GPU lost
				if counterType == nvml.VOLATILE_ECC && location == nvml.MEMORY_LOCATION_L2_CACHE && errorType == nvml.MEMORY_ERROR_TYPE_CORRECTED {
					return 0, nvml.ERROR_GPU_IS_LOST
				}
				return 0, nvml.SUCCESS
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		_, err := GetECCErrors("test-uuid", dev, true)

		assert.Error(t, err)
		assert.True(t, errors.Is(err, nvmlerrors.ErrGPULost))
	})
}

// TestGetECCErrors_SharedMemoryError tests shared memory error
func TestGetECCErrors_SharedMemoryError(t *testing.T) {
	mockey.PatchConvey("GetECCErrors shared memory error", t, func() {
		mockDevice := &mock.Device{
			GetTotalEccErrorsFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType) (uint64, nvml.Return) {
				return 0, nvml.SUCCESS
			},
			GetMemoryErrorCounterFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType, location nvml.MemoryLocation) (uint64, nvml.Return) {
				// Shared memory error
				if location == nvml.MEMORY_LOCATION_TEXTURE_SHM && counterType == nvml.AGGREGATE_ECC {
					return 0, nvml.ERROR_UNKNOWN
				}
				return 0, nvml.SUCCESS
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		_, err := GetECCErrors("test-uuid", dev, true)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "shared memory")
	})
}

// TestGetECCErrors_RegisterFileError tests register file error
func TestGetECCErrors_RegisterFileError(t *testing.T) {
	mockey.PatchConvey("GetECCErrors register file error", t, func() {
		mockDevice := &mock.Device{
			GetTotalEccErrorsFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType) (uint64, nvml.Return) {
				return 0, nvml.SUCCESS
			},
			GetMemoryErrorCounterFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType, location nvml.MemoryLocation) (uint64, nvml.Return) {
				// Register file error (only checked in volatile)
				if location == nvml.MEMORY_LOCATION_REGISTER_FILE && counterType == nvml.VOLATILE_ECC {
					return 0, nvml.ERROR_UNKNOWN
				}
				return 0, nvml.SUCCESS
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		_, err := GetECCErrors("test-uuid", dev, true)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "register file")
	})
}

// TestGetECCModeEnabled_GPULost tests GetECCModeEnabled with GPU lost error
func TestGetECCModeEnabled_GPULost(t *testing.T) {
	mockey.PatchConvey("GetECCModeEnabled GPU lost", t, func() {
		mockDevice := &mock.Device{
			GetEccModeFunc: func() (nvml.EnableState, nvml.EnableState, nvml.Return) {
				return nvml.FEATURE_DISABLED, nvml.FEATURE_DISABLED, nvml.ERROR_GPU_IS_LOST
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		_, err := GetECCModeEnabled("test-uuid", dev)

		assert.Error(t, err)
		assert.True(t, errors.Is(err, nvmlerrors.ErrGPULost))
	})
}

// TestCheck_MultipleDevices_OneFailure tests Check with multiple devices where one fails
func TestCheck_MultipleDevices_OneFailure(t *testing.T) {
	mockey.PatchConvey("Check with multiple devices one failure", t, func() {
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

		getDevicesFunc := func() map[string]device.Device {
			return devs
		}

		// First device succeeds, second device fails
		callCount := 0
		getECCModeEnabledFunc := func(uuid string, dev device.Device) (ECCMode, error) {
			callCount++
			if uuid == uuid2 {
				return ECCMode{}, nvmlerrors.ErrGPURequiresReset
			}
			return ECCMode{
				UUID:           uuid,
				EnabledCurrent: true,
				Supported:      true,
			}, nil
		}

		getECCErrorsFunc := func(uuid string, dev device.Device, eccModeEnabled bool) (ECCErrors, error) {
			return ECCErrors{
				UUID:      uuid,
				Supported: true,
			}, nil
		}

		comp := MockECCComponent(ctx, getDevicesFunc, getECCModeEnabledFunc, getECCErrorsFunc).(*component)
		result := comp.Check()
		cr, ok := result.(*checkResult)
		require.True(t, ok)

		// Should be unhealthy due to second device failure
		assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
	})
}

// TestCheck_ECCErrorsGPURequiresReset tests Check when ECC errors returns GPU requires reset
func TestCheck_ECCErrorsGPURequiresReset(t *testing.T) {
	mockey.PatchConvey("Check with ECC errors GPU requires reset", t, func() {
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

		getDevicesFunc := func() map[string]device.Device {
			return devs
		}

		getECCModeEnabledFunc := func(uuid string, dev device.Device) (ECCMode, error) {
			return ECCMode{
				UUID:           uuid,
				EnabledCurrent: true,
				Supported:      true,
			}, nil
		}

		getECCErrorsFunc := func(uuid string, dev device.Device, eccModeEnabled bool) (ECCErrors, error) {
			return ECCErrors{}, nvmlerrors.ErrGPURequiresReset
		}

		comp := MockECCComponent(ctx, getDevicesFunc, getECCModeEnabledFunc, getECCErrorsFunc).(*component)
		result := comp.Check()
		cr, ok := result.(*checkResult)
		require.True(t, ok)

		assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
		assert.Equal(t, nvmlerrors.ErrGPURequiresReset.Error(), cr.reason)
		assert.NotNil(t, cr.suggestedActions)
		assert.Contains(t, cr.suggestedActions.RepairActions, apiv1.RepairActionTypeRebootSystem)
	})
}

// TestLastHealthStates_RaceCondition tests for race conditions in LastHealthStates
func TestLastHealthStates_RaceCondition(t *testing.T) {
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

		getDevicesFunc := func() map[string]device.Device {
			return devs
		}

		var callCount atomic.Int32
		getECCModeEnabledFunc := func(uuid string, dev device.Device) (ECCMode, error) {
			callCount.Add(1)
			return ECCMode{
				UUID:           uuid,
				EnabledCurrent: true,
				Supported:      true,
			}, nil
		}

		getECCErrorsFunc := func(uuid string, dev device.Device, eccModeEnabled bool) (ECCErrors, error) {
			return ECCErrors{
				UUID:      uuid,
				Supported: true,
			}, nil
		}

		comp := MockECCComponent(ctx, getDevicesFunc, getECCModeEnabledFunc, getECCErrorsFunc).(*component)

		// Run multiple concurrent reads and writes
		var wg sync.WaitGroup
		for i := 0; i < 20; i++ {
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

		// Should not panic and should have been called multiple times
		assert.True(t, callCount.Load() > 0)
	})
}

// TestGetECCErrors_AggregateUncorrectedNotSupported tests aggregate uncorrected not supported
func TestGetECCErrors_AggregateUncorrectedNotSupported(t *testing.T) {
	mockey.PatchConvey("GetECCErrors aggregate uncorrected not supported", t, func() {
		callCount := 0
		mockDevice := &mock.Device{
			GetTotalEccErrorsFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType) (uint64, nvml.Return) {
				callCount++
				// First call (aggregate corrected) succeeds
				if callCount == 1 {
					return 0, nvml.SUCCESS
				}
				// Second call (aggregate uncorrected) not supported
				return 0, nvml.ERROR_NOT_SUPPORTED
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		result, err := GetECCErrors("test-uuid", dev, false)

		assert.NoError(t, err)
		assert.False(t, result.Supported)
	})
}

// TestGetECCErrors_VolatileCorrectedNotSupported tests volatile corrected not supported
func TestGetECCErrors_VolatileCorrectedNotSupported(t *testing.T) {
	mockey.PatchConvey("GetECCErrors volatile corrected not supported", t, func() {
		callCount := 0
		mockDevice := &mock.Device{
			GetTotalEccErrorsFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType) (uint64, nvml.Return) {
				callCount++
				// First two calls (aggregate) succeed
				if callCount <= 2 {
					return 0, nvml.SUCCESS
				}
				// Third call (volatile corrected) not supported
				return 0, nvml.ERROR_NOT_SUPPORTED
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		result, err := GetECCErrors("test-uuid", dev, false)

		assert.NoError(t, err)
		assert.False(t, result.Supported)
	})
}

// TestGetECCErrors_VolatileUncorrectedNotSupported tests volatile uncorrected not supported
func TestGetECCErrors_VolatileUncorrectedNotSupported(t *testing.T) {
	mockey.PatchConvey("GetECCErrors volatile uncorrected not supported", t, func() {
		callCount := 0
		mockDevice := &mock.Device{
			GetTotalEccErrorsFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType) (uint64, nvml.Return) {
				callCount++
				// First three calls succeed
				if callCount <= 3 {
					return 0, nvml.SUCCESS
				}
				// Fourth call (volatile uncorrected) not supported
				return 0, nvml.ERROR_NOT_SUPPORTED
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		result, err := GetECCErrors("test-uuid", dev, false)

		assert.NoError(t, err)
		assert.False(t, result.Supported)
	})
}

// TestGetECCErrors_AggregateUncorrectedGPURequiresReset tests aggregate uncorrected GPU requires reset
func TestGetECCErrors_AggregateUncorrectedGPURequiresReset(t *testing.T) {
	mockey.PatchConvey("GetECCErrors aggregate uncorrected GPU requires reset", t, func() {
		callCount := 0
		mockDevice := &mock.Device{
			GetTotalEccErrorsFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType) (uint64, nvml.Return) {
				callCount++
				// First call (aggregate corrected) succeeds
				if callCount == 1 {
					return 0, nvml.SUCCESS
				}
				// Second call (aggregate uncorrected) requires reset
				return 0, nvml.ERROR_RESET_REQUIRED
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		_, err := GetECCErrors("test-uuid", dev, false)

		assert.Error(t, err)
		assert.True(t, errors.Is(err, nvmlerrors.ErrGPURequiresReset))
	})
}

// TestGetECCErrors_VolatileCorrectedGPURequiresReset tests volatile corrected GPU requires reset
func TestGetECCErrors_VolatileCorrectedGPURequiresReset(t *testing.T) {
	mockey.PatchConvey("GetECCErrors volatile corrected GPU requires reset", t, func() {
		callCount := 0
		mockDevice := &mock.Device{
			GetTotalEccErrorsFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType) (uint64, nvml.Return) {
				callCount++
				// First two calls (aggregate) succeed
				if callCount <= 2 {
					return 0, nvml.SUCCESS
				}
				// Third call (volatile corrected) requires reset
				return 0, nvml.ERROR_RESET_REQUIRED
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		_, err := GetECCErrors("test-uuid", dev, false)

		assert.Error(t, err)
		assert.True(t, errors.Is(err, nvmlerrors.ErrGPURequiresReset))
	})
}

// TestGetECCErrors_VolatileUncorrectedGPURequiresReset tests volatile uncorrected GPU requires reset
func TestGetECCErrors_VolatileUncorrectedGPURequiresReset(t *testing.T) {
	mockey.PatchConvey("GetECCErrors volatile uncorrected GPU requires reset", t, func() {
		callCount := 0
		mockDevice := &mock.Device{
			GetTotalEccErrorsFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType) (uint64, nvml.Return) {
				callCount++
				// First three calls succeed
				if callCount <= 3 {
					return 0, nvml.SUCCESS
				}
				// Fourth call (volatile uncorrected) requires reset
				return 0, nvml.ERROR_RESET_REQUIRED
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		_, err := GetECCErrors("test-uuid", dev, false)

		assert.Error(t, err)
		assert.True(t, errors.Is(err, nvmlerrors.ErrGPURequiresReset))
	})
}

// TestGetECCErrors_MemoryCounterL2CacheNotSupported tests L2 cache not supported
func TestGetECCErrors_MemoryCounterL2CacheNotSupported(t *testing.T) {
	mockey.PatchConvey("GetECCErrors memory counter L2 cache not supported", t, func() {
		mockDevice := &mock.Device{
			GetTotalEccErrorsFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType) (uint64, nvml.Return) {
				return 0, nvml.SUCCESS
			},
			GetMemoryErrorCounterFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType, location nvml.MemoryLocation) (uint64, nvml.Return) {
				// L2 cache not supported
				if location == nvml.MEMORY_LOCATION_L2_CACHE && counterType == nvml.AGGREGATE_ECC && errorType == nvml.MEMORY_ERROR_TYPE_UNCORRECTED {
					return 0, nvml.ERROR_NOT_SUPPORTED
				}
				return 0, nvml.SUCCESS
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		result, err := GetECCErrors("test-uuid", dev, true)

		assert.NoError(t, err)
		assert.False(t, result.Supported)
	})
}

// TestGetECCErrors_VolatileSharedMemoryGPULost tests volatile shared memory GPU lost
func TestGetECCErrors_VolatileSharedMemoryGPULost(t *testing.T) {
	mockey.PatchConvey("GetECCErrors volatile shared memory GPU lost", t, func() {
		mockDevice := &mock.Device{
			GetTotalEccErrorsFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType) (uint64, nvml.Return) {
				return 0, nvml.SUCCESS
			},
			GetMemoryErrorCounterFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType, location nvml.MemoryLocation) (uint64, nvml.Return) {
				// Volatile shared memory GPU lost
				if location == nvml.MEMORY_LOCATION_TEXTURE_SHM && counterType == nvml.VOLATILE_ECC {
					return 0, nvml.ERROR_GPU_IS_LOST
				}
				return 0, nvml.SUCCESS
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		_, err := GetECCErrors("test-uuid", dev, true)

		assert.Error(t, err)
		assert.True(t, errors.Is(err, nvmlerrors.ErrGPULost))
	})
}

// TestGetECCErrors_VolatileRegisterFileGPULost tests volatile register file GPU lost
func TestGetECCErrors_VolatileRegisterFileGPULost(t *testing.T) {
	mockey.PatchConvey("GetECCErrors volatile register file GPU lost", t, func() {
		mockDevice := &mock.Device{
			GetTotalEccErrorsFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType) (uint64, nvml.Return) {
				return 0, nvml.SUCCESS
			},
			GetMemoryErrorCounterFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType, location nvml.MemoryLocation) (uint64, nvml.Return) {
				// Volatile register file GPU lost
				if location == nvml.MEMORY_LOCATION_REGISTER_FILE && counterType == nvml.VOLATILE_ECC {
					return 0, nvml.ERROR_GPU_IS_LOST
				}
				return 0, nvml.SUCCESS
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		_, err := GetECCErrors("test-uuid", dev, true)

		assert.Error(t, err)
		assert.True(t, errors.Is(err, nvmlerrors.ErrGPULost))
	})
}

// TestGetECCErrors_VolatileRegisterFileGPURequiresReset tests volatile register file GPU requires reset
func TestGetECCErrors_VolatileRegisterFileGPURequiresReset(t *testing.T) {
	mockey.PatchConvey("GetECCErrors volatile register file GPU requires reset", t, func() {
		mockDevice := &mock.Device{
			GetTotalEccErrorsFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType) (uint64, nvml.Return) {
				return 0, nvml.SUCCESS
			},
			GetMemoryErrorCounterFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType, location nvml.MemoryLocation) (uint64, nvml.Return) {
				// Volatile register file corrected GPU requires reset
				if location == nvml.MEMORY_LOCATION_REGISTER_FILE && counterType == nvml.VOLATILE_ECC && errorType == nvml.MEMORY_ERROR_TYPE_CORRECTED {
					return 0, nvml.ERROR_RESET_REQUIRED
				}
				return 0, nvml.SUCCESS
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		_, err := GetECCErrors("test-uuid", dev, true)

		assert.Error(t, err)
		assert.True(t, errors.Is(err, nvmlerrors.ErrGPURequiresReset))
	})
}

// TestGetECCErrors_VolatileDeviceMemoryNotSupported tests volatile device memory not supported
func TestGetECCErrors_VolatileDeviceMemoryNotSupported(t *testing.T) {
	mockey.PatchConvey("GetECCErrors volatile device memory not supported", t, func() {
		mockDevice := &mock.Device{
			GetTotalEccErrorsFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType) (uint64, nvml.Return) {
				return 0, nvml.SUCCESS
			},
			GetMemoryErrorCounterFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType, location nvml.MemoryLocation) (uint64, nvml.Return) {
				// Volatile device memory not supported
				if location == nvml.MEMORY_LOCATION_DEVICE_MEMORY && counterType == nvml.VOLATILE_ECC {
					return 0, nvml.ERROR_NOT_SUPPORTED
				}
				return 0, nvml.SUCCESS
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		result, err := GetECCErrors("test-uuid", dev, true)

		assert.NoError(t, err)
		assert.False(t, result.Supported)
	})
}

// TestGetECCErrors_VolatileTextureMemoryNotSupported tests volatile texture memory not supported
func TestGetECCErrors_VolatileTextureMemoryNotSupported(t *testing.T) {
	mockey.PatchConvey("GetECCErrors volatile texture memory not supported", t, func() {
		mockDevice := &mock.Device{
			GetTotalEccErrorsFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType) (uint64, nvml.Return) {
				return 0, nvml.SUCCESS
			},
			GetMemoryErrorCounterFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType, location nvml.MemoryLocation) (uint64, nvml.Return) {
				// Volatile texture memory not supported
				if location == nvml.MEMORY_LOCATION_TEXTURE_MEMORY && counterType == nvml.VOLATILE_ECC {
					return 0, nvml.ERROR_NOT_SUPPORTED
				}
				return 0, nvml.SUCCESS
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		result, err := GetECCErrors("test-uuid", dev, true)

		assert.NoError(t, err)
		assert.False(t, result.Supported)
	})
}

// TestCheckResult_HealthStates_NoExtraInfo tests HealthStates without ExtraInfo
func TestCheckResult_HealthStates_NoExtraInfo(t *testing.T) {
	mockey.PatchConvey("HealthStates no extra info", t, func() {
		cr := &checkResult{
			ts:       time.Now(),
			health:   apiv1.HealthStateTypeHealthy,
			reason:   "healthy with no data",
			ECCModes: nil,
		}

		states := cr.HealthStates()
		require.Len(t, states, 1)
		assert.Empty(t, states[0].ExtraInfo)
	})
}

// TestCheckResult_HealthStates_OnlyECCModes tests HealthStates with only ECCModes
func TestCheckResult_HealthStates_OnlyECCModes(t *testing.T) {
	mockey.PatchConvey("HealthStates only ECC modes", t, func() {
		cr := &checkResult{
			ts:     time.Now(),
			health: apiv1.HealthStateTypeHealthy,
			reason: "only ecc modes",
			ECCModes: []ECCMode{
				{UUID: "gpu-1", EnabledCurrent: true},
			},
			ECCErrors: nil,
		}

		states := cr.HealthStates()
		require.Len(t, states, 1)
		// No ExtraInfo because ECCErrors is nil
		assert.Empty(t, states[0].ExtraInfo)
	})
}

// TestCheckResult_HealthStates_OnlyECCErrors tests HealthStates with only ECCErrors
func TestCheckResult_HealthStates_OnlyECCErrors(t *testing.T) {
	mockey.PatchConvey("HealthStates only ECC errors", t, func() {
		cr := &checkResult{
			ts:       time.Now(),
			health:   apiv1.HealthStateTypeHealthy,
			reason:   "only ecc errors",
			ECCModes: nil,
			ECCErrors: []ECCErrors{
				{UUID: "gpu-1", Supported: true},
			},
		}

		states := cr.HealthStates()
		require.Len(t, states, 1)
		// No ExtraInfo because ECCModes is nil
		assert.Empty(t, states[0].ExtraInfo)
	})
}

// TestCheckResult_String_WithMultipleGPUs tests String with multiple GPUs
func TestCheckResult_String_WithMultipleGPUs(t *testing.T) {
	mockey.PatchConvey("String with multiple GPUs", t, func() {
		cr := &checkResult{
			ECCModes: []ECCMode{
				{
					UUID:           "gpu-1",
					BusID:          "0000:01:00.0",
					EnabledCurrent: true,
					EnabledPending: true,
					Supported:      true,
				},
				{
					UUID:           "gpu-2",
					BusID:          "0000:02:00.0",
					EnabledCurrent: false,
					EnabledPending: true,
					Supported:      true,
				},
			},
			ECCErrors: []ECCErrors{
				{
					UUID:  "gpu-1",
					BusID: "0000:01:00.0",
					Aggregate: AllECCErrorCounts{
						Total: ECCErrorCounts{Corrected: 10, Uncorrected: 1},
					},
					Volatile: AllECCErrorCounts{
						Total: ECCErrorCounts{Corrected: 5, Uncorrected: 0},
					},
					Supported: true,
				},
				{
					UUID:  "gpu-2",
					BusID: "0000:02:00.0",
					Aggregate: AllECCErrorCounts{
						Total: ECCErrorCounts{Corrected: 20, Uncorrected: 2},
					},
					Volatile: AllECCErrorCounts{
						Total: ECCErrorCounts{Corrected: 8, Uncorrected: 1},
					},
					Supported: true,
				},
			},
		}

		result := cr.String()
		assert.Contains(t, result, "gpu-1")
		assert.Contains(t, result, "gpu-2")
		assert.Contains(t, result, "0000:01:00.0")
		assert.Contains(t, result, "0000:02:00.0")
	})
}

// TestGetECCErrors_AggregateSRAMRequiresReset tests aggregate SRAM GPU requires reset
func TestGetECCErrors_AggregateSRAMRequiresReset(t *testing.T) {
	mockey.PatchConvey("GetECCErrors aggregate SRAM GPU requires reset", t, func() {
		mockDevice := &mock.Device{
			GetTotalEccErrorsFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType) (uint64, nvml.Return) {
				return 0, nvml.SUCCESS
			},
			GetMemoryErrorCounterFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType, location nvml.MemoryLocation) (uint64, nvml.Return) {
				// Aggregate SRAM uncorrected requires reset
				if location == nvml.MEMORY_LOCATION_SRAM && counterType == nvml.AGGREGATE_ECC && errorType == nvml.MEMORY_ERROR_TYPE_UNCORRECTED {
					return 0, nvml.ERROR_RESET_REQUIRED
				}
				return 0, nvml.SUCCESS
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		_, err := GetECCErrors("test-uuid", dev, true)

		assert.Error(t, err)
		assert.True(t, errors.Is(err, nvmlerrors.ErrGPURequiresReset))
	})
}

// TestGetECCErrors_AggregateDeviceMemoryRequiresReset tests aggregate device memory GPU requires reset
func TestGetECCErrors_AggregateDeviceMemoryRequiresReset(t *testing.T) {
	mockey.PatchConvey("GetECCErrors aggregate device memory GPU requires reset", t, func() {
		mockDevice := &mock.Device{
			GetTotalEccErrorsFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType) (uint64, nvml.Return) {
				return 0, nvml.SUCCESS
			},
			GetMemoryErrorCounterFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType, location nvml.MemoryLocation) (uint64, nvml.Return) {
				// Aggregate device memory uncorrected requires reset
				if location == nvml.MEMORY_LOCATION_DEVICE_MEMORY && counterType == nvml.AGGREGATE_ECC && errorType == nvml.MEMORY_ERROR_TYPE_UNCORRECTED {
					return 0, nvml.ERROR_RESET_REQUIRED
				}
				return 0, nvml.SUCCESS
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		_, err := GetECCErrors("test-uuid", dev, true)

		assert.Error(t, err)
		assert.True(t, errors.Is(err, nvmlerrors.ErrGPURequiresReset))
	})
}

// TestGetECCErrors_AggregateTextureMemoryRequiresReset tests aggregate texture memory GPU requires reset
func TestGetECCErrors_AggregateTextureMemoryRequiresReset(t *testing.T) {
	mockey.PatchConvey("GetECCErrors aggregate texture memory GPU requires reset", t, func() {
		mockDevice := &mock.Device{
			GetTotalEccErrorsFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType) (uint64, nvml.Return) {
				return 0, nvml.SUCCESS
			},
			GetMemoryErrorCounterFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType, location nvml.MemoryLocation) (uint64, nvml.Return) {
				// Aggregate texture memory uncorrected requires reset
				if location == nvml.MEMORY_LOCATION_TEXTURE_MEMORY && counterType == nvml.AGGREGATE_ECC && errorType == nvml.MEMORY_ERROR_TYPE_UNCORRECTED {
					return 0, nvml.ERROR_RESET_REQUIRED
				}
				return 0, nvml.SUCCESS
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		_, err := GetECCErrors("test-uuid", dev, true)

		assert.Error(t, err)
		assert.True(t, errors.Is(err, nvmlerrors.ErrGPURequiresReset))
	})
}

// TestGetECCErrors_VolatileL1CorrectedRequiresReset tests volatile L1 cache corrected GPU requires reset
func TestGetECCErrors_VolatileL1CorrectedRequiresReset(t *testing.T) {
	mockey.PatchConvey("GetECCErrors volatile L1 cache corrected GPU requires reset", t, func() {
		mockDevice := &mock.Device{
			GetTotalEccErrorsFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType) (uint64, nvml.Return) {
				return 0, nvml.SUCCESS
			},
			GetMemoryErrorCounterFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType, location nvml.MemoryLocation) (uint64, nvml.Return) {
				// Aggregate all succeed
				if counterType == nvml.AGGREGATE_ECC {
					return 0, nvml.SUCCESS
				}
				// Volatile L1 corrected requires reset
				if location == nvml.MEMORY_LOCATION_L1_CACHE && counterType == nvml.VOLATILE_ECC && errorType == nvml.MEMORY_ERROR_TYPE_CORRECTED {
					return 0, nvml.ERROR_GPU_IS_LOST
				}
				return 0, nvml.SUCCESS
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		_, err := GetECCErrors("test-uuid", dev, true)

		assert.Error(t, err)
		assert.True(t, errors.Is(err, nvmlerrors.ErrGPULost))
	})
}

// TestGetECCErrors_VolatileL1UncorrectedRequiresReset tests volatile L1 cache uncorrected GPU requires reset
func TestGetECCErrors_VolatileL1UncorrectedRequiresReset(t *testing.T) {
	mockey.PatchConvey("GetECCErrors volatile L1 cache uncorrected GPU requires reset", t, func() {
		mockDevice := &mock.Device{
			GetTotalEccErrorsFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType) (uint64, nvml.Return) {
				return 0, nvml.SUCCESS
			},
			GetMemoryErrorCounterFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType, location nvml.MemoryLocation) (uint64, nvml.Return) {
				// Aggregate all succeed
				if counterType == nvml.AGGREGATE_ECC {
					return 0, nvml.SUCCESS
				}
				// Volatile L1 uncorrected requires reset
				if location == nvml.MEMORY_LOCATION_L1_CACHE && counterType == nvml.VOLATILE_ECC && errorType == nvml.MEMORY_ERROR_TYPE_UNCORRECTED {
					return 0, nvml.ERROR_RESET_REQUIRED
				}
				return 0, nvml.SUCCESS
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		_, err := GetECCErrors("test-uuid", dev, true)

		assert.Error(t, err)
		assert.True(t, errors.Is(err, nvmlerrors.ErrGPURequiresReset))
	})
}

// TestGetECCErrors_VolatileL2UncorrectedRequiresReset tests volatile L2 cache uncorrected GPU requires reset
func TestGetECCErrors_VolatileL2UncorrectedRequiresReset(t *testing.T) {
	mockey.PatchConvey("GetECCErrors volatile L2 cache uncorrected GPU requires reset", t, func() {
		mockDevice := &mock.Device{
			GetTotalEccErrorsFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType) (uint64, nvml.Return) {
				return 0, nvml.SUCCESS
			},
			GetMemoryErrorCounterFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType, location nvml.MemoryLocation) (uint64, nvml.Return) {
				// Aggregate all succeed
				if counterType == nvml.AGGREGATE_ECC {
					return 0, nvml.SUCCESS
				}
				// Volatile L1 succeeds
				if location == nvml.MEMORY_LOCATION_L1_CACHE {
					return 0, nvml.SUCCESS
				}
				// Volatile L2 uncorrected requires reset
				if location == nvml.MEMORY_LOCATION_L2_CACHE && errorType == nvml.MEMORY_ERROR_TYPE_UNCORRECTED {
					return 0, nvml.ERROR_RESET_REQUIRED
				}
				return 0, nvml.SUCCESS
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		_, err := GetECCErrors("test-uuid", dev, true)

		assert.Error(t, err)
		assert.True(t, errors.Is(err, nvmlerrors.ErrGPURequiresReset))
	})
}

// TestGetECCErrors_VolatileDRAMRequiresReset tests volatile DRAM GPU requires reset
func TestGetECCErrors_VolatileDRAMRequiresReset(t *testing.T) {
	mockey.PatchConvey("GetECCErrors volatile DRAM GPU requires reset", t, func() {
		mockDevice := &mock.Device{
			GetTotalEccErrorsFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType) (uint64, nvml.Return) {
				return 0, nvml.SUCCESS
			},
			GetMemoryErrorCounterFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType, location nvml.MemoryLocation) (uint64, nvml.Return) {
				// Aggregate all succeed
				if counterType == nvml.AGGREGATE_ECC {
					return 0, nvml.SUCCESS
				}
				// Volatile L1, L2 succeed
				if location == nvml.MEMORY_LOCATION_L1_CACHE || location == nvml.MEMORY_LOCATION_L2_CACHE {
					return 0, nvml.SUCCESS
				}
				// Volatile DRAM corrected requires reset
				if location == nvml.MEMORY_LOCATION_DRAM && errorType == nvml.MEMORY_ERROR_TYPE_CORRECTED {
					return 0, nvml.ERROR_RESET_REQUIRED
				}
				return 0, nvml.SUCCESS
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		_, err := GetECCErrors("test-uuid", dev, true)

		assert.Error(t, err)
		assert.True(t, errors.Is(err, nvmlerrors.ErrGPURequiresReset))
	})
}

// TestGetECCErrors_VolatileDRAMGPULost tests volatile DRAM GPU lost
func TestGetECCErrors_VolatileDRAMGPULost(t *testing.T) {
	mockey.PatchConvey("GetECCErrors volatile DRAM GPU lost", t, func() {
		mockDevice := &mock.Device{
			GetTotalEccErrorsFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType) (uint64, nvml.Return) {
				return 0, nvml.SUCCESS
			},
			GetMemoryErrorCounterFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType, location nvml.MemoryLocation) (uint64, nvml.Return) {
				// Aggregate all succeed
				if counterType == nvml.AGGREGATE_ECC {
					return 0, nvml.SUCCESS
				}
				// Volatile L1, L2 succeed
				if location == nvml.MEMORY_LOCATION_L1_CACHE || location == nvml.MEMORY_LOCATION_L2_CACHE {
					return 0, nvml.SUCCESS
				}
				// Volatile DRAM corrected GPU lost
				if location == nvml.MEMORY_LOCATION_DRAM && errorType == nvml.MEMORY_ERROR_TYPE_CORRECTED {
					return 0, nvml.ERROR_GPU_IS_LOST
				}
				return 0, nvml.SUCCESS
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		_, err := GetECCErrors("test-uuid", dev, true)

		assert.Error(t, err)
		assert.True(t, errors.Is(err, nvmlerrors.ErrGPULost))
	})
}

// TestGetECCErrors_VolatileDRAMUncorrectedRequiresReset tests volatile DRAM uncorrected GPU requires reset
func TestGetECCErrors_VolatileDRAMUncorrectedRequiresReset(t *testing.T) {
	mockey.PatchConvey("GetECCErrors volatile DRAM uncorrected GPU requires reset", t, func() {
		mockDevice := &mock.Device{
			GetTotalEccErrorsFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType) (uint64, nvml.Return) {
				return 0, nvml.SUCCESS
			},
			GetMemoryErrorCounterFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType, location nvml.MemoryLocation) (uint64, nvml.Return) {
				// Aggregate all succeed
				if counterType == nvml.AGGREGATE_ECC {
					return 0, nvml.SUCCESS
				}
				// Volatile L1, L2 succeed
				if location == nvml.MEMORY_LOCATION_L1_CACHE || location == nvml.MEMORY_LOCATION_L2_CACHE {
					return 0, nvml.SUCCESS
				}
				// Volatile DRAM uncorrected requires reset
				if location == nvml.MEMORY_LOCATION_DRAM && errorType == nvml.MEMORY_ERROR_TYPE_UNCORRECTED {
					return 0, nvml.ERROR_RESET_REQUIRED
				}
				return 0, nvml.SUCCESS
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		_, err := GetECCErrors("test-uuid", dev, true)

		assert.Error(t, err)
		assert.True(t, errors.Is(err, nvmlerrors.ErrGPURequiresReset))
	})
}

// TestGetECCErrors_VolatileSRAMRequiresReset tests volatile SRAM GPU requires reset
func TestGetECCErrors_VolatileSRAMRequiresReset(t *testing.T) {
	mockey.PatchConvey("GetECCErrors volatile SRAM GPU requires reset", t, func() {
		mockDevice := &mock.Device{
			GetTotalEccErrorsFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType) (uint64, nvml.Return) {
				return 0, nvml.SUCCESS
			},
			GetMemoryErrorCounterFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType, location nvml.MemoryLocation) (uint64, nvml.Return) {
				// Aggregate all succeed
				if counterType == nvml.AGGREGATE_ECC {
					return 0, nvml.SUCCESS
				}
				// Volatile L1, L2, DRAM succeed
				if location == nvml.MEMORY_LOCATION_L1_CACHE ||
					location == nvml.MEMORY_LOCATION_L2_CACHE ||
					location == nvml.MEMORY_LOCATION_DRAM {
					return 0, nvml.SUCCESS
				}
				// Volatile SRAM corrected requires reset
				if location == nvml.MEMORY_LOCATION_SRAM && errorType == nvml.MEMORY_ERROR_TYPE_CORRECTED {
					return 0, nvml.ERROR_RESET_REQUIRED
				}
				return 0, nvml.SUCCESS
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		_, err := GetECCErrors("test-uuid", dev, true)

		assert.Error(t, err)
		assert.True(t, errors.Is(err, nvmlerrors.ErrGPURequiresReset))
	})
}

// TestGetECCErrors_VolatileSRAMGPULost tests volatile SRAM GPU lost
func TestGetECCErrors_VolatileSRAMGPULost(t *testing.T) {
	mockey.PatchConvey("GetECCErrors volatile SRAM GPU lost", t, func() {
		mockDevice := &mock.Device{
			GetTotalEccErrorsFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType) (uint64, nvml.Return) {
				return 0, nvml.SUCCESS
			},
			GetMemoryErrorCounterFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType, location nvml.MemoryLocation) (uint64, nvml.Return) {
				// Aggregate all succeed
				if counterType == nvml.AGGREGATE_ECC {
					return 0, nvml.SUCCESS
				}
				// Volatile L1, L2, DRAM succeed
				if location == nvml.MEMORY_LOCATION_L1_CACHE ||
					location == nvml.MEMORY_LOCATION_L2_CACHE ||
					location == nvml.MEMORY_LOCATION_DRAM {
					return 0, nvml.SUCCESS
				}
				// Volatile SRAM corrected GPU lost
				if location == nvml.MEMORY_LOCATION_SRAM && errorType == nvml.MEMORY_ERROR_TYPE_CORRECTED {
					return 0, nvml.ERROR_GPU_IS_LOST
				}
				return 0, nvml.SUCCESS
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		_, err := GetECCErrors("test-uuid", dev, true)

		assert.Error(t, err)
		assert.True(t, errors.Is(err, nvmlerrors.ErrGPULost))
	})
}

// TestGetECCErrors_VolatileDeviceMemoryGPULost tests volatile device memory GPU lost
func TestGetECCErrors_VolatileDeviceMemoryGPULost(t *testing.T) {
	mockey.PatchConvey("GetECCErrors volatile device memory GPU lost", t, func() {
		mockDevice := &mock.Device{
			GetTotalEccErrorsFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType) (uint64, nvml.Return) {
				return 0, nvml.SUCCESS
			},
			GetMemoryErrorCounterFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType, location nvml.MemoryLocation) (uint64, nvml.Return) {
				// Volatile device memory corrected GPU lost - check this first to match the exact case
				if location == nvml.MEMORY_LOCATION_DEVICE_MEMORY && counterType == nvml.VOLATILE_ECC && errorType == nvml.MEMORY_ERROR_TYPE_CORRECTED {
					return 0, nvml.ERROR_GPU_IS_LOST
				}
				return 0, nvml.SUCCESS
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		_, err := GetECCErrors("test-uuid", dev, true)

		assert.Error(t, err)
		assert.True(t, errors.Is(err, nvmlerrors.ErrGPULost))
	})
}

// TestGetECCErrors_VolatileTextureMemoryGPULost tests volatile texture memory GPU lost
func TestGetECCErrors_VolatileTextureMemoryGPULost(t *testing.T) {
	mockey.PatchConvey("GetECCErrors volatile texture memory GPU lost", t, func() {
		mockDevice := &mock.Device{
			GetTotalEccErrorsFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType) (uint64, nvml.Return) {
				return 0, nvml.SUCCESS
			},
			GetMemoryErrorCounterFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType, location nvml.MemoryLocation) (uint64, nvml.Return) {
				// Aggregate all succeed
				if counterType == nvml.AGGREGATE_ECC {
					return 0, nvml.SUCCESS
				}
				// Volatile L1, L2, DRAM, SRAM, device memory succeed
				if location == nvml.MEMORY_LOCATION_L1_CACHE ||
					location == nvml.MEMORY_LOCATION_L2_CACHE ||
					location == nvml.MEMORY_LOCATION_DRAM ||
					location == nvml.MEMORY_LOCATION_SRAM ||
					location == nvml.MEMORY_LOCATION_DEVICE_MEMORY {
					return 0, nvml.SUCCESS
				}
				// Volatile texture memory corrected GPU lost
				if location == nvml.MEMORY_LOCATION_TEXTURE_MEMORY && errorType == nvml.MEMORY_ERROR_TYPE_CORRECTED {
					return 0, nvml.ERROR_GPU_IS_LOST
				}
				return 0, nvml.SUCCESS
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		_, err := GetECCErrors("test-uuid", dev, true)

		assert.Error(t, err)
		assert.True(t, errors.Is(err, nvmlerrors.ErrGPULost))
	})
}

// TestGetECCErrors_VolatileSharedMemoryNotSupported tests volatile shared memory not supported
func TestGetECCErrors_VolatileSharedMemoryNotSupported(t *testing.T) {
	mockey.PatchConvey("GetECCErrors volatile shared memory not supported", t, func() {
		mockDevice := &mock.Device{
			GetTotalEccErrorsFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType) (uint64, nvml.Return) {
				return 0, nvml.SUCCESS
			},
			GetMemoryErrorCounterFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType, location nvml.MemoryLocation) (uint64, nvml.Return) {
				// Volatile shared memory not supported
				if location == nvml.MEMORY_LOCATION_TEXTURE_SHM && counterType == nvml.VOLATILE_ECC {
					return 0, nvml.ERROR_NOT_SUPPORTED
				}
				return 0, nvml.SUCCESS
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		result, err := GetECCErrors("test-uuid", dev, true)

		assert.NoError(t, err)
		assert.False(t, result.Supported)
	})
}

// TestGetECCErrors_VolatileRegisterFileNotSupported tests volatile register file not supported
func TestGetECCErrors_VolatileRegisterFileNotSupported(t *testing.T) {
	mockey.PatchConvey("GetECCErrors volatile register file not supported", t, func() {
		mockDevice := &mock.Device{
			GetTotalEccErrorsFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType) (uint64, nvml.Return) {
				return 0, nvml.SUCCESS
			},
			GetMemoryErrorCounterFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType, location nvml.MemoryLocation) (uint64, nvml.Return) {
				// Volatile register file not supported
				if location == nvml.MEMORY_LOCATION_REGISTER_FILE && counterType == nvml.VOLATILE_ECC {
					return 0, nvml.ERROR_NOT_SUPPORTED
				}
				return 0, nvml.SUCCESS
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		result, err := GetECCErrors("test-uuid", dev, true)

		assert.NoError(t, err)
		assert.False(t, result.Supported)
	})
}

// TestGetECCErrors_AggregateDRAMNotSupported tests aggregate DRAM not supported
func TestGetECCErrors_AggregateDRAMNotSupported(t *testing.T) {
	mockey.PatchConvey("GetECCErrors aggregate DRAM not supported", t, func() {
		mockDevice := &mock.Device{
			GetTotalEccErrorsFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType) (uint64, nvml.Return) {
				return 0, nvml.SUCCESS
			},
			GetMemoryErrorCounterFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType, location nvml.MemoryLocation) (uint64, nvml.Return) {
				// Aggregate DRAM not supported
				if location == nvml.MEMORY_LOCATION_DRAM && counterType == nvml.AGGREGATE_ECC {
					return 0, nvml.ERROR_NOT_SUPPORTED
				}
				return 0, nvml.SUCCESS
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		result, err := GetECCErrors("test-uuid", dev, true)

		assert.NoError(t, err)
		assert.False(t, result.Supported)
	})
}

// TestGetECCErrors_AggregateSRAMNotSupported tests aggregate SRAM not supported
func TestGetECCErrors_AggregateSRAMNotSupported(t *testing.T) {
	mockey.PatchConvey("GetECCErrors aggregate SRAM not supported", t, func() {
		mockDevice := &mock.Device{
			GetTotalEccErrorsFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType) (uint64, nvml.Return) {
				return 0, nvml.SUCCESS
			},
			GetMemoryErrorCounterFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType, location nvml.MemoryLocation) (uint64, nvml.Return) {
				// L1, L2 succeed
				if location == nvml.MEMORY_LOCATION_L1_CACHE || location == nvml.MEMORY_LOCATION_L2_CACHE {
					return 0, nvml.SUCCESS
				}
				// DRAM succeed
				if location == nvml.MEMORY_LOCATION_DRAM {
					return 0, nvml.SUCCESS
				}
				// Aggregate SRAM not supported
				if location == nvml.MEMORY_LOCATION_SRAM && counterType == nvml.AGGREGATE_ECC {
					return 0, nvml.ERROR_NOT_SUPPORTED
				}
				return 0, nvml.SUCCESS
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		result, err := GetECCErrors("test-uuid", dev, true)

		assert.NoError(t, err)
		assert.False(t, result.Supported)
	})
}

// TestGetECCErrors_AggregateTextureMemoryNotSupported tests aggregate texture memory not supported
func TestGetECCErrors_AggregateTextureMemoryNotSupported(t *testing.T) {
	mockey.PatchConvey("GetECCErrors aggregate texture memory not supported", t, func() {
		mockDevice := &mock.Device{
			GetTotalEccErrorsFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType) (uint64, nvml.Return) {
				return 0, nvml.SUCCESS
			},
			GetMemoryErrorCounterFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType, location nvml.MemoryLocation) (uint64, nvml.Return) {
				// Most locations succeed
				if location == nvml.MEMORY_LOCATION_L1_CACHE ||
					location == nvml.MEMORY_LOCATION_L2_CACHE ||
					location == nvml.MEMORY_LOCATION_DRAM ||
					location == nvml.MEMORY_LOCATION_SRAM ||
					location == nvml.MEMORY_LOCATION_DEVICE_MEMORY {
					return 0, nvml.SUCCESS
				}
				// Aggregate texture memory not supported
				if location == nvml.MEMORY_LOCATION_TEXTURE_MEMORY && counterType == nvml.AGGREGATE_ECC {
					return 0, nvml.ERROR_NOT_SUPPORTED
				}
				return 0, nvml.SUCCESS
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		result, err := GetECCErrors("test-uuid", dev, true)

		assert.NoError(t, err)
		assert.False(t, result.Supported)
	})
}

// TestGetECCErrors_AggregateSharedMemoryNotSupported tests aggregate shared memory not supported
func TestGetECCErrors_AggregateSharedMemoryNotSupported(t *testing.T) {
	mockey.PatchConvey("GetECCErrors aggregate shared memory not supported", t, func() {
		mockDevice := &mock.Device{
			GetTotalEccErrorsFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType) (uint64, nvml.Return) {
				return 0, nvml.SUCCESS
			},
			GetMemoryErrorCounterFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType, location nvml.MemoryLocation) (uint64, nvml.Return) {
				// Most locations succeed
				if location == nvml.MEMORY_LOCATION_L1_CACHE ||
					location == nvml.MEMORY_LOCATION_L2_CACHE ||
					location == nvml.MEMORY_LOCATION_DRAM ||
					location == nvml.MEMORY_LOCATION_SRAM ||
					location == nvml.MEMORY_LOCATION_DEVICE_MEMORY ||
					location == nvml.MEMORY_LOCATION_TEXTURE_MEMORY {
					return 0, nvml.SUCCESS
				}
				// Aggregate shared memory not supported
				if location == nvml.MEMORY_LOCATION_TEXTURE_SHM && counterType == nvml.AGGREGATE_ECC {
					return 0, nvml.ERROR_NOT_SUPPORTED
				}
				return 0, nvml.SUCCESS
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		result, err := GetECCErrors("test-uuid", dev, true)

		assert.NoError(t, err)
		assert.False(t, result.Supported)
	})
}

// TestGetECCErrors_AggregateL1CacheNotSupported tests aggregate L1 cache not supported
func TestGetECCErrors_AggregateL1CacheNotSupported(t *testing.T) {
	mockey.PatchConvey("GetECCErrors aggregate L1 cache not supported", t, func() {
		mockDevice := &mock.Device{
			GetTotalEccErrorsFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType) (uint64, nvml.Return) {
				return 0, nvml.SUCCESS
			},
			GetMemoryErrorCounterFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType, location nvml.MemoryLocation) (uint64, nvml.Return) {
				// Aggregate L1 cache not supported
				if location == nvml.MEMORY_LOCATION_L1_CACHE && counterType == nvml.AGGREGATE_ECC {
					return 0, nvml.ERROR_NOT_SUPPORTED
				}
				return 0, nvml.SUCCESS
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		result, err := GetECCErrors("test-uuid", dev, true)

		assert.NoError(t, err)
		assert.False(t, result.Supported)
	})
}

// TestGetECCErrors_VolatileL1CacheNotSupported tests volatile L1 cache not supported
func TestGetECCErrors_VolatileL1CacheNotSupported(t *testing.T) {
	mockey.PatchConvey("GetECCErrors volatile L1 cache not supported", t, func() {
		mockDevice := &mock.Device{
			GetTotalEccErrorsFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType) (uint64, nvml.Return) {
				return 0, nvml.SUCCESS
			},
			GetMemoryErrorCounterFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType, location nvml.MemoryLocation) (uint64, nvml.Return) {
				// Aggregate succeeds
				if counterType == nvml.AGGREGATE_ECC {
					return 0, nvml.SUCCESS
				}
				// Volatile L1 cache not supported
				if location == nvml.MEMORY_LOCATION_L1_CACHE && counterType == nvml.VOLATILE_ECC {
					return 0, nvml.ERROR_NOT_SUPPORTED
				}
				return 0, nvml.SUCCESS
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		result, err := GetECCErrors("test-uuid", dev, true)

		assert.NoError(t, err)
		assert.False(t, result.Supported)
	})
}

// TestGetECCErrors_VolatileDRAMNotSupported tests volatile DRAM not supported
func TestGetECCErrors_VolatileDRAMNotSupported(t *testing.T) {
	mockey.PatchConvey("GetECCErrors volatile DRAM not supported", t, func() {
		mockDevice := &mock.Device{
			GetTotalEccErrorsFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType) (uint64, nvml.Return) {
				return 0, nvml.SUCCESS
			},
			GetMemoryErrorCounterFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType, location nvml.MemoryLocation) (uint64, nvml.Return) {
				// Aggregate succeeds
				if counterType == nvml.AGGREGATE_ECC {
					return 0, nvml.SUCCESS
				}
				// Volatile L1, L2 succeed
				if location == nvml.MEMORY_LOCATION_L1_CACHE || location == nvml.MEMORY_LOCATION_L2_CACHE {
					return 0, nvml.SUCCESS
				}
				// Volatile DRAM not supported
				if location == nvml.MEMORY_LOCATION_DRAM && counterType == nvml.VOLATILE_ECC {
					return 0, nvml.ERROR_NOT_SUPPORTED
				}
				return 0, nvml.SUCCESS
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		result, err := GetECCErrors("test-uuid", dev, true)

		assert.NoError(t, err)
		assert.False(t, result.Supported)
	})
}

// TestGetECCErrors_VolatileSRAMNotSupported tests volatile SRAM not supported
func TestGetECCErrors_VolatileSRAMNotSupported(t *testing.T) {
	mockey.PatchConvey("GetECCErrors volatile SRAM not supported", t, func() {
		mockDevice := &mock.Device{
			GetTotalEccErrorsFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType) (uint64, nvml.Return) {
				return 0, nvml.SUCCESS
			},
			GetMemoryErrorCounterFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType, location nvml.MemoryLocation) (uint64, nvml.Return) {
				// Aggregate succeeds
				if counterType == nvml.AGGREGATE_ECC {
					return 0, nvml.SUCCESS
				}
				// Volatile L1, L2, DRAM succeed
				if location == nvml.MEMORY_LOCATION_L1_CACHE ||
					location == nvml.MEMORY_LOCATION_L2_CACHE ||
					location == nvml.MEMORY_LOCATION_DRAM {
					return 0, nvml.SUCCESS
				}
				// Volatile SRAM not supported
				if location == nvml.MEMORY_LOCATION_SRAM && counterType == nvml.VOLATILE_ECC {
					return 0, nvml.ERROR_NOT_SUPPORTED
				}
				return 0, nvml.SUCCESS
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		result, err := GetECCErrors("test-uuid", dev, true)

		assert.NoError(t, err)
		assert.False(t, result.Supported)
	})
}

// TestGetECCErrors_AggregateL1CacheGPULost tests aggregate L1 cache GPU lost
func TestGetECCErrors_AggregateL1CacheGPULost(t *testing.T) {
	mockey.PatchConvey("GetECCErrors aggregate L1 cache GPU lost", t, func() {
		mockDevice := &mock.Device{
			GetTotalEccErrorsFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType) (uint64, nvml.Return) {
				return 0, nvml.SUCCESS
			},
			GetMemoryErrorCounterFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType, location nvml.MemoryLocation) (uint64, nvml.Return) {
				// Aggregate L1 cache GPU lost
				if location == nvml.MEMORY_LOCATION_L1_CACHE && counterType == nvml.AGGREGATE_ECC && errorType == nvml.MEMORY_ERROR_TYPE_CORRECTED {
					return 0, nvml.ERROR_GPU_IS_LOST
				}
				return 0, nvml.SUCCESS
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		_, err := GetECCErrors("test-uuid", dev, true)

		assert.Error(t, err)
		assert.True(t, errors.Is(err, nvmlerrors.ErrGPULost))
	})
}

// TestGetECCErrors_AggregateL1CacheRequiresReset tests aggregate L1 cache requires reset
func TestGetECCErrors_AggregateL1CacheRequiresReset(t *testing.T) {
	mockey.PatchConvey("GetECCErrors aggregate L1 cache requires reset", t, func() {
		mockDevice := &mock.Device{
			GetTotalEccErrorsFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType) (uint64, nvml.Return) {
				return 0, nvml.SUCCESS
			},
			GetMemoryErrorCounterFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType, location nvml.MemoryLocation) (uint64, nvml.Return) {
				// Aggregate L1 cache requires reset
				if location == nvml.MEMORY_LOCATION_L1_CACHE && counterType == nvml.AGGREGATE_ECC && errorType == nvml.MEMORY_ERROR_TYPE_CORRECTED {
					return 0, nvml.ERROR_RESET_REQUIRED
				}
				return 0, nvml.SUCCESS
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		_, err := GetECCErrors("test-uuid", dev, true)

		assert.Error(t, err)
		assert.True(t, errors.Is(err, nvmlerrors.ErrGPURequiresReset))
	})
}

// TestGetECCErrors_AggregateL2CacheGPULost tests aggregate L2 cache GPU lost
func TestGetECCErrors_AggregateL2CacheGPULost(t *testing.T) {
	mockey.PatchConvey("GetECCErrors aggregate L2 cache GPU lost", t, func() {
		mockDevice := &mock.Device{
			GetTotalEccErrorsFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType) (uint64, nvml.Return) {
				return 0, nvml.SUCCESS
			},
			GetMemoryErrorCounterFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType, location nvml.MemoryLocation) (uint64, nvml.Return) {
				// L1 succeeds
				if location == nvml.MEMORY_LOCATION_L1_CACHE {
					return 0, nvml.SUCCESS
				}
				// Aggregate L2 cache GPU lost
				if location == nvml.MEMORY_LOCATION_L2_CACHE && counterType == nvml.AGGREGATE_ECC && errorType == nvml.MEMORY_ERROR_TYPE_CORRECTED {
					return 0, nvml.ERROR_GPU_IS_LOST
				}
				return 0, nvml.SUCCESS
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		_, err := GetECCErrors("test-uuid", dev, true)

		assert.Error(t, err)
		assert.True(t, errors.Is(err, nvmlerrors.ErrGPULost))
	})
}

// TestGetECCErrors_AggregateL2CacheRequiresReset tests aggregate L2 cache requires reset
func TestGetECCErrors_AggregateL2CacheRequiresReset(t *testing.T) {
	mockey.PatchConvey("GetECCErrors aggregate L2 cache requires reset", t, func() {
		mockDevice := &mock.Device{
			GetTotalEccErrorsFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType) (uint64, nvml.Return) {
				return 0, nvml.SUCCESS
			},
			GetMemoryErrorCounterFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType, location nvml.MemoryLocation) (uint64, nvml.Return) {
				// L1 succeeds
				if location == nvml.MEMORY_LOCATION_L1_CACHE {
					return 0, nvml.SUCCESS
				}
				// Aggregate L2 cache requires reset
				if location == nvml.MEMORY_LOCATION_L2_CACHE && counterType == nvml.AGGREGATE_ECC && errorType == nvml.MEMORY_ERROR_TYPE_CORRECTED {
					return 0, nvml.ERROR_RESET_REQUIRED
				}
				return 0, nvml.SUCCESS
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		_, err := GetECCErrors("test-uuid", dev, true)

		assert.Error(t, err)
		assert.True(t, errors.Is(err, nvmlerrors.ErrGPURequiresReset))
	})
}

// TestGetECCErrors_AggregateDRAMGPULost tests aggregate DRAM GPU lost
func TestGetECCErrors_AggregateDRAMGPULost(t *testing.T) {
	mockey.PatchConvey("GetECCErrors aggregate DRAM GPU lost", t, func() {
		mockDevice := &mock.Device{
			GetTotalEccErrorsFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType) (uint64, nvml.Return) {
				return 0, nvml.SUCCESS
			},
			GetMemoryErrorCounterFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType, location nvml.MemoryLocation) (uint64, nvml.Return) {
				// L1, L2 succeed
				if location == nvml.MEMORY_LOCATION_L1_CACHE || location == nvml.MEMORY_LOCATION_L2_CACHE {
					return 0, nvml.SUCCESS
				}
				// Aggregate DRAM GPU lost
				if location == nvml.MEMORY_LOCATION_DRAM && counterType == nvml.AGGREGATE_ECC && errorType == nvml.MEMORY_ERROR_TYPE_CORRECTED {
					return 0, nvml.ERROR_GPU_IS_LOST
				}
				return 0, nvml.SUCCESS
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		_, err := GetECCErrors("test-uuid", dev, true)

		assert.Error(t, err)
		assert.True(t, errors.Is(err, nvmlerrors.ErrGPULost))
	})
}

// TestGetECCErrors_AggregateDRAMRequiresReset tests aggregate DRAM requires reset
func TestGetECCErrors_AggregateDRAMRequiresReset(t *testing.T) {
	mockey.PatchConvey("GetECCErrors aggregate DRAM requires reset", t, func() {
		mockDevice := &mock.Device{
			GetTotalEccErrorsFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType) (uint64, nvml.Return) {
				return 0, nvml.SUCCESS
			},
			GetMemoryErrorCounterFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType, location nvml.MemoryLocation) (uint64, nvml.Return) {
				// L1, L2 succeed
				if location == nvml.MEMORY_LOCATION_L1_CACHE || location == nvml.MEMORY_LOCATION_L2_CACHE {
					return 0, nvml.SUCCESS
				}
				// Aggregate DRAM requires reset
				if location == nvml.MEMORY_LOCATION_DRAM && counterType == nvml.AGGREGATE_ECC && errorType == nvml.MEMORY_ERROR_TYPE_CORRECTED {
					return 0, nvml.ERROR_RESET_REQUIRED
				}
				return 0, nvml.SUCCESS
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		_, err := GetECCErrors("test-uuid", dev, true)

		assert.Error(t, err)
		assert.True(t, errors.Is(err, nvmlerrors.ErrGPURequiresReset))
	})
}

// TestGetECCErrors_AggregateDeviceMemoryGPULost tests aggregate device memory GPU lost
func TestGetECCErrors_AggregateDeviceMemoryGPULost(t *testing.T) {
	mockey.PatchConvey("GetECCErrors aggregate device memory GPU lost", t, func() {
		mockDevice := &mock.Device{
			GetTotalEccErrorsFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType) (uint64, nvml.Return) {
				return 0, nvml.SUCCESS
			},
			GetMemoryErrorCounterFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType, location nvml.MemoryLocation) (uint64, nvml.Return) {
				// L1, L2, SRAM succeed
				// NOTE: MEMORY_LOCATION_DRAM == MEMORY_LOCATION_DEVICE_MEMORY (both are 2),
				// so we cannot list DRAM in the success path here.
				if location == nvml.MEMORY_LOCATION_L1_CACHE ||
					location == nvml.MEMORY_LOCATION_L2_CACHE ||
					location == nvml.MEMORY_LOCATION_SRAM {
					return 0, nvml.SUCCESS
				}
				// Aggregate device memory (== DRAM) GPU lost
				if location == nvml.MEMORY_LOCATION_DEVICE_MEMORY && counterType == nvml.AGGREGATE_ECC && errorType == nvml.MEMORY_ERROR_TYPE_CORRECTED {
					return 0, nvml.ERROR_GPU_IS_LOST
				}
				return 0, nvml.SUCCESS
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		_, err := GetECCErrors("test-uuid", dev, true)

		assert.Error(t, err)
		assert.True(t, errors.Is(err, nvmlerrors.ErrGPULost))
	})
}

// TestGetECCErrors_AggregateTextureMemoryGPULost tests aggregate texture memory GPU lost
func TestGetECCErrors_AggregateTextureMemoryGPULost(t *testing.T) {
	mockey.PatchConvey("GetECCErrors aggregate texture memory GPU lost", t, func() {
		mockDevice := &mock.Device{
			GetTotalEccErrorsFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType) (uint64, nvml.Return) {
				return 0, nvml.SUCCESS
			},
			GetMemoryErrorCounterFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType, location nvml.MemoryLocation) (uint64, nvml.Return) {
				// L1, L2, DRAM/device memory, SRAM succeed
				if location == nvml.MEMORY_LOCATION_L1_CACHE ||
					location == nvml.MEMORY_LOCATION_L2_CACHE ||
					location == nvml.MEMORY_LOCATION_DEVICE_MEMORY ||
					location == nvml.MEMORY_LOCATION_SRAM {
					return 0, nvml.SUCCESS
				}
				// Aggregate texture memory GPU lost
				if location == nvml.MEMORY_LOCATION_TEXTURE_MEMORY && counterType == nvml.AGGREGATE_ECC && errorType == nvml.MEMORY_ERROR_TYPE_CORRECTED {
					return 0, nvml.ERROR_GPU_IS_LOST
				}
				return 0, nvml.SUCCESS
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		_, err := GetECCErrors("test-uuid", dev, true)

		assert.Error(t, err)
		assert.True(t, errors.Is(err, nvmlerrors.ErrGPULost))
	})
}

// TestGetECCErrors_AggregateSharedMemoryGPULost tests aggregate shared memory GPU lost
func TestGetECCErrors_AggregateSharedMemoryGPULost(t *testing.T) {
	mockey.PatchConvey("GetECCErrors aggregate shared memory GPU lost", t, func() {
		mockDevice := &mock.Device{
			GetTotalEccErrorsFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType) (uint64, nvml.Return) {
				return 0, nvml.SUCCESS
			},
			GetMemoryErrorCounterFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType, location nvml.MemoryLocation) (uint64, nvml.Return) {
				// L1, L2, DRAM, SRAM, device memory, texture memory succeed
				if location == nvml.MEMORY_LOCATION_L1_CACHE ||
					location == nvml.MEMORY_LOCATION_L2_CACHE ||
					location == nvml.MEMORY_LOCATION_DRAM ||
					location == nvml.MEMORY_LOCATION_SRAM ||
					location == nvml.MEMORY_LOCATION_DEVICE_MEMORY ||
					location == nvml.MEMORY_LOCATION_TEXTURE_MEMORY {
					return 0, nvml.SUCCESS
				}
				// Aggregate shared memory GPU lost
				if location == nvml.MEMORY_LOCATION_TEXTURE_SHM && counterType == nvml.AGGREGATE_ECC && errorType == nvml.MEMORY_ERROR_TYPE_CORRECTED {
					return 0, nvml.ERROR_GPU_IS_LOST
				}
				return 0, nvml.SUCCESS
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		_, err := GetECCErrors("test-uuid", dev, true)

		assert.Error(t, err)
		assert.True(t, errors.Is(err, nvmlerrors.ErrGPULost))
	})
}

// TestGetECCModeEnabled_RequiresReset tests GetECCModeEnabled with GPU requires reset error
func TestGetECCModeEnabled_RequiresReset(t *testing.T) {
	mockey.PatchConvey("GetECCModeEnabled GPU requires reset", t, func() {
		mockDevice := &mock.Device{
			GetEccModeFunc: func() (nvml.EnableState, nvml.EnableState, nvml.Return) {
				return nvml.FEATURE_DISABLED, nvml.FEATURE_DISABLED, nvml.ERROR_RESET_REQUIRED
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		_, err := GetECCModeEnabled("test-uuid", dev)

		assert.Error(t, err)
		assert.True(t, errors.Is(err, nvmlerrors.ErrGPURequiresReset))
	})
}

// TestCheckResult_getError_NilError tests getError method when error is nil
func TestCheckResult_getError_NilError(t *testing.T) {
	mockey.PatchConvey("getError with nil error", t, func() {
		cr := &checkResult{
			health: apiv1.HealthStateTypeHealthy,
			reason: "all good",
			err:    nil,
		}
		assert.Equal(t, "", cr.getError())
	})
}

// TestCheckResult_getError_WithError tests getError method when error is present
func TestCheckResult_getError_WithError(t *testing.T) {
	mockey.PatchConvey("getError with error", t, func() {
		cr := &checkResult{
			health: apiv1.HealthStateTypeUnhealthy,
			reason: "error occurred",
			err:    errors.New("test error message"),
		}
		assert.Equal(t, "test error message", cr.getError())
	})
}

// TestComponent_Name tests the Name method
func TestComponent_Name(t *testing.T) {
	mockey.PatchConvey("Name method", t, func() {
		comp := &component{}
		assert.Equal(t, Name, comp.Name())
	})
}

// TestCheck_EmptyDeviceMap tests Check with empty device map (not nil)
func TestCheck_EmptyDeviceMap(t *testing.T) {
	mockey.PatchConvey("Check with empty device map", t, func() {
		ctx := context.Background()
		cctx, cancel := context.WithCancel(ctx)
		defer cancel()

		mockInst := &customMockNVMLInstance{
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
			getECCModeEnabledFunc: func(uuid string, dev device.Device) (ECCMode, error) {
				return ECCMode{}, nil
			},
			getECCErrorsFunc: func(uuid string, dev device.Device, eccModeEnabledCurrent bool) (ECCErrors, error) {
				return ECCErrors{}, nil
			},
		}

		result := comp.Check()
		cr, ok := result.(*checkResult)
		require.True(t, ok)

		assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
		assert.Contains(t, cr.reason, "all 0 GPU(s) were checked")
	})
}

// TestIsSupported_NilInstance tests IsSupported when nvmlInstance is nil
func TestIsSupported_NilInstance(t *testing.T) {
	mockey.PatchConvey("IsSupported with nil instance", t, func() {
		comp := &component{
			nvmlInstance: nil,
		}
		assert.False(t, comp.IsSupported())
	})
}

// TestIsSupported_NVMLNotExists tests IsSupported when NVML library is not loaded
func TestIsSupported_NVMLNotExists(t *testing.T) {
	mockey.PatchConvey("IsSupported with NVML not loaded", t, func() {
		mockInst := &customMockNVMLInstance{
			devs:        map[string]device.Device{},
			nvmlExists:  false,
			productName: "NVIDIA H100",
		}
		comp := &component{
			nvmlInstance: mockInst,
		}
		assert.False(t, comp.IsSupported())
	})
}

// TestIsSupported_NoProductName tests IsSupported when product name is empty
func TestIsSupported_NoProductName(t *testing.T) {
	mockey.PatchConvey("IsSupported with no product name", t, func() {
		mockInst := &customMockNVMLInstance{
			devs:        map[string]device.Device{},
			nvmlExists:  true,
			productName: "",
		}
		comp := &component{
			nvmlInstance: mockInst,
		}
		assert.False(t, comp.IsSupported())
	})
}

// TestIsSupported_Success tests IsSupported when everything is configured correctly
func TestIsSupported_Success(t *testing.T) {
	mockey.PatchConvey("IsSupported with valid configuration", t, func() {
		mockInst := &customMockNVMLInstance{
			devs:        map[string]device.Device{},
			nvmlExists:  true,
			productName: "NVIDIA H100",
		}
		comp := &component{
			nvmlInstance: mockInst,
		}
		assert.True(t, comp.IsSupported())
	})
}

// TestCheckResult_ComponentName tests ComponentName method
func TestCheckResult_ComponentName(t *testing.T) {
	mockey.PatchConvey("ComponentName method", t, func() {
		cr := &checkResult{}
		assert.Equal(t, Name, cr.ComponentName())
	})
}

// TestCheck_MultipleGPUs_AllSuccessful tests Check with multiple GPUs all working
func TestCheck_MultipleGPUs_AllSuccessful(t *testing.T) {
	mockey.PatchConvey("Check with multiple GPUs all successful", t, func() {
		ctx := context.Background()

		mockDeviceObj1 := &mock.Device{
			GetUUIDFunc: func() (string, nvml.Return) {
				return "gpu-1", nvml.SUCCESS
			},
		}
		mockDev1 := testutil.NewMockDevice(mockDeviceObj1, "test-arch", "test-brand", "test-cuda", "test-pci-1")

		mockDeviceObj2 := &mock.Device{
			GetUUIDFunc: func() (string, nvml.Return) {
				return "gpu-2", nvml.SUCCESS
			},
		}
		mockDev2 := testutil.NewMockDevice(mockDeviceObj2, "test-arch", "test-brand", "test-cuda", "test-pci-2")

		devs := map[string]device.Device{
			"gpu-1": mockDev1,
			"gpu-2": mockDev2,
		}

		getDevicesFunc := func() map[string]device.Device {
			return devs
		}

		getECCModeEnabledFunc := func(uuid string, dev device.Device) (ECCMode, error) {
			return ECCMode{
				UUID:           uuid,
				EnabledCurrent: true,
				Supported:      true,
			}, nil
		}

		getECCErrorsFunc := func(uuid string, dev device.Device, eccModeEnabled bool) (ECCErrors, error) {
			return ECCErrors{
				UUID:      uuid,
				Supported: true,
			}, nil
		}

		comp := MockECCComponent(ctx, getDevicesFunc, getECCModeEnabledFunc, getECCErrorsFunc).(*component)
		result := comp.Check()
		cr, ok := result.(*checkResult)
		require.True(t, ok)

		assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
		assert.Contains(t, cr.reason, "all 2 GPU(s) were checked")
		assert.Len(t, cr.ECCModes, 2)
		assert.Len(t, cr.ECCErrors, 2)
	})
}

// TestGetECCModeEnabled_NotSupported tests GetECCModeEnabled when feature is not supported
func TestGetECCModeEnabled_NotSupported(t *testing.T) {
	mockey.PatchConvey("GetECCModeEnabled not supported", t, func() {
		mockDevice := &mock.Device{
			GetEccModeFunc: func() (nvml.EnableState, nvml.EnableState, nvml.Return) {
				return nvml.FEATURE_DISABLED, nvml.FEATURE_DISABLED, nvml.ERROR_NOT_SUPPORTED
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		result, err := GetECCModeEnabled("test-uuid", dev)

		assert.NoError(t, err)
		assert.False(t, result.Supported)
		assert.False(t, result.EnabledCurrent)
		assert.False(t, result.EnabledPending)
	})
}

// TestGetECCModeEnabled_UnknownError tests GetECCModeEnabled with unknown error
func TestGetECCModeEnabled_UnknownError(t *testing.T) {
	mockey.PatchConvey("GetECCModeEnabled unknown error", t, func() {
		mockDevice := &mock.Device{
			GetEccModeFunc: func() (nvml.EnableState, nvml.EnableState, nvml.Return) {
				return nvml.FEATURE_DISABLED, nvml.FEATURE_DISABLED, nvml.ERROR_UNKNOWN
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		_, err := GetECCModeEnabled("test-uuid", dev)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get current/pending ecc mode")
	})
}

// TestCheck_ECCModeEnabledGPULost tests Check when ECC mode returns GPU lost
func TestCheck_ECCModeEnabledGPULost(t *testing.T) {
	mockey.PatchConvey("Check with ECC mode GPU lost", t, func() {
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

		getDevicesFunc := func() map[string]device.Device {
			return devs
		}

		getECCModeEnabledFunc := func(uuid string, dev device.Device) (ECCMode, error) {
			return ECCMode{}, nvmlerrors.ErrGPULost
		}

		comp := MockECCComponent(ctx, getDevicesFunc, getECCModeEnabledFunc, nil).(*component)
		result := comp.Check()
		cr, ok := result.(*checkResult)
		require.True(t, ok)

		assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
		assert.Equal(t, nvmlerrors.ErrGPULost.Error(), cr.reason)
		assert.NotNil(t, cr.suggestedActions)
		assert.Contains(t, cr.suggestedActions.RepairActions, apiv1.RepairActionTypeRebootSystem)
	})
}

// TestCheck_ProductNameEmpty tests Check when product name is empty
func TestCheck_ProductNameEmpty(t *testing.T) {
	mockey.PatchConvey("Check with empty product name", t, func() {
		ctx := context.Background()
		cctx, cancel := context.WithCancel(ctx)
		defer cancel()

		mockInst := &customMockNVMLInstance{
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
		assert.Contains(t, cr.reason, "GPU is not detected")
	})
}

// TestGetECCErrors_VolatileCorrectedGPULost tests volatile corrected with GPU lost
func TestGetECCErrors_VolatileCorrectedGPULost(t *testing.T) {
	mockey.PatchConvey("GetECCErrors volatile corrected GPU lost", t, func() {
		callCount := 0
		mockDevice := &mock.Device{
			GetTotalEccErrorsFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType) (uint64, nvml.Return) {
				callCount++
				// First two calls (aggregate) succeed
				if callCount <= 2 {
					return 0, nvml.SUCCESS
				}
				// Third call (volatile corrected) returns GPU lost
				return 0, nvml.ERROR_GPU_IS_LOST
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		_, err := GetECCErrors("test-uuid", dev, false)

		assert.Error(t, err)
		assert.True(t, errors.Is(err, nvmlerrors.ErrGPULost))
	})
}

// TestGetECCErrors_AggregateUncorrectedUnknownErrorFmt tests aggregate uncorrected with unknown error for fmt branch
func TestGetECCErrors_AggregateUncorrectedUnknownErrorFmt(t *testing.T) {
	mockey.PatchConvey("GetECCErrors aggregate uncorrected unknown error fmt", t, func() {
		callCount := 0
		mockDevice := &mock.Device{
			GetTotalEccErrorsFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType) (uint64, nvml.Return) {
				callCount++
				// First call (aggregate corrected) succeeds
				if callCount == 1 {
					return 0, nvml.SUCCESS
				}
				// Second call (aggregate uncorrected) returns unknown error
				return 0, nvml.ERROR_UNKNOWN
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		_, err := GetECCErrors("test-uuid", dev, false)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "(aggregate, uncorrected) failed to get total ecc errors")
	})
}

// TestGetECCErrors_VolatileUncorrectedUnknownErrorFmt tests volatile uncorrected with unknown error for fmt branch
func TestGetECCErrors_VolatileUncorrectedUnknownErrorFmt(t *testing.T) {
	mockey.PatchConvey("GetECCErrors volatile uncorrected unknown error fmt", t, func() {
		callCount := 0
		mockDevice := &mock.Device{
			GetTotalEccErrorsFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType) (uint64, nvml.Return) {
				callCount++
				// First three calls succeed
				if callCount <= 3 {
					return 0, nvml.SUCCESS
				}
				// Fourth call (volatile uncorrected) returns unknown error
				return 0, nvml.ERROR_UNKNOWN
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		_, err := GetECCErrors("test-uuid", dev, false)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "(volatile, uncorrected) failed to get total ecc errors")
	})
}

// TestGetECCErrors_AggregateL1UncorrectedNotSupportedBranch tests aggregate L1 uncorrected not supported
func TestGetECCErrors_AggregateL1UncorrectedNotSupportedBranch(t *testing.T) {
	mockey.PatchConvey("GetECCErrors aggregate L1 uncorrected not supported branch", t, func() {
		mockDevice := &mock.Device{
			GetTotalEccErrorsFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType) (uint64, nvml.Return) {
				return 0, nvml.SUCCESS
			},
			GetMemoryErrorCounterFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType, location nvml.MemoryLocation) (uint64, nvml.Return) {
				// Aggregate L1 corrected succeeds
				if location == nvml.MEMORY_LOCATION_L1_CACHE && counterType == nvml.AGGREGATE_ECC && errorType == nvml.MEMORY_ERROR_TYPE_CORRECTED {
					return 0, nvml.SUCCESS
				}
				// Aggregate L1 uncorrected not supported
				if location == nvml.MEMORY_LOCATION_L1_CACHE && counterType == nvml.AGGREGATE_ECC && errorType == nvml.MEMORY_ERROR_TYPE_UNCORRECTED {
					return 0, nvml.ERROR_NOT_SUPPORTED
				}
				return 0, nvml.SUCCESS
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		result, err := GetECCErrors("test-uuid", dev, true)

		assert.NoError(t, err)
		assert.False(t, result.Supported)
	})
}

// TestGetECCErrors_AggregateL1UncorrectedGPULostBranch tests aggregate L1 uncorrected GPU lost
func TestGetECCErrors_AggregateL1UncorrectedGPULostBranch(t *testing.T) {
	mockey.PatchConvey("GetECCErrors aggregate L1 uncorrected GPU lost branch", t, func() {
		mockDevice := &mock.Device{
			GetTotalEccErrorsFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType) (uint64, nvml.Return) {
				return 0, nvml.SUCCESS
			},
			GetMemoryErrorCounterFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType, location nvml.MemoryLocation) (uint64, nvml.Return) {
				// Aggregate L1 corrected succeeds
				if location == nvml.MEMORY_LOCATION_L1_CACHE && counterType == nvml.AGGREGATE_ECC && errorType == nvml.MEMORY_ERROR_TYPE_CORRECTED {
					return 0, nvml.SUCCESS
				}
				// Aggregate L1 uncorrected GPU lost
				if location == nvml.MEMORY_LOCATION_L1_CACHE && counterType == nvml.AGGREGATE_ECC && errorType == nvml.MEMORY_ERROR_TYPE_UNCORRECTED {
					return 0, nvml.ERROR_GPU_IS_LOST
				}
				return 0, nvml.SUCCESS
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		_, err := GetECCErrors("test-uuid", dev, true)

		assert.Error(t, err)
		assert.True(t, errors.Is(err, nvmlerrors.ErrGPULost))
	})
}

// TestGetECCErrors_AggregateL1UncorrectedRequiresResetBranch tests aggregate L1 uncorrected requires reset
func TestGetECCErrors_AggregateL1UncorrectedRequiresResetBranch(t *testing.T) {
	mockey.PatchConvey("GetECCErrors aggregate L1 uncorrected requires reset branch", t, func() {
		mockDevice := &mock.Device{
			GetTotalEccErrorsFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType) (uint64, nvml.Return) {
				return 0, nvml.SUCCESS
			},
			GetMemoryErrorCounterFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType, location nvml.MemoryLocation) (uint64, nvml.Return) {
				// Aggregate L1 corrected succeeds
				if location == nvml.MEMORY_LOCATION_L1_CACHE && counterType == nvml.AGGREGATE_ECC && errorType == nvml.MEMORY_ERROR_TYPE_CORRECTED {
					return 0, nvml.SUCCESS
				}
				// Aggregate L1 uncorrected requires reset
				if location == nvml.MEMORY_LOCATION_L1_CACHE && counterType == nvml.AGGREGATE_ECC && errorType == nvml.MEMORY_ERROR_TYPE_UNCORRECTED {
					return 0, nvml.ERROR_RESET_REQUIRED
				}
				return 0, nvml.SUCCESS
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		_, err := GetECCErrors("test-uuid", dev, true)

		assert.Error(t, err)
		assert.True(t, errors.Is(err, nvmlerrors.ErrGPURequiresReset))
	})
}

// TestGetECCErrors_AggregateL2CorrectedNotSupportedBranch tests aggregate L2 corrected not supported
func TestGetECCErrors_AggregateL2CorrectedNotSupportedBranch(t *testing.T) {
	mockey.PatchConvey("GetECCErrors aggregate L2 corrected not supported branch", t, func() {
		mockDevice := &mock.Device{
			GetTotalEccErrorsFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType) (uint64, nvml.Return) {
				return 0, nvml.SUCCESS
			},
			GetMemoryErrorCounterFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType, location nvml.MemoryLocation) (uint64, nvml.Return) {
				// L1 succeeds
				if location == nvml.MEMORY_LOCATION_L1_CACHE {
					return 0, nvml.SUCCESS
				}
				// Aggregate L2 corrected not supported
				if location == nvml.MEMORY_LOCATION_L2_CACHE && counterType == nvml.AGGREGATE_ECC && errorType == nvml.MEMORY_ERROR_TYPE_CORRECTED {
					return 0, nvml.ERROR_NOT_SUPPORTED
				}
				return 0, nvml.SUCCESS
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		result, err := GetECCErrors("test-uuid", dev, true)

		assert.NoError(t, err)
		assert.False(t, result.Supported)
	})
}

// TestGetECCErrors_AggregateL2CorrectedUnknownErrorBranch tests aggregate L2 corrected unknown error
func TestGetECCErrors_AggregateL2CorrectedUnknownErrorBranch(t *testing.T) {
	mockey.PatchConvey("GetECCErrors aggregate L2 corrected unknown error branch", t, func() {
		mockDevice := &mock.Device{
			GetTotalEccErrorsFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType) (uint64, nvml.Return) {
				return 0, nvml.SUCCESS
			},
			GetMemoryErrorCounterFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType, location nvml.MemoryLocation) (uint64, nvml.Return) {
				// L1 succeeds
				if location == nvml.MEMORY_LOCATION_L1_CACHE {
					return 0, nvml.SUCCESS
				}
				// Aggregate L2 corrected unknown error
				if location == nvml.MEMORY_LOCATION_L2_CACHE && counterType == nvml.AGGREGATE_ECC && errorType == nvml.MEMORY_ERROR_TYPE_CORRECTED {
					return 0, nvml.ERROR_UNKNOWN
				}
				return 0, nvml.SUCCESS
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		_, err := GetECCErrors("test-uuid", dev, true)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "(aggregate, corrected) failed to get l2 cache ecc errors")
	})
}

// TestGetECCErrors_AggregateL2UncorrectedGPULostBranch tests aggregate L2 uncorrected GPU lost
func TestGetECCErrors_AggregateL2UncorrectedGPULostBranch(t *testing.T) {
	mockey.PatchConvey("GetECCErrors aggregate L2 uncorrected GPU lost branch", t, func() {
		mockDevice := &mock.Device{
			GetTotalEccErrorsFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType) (uint64, nvml.Return) {
				return 0, nvml.SUCCESS
			},
			GetMemoryErrorCounterFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType, location nvml.MemoryLocation) (uint64, nvml.Return) {
				// L1, L2 corrected succeed
				if location == nvml.MEMORY_LOCATION_L1_CACHE {
					return 0, nvml.SUCCESS
				}
				if location == nvml.MEMORY_LOCATION_L2_CACHE && counterType == nvml.AGGREGATE_ECC && errorType == nvml.MEMORY_ERROR_TYPE_CORRECTED {
					return 0, nvml.SUCCESS
				}
				// Aggregate L2 uncorrected GPU lost
				if location == nvml.MEMORY_LOCATION_L2_CACHE && counterType == nvml.AGGREGATE_ECC && errorType == nvml.MEMORY_ERROR_TYPE_UNCORRECTED {
					return 0, nvml.ERROR_GPU_IS_LOST
				}
				return 0, nvml.SUCCESS
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		_, err := GetECCErrors("test-uuid", dev, true)

		assert.Error(t, err)
		assert.True(t, errors.Is(err, nvmlerrors.ErrGPULost))
	})
}

// TestGetECCErrors_AggregateL2UncorrectedRequiresResetBranch tests aggregate L2 uncorrected requires reset
func TestGetECCErrors_AggregateL2UncorrectedRequiresResetBranch(t *testing.T) {
	mockey.PatchConvey("GetECCErrors aggregate L2 uncorrected requires reset branch", t, func() {
		mockDevice := &mock.Device{
			GetTotalEccErrorsFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType) (uint64, nvml.Return) {
				return 0, nvml.SUCCESS
			},
			GetMemoryErrorCounterFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType, location nvml.MemoryLocation) (uint64, nvml.Return) {
				// L1, L2 corrected succeed
				if location == nvml.MEMORY_LOCATION_L1_CACHE {
					return 0, nvml.SUCCESS
				}
				if location == nvml.MEMORY_LOCATION_L2_CACHE && counterType == nvml.AGGREGATE_ECC && errorType == nvml.MEMORY_ERROR_TYPE_CORRECTED {
					return 0, nvml.SUCCESS
				}
				// Aggregate L2 uncorrected requires reset
				if location == nvml.MEMORY_LOCATION_L2_CACHE && counterType == nvml.AGGREGATE_ECC && errorType == nvml.MEMORY_ERROR_TYPE_UNCORRECTED {
					return 0, nvml.ERROR_RESET_REQUIRED
				}
				return 0, nvml.SUCCESS
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		_, err := GetECCErrors("test-uuid", dev, true)

		assert.Error(t, err)
		assert.True(t, errors.Is(err, nvmlerrors.ErrGPURequiresReset))
	})
}

// TestGetECCErrors_AggregateL2UncorrectedUnknownErrorBranch tests aggregate L2 uncorrected unknown error
func TestGetECCErrors_AggregateL2UncorrectedUnknownErrorBranch(t *testing.T) {
	mockey.PatchConvey("GetECCErrors aggregate L2 uncorrected unknown error branch", t, func() {
		mockDevice := &mock.Device{
			GetTotalEccErrorsFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType) (uint64, nvml.Return) {
				return 0, nvml.SUCCESS
			},
			GetMemoryErrorCounterFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType, location nvml.MemoryLocation) (uint64, nvml.Return) {
				// L1, L2 corrected succeed
				if location == nvml.MEMORY_LOCATION_L1_CACHE {
					return 0, nvml.SUCCESS
				}
				if location == nvml.MEMORY_LOCATION_L2_CACHE && counterType == nvml.AGGREGATE_ECC && errorType == nvml.MEMORY_ERROR_TYPE_CORRECTED {
					return 0, nvml.SUCCESS
				}
				// Aggregate L2 uncorrected unknown error
				if location == nvml.MEMORY_LOCATION_L2_CACHE && counterType == nvml.AGGREGATE_ECC && errorType == nvml.MEMORY_ERROR_TYPE_UNCORRECTED {
					return 0, nvml.ERROR_UNKNOWN
				}
				return 0, nvml.SUCCESS
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		_, err := GetECCErrors("test-uuid", dev, true)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "(aggregate, uncorrected) failed to get l2 cache ecc errors")
	})
}

// TestGetECCErrors_AggregateDRAMUncorrectedNotSupportedBranch tests aggregate DRAM uncorrected not supported
func TestGetECCErrors_AggregateDRAMUncorrectedNotSupportedBranch(t *testing.T) {
	mockey.PatchConvey("GetECCErrors aggregate DRAM uncorrected not supported branch", t, func() {
		mockDevice := &mock.Device{
			GetTotalEccErrorsFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType) (uint64, nvml.Return) {
				return 0, nvml.SUCCESS
			},
			GetMemoryErrorCounterFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType, location nvml.MemoryLocation) (uint64, nvml.Return) {
				// L1, L2, DRAM corrected succeed
				if location == nvml.MEMORY_LOCATION_L1_CACHE || location == nvml.MEMORY_LOCATION_L2_CACHE {
					return 0, nvml.SUCCESS
				}
				if location == nvml.MEMORY_LOCATION_DRAM && counterType == nvml.AGGREGATE_ECC && errorType == nvml.MEMORY_ERROR_TYPE_CORRECTED {
					return 0, nvml.SUCCESS
				}
				// Aggregate DRAM uncorrected not supported
				if location == nvml.MEMORY_LOCATION_DRAM && counterType == nvml.AGGREGATE_ECC && errorType == nvml.MEMORY_ERROR_TYPE_UNCORRECTED {
					return 0, nvml.ERROR_NOT_SUPPORTED
				}
				return 0, nvml.SUCCESS
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		result, err := GetECCErrors("test-uuid", dev, true)

		assert.NoError(t, err)
		assert.False(t, result.Supported)
	})
}

// TestGetECCErrors_AggregateDRAMUncorrectedGPULostBranch tests aggregate DRAM uncorrected GPU lost
func TestGetECCErrors_AggregateDRAMUncorrectedGPULostBranch(t *testing.T) {
	mockey.PatchConvey("GetECCErrors aggregate DRAM uncorrected GPU lost branch", t, func() {
		mockDevice := &mock.Device{
			GetTotalEccErrorsFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType) (uint64, nvml.Return) {
				return 0, nvml.SUCCESS
			},
			GetMemoryErrorCounterFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType, location nvml.MemoryLocation) (uint64, nvml.Return) {
				// L1, L2, DRAM corrected succeed
				if location == nvml.MEMORY_LOCATION_L1_CACHE || location == nvml.MEMORY_LOCATION_L2_CACHE {
					return 0, nvml.SUCCESS
				}
				if location == nvml.MEMORY_LOCATION_DRAM && counterType == nvml.AGGREGATE_ECC && errorType == nvml.MEMORY_ERROR_TYPE_CORRECTED {
					return 0, nvml.SUCCESS
				}
				// Aggregate DRAM uncorrected GPU lost
				if location == nvml.MEMORY_LOCATION_DRAM && counterType == nvml.AGGREGATE_ECC && errorType == nvml.MEMORY_ERROR_TYPE_UNCORRECTED {
					return 0, nvml.ERROR_GPU_IS_LOST
				}
				return 0, nvml.SUCCESS
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		_, err := GetECCErrors("test-uuid", dev, true)

		assert.Error(t, err)
		assert.True(t, errors.Is(err, nvmlerrors.ErrGPULost))
	})
}

// TestGetECCErrors_AggregateDRAMUncorrectedUnknownErrorBranch tests aggregate DRAM uncorrected unknown error
func TestGetECCErrors_AggregateDRAMUncorrectedUnknownErrorBranch(t *testing.T) {
	mockey.PatchConvey("GetECCErrors aggregate DRAM uncorrected unknown error branch", t, func() {
		mockDevice := &mock.Device{
			GetTotalEccErrorsFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType) (uint64, nvml.Return) {
				return 0, nvml.SUCCESS
			},
			GetMemoryErrorCounterFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType, location nvml.MemoryLocation) (uint64, nvml.Return) {
				// L1, L2, DRAM corrected succeed
				if location == nvml.MEMORY_LOCATION_L1_CACHE || location == nvml.MEMORY_LOCATION_L2_CACHE {
					return 0, nvml.SUCCESS
				}
				if location == nvml.MEMORY_LOCATION_DRAM && counterType == nvml.AGGREGATE_ECC && errorType == nvml.MEMORY_ERROR_TYPE_CORRECTED {
					return 0, nvml.SUCCESS
				}
				// Aggregate DRAM uncorrected unknown error
				if location == nvml.MEMORY_LOCATION_DRAM && counterType == nvml.AGGREGATE_ECC && errorType == nvml.MEMORY_ERROR_TYPE_UNCORRECTED {
					return 0, nvml.ERROR_UNKNOWN
				}
				return 0, nvml.SUCCESS
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		_, err := GetECCErrors("test-uuid", dev, true)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "(aggregate, uncorrected) failed to get dram cache ecc errors")
	})
}

// TestGetECCErrors_AggregateSRAMCorrectedRequiresResetBranch tests aggregate SRAM corrected requires reset
func TestGetECCErrors_AggregateSRAMCorrectedRequiresResetBranch(t *testing.T) {
	mockey.PatchConvey("GetECCErrors aggregate SRAM corrected requires reset branch", t, func() {
		mockDevice := &mock.Device{
			GetTotalEccErrorsFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType) (uint64, nvml.Return) {
				return 0, nvml.SUCCESS
			},
			GetMemoryErrorCounterFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType, location nvml.MemoryLocation) (uint64, nvml.Return) {
				// L1, L2, DRAM succeed
				if location == nvml.MEMORY_LOCATION_L1_CACHE || location == nvml.MEMORY_LOCATION_L2_CACHE || location == nvml.MEMORY_LOCATION_DRAM {
					return 0, nvml.SUCCESS
				}
				// Aggregate SRAM corrected requires reset
				if location == nvml.MEMORY_LOCATION_SRAM && counterType == nvml.AGGREGATE_ECC && errorType == nvml.MEMORY_ERROR_TYPE_CORRECTED {
					return 0, nvml.ERROR_RESET_REQUIRED
				}
				return 0, nvml.SUCCESS
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		_, err := GetECCErrors("test-uuid", dev, true)

		assert.Error(t, err)
		assert.True(t, errors.Is(err, nvmlerrors.ErrGPURequiresReset))
	})
}

// TestGetECCErrors_AggregateSRAMCorrectedUnknownErrorBranch tests aggregate SRAM corrected unknown error
func TestGetECCErrors_AggregateSRAMCorrectedUnknownErrorBranch(t *testing.T) {
	mockey.PatchConvey("GetECCErrors aggregate SRAM corrected unknown error branch", t, func() {
		mockDevice := &mock.Device{
			GetTotalEccErrorsFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType) (uint64, nvml.Return) {
				return 0, nvml.SUCCESS
			},
			GetMemoryErrorCounterFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType, location nvml.MemoryLocation) (uint64, nvml.Return) {
				// L1, L2, DRAM succeed
				if location == nvml.MEMORY_LOCATION_L1_CACHE || location == nvml.MEMORY_LOCATION_L2_CACHE || location == nvml.MEMORY_LOCATION_DRAM {
					return 0, nvml.SUCCESS
				}
				// Aggregate SRAM corrected unknown error
				if location == nvml.MEMORY_LOCATION_SRAM && counterType == nvml.AGGREGATE_ECC && errorType == nvml.MEMORY_ERROR_TYPE_CORRECTED {
					return 0, nvml.ERROR_UNKNOWN
				}
				return 0, nvml.SUCCESS
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		_, err := GetECCErrors("test-uuid", dev, true)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "(aggregate, corrected) failed to get sram ecc errors")
	})
}

// TestGetECCErrors_AggregateSRAMUncorrectedNotSupportedBranch tests aggregate SRAM uncorrected not supported
func TestGetECCErrors_AggregateSRAMUncorrectedNotSupportedBranch(t *testing.T) {
	mockey.PatchConvey("GetECCErrors aggregate SRAM uncorrected not supported branch", t, func() {
		mockDevice := &mock.Device{
			GetTotalEccErrorsFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType) (uint64, nvml.Return) {
				return 0, nvml.SUCCESS
			},
			GetMemoryErrorCounterFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType, location nvml.MemoryLocation) (uint64, nvml.Return) {
				// L1, L2, DRAM succeed
				if location == nvml.MEMORY_LOCATION_L1_CACHE || location == nvml.MEMORY_LOCATION_L2_CACHE || location == nvml.MEMORY_LOCATION_DRAM {
					return 0, nvml.SUCCESS
				}
				// Aggregate SRAM corrected succeeds
				if location == nvml.MEMORY_LOCATION_SRAM && counterType == nvml.AGGREGATE_ECC && errorType == nvml.MEMORY_ERROR_TYPE_CORRECTED {
					return 0, nvml.SUCCESS
				}
				// Aggregate SRAM uncorrected not supported
				if location == nvml.MEMORY_LOCATION_SRAM && counterType == nvml.AGGREGATE_ECC && errorType == nvml.MEMORY_ERROR_TYPE_UNCORRECTED {
					return 0, nvml.ERROR_NOT_SUPPORTED
				}
				return 0, nvml.SUCCESS
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		result, err := GetECCErrors("test-uuid", dev, true)

		assert.NoError(t, err)
		assert.False(t, result.Supported)
	})
}

// TestGetECCErrors_AggregateSRAMUncorrectedGPULostBranch tests aggregate SRAM uncorrected GPU lost
func TestGetECCErrors_AggregateSRAMUncorrectedGPULostBranch(t *testing.T) {
	mockey.PatchConvey("GetECCErrors aggregate SRAM uncorrected GPU lost branch", t, func() {
		mockDevice := &mock.Device{
			GetTotalEccErrorsFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType) (uint64, nvml.Return) {
				return 0, nvml.SUCCESS
			},
			GetMemoryErrorCounterFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType, location nvml.MemoryLocation) (uint64, nvml.Return) {
				// L1, L2, DRAM succeed
				if location == nvml.MEMORY_LOCATION_L1_CACHE || location == nvml.MEMORY_LOCATION_L2_CACHE || location == nvml.MEMORY_LOCATION_DRAM {
					return 0, nvml.SUCCESS
				}
				// Aggregate SRAM corrected succeeds
				if location == nvml.MEMORY_LOCATION_SRAM && counterType == nvml.AGGREGATE_ECC && errorType == nvml.MEMORY_ERROR_TYPE_CORRECTED {
					return 0, nvml.SUCCESS
				}
				// Aggregate SRAM uncorrected GPU lost
				if location == nvml.MEMORY_LOCATION_SRAM && counterType == nvml.AGGREGATE_ECC && errorType == nvml.MEMORY_ERROR_TYPE_UNCORRECTED {
					return 0, nvml.ERROR_GPU_IS_LOST
				}
				return 0, nvml.SUCCESS
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		_, err := GetECCErrors("test-uuid", dev, true)

		assert.Error(t, err)
		assert.True(t, errors.Is(err, nvmlerrors.ErrGPULost))
	})
}

// TestGetECCErrors_AggregateSRAMUncorrectedUnknownErrorBranch tests aggregate SRAM uncorrected unknown error
func TestGetECCErrors_AggregateSRAMUncorrectedUnknownErrorBranch(t *testing.T) {
	mockey.PatchConvey("GetECCErrors aggregate SRAM uncorrected unknown error branch", t, func() {
		mockDevice := &mock.Device{
			GetTotalEccErrorsFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType) (uint64, nvml.Return) {
				return 0, nvml.SUCCESS
			},
			GetMemoryErrorCounterFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType, location nvml.MemoryLocation) (uint64, nvml.Return) {
				// L1, L2, DRAM succeed
				if location == nvml.MEMORY_LOCATION_L1_CACHE || location == nvml.MEMORY_LOCATION_L2_CACHE || location == nvml.MEMORY_LOCATION_DRAM {
					return 0, nvml.SUCCESS
				}
				// Aggregate SRAM corrected succeeds
				if location == nvml.MEMORY_LOCATION_SRAM && counterType == nvml.AGGREGATE_ECC && errorType == nvml.MEMORY_ERROR_TYPE_CORRECTED {
					return 0, nvml.SUCCESS
				}
				// Aggregate SRAM uncorrected unknown error
				if location == nvml.MEMORY_LOCATION_SRAM && counterType == nvml.AGGREGATE_ECC && errorType == nvml.MEMORY_ERROR_TYPE_UNCORRECTED {
					return 0, nvml.ERROR_UNKNOWN
				}
				return 0, nvml.SUCCESS
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		_, err := GetECCErrors("test-uuid", dev, true)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "(aggregate, uncorrected) failed to get sram ecc errors")
	})
}

// Note: TestGetECCErrors_AggregateDeviceMemoryCorrectedNotSupported cannot be tested separately
// because MEMORY_LOCATION_DRAM == MEMORY_LOCATION_DEVICE_MEMORY (both are 2 in NVML)

// TestNew_TimeNowCalled tests that getTimeNowFunc is correctly used during Check
func TestNew_TimeNowCalled(t *testing.T) {
	mockey.PatchConvey("New function time now called", t, func() {
		ctx := context.Background()
		mockInstance := &mockNVMLInstance{
			devicesFunc: func() map[string]device.Device { return nil },
		}

		gpudInstance := &components.GPUdInstance{
			RootCtx:      ctx,
			NVMLInstance: mockInstance,
		}

		c, err := New(gpudInstance)
		assert.NoError(t, err)
		assert.NotNil(t, c)

		// Access internal component and call Check which uses getTimeNowFunc
		tc, ok := c.(*component)
		require.True(t, ok)

		// Make sure getTimeNowFunc is not nil
		assert.NotNil(t, tc.getTimeNowFunc)

		// Call the getTimeNowFunc directly to ensure coverage
		now := tc.getTimeNowFunc()
		assert.False(t, now.IsZero())
	})
}

// ========================================================================
// Additional tests to increase coverage for GetECCErrors uncovered branches
// ========================================================================

// TestGetECCErrors_AggregateDeviceMemoryCorrectedNotSupported tests aggregate device memory corrected not supported
// Note: MEMORY_LOCATION_DRAM == MEMORY_LOCATION_DEVICE_MEMORY, so we target this path via device memory location
func TestGetECCErrors_AggregateDeviceMemoryCorrectedNotSupported(t *testing.T) {
	mockey.PatchConvey("GetECCErrors aggregate device memory corrected not supported", t, func() {
		mockDevice := &mock.Device{
			GetTotalEccErrorsFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType) (uint64, nvml.Return) {
				return 0, nvml.SUCCESS
			},
			GetMemoryErrorCounterFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType, location nvml.MemoryLocation) (uint64, nvml.Return) {
				// L1, L2, SRAM succeed
				if location == nvml.MEMORY_LOCATION_L1_CACHE || location == nvml.MEMORY_LOCATION_L2_CACHE || location == nvml.MEMORY_LOCATION_SRAM {
					return 0, nvml.SUCCESS
				}
				// Aggregate device memory corrected not supported
				if location == nvml.MEMORY_LOCATION_DEVICE_MEMORY && counterType == nvml.AGGREGATE_ECC && errorType == nvml.MEMORY_ERROR_TYPE_CORRECTED {
					return 0, nvml.ERROR_NOT_SUPPORTED
				}
				return 0, nvml.SUCCESS
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		result, err := GetECCErrors("test-uuid", dev, true)

		assert.NoError(t, err)
		assert.False(t, result.Supported)
	})
}

// TestGetECCErrors_AggregateDeviceMemoryCorrectedGPULost tests aggregate device memory corrected GPU lost
func TestGetECCErrors_AggregateDeviceMemoryCorrectedGPULost(t *testing.T) {
	mockey.PatchConvey("GetECCErrors aggregate device memory corrected GPU lost", t, func() {
		mockDevice := &mock.Device{
			GetTotalEccErrorsFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType) (uint64, nvml.Return) {
				return 0, nvml.SUCCESS
			},
			GetMemoryErrorCounterFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType, location nvml.MemoryLocation) (uint64, nvml.Return) {
				// L1, L2, SRAM succeed
				if location == nvml.MEMORY_LOCATION_L1_CACHE || location == nvml.MEMORY_LOCATION_L2_CACHE || location == nvml.MEMORY_LOCATION_SRAM {
					return 0, nvml.SUCCESS
				}
				// Aggregate device memory corrected GPU lost
				if location == nvml.MEMORY_LOCATION_DEVICE_MEMORY && counterType == nvml.AGGREGATE_ECC && errorType == nvml.MEMORY_ERROR_TYPE_CORRECTED {
					return 0, nvml.ERROR_GPU_IS_LOST
				}
				return 0, nvml.SUCCESS
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		_, err := GetECCErrors("test-uuid", dev, true)

		assert.Error(t, err)
		assert.True(t, errors.Is(err, nvmlerrors.ErrGPULost))
	})
}

// TestGetECCErrors_AggregateDeviceMemoryCorrectedRequiresReset tests aggregate device memory corrected requires reset
func TestGetECCErrors_AggregateDeviceMemoryCorrectedRequiresReset(t *testing.T) {
	mockey.PatchConvey("GetECCErrors aggregate device memory corrected requires reset", t, func() {
		mockDevice := &mock.Device{
			GetTotalEccErrorsFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType) (uint64, nvml.Return) {
				return 0, nvml.SUCCESS
			},
			GetMemoryErrorCounterFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType, location nvml.MemoryLocation) (uint64, nvml.Return) {
				// L1, L2, SRAM succeed
				if location == nvml.MEMORY_LOCATION_L1_CACHE || location == nvml.MEMORY_LOCATION_L2_CACHE || location == nvml.MEMORY_LOCATION_SRAM {
					return 0, nvml.SUCCESS
				}
				// Aggregate device memory corrected requires reset
				if location == nvml.MEMORY_LOCATION_DEVICE_MEMORY && counterType == nvml.AGGREGATE_ECC && errorType == nvml.MEMORY_ERROR_TYPE_CORRECTED {
					return 0, nvml.ERROR_RESET_REQUIRED
				}
				return 0, nvml.SUCCESS
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		_, err := GetECCErrors("test-uuid", dev, true)

		assert.Error(t, err)
		assert.True(t, errors.Is(err, nvmlerrors.ErrGPURequiresReset))
	})
}

// TestGetECCErrors_AggregateDeviceMemoryCorrectedUnknownError tests aggregate device memory corrected unknown error
// Note: MEMORY_LOCATION_DRAM == MEMORY_LOCATION_DEVICE_MEMORY (both are 2), so error message says "dram"
func TestGetECCErrors_AggregateDeviceMemoryCorrectedUnknownError(t *testing.T) {
	mockey.PatchConvey("GetECCErrors aggregate device memory corrected unknown error", t, func() {
		mockDevice := &mock.Device{
			GetTotalEccErrorsFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType) (uint64, nvml.Return) {
				return 0, nvml.SUCCESS
			},
			GetMemoryErrorCounterFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType, location nvml.MemoryLocation) (uint64, nvml.Return) {
				// L1, L2, SRAM succeed
				if location == nvml.MEMORY_LOCATION_L1_CACHE || location == nvml.MEMORY_LOCATION_L2_CACHE || location == nvml.MEMORY_LOCATION_SRAM {
					return 0, nvml.SUCCESS
				}
				// Aggregate device memory corrected unknown error
				if location == nvml.MEMORY_LOCATION_DEVICE_MEMORY && counterType == nvml.AGGREGATE_ECC && errorType == nvml.MEMORY_ERROR_TYPE_CORRECTED {
					return 0, nvml.ERROR_UNKNOWN
				}
				return 0, nvml.SUCCESS
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		_, err := GetECCErrors("test-uuid", dev, true)

		assert.Error(t, err)
		// DRAM and DEVICE_MEMORY are the same location (2), so error comes from DRAM check first
		assert.Contains(t, err.Error(), "(aggregate, corrected) failed to get dram cache ecc errors")
	})
}

// TestGetECCErrors_AggregateDeviceMemoryUncorrectedNotSupported tests aggregate device memory uncorrected not supported
func TestGetECCErrors_AggregateDeviceMemoryUncorrectedNotSupported(t *testing.T) {
	mockey.PatchConvey("GetECCErrors aggregate device memory uncorrected not supported", t, func() {
		mockDevice := &mock.Device{
			GetTotalEccErrorsFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType) (uint64, nvml.Return) {
				return 0, nvml.SUCCESS
			},
			GetMemoryErrorCounterFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType, location nvml.MemoryLocation) (uint64, nvml.Return) {
				// L1, L2, SRAM succeed
				if location == nvml.MEMORY_LOCATION_L1_CACHE || location == nvml.MEMORY_LOCATION_L2_CACHE || location == nvml.MEMORY_LOCATION_SRAM {
					return 0, nvml.SUCCESS
				}
				// Aggregate device memory corrected succeeds
				if location == nvml.MEMORY_LOCATION_DEVICE_MEMORY && counterType == nvml.AGGREGATE_ECC && errorType == nvml.MEMORY_ERROR_TYPE_CORRECTED {
					return 0, nvml.SUCCESS
				}
				// Aggregate device memory uncorrected not supported
				if location == nvml.MEMORY_LOCATION_DEVICE_MEMORY && counterType == nvml.AGGREGATE_ECC && errorType == nvml.MEMORY_ERROR_TYPE_UNCORRECTED {
					return 0, nvml.ERROR_NOT_SUPPORTED
				}
				return 0, nvml.SUCCESS
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		result, err := GetECCErrors("test-uuid", dev, true)

		assert.NoError(t, err)
		assert.False(t, result.Supported)
	})
}

// TestGetECCErrors_AggregateDeviceMemoryUncorrectedGPULost tests aggregate device memory uncorrected GPU lost
func TestGetECCErrors_AggregateDeviceMemoryUncorrectedGPULost(t *testing.T) {
	mockey.PatchConvey("GetECCErrors aggregate device memory uncorrected GPU lost", t, func() {
		mockDevice := &mock.Device{
			GetTotalEccErrorsFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType) (uint64, nvml.Return) {
				return 0, nvml.SUCCESS
			},
			GetMemoryErrorCounterFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType, location nvml.MemoryLocation) (uint64, nvml.Return) {
				// L1, L2, SRAM succeed
				if location == nvml.MEMORY_LOCATION_L1_CACHE || location == nvml.MEMORY_LOCATION_L2_CACHE || location == nvml.MEMORY_LOCATION_SRAM {
					return 0, nvml.SUCCESS
				}
				// Aggregate device memory corrected succeeds
				if location == nvml.MEMORY_LOCATION_DEVICE_MEMORY && counterType == nvml.AGGREGATE_ECC && errorType == nvml.MEMORY_ERROR_TYPE_CORRECTED {
					return 0, nvml.SUCCESS
				}
				// Aggregate device memory uncorrected GPU lost
				if location == nvml.MEMORY_LOCATION_DEVICE_MEMORY && counterType == nvml.AGGREGATE_ECC && errorType == nvml.MEMORY_ERROR_TYPE_UNCORRECTED {
					return 0, nvml.ERROR_GPU_IS_LOST
				}
				return 0, nvml.SUCCESS
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		_, err := GetECCErrors("test-uuid", dev, true)

		assert.Error(t, err)
		assert.True(t, errors.Is(err, nvmlerrors.ErrGPULost))
	})
}

// TestGetECCErrors_AggregateDeviceMemoryUncorrectedUnknownError tests aggregate device memory uncorrected unknown error
// Note: MEMORY_LOCATION_DRAM == MEMORY_LOCATION_DEVICE_MEMORY (both are 2), so error message says "dram"
func TestGetECCErrors_AggregateDeviceMemoryUncorrectedUnknownError(t *testing.T) {
	mockey.PatchConvey("GetECCErrors aggregate device memory uncorrected unknown error", t, func() {
		mockDevice := &mock.Device{
			GetTotalEccErrorsFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType) (uint64, nvml.Return) {
				return 0, nvml.SUCCESS
			},
			GetMemoryErrorCounterFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType, location nvml.MemoryLocation) (uint64, nvml.Return) {
				// L1, L2, SRAM succeed
				if location == nvml.MEMORY_LOCATION_L1_CACHE || location == nvml.MEMORY_LOCATION_L2_CACHE || location == nvml.MEMORY_LOCATION_SRAM {
					return 0, nvml.SUCCESS
				}
				// Aggregate device memory corrected succeeds
				if location == nvml.MEMORY_LOCATION_DEVICE_MEMORY && counterType == nvml.AGGREGATE_ECC && errorType == nvml.MEMORY_ERROR_TYPE_CORRECTED {
					return 0, nvml.SUCCESS
				}
				// Aggregate device memory uncorrected unknown error
				if location == nvml.MEMORY_LOCATION_DEVICE_MEMORY && counterType == nvml.AGGREGATE_ECC && errorType == nvml.MEMORY_ERROR_TYPE_UNCORRECTED {
					return 0, nvml.ERROR_UNKNOWN
				}
				return 0, nvml.SUCCESS
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		_, err := GetECCErrors("test-uuid", dev, true)

		assert.Error(t, err)
		// DRAM and DEVICE_MEMORY are the same location (2), so error comes from DRAM check first
		assert.Contains(t, err.Error(), "(aggregate, uncorrected) failed to get dram cache ecc errors")
	})
}

// TestGetECCErrors_AggregateTextureMemoryCorrectedRequiresReset tests aggregate texture memory corrected requires reset
func TestGetECCErrors_AggregateTextureMemoryCorrectedRequiresReset(t *testing.T) {
	mockey.PatchConvey("GetECCErrors aggregate texture memory corrected requires reset", t, func() {
		mockDevice := &mock.Device{
			GetTotalEccErrorsFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType) (uint64, nvml.Return) {
				return 0, nvml.SUCCESS
			},
			GetMemoryErrorCounterFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType, location nvml.MemoryLocation) (uint64, nvml.Return) {
				// L1, L2, DRAM, SRAM, device memory succeed
				if location == nvml.MEMORY_LOCATION_L1_CACHE || location == nvml.MEMORY_LOCATION_L2_CACHE ||
					location == nvml.MEMORY_LOCATION_DRAM || location == nvml.MEMORY_LOCATION_SRAM ||
					location == nvml.MEMORY_LOCATION_DEVICE_MEMORY {
					return 0, nvml.SUCCESS
				}
				// Aggregate texture memory corrected requires reset
				if location == nvml.MEMORY_LOCATION_TEXTURE_MEMORY && counterType == nvml.AGGREGATE_ECC && errorType == nvml.MEMORY_ERROR_TYPE_CORRECTED {
					return 0, nvml.ERROR_RESET_REQUIRED
				}
				return 0, nvml.SUCCESS
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		_, err := GetECCErrors("test-uuid", dev, true)

		assert.Error(t, err)
		assert.True(t, errors.Is(err, nvmlerrors.ErrGPURequiresReset))
	})
}

// TestGetECCErrors_AggregateTextureMemoryUncorrectedNotSupported tests aggregate texture memory uncorrected not supported
func TestGetECCErrors_AggregateTextureMemoryUncorrectedNotSupported(t *testing.T) {
	mockey.PatchConvey("GetECCErrors aggregate texture memory uncorrected not supported", t, func() {
		mockDevice := &mock.Device{
			GetTotalEccErrorsFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType) (uint64, nvml.Return) {
				return 0, nvml.SUCCESS
			},
			GetMemoryErrorCounterFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType, location nvml.MemoryLocation) (uint64, nvml.Return) {
				// L1, L2, DRAM, SRAM, device memory succeed
				if location == nvml.MEMORY_LOCATION_L1_CACHE || location == nvml.MEMORY_LOCATION_L2_CACHE ||
					location == nvml.MEMORY_LOCATION_DRAM || location == nvml.MEMORY_LOCATION_SRAM ||
					location == nvml.MEMORY_LOCATION_DEVICE_MEMORY {
					return 0, nvml.SUCCESS
				}
				// Aggregate texture memory corrected succeeds
				if location == nvml.MEMORY_LOCATION_TEXTURE_MEMORY && counterType == nvml.AGGREGATE_ECC && errorType == nvml.MEMORY_ERROR_TYPE_CORRECTED {
					return 0, nvml.SUCCESS
				}
				// Aggregate texture memory uncorrected not supported
				if location == nvml.MEMORY_LOCATION_TEXTURE_MEMORY && counterType == nvml.AGGREGATE_ECC && errorType == nvml.MEMORY_ERROR_TYPE_UNCORRECTED {
					return 0, nvml.ERROR_NOT_SUPPORTED
				}
				return 0, nvml.SUCCESS
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		result, err := GetECCErrors("test-uuid", dev, true)

		assert.NoError(t, err)
		assert.False(t, result.Supported)
	})
}

// TestGetECCErrors_AggregateTextureMemoryUncorrectedGPULost tests aggregate texture memory uncorrected GPU lost
func TestGetECCErrors_AggregateTextureMemoryUncorrectedGPULost(t *testing.T) {
	mockey.PatchConvey("GetECCErrors aggregate texture memory uncorrected GPU lost", t, func() {
		mockDevice := &mock.Device{
			GetTotalEccErrorsFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType) (uint64, nvml.Return) {
				return 0, nvml.SUCCESS
			},
			GetMemoryErrorCounterFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType, location nvml.MemoryLocation) (uint64, nvml.Return) {
				// L1, L2, DRAM, SRAM, device memory succeed
				if location == nvml.MEMORY_LOCATION_L1_CACHE || location == nvml.MEMORY_LOCATION_L2_CACHE ||
					location == nvml.MEMORY_LOCATION_DRAM || location == nvml.MEMORY_LOCATION_SRAM ||
					location == nvml.MEMORY_LOCATION_DEVICE_MEMORY {
					return 0, nvml.SUCCESS
				}
				// Aggregate texture memory corrected succeeds
				if location == nvml.MEMORY_LOCATION_TEXTURE_MEMORY && counterType == nvml.AGGREGATE_ECC && errorType == nvml.MEMORY_ERROR_TYPE_CORRECTED {
					return 0, nvml.SUCCESS
				}
				// Aggregate texture memory uncorrected GPU lost
				if location == nvml.MEMORY_LOCATION_TEXTURE_MEMORY && counterType == nvml.AGGREGATE_ECC && errorType == nvml.MEMORY_ERROR_TYPE_UNCORRECTED {
					return 0, nvml.ERROR_GPU_IS_LOST
				}
				return 0, nvml.SUCCESS
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		_, err := GetECCErrors("test-uuid", dev, true)

		assert.Error(t, err)
		assert.True(t, errors.Is(err, nvmlerrors.ErrGPULost))
	})
}

// TestGetECCErrors_AggregateTextureMemoryUncorrectedUnknownError tests aggregate texture memory uncorrected unknown error
func TestGetECCErrors_AggregateTextureMemoryUncorrectedUnknownError(t *testing.T) {
	mockey.PatchConvey("GetECCErrors aggregate texture memory uncorrected unknown error", t, func() {
		mockDevice := &mock.Device{
			GetTotalEccErrorsFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType) (uint64, nvml.Return) {
				return 0, nvml.SUCCESS
			},
			GetMemoryErrorCounterFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType, location nvml.MemoryLocation) (uint64, nvml.Return) {
				// L1, L2, DRAM, SRAM, device memory succeed
				if location == nvml.MEMORY_LOCATION_L1_CACHE || location == nvml.MEMORY_LOCATION_L2_CACHE ||
					location == nvml.MEMORY_LOCATION_DRAM || location == nvml.MEMORY_LOCATION_SRAM ||
					location == nvml.MEMORY_LOCATION_DEVICE_MEMORY {
					return 0, nvml.SUCCESS
				}
				// Aggregate texture memory corrected succeeds
				if location == nvml.MEMORY_LOCATION_TEXTURE_MEMORY && counterType == nvml.AGGREGATE_ECC && errorType == nvml.MEMORY_ERROR_TYPE_CORRECTED {
					return 0, nvml.SUCCESS
				}
				// Aggregate texture memory uncorrected unknown error
				if location == nvml.MEMORY_LOCATION_TEXTURE_MEMORY && counterType == nvml.AGGREGATE_ECC && errorType == nvml.MEMORY_ERROR_TYPE_UNCORRECTED {
					return 0, nvml.ERROR_UNKNOWN
				}
				return 0, nvml.SUCCESS
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		_, err := GetECCErrors("test-uuid", dev, true)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "(aggregate, uncorrected) failed to get gpu texture memory ecc errors")
	})
}

// TestGetECCErrors_AggregateSharedMemoryUncorrectedNotSupported tests aggregate shared memory uncorrected not supported
func TestGetECCErrors_AggregateSharedMemoryUncorrectedNotSupported(t *testing.T) {
	mockey.PatchConvey("GetECCErrors aggregate shared memory uncorrected not supported", t, func() {
		mockDevice := &mock.Device{
			GetTotalEccErrorsFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType) (uint64, nvml.Return) {
				return 0, nvml.SUCCESS
			},
			GetMemoryErrorCounterFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType, location nvml.MemoryLocation) (uint64, nvml.Return) {
				// L1, L2, DRAM, SRAM, device memory, texture memory succeed
				if location == nvml.MEMORY_LOCATION_L1_CACHE || location == nvml.MEMORY_LOCATION_L2_CACHE ||
					location == nvml.MEMORY_LOCATION_DRAM || location == nvml.MEMORY_LOCATION_SRAM ||
					location == nvml.MEMORY_LOCATION_DEVICE_MEMORY || location == nvml.MEMORY_LOCATION_TEXTURE_MEMORY {
					return 0, nvml.SUCCESS
				}
				// Aggregate shared memory corrected succeeds
				if location == nvml.MEMORY_LOCATION_TEXTURE_SHM && counterType == nvml.AGGREGATE_ECC && errorType == nvml.MEMORY_ERROR_TYPE_CORRECTED {
					return 0, nvml.SUCCESS
				}
				// Aggregate shared memory uncorrected not supported
				if location == nvml.MEMORY_LOCATION_TEXTURE_SHM && counterType == nvml.AGGREGATE_ECC && errorType == nvml.MEMORY_ERROR_TYPE_UNCORRECTED {
					return 0, nvml.ERROR_NOT_SUPPORTED
				}
				return 0, nvml.SUCCESS
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		result, err := GetECCErrors("test-uuid", dev, true)

		assert.NoError(t, err)
		assert.False(t, result.Supported)
	})
}

// TestGetECCErrors_AggregateSharedMemoryUncorrectedGPULost tests aggregate shared memory uncorrected GPU lost
func TestGetECCErrors_AggregateSharedMemoryUncorrectedGPULost(t *testing.T) {
	mockey.PatchConvey("GetECCErrors aggregate shared memory uncorrected GPU lost", t, func() {
		mockDevice := &mock.Device{
			GetTotalEccErrorsFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType) (uint64, nvml.Return) {
				return 0, nvml.SUCCESS
			},
			GetMemoryErrorCounterFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType, location nvml.MemoryLocation) (uint64, nvml.Return) {
				// L1, L2, DRAM, SRAM, device memory, texture memory succeed
				if location == nvml.MEMORY_LOCATION_L1_CACHE || location == nvml.MEMORY_LOCATION_L2_CACHE ||
					location == nvml.MEMORY_LOCATION_DRAM || location == nvml.MEMORY_LOCATION_SRAM ||
					location == nvml.MEMORY_LOCATION_DEVICE_MEMORY || location == nvml.MEMORY_LOCATION_TEXTURE_MEMORY {
					return 0, nvml.SUCCESS
				}
				// Aggregate shared memory corrected succeeds
				if location == nvml.MEMORY_LOCATION_TEXTURE_SHM && counterType == nvml.AGGREGATE_ECC && errorType == nvml.MEMORY_ERROR_TYPE_CORRECTED {
					return 0, nvml.SUCCESS
				}
				// Aggregate shared memory uncorrected GPU lost
				if location == nvml.MEMORY_LOCATION_TEXTURE_SHM && counterType == nvml.AGGREGATE_ECC && errorType == nvml.MEMORY_ERROR_TYPE_UNCORRECTED {
					return 0, nvml.ERROR_GPU_IS_LOST
				}
				return 0, nvml.SUCCESS
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		_, err := GetECCErrors("test-uuid", dev, true)

		assert.Error(t, err)
		assert.True(t, errors.Is(err, nvmlerrors.ErrGPULost))
	})
}

// TestGetECCErrors_AggregateSharedMemoryUncorrectedUnknownError tests aggregate shared memory uncorrected unknown error
func TestGetECCErrors_AggregateSharedMemoryUncorrectedUnknownError(t *testing.T) {
	mockey.PatchConvey("GetECCErrors aggregate shared memory uncorrected unknown error", t, func() {
		mockDevice := &mock.Device{
			GetTotalEccErrorsFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType) (uint64, nvml.Return) {
				return 0, nvml.SUCCESS
			},
			GetMemoryErrorCounterFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType, location nvml.MemoryLocation) (uint64, nvml.Return) {
				// L1, L2, DRAM, SRAM, device memory, texture memory succeed
				if location == nvml.MEMORY_LOCATION_L1_CACHE || location == nvml.MEMORY_LOCATION_L2_CACHE ||
					location == nvml.MEMORY_LOCATION_DRAM || location == nvml.MEMORY_LOCATION_SRAM ||
					location == nvml.MEMORY_LOCATION_DEVICE_MEMORY || location == nvml.MEMORY_LOCATION_TEXTURE_MEMORY {
					return 0, nvml.SUCCESS
				}
				// Aggregate shared memory corrected succeeds
				if location == nvml.MEMORY_LOCATION_TEXTURE_SHM && counterType == nvml.AGGREGATE_ECC && errorType == nvml.MEMORY_ERROR_TYPE_CORRECTED {
					return 0, nvml.SUCCESS
				}
				// Aggregate shared memory uncorrected unknown error
				if location == nvml.MEMORY_LOCATION_TEXTURE_SHM && counterType == nvml.AGGREGATE_ECC && errorType == nvml.MEMORY_ERROR_TYPE_UNCORRECTED {
					return 0, nvml.ERROR_UNKNOWN
				}
				return 0, nvml.SUCCESS
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		_, err := GetECCErrors("test-uuid", dev, true)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "(aggregate, uncorrected) failed to get shared memory ecc errors")
	})
}

// TestGetECCErrors_VolatileL1CorrectedUnknownError tests volatile L1 corrected unknown error
func TestGetECCErrors_VolatileL1CorrectedUnknownError(t *testing.T) {
	mockey.PatchConvey("GetECCErrors volatile L1 corrected unknown error", t, func() {
		mockDevice := &mock.Device{
			GetTotalEccErrorsFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType) (uint64, nvml.Return) {
				return 0, nvml.SUCCESS
			},
			GetMemoryErrorCounterFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType, location nvml.MemoryLocation) (uint64, nvml.Return) {
				// All aggregate succeed
				if counterType == nvml.AGGREGATE_ECC {
					return 0, nvml.SUCCESS
				}
				// Volatile L1 corrected unknown error
				if location == nvml.MEMORY_LOCATION_L1_CACHE && counterType == nvml.VOLATILE_ECC && errorType == nvml.MEMORY_ERROR_TYPE_CORRECTED {
					return 0, nvml.ERROR_UNKNOWN
				}
				return 0, nvml.SUCCESS
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		_, err := GetECCErrors("test-uuid", dev, true)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "(volatile, corrected) failed to get l1 cache ecc errors")
	})
}

// TestGetECCErrors_VolatileL1UncorrectedNotSupported tests volatile L1 uncorrected not supported
func TestGetECCErrors_VolatileL1UncorrectedNotSupported(t *testing.T) {
	mockey.PatchConvey("GetECCErrors volatile L1 uncorrected not supported", t, func() {
		mockDevice := &mock.Device{
			GetTotalEccErrorsFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType) (uint64, nvml.Return) {
				return 0, nvml.SUCCESS
			},
			GetMemoryErrorCounterFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType, location nvml.MemoryLocation) (uint64, nvml.Return) {
				// All aggregate succeed
				if counterType == nvml.AGGREGATE_ECC {
					return 0, nvml.SUCCESS
				}
				// Volatile L1 corrected succeeds
				if location == nvml.MEMORY_LOCATION_L1_CACHE && counterType == nvml.VOLATILE_ECC && errorType == nvml.MEMORY_ERROR_TYPE_CORRECTED {
					return 0, nvml.SUCCESS
				}
				// Volatile L1 uncorrected not supported
				if location == nvml.MEMORY_LOCATION_L1_CACHE && counterType == nvml.VOLATILE_ECC && errorType == nvml.MEMORY_ERROR_TYPE_UNCORRECTED {
					return 0, nvml.ERROR_NOT_SUPPORTED
				}
				return 0, nvml.SUCCESS
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		result, err := GetECCErrors("test-uuid", dev, true)

		assert.NoError(t, err)
		assert.False(t, result.Supported)
	})
}

// TestGetECCErrors_VolatileL1UncorrectedGPULost tests volatile L1 uncorrected GPU lost
func TestGetECCErrors_VolatileL1UncorrectedGPULost(t *testing.T) {
	mockey.PatchConvey("GetECCErrors volatile L1 uncorrected GPU lost", t, func() {
		mockDevice := &mock.Device{
			GetTotalEccErrorsFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType) (uint64, nvml.Return) {
				return 0, nvml.SUCCESS
			},
			GetMemoryErrorCounterFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType, location nvml.MemoryLocation) (uint64, nvml.Return) {
				// All aggregate succeed
				if counterType == nvml.AGGREGATE_ECC {
					return 0, nvml.SUCCESS
				}
				// Volatile L1 corrected succeeds
				if location == nvml.MEMORY_LOCATION_L1_CACHE && counterType == nvml.VOLATILE_ECC && errorType == nvml.MEMORY_ERROR_TYPE_CORRECTED {
					return 0, nvml.SUCCESS
				}
				// Volatile L1 uncorrected GPU lost
				if location == nvml.MEMORY_LOCATION_L1_CACHE && counterType == nvml.VOLATILE_ECC && errorType == nvml.MEMORY_ERROR_TYPE_UNCORRECTED {
					return 0, nvml.ERROR_GPU_IS_LOST
				}
				return 0, nvml.SUCCESS
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		_, err := GetECCErrors("test-uuid", dev, true)

		assert.Error(t, err)
		assert.True(t, errors.Is(err, nvmlerrors.ErrGPULost))
	})
}

// TestGetECCErrors_VolatileL1UncorrectedUnknownError tests volatile L1 uncorrected unknown error
func TestGetECCErrors_VolatileL1UncorrectedUnknownError(t *testing.T) {
	mockey.PatchConvey("GetECCErrors volatile L1 uncorrected unknown error", t, func() {
		mockDevice := &mock.Device{
			GetTotalEccErrorsFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType) (uint64, nvml.Return) {
				return 0, nvml.SUCCESS
			},
			GetMemoryErrorCounterFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType, location nvml.MemoryLocation) (uint64, nvml.Return) {
				// All aggregate succeed
				if counterType == nvml.AGGREGATE_ECC {
					return 0, nvml.SUCCESS
				}
				// Volatile L1 corrected succeeds
				if location == nvml.MEMORY_LOCATION_L1_CACHE && counterType == nvml.VOLATILE_ECC && errorType == nvml.MEMORY_ERROR_TYPE_CORRECTED {
					return 0, nvml.SUCCESS
				}
				// Volatile L1 uncorrected unknown error
				if location == nvml.MEMORY_LOCATION_L1_CACHE && counterType == nvml.VOLATILE_ECC && errorType == nvml.MEMORY_ERROR_TYPE_UNCORRECTED {
					return 0, nvml.ERROR_UNKNOWN
				}
				return 0, nvml.SUCCESS
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		_, err := GetECCErrors("test-uuid", dev, true)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "(volatile, uncorrected) failed to get l1 cache ecc errors")
	})
}

// TestGetECCErrors_VolatileL2UncorrectedNotSupported tests volatile L2 uncorrected not supported
func TestGetECCErrors_VolatileL2UncorrectedNotSupported(t *testing.T) {
	mockey.PatchConvey("GetECCErrors volatile L2 uncorrected not supported", t, func() {
		mockDevice := &mock.Device{
			GetTotalEccErrorsFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType) (uint64, nvml.Return) {
				return 0, nvml.SUCCESS
			},
			GetMemoryErrorCounterFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType, location nvml.MemoryLocation) (uint64, nvml.Return) {
				// All aggregate succeed
				if counterType == nvml.AGGREGATE_ECC {
					return 0, nvml.SUCCESS
				}
				// Volatile L1 succeeds
				if location == nvml.MEMORY_LOCATION_L1_CACHE {
					return 0, nvml.SUCCESS
				}
				// Volatile L2 corrected succeeds
				if location == nvml.MEMORY_LOCATION_L2_CACHE && errorType == nvml.MEMORY_ERROR_TYPE_CORRECTED {
					return 0, nvml.SUCCESS
				}
				// Volatile L2 uncorrected not supported
				if location == nvml.MEMORY_LOCATION_L2_CACHE && errorType == nvml.MEMORY_ERROR_TYPE_UNCORRECTED {
					return 0, nvml.ERROR_NOT_SUPPORTED
				}
				return 0, nvml.SUCCESS
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		result, err := GetECCErrors("test-uuid", dev, true)

		assert.NoError(t, err)
		assert.False(t, result.Supported)
	})
}

// TestGetECCErrors_VolatileL2UncorrectedGPULost tests volatile L2 uncorrected GPU lost
func TestGetECCErrors_VolatileL2UncorrectedGPULost(t *testing.T) {
	mockey.PatchConvey("GetECCErrors volatile L2 uncorrected GPU lost", t, func() {
		mockDevice := &mock.Device{
			GetTotalEccErrorsFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType) (uint64, nvml.Return) {
				return 0, nvml.SUCCESS
			},
			GetMemoryErrorCounterFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType, location nvml.MemoryLocation) (uint64, nvml.Return) {
				// All aggregate succeed
				if counterType == nvml.AGGREGATE_ECC {
					return 0, nvml.SUCCESS
				}
				// Volatile L1 succeeds
				if location == nvml.MEMORY_LOCATION_L1_CACHE {
					return 0, nvml.SUCCESS
				}
				// Volatile L2 corrected succeeds
				if location == nvml.MEMORY_LOCATION_L2_CACHE && errorType == nvml.MEMORY_ERROR_TYPE_CORRECTED {
					return 0, nvml.SUCCESS
				}
				// Volatile L2 uncorrected GPU lost
				if location == nvml.MEMORY_LOCATION_L2_CACHE && errorType == nvml.MEMORY_ERROR_TYPE_UNCORRECTED {
					return 0, nvml.ERROR_GPU_IS_LOST
				}
				return 0, nvml.SUCCESS
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		_, err := GetECCErrors("test-uuid", dev, true)

		assert.Error(t, err)
		assert.True(t, errors.Is(err, nvmlerrors.ErrGPULost))
	})
}

// TestGetECCErrors_VolatileL2UncorrectedUnknownError tests volatile L2 uncorrected unknown error
func TestGetECCErrors_VolatileL2UncorrectedUnknownError(t *testing.T) {
	mockey.PatchConvey("GetECCErrors volatile L2 uncorrected unknown error", t, func() {
		mockDevice := &mock.Device{
			GetTotalEccErrorsFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType) (uint64, nvml.Return) {
				return 0, nvml.SUCCESS
			},
			GetMemoryErrorCounterFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType, location nvml.MemoryLocation) (uint64, nvml.Return) {
				// All aggregate succeed
				if counterType == nvml.AGGREGATE_ECC {
					return 0, nvml.SUCCESS
				}
				// Volatile L1 succeeds
				if location == nvml.MEMORY_LOCATION_L1_CACHE {
					return 0, nvml.SUCCESS
				}
				// Volatile L2 corrected succeeds
				if location == nvml.MEMORY_LOCATION_L2_CACHE && errorType == nvml.MEMORY_ERROR_TYPE_CORRECTED {
					return 0, nvml.SUCCESS
				}
				// Volatile L2 uncorrected unknown error
				if location == nvml.MEMORY_LOCATION_L2_CACHE && errorType == nvml.MEMORY_ERROR_TYPE_UNCORRECTED {
					return 0, nvml.ERROR_UNKNOWN
				}
				return 0, nvml.SUCCESS
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		_, err := GetECCErrors("test-uuid", dev, true)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "(volatile, uncorrected) failed to get l2 cache ecc errors")
	})
}

// TestGetECCErrors_VolatileDRAMCorrectedUnknownError tests volatile DRAM corrected unknown error
func TestGetECCErrors_VolatileDRAMCorrectedUnknownError(t *testing.T) {
	mockey.PatchConvey("GetECCErrors volatile DRAM corrected unknown error", t, func() {
		mockDevice := &mock.Device{
			GetTotalEccErrorsFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType) (uint64, nvml.Return) {
				return 0, nvml.SUCCESS
			},
			GetMemoryErrorCounterFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType, location nvml.MemoryLocation) (uint64, nvml.Return) {
				// All aggregate succeed
				if counterType == nvml.AGGREGATE_ECC {
					return 0, nvml.SUCCESS
				}
				// Volatile L1, L2 succeed
				if location == nvml.MEMORY_LOCATION_L1_CACHE || location == nvml.MEMORY_LOCATION_L2_CACHE {
					return 0, nvml.SUCCESS
				}
				// Volatile DRAM corrected unknown error
				if location == nvml.MEMORY_LOCATION_DRAM && errorType == nvml.MEMORY_ERROR_TYPE_CORRECTED {
					return 0, nvml.ERROR_UNKNOWN
				}
				return 0, nvml.SUCCESS
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		_, err := GetECCErrors("test-uuid", dev, true)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "(volatile, corrected) failed to get dram cache ecc errors")
	})
}

// TestGetECCErrors_VolatileDRAMUncorrectedNotSupportedBranch tests volatile DRAM uncorrected not supported
func TestGetECCErrors_VolatileDRAMUncorrectedNotSupportedBranch(t *testing.T) {
	mockey.PatchConvey("GetECCErrors volatile DRAM uncorrected not supported branch", t, func() {
		mockDevice := &mock.Device{
			GetTotalEccErrorsFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType) (uint64, nvml.Return) {
				return 0, nvml.SUCCESS
			},
			GetMemoryErrorCounterFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType, location nvml.MemoryLocation) (uint64, nvml.Return) {
				// All aggregate succeed
				if counterType == nvml.AGGREGATE_ECC {
					return 0, nvml.SUCCESS
				}
				// Volatile L1, L2 succeed
				if location == nvml.MEMORY_LOCATION_L1_CACHE || location == nvml.MEMORY_LOCATION_L2_CACHE {
					return 0, nvml.SUCCESS
				}
				// Volatile DRAM corrected succeeds
				if location == nvml.MEMORY_LOCATION_DRAM && errorType == nvml.MEMORY_ERROR_TYPE_CORRECTED {
					return 0, nvml.SUCCESS
				}
				// Volatile DRAM uncorrected not supported
				if location == nvml.MEMORY_LOCATION_DRAM && errorType == nvml.MEMORY_ERROR_TYPE_UNCORRECTED {
					return 0, nvml.ERROR_NOT_SUPPORTED
				}
				return 0, nvml.SUCCESS
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		result, err := GetECCErrors("test-uuid", dev, true)

		assert.NoError(t, err)
		assert.False(t, result.Supported)
	})
}

// TestGetECCErrors_VolatileDRAMUncorrectedGPULostBranch tests volatile DRAM uncorrected GPU lost
func TestGetECCErrors_VolatileDRAMUncorrectedGPULostBranch(t *testing.T) {
	mockey.PatchConvey("GetECCErrors volatile DRAM uncorrected GPU lost branch", t, func() {
		mockDevice := &mock.Device{
			GetTotalEccErrorsFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType) (uint64, nvml.Return) {
				return 0, nvml.SUCCESS
			},
			GetMemoryErrorCounterFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType, location nvml.MemoryLocation) (uint64, nvml.Return) {
				// All aggregate succeed
				if counterType == nvml.AGGREGATE_ECC {
					return 0, nvml.SUCCESS
				}
				// Volatile L1, L2 succeed
				if location == nvml.MEMORY_LOCATION_L1_CACHE || location == nvml.MEMORY_LOCATION_L2_CACHE {
					return 0, nvml.SUCCESS
				}
				// Volatile DRAM corrected succeeds
				if location == nvml.MEMORY_LOCATION_DRAM && errorType == nvml.MEMORY_ERROR_TYPE_CORRECTED {
					return 0, nvml.SUCCESS
				}
				// Volatile DRAM uncorrected GPU lost
				if location == nvml.MEMORY_LOCATION_DRAM && errorType == nvml.MEMORY_ERROR_TYPE_UNCORRECTED {
					return 0, nvml.ERROR_GPU_IS_LOST
				}
				return 0, nvml.SUCCESS
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		_, err := GetECCErrors("test-uuid", dev, true)

		assert.Error(t, err)
		assert.True(t, errors.Is(err, nvmlerrors.ErrGPULost))
	})
}

// TestGetECCErrors_VolatileDRAMUncorrectedUnknownError tests volatile DRAM uncorrected unknown error
func TestGetECCErrors_VolatileDRAMUncorrectedUnknownError(t *testing.T) {
	mockey.PatchConvey("GetECCErrors volatile DRAM uncorrected unknown error", t, func() {
		mockDevice := &mock.Device{
			GetTotalEccErrorsFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType) (uint64, nvml.Return) {
				return 0, nvml.SUCCESS
			},
			GetMemoryErrorCounterFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType, location nvml.MemoryLocation) (uint64, nvml.Return) {
				// All aggregate succeed
				if counterType == nvml.AGGREGATE_ECC {
					return 0, nvml.SUCCESS
				}
				// Volatile L1, L2 succeed
				if location == nvml.MEMORY_LOCATION_L1_CACHE || location == nvml.MEMORY_LOCATION_L2_CACHE {
					return 0, nvml.SUCCESS
				}
				// Volatile DRAM corrected succeeds
				if location == nvml.MEMORY_LOCATION_DRAM && errorType == nvml.MEMORY_ERROR_TYPE_CORRECTED {
					return 0, nvml.SUCCESS
				}
				// Volatile DRAM uncorrected unknown error
				if location == nvml.MEMORY_LOCATION_DRAM && errorType == nvml.MEMORY_ERROR_TYPE_UNCORRECTED {
					return 0, nvml.ERROR_UNKNOWN
				}
				return 0, nvml.SUCCESS
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		_, err := GetECCErrors("test-uuid", dev, true)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "(volatile, uncorrected) failed to get dram cache ecc errors")
	})
}

// TestGetECCErrors_VolatileSRAMCorrectedUnknownError tests volatile SRAM corrected unknown error
func TestGetECCErrors_VolatileSRAMCorrectedUnknownError(t *testing.T) {
	mockey.PatchConvey("GetECCErrors volatile SRAM corrected unknown error", t, func() {
		mockDevice := &mock.Device{
			GetTotalEccErrorsFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType) (uint64, nvml.Return) {
				return 0, nvml.SUCCESS
			},
			GetMemoryErrorCounterFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType, location nvml.MemoryLocation) (uint64, nvml.Return) {
				// All aggregate succeed
				if counterType == nvml.AGGREGATE_ECC {
					return 0, nvml.SUCCESS
				}
				// Volatile L1, L2, DRAM succeed
				if location == nvml.MEMORY_LOCATION_L1_CACHE || location == nvml.MEMORY_LOCATION_L2_CACHE || location == nvml.MEMORY_LOCATION_DRAM {
					return 0, nvml.SUCCESS
				}
				// Volatile SRAM corrected unknown error
				if location == nvml.MEMORY_LOCATION_SRAM && errorType == nvml.MEMORY_ERROR_TYPE_CORRECTED {
					return 0, nvml.ERROR_UNKNOWN
				}
				return 0, nvml.SUCCESS
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		_, err := GetECCErrors("test-uuid", dev, true)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "(volatile, corrected) failed to get sram ecc errors")
	})
}

// TestGetECCErrors_VolatileSRAMUncorrectedNotSupportedBranch tests volatile SRAM uncorrected not supported
func TestGetECCErrors_VolatileSRAMUncorrectedNotSupportedBranch(t *testing.T) {
	mockey.PatchConvey("GetECCErrors volatile SRAM uncorrected not supported branch", t, func() {
		mockDevice := &mock.Device{
			GetTotalEccErrorsFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType) (uint64, nvml.Return) {
				return 0, nvml.SUCCESS
			},
			GetMemoryErrorCounterFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType, location nvml.MemoryLocation) (uint64, nvml.Return) {
				// All aggregate succeed
				if counterType == nvml.AGGREGATE_ECC {
					return 0, nvml.SUCCESS
				}
				// Volatile L1, L2, DRAM succeed
				if location == nvml.MEMORY_LOCATION_L1_CACHE || location == nvml.MEMORY_LOCATION_L2_CACHE || location == nvml.MEMORY_LOCATION_DRAM {
					return 0, nvml.SUCCESS
				}
				// Volatile SRAM corrected succeeds
				if location == nvml.MEMORY_LOCATION_SRAM && errorType == nvml.MEMORY_ERROR_TYPE_CORRECTED {
					return 0, nvml.SUCCESS
				}
				// Volatile SRAM uncorrected not supported
				if location == nvml.MEMORY_LOCATION_SRAM && errorType == nvml.MEMORY_ERROR_TYPE_UNCORRECTED {
					return 0, nvml.ERROR_NOT_SUPPORTED
				}
				return 0, nvml.SUCCESS
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		result, err := GetECCErrors("test-uuid", dev, true)

		assert.NoError(t, err)
		assert.False(t, result.Supported)
	})
}

// TestGetECCErrors_VolatileSRAMUncorrectedGPULostBranch tests volatile SRAM uncorrected GPU lost
func TestGetECCErrors_VolatileSRAMUncorrectedGPULostBranch(t *testing.T) {
	mockey.PatchConvey("GetECCErrors volatile SRAM uncorrected GPU lost branch", t, func() {
		mockDevice := &mock.Device{
			GetTotalEccErrorsFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType) (uint64, nvml.Return) {
				return 0, nvml.SUCCESS
			},
			GetMemoryErrorCounterFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType, location nvml.MemoryLocation) (uint64, nvml.Return) {
				// All aggregate succeed
				if counterType == nvml.AGGREGATE_ECC {
					return 0, nvml.SUCCESS
				}
				// Volatile L1, L2, DRAM succeed
				if location == nvml.MEMORY_LOCATION_L1_CACHE || location == nvml.MEMORY_LOCATION_L2_CACHE || location == nvml.MEMORY_LOCATION_DRAM {
					return 0, nvml.SUCCESS
				}
				// Volatile SRAM corrected succeeds
				if location == nvml.MEMORY_LOCATION_SRAM && errorType == nvml.MEMORY_ERROR_TYPE_CORRECTED {
					return 0, nvml.SUCCESS
				}
				// Volatile SRAM uncorrected GPU lost
				if location == nvml.MEMORY_LOCATION_SRAM && errorType == nvml.MEMORY_ERROR_TYPE_UNCORRECTED {
					return 0, nvml.ERROR_GPU_IS_LOST
				}
				return 0, nvml.SUCCESS
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		_, err := GetECCErrors("test-uuid", dev, true)

		assert.Error(t, err)
		assert.True(t, errors.Is(err, nvmlerrors.ErrGPULost))
	})
}

// TestGetECCErrors_VolatileSRAMUncorrectedUnknownError tests volatile SRAM uncorrected unknown error
func TestGetECCErrors_VolatileSRAMUncorrectedUnknownError(t *testing.T) {
	mockey.PatchConvey("GetECCErrors volatile SRAM uncorrected unknown error", t, func() {
		mockDevice := &mock.Device{
			GetTotalEccErrorsFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType) (uint64, nvml.Return) {
				return 0, nvml.SUCCESS
			},
			GetMemoryErrorCounterFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType, location nvml.MemoryLocation) (uint64, nvml.Return) {
				// All aggregate succeed
				if counterType == nvml.AGGREGATE_ECC {
					return 0, nvml.SUCCESS
				}
				// Volatile L1, L2, DRAM succeed
				if location == nvml.MEMORY_LOCATION_L1_CACHE || location == nvml.MEMORY_LOCATION_L2_CACHE || location == nvml.MEMORY_LOCATION_DRAM {
					return 0, nvml.SUCCESS
				}
				// Volatile SRAM corrected succeeds
				if location == nvml.MEMORY_LOCATION_SRAM && errorType == nvml.MEMORY_ERROR_TYPE_CORRECTED {
					return 0, nvml.SUCCESS
				}
				// Volatile SRAM uncorrected unknown error
				if location == nvml.MEMORY_LOCATION_SRAM && errorType == nvml.MEMORY_ERROR_TYPE_UNCORRECTED {
					return 0, nvml.ERROR_UNKNOWN
				}
				return 0, nvml.SUCCESS
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		_, err := GetECCErrors("test-uuid", dev, true)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "(volatile, uncorrected) failed to get sram ecc errors")
	})
}

// TestGetECCErrors_VolatileDeviceMemoryCorrectedNotSupported tests volatile device memory corrected not supported
func TestGetECCErrors_VolatileDeviceMemoryCorrectedNotSupported(t *testing.T) {
	mockey.PatchConvey("GetECCErrors volatile device memory corrected not supported", t, func() {
		mockDevice := &mock.Device{
			GetTotalEccErrorsFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType) (uint64, nvml.Return) {
				return 0, nvml.SUCCESS
			},
			GetMemoryErrorCounterFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType, location nvml.MemoryLocation) (uint64, nvml.Return) {
				// All aggregate succeed
				if counterType == nvml.AGGREGATE_ECC {
					return 0, nvml.SUCCESS
				}
				// Volatile L1, L2, SRAM succeed (but not DRAM since DRAM == DEVICE_MEMORY)
				if location == nvml.MEMORY_LOCATION_L1_CACHE || location == nvml.MEMORY_LOCATION_L2_CACHE || location == nvml.MEMORY_LOCATION_SRAM {
					return 0, nvml.SUCCESS
				}
				// Volatile device memory corrected not supported
				if location == nvml.MEMORY_LOCATION_DEVICE_MEMORY && counterType == nvml.VOLATILE_ECC && errorType == nvml.MEMORY_ERROR_TYPE_CORRECTED {
					return 0, nvml.ERROR_NOT_SUPPORTED
				}
				return 0, nvml.SUCCESS
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		result, err := GetECCErrors("test-uuid", dev, true)

		assert.NoError(t, err)
		assert.False(t, result.Supported)
	})
}

// TestGetECCErrors_VolatileDeviceMemoryCorrectedUnknownError tests volatile device memory corrected unknown error
// Note: MEMORY_LOCATION_DRAM == MEMORY_LOCATION_DEVICE_MEMORY (both are 2), so error message says "dram"
func TestGetECCErrors_VolatileDeviceMemoryCorrectedUnknownError(t *testing.T) {
	mockey.PatchConvey("GetECCErrors volatile device memory corrected unknown error", t, func() {
		mockDevice := &mock.Device{
			GetTotalEccErrorsFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType) (uint64, nvml.Return) {
				return 0, nvml.SUCCESS
			},
			GetMemoryErrorCounterFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType, location nvml.MemoryLocation) (uint64, nvml.Return) {
				// All aggregate succeed
				if counterType == nvml.AGGREGATE_ECC {
					return 0, nvml.SUCCESS
				}
				// Volatile L1, L2, SRAM succeed
				if location == nvml.MEMORY_LOCATION_L1_CACHE || location == nvml.MEMORY_LOCATION_L2_CACHE || location == nvml.MEMORY_LOCATION_SRAM {
					return 0, nvml.SUCCESS
				}
				// Volatile device memory corrected unknown error
				if location == nvml.MEMORY_LOCATION_DEVICE_MEMORY && counterType == nvml.VOLATILE_ECC && errorType == nvml.MEMORY_ERROR_TYPE_CORRECTED {
					return 0, nvml.ERROR_UNKNOWN
				}
				return 0, nvml.SUCCESS
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		_, err := GetECCErrors("test-uuid", dev, true)

		assert.Error(t, err)
		// DRAM and DEVICE_MEMORY are the same location (2), so error comes from DRAM check first
		assert.Contains(t, err.Error(), "(volatile, corrected) failed to get dram cache ecc errors")
	})
}

// TestGetECCErrors_VolatileDeviceMemoryUncorrectedNotSupported tests volatile device memory uncorrected not supported
func TestGetECCErrors_VolatileDeviceMemoryUncorrectedNotSupportedBranch(t *testing.T) {
	mockey.PatchConvey("GetECCErrors volatile device memory uncorrected not supported branch", t, func() {
		mockDevice := &mock.Device{
			GetTotalEccErrorsFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType) (uint64, nvml.Return) {
				return 0, nvml.SUCCESS
			},
			GetMemoryErrorCounterFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType, location nvml.MemoryLocation) (uint64, nvml.Return) {
				// All aggregate succeed
				if counterType == nvml.AGGREGATE_ECC {
					return 0, nvml.SUCCESS
				}
				// Volatile L1, L2, SRAM succeed
				if location == nvml.MEMORY_LOCATION_L1_CACHE || location == nvml.MEMORY_LOCATION_L2_CACHE || location == nvml.MEMORY_LOCATION_SRAM {
					return 0, nvml.SUCCESS
				}
				// Volatile device memory corrected succeeds
				if location == nvml.MEMORY_LOCATION_DEVICE_MEMORY && counterType == nvml.VOLATILE_ECC && errorType == nvml.MEMORY_ERROR_TYPE_CORRECTED {
					return 0, nvml.SUCCESS
				}
				// Volatile device memory uncorrected not supported
				if location == nvml.MEMORY_LOCATION_DEVICE_MEMORY && counterType == nvml.VOLATILE_ECC && errorType == nvml.MEMORY_ERROR_TYPE_UNCORRECTED {
					return 0, nvml.ERROR_NOT_SUPPORTED
				}
				return 0, nvml.SUCCESS
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		result, err := GetECCErrors("test-uuid", dev, true)

		assert.NoError(t, err)
		assert.False(t, result.Supported)
	})
}

// TestGetECCErrors_VolatileDeviceMemoryUncorrectedGPULostBranch tests volatile device memory uncorrected GPU lost
func TestGetECCErrors_VolatileDeviceMemoryUncorrectedGPULostBranch(t *testing.T) {
	mockey.PatchConvey("GetECCErrors volatile device memory uncorrected GPU lost branch", t, func() {
		mockDevice := &mock.Device{
			GetTotalEccErrorsFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType) (uint64, nvml.Return) {
				return 0, nvml.SUCCESS
			},
			GetMemoryErrorCounterFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType, location nvml.MemoryLocation) (uint64, nvml.Return) {
				// All aggregate succeed
				if counterType == nvml.AGGREGATE_ECC {
					return 0, nvml.SUCCESS
				}
				// Volatile L1, L2, SRAM succeed
				if location == nvml.MEMORY_LOCATION_L1_CACHE || location == nvml.MEMORY_LOCATION_L2_CACHE || location == nvml.MEMORY_LOCATION_SRAM {
					return 0, nvml.SUCCESS
				}
				// Volatile device memory corrected succeeds
				if location == nvml.MEMORY_LOCATION_DEVICE_MEMORY && counterType == nvml.VOLATILE_ECC && errorType == nvml.MEMORY_ERROR_TYPE_CORRECTED {
					return 0, nvml.SUCCESS
				}
				// Volatile device memory uncorrected GPU lost
				if location == nvml.MEMORY_LOCATION_DEVICE_MEMORY && counterType == nvml.VOLATILE_ECC && errorType == nvml.MEMORY_ERROR_TYPE_UNCORRECTED {
					return 0, nvml.ERROR_GPU_IS_LOST
				}
				return 0, nvml.SUCCESS
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		_, err := GetECCErrors("test-uuid", dev, true)

		assert.Error(t, err)
		assert.True(t, errors.Is(err, nvmlerrors.ErrGPULost))
	})
}

// TestGetECCErrors_VolatileDeviceMemoryUncorrectedUnknownError tests volatile device memory uncorrected unknown error
// Note: MEMORY_LOCATION_DRAM == MEMORY_LOCATION_DEVICE_MEMORY (both are 2), so error message says "dram"
func TestGetECCErrors_VolatileDeviceMemoryUncorrectedUnknownError(t *testing.T) {
	mockey.PatchConvey("GetECCErrors volatile device memory uncorrected unknown error", t, func() {
		mockDevice := &mock.Device{
			GetTotalEccErrorsFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType) (uint64, nvml.Return) {
				return 0, nvml.SUCCESS
			},
			GetMemoryErrorCounterFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType, location nvml.MemoryLocation) (uint64, nvml.Return) {
				// All aggregate succeed
				if counterType == nvml.AGGREGATE_ECC {
					return 0, nvml.SUCCESS
				}
				// Volatile L1, L2, SRAM succeed
				if location == nvml.MEMORY_LOCATION_L1_CACHE || location == nvml.MEMORY_LOCATION_L2_CACHE || location == nvml.MEMORY_LOCATION_SRAM {
					return 0, nvml.SUCCESS
				}
				// Volatile device memory corrected succeeds
				if location == nvml.MEMORY_LOCATION_DEVICE_MEMORY && counterType == nvml.VOLATILE_ECC && errorType == nvml.MEMORY_ERROR_TYPE_CORRECTED {
					return 0, nvml.SUCCESS
				}
				// Volatile device memory uncorrected unknown error
				if location == nvml.MEMORY_LOCATION_DEVICE_MEMORY && counterType == nvml.VOLATILE_ECC && errorType == nvml.MEMORY_ERROR_TYPE_UNCORRECTED {
					return 0, nvml.ERROR_UNKNOWN
				}
				return 0, nvml.SUCCESS
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		_, err := GetECCErrors("test-uuid", dev, true)

		assert.Error(t, err)
		// DRAM and DEVICE_MEMORY are the same location (2), so error comes from DRAM check first
		assert.Contains(t, err.Error(), "(volatile, uncorrected) failed to get dram cache ecc errors")
	})
}

// TestGetECCErrors_VolatileTextureMemoryCorrectedUnknownError tests volatile texture memory corrected unknown error
func TestGetECCErrors_VolatileTextureMemoryCorrectedUnknownError(t *testing.T) {
	mockey.PatchConvey("GetECCErrors volatile texture memory corrected unknown error", t, func() {
		mockDevice := &mock.Device{
			GetTotalEccErrorsFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType) (uint64, nvml.Return) {
				return 0, nvml.SUCCESS
			},
			GetMemoryErrorCounterFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType, location nvml.MemoryLocation) (uint64, nvml.Return) {
				// All aggregate succeed
				if counterType == nvml.AGGREGATE_ECC {
					return 0, nvml.SUCCESS
				}
				// Volatile L1, L2, DRAM, SRAM, device memory succeed
				if location == nvml.MEMORY_LOCATION_L1_CACHE || location == nvml.MEMORY_LOCATION_L2_CACHE ||
					location == nvml.MEMORY_LOCATION_DRAM || location == nvml.MEMORY_LOCATION_SRAM ||
					location == nvml.MEMORY_LOCATION_DEVICE_MEMORY {
					return 0, nvml.SUCCESS
				}
				// Volatile texture memory corrected unknown error
				if location == nvml.MEMORY_LOCATION_TEXTURE_MEMORY && counterType == nvml.VOLATILE_ECC && errorType == nvml.MEMORY_ERROR_TYPE_CORRECTED {
					return 0, nvml.ERROR_UNKNOWN
				}
				return 0, nvml.SUCCESS
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		_, err := GetECCErrors("test-uuid", dev, true)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "(volatile, corrected) failed to get gpu texture memory ecc errors")
	})
}

// TestGetECCErrors_VolatileTextureMemoryUncorrectedNotSupportedBranch tests volatile texture memory uncorrected not supported
func TestGetECCErrors_VolatileTextureMemoryUncorrectedNotSupportedBranch(t *testing.T) {
	mockey.PatchConvey("GetECCErrors volatile texture memory uncorrected not supported branch", t, func() {
		mockDevice := &mock.Device{
			GetTotalEccErrorsFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType) (uint64, nvml.Return) {
				return 0, nvml.SUCCESS
			},
			GetMemoryErrorCounterFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType, location nvml.MemoryLocation) (uint64, nvml.Return) {
				// All aggregate succeed
				if counterType == nvml.AGGREGATE_ECC {
					return 0, nvml.SUCCESS
				}
				// Volatile L1, L2, DRAM, SRAM, device memory succeed
				if location == nvml.MEMORY_LOCATION_L1_CACHE || location == nvml.MEMORY_LOCATION_L2_CACHE ||
					location == nvml.MEMORY_LOCATION_DRAM || location == nvml.MEMORY_LOCATION_SRAM ||
					location == nvml.MEMORY_LOCATION_DEVICE_MEMORY {
					return 0, nvml.SUCCESS
				}
				// Volatile texture memory corrected succeeds
				if location == nvml.MEMORY_LOCATION_TEXTURE_MEMORY && counterType == nvml.VOLATILE_ECC && errorType == nvml.MEMORY_ERROR_TYPE_CORRECTED {
					return 0, nvml.SUCCESS
				}
				// Volatile texture memory uncorrected not supported
				if location == nvml.MEMORY_LOCATION_TEXTURE_MEMORY && counterType == nvml.VOLATILE_ECC && errorType == nvml.MEMORY_ERROR_TYPE_UNCORRECTED {
					return 0, nvml.ERROR_NOT_SUPPORTED
				}
				return 0, nvml.SUCCESS
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		result, err := GetECCErrors("test-uuid", dev, true)

		assert.NoError(t, err)
		assert.False(t, result.Supported)
	})
}

// TestGetECCErrors_VolatileTextureMemoryUncorrectedGPULostBranch tests volatile texture memory uncorrected GPU lost
func TestGetECCErrors_VolatileTextureMemoryUncorrectedGPULostBranch(t *testing.T) {
	mockey.PatchConvey("GetECCErrors volatile texture memory uncorrected GPU lost branch", t, func() {
		mockDevice := &mock.Device{
			GetTotalEccErrorsFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType) (uint64, nvml.Return) {
				return 0, nvml.SUCCESS
			},
			GetMemoryErrorCounterFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType, location nvml.MemoryLocation) (uint64, nvml.Return) {
				// All aggregate succeed
				if counterType == nvml.AGGREGATE_ECC {
					return 0, nvml.SUCCESS
				}
				// Volatile L1, L2, DRAM, SRAM, device memory succeed
				if location == nvml.MEMORY_LOCATION_L1_CACHE || location == nvml.MEMORY_LOCATION_L2_CACHE ||
					location == nvml.MEMORY_LOCATION_DRAM || location == nvml.MEMORY_LOCATION_SRAM ||
					location == nvml.MEMORY_LOCATION_DEVICE_MEMORY {
					return 0, nvml.SUCCESS
				}
				// Volatile texture memory corrected succeeds
				if location == nvml.MEMORY_LOCATION_TEXTURE_MEMORY && counterType == nvml.VOLATILE_ECC && errorType == nvml.MEMORY_ERROR_TYPE_CORRECTED {
					return 0, nvml.SUCCESS
				}
				// Volatile texture memory uncorrected GPU lost
				if location == nvml.MEMORY_LOCATION_TEXTURE_MEMORY && counterType == nvml.VOLATILE_ECC && errorType == nvml.MEMORY_ERROR_TYPE_UNCORRECTED {
					return 0, nvml.ERROR_GPU_IS_LOST
				}
				return 0, nvml.SUCCESS
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		_, err := GetECCErrors("test-uuid", dev, true)

		assert.Error(t, err)
		assert.True(t, errors.Is(err, nvmlerrors.ErrGPULost))
	})
}

// TestGetECCErrors_VolatileTextureMemoryUncorrectedUnknownError tests volatile texture memory uncorrected unknown error
func TestGetECCErrors_VolatileTextureMemoryUncorrectedUnknownError(t *testing.T) {
	mockey.PatchConvey("GetECCErrors volatile texture memory uncorrected unknown error", t, func() {
		mockDevice := &mock.Device{
			GetTotalEccErrorsFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType) (uint64, nvml.Return) {
				return 0, nvml.SUCCESS
			},
			GetMemoryErrorCounterFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType, location nvml.MemoryLocation) (uint64, nvml.Return) {
				// All aggregate succeed
				if counterType == nvml.AGGREGATE_ECC {
					return 0, nvml.SUCCESS
				}
				// Volatile L1, L2, DRAM, SRAM, device memory succeed
				if location == nvml.MEMORY_LOCATION_L1_CACHE || location == nvml.MEMORY_LOCATION_L2_CACHE ||
					location == nvml.MEMORY_LOCATION_DRAM || location == nvml.MEMORY_LOCATION_SRAM ||
					location == nvml.MEMORY_LOCATION_DEVICE_MEMORY {
					return 0, nvml.SUCCESS
				}
				// Volatile texture memory corrected succeeds
				if location == nvml.MEMORY_LOCATION_TEXTURE_MEMORY && counterType == nvml.VOLATILE_ECC && errorType == nvml.MEMORY_ERROR_TYPE_CORRECTED {
					return 0, nvml.SUCCESS
				}
				// Volatile texture memory uncorrected unknown error
				if location == nvml.MEMORY_LOCATION_TEXTURE_MEMORY && counterType == nvml.VOLATILE_ECC && errorType == nvml.MEMORY_ERROR_TYPE_UNCORRECTED {
					return 0, nvml.ERROR_UNKNOWN
				}
				return 0, nvml.SUCCESS
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		_, err := GetECCErrors("test-uuid", dev, true)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "(volatile, uncorrected) failed to get gpu texture memory ecc errors")
	})
}

// TestGetECCErrors_VolatileSharedMemoryCorrectedUnknownError tests volatile shared memory corrected unknown error
func TestGetECCErrors_VolatileSharedMemoryCorrectedUnknownError(t *testing.T) {
	mockey.PatchConvey("GetECCErrors volatile shared memory corrected unknown error", t, func() {
		mockDevice := &mock.Device{
			GetTotalEccErrorsFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType) (uint64, nvml.Return) {
				return 0, nvml.SUCCESS
			},
			GetMemoryErrorCounterFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType, location nvml.MemoryLocation) (uint64, nvml.Return) {
				// All aggregate succeed
				if counterType == nvml.AGGREGATE_ECC {
					return 0, nvml.SUCCESS
				}
				// Volatile L1, L2, DRAM, SRAM, device memory, texture memory succeed
				if location == nvml.MEMORY_LOCATION_L1_CACHE || location == nvml.MEMORY_LOCATION_L2_CACHE ||
					location == nvml.MEMORY_LOCATION_DRAM || location == nvml.MEMORY_LOCATION_SRAM ||
					location == nvml.MEMORY_LOCATION_DEVICE_MEMORY || location == nvml.MEMORY_LOCATION_TEXTURE_MEMORY {
					return 0, nvml.SUCCESS
				}
				// Volatile shared memory corrected unknown error
				if location == nvml.MEMORY_LOCATION_TEXTURE_SHM && counterType == nvml.VOLATILE_ECC && errorType == nvml.MEMORY_ERROR_TYPE_CORRECTED {
					return 0, nvml.ERROR_UNKNOWN
				}
				return 0, nvml.SUCCESS
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		_, err := GetECCErrors("test-uuid", dev, true)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "(volatile, corrected) failed to get shared memory ecc errors")
	})
}

// TestGetECCErrors_VolatileSharedMemoryUncorrectedNotSupportedBranch tests volatile shared memory uncorrected not supported
func TestGetECCErrors_VolatileSharedMemoryUncorrectedNotSupportedBranch(t *testing.T) {
	mockey.PatchConvey("GetECCErrors volatile shared memory uncorrected not supported branch", t, func() {
		mockDevice := &mock.Device{
			GetTotalEccErrorsFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType) (uint64, nvml.Return) {
				return 0, nvml.SUCCESS
			},
			GetMemoryErrorCounterFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType, location nvml.MemoryLocation) (uint64, nvml.Return) {
				// All aggregate succeed
				if counterType == nvml.AGGREGATE_ECC {
					return 0, nvml.SUCCESS
				}
				// Volatile L1, L2, DRAM, SRAM, device memory, texture memory succeed
				if location == nvml.MEMORY_LOCATION_L1_CACHE || location == nvml.MEMORY_LOCATION_L2_CACHE ||
					location == nvml.MEMORY_LOCATION_DRAM || location == nvml.MEMORY_LOCATION_SRAM ||
					location == nvml.MEMORY_LOCATION_DEVICE_MEMORY || location == nvml.MEMORY_LOCATION_TEXTURE_MEMORY {
					return 0, nvml.SUCCESS
				}
				// Volatile shared memory corrected succeeds
				if location == nvml.MEMORY_LOCATION_TEXTURE_SHM && counterType == nvml.VOLATILE_ECC && errorType == nvml.MEMORY_ERROR_TYPE_CORRECTED {
					return 0, nvml.SUCCESS
				}
				// Volatile shared memory uncorrected not supported
				if location == nvml.MEMORY_LOCATION_TEXTURE_SHM && counterType == nvml.VOLATILE_ECC && errorType == nvml.MEMORY_ERROR_TYPE_UNCORRECTED {
					return 0, nvml.ERROR_NOT_SUPPORTED
				}
				return 0, nvml.SUCCESS
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		result, err := GetECCErrors("test-uuid", dev, true)

		assert.NoError(t, err)
		assert.False(t, result.Supported)
	})
}

// TestGetECCErrors_VolatileSharedMemoryUncorrectedGPULostBranch tests volatile shared memory uncorrected GPU lost
func TestGetECCErrors_VolatileSharedMemoryUncorrectedGPULostBranch(t *testing.T) {
	mockey.PatchConvey("GetECCErrors volatile shared memory uncorrected GPU lost branch", t, func() {
		mockDevice := &mock.Device{
			GetTotalEccErrorsFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType) (uint64, nvml.Return) {
				return 0, nvml.SUCCESS
			},
			GetMemoryErrorCounterFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType, location nvml.MemoryLocation) (uint64, nvml.Return) {
				// All aggregate succeed
				if counterType == nvml.AGGREGATE_ECC {
					return 0, nvml.SUCCESS
				}
				// Volatile L1, L2, DRAM, SRAM, device memory, texture memory succeed
				if location == nvml.MEMORY_LOCATION_L1_CACHE || location == nvml.MEMORY_LOCATION_L2_CACHE ||
					location == nvml.MEMORY_LOCATION_DRAM || location == nvml.MEMORY_LOCATION_SRAM ||
					location == nvml.MEMORY_LOCATION_DEVICE_MEMORY || location == nvml.MEMORY_LOCATION_TEXTURE_MEMORY {
					return 0, nvml.SUCCESS
				}
				// Volatile shared memory corrected succeeds
				if location == nvml.MEMORY_LOCATION_TEXTURE_SHM && counterType == nvml.VOLATILE_ECC && errorType == nvml.MEMORY_ERROR_TYPE_CORRECTED {
					return 0, nvml.SUCCESS
				}
				// Volatile shared memory uncorrected GPU lost
				if location == nvml.MEMORY_LOCATION_TEXTURE_SHM && counterType == nvml.VOLATILE_ECC && errorType == nvml.MEMORY_ERROR_TYPE_UNCORRECTED {
					return 0, nvml.ERROR_GPU_IS_LOST
				}
				return 0, nvml.SUCCESS
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		_, err := GetECCErrors("test-uuid", dev, true)

		assert.Error(t, err)
		assert.True(t, errors.Is(err, nvmlerrors.ErrGPULost))
	})
}

// TestGetECCErrors_VolatileSharedMemoryUncorrectedUnknownError tests volatile shared memory uncorrected unknown error
func TestGetECCErrors_VolatileSharedMemoryUncorrectedUnknownError(t *testing.T) {
	mockey.PatchConvey("GetECCErrors volatile shared memory uncorrected unknown error", t, func() {
		mockDevice := &mock.Device{
			GetTotalEccErrorsFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType) (uint64, nvml.Return) {
				return 0, nvml.SUCCESS
			},
			GetMemoryErrorCounterFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType, location nvml.MemoryLocation) (uint64, nvml.Return) {
				// All aggregate succeed
				if counterType == nvml.AGGREGATE_ECC {
					return 0, nvml.SUCCESS
				}
				// Volatile L1, L2, DRAM, SRAM, device memory, texture memory succeed
				if location == nvml.MEMORY_LOCATION_L1_CACHE || location == nvml.MEMORY_LOCATION_L2_CACHE ||
					location == nvml.MEMORY_LOCATION_DRAM || location == nvml.MEMORY_LOCATION_SRAM ||
					location == nvml.MEMORY_LOCATION_DEVICE_MEMORY || location == nvml.MEMORY_LOCATION_TEXTURE_MEMORY {
					return 0, nvml.SUCCESS
				}
				// Volatile shared memory corrected succeeds
				if location == nvml.MEMORY_LOCATION_TEXTURE_SHM && counterType == nvml.VOLATILE_ECC && errorType == nvml.MEMORY_ERROR_TYPE_CORRECTED {
					return 0, nvml.SUCCESS
				}
				// Volatile shared memory uncorrected unknown error
				if location == nvml.MEMORY_LOCATION_TEXTURE_SHM && counterType == nvml.VOLATILE_ECC && errorType == nvml.MEMORY_ERROR_TYPE_UNCORRECTED {
					return 0, nvml.ERROR_UNKNOWN
				}
				return 0, nvml.SUCCESS
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		_, err := GetECCErrors("test-uuid", dev, true)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "(volatile, uncorrected) failed to get shared memory ecc errors")
	})
}

// TestGetECCErrors_VolatileRegisterFileUncorrectedNotSupported tests volatile register file uncorrected not supported
func TestGetECCErrors_VolatileRegisterFileUncorrectedNotSupportedBranch(t *testing.T) {
	mockey.PatchConvey("GetECCErrors volatile register file uncorrected not supported branch", t, func() {
		mockDevice := &mock.Device{
			GetTotalEccErrorsFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType) (uint64, nvml.Return) {
				return 0, nvml.SUCCESS
			},
			GetMemoryErrorCounterFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType, location nvml.MemoryLocation) (uint64, nvml.Return) {
				// All aggregate succeed
				if counterType == nvml.AGGREGATE_ECC {
					return 0, nvml.SUCCESS
				}
				// All other volatile locations succeed
				if location == nvml.MEMORY_LOCATION_L1_CACHE || location == nvml.MEMORY_LOCATION_L2_CACHE ||
					location == nvml.MEMORY_LOCATION_DRAM || location == nvml.MEMORY_LOCATION_SRAM ||
					location == nvml.MEMORY_LOCATION_DEVICE_MEMORY || location == nvml.MEMORY_LOCATION_TEXTURE_MEMORY ||
					location == nvml.MEMORY_LOCATION_TEXTURE_SHM {
					return 0, nvml.SUCCESS
				}
				// Volatile register file corrected succeeds
				if location == nvml.MEMORY_LOCATION_REGISTER_FILE && counterType == nvml.VOLATILE_ECC && errorType == nvml.MEMORY_ERROR_TYPE_CORRECTED {
					return 0, nvml.SUCCESS
				}
				// Volatile register file uncorrected not supported
				if location == nvml.MEMORY_LOCATION_REGISTER_FILE && counterType == nvml.VOLATILE_ECC && errorType == nvml.MEMORY_ERROR_TYPE_UNCORRECTED {
					return 0, nvml.ERROR_NOT_SUPPORTED
				}
				return 0, nvml.SUCCESS
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		result, err := GetECCErrors("test-uuid", dev, true)

		assert.NoError(t, err)
		assert.False(t, result.Supported)
	})
}

// TestGetECCErrors_VolatileRegisterFileUncorrectedGPULostBranch tests volatile register file uncorrected GPU lost
func TestGetECCErrors_VolatileRegisterFileUncorrectedGPULostBranch(t *testing.T) {
	mockey.PatchConvey("GetECCErrors volatile register file uncorrected GPU lost branch", t, func() {
		mockDevice := &mock.Device{
			GetTotalEccErrorsFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType) (uint64, nvml.Return) {
				return 0, nvml.SUCCESS
			},
			GetMemoryErrorCounterFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType, location nvml.MemoryLocation) (uint64, nvml.Return) {
				// All aggregate succeed
				if counterType == nvml.AGGREGATE_ECC {
					return 0, nvml.SUCCESS
				}
				// All other volatile locations succeed
				if location == nvml.MEMORY_LOCATION_L1_CACHE || location == nvml.MEMORY_LOCATION_L2_CACHE ||
					location == nvml.MEMORY_LOCATION_DRAM || location == nvml.MEMORY_LOCATION_SRAM ||
					location == nvml.MEMORY_LOCATION_DEVICE_MEMORY || location == nvml.MEMORY_LOCATION_TEXTURE_MEMORY ||
					location == nvml.MEMORY_LOCATION_TEXTURE_SHM {
					return 0, nvml.SUCCESS
				}
				// Volatile register file corrected succeeds
				if location == nvml.MEMORY_LOCATION_REGISTER_FILE && counterType == nvml.VOLATILE_ECC && errorType == nvml.MEMORY_ERROR_TYPE_CORRECTED {
					return 0, nvml.SUCCESS
				}
				// Volatile register file uncorrected GPU lost
				if location == nvml.MEMORY_LOCATION_REGISTER_FILE && counterType == nvml.VOLATILE_ECC && errorType == nvml.MEMORY_ERROR_TYPE_UNCORRECTED {
					return 0, nvml.ERROR_GPU_IS_LOST
				}
				return 0, nvml.SUCCESS
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		_, err := GetECCErrors("test-uuid", dev, true)

		assert.Error(t, err)
		assert.True(t, errors.Is(err, nvmlerrors.ErrGPULost))
	})
}

// TestGetECCErrors_VolatileRegisterFileUncorrectedUnknownError tests volatile register file uncorrected unknown error
func TestGetECCErrors_VolatileRegisterFileUncorrectedUnknownError(t *testing.T) {
	mockey.PatchConvey("GetECCErrors volatile register file uncorrected unknown error", t, func() {
		mockDevice := &mock.Device{
			GetTotalEccErrorsFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType) (uint64, nvml.Return) {
				return 0, nvml.SUCCESS
			},
			GetMemoryErrorCounterFunc: func(errorType nvml.MemoryErrorType, counterType nvml.EccCounterType, location nvml.MemoryLocation) (uint64, nvml.Return) {
				// All aggregate succeed
				if counterType == nvml.AGGREGATE_ECC {
					return 0, nvml.SUCCESS
				}
				// All other volatile locations succeed
				if location == nvml.MEMORY_LOCATION_L1_CACHE || location == nvml.MEMORY_LOCATION_L2_CACHE ||
					location == nvml.MEMORY_LOCATION_DRAM || location == nvml.MEMORY_LOCATION_SRAM ||
					location == nvml.MEMORY_LOCATION_DEVICE_MEMORY || location == nvml.MEMORY_LOCATION_TEXTURE_MEMORY ||
					location == nvml.MEMORY_LOCATION_TEXTURE_SHM {
					return 0, nvml.SUCCESS
				}
				// Volatile register file corrected succeeds
				if location == nvml.MEMORY_LOCATION_REGISTER_FILE && counterType == nvml.VOLATILE_ECC && errorType == nvml.MEMORY_ERROR_TYPE_CORRECTED {
					return 0, nvml.SUCCESS
				}
				// Volatile register file uncorrected unknown error
				if location == nvml.MEMORY_LOCATION_REGISTER_FILE && counterType == nvml.VOLATILE_ECC && errorType == nvml.MEMORY_ERROR_TYPE_UNCORRECTED {
					return 0, nvml.ERROR_UNKNOWN
				}
				return 0, nvml.SUCCESS
			},
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
		}

		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		_, err := GetECCErrors("test-uuid", dev, true)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "(volatile, uncorrected) failed to get register file ecc errors")
	})
}
