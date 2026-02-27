package disk

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"testing"

	"github.com/bytedance/mockey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/leptonai/gpud/pkg/file"
	"github.com/leptonai/gpud/pkg/process"
)

type lsblkProcessStub struct {
	started bool
	closed  bool

	startErr    error
	combinedOut []byte
	combinedErr error

	stdout io.Reader
	stderr io.Reader
	waitCh chan error
}

func (s *lsblkProcessStub) Start(context.Context) error { s.started = true; return s.startErr }
func (s *lsblkProcessStub) Started() bool               { return s.started }
func (s *lsblkProcessStub) StartAndWaitForCombinedOutput(context.Context) ([]byte, error) {
	return s.combinedOut, s.combinedErr
}
func (s *lsblkProcessStub) Close(context.Context) error { s.closed = true; return nil }
func (s *lsblkProcessStub) Closed() bool                { return s.closed }
func (s *lsblkProcessStub) Wait() <-chan error          { return s.waitCh }
func (s *lsblkProcessStub) PID() int32                  { return 0 }
func (s *lsblkProcessStub) ExitCode() int32             { return 0 }
func (s *lsblkProcessStub) StdoutReader() io.Reader     { return s.stdout }
func (s *lsblkProcessStub) StderrReader() io.Reader     { return s.stderr }

func newLsblkProcessStub(stdout string) *lsblkProcessStub {
	ch := make(chan error, 1)
	ch <- nil
	close(ch)
	return &lsblkProcessStub{
		stdout: bytes.NewBufferString(stdout),
		stderr: bytes.NewBuffer(nil),
		waitCh: ch,
	}
}

func TestExecuteLsblkCommand_WithMockey(t *testing.T) {
	mockey.PatchConvey("executeLsblkCommand returns process.New error", t, func() {
		mockey.Mock(process.New).To(func(opts ...process.OpOption) (process.Process, error) {
			return nil, errors.New("new failed")
		}).Build()

		_, err := executeLsblkCommand(context.Background(), "/usr/bin/lsblk", "--json")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "new failed")
	})

	mockey.PatchConvey("executeLsblkCommand wraps command execution errors", t, func() {
		mockey.Mock(process.New).To(func(opts ...process.OpOption) (process.Process, error) {
			stub := newLsblkProcessStub("")
			stub.combinedErr = errors.New("command failed")
			return stub, nil
		}).Build()

		_, err := executeLsblkCommand(context.Background(), "/usr/bin/lsblk", "--json")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to run lsblk command")
		assert.Contains(t, err.Error(), "command failed")
	})

	mockey.PatchConvey("executeLsblkCommand returns command output", t, func() {
		mockey.Mock(process.New).To(func(opts ...process.OpOption) (process.Process, error) {
			stub := newLsblkProcessStub("")
			stub.combinedOut = []byte(`{"blockdevices":[]}`)
			return stub, nil
		}).Build()

		out, err := executeLsblkCommand(context.Background(), "/usr/bin/lsblk", "--json")
		require.NoError(t, err)
		assert.Equal(t, `{"blockdevices":[]}`, string(out))
	})
}

func TestGetLsblkBinPathAndVersion_WithMockey(t *testing.T) {
	mockey.PatchConvey("getLsblkBinPathAndVersion returns locate executable error", t, func() {
		mockey.Mock(file.LocateExecutable).To(func(bin string) (string, error) {
			return "", errors.New("not found")
		}).Build()

		_, _, err := getLsblkBinPathAndVersion(context.Background())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})

	mockey.PatchConvey("getLsblkBinPathAndVersion returns process.New error", t, func() {
		mockey.Mock(file.LocateExecutable).To(func(bin string) (string, error) {
			return "/usr/bin/lsblk", nil
		}).Build()
		mockey.Mock(process.New).To(func(opts ...process.OpOption) (process.Process, error) {
			return nil, errors.New("process create failed")
		}).Build()

		_, _, err := getLsblkBinPathAndVersion(context.Background())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "process create failed")
	})

	mockey.PatchConvey("getLsblkBinPathAndVersion returns start error", t, func() {
		mockey.Mock(file.LocateExecutable).To(func(bin string) (string, error) {
			return "/usr/bin/lsblk", nil
		}).Build()
		mockey.Mock(process.New).To(func(opts ...process.OpOption) (process.Process, error) {
			stub := newLsblkProcessStub("")
			stub.startErr = errors.New("start failed")
			return stub, nil
		}).Build()

		_, _, err := getLsblkBinPathAndVersion(context.Background())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "start failed")
	})

	mockey.PatchConvey("getLsblkBinPathAndVersion wraps read errors", t, func() {
		mockey.Mock(file.LocateExecutable).To(func(bin string) (string, error) {
			return "/usr/bin/lsblk", nil
		}).Build()
		mockey.Mock(process.New).To(func(opts ...process.OpOption) (process.Process, error) {
			stub := newLsblkProcessStub("")
			return stub, nil
		}).Build()
		mockey.Mock(process.Read).To(func(ctx context.Context, p process.Process, opts ...process.ReadOpOption) error {
			return fmt.Errorf("read failed")
		}).Build()

		_, _, err := getLsblkBinPathAndVersion(context.Background())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to check lsblk version")
		assert.Contains(t, err.Error(), "read failed")
	})

	mockey.PatchConvey("getLsblkBinPathAndVersion returns trimmed version output", t, func() {
		mockey.Mock(file.LocateExecutable).To(func(bin string) (string, error) {
			return "/usr/bin/lsblk", nil
		}).Build()
		mockey.Mock(process.New).To(func(opts ...process.OpOption) (process.Process, error) {
			return newLsblkProcessStub(" lsblk from util-linux 2.39 \n"), nil
		}).Build()

		bin, ver, err := getLsblkBinPathAndVersion(context.Background())
		require.NoError(t, err)
		assert.Equal(t, "/usr/bin/lsblk", bin)
		assert.Equal(t, "lsblk from util-linux 2.39", ver)
	})
}

func TestGetBlockDevicesWithLsblk_Wrapper_WithMockey(t *testing.T) {
	mockey.PatchConvey("GetBlockDevicesWithLsblk delegates to getBlockDevicesWithLsblk", t, func() {
		expected := BlockDevices{
			{Name: "/dev/sda", Type: "disk"},
		}
		mockey.Mock(getBlockDevicesWithLsblk).To(
			func(ctx context.Context, deps getBlockDevicesDeps, opts ...OpOption) (BlockDevices, error) {
				return expected, nil
			},
		).Build()

		got, err := GetBlockDevicesWithLsblk(context.Background())
		require.NoError(t, err)
		assert.Equal(t, expected, got)
	})
}
