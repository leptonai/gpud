package controllers

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/bytedance/mockey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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

func (s *stubProcess) Start(context.Context) error                                   { s.started = true; return s.startErr }
func (s *stubProcess) Started() bool                                                 { return s.started }
func (s *stubProcess) StartAndWaitForCombinedOutput(context.Context) ([]byte, error) { return nil, nil }
func (s *stubProcess) Close(context.Context) error                                   { s.closed = true; return nil }
func (s *stubProcess) Closed() bool                                                  { return s.closed }
func (s *stubProcess) Wait() <-chan error                                            { return s.waitCh }
func (s *stubProcess) PID() int32                                                    { return 0 }
func (s *stubProcess) ExitCode() int32                                               { return 0 }
func (s *stubProcess) StdoutReader() io.Reader                                       { return s.stdout }
func (s *stubProcess) StderrReader() io.Reader                                       { return s.stderr }

func newStubProcess(output string) *stubProcess {
	ch := make(chan error)
	close(ch)
	return &stubProcess{
		stdout: strings.NewReader(output),
		stderr: strings.NewReader(""),
		waitCh: ch,
	}
}

func TestRunCommand_ProcessNewErrorWithMockey(t *testing.T) {
	mockey.PatchConvey("runCommand returns process.New error", t, func() {
		mockey.Mock(process.New).To(func(opts ...process.OpOption) (process.Process, error) {
			return nil, errors.New("new failed")
		}).Build()

		err := runCommand(context.Background(), "/tmp/script.sh", "version", nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "new failed")
	})
}

func TestRunCommand_StartErrorWithMockey(t *testing.T) {
	mockey.PatchConvey("runCommand returns start error", t, func() {
		mockey.Mock(process.New).To(func(opts ...process.OpOption) (process.Process, error) {
			proc := newStubProcess("")
			proc.startErr = errors.New("start failed")
			return proc, nil
		}).Build()

		err := runCommand(context.Background(), "/tmp/script.sh", "version", nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "start failed")
	})
}

func TestRunCommand_ReadErrorWithMockey(t *testing.T) {
	mockey.PatchConvey("runCommand captures read error output", t, func() {
		mockey.Mock(process.New).To(func(opts ...process.OpOption) (process.Process, error) {
			return newStubProcess("version-output"), nil
		}).Build()
		mockey.Mock(process.Read).To(func(ctx context.Context, p process.Process, opts ...process.ReadOpOption) error {
			return errors.New("read failed")
		}).Build()

		var result string
		err := runCommand(context.Background(), "/tmp/script.sh", "version", &result)
		require.NoError(t, err)
		assert.Contains(t, result, "failed to run")
		assert.Contains(t, result, "read failed")
	})
}

func TestRunCommand_ResultSuccessWithMockey(t *testing.T) {
	mockey.PatchConvey("runCommand returns output in result", t, func() {
		mockey.Mock(process.New).To(func(opts ...process.OpOption) (process.Process, error) {
			proc := newStubProcess("line1\nline2\n")
			return proc, nil
		}).Build()

		var result string
		err := runCommand(context.Background(), "/tmp/script.sh", "version", &result)
		require.NoError(t, err)
		assert.Equal(t, "line1\nline2", result)
	})
}

func TestRunCommand_OutputFileOpenErrorWithMockey(t *testing.T) {
	mockey.PatchConvey("runCommand returns error when output file cannot be created", t, func() {
		mockey.Mock(os.OpenFile).To(func(name string, flag int, perm os.FileMode) (*os.File, error) {
			return nil, errors.New("open failed")
		}).Build()

		err := runCommand(context.Background(), "/tmp/script.sh", "install", nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "open failed")
	})
}
