package xid

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/pkg/eventstore"
	"github.com/leptonai/gpud/pkg/kmsg"
	"github.com/leptonai/gpud/pkg/nvsentinel/nvsentineltest"
)

func newRecoveryBatchComponent(t *testing.T, native bool) (*component, chan kmsg.Message) {
	t.Helper()
	c, _ := newTestComponentWithNVSentinel(t, nvsentineltest.NewFakeSource())
	if c.kmsgWatcher != nil {
		require.NoError(t, c.kmsgWatcher.Close())
	}
	c.kmsgWatcher = nil
	var ch chan kmsg.Message
	if native {
		ch = make(chan kmsg.Message, 64)
		c.kmsgWatcher = &stubWatcher{ch: ch}
	}
	require.NoError(t, c.Start())
	return c, ch
}

func sendRecoveryBatchEvent(c *component, ch chan kmsg.Message, ts time.Time, sequence, from, to int) {
	line := fmt.Sprintf("NVRM: Xid (PCI:0000:01:00): 154, GPU recovery action changed from 0x%x (previous) to 0x%x (next)", from, to)
	if ch != nil {
		ch <- kmsg.Message{Timestamp: metav1.NewTime(ts), SequenceNumber: sequence, Message: line}
		return
	}
	ev := newNVSentinelXidEvent("154", "PCI:0000:01:00")
	ev.ID = fmt.Sprintf("batch-%d", sequence)
	ev.GeneratedTimestamp = ts
	ev.Message = line
	c.onNVSentinelEvent(ev)
}

func waitForRecoveryBatch(t *testing.T, c *component, since time.Time, count int) eventstore.Events {
	t.Helper()
	var events eventstore.Events
	require.Eventually(t, func() bool {
		var err error
		events, err = c.eventBucket.Get(context.Background(), since)
		return err == nil && len(events) == count
	}, 2*time.Second, 10*time.Millisecond)
	require.NoError(t, c.updateCurrentState())
	return events
}

func TestRecoverySameSecondBatch(t *testing.T) {
	for _, native := range []bool{false, true} {
		for _, action := range []int{1, 3, 4} {
			for _, gap := range []time.Duration{0, time.Millisecond} {
				t.Run(fmt.Sprintf("native=%t/action=%d/gap=%s", native, action, gap), func(t *testing.T) {
					c, ch := newRecoveryBatchComponent(t, native)
					now := time.Now().UTC().Truncate(time.Second).Add(-time.Second)
					sendRecoveryBatchEvent(c, ch, now, 1, 0, action)
					sendRecoveryBatchEvent(c, ch, now.Add(gap), 2, action, 0)
					waitForRecoveryBatch(t, c, now.Add(-time.Second), 2)
					state := c.LastHealthStates()[0]
					if !native && gap == 0 {
						// NVSentinel carries no kernel position. Tied timestamps
						// cannot establish whether None preceded the request.
						expected := apiv1.HealthStateTypeHealthy
						if action == 1 {
							expected = apiv1.HealthStateTypeUnhealthy
						}
						if action == 3 {
							expected = apiv1.HealthStateTypeDegraded
						}
						assert.Equal(t, expected, state.Health)
						assert.Contains(t, state.Reason, "XID 154")
						return
					}
					assert.Equal(t, apiv1.HealthStateTypeHealthy, state.Health)
					assert.Nil(t, state.SuggestedActions)
					assert.Equal(t, "XIDComponent is healthy", state.Reason)
				})
			}
		}
	}
}

func TestRecoveryProducerPrecisionSurvivesStorage(t *testing.T) {
	for _, native := range []bool{false, true} {
		t.Run(fmt.Sprintf("native=%t", native), func(t *testing.T) {
			c, ch := newRecoveryBatchComponent(t, native)
			now := time.Now().UTC().Truncate(time.Second).Add(-time.Second)
			// Even a matching None cannot clear a request with a newer source timestamp.
			sendRecoveryBatchEvent(c, ch, now.Add(600*time.Millisecond), 2, 0, 1)
			sendRecoveryBatchEvent(c, ch, now.Add(100*time.Millisecond), 1, 1, 0)
			waitForRecoveryBatch(t, c, now.Add(-time.Second), 2)
			assert.Equal(t, apiv1.HealthStateTypeUnhealthy, c.LastHealthStates()[0].Health)
		})
	}
}

func TestRecoveryLongTiedBatchKeepsOrder(t *testing.T) {
	for _, native := range []bool{false, true} {
		t.Run(fmt.Sprintf("native=%t", native), func(t *testing.T) {
			c, ch := newRecoveryBatchComponent(t, native)
			now := time.Now().UTC().Truncate(time.Second).Add(-time.Second)
			for i := range 32 {
				to := (i + 1) % 2
				sendRecoveryBatchEvent(c, ch, now, i+1, 1-to, to)
			}
			waitForRecoveryBatch(t, c, now.Add(-time.Second), 32)
			if !native {
				assert.Equal(t, apiv1.HealthStateTypeUnhealthy, c.LastHealthStates()[0].Health)
				require.NotNil(t, c.LastHealthStates()[0].SuggestedActions)
				assert.Equal(t, []apiv1.RepairActionType{apiv1.RepairActionTypeRebootSystem}, c.LastHealthStates()[0].SuggestedActions.RepairActions)
				return
			}
			assert.Equal(t, apiv1.HealthStateTypeHealthy, c.LastHealthStates()[0].Health)
			assert.Nil(t, c.LastHealthStates()[0].SuggestedActions)
		})
	}
}

func TestRecoverySameSecondRecurrence(t *testing.T) {
	for _, native := range []bool{false, true} {
		t.Run(fmt.Sprintf("native=%t", native), func(t *testing.T) {
			c, ch := newRecoveryBatchComponent(t, native)
			now := time.Now().UTC().Truncate(time.Second).Add(-time.Second)
			for i, to := range []int{1, 0, 1} {
				sendRecoveryBatchEvent(c, ch, now.Add(time.Duration(i+1)*time.Millisecond), i+1, 1-to, to)
			}
			waitForRecoveryBatch(t, c, now.Add(-time.Second), 3)
			state := c.LastHealthStates()[0]
			assert.Equal(t, apiv1.HealthStateTypeUnhealthy, state.Health)
			require.NotNil(t, state.SuggestedActions)
			assert.Equal(t, []apiv1.RepairActionType{apiv1.RepairActionTypeRebootSystem}, state.SuggestedActions.RepairActions)
		})
	}
}

func TestRecoveryNativeSuppressesNVSentinelReplay(t *testing.T) {
	c, ch := newRecoveryBatchComponent(t, true)
	now := time.Now().UTC().Truncate(time.Second)
	metric := metricXIDErrs.With(prometheus.Labels{"uuid": "", "xid": "154"})
	metricValue := func() float64 {
		var value dto.Metric
		require.NoError(t, metric.Write(&value))
		return value.GetCounter().GetValue()
	}
	before := metricValue()
	sendRecoveryBatchEvent(c, ch, now.Add(-31*time.Minute), 1, 0, 1)
	sendRecoveryBatchEvent(c, ch, now.Add(-31*time.Minute+10*time.Second), 2, 1, 0)
	waitForRecoveryBatch(t, c, now.Add(-time.Hour), 2)
	require.Equal(t, apiv1.HealthStateTypeHealthy, c.LastHealthStates()[0].Health)

	sendRecoveryBatchEvent(c, nil, now.Add(100*time.Millisecond), 1, 0, 1)
	sendRecoveryBatchEvent(c, nil, now.Add(200*time.Millisecond), 2, 1, 0)
	// A queued marker proves both preceding NVSentinel copies were consumed.
	c.extraEventCh <- pendingEvent{event: &eventstore.Event{Name: "batch-drained", Time: now}}
	events := waitForRecoveryBatch(t, c, now.Add(-time.Hour), 3)
	xidCount := 0
	for _, ev := range events {
		if ev.Name == EventNameErrorXid {
			xidCount++
			assert.Empty(t, ev.ExtraInfo[EventKeyNVSentinelEventID])
		}
	}
	assert.Equal(t, 2, xidCount)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, c.LastHealthStates()[0].Health)
	assert.Equal(t, before+2, metricValue())
	apiEvents, err := c.Events(context.Background(), now.Add(-time.Hour))
	require.NoError(t, err)
	assert.Len(t, apiEvents, 3) // Two native reports and the drain marker.
}

func TestRecoveryNVSentinelResumesAfterNativeCloses(t *testing.T) {
	c, ch := newRecoveryBatchComponent(t, true)
	close(ch)
	now := time.Now().UTC().Add(-time.Second)
	require.Eventually(t, func() bool {
		if c.LastHealthStates()[0].Health == apiv1.HealthStateTypeUnhealthy {
			return true
		}
		sendRecoveryBatchEvent(c, nil, now, 1, 0, 2)
		return false
	}, time.Second, 10*time.Millisecond)
}
