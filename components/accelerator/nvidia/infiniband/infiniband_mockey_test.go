//go:build linux

package infiniband

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/bytedance/mockey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	infinibandclass "github.com/leptonai/gpud/components/accelerator/nvidia/infiniband/class"
	"github.com/leptonai/gpud/components/accelerator/nvidia/infiniband/types"
	pkgconfigcommon "github.com/leptonai/gpud/pkg/config/common"
	"github.com/leptonai/gpud/pkg/eventstore"
	"github.com/leptonai/gpud/pkg/nvidia/nvml/device"
	"github.com/leptonai/gpud/pkg/nvidia/nvml/lib"
	nvidiaproduct "github.com/leptonai/gpud/pkg/nvidia/product"
)

// customMockNVMLInstanceIB implements the nvml.Instance interface for testing with customizable behavior
type customMockNVMLInstanceIB struct {
	devs        map[string]device.Device
	nvmlExists  bool
	productName string
	initError   error
}

func (m *customMockNVMLInstanceIB) Devices() map[string]device.Device { return m.devs }
func (m *customMockNVMLInstanceIB) FabricManagerSupported() bool      { return true }
func (m *customMockNVMLInstanceIB) FabricStateSupported() bool        { return false }
func (m *customMockNVMLInstanceIB) GetMemoryErrorManagementCapabilities() nvidiaproduct.MemoryErrorManagementCapabilities {
	return nvidiaproduct.MemoryErrorManagementCapabilities{}
}
func (m *customMockNVMLInstanceIB) ProductName() string   { return m.productName }
func (m *customMockNVMLInstanceIB) Architecture() string  { return "" }
func (m *customMockNVMLInstanceIB) Brand() string         { return "" }
func (m *customMockNVMLInstanceIB) DriverVersion() string { return "" }
func (m *customMockNVMLInstanceIB) DriverMajor() int      { return 0 }
func (m *customMockNVMLInstanceIB) CUDAVersion() string   { return "" }
func (m *customMockNVMLInstanceIB) NVMLExists() bool      { return m.nvmlExists }
func (m *customMockNVMLInstanceIB) Library() lib.Library  { return nil }
func (m *customMockNVMLInstanceIB) Shutdown() error       { return nil }
func (m *customMockNVMLInstanceIB) InitError() error      { return m.initError }

// mockEventBucketIB implements eventstore.Bucket for testing
type mockEventBucketIB struct {
	events    eventstore.Events
	mu        sync.Mutex
	insertErr error
}

func (m *mockEventBucketIB) Name() string { return "mock-event-bucket" }
func (m *mockEventBucketIB) Insert(ctx context.Context, event eventstore.Event) error {
	if m.insertErr != nil {
		return m.insertErr
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, event)
	return nil
}
func (m *mockEventBucketIB) Find(ctx context.Context, event eventstore.Event) (*eventstore.Event, error) {
	return nil, nil
}
func (m *mockEventBucketIB) Get(ctx context.Context, since time.Time) (eventstore.Events, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.events, nil
}
func (m *mockEventBucketIB) Latest(ctx context.Context) (*eventstore.Event, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.events) == 0 {
		return nil, nil
	}
	return &m.events[len(m.events)-1], nil
}
func (m *mockEventBucketIB) Purge(ctx context.Context, beforeTimestamp int64) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = nil
	return 0, nil
}
func (m *mockEventBucketIB) Close() {}

// mockEventStoreIB implements eventstore.Store for testing
type mockEventStoreIB struct {
	bucket    eventstore.Bucket
	bucketErr error
}

func (m *mockEventStoreIB) Bucket(name string, opts ...eventstore.OpOption) (eventstore.Bucket, error) {
	if m.bucketErr != nil {
		return nil, m.bucketErr
	}
	return m.bucket, nil
}

// TestNew_WithMockey tests the New function using mockey for isolation
func TestNew_WithMockey(t *testing.T) {
	mockey.PatchConvey("New component creation with basic setup", t, func() {
		ctx := context.Background()
		mockInstance := &customMockNVMLInstanceIB{
			devs:        map[string]device.Device{},
			nvmlExists:  true,
			productName: "NVIDIA H100",
		}

		gpudInstance := &components.GPUdInstance{
			RootCtx:              ctx,
			NVMLInstance:         mockInstance,
			NVIDIAToolOverwrites: pkgconfigcommon.ToolOverwrites{},
		}

		c, err := New(gpudInstance)

		assert.NoError(t, err)
		assert.NotNil(t, c)
		assert.Equal(t, Name, c.Name())

		tc, ok := c.(*component)
		require.True(t, ok)
		assert.NotNil(t, tc.getTimeNowFunc)
		assert.NotNil(t, tc.getThresholdsFunc)
		assert.NotNil(t, tc.getClassDevicesFunc)
	})
}

// TestNew_WithExcludedDevices tests New with excluded IB devices
func TestNew_WithExcludedDevices(t *testing.T) {
	mockey.PatchConvey("New with excluded IB devices", t, func() {
		ctx := context.Background()
		mockInstance := &customMockNVMLInstanceIB{
			devs:        map[string]device.Device{},
			nvmlExists:  true,
			productName: "NVIDIA H100",
		}

		gpudInstance := &components.GPUdInstance{
			RootCtx:      ctx,
			NVMLInstance: mockInstance,
			NVIDIAToolOverwrites: pkgconfigcommon.ToolOverwrites{
				ExcludedInfinibandDevices: []string{"mlx5_0", "mlx5_1"},
			},
		}

		c, err := New(gpudInstance)

		assert.NoError(t, err)
		assert.NotNil(t, c)

		tc, ok := c.(*component)
		require.True(t, ok)
		assert.NotNil(t, tc.getClassDevicesFunc)
	})
}

// TestNew_WithEventStore tests New with event store only (no database)
func TestNew_WithEventStore(t *testing.T) {
	mockey.PatchConvey("New with event store", t, func() {
		ctx := context.Background()

		mockInstance := &customMockNVMLInstanceIB{
			devs:        map[string]device.Device{},
			nvmlExists:  true,
			productName: "NVIDIA H100",
		}

		mockBucket := &mockEventBucketIB{}
		mockStore := &mockEventStoreIB{bucket: mockBucket}

		gpudInstance := &components.GPUdInstance{
			RootCtx:              ctx,
			NVMLInstance:         mockInstance,
			NVIDIAToolOverwrites: pkgconfigcommon.ToolOverwrites{},
			EventStore:           mockStore,
		}

		c, err := New(gpudInstance)

		assert.NoError(t, err)
		assert.NotNil(t, c)

		tc, ok := c.(*component)
		require.True(t, ok)
		assert.NotNil(t, tc.eventBucket)
	})
}

// TestNew_EventStoreBucketError tests New when event store bucket creation fails
func TestNew_EventStoreBucketError(t *testing.T) {
	mockey.PatchConvey("New with event store bucket error", t, func() {
		ctx := context.Background()
		mockInstance := &customMockNVMLInstanceIB{
			devs:        map[string]device.Device{},
			nvmlExists:  true,
			productName: "NVIDIA H100",
		}

		mockStore := &mockEventStoreIB{bucketErr: errors.New("bucket creation failed")}

		gpudInstance := &components.GPUdInstance{
			RootCtx:              ctx,
			NVMLInstance:         mockInstance,
			NVIDIAToolOverwrites: pkgconfigcommon.ToolOverwrites{},
			EventStore:           mockStore,
		}

		c, err := New(gpudInstance)

		assert.Error(t, err)
		assert.Nil(t, c)
		assert.Contains(t, err.Error(), "bucket creation failed")
	})
}

// TestComponent_Tags tests the Tags method
func TestComponent_Tags_WithMockey(t *testing.T) {
	mockey.PatchConvey("Tags returns correct tags", t, func() {
		ctx := context.Background()
		cctx, cancel := context.WithCancel(ctx)
		defer cancel()

		comp := &component{
			ctx:    cctx,
			cancel: cancel,
		}

		tags := comp.Tags()

		assert.Contains(t, tags, "accelerator")
		assert.Contains(t, tags, "gpu")
		assert.Contains(t, tags, "nvidia")
		assert.Contains(t, tags, Name)
		assert.Len(t, tags, 4)
	})
}

// TestComponent_Start tests the Start method
func TestComponent_Start_WithMockey(t *testing.T) {
	mockey.PatchConvey("Start initiates check loop", t, func() {
		ctx := context.Background()
		cctx, cancel := context.WithCancel(ctx)
		defer cancel()

		checkCalled := make(chan struct{}, 10)
		mockNvml := &customMockNVMLInstanceIB{
			devs:        map[string]device.Device{},
			nvmlExists:  true,
			productName: "NVIDIA H100",
		}

		comp := &component{
			ctx:           cctx,
			cancel:        cancel,
			checkInterval: 10 * time.Millisecond, // Short interval for testing
			nvmlInstance:  mockNvml,
			getTimeNowFunc: func() time.Time {
				select {
				case checkCalled <- struct{}{}:
				default:
				}
				return time.Now().UTC()
			},
			getThresholdsFunc: func() types.ExpectedPortStates {
				// Zero threshold - Check() returns early but we detect it via getTimeNowFunc
				return types.ExpectedPortStates{}
			},
			getClassDevicesFunc: func(ignoreFiles map[string]struct{}) (infinibandclass.Devices, error) {
				return infinibandclass.Devices{}, nil
			},
			ignoreFiles: make(map[string]struct{}),
		}

		err := comp.Start()
		assert.NoError(t, err)

		// Wait for at least one check to be called (detected via getTimeNowFunc)
		select {
		case <-checkCalled:
			// Check was called successfully
		case <-time.After(100 * time.Millisecond):
			t.Error("Check was not called within expected time")
		}
	})
}

// TestComponent_Start_WithContextCancel tests Start respects context cancellation
func TestComponent_Start_WithContextCancel(t *testing.T) {
	mockey.PatchConvey("Start respects context cancellation", t, func() {
		ctx := context.Background()
		cctx, cancel := context.WithCancel(ctx)

		checkCount := 0
		mockNvml := &customMockNVMLInstanceIB{
			devs:        map[string]device.Device{},
			nvmlExists:  true,
			productName: "NVIDIA H100",
		}

		comp := &component{
			ctx:           cctx,
			cancel:        cancel,
			checkInterval: 5 * time.Millisecond,
			nvmlInstance:  mockNvml,
			getTimeNowFunc: func() time.Time {
				return time.Now().UTC()
			},
			getThresholdsFunc: func() types.ExpectedPortStates {
				return types.ExpectedPortStates{}
			},
			getClassDevicesFunc: func(ignoreFiles map[string]struct{}) (infinibandclass.Devices, error) {
				checkCount++
				return infinibandclass.Devices{}, nil
			},
			ignoreFiles: make(map[string]struct{}),
		}

		err := comp.Start()
		assert.NoError(t, err)

		// Let a few checks run
		time.Sleep(20 * time.Millisecond)

		// Cancel the context
		cancel()

		// Give time for goroutine to exit
		time.Sleep(20 * time.Millisecond)

		// Capture count after cancel
		finalCount := checkCount

		// Wait to ensure no more checks happen
		time.Sleep(30 * time.Millisecond)

		// Check count should not have increased significantly after cancel
		assert.LessOrEqual(t, checkCount, finalCount+1)
	})
}

// TestComponent_IsSupported_Variations tests IsSupported with various conditions
func TestComponent_IsSupported_Variations(t *testing.T) {
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
					customMock := &customMockNVMLInstanceIB{
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

// TestCheckResult_ComponentName tests ComponentName method
func TestCheckResult_ComponentName_WithMockey(t *testing.T) {
	t.Run("returns correct component name", func(t *testing.T) {
		cr := &checkResult{}
		assert.Equal(t, Name, cr.ComponentName())
	})
}

// TestCheckResult_Summary tests Summary method variations
func TestCheckResult_Summary_WithMockey(t *testing.T) {
	t.Run("nil check result returns empty string", func(t *testing.T) {
		var cr *checkResult
		assert.Equal(t, "", cr.Summary())
	})

	t.Run("with reason returns reason", func(t *testing.T) {
		cr := &checkResult{reason: "test reason"}
		assert.Equal(t, "test reason", cr.Summary())
	})

	t.Run("empty reason returns empty string", func(t *testing.T) {
		cr := &checkResult{reason: ""}
		assert.Equal(t, "", cr.Summary())
	})
}

// TestCheckResult_HealthStateType tests HealthStateType method variations
func TestCheckResult_HealthStateType_WithMockey(t *testing.T) {
	t.Run("nil check result returns empty", func(t *testing.T) {
		var cr *checkResult
		assert.Equal(t, apiv1.HealthStateType(""), cr.HealthStateType())
	})

	t.Run("healthy state returns healthy", func(t *testing.T) {
		cr := &checkResult{health: apiv1.HealthStateTypeHealthy}
		assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.HealthStateType())
	})

	t.Run("unhealthy state returns unhealthy", func(t *testing.T) {
		cr := &checkResult{health: apiv1.HealthStateTypeUnhealthy}
		assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.HealthStateType())
	})
}

// TestCheckResult_getError tests getError method variations
func TestCheckResult_getError_WithMockey(t *testing.T) {
	t.Run("nil check result returns empty string", func(t *testing.T) {
		var cr *checkResult
		assert.Equal(t, "", cr.getError())
	})

	t.Run("with error returns error string", func(t *testing.T) {
		cr := &checkResult{err: errors.New("test error")}
		assert.Equal(t, "test error", cr.getError())
	})

	t.Run("nil error returns empty string", func(t *testing.T) {
		cr := &checkResult{err: nil}
		assert.Equal(t, "", cr.getError())
	})
}

// TestCheckResult_getSuggestedActions tests getSuggestedActions method
func TestCheckResult_getSuggestedActions_WithMockey(t *testing.T) {
	t.Run("nil check result returns nil", func(t *testing.T) {
		var cr *checkResult
		assert.Nil(t, cr.getSuggestedActions())
	})

	t.Run("with suggested actions returns actions", func(t *testing.T) {
		actions := &apiv1.SuggestedActions{
			RepairActions: []apiv1.RepairActionType{apiv1.RepairActionTypeRebootSystem},
		}
		cr := &checkResult{suggestedActions: actions}
		assert.Equal(t, actions, cr.getSuggestedActions())
	})

	t.Run("nil suggested actions returns nil", func(t *testing.T) {
		cr := &checkResult{suggestedActions: nil}
		assert.Nil(t, cr.getSuggestedActions())
	})
}

// TestCheckResult_HealthStates tests HealthStates method variations
func TestCheckResult_HealthStates_WithMockey(t *testing.T) {
	t.Run("nil check result returns default healthy state", func(t *testing.T) {
		var cr *checkResult
		states := cr.HealthStates()
		require.Len(t, states, 1)
		assert.Equal(t, apiv1.HealthStateTypeHealthy, states[0].Health)
		assert.Equal(t, "no data yet", states[0].Reason)
		assert.Equal(t, Name, states[0].Component)
	})

	t.Run("with all fields set", func(t *testing.T) {
		now := time.Now()
		actions := &apiv1.SuggestedActions{
			RepairActions: []apiv1.RepairActionType{apiv1.RepairActionTypeHardwareInspection},
		}
		cr := &checkResult{
			ts:               now,
			health:           apiv1.HealthStateTypeUnhealthy,
			reason:           "port down",
			err:              errors.New("connection error"),
			suggestedActions: actions,
		}

		states := cr.HealthStates()
		require.Len(t, states, 1)
		assert.Equal(t, apiv1.HealthStateTypeUnhealthy, states[0].Health)
		assert.Equal(t, "port down", states[0].Reason)
		assert.Equal(t, "connection error", states[0].Error)
		assert.NotNil(t, states[0].SuggestedActions)
		assert.Contains(t, states[0].SuggestedActions.RepairActions, apiv1.RepairActionTypeHardwareInspection)
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
			getThresholdsFunc: func() types.ExpectedPortStates {
				// Both AtLeastPorts AND AtLeastRate must be > 0 to pass IsZero() check
				return types.ExpectedPortStates{AtLeastPorts: 1, AtLeastRate: 100}
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

		mockInst := &customMockNVMLInstanceIB{
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
			getThresholdsFunc: func() types.ExpectedPortStates {
				// Both AtLeastPorts AND AtLeastRate must be > 0 to pass IsZero() check
				return types.ExpectedPortStates{AtLeastPorts: 1, AtLeastRate: 100}
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

// TestCheck_InitError tests Check when NVML has an initialization error
func TestCheck_InitError_WithMockey(t *testing.T) {
	mockey.PatchConvey("Check with NVML init error", t, func() {
		ctx := context.Background()
		cctx, cancel := context.WithCancel(ctx)
		defer cancel()

		initErr := errors.New("error getting device handle for index '0': Unknown Error")
		mockInst := &customMockNVMLInstanceIB{
			devs:        map[string]device.Device{},
			nvmlExists:  true,
			productName: "NVIDIA H100",
			initError:   initErr,
		}

		comp := &component{
			ctx:    cctx,
			cancel: cancel,
			getTimeNowFunc: func() time.Time {
				return time.Now().UTC()
			},
			getThresholdsFunc: func() types.ExpectedPortStates {
				// Both AtLeastPorts AND AtLeastRate must be > 0 to pass IsZero() check
				return types.ExpectedPortStates{AtLeastPorts: 1, AtLeastRate: 100}
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

		mockInst := &customMockNVMLInstanceIB{
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
			getThresholdsFunc: func() types.ExpectedPortStates {
				// Both AtLeastPorts AND AtLeastRate must be > 0 to pass IsZero() check
				return types.ExpectedPortStates{AtLeastPorts: 1, AtLeastRate: 100}
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

// TestCheck_ClassDevicesError tests Check when loading class devices fails
func TestCheck_ClassDevicesError_WithMockey(t *testing.T) {
	mockey.PatchConvey("Check with class devices error", t, func() {
		ctx := context.Background()
		cctx, cancel := context.WithCancel(ctx)
		defer cancel()

		mockInst := &customMockNVMLInstanceIB{
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
			getThresholdsFunc: func() types.ExpectedPortStates {
				return types.ExpectedPortStates{AtLeastPorts: 1, AtLeastRate: 100}
			},
			getClassDevicesFunc: func(ignoreFiles map[string]struct{}) (infinibandclass.Devices, error) {
				return nil, errors.New("failed to load class devices")
			},
			nvmlInstance: mockInst,
			ignoreFiles:  make(map[string]struct{}),
		}

		result := comp.Check()
		cr, ok := result.(*checkResult)
		require.True(t, ok)

		assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
		assert.Contains(t, cr.reason, "error loading infiniband class devices")
		assert.NotNil(t, cr.err)
	})
}

// TestCheck_ConcurrentAccess tests concurrent access to Check and LastHealthStates
func TestCheck_ConcurrentAccess_WithMockey(t *testing.T) {
	mockey.PatchConvey("Concurrent Check and LastHealthStates access", t, func() {
		ctx := context.Background()
		cctx, cancel := context.WithCancel(ctx)
		defer cancel()

		mockInst := &customMockNVMLInstanceIB{
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
			getThresholdsFunc: func() types.ExpectedPortStates {
				return types.ExpectedPortStates{} // Zero threshold
			},
			getClassDevicesFunc: func(ignoreFiles map[string]struct{}) (infinibandclass.Devices, error) {
				return infinibandclass.Devices{}, nil
			},
			nvmlInstance: mockInst,
			ignoreFiles:  make(map[string]struct{}),
		}

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

// TestCheckResult_String_WithDevices tests String method with class devices
func TestCheckResult_String_WithDevices_WithMockey(t *testing.T) {
	t.Run("nil check result returns empty string", func(t *testing.T) {
		var cr *checkResult
		assert.Equal(t, "", cr.String())
	})

	t.Run("empty check result returns empty string", func(t *testing.T) {
		cr := &checkResult{}
		assert.Equal(t, "", cr.String())
	})

	t.Run("with class devices renders table", func(t *testing.T) {
		cr := &checkResult{
			ClassDevices: infinibandclass.Devices{
				{
					Name:            "mlx5_0",
					BoardID:         "MT_0000000838",
					FirmwareVersion: "28.41.1000",
					HCAType:         "MT4129",
				},
			},
		}
		result := cr.String()
		assert.Contains(t, result, "mlx5_0")
	})
}

// TestLastHealthStates_SuggestedActionsPropagate tests suggested actions propagation
func TestLastHealthStates_SuggestedActionsPropagate_WithMockey(t *testing.T) {
	mockey.PatchConvey("Suggested actions propagate to health states", t, func() {
		ctx := context.Background()
		cctx, cancel := context.WithCancel(ctx)
		defer cancel()

		mockInst := &customMockNVMLInstanceIB{
			devs:        map[string]device.Device{},
			nvmlExists:  true,
			productName: "NVIDIA H100",
			initError:   errors.New("init error"),
		}

		comp := &component{
			ctx:    cctx,
			cancel: cancel,
			getTimeNowFunc: func() time.Time {
				return time.Now().UTC()
			},
			getThresholdsFunc: func() types.ExpectedPortStates {
				// Both AtLeastPorts AND AtLeastRate must be > 0 to pass IsZero() check
				return types.ExpectedPortStates{AtLeastPorts: 1, AtLeastRate: 100}
			},
			nvmlInstance: mockInst,
		}

		// Trigger check to populate lastCheckResult
		comp.Check()

		states := comp.LastHealthStates()
		require.Len(t, states, 1)
		assert.Equal(t, apiv1.HealthStateTypeUnhealthy, states[0].Health)
		assert.NotNil(t, states[0].SuggestedActions)
		assert.Contains(t, states[0].SuggestedActions.RepairActions, apiv1.RepairActionTypeRebootSystem)
	})
}

// TestComponent_Close_WithComponents tests Close with various components set
func TestComponent_Close_WithComponents_WithMockey(t *testing.T) {
	mockey.PatchConvey("Close with event bucket", t, func() {
		ctx := context.Background()
		cctx, cancel := context.WithCancel(ctx)

		mockBucket := &mockEventBucketIB{}

		comp := &component{
			ctx:         cctx,
			cancel:      cancel,
			eventBucket: mockBucket,
			kmsgSyncer:  nil,
		}

		err := comp.Close()
		assert.NoError(t, err)

		// Verify context is canceled
		select {
		case <-cctx.Done():
			// Success
		default:
			t.Error("Context was not canceled")
		}
	})
}

// TestComponent_Events_NilBucket tests Events with nil event bucket
func TestComponent_Events_NilBucket_WithMockey(t *testing.T) {
	mockey.PatchConvey("Events with nil bucket returns nil", t, func() {
		comp := &component{
			eventBucket: nil,
		}

		events, err := comp.Events(context.Background(), time.Now())
		assert.NoError(t, err)
		assert.Nil(t, events)
	})
}
