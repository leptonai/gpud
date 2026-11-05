package kmsg

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/bytedance/mockey"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apiv1 "github.com/leptonai/gpud/api/v1"
	legacy "github.com/leptonai/gpud/pkg/kmsg"
)

type legacyWatcher struct {
	input <-chan legacy.Message
	err   error
}

func (w legacyWatcher) Watch() (<-chan legacy.Message, error) { return w.input, w.err }
func (w legacyWatcher) Close() error                          { return nil }

func TestLegacyAdapters(t *testing.T) {
	message := legacy.Message{Timestamp: metav1.NewTime(time.Unix(1700000000, 0)), Priority: 3, SequenceNumber: 12, Message: "kernel record"}
	readErr := errors.New("partial read")
	mockey.PatchConvey("legacy snapshot retains partial data and errors", t, func() {
		mockey.Mock(legacy.ReadAll).Return([]legacy.Message{message}, readErr).Build()
		records, err := ReadLegacy(t.Context())
		require.ErrorIs(t, err, readErr)
		require.Equal(t, []apiv1.KernelMessage{legacyMessage(message)}, records)
	})
	input := make(chan legacy.Message, 1)
	input <- message
	close(input)
	records, err := WatchLegacy(t.Context(), legacyWatcher{input: input})
	require.NoError(t, err)
	require.Equal(t, legacyMessage(message), <-records)
	_, ok := <-records
	require.False(t, ok)
	_, err = WatchLegacy(t.Context(), legacyWatcher{err: readErr})
	require.ErrorIs(t, err, readErr)
}

func TestLegacyWatchCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	input := make(chan legacy.Message, 1)
	input <- legacy.Message{Message: "blocked consumer"}
	records, err := WatchLegacy(ctx, legacyWatcher{input: input})
	require.NoError(t, err)
	cancel()
	done := make(chan struct{})
	go func() {
		for range records {
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("legacy forwarding did not stop")
	}
}
