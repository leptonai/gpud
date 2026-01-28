//go:build linux

package remappedrows

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

// customMockNVMLInstanceRR implements the nvml.Instance interface for testing with customizable behavior
type customMockNVMLInstanceRR struct {
	devs        map[string]device.Device
	nvmlExists  bool
	productName string
	initError   error
	memCaps     nvidiaproduct.MemoryErrorManagementCapabilities
}

func (m *customMockNVMLInstanceRR) Devices() map[string]device.Device { return m.devs }
func (m *customMockNVMLInstanceRR) FabricManagerSupported() bool      { return true }
func (m *customMockNVMLInstanceRR) FabricStateSupported() bool        { return false }
func (m *customMockNVMLInstanceRR) GetMemoryErrorManagementCapabilities() nvidiaproduct.MemoryErrorManagementCapabilities {
	return m.memCaps
}
func (m *customMockNVMLInstanceRR) ProductName() string   { return m.productName }
func (m *customMockNVMLInstanceRR) Architecture() string  { return "" }
func (m *customMockNVMLInstanceRR) Brand() string         { return "" }
func (m *customMockNVMLInstanceRR) DriverVersion() string { return "" }
func (m *customMockNVMLInstanceRR) DriverMajor() int      { return 0 }
func (m *customMockNVMLInstanceRR) CUDAVersion() string   { return "" }
func (m *customMockNVMLInstanceRR) NVMLExists() bool      { return m.nvmlExists }
func (m *customMockNVMLInstanceRR) Library() lib.Library  { return nil }
func (m *customMockNVMLInstanceRR) Shutdown() error       { return nil }
func (m *customMockNVMLInstanceRR) InitError() error      { return m.initError }

// createMockRemappedRowsComponent creates a component with mocked functions for testing
func createMockRemappedRowsComponent(
	ctx context.Context,
	nvmlInstance *customMockNVMLInstanceRR,
	getRemappedRowsFunc func(uuid string, dev device.Device) (RemappedRows, error),
) *component {
	cctx, cancel := context.WithCancel(ctx)

	return &component{
		ctx:    cctx,
		cancel: cancel,
		getTimeNowFunc: func() time.Time {
			return time.Now().UTC()
		},
		nvmlInstance:                    nvmlInstance,
		getRemappedRowsFunc:             getRemappedRowsFunc,
		gpuUUIDsWithRowRemappingPending: make(map[string]any),
		gpuUUIDsWithRowRemappingFailed:  make(map[string]any),
	}
}

// TestNew_WithMockey tests the New function using mockey for isolation
func TestNew_WithMockey(t *testing.T) {
	mockey.PatchConvey("New component creation", t, func() {
		ctx := context.Background()
		mockInstance := &customMockNVMLInstanceRR{
			devs:        map[string]device.Device{},
			nvmlExists:  true,
			productName: "NVIDIA H100",
			memCaps: nvidiaproduct.MemoryErrorManagementCapabilities{
				RowRemapping: true,
			},
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
		assert.NotNil(t, tc.getRemappedRowsFunc)
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
					customMock := &customMockNVMLInstanceRR{
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
		mockInst := &customMockNVMLInstanceRR{
			devs:        map[string]device.Device{},
			nvmlExists:  true,
			productName: "NVIDIA H100",
			initError:   initErr,
			memCaps: nvidiaproduct.MemoryErrorManagementCapabilities{
				RowRemapping: true,
			},
		}

		comp := createMockRemappedRowsComponent(ctx, mockInst, nil)

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

		mockInst := &customMockNVMLInstanceRR{
			devs:        map[string]device.Device{},
			nvmlExists:  true,
			productName: "",
		}

		comp := createMockRemappedRowsComponent(ctx, mockInst, nil)

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
			nvmlInstance:                    nil,
			gpuUUIDsWithRowRemappingPending: make(map[string]any),
			gpuUUIDsWithRowRemappingFailed:  make(map[string]any),
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

		mockInst := &customMockNVMLInstanceRR{
			devs:        map[string]device.Device{},
			nvmlExists:  false,
			productName: "NVIDIA H100",
		}

		comp := createMockRemappedRowsComponent(ctx, mockInst, nil)

		result := comp.Check()
		cr, ok := result.(*checkResult)
		require.True(t, ok)

		assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
		assert.Contains(t, cr.reason, "NVIDIA NVML library is not loaded")
	})
}

// TestCheck_RowRemappingNotSupported_WithMockey tests Check when row remapping is not supported
func TestCheck_RowRemappingNotSupported_WithMockey(t *testing.T) {
	mockey.PatchConvey("Check with row remapping not supported", t, func() {
		ctx := context.Background()

		mockInst := &customMockNVMLInstanceRR{
			devs:        map[string]device.Device{},
			nvmlExists:  true,
			productName: "NVIDIA GeForce RTX 4090",
			memCaps: nvidiaproduct.MemoryErrorManagementCapabilities{
				RowRemapping: false,
			},
		}

		comp := createMockRemappedRowsComponent(ctx, mockInst, nil)

		result := comp.Check()
		cr, ok := result.(*checkResult)
		require.True(t, ok)

		assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
		assert.Contains(t, cr.reason, "does not support row remapping")
	})
}

// TestCheck_RemappingPending_WithMockey tests Check when row remapping is pending
func TestCheck_RemappingPending_WithMockey(t *testing.T) {
	mockey.PatchConvey("Check with remapping pending", t, func() {
		ctx := context.Background()

		uuid := "gpu-uuid-pending"
		mockDeviceObj := &mock.Device{
			GetUUIDFunc: func() (string, nvml.Return) {
				return uuid, nvml.SUCCESS
			},
		}
		mockDev := testutil.NewMockDevice(mockDeviceObj, "test-arch", "test-brand", "test-cuda", "0000:01:00.0")

		mockInst := &customMockNVMLInstanceRR{
			devs:        map[string]device.Device{uuid: mockDev},
			nvmlExists:  true,
			productName: "NVIDIA H100",
			memCaps: nvidiaproduct.MemoryErrorManagementCapabilities{
				RowRemapping: true,
			},
		}

		getRemappedRowsFunc := func(uuid string, dev device.Device) (RemappedRows, error) {
			return RemappedRows{
				UUID:             uuid,
				BusID:            dev.PCIBusID(),
				RemappingPending: true,
				Supported:        true,
			}, nil
		}

		comp := createMockRemappedRowsComponent(ctx, mockInst, getRemappedRowsFunc)
		result := comp.Check()
		cr, ok := result.(*checkResult)
		require.True(t, ok)

		assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
		assert.Contains(t, cr.reason, "needs reset")
		assert.NotNil(t, cr.suggestedActions)
		assert.Contains(t, cr.suggestedActions.RepairActions, apiv1.RepairActionTypeRebootSystem)
	})
}

// TestCheck_RemappingFailed_WithMockey tests Check when row remapping has failed
func TestCheck_RemappingFailed_WithMockey(t *testing.T) {
	mockey.PatchConvey("Check with remapping failed", t, func() {
		ctx := context.Background()

		uuid := "gpu-uuid-failed"
		mockDeviceObj := &mock.Device{
			GetUUIDFunc: func() (string, nvml.Return) {
				return uuid, nvml.SUCCESS
			},
		}
		mockDev := testutil.NewMockDevice(mockDeviceObj, "test-arch", "test-brand", "test-cuda", "0000:01:00.0")

		mockInst := &customMockNVMLInstanceRR{
			devs:        map[string]device.Device{uuid: mockDev},
			nvmlExists:  true,
			productName: "NVIDIA H100",
			memCaps: nvidiaproduct.MemoryErrorManagementCapabilities{
				RowRemapping: true,
			},
		}

		getRemappedRowsFunc := func(uuid string, dev device.Device) (RemappedRows, error) {
			return RemappedRows{
				UUID:                             uuid,
				BusID:                            dev.PCIBusID(),
				RemappingFailed:                  true,
				RemappedDueToUncorrectableErrors: 3,
				Supported:                        true,
			}, nil
		}

		comp := createMockRemappedRowsComponent(ctx, mockInst, getRemappedRowsFunc)
		result := comp.Check()
		cr, ok := result.(*checkResult)
		require.True(t, ok)

		assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
		assert.Contains(t, cr.reason, "qualifies for RMA")
		assert.NotNil(t, cr.suggestedActions)
		assert.Contains(t, cr.suggestedActions.RepairActions, apiv1.RepairActionTypeHardwareInspection)
	})
}

// TestCheck_GPULostError_WithMockey tests Check when GPU lost error occurs
// Note: When getRemappedRowsFunc returns an error, the component continues to next device
// and the final health is determined by whether issues were found (not by the error itself).
// The error IS stored in cr.err but health may be reset to Healthy if no issues found.
func TestCheck_GPULostError_WithMockey(t *testing.T) {
	mockey.PatchConvey("Check with GPU lost error - error is stored but health determined by issues", t, func() {
		ctx := context.Background()

		uuid := "gpu-uuid-lost"
		mockDeviceObj := &mock.Device{
			GetUUIDFunc: func() (string, nvml.Return) {
				return uuid, nvml.SUCCESS
			},
		}
		mockDev := testutil.NewMockDevice(mockDeviceObj, "test-arch", "test-brand", "test-cuda", "0000:01:00.0")

		mockInst := &customMockNVMLInstanceRR{
			devs:        map[string]device.Device{uuid: mockDev},
			nvmlExists:  true,
			productName: "NVIDIA H100",
			memCaps: nvidiaproduct.MemoryErrorManagementCapabilities{
				RowRemapping: true,
			},
		}

		getRemappedRowsFunc := func(uuid string, dev device.Device) (RemappedRows, error) {
			return RemappedRows{}, nvmlerrors.ErrGPULost
		}

		comp := createMockRemappedRowsComponent(ctx, mockInst, getRemappedRowsFunc)
		result := comp.Check()
		cr, ok := result.(*checkResult)
		require.True(t, ok)

		// Error is stored in cr.err
		assert.True(t, errors.Is(cr.err, nvmlerrors.ErrGPULost))
		// Health is determined by issues at the end, since no issues found, it's Healthy
		// (component continues past errors and resets health at end of Check)
		assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
	})
}

// TestCheck_GPURequiresReset_WithMockey tests Check when GPU requires reset
// Note: Same as GPULost - error is stored but health is determined by issues at the end
func TestCheck_GPURequiresReset_WithMockey(t *testing.T) {
	mockey.PatchConvey("Check with GPU requires reset - error is stored but health determined by issues", t, func() {
		ctx := context.Background()

		uuid := "gpu-uuid-reset"
		mockDeviceObj := &mock.Device{
			GetUUIDFunc: func() (string, nvml.Return) {
				return uuid, nvml.SUCCESS
			},
		}
		mockDev := testutil.NewMockDevice(mockDeviceObj, "test-arch", "test-brand", "test-cuda", "0000:01:00.0")

		mockInst := &customMockNVMLInstanceRR{
			devs:        map[string]device.Device{uuid: mockDev},
			nvmlExists:  true,
			productName: "NVIDIA H100",
			memCaps: nvidiaproduct.MemoryErrorManagementCapabilities{
				RowRemapping: true,
			},
		}

		getRemappedRowsFunc := func(uuid string, dev device.Device) (RemappedRows, error) {
			return RemappedRows{}, nvmlerrors.ErrGPURequiresReset
		}

		comp := createMockRemappedRowsComponent(ctx, mockInst, getRemappedRowsFunc)
		result := comp.Check()
		cr, ok := result.(*checkResult)
		require.True(t, ok)

		// Error is stored
		assert.True(t, errors.Is(cr.err, nvmlerrors.ErrGPURequiresReset))
		// Health is determined by issues at the end
		assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
	})
}

// TestCheck_HealthyGPUs_WithMockey tests Check with healthy GPUs
func TestCheck_HealthyGPUs_WithMockey(t *testing.T) {
	mockey.PatchConvey("Check with healthy GPUs", t, func() {
		ctx := context.Background()

		uuid1 := "gpu-uuid-1"
		uuid2 := "gpu-uuid-2"

		mockDeviceObj1 := &mock.Device{
			GetUUIDFunc: func() (string, nvml.Return) {
				return uuid1, nvml.SUCCESS
			},
		}
		mockDev1 := testutil.NewMockDevice(mockDeviceObj1, "test-arch", "test-brand", "test-cuda", "0000:01:00.0")

		mockDeviceObj2 := &mock.Device{
			GetUUIDFunc: func() (string, nvml.Return) {
				return uuid2, nvml.SUCCESS
			},
		}
		mockDev2 := testutil.NewMockDevice(mockDeviceObj2, "test-arch", "test-brand", "test-cuda", "0000:02:00.0")

		mockInst := &customMockNVMLInstanceRR{
			devs:        map[string]device.Device{uuid1: mockDev1, uuid2: mockDev2},
			nvmlExists:  true,
			productName: "NVIDIA H100",
			memCaps: nvidiaproduct.MemoryErrorManagementCapabilities{
				RowRemapping: true,
			},
		}

		getRemappedRowsFunc := func(uuid string, dev device.Device) (RemappedRows, error) {
			return RemappedRows{
				UUID:      uuid,
				BusID:     dev.PCIBusID(),
				Supported: true,
			}, nil
		}

		comp := createMockRemappedRowsComponent(ctx, mockInst, getRemappedRowsFunc)
		result := comp.Check()
		cr, ok := result.(*checkResult)
		require.True(t, ok)

		assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
		assert.Contains(t, cr.reason, "2 devices support remapped rows and found no issue")
		assert.Len(t, cr.RemappedRows, 2)
	})
}

// TestCheck_FailureInjection_WithMockey tests Check with failure injection
func TestCheck_FailureInjection_WithMockey(t *testing.T) {
	mockey.PatchConvey("Check with failure injection", t, func() {
		ctx := context.Background()

		uuid := "gpu-uuid-inject"
		mockDeviceObj := &mock.Device{
			GetUUIDFunc: func() (string, nvml.Return) {
				return uuid, nvml.SUCCESS
			},
		}
		mockDev := testutil.NewMockDevice(mockDeviceObj, "test-arch", "test-brand", "test-cuda", "0000:01:00.0")

		mockInst := &customMockNVMLInstanceRR{
			devs:        map[string]device.Device{uuid: mockDev},
			nvmlExists:  true,
			productName: "NVIDIA H100",
			memCaps: nvidiaproduct.MemoryErrorManagementCapabilities{
				RowRemapping: true,
			},
		}

		getRemappedRowsFunc := func(uuid string, dev device.Device) (RemappedRows, error) {
			return RemappedRows{
				UUID:      uuid,
				BusID:     dev.PCIBusID(),
				Supported: true,
			}, nil
		}

		comp := createMockRemappedRowsComponent(ctx, mockInst, getRemappedRowsFunc)
		// Inject failure for this UUID
		comp.gpuUUIDsWithRowRemappingPending[uuid] = nil

		result := comp.Check()
		cr, ok := result.(*checkResult)
		require.True(t, ok)

		assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
		assert.Contains(t, cr.reason, "needs reset")

		// Also test remapping failed injection
		delete(comp.gpuUUIDsWithRowRemappingPending, uuid)
		comp.gpuUUIDsWithRowRemappingFailed[uuid] = nil

		result = comp.Check()
		cr, ok = result.(*checkResult)
		require.True(t, ok)

		assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
		assert.Contains(t, cr.reason, "qualifies for RMA")
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

	t.Run("HealthStates with suggested actions", func(t *testing.T) {
		cr := &checkResult{
			ts:     time.Now(),
			health: apiv1.HealthStateTypeUnhealthy,
			reason: "row remapping failed",
			err:    errors.New("test error"),
			suggestedActions: &apiv1.SuggestedActions{
				Description: "hardware inspection required",
				RepairActions: []apiv1.RepairActionType{
					apiv1.RepairActionTypeHardwareInspection,
				},
			},
		}

		states := cr.HealthStates()
		require.Len(t, states, 1)
		assert.NotNil(t, states[0].SuggestedActions)
		assert.Contains(t, states[0].SuggestedActions.RepairActions, apiv1.RepairActionTypeHardwareInspection)
	})

	t.Run("HealthStates with extra info", func(t *testing.T) {
		cr := &checkResult{
			ts:     time.Now(),
			health: apiv1.HealthStateTypeHealthy,
			reason: "all good",
			RemappedRows: []RemappedRows{
				{
					UUID:      "gpu-1",
					BusID:     "0000:01:00.0",
					Supported: true,
				},
			},
		}

		states := cr.HealthStates()
		require.Len(t, states, 1)
		assert.NotEmpty(t, states[0].ExtraInfo)
		assert.Contains(t, states[0].ExtraInfo["data"], "gpu-1")
	})
}

// TestGetRemappedRows_WithMockey tests the GetRemappedRows function
func TestGetRemappedRows_WithMockey(t *testing.T) {
	testCases := []struct {
		name             string
		corrRows         int
		uncRows          int
		isPending        bool
		failureOccurred  bool
		ret              nvml.Return
		expectedSupport  bool
		expectedErr      error
		expectError      bool
		expectedPending  bool
		expectedFailed   bool
		expectedCorrRows int
		expectedUncRows  int
	}{
		{
			name:             "success case",
			corrRows:         2,
			uncRows:          1,
			isPending:        true,
			failureOccurred:  false,
			ret:              nvml.SUCCESS,
			expectedSupport:  true,
			expectError:      false,
			expectedPending:  true,
			expectedFailed:   false,
			expectedCorrRows: 2,
			expectedUncRows:  1,
		},
		{
			name:            "not supported",
			ret:             nvml.ERROR_NOT_SUPPORTED,
			expectedSupport: false,
			expectError:     false,
		},
		{
			name:        "GPU lost error",
			ret:         nvml.ERROR_GPU_IS_LOST,
			expectedErr: nvmlerrors.ErrGPULost,
			expectError: true,
		},
		{
			name:        "GPU requires reset",
			ret:         nvml.ERROR_RESET_REQUIRED,
			expectedErr: nvmlerrors.ErrGPURequiresReset,
			expectError: true,
		},
		{
			name:        "unknown error",
			ret:         nvml.ERROR_UNKNOWN,
			expectError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockey.PatchConvey(tc.name, t, func() {
				mockDevice := &mock.Device{
					GetRemappedRowsFunc: func() (int, int, bool, bool, nvml.Return) {
						return tc.corrRows, tc.uncRows, tc.isPending, tc.failureOccurred, tc.ret
					},
				}

				dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "0000:01:00.0")

				result, err := GetRemappedRows("test-uuid", dev)

				if tc.expectError {
					assert.Error(t, err)
					if tc.expectedErr != nil {
						assert.True(t, errors.Is(err, tc.expectedErr))
					}
				} else {
					assert.NoError(t, err)
					assert.Equal(t, tc.expectedSupport, result.Supported)
					assert.Equal(t, tc.expectedPending, result.RemappingPending)
					assert.Equal(t, tc.expectedFailed, result.RemappingFailed)
					assert.Equal(t, tc.expectedCorrRows, result.RemappedDueToCorrectableErrors)
					assert.Equal(t, tc.expectedUncRows, result.RemappedDueToUncorrectableErrors)
				}
			})
		})
	}
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
		mockDev := testutil.NewMockDevice(mockDeviceObj, "test-arch", "test-brand", "test-cuda", "0000:01:00.0")

		mockInst := &customMockNVMLInstanceRR{
			devs:        map[string]device.Device{uuid: mockDev},
			nvmlExists:  true,
			productName: "NVIDIA H100",
			memCaps: nvidiaproduct.MemoryErrorManagementCapabilities{
				RowRemapping: true,
			},
		}

		getRemappedRowsFunc := func(uuid string, dev device.Device) (RemappedRows, error) {
			return RemappedRows{
				UUID:      uuid,
				BusID:     dev.PCIBusID(),
				Supported: true,
			}, nil
		}

		comp := createMockRemappedRowsComponent(ctx, mockInst, getRemappedRowsFunc)

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

// TestClose_WithMockey tests the Close method
func TestClose_WithMockey(t *testing.T) {
	mockey.PatchConvey("Close component", t, func() {
		ctx := context.Background()

		mockInst := &customMockNVMLInstanceRR{
			devs:        map[string]device.Device{},
			nvmlExists:  true,
			productName: "NVIDIA H100",
		}

		comp := createMockRemappedRowsComponent(ctx, mockInst, nil)

		err := comp.Close()
		assert.NoError(t, err)

		// Verify context is canceled
		select {
		case <-comp.ctx.Done():
			// Context properly canceled
		default:
			t.Fatal("context was not canceled on Close")
		}
	})
}

// TestEvents_WithMockey tests the Events method
func TestEvents_WithMockey(t *testing.T) {
	mockey.PatchConvey("Events returns nil when no bucket", t, func() {
		ctx := context.Background()

		mockInst := &customMockNVMLInstanceRR{
			devs:        map[string]device.Device{},
			nvmlExists:  true,
			productName: "NVIDIA H100",
		}

		comp := createMockRemappedRowsComponent(ctx, mockInst, nil)

		events, err := comp.Events(ctx, time.Now())
		assert.NoError(t, err)
		assert.Nil(t, events)
	})
}

// TestStart_WithMockey tests the Start method
func TestStart_WithMockey(t *testing.T) {
	mockey.PatchConvey("Start component", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		checkCalled := make(chan bool, 1)

		uuid := "gpu-uuid-start"
		mockDeviceObj := &mock.Device{
			GetUUIDFunc: func() (string, nvml.Return) {
				return uuid, nvml.SUCCESS
			},
		}
		mockDev := testutil.NewMockDevice(mockDeviceObj, "test-arch", "test-brand", "test-cuda", "0000:01:00.0")

		mockInst := &customMockNVMLInstanceRR{
			devs:        map[string]device.Device{uuid: mockDev},
			nvmlExists:  true,
			productName: "NVIDIA H100",
			memCaps: nvidiaproduct.MemoryErrorManagementCapabilities{
				RowRemapping: true,
			},
		}

		comp := createMockRemappedRowsComponent(ctx, mockInst, func(uuid string, dev device.Device) (RemappedRows, error) {
			select {
			case checkCalled <- true:
			default:
			}
			return RemappedRows{UUID: uuid, BusID: dev.PCIBusID(), Supported: true}, nil
		})

		err := comp.Start()
		assert.NoError(t, err)

		// Wait for Check to be called
		select {
		case <-checkCalled:
			// Success
		case <-time.After(500 * time.Millisecond):
			t.Fatal("Check was not called within expected time")
		}
	})
}
