package xid

import (
	"context"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/bytedance/mockey"
	kmsgsource "github.com/dsx-ai-factory/health-validation/collect/source/kmsg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apiv1 "github.com/leptonai/gpud/api/v1"
	componentkmsg "github.com/leptonai/gpud/components/kmsg"
	"github.com/leptonai/gpud/pkg/nvsentinel/nvsentineltest"
)

func TestKernelSensorClosesAfterSourceFailure(t *testing.T) {
	t.Setenv("KMSG_FILE_PATH", "")
	mockey.PatchConvey("terminal source failure reaches the component event loop", t, func() {
		var closes atomic.Int32
		mockey.Mock(kmsgsource.NewStream).To(func(int) (*kmsgsource.Stream, error) {
			return &kmsgsource.Stream{}, nil
		}).Build()
		mockey.Mock((*kmsgsource.Stream).Read).To(func(*kmsgsource.Stream, context.Context) ([]kmsgsource.Message, error) {
			return []kmsgsource.Message{{BootID: "boot", Sequence: 1, Timestamp: time.Now(), Priority: 3, Body: "retained before failure"}}, syscall.EIO
		}).Build()
		mockey.Mock((*kmsgsource.Stream).Close).To(func(*kmsgsource.Stream) error {
			closes.Add(1)
			return nil
		}).Build()
		c, _ := newTestComponentWithNVSentinel(t, nvsentineltest.NewFakeSource())
		messages, stop, err := c.observeKernel(componentkmsg.New(t.Context()).Subscribe)
		require.NoError(t, err)
		defer stop()
		select {
		case message, open := <-messages:
			require.True(t, open)
			assert.Equal(t, "retained before failure", message.Message)
		case <-time.After(time.Second):
			t.Fatal("lost partial source data")
		}
		select {
		case _, open := <-messages:
			require.False(t, open, "terminal reader must close the component channel")
		case <-time.After(time.Second):
			t.Fatal("component channel stayed open after the native reader exited")
		}
		stop()
		stop()
		assert.EqualValues(t, 1, closes.Load())
		loopDone := make(chan struct{})
		go func() {
			defer close(loopDone)
			c.start(messages, time.Hour)
		}()
		defer func() { c.cancel(); <-loopDone }()
		event := newNVSentinelXidEvent("154", "GPU-recovery")
		event.Message = recoveryLine("0000:01:00", "0x2 (OS Reboot)")
		require.Eventually(t, func() bool {
			// A report can race the loop observing EOF; subsequent reports
			// must resume instead of being suppressed for the daemon lifetime.
			c.onNVSentinelEvent(event)
			return c.LastHealthStates()[0].Health == apiv1.HealthStateTypeUnhealthy
		}, time.Second, 10*time.Millisecond)
		state := c.LastHealthStates()[0]
		require.NotNil(t, state.SuggestedActions)
		assert.Contains(t, state.SuggestedActions.RepairActions, apiv1.RepairActionTypeRebootSystem)
	})
}
