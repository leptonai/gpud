//go:build linux

package hwslowdown

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
	"github.com/leptonai/gpud/pkg/eventstore"
	nvmlerrors "github.com/leptonai/gpud/pkg/nvidia/errors"
	"github.com/leptonai/gpud/pkg/nvidia/nvml/device"
	"github.com/leptonai/gpud/pkg/nvidia/nvml/lib"
	"github.com/leptonai/gpud/pkg/nvidia/nvml/testutil"
	nvidiaproduct "github.com/leptonai/gpud/pkg/nvidia/product"
)

// customMockNVMLInstanceHW implements the nvml.Instance interface for testing with customizable behavior
type customMockNVMLInstanceHW struct {
	devs        map[string]device.Device
	nvmlExists  bool
	productName string
	initError   error
}

func (m *customMockNVMLInstanceHW) Devices() map[string]device.Device { return m.devs }
func (m *customMockNVMLInstanceHW) FabricManagerSupported() bool      { return true }
func (m *customMockNVMLInstanceHW) FabricStateSupported() bool        { return false }
func (m *customMockNVMLInstanceHW) GetMemoryErrorManagementCapabilities() nvidiaproduct.MemoryErrorManagementCapabilities {
	return nvidiaproduct.MemoryErrorManagementCapabilities{}
}
func (m *customMockNVMLInstanceHW) ProductName() string   { return m.productName }
func (m *customMockNVMLInstanceHW) Architecture() string  { return "" }
func (m *customMockNVMLInstanceHW) Brand() string         { return "" }
func (m *customMockNVMLInstanceHW) DriverVersion() string { return "" }
func (m *customMockNVMLInstanceHW) DriverMajor() int      { return 0 }
func (m *customMockNVMLInstanceHW) CUDAVersion() string   { return "" }
func (m *customMockNVMLInstanceHW) NVMLExists() bool      { return m.nvmlExists }
func (m *customMockNVMLInstanceHW) Library() lib.Library  { return nil }
func (m *customMockNVMLInstanceHW) Shutdown() error       { return nil }
func (m *customMockNVMLInstanceHW) InitError() error      { return m.initError }

// mockEventBucketHW implements eventstore.Bucket for testing
type mockEventBucketHW struct {
	events    eventstore.Events
	purgeErr  error
	purgeRet  int
	insertErr error
}

func (m *mockEventBucketHW) Name() string { return "mock-event-bucket" }
func (m *mockEventBucketHW) Insert(ctx context.Context, event eventstore.Event) error {
	if m.insertErr != nil {
		return m.insertErr
	}
	m.events = append(m.events, event)
	return nil
}
func (m *mockEventBucketHW) Find(ctx context.Context, event eventstore.Event) (*eventstore.Event, error) {
	return nil, nil
}
func (m *mockEventBucketHW) Get(ctx context.Context, since time.Time) (eventstore.Events, error) {
	return m.events, nil
}
func (m *mockEventBucketHW) Latest(ctx context.Context) (*eventstore.Event, error) {
	if len(m.events) == 0 {
		return nil, nil
	}
	return &m.events[len(m.events)-1], nil
}
func (m *mockEventBucketHW) Purge(ctx context.Context, beforeTimestamp int64) (int, error) {
	if m.purgeErr != nil {
		return 0, m.purgeErr
	}
	m.events = nil
	return m.purgeRet, nil
}
func (m *mockEventBucketHW) Close() {}

// createMockHWSlowdownComponent creates a component with mocked functions for testing
func createMockHWSlowdownComponent(
	ctx context.Context,
	nvmlInstance *customMockNVMLInstanceHW,
	getClockEventsSupportedFunc func(dev device.Device) (bool, error),
	getClockEventsFunc func(uuid string, dev device.Device) (ClockEvents, error),
) *component {
	cctx, cancel := context.WithCancel(ctx)

	return &component{
		ctx:    cctx,
		cancel: cancel,
		getTimeNowFunc: func() time.Time {
			return time.Now().UTC()
		},
		nvmlInstance:                     nvmlInstance,
		getClockEventsSupportedFunc:      getClockEventsSupportedFunc,
		getClockEventsFunc:               getClockEventsFunc,
		gpuUUIDsWithHWSlowdown:           make(map[string]any),
		gpuUUIDsWithHWSlowdownThermal:    make(map[string]any),
		gpuUUIDsWithHWSlowdownPowerBrake: make(map[string]any),
		freqPerMinEvaluationWindow:       DefaultStateHWSlowdownEvaluationWindow,
		freqPerMinThreshold:              DefaultStateHWSlowdownEventsThresholdFrequencyPerMinute,
	}
}

// TestNew_WithMockey tests the New function
func TestNew_WithMockey(t *testing.T) {
	mockey.PatchConvey("New component creation", t, func() {
		ctx := context.Background()
		mockInstance := &customMockNVMLInstanceHW{
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
		assert.NotNil(t, tc.getClockEventsSupportedFunc)
		assert.NotNil(t, tc.getClockEventsFunc)
	})
}

// TestNew_WithFailureInjector tests New with failure injector
func TestNew_WithFailureInjector(t *testing.T) {
	mockey.PatchConvey("New with failure injector", t, func() {
		ctx := context.Background()
		mockInstance := &customMockNVMLInstanceHW{
			devs:        map[string]device.Device{},
			nvmlExists:  true,
			productName: "NVIDIA H100",
		}

		failureInjector := &components.FailureInjector{
			GPUUUIDsWithHWSlowdown:           []string{"gpu-slow-1"},
			GPUUUIDsWithHWSlowdownThermal:    []string{"gpu-thermal-1"},
			GPUUUIDsWithHWSlowdownPowerBrake: []string{"gpu-power-1"},
		}

		gpudInstance := &components.GPUdInstance{
			RootCtx:         ctx,
			NVMLInstance:    mockInstance,
			FailureInjector: failureInjector,
		}

		c, err := New(gpudInstance)

		assert.NoError(t, err)
		assert.NotNil(t, c)

		tc, ok := c.(*component)
		require.True(t, ok)
		_, hasSlow := tc.gpuUUIDsWithHWSlowdown["gpu-slow-1"]
		assert.True(t, hasSlow)
		_, hasThermal := tc.gpuUUIDsWithHWSlowdownThermal["gpu-thermal-1"]
		assert.True(t, hasThermal)
		_, hasPower := tc.gpuUUIDsWithHWSlowdownPowerBrake["gpu-power-1"]
		assert.True(t, hasPower)
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
					customMock := &customMockNVMLInstanceHW{
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
			nvmlInstance:                     nil,
			gpuUUIDsWithHWSlowdown:           make(map[string]any),
			gpuUUIDsWithHWSlowdownThermal:    make(map[string]any),
			gpuUUIDsWithHWSlowdownPowerBrake: make(map[string]any),
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

		mockInst := &customMockNVMLInstanceHW{
			devs:        map[string]device.Device{},
			nvmlExists:  false,
			productName: "NVIDIA H100",
		}

		comp := createMockHWSlowdownComponent(ctx, mockInst, nil, nil)

		result := comp.Check()
		cr, ok := result.(*checkResult)
		require.True(t, ok)

		assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
		assert.Contains(t, cr.reason, "NVIDIA NVML library is not loaded")
	})
}

// TestCheck_InitError_WithMockey tests Check when NVML has an initialization error
func TestCheck_InitError_WithMockey(t *testing.T) {
	mockey.PatchConvey("Check with NVML init error", t, func() {
		ctx := context.Background()

		initErr := errors.New("error getting device handle for index '0': Unknown Error")
		mockInst := &customMockNVMLInstanceHW{
			devs:        map[string]device.Device{},
			nvmlExists:  true,
			productName: "NVIDIA H100",
			initError:   initErr,
		}

		comp := createMockHWSlowdownComponent(ctx, mockInst, nil, nil)

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

		mockInst := &customMockNVMLInstanceHW{
			devs:        map[string]device.Device{},
			nvmlExists:  true,
			productName: "",
		}

		comp := createMockHWSlowdownComponent(ctx, mockInst, nil, nil)

		result := comp.Check()
		cr, ok := result.(*checkResult)
		require.True(t, ok)

		assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
		assert.Contains(t, cr.reason, "missing product name")
	})
}

// TestCheck_ClockEventsNotSupported_WithMockey tests Check when clock events are not supported
func TestCheck_ClockEventsNotSupported_WithMockey(t *testing.T) {
	mockey.PatchConvey("Check with clock events not supported", t, func() {
		ctx := context.Background()

		uuid := "gpu-uuid-no-clock"
		mockDeviceObj := &mock.Device{
			GetUUIDFunc: func() (string, nvml.Return) {
				return uuid, nvml.SUCCESS
			},
		}
		mockDev := testutil.NewMockDevice(mockDeviceObj, "test-arch", "test-brand", "test-cuda", "test-pci")

		mockInst := &customMockNVMLInstanceHW{
			devs:        map[string]device.Device{uuid: mockDev},
			nvmlExists:  true,
			productName: "NVIDIA H100",
		}

		getClockEventsSupportedFunc := func(dev device.Device) (bool, error) {
			return false, nil
		}

		comp := createMockHWSlowdownComponent(ctx, mockInst, getClockEventsSupportedFunc, nil)

		result := comp.Check()
		cr, ok := result.(*checkResult)
		require.True(t, ok)

		assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
		assert.Contains(t, cr.reason, "clock events not supported")
	})
}

// TestCheck_HWSlowdownDetected_WithMockey tests Check when HW slowdown is detected
func TestCheck_HWSlowdownDetected_WithMockey(t *testing.T) {
	mockey.PatchConvey("Check with HW slowdown detected", t, func() {
		ctx := context.Background()

		uuid := "gpu-uuid-slow"
		mockDeviceObj := &mock.Device{
			GetUUIDFunc: func() (string, nvml.Return) {
				return uuid, nvml.SUCCESS
			},
		}
		mockDev := testutil.NewMockDevice(mockDeviceObj, "test-arch", "test-brand", "test-cuda", "test-pci")

		mockInst := &customMockNVMLInstanceHW{
			devs:        map[string]device.Device{uuid: mockDev},
			nvmlExists:  true,
			productName: "NVIDIA H100",
		}

		getClockEventsSupportedFunc := func(dev device.Device) (bool, error) {
			return true, nil
		}

		getClockEventsFunc := func(uuid string, dev device.Device) (ClockEvents, error) {
			return ClockEvents{
				UUID:                 uuid,
				HWSlowdown:           true,
				HWSlowdownThermal:    false,
				HWSlowdownPowerBrake: false,
				HWSlowdownReasons:    []string{"HW Slowdown"},
				Supported:            true,
			}, nil
		}

		comp := createMockHWSlowdownComponent(ctx, mockInst, getClockEventsSupportedFunc, getClockEventsFunc)

		result := comp.Check()
		cr, ok := result.(*checkResult)
		require.True(t, ok)

		// Should be healthy if it's just a one-time event
		// The component evaluates based on frequency threshold
		assert.Len(t, cr.ClockEvents, 1)
		assert.True(t, cr.ClockEvents[0].HWSlowdown)
	})
}

// TestCheck_GPULostError_WithMockey tests Check when GPU lost error occurs
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

		mockInst := &customMockNVMLInstanceHW{
			devs:        map[string]device.Device{uuid: mockDev},
			nvmlExists:  true,
			productName: "NVIDIA H100",
		}

		getClockEventsSupportedFunc := func(dev device.Device) (bool, error) {
			return false, nvmlerrors.ErrGPULost
		}

		comp := createMockHWSlowdownComponent(ctx, mockInst, getClockEventsSupportedFunc, nil)

		result := comp.Check()
		cr, ok := result.(*checkResult)
		require.True(t, ok)

		assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
		assert.Contains(t, cr.reason, "error getting clock events supported")
		assert.True(t, errors.Is(cr.err, nvmlerrors.ErrGPULost))
	})
}

// TestCheck_GPURequiresReset_WithMockey tests Check when GPU requires reset
func TestCheck_GPURequiresReset_WithMockey(t *testing.T) {
	mockey.PatchConvey("Check with GPU requires reset", t, func() {
		ctx := context.Background()

		uuid := "gpu-uuid-reset"
		mockDeviceObj := &mock.Device{
			GetUUIDFunc: func() (string, nvml.Return) {
				return uuid, nvml.SUCCESS
			},
		}
		mockDev := testutil.NewMockDevice(mockDeviceObj, "test-arch", "test-brand", "test-cuda", "test-pci")

		mockInst := &customMockNVMLInstanceHW{
			devs:        map[string]device.Device{uuid: mockDev},
			nvmlExists:  true,
			productName: "NVIDIA H100",
		}

		getClockEventsSupportedFunc := func(dev device.Device) (bool, error) {
			return false, nvmlerrors.ErrGPURequiresReset
		}

		comp := createMockHWSlowdownComponent(ctx, mockInst, getClockEventsSupportedFunc, nil)

		result := comp.Check()
		cr, ok := result.(*checkResult)
		require.True(t, ok)

		assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
		assert.Contains(t, cr.reason, "error getting clock events supported")
		assert.True(t, errors.Is(cr.err, nvmlerrors.ErrGPURequiresReset))
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

	t.Run("String with clock events", func(t *testing.T) {
		cr := &checkResult{
			ClockEvents: []ClockEvents{
				{
					UUID:                 "gpu-1",
					HWSlowdown:           true,
					HWSlowdownThermal:    false,
					HWSlowdownPowerBrake: false,
					HWSlowdownReasons:    []string{"reason1"},
				},
			},
		}
		result := cr.String()
		assert.Contains(t, result, "gpu-1")
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

	t.Run("HealthStates with clock events", func(t *testing.T) {
		cr := &checkResult{
			ts:     time.Now(),
			health: apiv1.HealthStateTypeHealthy,
			reason: "all good",
			ClockEvents: []ClockEvents{
				{
					UUID:       "gpu-1",
					HWSlowdown: false,
				},
			},
		}

		states := cr.HealthStates()
		require.Len(t, states, 1)
		assert.NotEmpty(t, states[0].ExtraInfo)
		assert.Contains(t, states[0].ExtraInfo["data"], "gpu-1")
	})
}

// TestSetHealthy_WithMockey tests the SetHealthy method
func TestSetHealthy_WithMockey(t *testing.T) {
	t.Run("SetHealthy with nil event bucket", func(t *testing.T) {
		mockey.PatchConvey("SetHealthy with nil bucket", t, func() {
			ctx := context.Background()
			cctx, cancel := context.WithCancel(ctx)
			defer cancel()

			comp := &component{
				ctx:         cctx,
				cancel:      cancel,
				eventBucket: nil,
				getTimeNowFunc: func() time.Time {
					return time.Now().UTC()
				},
			}

			err := comp.SetHealthy()
			assert.NoError(t, err)
		})
	})

	t.Run("SetHealthy with event bucket", func(t *testing.T) {
		mockey.PatchConvey("SetHealthy with bucket", t, func() {
			ctx := context.Background()
			cctx, cancel := context.WithCancel(ctx)
			defer cancel()

			mockBucket := &mockEventBucketHW{
				purgeRet: 5,
			}

			comp := &component{
				ctx:         cctx,
				cancel:      cancel,
				eventBucket: mockBucket,
				getTimeNowFunc: func() time.Time {
					return time.Now().UTC()
				},
			}

			err := comp.SetHealthy()
			assert.NoError(t, err)
		})
	})

	t.Run("SetHealthy with purge error", func(t *testing.T) {
		mockey.PatchConvey("SetHealthy with purge error", t, func() {
			ctx := context.Background()
			cctx, cancel := context.WithCancel(ctx)
			defer cancel()

			purgeErr := errors.New("purge failed")
			mockBucket := &mockEventBucketHW{
				purgeErr: purgeErr,
			}

			comp := &component{
				ctx:         cctx,
				cancel:      cancel,
				eventBucket: mockBucket,
				getTimeNowFunc: func() time.Time {
					return time.Now().UTC()
				},
			}

			err := comp.SetHealthy()
			assert.Error(t, err)
			assert.Equal(t, purgeErr, err)
		})
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
		mockDev := testutil.NewMockDevice(mockDeviceObj, "test-arch", "test-brand", "test-cuda", "test-pci")

		mockInst := &customMockNVMLInstanceHW{
			devs:        map[string]device.Device{uuid: mockDev},
			nvmlExists:  true,
			productName: "NVIDIA H100",
		}

		getClockEventsSupportedFunc := func(dev device.Device) (bool, error) {
			return true, nil
		}

		getClockEventsFunc := func(uuid string, dev device.Device) (ClockEvents, error) {
			return ClockEvents{
				UUID:      uuid,
				Supported: true,
			}, nil
		}

		comp := createMockHWSlowdownComponent(ctx, mockInst, getClockEventsSupportedFunc, getClockEventsFunc)
		// Inject HW slowdown for this UUID
		comp.gpuUUIDsWithHWSlowdown[uuid] = nil

		result := comp.Check()
		cr, ok := result.(*checkResult)
		require.True(t, ok)

		// The injected HW slowdown should be detected
		assert.Len(t, cr.ClockEvents, 1)
		assert.True(t, cr.ClockEvents[0].HWSlowdown)
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

		mockInst := &customMockNVMLInstanceHW{
			devs:        map[string]device.Device{uuid: mockDev},
			nvmlExists:  true,
			productName: "NVIDIA H100",
		}

		getClockEventsSupportedFunc := func(dev device.Device) (bool, error) {
			return true, nil
		}

		getClockEventsFunc := func(uuid string, dev device.Device) (ClockEvents, error) {
			return ClockEvents{
				UUID:      uuid,
				Supported: true,
			}, nil
		}

		comp := createMockHWSlowdownComponent(ctx, mockInst, getClockEventsSupportedFunc, getClockEventsFunc)

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

// TestClockEventsSupportedByDevice_WithMockey tests ClockEventsSupportedByDevice function
func TestClockEventsSupportedByDevice_WithMockey(t *testing.T) {
	testCases := []struct {
		name           string
		ret            nvml.Return
		expectedResult bool
		expectedErr    error
		expectError    bool
	}{
		{
			name:           "supported",
			ret:            nvml.SUCCESS,
			expectedResult: true,
			expectError:    false,
		},
		{
			name:           "not supported error",
			ret:            nvml.ERROR_NOT_SUPPORTED,
			expectedResult: false,
			expectError:    false,
		},
		{
			name:        "GPU lost",
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
					GetCurrentClocksEventReasonsFunc: func() (uint64, nvml.Return) {
						return 0, tc.ret
					},
				}

				dev := testutil.NewMockDevice(mockDevice, "test-arch", "test-brand", "test-cuda", "test-pci")

				result, err := ClockEventsSupportedByDevice(dev)

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
