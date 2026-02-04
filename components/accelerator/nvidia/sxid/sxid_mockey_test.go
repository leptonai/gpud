//go:build linux

package sxid

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/bytedance/mockey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/eventstore"
	"github.com/leptonai/gpud/pkg/kmsg"
	"github.com/leptonai/gpud/pkg/nvidia/nvml/device"
	nvmllib "github.com/leptonai/gpud/pkg/nvidia/nvml/lib"
	nvidiaproduct "github.com/leptonai/gpud/pkg/nvidia/product"
)

// customMockNVMLInstanceSXID implements the nvml.Instance interface for testing with customizable behavior
type customMockNVMLInstanceSXID struct {
	devs        map[string]device.Device
	nvmlExists  bool
	productName string
	initError   error
}

func (m *customMockNVMLInstanceSXID) Devices() map[string]device.Device { return m.devs }
func (m *customMockNVMLInstanceSXID) FabricManagerSupported() bool      { return true }
func (m *customMockNVMLInstanceSXID) FabricStateSupported() bool        { return false }
func (m *customMockNVMLInstanceSXID) GetMemoryErrorManagementCapabilities() nvidiaproduct.MemoryErrorManagementCapabilities {
	return nvidiaproduct.MemoryErrorManagementCapabilities{}
}
func (m *customMockNVMLInstanceSXID) ProductName() string      { return m.productName }
func (m *customMockNVMLInstanceSXID) Architecture() string     { return "" }
func (m *customMockNVMLInstanceSXID) Brand() string            { return "" }
func (m *customMockNVMLInstanceSXID) DriverVersion() string    { return "" }
func (m *customMockNVMLInstanceSXID) DriverMajor() int         { return 0 }
func (m *customMockNVMLInstanceSXID) CUDAVersion() string      { return "" }
func (m *customMockNVMLInstanceSXID) NVMLExists() bool         { return m.nvmlExists }
func (m *customMockNVMLInstanceSXID) Library() nvmllib.Library { return nil }
func (m *customMockNVMLInstanceSXID) Shutdown() error          { return nil }
func (m *customMockNVMLInstanceSXID) InitError() error         { return m.initError }

// mockEventBucketSXID implements eventstore.Bucket for testing
type mockEventBucketSXID struct {
	events    eventstore.Events
	getError  error
	insertErr error
	findEvent *eventstore.Event
	findErr   error
	purgeErr  error
}

func (m *mockEventBucketSXID) Name() string { return "mock-sxid-bucket" }
func (m *mockEventBucketSXID) Insert(ctx context.Context, event eventstore.Event) error {
	if m.insertErr != nil {
		return m.insertErr
	}
	m.events = append(m.events, event)
	return nil
}
func (m *mockEventBucketSXID) Find(ctx context.Context, event eventstore.Event) (*eventstore.Event, error) {
	if m.findErr != nil {
		return nil, m.findErr
	}
	return m.findEvent, nil
}
func (m *mockEventBucketSXID) Get(ctx context.Context, since time.Time) (eventstore.Events, error) {
	if m.getError != nil {
		return nil, m.getError
	}
	return m.events, nil
}
func (m *mockEventBucketSXID) Latest(ctx context.Context) (*eventstore.Event, error) {
	if len(m.events) == 0 {
		return nil, nil
	}
	return &m.events[len(m.events)-1], nil
}
func (m *mockEventBucketSXID) Purge(ctx context.Context, beforeTimestamp int64) (int, error) {
	if m.purgeErr != nil {
		return 0, m.purgeErr
	}
	m.events = nil
	return 0, nil
}
func (m *mockEventBucketSXID) Close() {}

// mockRebootEventStoreSXID implements pkghost.RebootEventStore for testing
type mockRebootEventStoreSXID struct {
	rebootEvents eventstore.Events
	getErr       error
}

func (m *mockRebootEventStoreSXID) GetRebootEvents(ctx context.Context, since time.Time) (eventstore.Events, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	return m.rebootEvents, nil
}

func (m *mockRebootEventStoreSXID) RecordReboot(ctx context.Context) error {
	return nil
}

// mockKmsgWatcherSXID implements kmsg.Watcher for testing
type mockKmsgWatcherSXID struct {
	watchCh    chan kmsg.Message
	watchErr   error
	closeErr   error
	closeCalls int
}

func (m *mockKmsgWatcherSXID) Watch() (<-chan kmsg.Message, error) {
	if m.watchErr != nil {
		return nil, m.watchErr
	}
	if m.watchCh == nil {
		m.watchCh = make(chan kmsg.Message)
	}
	return m.watchCh, nil
}

func (m *mockKmsgWatcherSXID) Close() error {
	m.closeCalls++
	if m.watchCh != nil {
		close(m.watchCh)
		m.watchCh = nil
	}
	return m.closeErr
}

// TestNew_WithMockey tests the New function using mockey for isolation.
func TestNew_WithMockey(t *testing.T) {
	mockey.PatchConvey("New component creation with NVMLInstance", t, func() {
		ctx := context.Background()

		mockInstance := &customMockNVMLInstanceSXID{
			devs:        map[string]device.Device{"gpu-1": nil},
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
		assert.NotNil(t, tc.nvmlInstance)
	})
}

// TestNew_NilNVMLInstance_WithMockey tests New with nil NVML instance.
func TestNew_NilNVMLInstance_WithMockey(t *testing.T) {
	mockey.PatchConvey("New component creation with nil NVMLInstance", t, func() {
		ctx := context.Background()

		gpudInstance := &components.GPUdInstance{
			RootCtx:      ctx,
			NVMLInstance: nil,
		}

		c, err := New(gpudInstance)

		assert.NoError(t, err)
		assert.NotNil(t, c)

		tc, ok := c.(*component)
		require.True(t, ok)
		assert.Nil(t, tc.nvmlInstance)
	})
}

// TestComponent_IsSupported_WithMockey tests IsSupported method with various conditions.
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
					customMock := &customMockNVMLInstanceSXID{
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

// TestCheck_InitError_WithMockey tests Check when NVML has an initialization error.
func TestCheck_InitError_WithMockey(t *testing.T) {
	mockey.PatchConvey("Check with NVML init error", t, func() {
		ctx := context.Background()
		cctx, cancel := context.WithCancel(ctx)
		defer cancel()

		initErr := errors.New("error getting device handle for index '0': Unknown Error")
		mockInst := &customMockNVMLInstanceSXID{
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

// TestCheck_MissingProductName_WithMockey tests Check when product name is empty.
func TestCheck_MissingProductName_WithMockey(t *testing.T) {
	mockey.PatchConvey("Check with missing product name", t, func() {
		ctx := context.Background()
		cctx, cancel := context.WithCancel(ctx)
		defer cancel()

		mockInst := &customMockNVMLInstanceSXID{
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

// TestCheck_NilNVMLInstance_WithMockey tests Check with nil NVML instance.
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

// TestCheck_NVMLNotExists_WithMockey tests Check when NVML library is not loaded.
func TestCheck_NVMLNotExists_WithMockey(t *testing.T) {
	mockey.PatchConvey("Check with NVML not exists", t, func() {
		ctx := context.Background()
		cctx, cancel := context.WithCancel(ctx)
		defer cancel()

		mockInst := &customMockNVMLInstanceSXID{
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

// TestCheck_NilKmsgReader_WithMockey tests Check when kmsg reader is nil.
func TestCheck_NilKmsgReader_WithMockey(t *testing.T) {
	mockey.PatchConvey("Check with nil kmsg reader", t, func() {
		ctx := context.Background()
		cctx, cancel := context.WithCancel(ctx)
		defer cancel()

		mockInst := &customMockNVMLInstanceSXID{
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
			readAllKmsg:  nil, // nil reader
		}

		result := comp.Check()
		cr, ok := result.(*checkResult)
		require.True(t, ok)

		assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
		assert.Contains(t, cr.reason, "kmsg reader is not set")
	})
}

// TestCheck_ReadKmsgError_WithMockey tests Check when reading kmsg fails.
func TestCheck_ReadKmsgError_WithMockey(t *testing.T) {
	mockey.PatchConvey("Check with kmsg read error", t, func() {
		ctx := context.Background()
		cctx, cancel := context.WithCancel(ctx)
		defer cancel()

		mockInst := &customMockNVMLInstanceSXID{
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
			readAllKmsg: func(ctx context.Context) ([]kmsg.Message, error) {
				return nil, errors.New("failed to read kmsg: permission denied")
			},
		}

		result := comp.Check()
		cr, ok := result.(*checkResult)
		require.True(t, ok)

		assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
		assert.Contains(t, cr.reason, "failed to read kmsg")
		assert.NotNil(t, cr.err)
	})
}

// TestCheck_WithSXidErrors_WithMockey tests Check when SXID errors are found.
func TestCheck_WithSXidErrors_WithMockey(t *testing.T) {
	mockey.PatchConvey("Check with SXID errors", t, func() {
		ctx := context.Background()
		cctx, cancel := context.WithCancel(ctx)
		defer cancel()

		mockInst := &customMockNVMLInstanceSXID{
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
			readAllKmsg: func(ctx context.Context) ([]kmsg.Message, error) {
				return []kmsg.Message{
					{
						Timestamp: metav1.NewTime(time.Now()),
						Message:   "nvidia-nvswitch3: SXid (PCI:0000:05:00.0): 12028, Non-fatal, Link 32 egress non-posted PRIV error (First)",
					},
				}, nil
			},
		}

		result := comp.Check()
		cr, ok := result.(*checkResult)
		require.True(t, ok)

		assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
		assert.Len(t, cr.FoundErrors, 1)
		assert.Equal(t, 12028, cr.FoundErrors[0].SXid)
	})
}

// TestCheck_NoSXidErrors_WithMockey tests Check when no SXID errors are found.
func TestCheck_NoSXidErrors_WithMockey(t *testing.T) {
	mockey.PatchConvey("Check with no SXID errors", t, func() {
		ctx := context.Background()
		cctx, cancel := context.WithCancel(ctx)
		defer cancel()

		mockInst := &customMockNVMLInstanceSXID{
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
			readAllKmsg: func(ctx context.Context) ([]kmsg.Message, error) {
				return []kmsg.Message{
					{
						Timestamp: metav1.NewTime(time.Now()),
						Message:   "some other kernel message that is not an SXID error",
					},
				}, nil
			},
		}

		result := comp.Check()
		cr, ok := result.(*checkResult)
		require.True(t, ok)

		assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
		assert.Len(t, cr.FoundErrors, 0)
	})
}

// TestCheckResult_Methods_WithMockey tests checkResult methods.
func TestCheckResult_Methods_WithMockey(t *testing.T) {
	t.Run("ComponentName", func(t *testing.T) {
		cr := &checkResult{}
		assert.Equal(t, Name, cr.ComponentName())
	})

	t.Run("String nil", func(t *testing.T) {
		var cr *checkResult
		assert.Equal(t, "", cr.String())
	})

	t.Run("String no errors", func(t *testing.T) {
		cr := &checkResult{FoundErrors: []FoundError{}}
		assert.Equal(t, "no sxid error found", cr.String())
	})

	t.Run("Summary nil", func(t *testing.T) {
		var cr *checkResult
		assert.Equal(t, "", cr.Summary())
	})

	t.Run("Summary with reason", func(t *testing.T) {
		cr := &checkResult{reason: "test reason"}
		assert.Equal(t, "test reason", cr.Summary())
	})

	t.Run("HealthStateType nil", func(t *testing.T) {
		var cr *checkResult
		assert.Equal(t, apiv1.HealthStateType(""), cr.HealthStateType())
	})

	t.Run("HealthStateType healthy", func(t *testing.T) {
		cr := &checkResult{health: apiv1.HealthStateTypeHealthy}
		assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.HealthStateType())
	})

	t.Run("HealthStates nil result", func(t *testing.T) {
		var cr *checkResult
		states := cr.HealthStates()
		require.Len(t, states, 1)
		assert.Equal(t, apiv1.HealthStateTypeHealthy, states[0].Health)
		assert.Equal(t, "no data yet", states[0].Reason)
	})

	t.Run("HealthStates with error", func(t *testing.T) {
		cr := &checkResult{
			ts:     time.Now(),
			health: apiv1.HealthStateTypeUnhealthy,
			reason: "test error",
			err:    errors.New("test error details"),
		}
		states := cr.HealthStates()
		require.Len(t, states, 1)
		assert.Equal(t, apiv1.HealthStateTypeUnhealthy, states[0].Health)
		assert.Equal(t, "test error details", states[0].Error)
	})

	t.Run("getError nil result", func(t *testing.T) {
		var cr *checkResult
		assert.Equal(t, "", cr.getError())
	})

	t.Run("getError nil error", func(t *testing.T) {
		cr := &checkResult{}
		assert.Equal(t, "", cr.getError())
	})

	t.Run("getError with error", func(t *testing.T) {
		cr := &checkResult{err: errors.New("test error")}
		assert.Equal(t, "test error", cr.getError())
	})
}

// TestClose_WithMockey tests the Close method.
func TestClose_WithMockey(t *testing.T) {
	mockey.PatchConvey("Close method", t, func() {
		ctx := context.Background()
		cctx, cancel := context.WithCancel(ctx)

		mockWatcher := &mockKmsgWatcherSXID{
			watchCh: make(chan kmsg.Message),
		}
		mockBucket := &mockEventBucketSXID{}

		comp := &component{
			ctx:         cctx,
			cancel:      cancel,
			kmsgWatcher: mockWatcher,
			eventBucket: mockBucket,
		}

		err := comp.Close()
		assert.NoError(t, err)
		assert.Equal(t, 1, mockWatcher.closeCalls)
	})
}

// TestClose_WithKmsgWatcherError_WithMockey tests Close when kmsg watcher returns an error.
func TestClose_WithKmsgWatcherError_WithMockey(t *testing.T) {
	mockey.PatchConvey("Close with kmsg watcher error", t, func() {
		ctx := context.Background()
		cctx, cancel := context.WithCancel(ctx)

		mockWatcher := &mockKmsgWatcherSXID{
			watchCh:  make(chan kmsg.Message),
			closeErr: errors.New("close error"),
		}
		mockBucket := &mockEventBucketSXID{}

		comp := &component{
			ctx:         cctx,
			cancel:      cancel,
			kmsgWatcher: mockWatcher,
			eventBucket: mockBucket,
		}

		err := comp.Close()
		// Close should not return an error even if kmsgWatcher.Close fails (it just logs)
		assert.NoError(t, err)
		assert.Equal(t, 1, mockWatcher.closeCalls)
	})
}

// TestClose_NilComponents_WithMockey tests Close with nil kmsgWatcher and eventBucket.
func TestClose_NilComponents_WithMockey(t *testing.T) {
	mockey.PatchConvey("Close with nil components", t, func() {
		ctx := context.Background()
		cctx, cancel := context.WithCancel(ctx)

		comp := &component{
			ctx:         cctx,
			cancel:      cancel,
			kmsgWatcher: nil,
			eventBucket: nil,
		}

		err := comp.Close()
		assert.NoError(t, err)
	})
}

// TestEvents_NilBucket_WithMockey tests Events method with nil eventBucket.
func TestEvents_NilBucket_WithMockey(t *testing.T) {
	mockey.PatchConvey("Events with nil bucket", t, func() {
		ctx := context.Background()
		cctx, cancel := context.WithCancel(ctx)
		defer cancel()

		comp := &component{
			ctx:         cctx,
			cancel:      cancel,
			eventBucket: nil,
		}

		events, err := comp.Events(ctx, time.Now().Add(-1*time.Hour))
		assert.NoError(t, err)
		assert.Nil(t, events)
	})
}

// TestTranslateToStateHealth_WithMockey tests the translateToStateHealth function.
func TestTranslateToStateHealth_WithMockey(t *testing.T) {
	testCases := []struct {
		name     string
		health   int
		expected apiv1.HealthStateType
	}{
		{"healthy", healthStateHealthy, apiv1.HealthStateTypeHealthy},
		{"degraded", healthStateDegraded, apiv1.HealthStateTypeDegraded},
		{"unhealthy", healthStateUnhealthy, apiv1.HealthStateTypeUnhealthy},
		{"unknown defaults to healthy", 999, apiv1.HealthStateTypeHealthy},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := translateToStateHealth(tc.health)
			assert.Equal(t, tc.expected, result)
		})
	}
}

// TestGetDetail_WithMockey tests the GetDetail function.
func TestGetDetail_WithMockey(t *testing.T) {
	// Test known SXID
	detail, ok := GetDetail(12028)
	assert.True(t, ok)
	assert.NotNil(t, detail)
	assert.Equal(t, 12028, detail.SXid)

	// Test unknown SXID
	_, ok = GetDetail(99999)
	assert.False(t, ok)
}

// TestEvolveHealthyState_NoEvents_WithMockey tests evolveHealthyState with no events.
func TestEvolveHealthyState_NoEvents_WithMockey(t *testing.T) {
	mockey.PatchConvey("evolveHealthyState with no events", t, func() {
		events := eventstore.Events{}
		state := evolveHealthyState(events)
		assert.Equal(t, apiv1.HealthStateTypeHealthy, state.Health)
		assert.Equal(t, "SXIDComponent is healthy", state.Reason)
	})
}

// TestEvolveHealthyState_WithFatalEvent_WithMockey tests evolveHealthyState with fatal events.
func TestEvolveHealthyState_WithFatalEvent_WithMockey(t *testing.T) {
	mockey.PatchConvey("evolveHealthyState with fatal event", t, func() {
		events := eventstore.Events{
			createSXidEvent(time.Now(), 11004, apiv1.EventTypeFatal, apiv1.RepairActionTypeRebootSystem),
		}
		state := evolveHealthyState(events)
		assert.Equal(t, apiv1.HealthStateTypeUnhealthy, state.Health)
	})
}

// TestEvolveHealthyState_WithCriticalEvent_WithMockey tests evolveHealthyState with critical events.
func TestEvolveHealthyState_WithCriticalEvent_WithMockey(t *testing.T) {
	mockey.PatchConvey("evolveHealthyState with critical event", t, func() {
		events := eventstore.Events{
			createSXidEvent(time.Now(), 11012, apiv1.EventTypeCritical, apiv1.RepairActionTypeCheckUserAppAndGPU),
		}
		state := evolveHealthyState(events)
		// Critical events result in degraded state
		assert.Equal(t, apiv1.HealthStateTypeDegraded, state.Health)
	})
}

// TestEvolveHealthyState_RebootClearsAction_WithMockey tests that reboot clears RebootSystem action.
func TestEvolveHealthyState_RebootClearsAction_WithMockey(t *testing.T) {
	mockey.PatchConvey("reboot clears RebootSystem action", t, func() {
		events := eventstore.Events{
			{Name: "reboot", Time: time.Now()},
			createSXidEvent(time.Now().Add(-1*time.Hour), 11004, apiv1.EventTypeFatal, apiv1.RepairActionTypeRebootSystem),
		}
		state := evolveHealthyState(events)
		assert.Equal(t, apiv1.HealthStateTypeHealthy, state.Health)
	})
}

// TestEvolveHealthyState_RebootClearsCheckUserAction_WithMockey tests that reboot clears CheckUserAppAndGPU action.
func TestEvolveHealthyState_RebootClearsCheckUserAction_WithMockey(t *testing.T) {
	mockey.PatchConvey("reboot clears CheckUserAppAndGPU action", t, func() {
		events := eventstore.Events{
			{Name: "reboot", Time: time.Now()},
			createSXidEvent(time.Now().Add(-1*time.Hour), 11012, apiv1.EventTypeCritical, apiv1.RepairActionTypeCheckUserAppAndGPU),
		}
		state := evolveHealthyState(events)
		assert.Equal(t, apiv1.HealthStateTypeHealthy, state.Health)
	})
}

// TestEvolveHealthyState_RebootDoesNotClearHardwareInspection_WithMockey tests that reboot does not clear HardwareInspection.
func TestEvolveHealthyState_RebootDoesNotClearHardwareInspection_WithMockey(t *testing.T) {
	mockey.PatchConvey("reboot does not clear HardwareInspection", t, func() {
		events := eventstore.Events{
			{Name: "reboot", Time: time.Now()},
			createSXidEvent(time.Now().Add(-1*time.Hour), 11004, apiv1.EventTypeFatal, apiv1.RepairActionTypeHardwareInspection),
		}
		state := evolveHealthyState(events)
		// Should remain unhealthy because HardwareInspection is not cleared by reboot
		assert.Equal(t, apiv1.HealthStateTypeUnhealthy, state.Health)
		require.NotNil(t, state.SuggestedActions)
		assert.Equal(t, apiv1.RepairActionTypeHardwareInspection, state.SuggestedActions.RepairActions[0])
	})
}

// TestEvolveHealthyState_RebootThresholdExceeded_WithMockey tests that after multiple reboots, action changes to HardwareInspection.
func TestEvolveHealthyState_RebootThresholdExceeded_WithMockey(t *testing.T) {
	mockey.PatchConvey("reboot threshold exceeded changes to HardwareInspection", t, func() {
		// Oldest events first (as they appear in time order)
		now := time.Now()
		events := eventstore.Events{
			// Most recent first (descending time order)
			createSXidEvent(now, 11004, apiv1.EventTypeFatal, apiv1.RepairActionTypeRebootSystem),
			{Name: "reboot", Time: now.Add(-10 * time.Minute)},
			createSXidEvent(now.Add(-20*time.Minute), 11004, apiv1.EventTypeFatal, apiv1.RepairActionTypeRebootSystem),
			{Name: "reboot", Time: now.Add(-30 * time.Minute)},
			createSXidEvent(now.Add(-40*time.Minute), 11004, apiv1.EventTypeFatal, apiv1.RepairActionTypeRebootSystem),
			{Name: "reboot", Time: now.Add(-50 * time.Minute)},
			createSXidEvent(now.Add(-60*time.Minute), 11004, apiv1.EventTypeFatal, apiv1.RepairActionTypeRebootSystem),
		}
		state := evolveHealthyState(events)
		// After exceeding reboot threshold, suggested action should change to HardwareInspection
		assert.Equal(t, apiv1.HealthStateTypeUnhealthy, state.Health)
		require.NotNil(t, state.SuggestedActions)
		assert.Equal(t, apiv1.RepairActionTypeHardwareInspection, state.SuggestedActions.RepairActions[0])
	})
}

// TestEvolveHealthyState_WithInvalidJSON_WithMockey tests evolveHealthyState with invalid JSON.
func TestEvolveHealthyState_WithInvalidJSON_WithMockey(t *testing.T) {
	mockey.PatchConvey("evolveHealthyState with invalid JSON", t, func() {
		events := eventstore.Events{
			{
				Name: EventNameErrorSXid,
				Type: string(apiv1.EventTypeFatal),
				ExtraInfo: map[string]string{
					EventKeyErrorSXidData: "invalid json {{{",
				},
			},
		}
		state := evolveHealthyState(events)
		// Should remain healthy since the event cannot be parsed
		assert.Equal(t, apiv1.HealthStateTypeHealthy, state.Health)
	})
}

// TestResolveSXIDEvent_WithNilExtraInfo_WithMockey tests resolveSXIDEvent with nil ExtraInfo.
func TestResolveSXIDEvent_WithNilExtraInfo_WithMockey(t *testing.T) {
	mockey.PatchConvey("resolveSXIDEvent with nil ExtraInfo", t, func() {
		event := eventstore.Event{
			Name:      EventNameErrorSXid,
			Time:      time.Now(),
			ExtraInfo: nil,
		}

		result := resolveSXIDEvent(event)

		// Should return the original event unchanged
		assert.Equal(t, event, result)
	})
}

// TestResolveSXIDEvent_WithLegacyFormat_WithMockey tests resolveSXIDEvent with legacy SXID format.
func TestResolveSXIDEvent_WithLegacyFormat_WithMockey(t *testing.T) {
	mockey.PatchConvey("resolveSXIDEvent with legacy format", t, func() {
		event := eventstore.Event{
			Name: EventNameErrorSXid,
			Time: time.Now(),
			ExtraInfo: map[string]string{
				EventKeyErrorSXidData: "12028",
				EventKeyDeviceUUID:    "PCI:0000:01:00",
			},
		}

		result := resolveSXIDEvent(event)

		assert.Contains(t, result.Message, "SXID 12028")
		assert.Contains(t, result.Message, "PCI:0000:01:00")
	})
}

// TestResolveSXIDEvent_WithUnknownSXID_WithMockey tests resolveSXIDEvent with unknown SXID.
func TestResolveSXIDEvent_WithUnknownSXID_WithMockey(t *testing.T) {
	mockey.PatchConvey("resolveSXIDEvent with unknown SXID", t, func() {
		event := eventstore.Event{
			Name: EventNameErrorSXid,
			Time: time.Now(),
			ExtraInfo: map[string]string{
				EventKeyErrorSXidData: "99999",
				EventKeyDeviceUUID:    "PCI:0000:01:00",
			},
		}

		result := resolveSXIDEvent(event)

		// Should return original event when SXID is unknown
		assert.Equal(t, event, result)
	})
}

// TestStart_ContextCanceled_WithMockey tests Start when context is canceled.
func TestStart_ContextCanceled_WithMockey(t *testing.T) {
	mockey.PatchConvey("Start with canceled context", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		mockBucket := &mockEventBucketSXID{}
		mockReboot := &mockRebootEventStoreSXID{}

		comp := &component{
			ctx:              ctx,
			cancel:           cancel,
			eventBucket:      mockBucket,
			rebootEventStore: mockReboot,
			getTimeNowFunc: func() time.Time {
				return time.Now().UTC()
			},
		}

		// Start should exit cleanly when context is canceled
		err := comp.Start()
		assert.NoError(t, err)
	})
}

// TestStart_WithKmsgWatcherError_WithMockey tests Start when kmsg watcher fails to watch.
func TestStart_WithKmsgWatcherError_WithMockey(t *testing.T) {
	mockey.PatchConvey("Start with kmsg watcher error", t, func() {
		ctx := context.Background()
		cctx, cancel := context.WithCancel(ctx)
		defer cancel()

		mockBucket := &mockEventBucketSXID{}
		mockReboot := &mockRebootEventStoreSXID{}
		mockWatcher := &mockKmsgWatcherSXID{
			watchErr: errors.New("watch error"),
		}

		comp := &component{
			ctx:              cctx,
			cancel:           cancel,
			eventBucket:      mockBucket,
			rebootEventStore: mockReboot,
			kmsgWatcher:      mockWatcher,
			getTimeNowFunc: func() time.Time {
				return time.Now().UTC()
			},
		}

		// Start should return the error from kmsgWatcher.Watch
		err := comp.Start()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "watch error")
	})
}

// TestStart_WithKmsgMessages_WithMockey tests start processing kmsg messages.
func TestStart_WithKmsgMessages_WithMockey(t *testing.T) {
	mockey.PatchConvey("Start processing kmsg messages", t, func() {
		ctx := context.Background()
		cctx, cancel := context.WithCancel(ctx)

		mockBucket := &mockEventBucketSXID{}
		mockReboot := &mockRebootEventStoreSXID{}
		mockWatcher := &mockKmsgWatcherSXID{
			watchCh: make(chan kmsg.Message, 10),
		}

		comp := &component{
			ctx:              cctx,
			cancel:           cancel,
			eventBucket:      mockBucket,
			rebootEventStore: mockReboot,
			kmsgWatcher:      mockWatcher,
			extraEventCh:     make(chan *eventstore.Event, 256),
			getTimeNowFunc: func() time.Time {
				return time.Now().UTC()
			},
		}

		// Start in background
		go comp.start(mockWatcher.watchCh, 50*time.Millisecond)

		// Send SXID message
		mockWatcher.watchCh <- kmsg.Message{
			Message:   "nvidia-nvswitch3: SXid (PCI:0000:05:00.0): 12028, Non-fatal, Link 32 egress non-posted PRIV error (First)",
			Timestamp: metav1.Time{Time: time.Now()},
		}

		// Send non-matching message
		mockWatcher.watchCh <- kmsg.Message{
			Message:   "some other non-matching message",
			Timestamp: metav1.Time{Time: time.Now()},
		}

		// Allow time for processing
		time.Sleep(200 * time.Millisecond)

		cancel()
	})
}

// TestStart_WithExtraEvent_WithMockey tests start processing extra events.
func TestStart_WithExtraEvent_WithMockey(t *testing.T) {
	mockey.PatchConvey("Start processing extra events", t, func() {
		ctx := context.Background()
		cctx, cancel := context.WithCancel(ctx)

		mockBucket := &mockEventBucketSXID{}
		mockReboot := &mockRebootEventStoreSXID{}

		comp := &component{
			ctx:              cctx,
			cancel:           cancel,
			eventBucket:      mockBucket,
			rebootEventStore: mockReboot,
			extraEventCh:     make(chan *eventstore.Event, 256),
			getTimeNowFunc: func() time.Time {
				return time.Now().UTC()
			},
		}

		// Start in background with a dummy channel
		dummyCh := make(chan kmsg.Message)
		go comp.start(dummyCh, 50*time.Millisecond)

		// Send an extra event
		event := &eventstore.Event{
			Time:    time.Now().UTC(),
			Name:    "test_event",
			Message: "test message",
		}
		comp.extraEventCh <- event

		// Allow time for processing
		time.Sleep(200 * time.Millisecond)

		cancel()
	})
}

// TestStart_WithNilExtraEvent_WithMockey tests start with nil extra event.
func TestStart_WithNilExtraEvent_WithMockey(t *testing.T) {
	mockey.PatchConvey("Start with nil extra event", t, func() {
		ctx := context.Background()
		cctx, cancel := context.WithCancel(ctx)

		mockBucket := &mockEventBucketSXID{}
		mockReboot := &mockRebootEventStoreSXID{}

		comp := &component{
			ctx:              cctx,
			cancel:           cancel,
			eventBucket:      mockBucket,
			rebootEventStore: mockReboot,
			extraEventCh:     make(chan *eventstore.Event, 256),
			getTimeNowFunc: func() time.Time {
				return time.Now().UTC()
			},
		}

		// Start in background with a dummy channel
		dummyCh := make(chan kmsg.Message)
		go comp.start(dummyCh, 50*time.Millisecond)

		// Send nil event - should be handled gracefully
		comp.extraEventCh <- nil

		// Allow time for processing
		time.Sleep(200 * time.Millisecond)

		cancel()
	})
}

// TestStart_InsertEventError_WithMockey tests start when event insert fails.
func TestStart_InsertEventError_WithMockey(t *testing.T) {
	mockey.PatchConvey("Start with event insert error", t, func() {
		ctx := context.Background()
		cctx, cancel := context.WithCancel(ctx)

		mockBucket := &mockEventBucketSXID{
			insertErr: errors.New("insert error"),
		}
		mockReboot := &mockRebootEventStoreSXID{}

		comp := &component{
			ctx:              cctx,
			cancel:           cancel,
			eventBucket:      mockBucket,
			rebootEventStore: mockReboot,
			extraEventCh:     make(chan *eventstore.Event, 256),
			getTimeNowFunc: func() time.Time {
				return time.Now().UTC()
			},
		}

		// Start in background with a dummy channel
		dummyCh := make(chan kmsg.Message)
		go comp.start(dummyCh, 50*time.Millisecond)

		// Send an extra event - should fail to insert but not crash
		event := &eventstore.Event{
			Time:    time.Now().UTC(),
			Name:    "test_event",
			Message: "test message",
		}
		comp.extraEventCh <- event

		// Allow time for processing
		time.Sleep(200 * time.Millisecond)

		cancel()
	})
}

// TestStart_DuplicateSXIDEvent_WithMockey tests start when a duplicate SXID event is found.
func TestStart_DuplicateSXIDEvent_WithMockey(t *testing.T) {
	mockey.PatchConvey("Start with duplicate SXID event", t, func() {
		ctx := context.Background()
		cctx, cancel := context.WithCancel(ctx)

		existingEvent := eventstore.Event{
			Time: time.Now(),
			Name: EventNameErrorSXid,
		}
		mockBucket := &mockEventBucketSXID{
			findEvent: &existingEvent, // Event already exists
		}
		mockReboot := &mockRebootEventStoreSXID{}
		mockWatcher := &mockKmsgWatcherSXID{
			watchCh: make(chan kmsg.Message, 10),
		}

		comp := &component{
			ctx:              cctx,
			cancel:           cancel,
			eventBucket:      mockBucket,
			rebootEventStore: mockReboot,
			kmsgWatcher:      mockWatcher,
			extraEventCh:     make(chan *eventstore.Event, 256),
			getTimeNowFunc: func() time.Time {
				return time.Now().UTC()
			},
		}

		// Start in background
		go comp.start(mockWatcher.watchCh, 50*time.Millisecond)

		// Send SXID message - should be skipped as duplicate
		mockWatcher.watchCh <- kmsg.Message{
			Message:   "nvidia-nvswitch3: SXid (PCI:0000:05:00.0): 12028, Non-fatal, Link 32 egress non-posted PRIV error (First)",
			Timestamp: metav1.Time{Time: time.Now()},
		}

		// Allow time for processing
		time.Sleep(200 * time.Millisecond)

		cancel()

		// Verify no new event was inserted (events should be empty since insert was not called)
		assert.Len(t, mockBucket.events, 0)
	})
}

// TestStart_FindEventError_WithMockey tests start when finding event fails.
func TestStart_FindEventError_WithMockey(t *testing.T) {
	mockey.PatchConvey("Start with find event error", t, func() {
		ctx := context.Background()
		cctx, cancel := context.WithCancel(ctx)

		mockBucket := &mockEventBucketSXID{
			findErr: errors.New("find error"),
		}
		mockReboot := &mockRebootEventStoreSXID{}
		mockWatcher := &mockKmsgWatcherSXID{
			watchCh: make(chan kmsg.Message, 10),
		}

		comp := &component{
			ctx:              cctx,
			cancel:           cancel,
			eventBucket:      mockBucket,
			rebootEventStore: mockReboot,
			kmsgWatcher:      mockWatcher,
			extraEventCh:     make(chan *eventstore.Event, 256),
			getTimeNowFunc: func() time.Time {
				return time.Now().UTC()
			},
		}

		// Start in background
		go comp.start(mockWatcher.watchCh, 50*time.Millisecond)

		// Send SXID message - should fail to find but handle gracefully
		mockWatcher.watchCh <- kmsg.Message{
			Message:   "nvidia-nvswitch3: SXid (PCI:0000:05:00.0): 12028, Non-fatal, Link 32 egress non-posted PRIV error (First)",
			Timestamp: metav1.Time{Time: time.Now()},
		}

		// Allow time for processing
		time.Sleep(200 * time.Millisecond)

		cancel()
	})
}

// TestUpdateCurrentState_NilStores_WithMockey tests updateCurrentState with nil stores.
func TestUpdateCurrentState_NilStores_WithMockey(t *testing.T) {
	mockey.PatchConvey("updateCurrentState with nil stores", t, func() {
		ctx := context.Background()
		cctx, cancel := context.WithCancel(ctx)
		defer cancel()

		comp := &component{
			ctx:              cctx,
			cancel:           cancel,
			rebootEventStore: nil,
			eventBucket:      nil,
			getTimeNowFunc: func() time.Time {
				return time.Now().UTC()
			},
		}

		err := comp.updateCurrentState()
		assert.NoError(t, err)
	})
}

// TestMatch_UnknownSXID_WithMockey tests Match with unknown SXID.
func TestMatch_UnknownSXID_WithMockey(t *testing.T) {
	mockey.PatchConvey("Match with unknown SXID", t, func() {
		// Unknown SXID returns nil even if the pattern matches
		result := Match("nvidia-nvswitch3: SXid (PCI:0000:05:00.0): 99999, Some error")
		assert.Nil(t, result)
	})
}

// TestMatch_ValidSXID_WithMockey tests Match with valid SXID.
func TestMatch_ValidSXID_WithMockey(t *testing.T) {
	mockey.PatchConvey("Match with valid SXID", t, func() {
		result := Match("nvidia-nvswitch3: SXid (PCI:0000:05:00.0): 12028, Non-fatal, Link 32 egress non-posted PRIV error")
		require.NotNil(t, result)
		assert.Equal(t, 12028, result.SXid)
		assert.Equal(t, "PCI:0000:05:00.0", result.DeviceUUID)
		assert.NotNil(t, result.Detail)
	})
}

// TestMatch_NoMatch_WithMockey tests Match with non-matching line.
func TestMatch_NoMatch_WithMockey(t *testing.T) {
	mockey.PatchConvey("Match with non-matching line", t, func() {
		result := Match("some random kernel message")
		assert.Nil(t, result)
	})
}

// TestExtractNVSwitchSXid_WithMockey tests ExtractNVSwitchSXid function.
func TestExtractNVSwitchSXid_WithMockey(t *testing.T) {
	testCases := []struct {
		name     string
		line     string
		expected int
	}{
		{
			name:     "valid sxid",
			line:     "nvidia-nvswitch3: SXid (PCI:0000:05:00.0): 12028, Non-fatal",
			expected: 12028,
		},
		{
			name:     "no sxid",
			line:     "some other message",
			expected: 0,
		},
		{
			name:     "empty string",
			line:     "",
			expected: 0,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := ExtractNVSwitchSXid(tc.line)
			assert.Equal(t, tc.expected, result)
		})
	}
}

// TestExtractNVSwitchSXidDeviceUUID_WithMockey tests ExtractNVSwitchSXidDeviceUUID function.
func TestExtractNVSwitchSXidDeviceUUID_WithMockey(t *testing.T) {
	testCases := []struct {
		name     string
		line     string
		expected string
	}{
		{
			name:     "valid device uuid",
			line:     "nvidia-nvswitch3: SXid (PCI:0000:05:00.0): 12028, Non-fatal",
			expected: "PCI:0000:05:00.0",
		},
		{
			name:     "no device uuid",
			line:     "some other message",
			expected: "",
		},
		{
			name:     "empty string",
			line:     "",
			expected: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := ExtractNVSwitchSXidDeviceUUID(tc.line)
			assert.Equal(t, tc.expected, result)
		})
	}
}

// TestCheckResult_String_WithErrors_WithMockey tests checkResult.String with errors.
func TestCheckResult_String_WithErrors_WithMockey(t *testing.T) {
	mockey.PatchConvey("checkResult.String with errors", t, func() {
		cr := &checkResult{
			FoundErrors: []FoundError{
				{
					Kmsg: kmsg.Message{
						Message:   "nvidia-nvswitch3: SXid (PCI:0000:05:00.0): 12028, Non-fatal error",
						Timestamp: metav1.Time{Time: time.Now()},
					},
					SXidError: SXidError{
						SXid:       12028,
						DeviceUUID: "PCI:0000:05:00.0",
						Detail: &Detail{
							Name: "Test SXid Error",
							SuggestedActionsByGPUd: &apiv1.SuggestedActions{
								RepairActions: []apiv1.RepairActionType{apiv1.RepairActionTypeRebootSystem},
							},
							EventType: apiv1.EventTypeFatal,
						},
					},
				},
			},
			health: apiv1.HealthStateTypeUnhealthy,
			reason: "found some errors",
		}

		result := cr.String()
		assert.Contains(t, result, "12028")
		assert.Contains(t, result, "PCI:0000:05:00.0")
		assert.Contains(t, result, "Test SXid Error")
	})
}

// TestCheckResult_String_WithNilDetail_WithMockey tests checkResult.String with nil Detail.
func TestCheckResult_String_WithNilDetail_WithMockey(t *testing.T) {
	mockey.PatchConvey("checkResult.String with nil Detail", t, func() {
		cr := &checkResult{
			FoundErrors: []FoundError{
				{
					Kmsg: kmsg.Message{
						Message:   "nvidia-nvswitch3: SXid (PCI:0000:05:00.0): 12028, error",
						Timestamp: metav1.Time{Time: time.Now()},
					},
					SXidError: SXidError{
						SXid:       12028,
						DeviceUUID: "PCI:0000:05:00.0",
						Detail:     nil, // nil detail should be handled
					},
				},
			},
			health: apiv1.HealthStateTypeHealthy,
			reason: "found 1 error with nil detail",
		}

		result := cr.String()
		assert.Contains(t, result, "12028")
		assert.Contains(t, result, "unknown") // action should be unknown
	})
}

// TestCheckResult_String_WithNilSuggestedActions_WithMockey tests checkResult.String with nil SuggestedActionsByGPUd.
func TestCheckResult_String_WithNilSuggestedActions_WithMockey(t *testing.T) {
	mockey.PatchConvey("checkResult.String with nil SuggestedActionsByGPUd", t, func() {
		cr := &checkResult{
			FoundErrors: []FoundError{
				{
					Kmsg: kmsg.Message{
						Message:   "nvidia-nvswitch3: SXid (PCI:0000:05:00.0): 12028, error",
						Timestamp: metav1.Time{Time: time.Now()},
					},
					SXidError: SXidError{
						SXid:       12028,
						DeviceUUID: "PCI:0000:05:00.0",
						Detail: &Detail{
							Name:                   "Test Error",
							SuggestedActionsByGPUd: nil, // nil suggested actions
							EventType:              apiv1.EventTypeCritical,
						},
					},
				},
			},
			health: apiv1.HealthStateTypeHealthy,
			reason: "found 1 error",
		}

		result := cr.String()
		assert.Contains(t, result, "12028")
		assert.Contains(t, result, "unknown") // action should be unknown when suggested actions is nil
	})
}

// TestLastHealthStates_WithMockey tests LastHealthStates method.
func TestLastHealthStates_WithMockey(t *testing.T) {
	mockey.PatchConvey("LastHealthStates returns current state", t, func() {
		ctx := context.Background()
		cctx, cancel := context.WithCancel(ctx)
		defer cancel()

		expectedState := apiv1.HealthState{
			Name:   StateNameErrorSXid,
			Health: apiv1.HealthStateTypeHealthy,
			Reason: "Test reason",
		}

		comp := &component{
			ctx:       cctx,
			cancel:    cancel,
			currState: expectedState,
		}

		states := comp.LastHealthStates()
		require.Len(t, states, 1)
		assert.Equal(t, expectedState, states[0])
	})
}
