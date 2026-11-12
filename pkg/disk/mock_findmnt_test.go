package disk

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"
	"time"

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

type exitedBeforeReadProcess struct {
	process.Process
}

func (p *exitedBeforeReadProcess) Start(ctx context.Context) error {
	if err := p.Process.Start(ctx); err != nil {
		return err
	}
	select {
	case err := <-p.Wait():
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func TestFindMntWithCommandAfterProcessExit(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	p, err := process.New(process.WithBashScriptContentsToRun(`printf '%s\n' '{"filesystems":[{"source":"example:/export","target":"/mnt/nfs-share","fstype":"nfs4","size":"100G","used":"1G","avail":"99G","use%":"1%"}]}'`), process.WithRunBashInline())
	require.NoError(t, err)
	defer func() { _ = p.Close(ctx) }()
	mockey.PatchConvey("findmnt retains JSON when the command exits before reading", t, func() {
		// Force the adverse scheduling order while retaining real process pipes,
		// process.Read, and the findmnt JSON parser. Before the pipe-ownership fix,
		// this valid JSON produced "unexpected end of JSON input" after exit.
		mockey.Mock(process.New).To(func(...process.OpOption) (process.Process, error) {
			return &exitedBeforeReadProcess{Process: p}, nil
		}).Build()
		out, err := FindMntWithCommand(ctx, "/mnt/nfs-share", "findmnt")
		require.NoError(t, err)
		require.Len(t, out.Filesystems, 1)
		require.Equal(t, "/mnt/nfs-share", out.Filesystems[0].MountedPoint)
		require.Equal(t, []string{"example:/export"}, out.Filesystems[0].Sources)
		require.Equal(t, "nfs4", out.Filesystems[0].Fstype)
	})
}
