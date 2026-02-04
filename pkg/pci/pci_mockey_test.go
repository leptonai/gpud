package pci

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
		stdout: bytes.NewBufferString(output),
		stderr: bytes.NewBufferString(""),
		waitCh: ch,
	}
}

func TestList_LocateExecutableErrorWithMockey(t *testing.T) {
	mockey.PatchConvey("List returns nil when lspci missing", t, func() {
		mockey.Mock(file.LocateExecutable).To(func(bin string) (string, error) {
			return "", errors.New("not found")
		}).Build()

		devs, err := List(context.Background())
		require.NoError(t, err)
		assert.Nil(t, devs)
	})
}

func TestList_ProcessNewErrorWithMockey(t *testing.T) {
	mockey.PatchConvey("List returns error on process.New", t, func() {
		mockey.Mock(file.LocateExecutable).To(func(bin string) (string, error) {
			return "/usr/bin/lspci", nil
		}).Build()
		mockey.Mock(process.New).To(func(opts ...process.OpOption) (process.Process, error) {
			return nil, errors.New("new failed")
		}).Build()

		devs, err := List(context.Background())
		require.Error(t, err)
		assert.Nil(t, devs)
	})
}

func TestList_SuccessWithMockey(t *testing.T) {
	mockey.PatchConvey("List parses lspci output", t, func() {
		mockey.Mock(file.LocateExecutable).To(func(bin string) (string, error) {
			return "/usr/bin/lspci", nil
		}).Build()

		output := "00:00.0 Host bridge: Intel Corporation Device 1234\n\tKernel driver in use: pcieport\n\tKernel modules: pcieport\n"
		mockey.Mock(process.New).To(func(opts ...process.OpOption) (process.Process, error) {
			return newStubProcess(output), nil
		}).Build()

		devs, err := List(context.Background())
		require.NoError(t, err)
		require.Len(t, devs, 1)
		assert.Equal(t, "00:00.0", devs[0].ID)
		assert.Contains(t, devs[0].Name, "Host bridge")
		assert.Equal(t, "pcieport", devs[0].KernelDriverInUse)
		assert.Equal(t, []string{"pcieport"}, devs[0].KernelModules)
	})
}
