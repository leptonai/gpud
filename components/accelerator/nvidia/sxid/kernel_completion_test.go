package sxid

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

	componentkmsg "github.com/leptonai/gpud/components/kmsg"
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
		c := &component{ctx: t.Context()}
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
	})
}
