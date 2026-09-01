//go:build linux

package run

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/bytedance/mockey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	gpudmanager "github.com/leptonai/gpud/pkg/gpud-manager"
	"github.com/leptonai/gpud/pkg/login"
	nvidianvml "github.com/leptonai/gpud/pkg/nvidia/nvml"
	nvidiapci "github.com/leptonai/gpud/pkg/nvidia/pci"
)

func stubNVIDIADriverRequirement(
	t *testing.T,
	hasGPU bool,
	detectErr error,
	instance nvidianvml.Instance,
	nvmlErr error,
) {
	t.Helper()
	originalHasGPU := hasNVIDIAGPU
	originalNewNVML := newNVMLInstance
	hasNVIDIAGPU = func() (bool, error) { return hasGPU, detectErr }
	newNVMLInstance = func() (nvidianvml.Instance, error) { return instance, nvmlErr }
	t.Cleanup(func() {
		hasNVIDIAGPU = originalHasGPU
		newNVMLInstance = originalNewNVML
	})
}

func TestRequireNVIDIADriver(t *testing.T) {
	t.Run("CPU-only node does not require NVML", func(t *testing.T) {
		stubNVIDIADriverRequirement(t, false, nil, nil, errors.New("must not be called"))
		require.NoError(t, requireNVIDIADriver())
	})

	t.Run("PCI detection failure is fatal", func(t *testing.T) {
		stubNVIDIADriverRequirement(t, false, errors.New("sysfs unavailable"), nil, nil)
		err := requireNVIDIADriver()
		require.ErrorContains(t, err, "failed to detect NVIDIA GPU devices")
	})

	t.Run("GPU with missing NVML is rejected", func(t *testing.T) {
		stubNVIDIADriverRequirement(t, true, nil, nvidianvml.NewNoOp(), nil)
		err := requireNVIDIADriver()
		require.ErrorContains(t, err, "NVML library is not loaded")
	})

	t.Run("GPU with NVML initialization error is rejected", func(t *testing.T) {
		stubNVIDIADriverRequirement(t, true, nil, nil, errors.New("driver not ready"))
		err := requireNVIDIADriver()
		require.ErrorContains(t, err, "driver is not ready")
	})

	t.Run("GPU with working NVML is accepted", func(t *testing.T) {
		stubNVIDIADriverRequirement(t, true, nil, &testNVMLInstance{exists: true}, nil)
		require.NoError(t, requireNVIDIADriver())
	})

	t.Run("GPU with NVML device initialization error is rejected", func(t *testing.T) {
		stubNVIDIADriverRequirement(t, true, nil, &testNVMLInstance{exists: true, initErr: errors.New("device enumeration failed")}, nil)
		err := requireNVIDIADriver()
		require.ErrorContains(t, err, "device enumeration failed")
	})

	t.Run("NVML shutdown error is non-fatal", func(t *testing.T) {
		stubNVIDIADriverRequirement(t, true, nil, &testNVMLInstance{exists: true, shutdownErr: errors.New("shutdown failed")}, nil)
		require.NoError(t, requireNVIDIADriver())
	})
}

type testNVMLInstance struct {
	nvidianvml.Instance
	exists      bool
	initErr     error
	shutdownErr error
}

func (i *testNVMLInstance) NVMLExists() bool { return i.exists }
func (i *testNVMLInstance) InitError() error { return i.initErr }
func (i *testNVMLInstance) Shutdown() error  { return i.shutdownErr }

func TestHasNVIDIAGPUReadsDefaultSysfs(t *testing.T) {
	if _, err := os.Stat(nvidiapci.DefaultSysfsPCIDevicesDir); err != nil {
		t.Skipf("sysfs PCI devices directory %q not available", nvidiapci.DefaultSysfsPCIDevicesDir)
	}
	_, err := hasNVIDIAGPU()
	require.NoError(t, err)
}

func TestCommandRequireNVIDIADriverStopsBeforeLogin(t *testing.T) {
	ctx := newTestCLIContext(t, cliFlagValues{
		stringFlags: map[string]string{
			"log-level": "info",
			"data-dir":  t.TempDir(),
			"token":     "registration-token",
		},
		boolFlags: map[string]bool{
			"require-nvidia-driver": true,
		},
	})

	mockey.PatchConvey("driver gate stops before login", t, func() {
		loginCalled := false
		mockey.Mock(ensureNVIDIADriver).Return(errors.New("NVIDIA NVML library is not loaded")).Build()
		mockey.Mock(login.Login).To(func(context.Context, login.LoginConfig) error {
			loginCalled = true
			return nil
		}).Build()

		err := Command(ctx)
		require.ErrorContains(t, err, "NVML library is not loaded")
		assert.False(t, loginCalled)
	})
}

func TestCommandRequireNVIDIADriverSkipsGateWithoutLogin(t *testing.T) {
	ctx := newTestCLIContext(t, cliFlagValues{
		stringFlags: map[string]string{
			"log-level": "info",
			"data-dir":  t.TempDir(),
		},
		boolFlags: map[string]bool{
			"require-nvidia-driver": true,
		},
	})

	mockey.PatchConvey("driver gate only applies to login", t, func() {
		gateCalled := false
		mockey.Mock(ensureNVIDIADriver).To(func() error {
			gateCalled = true
			return errors.New("must not be called")
		}).Build()
		mockey.Mock((*gpudmanager.Manager).Start).Return(errors.New("stop after login gate")).Build()

		err := Command(ctx)
		require.ErrorContains(t, err, "stop after login gate")
		assert.False(t, gateCalled)
	})
}

func TestCommandRequireNVIDIADriverGatePassesBeforeLogin(t *testing.T) {
	ctx := newTestCLIContext(t, cliFlagValues{
		stringFlags: map[string]string{
			"log-level": "info",
			"data-dir":  t.TempDir(),
			"token":     "registration-token",
		},
		boolFlags: map[string]bool{
			"require-nvidia-driver": true,
		},
	})

	mockey.PatchConvey("driver gate passes and login proceeds", t, func() {
		gateCalled := false
		loginCalled := false
		mockey.Mock(ensureNVIDIADriver).To(func() error {
			gateCalled = true
			return nil
		}).Build()
		mockey.Mock(login.Login).To(func(context.Context, login.LoginConfig) error {
			loginCalled = true
			return nil
		}).Build()
		mockey.Mock(recordLoginSuccessState).To(func(context.Context, string) error {
			return nil
		}).Build()
		mockey.Mock((*gpudmanager.Manager).Start).Return(errors.New("stop after driver gate")).Build()

		err := Command(ctx)
		require.ErrorContains(t, err, "stop after driver gate")
		assert.True(t, gateCalled)
		assert.True(t, loginCalled)
	})
}
