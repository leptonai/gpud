package xid

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/bytedance/mockey"
	"github.com/stretchr/testify/require"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/pkg/kmsg"
)

func TestLegacyKernelParsing(t *testing.T) {
	message := apiv1.KernelMessage{Message: "NVRM: Xid (0000:03:00): 14, Channel 00000001"}
	mockey.PatchConvey("legacy snapshot preserves legacy Xid identity", t, func() {
		readErr := errors.New("partial read")
		mockey.Mock(kmsg.ReadAllWithOptions).Return([]kmsg.Message{{Message: message.Message}}, readErr).Build()
		records, err := readLegacyKernelMessages(t.Context())
		require.ErrorIs(t, err, readErr)
		require.Len(t, records, 1)
		require.Equal(t, "0000:03:00", records[0].xid.DeviceUUID)
	})
	c := &component{ctx: t.Context()}
	input := make(chan apiv1.KernelMessage, 1)
	input <- message
	close(input)
	output := c.convertLegacyKernel(input)
	record := <-output
	require.Equal(t, "0000:03:00", record.xid.DeviceUUID)
	require.Equal(t, message, record.KernelMessage)
	_, ok := <-output
	require.False(t, ok)
}

func TestLegacyKernelCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	c := &component{ctx: ctx}
	input := make(chan apiv1.KernelMessage, 1)
	input <- apiv1.KernelMessage{Message: "NVRM: Xid (0000:03:00): 14, Channel 00000001"}
	output := c.convertLegacyKernel(input)
	cancel()
	done := make(chan struct{})
	go func() {
		for range output {
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("legacy Xid forwarding did not stop")
	}
}
