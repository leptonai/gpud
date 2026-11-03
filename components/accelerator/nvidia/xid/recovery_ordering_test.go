package xid

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/NVIDIA/go-nvml/pkg/nvml/mock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/pkg/eventstore"
	"github.com/leptonai/gpud/pkg/kmsg"
	"github.com/leptonai/gpud/pkg/nvidia/nvml/device"
	"github.com/leptonai/gpud/pkg/nvidia/nvml/testutil"
	"github.com/leptonai/gpud/pkg/nvsentinel"
	"github.com/leptonai/gpud/pkg/nvsentinel/nvsentineltest"
)

func TestRecoveryRepeatedTransitionSurvivesDelivery(t *testing.T) {
	for _, producers := range [][3]string{
		{"nvsentinel", "nvsentinel", "nvsentinel"},
		{"nvsentinel", "kmsg", "kmsg"},
		{"kmsg", "nvsentinel", "nvsentinel"},
		{"kmsg", "kmsg", "kmsg"},
	} {
		for _, awaitInsert := range []bool{false, true} {
			t.Run(fmt.Sprintf("%v/await-insert=%t", producers, awaitInsert), func(t *testing.T) {
				c, _ := newTestComponentWithNVSentinel(t, nvsentineltest.NewFakeSource())
				ch := make(chan kmsg.Message, 3)
				go c.start(ch, time.Hour)
				now := time.Now().UTC().Truncate(time.Second).Add(-10 * time.Second)
				request := "NVRM: Xid (PCI:0000:01:00): 154, GPU recovery action changed from 0x0 (None) to 0x1 (GPU Reset Required)"
				none := "NVRM: Xid (PCI:0000:01:00): 154, GPU recovery action changed from 0x1 (GPU Reset Required) to 0x0 (None)"
				waitForCount := func(count int) {
					t.Helper()
					require.Eventually(t, func() bool {
						events, err := c.eventBucket.Get(context.Background(), now.Add(-time.Second))
						return err == nil && len(events) == count
					}, time.Second, 10*time.Millisecond)
				}
				for i, line := range []string{request, none, request} {
					ts := now.Add(time.Duration(i) * time.Second)
					if producers[i] == "kmsg" {
						ch <- kmsg.Message{Timestamp: metav1.NewTime(ts), Message: line}
					} else {
						ev := newNVSentinelXidEvent("154", "PCI:0000:01:00")
						ev.ID = fmt.Sprintf("transition-%d", i)
						ev.GeneratedTimestamp = ts
						ev.Message = line
						if i == 1 {
							ev.IsFatal = false
							ev.Action = nvsentinel.RecommendedActionNone
						}
						c.onNVSentinelEvent(ev)
					}
					if awaitInsert {
						waitForCount(i + 1)
					}
				}
				waitForCount(3)
				require.Eventually(t, func() bool {
					return c.LastHealthStates()[0].Health == apiv1.HealthStateTypeUnhealthy
				}, time.Second, 10*time.Millisecond)
				state := c.LastHealthStates()[0]
				require.NotNil(t, state.SuggestedActions)
				assert.Equal(t, apiv1.RepairActionTypeRebootSystem, state.SuggestedActions.RepairActions[0])
			})
		}
	}
}

func TestRecoveryTiedTimestampsCannotClearRequest(t *testing.T) {
	for _, to := range []string{"0x0 (None)", "0x3 (Drain P2P)", "0x4 (Drain and Reset)"} {
		for _, reverse := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/reverse=%t", to, reverse), func(t *testing.T) {
				c, _ := newTestComponentWithNVSentinel(t, nil)
				now := time.Now().UTC().Truncate(time.Second).Add(-10 * time.Second)
				older := recoveryEvent(t, "0000:01:00", to)
				older.Time = now.Add(100 * time.Millisecond)
				request := recoveryEvent(t, "0000:01:00", "0x2 (OS Reboot)")
				request.Time = now.Add(600 * time.Millisecond)
				events := eventstore.Events{older, request}
				if reverse {
					events[0], events[1] = events[1], events[0]
				}
				for _, ev := range events {
					require.NoError(t, c.eventBucket.Insert(context.Background(), ev))
				}
				require.NoError(t, c.updateCurrentState())
				state := c.LastHealthStates()[0]
				assert.Equal(t, apiv1.HealthStateTypeUnhealthy, state.Health)
				require.NotNil(t, state.SuggestedActions)
				assert.Equal(t, apiv1.RepairActionTypeRebootSystem, state.SuggestedActions.RepairActions[0])
				// A strictly later None still retires the request.
				none := recoveryEvent(t, "0000:01:00", "0x0 (None)")
				none.Time = now.Add(time.Second)
				require.NoError(t, c.eventBucket.Insert(context.Background(), none))
				require.NoError(t, c.updateCurrentState())
				assert.Equal(t, apiv1.HealthStateTypeHealthy, c.LastHealthStates()[0].Health)
			})
		}
	}
	request := recoveryEvent(t, "0000:01:00", "0x2 (OS Reboot)")
	none := recoveryEvent(t, "0000:01:00", "0x0 (None)")
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, evolveHealthyState(eventstore.Events{none, request}, nil, 2).Health, "missing timestamps cannot prove recovery")
}

func TestStoredXidTwinNormalizesNativeDeviceIdentity(t *testing.T) {
	for _, legacy := range []bool{false, true} {
		t.Run(fmt.Sprintf("legacy=%t", legacy), func(t *testing.T) {
			c, _ := newTestComponentWithNVSentinel(t, nvsentineltest.NewFakeSource())
			c.devices = map[string]device.Device{
				"GPU-native": testutil.NewMockDevice(&mock.Device{}, "test-arch", "test-brand", "test-cuda", "0000:9b:00.0"),
			}
			now := time.Now().UTC()
			ev := createXidEvent(now, 79, apiv1.EventTypeFatal, apiv1.RepairActionTypeRebootSystem)
			if legacy {
				ev.ExtraInfo[EventKeyErrorXidData] = "79"
				ev.ExtraInfo[EventKeyDeviceUUID] = "PCI:0000:9b:00"
			}
			require.NoError(t, c.eventBucket.Insert(context.Background(), ev))
			assert.True(t, c.hasStoredXidTwin(79, "GPU-native", now))
			assert.False(t, c.hasStoredXidTwin(79, "GPU-other", now))
		})
	}
}

func TestRecoveryInformationalRequestNeedsLaterNone(t *testing.T) {
	request := recoveryEvent(t, "0000:01:00", "0x4 (Drain and Reset)")
	none := recoveryEvent(t, "0000:01:00", "0x0 (None)")
	request.Time = time.Unix(1, 0)
	none.Time = request.Time
	for _, events := range []eventstore.Events{{request, none}, {none, request}} {
		state := evolveHealthyState(events, nil, 2)
		assert.Equal(t, apiv1.HealthStateTypeHealthy, state.Health)
		assert.Contains(t, state.Reason, "0x4 (Drain and Reset)")
	}
	none.Time = time.Unix(2, 0)
	state := evolveHealthyState(eventstore.Events{none, request}, nil, 2)
	assert.Equal(t, "XIDComponent is healthy", state.Reason)
}
