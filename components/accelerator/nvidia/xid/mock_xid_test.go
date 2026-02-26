//go:build linux

package xid

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

// TestNew_WithMockey tests the New function using mockey for isolation.
func TestNew_WithMockey(t *testing.T) {
	mockey.PatchConvey("New component creation with NVMLInstance", t, func() {
		ctx := context.Background()

		mockInstance := &mockNVMLInstanceForMockey{
			devicesFunc: func() map[string]device.Device {
				return map[string]device.Device{"gpu-1": nil}
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
		assert.NotNil(t, tc.devices)
		assert.Len(t, tc.devices, 1)
	})
}

// TestComponent_IsSupported_WithMockey tests IsSupported method with various conditions.
func TestComponent_IsSupported_WithMockey(t *testing.T) {
	testCases := []struct {
		name         string
		nvmlInstance func() *mockNVMLInstanceForMockey
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
			nvmlInstance: func() *mockNVMLInstanceForMockey {
				return &mockNVMLInstanceForMockey{
					devicesFunc: func() map[string]device.Device { return nil },
				}
			},
			nvmlExists:  false,
			productName: "NVIDIA H100",
			expected:    false,
		},
		{
			name: "no product name returns false",
			nvmlInstance: func() *mockNVMLInstanceForMockey {
				return &mockNVMLInstanceForMockey{
					devicesFunc: func() map[string]device.Device { return nil },
				}
			},
			nvmlExists:  true,
			productName: "",
			expected:    false,
		},
		{
			name: "NVML loaded with product name returns true",
			nvmlInstance: func() *mockNVMLInstanceForMockey {
				return &mockNVMLInstanceForMockey{
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

// TestCheck_InitError_WithMockey tests Check when NVML has an initialization error.
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

// TestCheck_MissingProductName_WithMockey tests Check when product name is empty.
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

// TestCheck_ReadKmsgError_WithMockey tests Check when reading kmsg fails.
func TestCheck_ReadKmsgError_WithMockey(t *testing.T) {
	mockey.PatchConvey("Check with kmsg read error", t, func() {
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

// TestCheck_WithXidErrors_WithMockey tests Check when XID errors are found.
func TestCheck_WithXidErrors_WithMockey(t *testing.T) {
	mockey.PatchConvey("Check with XID errors", t, func() {
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
			readAllKmsg: func(ctx context.Context) ([]kmsg.Message, error) {
				return []kmsg.Message{
					{
						Timestamp: metav1.NewTime(time.Now()),
						Message:   "NVRM: Xid (PCI:0000:01:00): 79, GPU has fallen off the bus.",
					},
				}, nil
			},
		}

		result := comp.Check()
		cr, ok := result.(*checkResult)
		require.True(t, ok)

		assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
		assert.Len(t, cr.FoundErrors, 1)
		assert.Equal(t, 79, cr.FoundErrors[0].Xid)
	})
}

// TestCheck_Xid63And64Skipped_WithMockey tests Check skips XID 63/64 when row remapping is supported.
func TestCheck_Xid63And64Skipped_WithMockey(t *testing.T) {
	mockey.PatchConvey("Check skips XID 63/64 with row remapping", t, func() {
		ctx := context.Background()
		cctx, cancel := context.WithCancel(ctx)
		defer cancel()

		mockInst := &customMockNVMLInstanceForMockey{
			devs:        map[string]device.Device{},
			nvmlExists:  true,
			productName: "NVIDIA H100",
			memoryErrorCapabilities: nvidiaproduct.MemoryErrorManagementCapabilities{
				RowRemapping: true,
			},
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
						Message:   "NVRM: Xid (PCI:0000:01:00): 63, Row remapping pending",
					},
					{
						Timestamp: metav1.NewTime(time.Now()),
						Message:   "NVRM: Xid (PCI:0000:02:00): 64, Row remapping failure",
					},
					{
						Timestamp: metav1.NewTime(time.Now()),
						Message:   "NVRM: Xid (PCI:0000:03:00): 31, pid=1234",
					},
				}, nil
			},
		}

		result := comp.Check()
		cr, ok := result.(*checkResult)
		require.True(t, ok)

		// Only XID 31 should be found (63 and 64 are skipped)
		assert.Len(t, cr.FoundErrors, 1)
		assert.Equal(t, 31, cr.FoundErrors[0].Xid)
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
		assert.Equal(t, "no xid error found", cr.String())
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

// TestConvertBusIDToUUID_WithMockey tests the convertBusIDToUUID function.
func TestConvertBusIDToUUID_WithMockey(t *testing.T) {
	mockey.PatchConvey("convertBusIDToUUID", t, func() {
		// Test with nil devices
		result := convertBusIDToUUID("PCI:0000:01:00", nil)
		assert.Empty(t, result)

		// Test with empty devices
		result = convertBusIDToUUID("PCI:0000:01:00", map[string]device.Device{})
		assert.Empty(t, result)
	})
}

// TestGetDetail_WithMockey tests the GetDetail function.
func TestGetDetail_WithMockey(t *testing.T) {
	// Test known XID
	detail, ok := GetDetail(79)
	assert.True(t, ok)
	assert.NotNil(t, detail)
	assert.Equal(t, 79, detail.Code)

	// Test unknown XID
	_, ok = GetDetail(99999)
	assert.False(t, ok)
}

// TestRebootThreshold_WithMockey tests the reboot threshold functions.
func TestRebootThreshold_WithMockey(t *testing.T) {
	mockey.PatchConvey("RebootThreshold functions", t, func() {
		// Get default
		threshold := GetDefaultRebootThreshold()
		assert.Equal(t, DefaultRebootThreshold, threshold.Threshold)

		// Set new threshold
		newThreshold := RebootThreshold{Threshold: 5}
		SetDefaultRebootThreshold(newThreshold)

		// Verify it changed
		threshold = GetDefaultRebootThreshold()
		assert.Equal(t, 5, threshold.Threshold)

		// Reset to default
		SetDefaultRebootThreshold(RebootThreshold{Threshold: DefaultRebootThreshold})
	})
}

// TestEvolveHealthyState_WithCriticalEvent_WithMockey tests evolveHealthyState with critical events.
func TestEvolveHealthyState_WithCriticalEvent_WithMockey(t *testing.T) {
	mockey.PatchConvey("evolveHealthyState with critical event", t, func() {
		events := eventstore.Events{
			createXidEvent(time.Now(), 13, apiv1.EventTypeCritical, apiv1.RepairActionTypeCheckUserAppAndGPU),
		}
		state := evolveHealthyState(events, nil, DefaultRebootThreshold)
		// Critical events result in degraded state
		assert.Equal(t, apiv1.HealthStateTypeDegraded, state.Health)
	})
}

// TestEvolveHealthyState_WithInvalidJSON_WithMockey tests evolveHealthyState with invalid JSON.
func TestEvolveHealthyState_WithInvalidJSON_WithMockey(t *testing.T) {
	mockey.PatchConvey("evolveHealthyState with invalid JSON", t, func() {
		events := eventstore.Events{
			{
				Name: EventNameErrorXid,
				Type: string(apiv1.EventTypeFatal),
				ExtraInfo: map[string]string{
					EventKeyErrorXidData: "invalid json {{{",
				},
			},
		}
		state := evolveHealthyState(events, nil, DefaultRebootThreshold)
		// Should remain healthy since the event cannot be parsed
		assert.Equal(t, apiv1.HealthStateTypeHealthy, state.Health)
	})
}

// TestEvolveHealthyState_RebootClearsBothActions_WithMockey tests that reboot clears both RebootSystem and CheckUserAppAndGPU.
func TestEvolveHealthyState_RebootClearsBothActions_WithMockey(t *testing.T) {
	mockey.PatchConvey("reboot clears RebootSystem action", t, func() {
		events := eventstore.Events{
			{Name: "reboot", Time: time.Now()},
			createXidEvent(time.Now().Add(-1*time.Hour), 94, apiv1.EventTypeFatal, apiv1.RepairActionTypeRebootSystem),
		}
		state := evolveHealthyState(events, nil, DefaultRebootThreshold)
		assert.Equal(t, apiv1.HealthStateTypeHealthy, state.Health)
	})

	mockey.PatchConvey("reboot clears CheckUserAppAndGPU action", t, func() {
		events := eventstore.Events{
			{Name: "reboot", Time: time.Now()},
			createXidEvent(time.Now().Add(-1*time.Hour), 13, apiv1.EventTypeCritical, apiv1.RepairActionTypeCheckUserAppAndGPU),
		}
		state := evolveHealthyState(events, nil, DefaultRebootThreshold)
		assert.Equal(t, apiv1.HealthStateTypeHealthy, state.Health)
	})
}

// TestEvolveHealthyState_RebootDoesNotClearHardwareInspection_WithMockey tests that reboot does not clear HardwareInspection.
func TestEvolveHealthyState_RebootDoesNotClearHardwareInspection_WithMockey(t *testing.T) {
	mockey.PatchConvey("reboot does not clear HardwareInspection", t, func() {
		events := eventstore.Events{
			{Name: "reboot", Time: time.Now()},
			createXidEvent(time.Now().Add(-1*time.Hour), 999, apiv1.EventTypeFatal, apiv1.RepairActionTypeHardwareInspection),
		}
		state := evolveHealthyState(events, nil, DefaultRebootThreshold)
		// Should remain unhealthy because HardwareInspection is not cleared by reboot
		assert.Equal(t, apiv1.HealthStateTypeUnhealthy, state.Health)
		require.NotNil(t, state.SuggestedActions)
		assert.Equal(t, apiv1.RepairActionTypeHardwareInspection, state.SuggestedActions.RepairActions[0])
	})
}

// TestAddEventDetails_WithEmptyExtraInfo_WithMockey tests addEventDetails with nil ExtraInfo.
func TestAddEventDetails_WithEmptyExtraInfo_WithMockey(t *testing.T) {
	mockey.PatchConvey("addEventDetails with nil ExtraInfo", t, func() {
		ev := eventstore.Event{
			Name:      EventNameErrorXid,
			Time:      time.Now(),
			ExtraInfo: nil,
		}
		xidErr := &xidErrorEventDetail{
			Xid:        94,
			DeviceUUID: "PCI:0000:01:00",
		}

		result := addEventDetails(ev, xidErr, nil)

		assert.NotNil(t, result.ExtraInfo)
		assert.Contains(t, result.ExtraInfo, EventKeyErrorXidData)
	})
}

// TestAddEventDetails_PreservesExistingEventType_WithMockey tests that addEventDetails preserves existing event type.
func TestAddEventDetails_PreservesExistingEventType_WithMockey(t *testing.T) {
	mockey.PatchConvey("addEventDetails preserves existing event type", t, func() {
		ev := eventstore.Event{
			Name: EventNameErrorXid,
			Type: string(apiv1.EventTypeWarning),
			Time: time.Now(),
		}
		xidErr := &xidErrorEventDetail{
			Xid:        145,
			DeviceUUID: "PCI:0000:01:00",
		}

		result := addEventDetails(ev, xidErr, nil)

		// Should preserve the Warning type, not overwrite with merged detail
		assert.Equal(t, string(apiv1.EventTypeWarning), result.Type)
	})
}

// TestResolveXIDEvent_WithNilExtraInfo_WithMockey tests resolveXIDEvent with nil ExtraInfo.
func TestResolveXIDEvent_WithNilExtraInfo_WithMockey(t *testing.T) {
	mockey.PatchConvey("resolveXIDEvent with nil ExtraInfo", t, func() {
		event := eventstore.Event{
			Name:      EventNameErrorXid,
			Time:      time.Now(),
			ExtraInfo: nil,
		}

		result := resolveXIDEvent(event, nil)

		// Should return the original event unchanged
		assert.Equal(t, event, result)
	})
}

// TestResolveXIDEvent_WithLegacyFormat_WithMockey tests resolveXIDEvent with legacy XID format.
func TestResolveXIDEvent_WithLegacyFormat_WithMockey(t *testing.T) {
	mockey.PatchConvey("resolveXIDEvent with legacy format", t, func() {
		event := eventstore.Event{
			Name: EventNameErrorXid,
			Time: time.Now(),
			ExtraInfo: map[string]string{
				EventKeyErrorXidData: "94",
				EventKeyDeviceUUID:   "PCI:0000:01:00",
			},
		}

		result := resolveXIDEvent(event, nil)

		assert.Contains(t, result.Message, "XID 94")
		assert.Contains(t, result.Message, "PCI:0000:01:00")
	})
}

// TestResolveXIDEvent_WithUnknownXID_WithMockey tests resolveXIDEvent with unknown XID.
func TestResolveXIDEvent_WithUnknownXID_WithMockey(t *testing.T) {
	mockey.PatchConvey("resolveXIDEvent with unknown XID", t, func() {
		event := eventstore.Event{
			Name: EventNameErrorXid,
			Time: time.Now(),
			ExtraInfo: map[string]string{
				EventKeyErrorXidData: "99999",
				EventKeyDeviceUUID:   "PCI:0000:01:00",
			},
		}

		result := resolveXIDEvent(event, nil)

		// Should return original event when XID is unknown
		assert.Equal(t, event, result)
	})
}

// TestBuildMessage_NilXidErr_WithMockey tests buildMessage with nil xidErrorEventDetail.
func TestBuildMessage_NilXidErr_WithMockey(t *testing.T) {
	mockey.PatchConvey("buildMessage with nil", t, func() {
		var xidErr *xidErrorEventDetail
		result := xidErr.buildMessage(nil)
		assert.Equal(t, "unknown", result)
	})
}

// TestBuildMessage_NVLinkXID_WithMockey tests buildMessage for NVLink XIDs (144-150).
func TestBuildMessage_NVLinkXID_WithMockey(t *testing.T) {
	mockey.PatchConvey("buildMessage for NVLink XID", t, func() {
		xidErr := &xidErrorEventDetail{
			Xid:         145,
			SubCode:     0,
			ErrorStatus: 0x00000001,
			DeviceUUID:  "PCI:0000:01:00",
			Description: "NVLink RLW Error",
		}
		result := xidErr.buildMessage(nil)

		assert.Contains(t, result, "XID 145.0")
		assert.Contains(t, result, "err status 0x00000001")
		assert.Contains(t, result, "GPU PCI:0000:01:00")
	})
}

// TestBuildMessage_StandardXID_WithMockey tests buildMessage for standard XIDs.
func TestBuildMessage_StandardXID_WithMockey(t *testing.T) {
	mockey.PatchConvey("buildMessage for standard XID", t, func() {
		xidErr := &xidErrorEventDetail{
			Xid:         94,
			DeviceUUID:  "PCI:0000:01:00",
			Description: "Contained ECC error",
		}
		result := xidErr.buildMessage(nil)

		assert.Contains(t, result, "XID 94")
		assert.NotContains(t, result, "err status") // Standard XIDs don't have error status
		assert.Contains(t, result, "GPU PCI:0000:01:00")
	})
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
func (m *mockNVMLInstanceForMockey) Library() nvmllib.Library {
	return nil
}
func (m *mockNVMLInstanceForMockey) Shutdown() error  { return nil }
func (m *mockNVMLInstanceForMockey) InitError() error { return nil }

type customMockNVMLInstanceForMockey struct {
	devs                    map[string]device.Device
	nvmlExists              bool
	productName             string
	memoryErrorCapabilities nvidiaproduct.MemoryErrorManagementCapabilities
}

func (m *customMockNVMLInstanceForMockey) Devices() map[string]device.Device { return m.devs }
func (m *customMockNVMLInstanceForMockey) FabricManagerSupported() bool      { return true }
func (m *customMockNVMLInstanceForMockey) FabricStateSupported() bool        { return false }
func (m *customMockNVMLInstanceForMockey) GetMemoryErrorManagementCapabilities() nvidiaproduct.MemoryErrorManagementCapabilities {
	return m.memoryErrorCapabilities
}
func (m *customMockNVMLInstanceForMockey) ProductName() string   { return m.productName }
func (m *customMockNVMLInstanceForMockey) Architecture() string  { return "" }
func (m *customMockNVMLInstanceForMockey) Brand() string         { return "" }
func (m *customMockNVMLInstanceForMockey) DriverVersion() string { return "" }
func (m *customMockNVMLInstanceForMockey) DriverMajor() int      { return 0 }
func (m *customMockNVMLInstanceForMockey) CUDAVersion() string   { return "" }
func (m *customMockNVMLInstanceForMockey) NVMLExists() bool      { return m.nvmlExists }
func (m *customMockNVMLInstanceForMockey) Library() nvmllib.Library {
	return nil
}
func (m *customMockNVMLInstanceForMockey) Shutdown() error  { return nil }
func (m *customMockNVMLInstanceForMockey) InitError() error { return nil }

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
func (m *mockNVMLInstanceWithInitErrorForMockey) Library() nvmllib.Library {
	return nil
}
func (m *mockNVMLInstanceWithInitErrorForMockey) Shutdown() error  { return nil }
func (m *mockNVMLInstanceWithInitErrorForMockey) InitError() error { return m.initError }
