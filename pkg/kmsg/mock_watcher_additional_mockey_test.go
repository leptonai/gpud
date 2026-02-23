package kmsg

import (
	"context"
	"errors"
	"os"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/bytedance/mockey"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/shirou/gopsutil/v4/host"
)

type rawConnControlError struct {
	err error
}

func (r rawConnControlError) Control(func(uintptr)) error { return r.err }
func (r rawConnControlError) Read(func(uintptr) bool) error {
	return nil
}
func (r rawConnControlError) Write(func(uintptr) bool) error {
	return nil
}

func TestNewWatcher_Success_WithMockey(t *testing.T) {
	mockey.PatchConvey("NewWatcher succeeds and closes", t, func() {
		tmp, err := os.CreateTemp("", "kmsg")
		require.NoError(t, err)
		defer func() {
			_ = tmp.Close()
			_ = os.Remove(tmp.Name())
		}()

		orig := kmsgFilePath
		kmsgFilePath = tmp.Name()
		defer func() {
			kmsgFilePath = orig
		}()

		mockey.Mock(host.UptimeWithContext).To(func(ctx context.Context) (uint64, error) {
			return 10, nil
		}).Build()

		w, err := NewWatcher(WithCacheKeyTruncateSeconds(120))
		require.NoError(t, err)
		require.NotNil(t, w)
		require.NoError(t, w.Close())
	})
}

func TestWatcher_WatchSuccess_WithMockey(t *testing.T) {
	mockey.PatchConvey("Watch starts readFollow and emits messages", t, func() {
		tmp, err := os.CreateTemp("", "kmsg")
		require.NoError(t, err)
		defer func() {
			_ = tmp.Close()
			_ = os.Remove(tmp.Name())
		}()

		w := &watcher{
			kmsgFile:    tmp,
			bootTime:    time.Unix(0, 0),
			deduperOpts: []OpOption{WithCacheKeyTruncateSeconds(120)},
		}

		mockey.Mock(readFollow).To(func(file *os.File, bootTime time.Time, msgs chan<- Message, d *deduper) error {
			require.Equal(t, tmp, file)
			require.Equal(t, 120, d.cacheKeyTruncateSeconds)
			msgs <- Message{
				Timestamp: metav1.NewTime(time.Unix(1, 0)),
				Message:   "hello",
			}
			close(msgs)
			return nil
		}).Build()

		ch, err := w.Watch()
		require.NoError(t, err)
		msg := <-ch
		require.Equal(t, "hello", msg.Message)
		_, ok := <-ch
		require.False(t, ok)
	})
}

func TestReadAll_SyscallConnError_WithMockey(t *testing.T) {
	mockey.PatchConvey("readAll returns error when SyscallConn fails", t, func() {
		tmp, err := os.CreateTemp("", "kmsg")
		require.NoError(t, err)
		defer func() {
			_ = tmp.Close()
			_ = os.Remove(tmp.Name())
		}()

		mockey.Mock((*os.File).SyscallConn).To(func(f *os.File) (syscall.RawConn, error) {
			return nil, errors.New("syscallconn failed")
		}).Build()

		_, err = readAll(tmp, time.Unix(0, 0), nil)
		require.Error(t, err)
	})
}

func TestReadAll_ControlError_WithMockey(t *testing.T) {
	mockey.PatchConvey("readAll returns error when Control fails", t, func() {
		tmp, err := os.CreateTemp("", "kmsg")
		require.NoError(t, err)
		defer func() {
			_ = tmp.Close()
			_ = os.Remove(tmp.Name())
		}()

		mockey.Mock((*os.File).SyscallConn).To(func(f *os.File) (syscall.RawConn, error) {
			return rawConnControlError{err: errors.New("control failed")}, nil
		}).Build()

		_, err = readAll(tmp, time.Unix(0, 0), nil)
		require.Error(t, err)
	})
}

func TestReadAll_SetNonblockError_WithMockey(t *testing.T) {
	mockey.PatchConvey("readAll returns error when SetNonblock fails", t, func() {
		tmp, err := os.CreateTemp("", "kmsg")
		require.NoError(t, err)
		defer func() {
			_ = tmp.Close()
			_ = os.Remove(tmp.Name())
		}()

		mockey.Mock(syscall.SetNonblock).To(func(fd int, nonblocking bool) error {
			return errors.New("setnonblock failed")
		}).Build()

		_, err = readAll(tmp, time.Unix(0, 0), nil)
		require.Error(t, err)
	})
}

func TestReadAll_SuccessWithDeduper_WithMockey(t *testing.T) {
	mockey.PatchConvey("readAll handles empty lines and dedupes", t, func() {
		tmp, err := os.CreateTemp("", "kmsg")
		require.NoError(t, err)
		defer func() {
			_ = tmp.Close()
			_ = os.Remove(tmp.Name())
		}()

		mockey.Mock(syscall.SetNonblock).Return(nil).Build()

		var calls int32
		mockey.Mock(syscall.Read).To(func(fd int, p []byte) (int, error) {
			switch atomic.AddInt32(&calls, 1) {
			case 1:
				return 0, nil
			case 2:
				line := []byte("6,1,100,-;dup-message")
				copy(p, line)
				return len(line), nil
			case 3:
				line := []byte("6,2,101,-;dup-message")
				copy(p, line)
				return len(line), nil
			default:
				return 0, syscall.EAGAIN
			}
		}).Build()

		msgs, err := readAll(tmp, time.Unix(0, 0), newDeduper(time.Minute, time.Minute))
		require.NoError(t, err)
		require.Len(t, msgs, 1)
		require.Equal(t, "dup-message", msgs[0].Message)
	})
}

func TestReadAll_ParseError_WithMockey(t *testing.T) {
	mockey.PatchConvey("readAll returns error on malformed line", t, func() {
		tmp, err := os.CreateTemp("", "kmsg")
		require.NoError(t, err)
		defer func() {
			_ = tmp.Close()
			_ = os.Remove(tmp.Name())
		}()

		mockey.Mock(syscall.SetNonblock).Return(nil).Build()
		mockey.Mock(syscall.Read).To(func(fd int, p []byte) (int, error) {
			line := []byte("bad line")
			copy(p, line)
			return len(line), nil
		}).Build()

		_, err = readAll(tmp, time.Unix(0, 0), nil)
		require.Error(t, err)
	})
}

func TestReadAll_UnexpectedReadError_WithMockey(t *testing.T) {
	mockey.PatchConvey("readAll returns error on unexpected read error", t, func() {
		tmp, err := os.CreateTemp("", "kmsg")
		require.NoError(t, err)
		defer func() {
			_ = tmp.Close()
			_ = os.Remove(tmp.Name())
		}()

		mockey.Mock(syscall.SetNonblock).Return(nil).Build()
		mockey.Mock(syscall.Read).To(func(fd int, p []byte) (int, error) {
			return 0, errors.New("read boom")
		}).Build()

		_, err = readAll(tmp, time.Unix(0, 0), nil)
		require.Error(t, err)
	})
}

func TestReadAll_WrapperSuccess_WithMockey(t *testing.T) {
	mockey.PatchConvey("ReadAll succeeds with mocked syscalls", t, func() {
		tmp, err := os.CreateTemp("", "kmsg")
		require.NoError(t, err)
		defer func() {
			_ = tmp.Close()
			_ = os.Remove(tmp.Name())
		}()

		orig := kmsgFilePath
		kmsgFilePath = tmp.Name()
		defer func() {
			kmsgFilePath = orig
		}()

		mockey.Mock(host.BootTimeWithContext).To(func(ctx context.Context) (uint64, error) {
			return 0, nil
		}).Build()
		mockey.Mock(syscall.SetNonblock).Return(nil).Build()

		var calls int32
		mockey.Mock(syscall.Read).To(func(fd int, p []byte) (int, error) {
			if atomic.AddInt32(&calls, 1) == 1 {
				line := []byte("6,1,100,-;hello")
				copy(p, line)
				return len(line), nil
			}
			return 0, syscall.EAGAIN
		}).Build()

		msgs, err := ReadAll(context.Background())
		require.NoError(t, err)
		require.Len(t, msgs, 1)
		require.Equal(t, "hello", msgs[0].Message)
	})
}

func TestReadAll_BootTimeError_WithMockey(t *testing.T) {
	mockey.PatchConvey("ReadAll returns error when boot time lookup fails", t, func() {
		tmp, err := os.CreateTemp("", "kmsg")
		require.NoError(t, err)
		defer func() {
			_ = tmp.Close()
			_ = os.Remove(tmp.Name())
		}()

		orig := kmsgFilePath
		kmsgFilePath = tmp.Name()
		defer func() {
			kmsgFilePath = orig
		}()

		mockey.Mock(host.BootTimeWithContext).To(func(ctx context.Context) (uint64, error) {
			return 0, errors.New("boot time failed")
		}).Build()

		_, err = ReadAll(context.Background())
		require.Error(t, err)
	})
}
