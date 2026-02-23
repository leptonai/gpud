package pci

import (
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

// mockProcess is a minimal mock implementation of the Process interface for testing.
type mockProcess struct {
	startErr error
	closeErr error
}

func (m *mockProcess) Start(ctx context.Context) error {
	if m.startErr != nil {
		return m.startErr
	}
	return nil
}
func (m *mockProcess) Started() bool { return true }
func (m *mockProcess) StartAndWaitForCombinedOutput(ctx context.Context) ([]byte, error) {
	return nil, nil
}
func (m *mockProcess) Close(ctx context.Context) error {
	if m.closeErr != nil {
		return m.closeErr
	}
	return nil
}
func (m *mockProcess) Closed() bool            { return true }
func (m *mockProcess) Wait() <-chan error      { ch := make(chan error, 1); close(ch); return ch }
func (m *mockProcess) PID() int32              { return 12345 }
func (m *mockProcess) ExitCode() int32         { return 0 }
func (m *mockProcess) StdoutReader() io.Reader { return nil }
func (m *mockProcess) StderrReader() io.Reader { return nil }

// TestDeviceVendorID tests the vendor ID constant is correct.
func TestDeviceVendorID(t *testing.T) {
	assert.Equal(t, "10de", DeviceVendorID, "NVIDIA vendor ID should be 10de")
}

// TestListPCIs_LspciNotFound tests the lspci not found error path.
func TestListPCIs_LspciNotFound(t *testing.T) {
	mockey.PatchConvey("lspci not found", t, func() {
		mockey.Mock(file.LocateExecutable).Return("", errors.New("executable not found")).Build()

		ctx := context.Background()
		gpus, err := ListPCIGPUs(ctx)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to locate lspci")
		assert.Nil(t, gpus)
	})
}

// TestListPCIs_LspciEmptyPath tests when LocateExecutable returns empty path.
func TestListPCIs_LspciEmptyPath(t *testing.T) {
	mockey.PatchConvey("lspci empty path", t, func() {
		mockey.Mock(file.LocateExecutable).Return("", nil).Build()

		ctx := context.Background()
		gpus, err := ListPCIGPUs(ctx)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to locate lspci")
		assert.Nil(t, gpus)
	})
}

// TestListPCIs_ProcessNewError tests when process creation fails.
func TestListPCIs_ProcessNewError(t *testing.T) {
	mockey.PatchConvey("process new error", t, func() {
		mockey.Mock(file.LocateExecutable).Return("/usr/bin/lspci", nil).Build()
		mockey.Mock(process.New).Return(nil, errors.New("failed to create process")).Build()

		ctx := context.Background()
		gpus, err := ListPCIGPUs(ctx)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create process")
		assert.Nil(t, gpus)
	})
}

// TestListPCIs_ProcessStartError tests when process start fails.
func TestListPCIs_ProcessStartError(t *testing.T) {
	mockey.PatchConvey("process start error", t, func() {
		mockey.Mock(file.LocateExecutable).Return("/usr/bin/lspci", nil).Build()

		mockProc := &mockProcess{startErr: errors.New("failed to start process")}
		mockey.Mock(process.New).Return(mockProc, nil).Build()

		ctx := context.Background()
		gpus, err := ListPCIGPUs(ctx)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to start process")
		assert.Nil(t, gpus)
	})
}

// TestListPCIs_ProcessReadError tests when reading process output fails.
func TestListPCIs_ProcessReadError(t *testing.T) {
	mockey.PatchConvey("process read error", t, func() {
		mockey.Mock(file.LocateExecutable).Return("/usr/bin/lspci", nil).Build()

		mockProc := &mockProcess{}
		mockey.Mock(process.New).Return(mockProc, nil).Build()
		mockey.Mock(process.Read).Return(errors.New("failed to read output")).Build()

		ctx := context.Background()
		gpus, err := ListPCIGPUs(ctx)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to read lspci output")
		assert.Nil(t, gpus)
	})
}

// TestListPCIs_SuccessWithNoGPUs tests successful execution but no GPUs found.
func TestListPCIs_SuccessWithNoGPUs(t *testing.T) {
	mockey.PatchConvey("success with no GPUs", t, func() {
		mockey.Mock(file.LocateExecutable).Return("/usr/bin/lspci", nil).Build()

		mockProc := &mockProcess{}
		mockey.Mock(process.New).Return(mockProc, nil).Build()

		// Mock process.Read to simulate successful execution with no matching lines
		mockey.Mock(process.Read).Return(nil).Build()

		ctx := context.Background()
		gpus, err := ListPCIGPUs(ctx)

		require.NoError(t, err)
		assert.NotNil(t, gpus)
		assert.Equal(t, 0, len(gpus))
	})
}

// TestListPCIs_CloseError tests that close errors are logged but don't fail the operation.
func TestListPCIs_CloseError(t *testing.T) {
	mockey.PatchConvey("close error logged", t, func() {
		mockey.Mock(file.LocateExecutable).Return("/usr/bin/lspci", nil).Build()

		// Mock process that fails on Close
		mockProc := &mockProcess{closeErr: errors.New("failed to close")}
		mockey.Mock(process.New).Return(mockProc, nil).Build()
		mockey.Mock(process.Read).Return(nil).Build()

		ctx := context.Background()
		gpus, err := ListPCIGPUs(ctx)

		// Should still succeed despite close error (it's logged)
		require.NoError(t, err)
		assert.NotNil(t, gpus)
	})
}
