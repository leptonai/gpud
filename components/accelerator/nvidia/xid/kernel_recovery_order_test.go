package xid

import (
	"context"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/pkg/eventstore"
)

func TestKernelRecoverySequenceSurvivesScanAndStorage(t *testing.T) {
	for _, actions := range [][]int{{2, 0}, {2, 0, 2}, {2, 3}, {2, 4}} {
		for _, reverse := range []bool{false, true} {
			t.Run(fmt.Sprintf("actions=%v/reverse=%t", actions, reverse), func(t *testing.T) {
				c, _ := newTestComponentWithNVSentinel(t, nil)
				c.nvmlInstance = createMockNVMLInstance()
				at := time.Now().UTC().Truncate(time.Second).Add(-time.Minute)
				messages := make([]apiv1.KernelMessage, len(actions))
				from := 0
				for i, to := range actions {
					messages[i] = apiv1.KernelMessage{
						BootID: "boot", SequenceNumber: i + 1, Timestamp: metav1.NewTime(at),
						Message: fmt.Sprintf("NVRM: Xid (PCI:0000:01:00): 154, GPU recovery action changed from 0x%x (previous) to 0x%x (next)", from, to),
					}
					from = to
				}
				if reverse {
					slices.Reverse(messages)
				}
				c.readAllKmsg = kernelReaderForTest(func(context.Context) ([]apiv1.KernelMessage, error) { return messages, nil })
				expected := apiv1.HealthStateTypeHealthy
				switch actions[len(actions)-1] {
				case 2:
					expected = apiv1.HealthStateTypeUnhealthy
				case 3:
					expected = apiv1.HealthStateTypeDegraded
				}
				assert.Equal(t, expected, c.Check().HealthStateType(), "scan must use kernel order")
				ch := make(chan apiv1.KernelMessage, len(messages))
				go c.startKernelTest(ch, time.Hour)
				for _, message := range messages {
					ch <- message
				}
				stored := waitForRecoveryBatch(t, c, at.Add(-time.Second), len(messages))
				for _, event := range stored {
					assert.NotEmpty(t, event.ExtraInfo[eventKeyKmsgBootID])
					assert.NotEmpty(t, event.ExtraInfo[eventKeyKmsgSequence])
				}
				assert.Equal(t, expected, c.LastHealthStates()[0].Health, "persisted sequence must survive replay")
				if actions[len(actions)-1] == 0 {
					assert.Nil(t, c.LastHealthStates()[0].SuggestedActions)
					assert.Equal(t, "XIDComponent is healthy", c.LastHealthStates()[0].Reason)
				}
			})
		}
	}
}

func TestRecoveryAmbiguousOrderCannotInferAChain(t *testing.T) {
	for _, positions := range [][]string{{"", ""}, {"7", "7"}, {"7", ""}, {"7", "invalid"}} {
		for _, reverse := range []bool{false, true} {
			t.Run(fmt.Sprintf("positions=%v/reverse=%t", positions, reverse), func(t *testing.T) {
				request := recoveryEvent(t, "0000:01:00", "0x4 (Drain and Reset)")
				none := recoveryEvent(t, "0000:01:00", "0x0 (None)")
				request.Time = time.Unix(1, 0)
				none.Time = request.Time
				events := eventstore.Events{request, none}
				for i, sequence := range positions {
					if sequence != "" {
						events[i].ExtraInfo[eventKeyKmsgSequence] = sequence
					}
				}
				if reverse {
					slices.Reverse(events)
				}
				state := evolveHealthyState(events, nil, DefaultRebootThreshold)
				assert.Contains(t, state.Reason, "Drain and Reset", "a matching From/To pair does not prove chronology")
			})
		}
	}
}

func TestRecoverySequenceDoesNotCrossBoots(t *testing.T) {
	request := recoveryEvent(t, "0000:01:00", "0x2 (OS Reboot)")
	none := recoveryEvent(t, "0000:01:00", "0x0 (None)")
	request.Time, none.Time = time.Unix(1, 0), time.Unix(1, 0)
	request.ExtraInfo[eventKeyKmsgSequence], none.ExtraInfo[eventKeyKmsgSequence] = "1", "100"
	request.ExtraInfo["kmsg_boot_id"], none.ExtraInfo["kmsg_boot_id"] = "new-boot", "old-boot"
	for _, events := range []eventstore.Events{{request, none}, {none, request}} {
		state := evolveHealthyState(events, nil, DefaultRebootThreshold)
		require.Equal(t, apiv1.HealthStateTypeUnhealthy, state.Health)
	}
}

func TestKernelRecoveryReplayBeforeBootIdentityWasStored(t *testing.T) {
	first, _ := newTestComponentWithNVSentinel(t, nil)
	at := time.Now().UTC().Truncate(time.Second).Add(-time.Minute)
	request := apiv1.KernelMessage{
		SequenceNumber: 1, Timestamp: metav1.NewTime(at),
		Message: recoveryLine("0000:01:00", "0x2 (OS Reboot)"),
	}
	input := make(chan apiv1.KernelMessage, 1)
	go first.startKernelTest(input, time.Hour)
	input <- request
	original := waitForRecoveryBatch(t, first, at.Add(-time.Second), 1)[0]
	delete(original.ExtraInfo, eventKeyKmsgBootID)

	upgraded, _ := newTestComponentWithNVSentinel(t, nil)
	require.NoError(t, upgraded.eventBucket.Insert(t.Context(), original))
	replay := make(chan apiv1.KernelMessage, 2)
	go upgraded.startKernelTest(replay, time.Hour)
	replay <- request
	replay <- apiv1.KernelMessage{
		SequenceNumber: 2, Timestamp: metav1.NewTime(at),
		Message: recoveryLine("0000:01:00", "0x0 (None)"),
	}
	require.Eventually(t, func() bool {
		events, err := upgraded.eventBucket.Get(t.Context(), at.Add(-time.Second))
		return err == nil && len(events) == 2 && upgraded.LastHealthStates()[0].Health == apiv1.HealthStateTypeHealthy
	}, 2*time.Second, 10*time.Millisecond, "upgrading must not duplicate a retained request or prevent its later None")
	assert.Nil(t, upgraded.LastHealthStates()[0].SuggestedActions)
}
