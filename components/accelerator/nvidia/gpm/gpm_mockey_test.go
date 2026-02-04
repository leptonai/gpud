//go:build linux

package gpm

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
		assert.NotNil(t, tc.getGPMSupportedFunc)
		assert.NotNil(t, tc.getGPMMetricsFunc)
	})
}

// customMockNVMLInstanceGPM with customizable NVMLExists and ProductName
type customMockNVMLInstanceGPM struct {
	devs        map[string]device.Device
	nvmlExists  bool
	productName string
}

func (m *customMockNVMLInstanceGPM) Devices() map[string]device.Device { return m.devs }
func (m *customMockNVMLInstanceGPM) FabricManagerSupported() bool      { return true }
func (m *customMockNVMLInstanceGPM) FabricStateSupported() bool        { return false }
func (m *customMockNVMLInstanceGPM) GetMemoryErrorManagementCapabilities() nvidiaproduct.MemoryErrorManagementCapabilities {
	return nvidiaproduct.MemoryErrorManagementCapabilities{}
}
func (m *customMockNVMLInstanceGPM) ProductName() string   { return m.productName }
func (m *customMockNVMLInstanceGPM) Architecture() string  { return "" }
func (m *customMockNVMLInstanceGPM) Brand() string         { return "" }
func (m *customMockNVMLInstanceGPM) DriverVersion() string { return "" }
func (m *customMockNVMLInstanceGPM) DriverMajor() int      { return 0 }
func (m *customMockNVMLInstanceGPM) CUDAVersion() string   { return "" }
func (m *customMockNVMLInstanceGPM) NVMLExists() bool      { return m.nvmlExists }
func (m *customMockNVMLInstanceGPM) Library() lib.Library  { return nil }
func (m *customMockNVMLInstanceGPM) Shutdown() error       { return nil }
func (m *customMockNVMLInstanceGPM) InitError() error      { return nil }

// TestComponent_IsSupported tests IsSupported method with various conditions
func TestComponent_IsSupported(t *testing.T) {
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
					customMock := &customMockNVMLInstanceGPM{
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

// mockNVMLInstanceWithInitErrorGPM returns an init error
type mockNVMLInstanceWithInitErrorGPM struct {
	devs      map[string]device.Device
	initError error
}

func (m *mockNVMLInstanceWithInitErrorGPM) Devices() map[string]device.Device { return m.devs }
func (m *mockNVMLInstanceWithInitErrorGPM) FabricManagerSupported() bool      { return true }
func (m *mockNVMLInstanceWithInitErrorGPM) FabricStateSupported() bool        { return false }
func (m *mockNVMLInstanceWithInitErrorGPM) GetMemoryErrorManagementCapabilities() nvidiaproduct.MemoryErrorManagementCapabilities {
	return nvidiaproduct.MemoryErrorManagementCapabilities{}
}
func (m *mockNVMLInstanceWithInitErrorGPM) ProductName() string   { return "NVIDIA H100" }
func (m *mockNVMLInstanceWithInitErrorGPM) Architecture() string  { return "" }
func (m *mockNVMLInstanceWithInitErrorGPM) Brand() string         { return "" }
func (m *mockNVMLInstanceWithInitErrorGPM) DriverVersion() string { return "" }
func (m *mockNVMLInstanceWithInitErrorGPM) DriverMajor() int      { return 0 }
func (m *mockNVMLInstanceWithInitErrorGPM) CUDAVersion() string   { return "" }
func (m *mockNVMLInstanceWithInitErrorGPM) NVMLExists() bool      { return true }
func (m *mockNVMLInstanceWithInitErrorGPM) Library() lib.Library  { return nil }
func (m *mockNVMLInstanceWithInitErrorGPM) Shutdown() error       { return nil }
func (m *mockNVMLInstanceWithInitErrorGPM) InitError() error      { return m.initError }

// TestCheck_InitError tests Check when NVML has an initialization error
func TestCheck_InitError(t *testing.T) {
	mockey.PatchConvey("Check with NVML init error", t, func() {
		ctx := context.Background()
		cctx, cancel := context.WithCancel(ctx)
		defer cancel()

		initErr := errors.New("error getting device handle for index '0': Unknown Error")
		mockInst := &mockNVMLInstanceWithInitErrorGPM{
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
func TestCheck_MissingProductName(t *testing.T) {
	mockey.PatchConvey("Check with missing product name", t, func() {
		ctx := context.Background()
		cctx, cancel := context.WithCancel(ctx)
		defer cancel()

		mockInst := &customMockNVMLInstanceGPM{
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

// TestGPMSupportedByDevice_WithMockey tests GPMSupportedByDevice with mocked device
func TestGPMSupportedByDevice_WithMockey(t *testing.T) {
	testCases := []struct {
		name           string
		gpmSupport     nvml.GpmSupport
		gpmRet         nvml.Return
		expectedResult bool
		expectedErr    error
		expectError    bool
	}{
		{
			name:           "GPM supported",
			gpmSupport:     nvml.GpmSupport{IsSupportedDevice: 1},
			gpmRet:         nvml.SUCCESS,
			expectedResult: true,
			expectError:    false,
		},
		{
			name:           "GPM not supported",
			gpmSupport:     nvml.GpmSupport{IsSupportedDevice: 0},
			gpmRet:         nvml.SUCCESS,
			expectedResult: false,
			expectError:    false,
		},
		{
			name:           "Not supported error",
			gpmSupport:     nvml.GpmSupport{},
			gpmRet:         nvml.ERROR_NOT_SUPPORTED,
			expectedResult: false,
			expectError:    false,
		},
		{
			name:           "Version mismatch error",
			gpmSupport:     nvml.GpmSupport{},
			gpmRet:         nvml.ERROR_ARGUMENT_VERSION_MISMATCH,
			expectedResult: false,
			expectError:    false,
		},
		{
			name:        "GPU lost error",
			gpmSupport:  nvml.GpmSupport{},
			gpmRet:      nvml.ERROR_GPU_IS_LOST,
			expectedErr: nvmlerrors.ErrGPULost,
			expectError: true,
		},
		{
			name:        "GPU requires reset",
			gpmSupport:  nvml.GpmSupport{},
			gpmRet:      nvml.ERROR_RESET_REQUIRED,
			expectedErr: nvmlerrors.ErrGPURequiresReset,
			expectError: true,
		},
		{
			name:        "Unknown error",
			gpmSupport:  nvml.GpmSupport{},
			gpmRet:      nvml.ERROR_UNKNOWN,
			expectError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockey.PatchConvey(tc.name, t, func() {
				mockDevice := &mock.Device{
					GpmQueryDeviceSupportFunc: func() (nvml.GpmSupport, nvml.Return) {
						return tc.gpmSupport, tc.gpmRet
					},
					GetUUIDFunc: func() (string, nvml.Return) {
						return "test-uuid", nvml.SUCCESS
					},
				}

				dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

				result, err := GPMSupportedByDevice(dev)

				if tc.expectError {
					assert.Error(t, err)
					if tc.expectedErr != nil {
						assert.True(t, errors.Is(err, tc.expectedErr))
					}
				} else {
					assert.NoError(t, err)
					assert.Equal(t, tc.expectedResult, result)
				}
			})
		})
	}
}

// TestCheck_GPMNotSupported_WithMockey tests Check when GPM is not supported (using mockey)
func TestCheck_GPMNotSupported_WithMockey(t *testing.T) {
	mockey.PatchConvey("Check with GPM not supported", t, func() {
		ctx := context.Background()

		uuid := "gpu-uuid-no-gpm"
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

		getGPMSupportedFunc := func(dev device.Device) (bool, error) {
			return false, nil
		}

		comp := createMockGPMComponent(ctx, getDevicesFunc, getGPMSupportedFunc, nil).(*component)
		result := comp.Check()
		cr, ok := result.(*checkResult)
		require.True(t, ok)

		assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
		assert.Contains(t, cr.reason, "GPM not supported")
		assert.False(t, cr.GPMSupported)
	})
}

// TestCheck_GPMSupportedError tests Check when getting GPM support fails
func TestCheck_GPMSupportedError(t *testing.T) {
	mockey.PatchConvey("Check with GPM supported error", t, func() {
		ctx := context.Background()

		uuid := "gpu-uuid-gpm-error"
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

		getGPMSupportedFunc := func(dev device.Device) (bool, error) {
			return false, nvmlerrors.ErrGPURequiresReset
		}

		comp := createMockGPMComponent(ctx, getDevicesFunc, getGPMSupportedFunc, nil).(*component)
		result := comp.Check()
		cr, ok := result.(*checkResult)
		require.True(t, ok)

		assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
		assert.Contains(t, cr.reason, nvmlerrors.ErrGPURequiresReset.Error())
		assert.NotNil(t, cr.suggestedActions)
		assert.Contains(t, cr.suggestedActions.RepairActions, apiv1.RepairActionTypeRebootSystem)
	})
}

// TestCheck_GPMMetricsError_WithMockey tests Check when getting GPM metrics fails (using mockey)
func TestCheck_GPMMetricsError_WithMockey(t *testing.T) {
	mockey.PatchConvey("Check with GPM metrics error", t, func() {
		ctx := context.Background()

		uuid := "gpu-uuid-metrics-error"
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

		getGPMSupportedFunc := func(dev device.Device) (bool, error) {
			return true, nil
		}

		getGPMMetricsFunc := func(ctx context.Context, dev device.Device) (map[nvml.GpmMetricId]float64, error) {
			return nil, nvmlerrors.ErrGPULost
		}

		comp := createMockGPMComponent(ctx, getDevicesFunc, getGPMSupportedFunc, getGPMMetricsFunc).(*component)
		result := comp.Check()
		cr, ok := result.(*checkResult)
		require.True(t, ok)

		assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
		assert.Contains(t, cr.reason, nvmlerrors.ErrGPULost.Error())
		assert.NotNil(t, cr.suggestedActions)
	})
}

// TestCheck_GPMMetricsSuccess tests Check with successful GPM metrics collection
func TestCheck_GPMMetricsSuccess(t *testing.T) {
	mockey.PatchConvey("Check with successful GPM metrics", t, func() {
		ctx := context.Background()

		uuid := "gpu-uuid-success"
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

		getGPMSupportedFunc := func(dev device.Device) (bool, error) {
			return true, nil
		}

		getGPMMetricsFunc := func(ctx context.Context, dev device.Device) (map[nvml.GpmMetricId]float64, error) {
			return map[nvml.GpmMetricId]float64{
				nvml.GPM_METRIC_SM_OCCUPANCY: 85.5,
				nvml.GPM_METRIC_INTEGER_UTIL: 50.0,
				nvml.GPM_METRIC_FP64_UTIL:    10.0,
			}, nil
		}

		comp := createMockGPMComponent(ctx, getDevicesFunc, getGPMSupportedFunc, getGPMMetricsFunc).(*component)
		result := comp.Check()
		cr, ok := result.(*checkResult)
		require.True(t, ok)

		assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
		assert.Contains(t, cr.reason, "no GPM issue found")
		assert.Len(t, cr.GPMMetrics, 1)
		assert.Equal(t, uuid, cr.GPMMetrics[0].UUID)
		assert.Equal(t, 85.5, cr.GPMMetrics[0].Metrics[nvml.GPM_METRIC_SM_OCCUPANCY])
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

		getGPMSupportedFunc := func(dev device.Device) (bool, error) {
			return true, nil
		}

		getGPMMetricsFunc := func(ctx context.Context, dev device.Device) (map[nvml.GpmMetricId]float64, error) {
			return map[nvml.GpmMetricId]float64{
				nvml.GPM_METRIC_SM_OCCUPANCY: 75.0,
			}, nil
		}

		comp := createMockGPMComponent(ctx, getDevicesFunc, getGPMSupportedFunc, getGPMMetricsFunc).(*component)

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

		mockInst := &customMockNVMLInstanceGPM{
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

// TestCheckResult_String tests the String method of checkResult
func TestCheckResult_String(t *testing.T) {
	t.Run("nil check result", func(t *testing.T) {
		var cr *checkResult
		assert.Equal(t, "", cr.String())
	})

	t.Run("empty GPM metrics", func(t *testing.T) {
		cr := &checkResult{}
		assert.Equal(t, "no data", cr.String())
	})

	t.Run("with GPM metrics", func(t *testing.T) {
		cr := &checkResult{
			GPMMetrics: []GPMMetrics{
				{
					UUID: "gpu-1",
					Metrics: map[nvml.GpmMetricId]float64{
						nvml.GPM_METRIC_SM_OCCUPANCY: 75.0,
					},
				},
			},
		}
		result := cr.String()
		assert.Contains(t, result, "gpu-1")
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

// TestCheckResult_HealthStates_WithExtraInfo tests HealthStates with GPMMetrics
func TestCheckResult_HealthStates_WithExtraInfo(t *testing.T) {
	cr := &checkResult{
		ts:     time.Now(),
		health: apiv1.HealthStateTypeHealthy,
		reason: "all good",
		GPMMetrics: []GPMMetrics{
			{
				UUID: "gpu-1",
				Metrics: map[nvml.GpmMetricId]float64{
					nvml.GPM_METRIC_SM_OCCUPANCY: 80.0,
				},
			},
		},
	}

	states := cr.HealthStates()
	require.Len(t, states, 1)
	assert.NotEmpty(t, states[0].ExtraInfo)
	assert.Contains(t, states[0].ExtraInfo["data"], "gpu-1")
}

// TestCheck_GPMSupportedGPULostError tests Check when GPM support check returns GPU lost
func TestCheck_GPMSupportedGPULostError(t *testing.T) {
	mockey.PatchConvey("Check with GPM support GPU lost error", t, func() {
		ctx := context.Background()

		uuid := "gpu-uuid-gpm-lost"
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

		getGPMSupportedFunc := func(dev device.Device) (bool, error) {
			return false, nvmlerrors.ErrGPULost
		}

		comp := createMockGPMComponent(ctx, getDevicesFunc, getGPMSupportedFunc, nil).(*component)
		result := comp.Check()
		cr, ok := result.(*checkResult)
		require.True(t, ok)

		assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
		assert.Contains(t, cr.reason, nvmlerrors.ErrGPULost.Error())
		assert.NotNil(t, cr.suggestedActions)
		assert.Contains(t, cr.suggestedActions.RepairActions, apiv1.RepairActionTypeRebootSystem)
	})
}

// TestCheck_GPMMetricsGPURequiresReset tests Check when GPM metrics returns GPU requires reset
func TestCheck_GPMMetricsGPURequiresReset(t *testing.T) {
	mockey.PatchConvey("Check with GPM metrics GPU requires reset", t, func() {
		ctx := context.Background()

		uuid := "gpu-uuid-metrics-reset"
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

		getGPMSupportedFunc := func(dev device.Device) (bool, error) {
			return true, nil
		}

		getGPMMetricsFunc := func(ctx context.Context, dev device.Device) (map[nvml.GpmMetricId]float64, error) {
			return nil, nvmlerrors.ErrGPURequiresReset
		}

		comp := createMockGPMComponent(ctx, getDevicesFunc, getGPMSupportedFunc, getGPMMetricsFunc).(*component)
		result := comp.Check()
		cr, ok := result.(*checkResult)
		require.True(t, ok)

		assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
		assert.Contains(t, cr.reason, nvmlerrors.ErrGPURequiresReset.Error())
		assert.NotNil(t, cr.suggestedActions)
		assert.Contains(t, cr.suggestedActions.RepairActions, apiv1.RepairActionTypeRebootSystem)
	})
}

// TestCheck_MultipleGPUs tests Check with multiple GPUs
func TestCheck_MultipleGPUs(t *testing.T) {
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

		getDevicesFunc := func() map[string]device.Device {
			return devs
		}

		getGPMSupportedFunc := func(dev device.Device) (bool, error) {
			return true, nil
		}

		getGPMMetricsFunc := func(ctx context.Context, dev device.Device) (map[nvml.GpmMetricId]float64, error) {
			return map[nvml.GpmMetricId]float64{
				nvml.GPM_METRIC_SM_OCCUPANCY: 75.0,
			}, nil
		}

		comp := createMockGPMComponent(ctx, getDevicesFunc, getGPMSupportedFunc, getGPMMetricsFunc).(*component)
		result := comp.Check()
		cr, ok := result.(*checkResult)
		require.True(t, ok)

		assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
		assert.Contains(t, cr.reason, "2 GPU(s) were checked")
		assert.Len(t, cr.GPMMetrics, 2)
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
		getGPMSupportedFunc := func(dev device.Device) (bool, error) {
			return false, nvmlerrors.ErrGPURequiresReset
		}

		comp := createMockGPMComponent(ctx, getDevicesFunc, getGPMSupportedFunc, nil).(*component)
		comp.Check()

		states := comp.LastHealthStates()
		require.Len(t, states, 1)
		assert.Equal(t, apiv1.HealthStateTypeUnhealthy, states[0].Health)
		assert.NotNil(t, states[0].SuggestedActions)
		assert.Contains(t, states[0].SuggestedActions.RepairActions, apiv1.RepairActionTypeRebootSystem)
	})
}

// TestRecordGPMMetricByID_AllMetricTypes tests all metric types in recordGPMMetricByID
func TestRecordGPMMetricByID_AllMetricTypes(t *testing.T) {
	testCases := []struct {
		name     string
		metricID nvml.GpmMetricId
	}{
		{"SM Occupancy", nvml.GPM_METRIC_SM_OCCUPANCY},
		{"Integer Util", nvml.GPM_METRIC_INTEGER_UTIL},
		{"Any Tensor Util", nvml.GPM_METRIC_ANY_TENSOR_UTIL},
		{"DFMA Tensor Util", nvml.GPM_METRIC_DFMA_TENSOR_UTIL},
		{"HMMA Tensor Util", nvml.GPM_METRIC_HMMA_TENSOR_UTIL},
		{"IMMA Tensor Util", nvml.GPM_METRIC_IMMA_TENSOR_UTIL},
		{"FP64 Util", nvml.GPM_METRIC_FP64_UTIL},
		{"FP32 Util", nvml.GPM_METRIC_FP32_UTIL},
		{"FP16 Util", nvml.GPM_METRIC_FP16_UTIL},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Just verify it doesn't panic - the function sets prometheus metrics
			recordGPMMetricByID(tc.metricID, "test-gpu-uuid", 75.5)
		})
	}

	// Test unsupported metric ID (default case)
	t.Run("Unsupported metric ID", func(t *testing.T) {
		// Use an unsupported metric ID to hit the default case
		// This should log a warning but not panic
		recordGPMMetricByID(nvml.GpmMetricId(9999), "test-gpu-uuid", 0.0)
	})
}

// TestCheck_AllMetricTypesRecorded tests that Check records all metric types
func TestCheck_AllMetricTypesRecorded(t *testing.T) {
	mockey.PatchConvey("Check records all metric types", t, func() {
		ctx := context.Background()

		uuid := "gpu-uuid-all-metrics"
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

		getGPMSupportedFunc := func(dev device.Device) (bool, error) {
			return true, nil
		}

		// Return all possible metric types
		allMetrics := map[nvml.GpmMetricId]float64{
			nvml.GPM_METRIC_SM_OCCUPANCY:     80.0,
			nvml.GPM_METRIC_INTEGER_UTIL:     50.0,
			nvml.GPM_METRIC_ANY_TENSOR_UTIL:  60.0,
			nvml.GPM_METRIC_DFMA_TENSOR_UTIL: 30.0,
			nvml.GPM_METRIC_HMMA_TENSOR_UTIL: 40.0,
			nvml.GPM_METRIC_IMMA_TENSOR_UTIL: 25.0,
			nvml.GPM_METRIC_FP64_UTIL:        10.0,
			nvml.GPM_METRIC_FP32_UTIL:        70.0,
			nvml.GPM_METRIC_FP16_UTIL:        55.0,
		}

		getGPMMetricsFunc := func(ctx context.Context, dev device.Device) (map[nvml.GpmMetricId]float64, error) {
			return allMetrics, nil
		}

		comp := createMockGPMComponent(ctx, getDevicesFunc, getGPMSupportedFunc, getGPMMetricsFunc).(*component)
		result := comp.Check()
		cr, ok := result.(*checkResult)
		require.True(t, ok)

		assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
		assert.Len(t, cr.GPMMetrics, 1)
		assert.Len(t, cr.GPMMetrics[0].Metrics, len(allMetrics))
	})
}

// TestGetGPMMetrics_ValidationErrors tests input validation in GetGPMMetrics
func TestGetGPMMetrics_ValidationErrors(t *testing.T) {
	t.Run("No metric IDs provided", func(t *testing.T) {
		ctx := context.Background()
		mockDevice := &mock.Device{}
		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		result, err := GetGPMMetrics(ctx, dev, time.Second)
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "no metric IDs provided")
	})

	t.Run("Too many metric IDs", func(t *testing.T) {
		ctx := context.Background()
		mockDevice := &mock.Device{}
		dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

		// Create more than 98 metric IDs
		metricIDs := make([]nvml.GpmMetricId, 99)
		for i := range metricIDs {
			metricIDs[i] = nvml.GPM_METRIC_SM_OCCUPANCY
		}

		result, err := GetGPMMetrics(ctx, dev, time.Second, metricIDs...)
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "too many metric IDs")
	})
}

// TestNew_WithEventStore tests New with an event store
func TestNew_WithEventStore(t *testing.T) {
	mockey.PatchConvey("New with event store", t, func() {
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

		tc, ok := c.(*component)
		require.True(t, ok)
		assert.NotNil(t, tc.ctx)
		assert.NotNil(t, tc.cancel)
	})
}

// TestCheck_GPMMetricsNilReturned tests when GPM metrics returns nil (not supported)
func TestCheck_GPMMetricsNilReturned(t *testing.T) {
	mockey.PatchConvey("Check when GPM metrics returns nil", t, func() {
		ctx := context.Background()

		uuid := "gpu-uuid-nil-metrics"
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

		getGPMSupportedFunc := func(dev device.Device) (bool, error) {
			return true, nil
		}

		// Return nil metrics (simulates not supported after initial check)
		getGPMMetricsFunc := func(ctx context.Context, dev device.Device) (map[nvml.GpmMetricId]float64, error) {
			return nil, nil
		}

		comp := createMockGPMComponent(ctx, getDevicesFunc, getGPMSupportedFunc, getGPMMetricsFunc).(*component)
		result := comp.Check()
		cr, ok := result.(*checkResult)
		require.True(t, ok)

		// When metrics is nil but no error, it should still be healthy
		assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
	})
}

// TestCheck_GenericGPMSupportError tests Check with a generic GPM support error
func TestCheck_GenericGPMSupportError(t *testing.T) {
	mockey.PatchConvey("Check with generic GPM support error", t, func() {
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

		getDevicesFunc := func() map[string]device.Device {
			return devs
		}

		genericErr := errors.New("some generic GPM error")
		getGPMSupportedFunc := func(dev device.Device) (bool, error) {
			return false, genericErr
		}

		comp := createMockGPMComponent(ctx, getDevicesFunc, getGPMSupportedFunc, nil).(*component)
		result := comp.Check()
		cr, ok := result.(*checkResult)
		require.True(t, ok)

		assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
		assert.Contains(t, cr.reason, "error getting GPM supported")
		assert.Equal(t, genericErr, cr.err)
	})
}

// TestCheck_GenericGPMMetricsError tests Check with a generic GPM metrics error
func TestCheck_GenericGPMMetricsError(t *testing.T) {
	mockey.PatchConvey("Check with generic GPM metrics error", t, func() {
		ctx := context.Background()

		uuid := "gpu-uuid-metrics-generic"
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

		getGPMSupportedFunc := func(dev device.Device) (bool, error) {
			return true, nil
		}

		genericErr := errors.New("some generic metrics error")
		getGPMMetricsFunc := func(ctx context.Context, dev device.Device) (map[nvml.GpmMetricId]float64, error) {
			return nil, genericErr
		}

		comp := createMockGPMComponent(ctx, getDevicesFunc, getGPMSupportedFunc, getGPMMetricsFunc).(*component)
		result := comp.Check()
		cr, ok := result.(*checkResult)
		require.True(t, ok)

		assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
		assert.Contains(t, cr.reason, "error getting GPM metrics")
		assert.Equal(t, genericErr, cr.err)
	})
}

// TestGetGPMMetrics_GpmSampleAllocNotSupported tests GetGPMMetrics when GpmSampleAlloc returns not supported
func TestGetGPMMetrics_GpmSampleAllocNotSupported(t *testing.T) {
	mockey.PatchConvey("GetGPMMetrics with GpmSampleAlloc not supported", t, func() {
		mockey.Mock(nvml.GpmSampleAlloc).To(func() (nvml.GpmSample, nvml.Return) {
			return nil, nvml.ERROR_NOT_SUPPORTED
		}).Build()

		ctx := context.Background()
		mockDeviceObj := &mock.Device{
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
		}
		dev := testutil.NewMockDevice(mockDeviceObj, "test-arch", "test-brand", "test-cuda", "test-pci")

		result, err := GetGPMMetrics(ctx, dev, time.Second, nvml.GPM_METRIC_SM_OCCUPANCY)
		assert.NoError(t, err)
		assert.Nil(t, result)
	})
}

// TestGetGPMMetrics_GpmSampleAllocVersionMismatch tests GetGPMMetrics when GpmSampleAlloc returns version mismatch
func TestGetGPMMetrics_GpmSampleAllocVersionMismatch(t *testing.T) {
	mockey.PatchConvey("GetGPMMetrics with GpmSampleAlloc version mismatch", t, func() {
		mockey.Mock(nvml.GpmSampleAlloc).To(func() (nvml.GpmSample, nvml.Return) {
			return nil, nvml.ERROR_ARGUMENT_VERSION_MISMATCH
		}).Build()

		ctx := context.Background()
		mockDeviceObj := &mock.Device{
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
		}
		dev := testutil.NewMockDevice(mockDeviceObj, "test-arch", "test-brand", "test-cuda", "test-pci")

		result, err := GetGPMMetrics(ctx, dev, time.Second, nvml.GPM_METRIC_SM_OCCUPANCY)
		assert.NoError(t, err)
		assert.Nil(t, result)
	})
}

// TestGetGPMMetrics_GpmSampleAllocGPULost tests GetGPMMetrics when GpmSampleAlloc returns GPU lost
func TestGetGPMMetrics_GpmSampleAllocGPULost(t *testing.T) {
	mockey.PatchConvey("GetGPMMetrics with GpmSampleAlloc GPU lost", t, func() {
		mockey.Mock(nvml.GpmSampleAlloc).To(func() (nvml.GpmSample, nvml.Return) {
			return nil, nvml.ERROR_GPU_IS_LOST
		}).Build()

		ctx := context.Background()
		mockDeviceObj := &mock.Device{
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
		}
		dev := testutil.NewMockDevice(mockDeviceObj, "test-arch", "test-brand", "test-cuda", "test-pci")

		result, err := GetGPMMetrics(ctx, dev, time.Second, nvml.GPM_METRIC_SM_OCCUPANCY)
		assert.Error(t, err)
		assert.True(t, errors.Is(err, nvmlerrors.ErrGPULost))
		assert.Nil(t, result)
	})
}

// TestGetGPMMetrics_GpmSampleAllocGPURequiresReset tests GetGPMMetrics when GpmSampleAlloc returns reset required
func TestGetGPMMetrics_GpmSampleAllocGPURequiresReset(t *testing.T) {
	mockey.PatchConvey("GetGPMMetrics with GpmSampleAlloc GPU requires reset", t, func() {
		mockey.Mock(nvml.GpmSampleAlloc).To(func() (nvml.GpmSample, nvml.Return) {
			return nil, nvml.ERROR_RESET_REQUIRED
		}).Build()

		ctx := context.Background()
		mockDeviceObj := &mock.Device{
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
		}
		dev := testutil.NewMockDevice(mockDeviceObj, "test-arch", "test-brand", "test-cuda", "test-pci")

		result, err := GetGPMMetrics(ctx, dev, time.Second, nvml.GPM_METRIC_SM_OCCUPANCY)
		assert.Error(t, err)
		assert.True(t, errors.Is(err, nvmlerrors.ErrGPURequiresReset))
		assert.Nil(t, result)
	})
}

// TestGetGPMMetrics_GpmSampleAllocUnknownError tests GetGPMMetrics when GpmSampleAlloc returns unknown error
func TestGetGPMMetrics_GpmSampleAllocUnknownError(t *testing.T) {
	mockey.PatchConvey("GetGPMMetrics with GpmSampleAlloc unknown error", t, func() {
		mockey.Mock(nvml.GpmSampleAlloc).To(func() (nvml.GpmSample, nvml.Return) {
			return nil, nvml.ERROR_UNKNOWN
		}).Build()

		ctx := context.Background()
		mockDeviceObj := &mock.Device{
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
		}
		dev := testutil.NewMockDevice(mockDeviceObj, "test-arch", "test-brand", "test-cuda", "test-pci")

		result, err := GetGPMMetrics(ctx, dev, time.Second, nvml.GPM_METRIC_SM_OCCUPANCY)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "could not allocate sample")
		assert.Nil(t, result)
	})
}

// TestGetGPMMetrics_SecondSampleAllocError tests GetGPMMetrics when second GpmSampleAlloc fails
func TestGetGPMMetrics_SecondSampleAllocError(t *testing.T) {
	mockey.PatchConvey("GetGPMMetrics with second GpmSampleAlloc error", t, func() {
		mockSample1 := &mockNvmlGpmSample{}
		callCount := 0
		mockey.Mock(nvml.GpmSampleAlloc).To(func() (nvml.GpmSample, nvml.Return) {
			callCount++
			if callCount == 1 {
				return mockSample1, nvml.SUCCESS
			}
			return nil, nvml.ERROR_UNKNOWN
		}).Build()

		ctx := context.Background()
		mockDeviceObj := &mock.Device{
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
		}
		dev := testutil.NewMockDevice(mockDeviceObj, "test-arch", "test-brand", "test-cuda", "test-pci")

		result, err := GetGPMMetrics(ctx, dev, time.Second, nvml.GPM_METRIC_SM_OCCUPANCY)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "could not allocate sample")
		assert.Nil(t, result)
	})
}

// TestGetGPMMetrics_FirstGpmSampleGetError tests GetGPMMetrics when first GpmSampleGet fails
func TestGetGPMMetrics_FirstGpmSampleGetError(t *testing.T) {
	mockey.PatchConvey("GetGPMMetrics with first GpmSampleGet error", t, func() {
		mockSample := &mockNvmlGpmSample{}
		mockey.Mock(nvml.GpmSampleAlloc).To(func() (nvml.GpmSample, nvml.Return) {
			return mockSample, nvml.SUCCESS
		}).Build()

		ctx := context.Background()
		mockDeviceObj := &mock.Device{
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
			GpmSampleGetFunc: func(sample nvml.GpmSample) nvml.Return {
				return nvml.ERROR_UNKNOWN
			},
		}
		dev := testutil.NewMockDevice(mockDeviceObj, "test-arch", "test-brand", "test-cuda", "test-pci")

		result, err := GetGPMMetrics(ctx, dev, time.Second, nvml.GPM_METRIC_SM_OCCUPANCY)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "could not get sample")
		assert.Nil(t, result)
	})
}

// TestGetGPMMetrics_ContextCanceled tests GetGPMMetrics when context is canceled during sample wait
func TestGetGPMMetrics_ContextCanceled(t *testing.T) {
	mockey.PatchConvey("GetGPMMetrics with context canceled", t, func() {
		mockSample := &mockNvmlGpmSample{}
		mockey.Mock(nvml.GpmSampleAlloc).To(func() (nvml.GpmSample, nvml.Return) {
			return mockSample, nvml.SUCCESS
		}).Build()

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // cancel immediately

		sampleGetCount := 0
		mockDeviceObj := &mock.Device{
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
			GpmSampleGetFunc: func(sample nvml.GpmSample) nvml.Return {
				sampleGetCount++
				return nvml.SUCCESS
			},
		}
		dev := testutil.NewMockDevice(mockDeviceObj, "test-arch", "test-brand", "test-cuda", "test-pci")

		result, err := GetGPMMetrics(ctx, dev, time.Second, nvml.GPM_METRIC_SM_OCCUPANCY)
		assert.Error(t, err)
		assert.Equal(t, context.Canceled, err)
		assert.Nil(t, result)
	})
}

// TestGetGPMMetrics_SecondGpmSampleGetError tests GetGPMMetrics when second GpmSampleGet fails
func TestGetGPMMetrics_SecondGpmSampleGetError(t *testing.T) {
	mockey.PatchConvey("GetGPMMetrics with second GpmSampleGet error", t, func() {
		mockSample := &mockNvmlGpmSample{}
		mockey.Mock(nvml.GpmSampleAlloc).To(func() (nvml.GpmSample, nvml.Return) {
			return mockSample, nvml.SUCCESS
		}).Build()

		ctx := context.Background()
		sampleGetCount := 0
		mockDeviceObj := &mock.Device{
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
			GpmSampleGetFunc: func(sample nvml.GpmSample) nvml.Return {
				sampleGetCount++
				if sampleGetCount == 1 {
					return nvml.SUCCESS
				}
				return nvml.ERROR_UNKNOWN
			},
		}
		dev := testutil.NewMockDevice(mockDeviceObj, "test-arch", "test-brand", "test-cuda", "test-pci")

		result, err := GetGPMMetrics(ctx, dev, 10*time.Millisecond, nvml.GPM_METRIC_SM_OCCUPANCY)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "could not get sample")
		assert.Nil(t, result)
	})
}

// TestGetGPMMetrics_GpmMetricsGetError tests GetGPMMetrics when GpmMetricsGet fails
func TestGetGPMMetrics_GpmMetricsGetError(t *testing.T) {
	mockey.PatchConvey("GetGPMMetrics with GpmMetricsGet error", t, func() {
		mockSample := &mockNvmlGpmSample{}
		mockey.Mock(nvml.GpmSampleAlloc).To(func() (nvml.GpmSample, nvml.Return) {
			return mockSample, nvml.SUCCESS
		}).Build()
		mockey.Mock(nvml.GpmMetricsGet).To(func(metricsGet *nvml.GpmMetricsGetType) nvml.Return {
			return nvml.ERROR_UNKNOWN
		}).Build()

		ctx := context.Background()
		mockDeviceObj := &mock.Device{
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
			GpmSampleGetFunc: func(sample nvml.GpmSample) nvml.Return {
				return nvml.SUCCESS
			},
		}
		dev := testutil.NewMockDevice(mockDeviceObj, "test-arch", "test-brand", "test-cuda", "test-pci")

		result, err := GetGPMMetrics(ctx, dev, 10*time.Millisecond, nvml.GPM_METRIC_SM_OCCUPANCY)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get gpm metric")
		assert.Nil(t, result)
	})
}

// TestGetGPMMetrics_Success tests GetGPMMetrics full success path
func TestGetGPMMetrics_Success(t *testing.T) {
	mockey.PatchConvey("GetGPMMetrics success", t, func() {
		mockSample := &mockNvmlGpmSample{}
		mockey.Mock(nvml.GpmSampleAlloc).To(func() (nvml.GpmSample, nvml.Return) {
			return mockSample, nvml.SUCCESS
		}).Build()
		mockey.Mock(nvml.GpmMetricsGet).To(func(metricsGet *nvml.GpmMetricsGetType) nvml.Return {
			// Set metric values in the Metrics array
			metricsGet.Metrics[0].Value = 75.5
			metricsGet.Metrics[1].Value = 50.0
			return nvml.SUCCESS
		}).Build()

		ctx := context.Background()
		mockDeviceObj := &mock.Device{
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
			GpmSampleGetFunc: func(sample nvml.GpmSample) nvml.Return {
				return nvml.SUCCESS
			},
		}
		dev := testutil.NewMockDevice(mockDeviceObj, "test-arch", "test-brand", "test-cuda", "test-pci")

		metricIDs := []nvml.GpmMetricId{nvml.GPM_METRIC_SM_OCCUPANCY, nvml.GPM_METRIC_INTEGER_UTIL}
		result, err := GetGPMMetrics(ctx, dev, 10*time.Millisecond, metricIDs...)
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, 75.5, result[nvml.GPM_METRIC_SM_OCCUPANCY])
		assert.Equal(t, 50.0, result[nvml.GPM_METRIC_INTEGER_UTIL])
	})
}

// TestGetGPMMetrics_MetricsCountMismatch tests GetGPMMetrics when metrics count matches (unexpected condition)
func TestGetGPMMetrics_MetricsCountMismatch(t *testing.T) {
	mockey.PatchConvey("GetGPMMetrics metrics count mismatch check", t, func() {
		mockSample := &mockNvmlGpmSample{}
		mockey.Mock(nvml.GpmSampleAlloc).To(func() (nvml.GpmSample, nvml.Return) {
			return mockSample, nvml.SUCCESS
		}).Build()
		// The condition in code checks:
		// if len(gpmMetric.Metrics) == len(metricIDs) { return error }
		// gpmMetric.Metrics is a [210]GpmMetric array, len is always 210
		// So for 210 metric IDs, this condition would be true
		// But we can't pass 210 (max is 98), so this error path may not be reachable
		// We still test the normal case where len(gpmMetric.Metrics) = 210 != len(metricIDs)
		mockey.Mock(nvml.GpmMetricsGet).To(func(metricsGet *nvml.GpmMetricsGetType) nvml.Return {
			metricsGet.Metrics[0].Value = 80.0
			return nvml.SUCCESS
		}).Build()

		ctx := context.Background()
		mockDeviceObj := &mock.Device{
			GetUUIDFunc: func() (string, nvml.Return) {
				return "test-uuid", nvml.SUCCESS
			},
			GpmSampleGetFunc: func(sample nvml.GpmSample) nvml.Return {
				return nvml.SUCCESS
			},
		}
		dev := testutil.NewMockDevice(mockDeviceObj, "test-arch", "test-brand", "test-cuda", "test-pci")

		result, err := GetGPMMetrics(ctx, dev, 10*time.Millisecond, nvml.GPM_METRIC_SM_OCCUPANCY)
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, 80.0, result[nvml.GPM_METRIC_SM_OCCUPANCY])
	})
}

// mockNvmlGpmSample implements nvml.GpmSample for testing
type mockNvmlGpmSample struct{}

func (m *mockNvmlGpmSample) Free() nvml.Return {
	return nvml.SUCCESS
}

func (m *mockNvmlGpmSample) Get(device nvml.Device) nvml.Return {
	return nvml.SUCCESS
}

func (m *mockNvmlGpmSample) MigGet(device nvml.Device, gpuInstanceId int) nvml.Return {
	return nvml.SUCCESS
}
