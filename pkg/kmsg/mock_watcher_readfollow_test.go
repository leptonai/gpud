package kmsg

import (
	"errors"
	"io"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bytedance/mockey"
	"github.com/stretchr/testify/require"
)

func TestReadFollow_FileClosed_WithMockey(t *testing.T) {
	mockey.PatchConvey("readFollow returns nil on closed file", t, func() {
		tmp, err := os.CreateTemp("", "kmsg")
		require.NoError(t, err)
		defer func() {
			_ = tmp.Close()
			_ = os.Remove(tmp.Name())
		}()

		mockey.Mock((*os.File).Read).To(func(f *os.File, p []byte) (int, error) {
			return 0, os.ErrClosed
		}).Build()

		msgs := make(chan Message, 1)
		err = readFollow(tmp, time.Unix(0, 0), msgs, nil)
		require.NoError(t, err)
		_, ok := <-msgs
		require.False(t, ok)
	})
}

func TestReadFollow_UnexpectedError_WithMockey(t *testing.T) {
	mockey.PatchConvey("readFollow returns error on unexpected read error", t, func() {
		tmp, err := os.CreateTemp("", "kmsg")
		require.NoError(t, err)
		defer func() {
			_ = tmp.Close()
			_ = os.Remove(tmp.Name())
		}()

		mockey.Mock((*os.File).Read).To(func(f *os.File, p []byte) (int, error) {
			return 0, errors.New("read failed")
		}).Build()

		msgs := make(chan Message, 1)
		err = readFollow(tmp, time.Unix(0, 0), msgs, nil)
		require.Error(t, err)
	})
}

func TestReadFollow_DeduperSkipsDuplicates_WithMockey(t *testing.T) {
	mockey.PatchConvey("readFollow skips duplicate messages", t, func() {
		tmp, err := os.CreateTemp("", "kmsg")
		require.NoError(t, err)
		defer func() {
			_ = tmp.Close()
			_ = os.Remove(tmp.Name())
		}()

		var calls int32
		mockey.Mock((*os.File).Read).To(func(f *os.File, p []byte) (int, error) {
			switch atomic.AddInt32(&calls, 1) {
			case 1:
				line := []byte("6,1,100,-;dup-line")
				copy(p, line)
				return len(line), nil
			case 2:
				line := []byte("6,2,101,-;dup-line")
				copy(p, line)
				return len(line), nil
			default:
				return 0, io.EOF
			}
		}).Build()

		msgs := make(chan Message, 2)
		err = readFollow(tmp, time.Unix(0, 0), msgs, newDeduper(time.Minute, time.Minute))
		require.NoError(t, err)

		var collected []Message
		for msg := range msgs {
			collected = append(collected, msg)
		}
		require.Len(t, collected, 1)
		require.Equal(t, "dup-line", collected[0].Message)
	})
}
