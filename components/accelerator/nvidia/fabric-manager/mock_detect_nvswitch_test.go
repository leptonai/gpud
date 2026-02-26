package fabricmanager

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

type mockProcess struct {
	startErr     error
	closeErr     error
	stdoutReader io.Reader
}

func (m *mockProcess) Start(_ context.Context) error { return m.startErr }
func (m *mockProcess) Started() bool                 { return true }
func (m *mockProcess) StartAndWaitForCombinedOutput(_ context.Context) ([]byte, error) {
	return nil, nil
}
func (m *mockProcess) Close(_ context.Context) error { return m.closeErr }
func (m *mockProcess) Closed() bool                  { return false }
func (m *mockProcess) Wait() <-chan error {
	ch := make(chan error, 1)
	ch <- nil
	close(ch)
	return ch
}
func (m *mockProcess) PID() int32      { return 1 }
func (m *mockProcess) ExitCode() int32 { return 0 }
func (m *mockProcess) StdoutReader() io.Reader {
	if m.stdoutReader == nil {
		return bytes.NewReader(nil)
	}
	return m.stdoutReader
}
func (m *mockProcess) StderrReader() io.Reader { return bytes.NewReader(nil) }

func TestListPCINVSwitchesAndCountSMINVSwitches_Wrappers(t *testing.T) {
	lockMockeyPatch(t)

	mockey.PatchConvey("ListPCINVSwitches wrapper calls listPCIs path", t, func() {
		mockey.Mock(file.LocateExecutable).Return("/usr/bin/lspci", nil).Build()
		mockey.Mock(process.New).Return(&mockProcess{}, nil).Build()
		mockey.Mock(process.Read).Return(nil).Build()

		lines, err := ListPCINVSwitches(context.Background())
		require.NoError(t, err)
		assert.Empty(t, lines)
	})

	mockey.PatchConvey("CountSMINVSwitches wrapper calls countSMINVSwitches path", t, func() {
		mockey.Mock(file.LocateExecutable).Return("/usr/bin/nvidia-smi", nil).Build()
		mockey.Mock(process.New).Return(&mockProcess{}, nil).Build()
		mockey.Mock(process.Read).Return(nil).Build()

		lines, err := CountSMINVSwitches(context.Background())
		require.NoError(t, err)
		assert.Empty(t, lines)
	})
}

func TestListPCIs_ErrorsWithMockey(t *testing.T) {
	lockMockeyPatch(t)

	mockey.PatchConvey("lspci executable not found", t, func() {
		mockey.Mock(file.LocateExecutable).Return("", errors.New("not found")).Build()

		lines, err := listPCIs(context.Background(), "lspci -nn", isNVIDIANVSwitchPCI)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to locate lspci")
		assert.Nil(t, lines)
	})

	mockey.PatchConvey("process.New failure", t, func() {
		mockey.Mock(file.LocateExecutable).Return("/usr/bin/lspci", nil).Build()
		mockey.Mock(process.New).Return(nil, errors.New("new failed")).Build()

		lines, err := listPCIs(context.Background(), "lspci -nn", isNVIDIANVSwitchPCI)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "new failed")
		assert.Nil(t, lines)
	})

	mockey.PatchConvey("process start failure", t, func() {
		mockey.Mock(file.LocateExecutable).Return("/usr/bin/lspci", nil).Build()
		mockey.Mock(process.New).Return(&mockProcess{startErr: errors.New("start failed")}, nil).Build()

		lines, err := listPCIs(context.Background(), "lspci -nn", isNVIDIANVSwitchPCI)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "start failed")
		assert.Nil(t, lines)
	})

	mockey.PatchConvey("process read failure", t, func() {
		mockey.Mock(file.LocateExecutable).Return("/usr/bin/lspci", nil).Build()
		mockey.Mock(process.New).Return(&mockProcess{}, nil).Build()
		mockey.Mock(process.Read).Return(errors.New("read failed")).Build()

		lines, err := listPCIs(context.Background(), "lspci -nn", isNVIDIANVSwitchPCI)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to read lspci output")
		assert.Nil(t, lines)
	})

	mockey.PatchConvey("close failure is ignored", t, func() {
		mockey.Mock(file.LocateExecutable).Return("/usr/bin/lspci", nil).Build()
		mockey.Mock(process.New).Return(&mockProcess{closeErr: errors.New("close failed")}, nil).Build()
		mockey.Mock(process.Read).Return(nil).Build()

		lines, err := listPCIs(context.Background(), "lspci -nn", isNVIDIANVSwitchPCI)
		require.NoError(t, err)
		assert.Empty(t, lines)
	})
}

func TestCountSMINVSwitches_ErrorsWithMockey(t *testing.T) {
	lockMockeyPatch(t)

	mockey.PatchConvey("nvidia-smi executable not found", t, func() {
		mockey.Mock(file.LocateExecutable).Return("", errors.New("not found")).Build()

		lines, err := countSMINVSwitches(context.Background(), "nvidia-smi nvlink --status")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to locate nvidia-smi")
		assert.Nil(t, lines)
	})

	mockey.PatchConvey("process.New failure", t, func() {
		mockey.Mock(file.LocateExecutable).Return("/usr/bin/nvidia-smi", nil).Build()
		mockey.Mock(process.New).Return(nil, errors.New("new failed")).Build()

		lines, err := countSMINVSwitches(context.Background(), "nvidia-smi nvlink --status")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "new failed")
		assert.Nil(t, lines)
	})

	mockey.PatchConvey("process start failure", t, func() {
		mockey.Mock(file.LocateExecutable).Return("/usr/bin/nvidia-smi", nil).Build()
		mockey.Mock(process.New).Return(&mockProcess{startErr: errors.New("start failed")}, nil).Build()

		lines, err := countSMINVSwitches(context.Background(), "nvidia-smi nvlink --status")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "start failed")
		assert.Nil(t, lines)
	})

	mockey.PatchConvey("process read failure", t, func() {
		mockey.Mock(file.LocateExecutable).Return("/usr/bin/nvidia-smi", nil).Build()
		mockey.Mock(process.New).Return(&mockProcess{}, nil).Build()
		mockey.Mock(process.Read).Return(errors.New("read failed")).Build()

		lines, err := countSMINVSwitches(context.Background(), "nvidia-smi nvlink --status")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to read nvidia-smi nvlink output")
		assert.Nil(t, lines)
	})

	mockey.PatchConvey("close failure is ignored", t, func() {
		mockey.Mock(file.LocateExecutable).Return("/usr/bin/nvidia-smi", nil).Build()
		mockey.Mock(process.New).Return(&mockProcess{closeErr: errors.New("close failed")}, nil).Build()
		mockey.Mock(process.Read).Return(nil).Build()

		lines, err := countSMINVSwitches(context.Background(), "nvidia-smi nvlink --status")
		require.NoError(t, err)
		assert.Empty(t, lines)
	})
}

func TestListPCIs_FiltersNonNVIDIAVendors_WithMockey(t *testing.T) {
	lockMockeyPatch(t)

	data := []byte("0000:00:1f.0 ISA bridge [0601]: Intel Corporation Device [8086:1234]\n0005:00:00.0 Bridge [0680]: NVIDIA Corporation Device [10de:1af1] (rev a1)")
	mockey.PatchConvey("listPCIs filters non-NVIDIA vendors", t, func() {
		mockey.Mock(file.LocateExecutable).Return("/usr/bin/lspci", nil).Build()
		mockey.Mock(process.New).Return(&mockProcess{
			stdoutReader: bytes.NewReader(data),
		}, nil).Build()
		lines, err := listPCIs(context.Background(), "lspci -nn", isNVIDIANVSwitchPCI)
		require.NoError(t, err)
		require.Len(t, lines, 1)
		assert.Contains(t, lines[0], "10de")
		assert.Contains(t, lines[0], "NVIDIA")
	})
}

func TestCountSMINVSwitches_FiltersNonGPULines_WithMockey(t *testing.T) {
	lockMockeyPatch(t)

	data := []byte("GPU 0: NVIDIA A100-SXM4-80GB (UUID: GPU-1)\nGPU 0 Link 0: 25.781 GB/s\nGPU 1: NVIDIA A100-SXM4-80GB (UUID: GPU-2)")
	mockey.PatchConvey("countSMINVSwitches keeps only GPU descriptor lines", t, func() {
		mockey.Mock(file.LocateExecutable).Return("/usr/bin/nvidia-smi", nil).Build()
		mockey.Mock(process.New).Return(&mockProcess{
			stdoutReader: bytes.NewReader(data),
		}, nil).Build()
		lines, err := countSMINVSwitches(context.Background(), "nvidia-smi nvlink --status")
		require.NoError(t, err)
		require.Len(t, lines, 2)
		assert.Contains(t, lines[0], "GPU 0")
		assert.Contains(t, lines[1], "GPU 1")
	})
}
