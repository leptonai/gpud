//go:build linux

package peermem

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"testing"

	"github.com/bytedance/mockey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/leptonai/gpud/pkg/process"
)

type stubProcess struct {
	started  bool
	closed   bool
	stdout   io.Reader
	stderr   io.Reader
	startErr error
}

func (s *stubProcess) Start(context.Context) error                                   { s.started = true; return s.startErr }
func (s *stubProcess) Started() bool                                                 { return s.started }
func (s *stubProcess) StartAndWaitForCombinedOutput(context.Context) ([]byte, error) { return nil, nil }
func (s *stubProcess) Close(context.Context) error                                   { s.closed = true; return nil }
func (s *stubProcess) Closed() bool                                                  { return s.closed }
func (s *stubProcess) Wait() <-chan error                                            { ch := make(chan error); close(ch); return ch }
func (s *stubProcess) PID() int32                                                    { return 0 }
func (s *stubProcess) ExitCode() int32                                               { return 0 }
func (s *stubProcess) StdoutReader() io.Reader                                       { return s.stdout }
func (s *stubProcess) StderrReader() io.Reader                                       { return s.stderr }

func TestCheckLsmodPeermemModule_ReadErrorWithMockey(t *testing.T) {
	mockey.PatchConvey("CheckLsmodPeermemModule read error", t, func() {
		mockey.Mock(process.Read).To(func(ctx context.Context, p process.Process, opts ...process.ReadOpOption) error {
			return errors.New("read failed")
		}).Build()

		proc := &stubProcess{
			stdout: bytes.NewBufferString("nvidia_peermem"),
			stderr: bytes.NewBufferString(""),
		}

		out, err := checkLsmodPeermemModule(
			context.Background(),
			func() int { return 0 },
			func(opts ...process.OpOption) (process.Process, error) {
				return proc, nil
			},
		)
		require.Error(t, err)
		assert.Nil(t, out)
		assert.Contains(t, err.Error(), "failed to read lsmod output")
	})
}

func TestCheckLsmodPeermemModule_NotRootWithMockey(t *testing.T) {
	mockey.PatchConvey("CheckLsmodPeermemModule requires root", t, func() {
		mockey.Mock(os.Geteuid).To(func() int { return 1000 }).Build()

		out, err := CheckLsmodPeermemModule(context.Background())
		require.Error(t, err)
		assert.Nil(t, out)
		assert.Contains(t, err.Error(), "requires sudo/root access")
	})
}

func TestCheckLsmodPeermemModule_ProcessNewError_WithMockey(t *testing.T) {
	mockey.PatchConvey("CheckLsmodPeermemModule process.New error (local)", t, func() {
		mockey.Mock(os.Geteuid).To(func() int { return 0 }).Build()
		mockey.Mock(process.New).To(func(opts ...process.OpOption) (process.Process, error) {
			return nil, errors.New("new failed")
		}).Build()

		out, err := CheckLsmodPeermemModule(context.Background())
		require.Error(t, err)
		assert.Nil(t, out)
		assert.Contains(t, err.Error(), "new failed")
	})
}

func TestCheckLsmodPeermemModule_SuccessWithMockey(t *testing.T) {
	mockey.PatchConvey("CheckLsmodPeermemModule success", t, func() {
		mockey.Mock(os.Geteuid).To(func() int { return 0 }).Build()

		proc := &stubProcess{
			stdout: bytes.NewBufferString(`ib_core 434176 9 rdma_cm,ib_ipoib,nvidia_peermem,ib_umad`),
			stderr: bytes.NewBufferString(""),
		}

		mockey.Mock(process.New).To(func(opts ...process.OpOption) (process.Process, error) {
			return proc, nil
		}).Build()

		out, err := CheckLsmodPeermemModule(context.Background())
		require.NoError(t, err)
		require.NotNil(t, out)
		assert.True(t, out.IbcoreUsingPeermemModule)
		assert.Contains(t, out.Raw, "ib_core")
	})
}
