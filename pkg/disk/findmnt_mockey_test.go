package disk

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	"github.com/bytedance/mockey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/leptonai/gpud/pkg/file"
	"github.com/leptonai/gpud/pkg/process"
)

type stubProcess struct {
	started  bool
	closed   bool
	startErr error
	stdout   io.Reader
	stderr   io.Reader
	waitCh   chan error
}

func (s *stubProcess) Start(context.Context) error { s.started = true; return s.startErr }
func (s *stubProcess) Started() bool               { return s.started }
func (s *stubProcess) StartAndWaitForCombinedOutput(context.Context) ([]byte, error) {
	return nil, nil
}
func (s *stubProcess) Close(context.Context) error { s.closed = true; return nil }
func (s *stubProcess) Closed() bool                { return s.closed }
func (s *stubProcess) Wait() <-chan error          { return s.waitCh }
func (s *stubProcess) PID() int32                  { return 0 }
func (s *stubProcess) ExitCode() int32             { return 0 }
func (s *stubProcess) StdoutReader() io.Reader     { return s.stdout }
func (s *stubProcess) StderrReader() io.Reader     { return s.stderr }

func newStubProcess(output string) *stubProcess {
	ch := make(chan error)
	close(ch)
	return &stubProcess{
		stdout: bytes.NewBufferString(output),
		stderr: bytes.NewBufferString(""),
		waitCh: ch,
	}
}

func TestFindMnt_LocateExecutableErrorWithMockey(t *testing.T) {
	mockey.PatchConvey("FindMnt returns locate executable error", t, func() {
		mockey.Mock(file.LocateExecutable).To(func(bin string) (string, error) {
			return "", errors.New("not found")
		}).Build()

		out, err := FindMnt(context.Background(), "/")
		require.Error(t, err)
		assert.Nil(t, out)
	})
}

func TestFindMnt_ProcessStartErrorWithMockey(t *testing.T) {
	mockey.PatchConvey("FindMnt returns start error", t, func() {
		mockey.Mock(file.LocateExecutable).To(func(bin string) (string, error) {
			return "/usr/bin/findmnt", nil
		}).Build()

		mockey.Mock(process.New).To(func(opts ...process.OpOption) (process.Process, error) {
			proc := newStubProcess("{}")
			proc.startErr = errors.New("start failed")
			return proc, nil
		}).Build()

		out, err := FindMnt(context.Background(), "/")
		require.Error(t, err)
		assert.Nil(t, out)
	})
}

func TestFindMnt_ParseErrorWithMockey(t *testing.T) {
	mockey.PatchConvey("FindMnt returns parse error", t, func() {
		mockey.Mock(file.LocateExecutable).To(func(bin string) (string, error) {
			return "/usr/bin/findmnt", nil
		}).Build()

		mockey.Mock(process.New).To(func(opts ...process.OpOption) (process.Process, error) {
			return newStubProcess("not-json"), nil
		}).Build()

		out, err := FindMnt(context.Background(), "/")
		require.Error(t, err)
		assert.Nil(t, out)
	})
}

func TestFindMnt_SuccessWithMockey(t *testing.T) {
	mockey.PatchConvey("FindMnt success", t, func() {
		mockey.Mock(file.LocateExecutable).To(func(bin string) (string, error) {
			return "/usr/bin/findmnt", nil
		}).Build()

		jsonOutput := `{"filesystems":[{"target":"/","source":"/dev/sda1","fstype":"ext4","size":"10G","used":"1G","avail":"9G","use%":"10%"}]}`
		mockey.Mock(process.New).To(func(opts ...process.OpOption) (process.Process, error) {
			return newStubProcess(jsonOutput), nil
		}).Build()

		out, err := FindMnt(context.Background(), "/")
		require.NoError(t, err)
		require.NotNil(t, out)
		assert.Equal(t, "/", out.Target)
		require.Len(t, out.Filesystems, 1)
		assert.Equal(t, "/", out.Filesystems[0].MountedPoint)
		assert.Equal(t, "ext4", out.Filesystems[0].Fstype)
	})
}
