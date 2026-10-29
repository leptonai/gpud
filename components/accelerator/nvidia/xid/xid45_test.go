package xid

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/pkg/eventstore"
	pkghost "github.com/leptonai/gpud/pkg/host"
	"github.com/leptonai/gpud/pkg/nvsentinel"
)

func TestXID45StoredEventsRemainInformational(t *testing.T) {
	for _, severity := range []apiv1.EventType{apiv1.EventTypeCritical, apiv1.EventTypeFatal} {
		for _, format := range []string{"legacy", "ignore", "reboot", "hardware"} {
			for _, source := range []string{"kmsg", dataSourceNVSentinel} {
				if format == "legacy" && source != "kmsg" {
					continue
				}
				t.Run(fmt.Sprintf("%s/%s/%s", severity, format, source), func(t *testing.T) {
					c, store := newTestComponentWithNVSentinel(t, nil)
					ctx := context.Background()
					now := time.Now().UTC().Truncate(time.Second)
					action := map[string]apiv1.RepairActionType{
						"ignore":   apiv1.RepairActionTypeIgnoreNoActionRequired,
						"reboot":   apiv1.RepairActionTypeRebootSystem,
						"hardware": apiv1.RepairActionTypeHardwareInspection,
					}[format]
					event := createXidEvent(now.Add(-time.Minute), 45, severity, action)
					var payload xidErrorEventDetail
					require.NoError(t, json.Unmarshal([]byte(event.ExtraInfo[EventKeyErrorXidData]), &payload))
					payload.DataSource = source
					raw, err := json.Marshal(payload)
					require.NoError(t, err)
					event.ExtraInfo[EventKeyErrorXidData] = string(raw)
					if format == "legacy" {
						event.ExtraInfo[EventKeyErrorXidData] = "45"
					}
					originalPayload := event.ExtraInfo[EventKeyErrorXidData]
					require.NoError(t, c.eventBucket.Insert(ctx, event))

					for _, rebooted := range []bool{false, true} {
						if rebooted {
							bucket, err := store.Bucket(pkghost.EventBucketName)
							require.NoError(t, err)
							require.NoError(t, bucket.Insert(ctx, eventstore.Event{Time: now, Name: pkghost.EventNameReboot}))
						}
						require.NoError(t, c.updateCurrentState())
						state := c.LastHealthStates()[0]
						assert.Equal(t, apiv1.HealthStateTypeHealthy, state.Health, "rebooted=%t", rebooted)
						require.NotNil(t, state.SuggestedActions)
						assert.Equal(t, []apiv1.RepairActionType{apiv1.RepairActionTypeIgnoreNoActionRequired}, state.SuggestedActions.RepairActions)
					}

					apiEvents, err := c.Events(ctx, now.Add(-time.Hour))
					require.NoError(t, err)
					require.Len(t, apiEvents, 1)
					assert.Equal(t, apiv1.EventTypeWarning, apiEvents[0].Type)
					assert.Contains(t, apiEvents[0].Message, "XID 45")

					// Upgrade-time normalization must not erase the original diagnostic evidence.
					stored, err := c.eventBucket.Get(ctx, now.Add(-time.Hour))
					require.NoError(t, err)
					require.Len(t, stored, 1)
					assert.Equal(t, string(severity), stored[0].Type)
					assert.Equal(t, originalPayload, stored[0].ExtraInfo[EventKeyErrorXidData])
				})
			}
		}
	}

}

func TestXID45NVSentinelUsesCatalogPolicy(t *testing.T) {
	for _, fatal := range []bool{false, true} {
		for _, action := range []nvsentinel.RecommendedAction{nvsentinel.RecommendedActionNone, nvsentinel.RecommendedActionRestartBM, nvsentinel.RecommendedActionContactSupport} {
			t.Run(fmt.Sprintf("fatal=%t/%s", fatal, action), func(t *testing.T) {
				c, _ := newTestComponentWithNVSentinel(t, nil)
				ev := newNVSentinelXidEvent("45", "GPU-cleanup")
				ev.IsFatal = fatal
				ev.Action = action
				c.onNVSentinelEvent(ev)

				var pending pendingEvent
				select {
				case pending = <-c.extraEventCh:
				default:
					t.Fatal("XID 45 diagnostic event was discarded")
				}
				require.NotNil(t, pending.event)
				assert.Equal(t, string(apiv1.EventTypeWarning), pending.event.Type)
				assert.Equal(t, ev.Message, pending.event.Message)
				assert.Equal(t, ev.ID, pending.event.ExtraInfo[EventKeyNVSentinelEventID])
				assert.Equal(t, ev.CheckName, pending.event.ExtraInfo[EventKeyNVSentinelCheckName])
				var detail xidErrorEventDetail
				require.NoError(t, json.Unmarshal([]byte(pending.event.ExtraInfo[EventKeyErrorXidData]), &detail))
				assert.Equal(t, dataSourceNVSentinel, detail.DataSource)
				assert.Equal(t, ev.GeneratedTimestamp.Unix(), detail.Time.Unix())
				assert.Equal(t, "GPU-cleanup", detail.DeviceUUID)
				require.NotNil(t, detail.SuggestedActionsByGPUd)
				assert.Equal(t, []apiv1.RepairActionType{apiv1.RepairActionTypeIgnoreNoActionRequired}, detail.SuggestedActionsByGPUd.RepairActions)

				require.NoError(t, c.eventBucket.Insert(context.Background(), *pending.event))
				require.NoError(t, c.updateCurrentState())
				assert.Equal(t, apiv1.HealthStateTypeHealthy, c.LastHealthStates()[0].Health)
			})
		}
	}
}

func TestXID45DoesNotMaskIndependentFaults(t *testing.T) {
	cases := []struct {
		name   string
		event  eventstore.Event
		health apiv1.HealthStateType
		action apiv1.RepairActionType
		reason string
	}{
		{"reboot", createXidEvent(time.Time{}, 95, apiv1.EventTypeFatal, apiv1.RepairActionTypeRebootSystem), apiv1.HealthStateTypeUnhealthy, apiv1.RepairActionTypeRebootSystem, "XID 95"},
		{"hardware", createXidEvent(time.Time{}, 79, apiv1.EventTypeFatal, apiv1.RepairActionTypeHardwareInspection), apiv1.HealthStateTypeUnhealthy, apiv1.RepairActionTypeHardwareInspection, "XID 79"},
		{"recovery reboot", recoveryEvent(t, "0000:01:00", "0x2 (OS Reboot)"), apiv1.HealthStateTypeUnhealthy, apiv1.RepairActionTypeRebootSystem, "OS Reboot"},
		{"recovery degraded", recoveryEvent(t, "0000:01:00", "0x3 (Drain P2P)"), apiv1.HealthStateTypeDegraded, apiv1.RepairActionTypeCheckUserAppAndGPU, "Drain P2P"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, cleanupFirst := range []bool{false, true} {
				cleanup := createXidEvent(time.Time{}, 45, apiv1.EventTypeFatal, apiv1.RepairActionTypeHardwareInspection)
				events := eventstore.Events{cleanup, tc.event}
				if cleanupFirst {
					events[0], events[1] = events[1], events[0]
				}
				state := evolveHealthyState(events, nil, DefaultRebootThreshold)
				assert.Equal(t, tc.health, state.Health)
				require.NotNil(t, state.SuggestedActions)
				assert.Equal(t, []apiv1.RepairActionType{tc.action}, state.SuggestedActions.RepairActions)
				assert.Contains(t, state.Reason, tc.reason)
			}
		})
	}
}

func TestXID45NativeKmsgRemainsInformational(t *testing.T) {
	c, store := newTestComponentWithNVSentinel(t, nil)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	message := apiv1.KernelMessage{
		Timestamp: metav1.NewTime(now.Add(-time.Minute)),
		Message:   "NVRM: Xid (PCI:0000:01:00): 45, pid=123, name=test, Ch 00000001",
	}
	kmsgCh := make(chan apiv1.KernelMessage, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		c.startKernelTest(kmsgCh, time.Hour)
	}()
	t.Cleanup(func() {
		c.cancel()
		<-done
	})
	kmsgCh <- message

	var events eventstore.Events
	require.Eventually(t, func() bool {
		var err error
		events, err = c.eventBucket.Get(ctx, now.Add(-time.Hour))
		return err == nil && len(events) == 1
	}, 5*time.Second, 10*time.Millisecond)
	assert.Equal(t, string(apiv1.EventTypeWarning), events[0].Type)
	var payload xidErrorEventDetail
	require.NoError(t, json.Unmarshal([]byte(events[0].ExtraInfo[EventKeyErrorXidData]), &payload))
	assert.Equal(t, "kmsg", payload.DataSource)
	assert.Equal(t, uint64(45), payload.Xid)
	require.NotNil(t, payload.SuggestedActionsByGPUd)
	assert.Equal(t, []apiv1.RepairActionType{apiv1.RepairActionTypeIgnoreNoActionRequired}, payload.SuggestedActionsByGPUd.RepairActions)

	for _, rebooted := range []bool{false, true} {
		if rebooted {
			bucket, err := store.Bucket(pkghost.EventBucketName)
			require.NoError(t, err)
			require.NoError(t, bucket.Insert(ctx, eventstore.Event{Time: now, Name: pkghost.EventNameReboot}))
		}
		require.NoError(t, c.updateCurrentState())
		state := c.LastHealthStates()[0]
		assert.Equal(t, apiv1.HealthStateTypeHealthy, state.Health, "rebooted=%t", rebooted)
		require.NotNil(t, state.SuggestedActions)
		assert.Equal(t, []apiv1.RepairActionType{apiv1.RepairActionTypeIgnoreNoActionRequired}, state.SuggestedActions.RepairActions)
	}
}

func TestXID45NativeKmsgScanRemainsInformational(t *testing.T) {
	c, _ := newTestComponentWithNVSentinel(t, nil)
	c.nvmlInstance = createMockNVMLInstance()
	c.readAllKmsg = kernelReaderForTest(func(context.Context) ([]apiv1.KernelMessage, error) {
		return []apiv1.KernelMessage{{
			Timestamp: metav1.NewTime(time.Now().UTC()),
			Message:   "NVRM: Xid (PCI:0000:01:00): 45, pid=123, name=test, Ch 00000001",
		}}, nil
	})
	result := c.Check()
	assert.Equal(t, apiv1.HealthStateTypeHealthy, result.HealthStateType())
	assert.Contains(t, result.Summary(), "XID 45")
	state := result.HealthStates()[0]
	require.NotNil(t, state.SuggestedActions)
	assert.Equal(t, []apiv1.RepairActionType{apiv1.RepairActionTypeIgnoreNoActionRequired}, state.SuggestedActions.RepairActions)
}
