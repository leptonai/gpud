package kmsg

import (
	"context"
	"errors"
	"io"
	"os"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/bytedance/mockey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shirou/gopsutil/v4/host"
)

func TestNewWatcher_OpenErrorWithMockey(t *testing.T) {
	mockey.PatchConvey("NewWatcher returns open error", t, func() {
		mockey.Mock(os.Open).To(func(name string) (*os.File, error) {
			return nil, errors.New("open failed")
		}).Build()

		w, err := NewWatcher()
		require.Error(t, err)
		assert.Nil(t, w)
	})
}

func TestNewWatcher_UptimeErrorWithMockey(t *testing.T) {
	mockey.PatchConvey("NewWatcher returns uptime error", t, func() {
		tmp, err := os.CreateTemp("", "kmsg")
		require.NoError(t, err)
		defer func() {
			_ = tmp.Close()
			_ = os.Remove(tmp.Name())
		}()

		mockey.Mock(os.Open).To(func(name string) (*os.File, error) {
			return tmp, nil
		}).Build()
		mockey.Mock(host.UptimeWithContext).To(func(ctx context.Context) (uint64, error) {
			return 0, errors.New("uptime error")
		}).Build()

		w, err := NewWatcher()
		require.Error(t, err)
		assert.Nil(t, w)
	})
}

func TestWatcher_WatchAlreadyStarted(t *testing.T) {
	tmp, err := os.CreateTemp("", "kmsg")
	require.NoError(t, err)
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
	}()

	w := &watcher{kmsgFile: tmp}
	w.watchStarted.Store(true)

	ch, err := w.Watch()
	require.Error(t, err)
	assert.Nil(t, ch)
	assert.ErrorIs(t, err, ErrWatcherAlreadyStarted)
}

func TestReadFollow_EPIPEThenEOFWithMockey(t *testing.T) {
	mockey.PatchConvey("readFollow handles EPIPE then EOF", t, func() {
		tmp, err := os.CreateTemp("", "kmsg")
		require.NoError(t, err)
		defer func() {
			_ = tmp.Close()
			_ = os.Remove(tmp.Name())
		}()

		var calls int32
		mockey.Mock((*os.File).Read).To(func(f *os.File, p []byte) (int, error) {
			call := atomic.AddInt32(&calls, 1)
			if call == 1 {
				return 0, syscall.EPIPE
			}
			return 0, io.EOF
		}).Build()

		msgs := make(chan Message, 1)
		err = readFollow(tmp, time.Now(), msgs, nil)
		require.NoError(t, err)
		_, ok := <-msgs
		assert.False(t, ok)
		assert.GreaterOrEqual(t, atomic.LoadInt32(&calls), int32(2))
	})
}

func TestReadFollow_ParseErrorWithMockey(t *testing.T) {
	mockey.PatchConvey("readFollow returns parse error", t, func() {
		tmp, err := os.CreateTemp("", "kmsg")
		require.NoError(t, err)
		defer func() {
			_ = tmp.Close()
			_ = os.Remove(tmp.Name())
		}()

		var calls int32
		mockey.Mock((*os.File).Read).To(func(f *os.File, p []byte) (int, error) {
			if atomic.AddInt32(&calls, 1) == 1 {
				line := []byte("bad line")
				copy(p, line)
				return len(line), nil
			}
			return 0, io.EOF
		}).Build()

		msgs := make(chan Message, 1)
		err = readFollow(tmp, time.Now(), msgs, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "malformed kmsg message")
	})
}
