//nolint:forcetypeassert,revive // tests intentionally inspect concrete types; package name follows the directory import path.
package os

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/eventstore"
)

func TestSetHealthy_ClosesEpisodeAndResetsTracker(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	bucket := &recordingBucket{}
	comp, now := newTestComponent(t, ctx, &mockRebootEventStore{}, bucket)
	defer func() { _ = comp.Close() }()
	withBlockedProcessThresholds(comp, mustBlockedProcessThresholds(t, 5, "^nvidia"))

	// become persistent and unhealthy
	cr := runChecks(comp, now, 5, blockedList(&mockProcessStatus{pid: 42, name: "nvidia-smi"}))
	require.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.HealthStateType())
	require.Equal(t, []string{EventNameBlockedProcessesPersistent}, bucket.insertedNames())

	require.NoError(t, comp.SetHealthy())

	// the active episode is closed with a recovery event
	assert.Equal(t, []string{EventNameBlockedProcessesPersistent, EventNameBlockedProcessesRecovered}, bucket.insertedNames())

	// the tracker restarts: the same still-blocked process must re-earn the
	// persistence threshold before being flagged again
	cr = runChecks(comp, now, 4, blockedList(&mockProcessStatus{pid: 42, name: "nvidia-smi"}))
	assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.HealthStateType(),
		"set healthy must restart persistence tracking even while the process stays blocked")

	// and once it persists again, it flags a new episode
	cr = runChecks(comp, now, 1, blockedList(&mockProcessStatus{pid: 42, name: "nvidia-smi"}))
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.HealthStateType())
	assert.Equal(t, []string{
		EventNameBlockedProcessesPersistent,
		EventNameBlockedProcessesRecovered,
		EventNameBlockedProcessesPersistent,
	}, bucket.insertedNames())
}

func TestSetHealthy_ResetsEscalationWindow(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	// two reboots with no recovery in between escalate to hardware inspection
	rebootStore := &mockRebootEventStore{
		events: eventstore.Events{
			{Time: time.Unix(99000, 0), Name: "reboot", Type: string(apiv1.EventTypeWarning)},
			{Time: time.Unix(99500, 0), Name: "reboot", Type: string(apiv1.EventTypeWarning)},
		},
	}
	bucket := &recordingBucket{}
	comp, now := newTestComponent(t, ctx, rebootStore, bucket)
	defer func() { _ = comp.Close() }()
	withBlockedProcessThresholds(comp, mustBlockedProcessThresholds(t, 5, "^nvidia"))

	cr := runChecks(comp, now, 5, blockedList(&mockProcessStatus{pid: 42, name: "nvidia-smi"}))
	states := cr.HealthStates()
	require.Len(t, states, 1)
	require.NotNil(t, states[0].SuggestedActions)
	require.Equal(t, []apiv1.RepairActionType{apiv1.RepairActionTypeHardwareInspection}, states[0].SuggestedActions.RepairActions)

	// operator acknowledges: the recovery event anchors the escalation window,
	// so reboots before SetHealthy no longer count
	require.NoError(t, comp.SetHealthy())

	cr = runChecks(comp, now, 5, blockedList(&mockProcessStatus{pid: 42, name: "nvidia-smi"}))
	states = cr.HealthStates()
	require.Len(t, states, 1)
	require.NotNil(t, states[0].SuggestedActions)
	assert.Equal(t, []apiv1.RepairActionType{apiv1.RepairActionTypeRebootSystem}, states[0].SuggestedActions.RepairActions,
		"reboots before set healthy must not count toward hardware-inspection escalation")
}

func TestSetHealthy_NoActiveEpisodeEmitsNoEvent(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	bucket := &recordingBucket{}
	comp, _ := newTestComponent(t, ctx, &mockRebootEventStore{}, bucket)
	defer func() { _ = comp.Close() }()

	require.NoError(t, comp.SetHealthy())
	assert.Empty(t, bucket.insertedNames(), "no active episode must not emit a recovery event")
}

func TestSetHealthy_NilBucketAndTracker(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	// partially constructed component (no event bucket, no tracker, no clock)
	comp := &component{ctx: ctx, cancel: cancel}
	assert.NoError(t, comp.SetHealthy())
}

func TestSetHealthy_ImplementsHealthSettable(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	c, err := New(&components.GPUdInstance{
		RootCtx:          ctx,
		RebootEventStore: &mockRebootEventStore{},
	})
	require.NoError(t, err)
	defer func() { _ = c.Close() }()

	_, ok := c.(components.HealthSettable)
	assert.True(t, ok, "os component must implement components.HealthSettable")
}
