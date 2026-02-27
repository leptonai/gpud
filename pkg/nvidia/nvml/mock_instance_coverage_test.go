package nvml

import (
	"context"
	"errors"
	"io"
	"os"
	"testing"
	"time"

	nvlibdevice "github.com/NVIDIA/go-nvlib/pkg/nvlib/device"
	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/NVIDIA/go-nvml/pkg/nvml/mock"
	"github.com/bytedance/mockey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/leptonai/gpud/pkg/nvidia/nvml/device"
	nvmllib "github.com/leptonai/gpud/pkg/nvidia/nvml/lib"
	"github.com/leptonai/gpud/pkg/nvidia/nvml/testutil"
)

type busIDErrorDevice struct {
	*testutil.MockDevice
}

func (d *busIDErrorDevice) GetPCIBusID() (string, error) {
	return "", errors.New("failed to get pci bus id")
}

func newMockLibrary(
	driverVersion string,
	cudaVersion int,
	devs []nvlibdevice.Device,
	devErr error,
) nvmllib.Library {
	return &fullMockLibWrapper{
		nvmlIface: &mock.Interface{
			SystemGetDriverVersionFunc: func() (string, nvml.Return) {
				return driverVersion, nvml.SUCCESS
			},
			SystemGetCudaDriverVersion_v2Func: func() (int, nvml.Return) {
				return cudaVersion, nvml.SUCCESS
			},
		},
		shutdownRet: nvml.SUCCESS,
		devIface:    &mockDevInterface{devices: devs, err: devErr},
		infoIface:   &mockInfoWrapper{hasNvml: true, hasNvmlMsg: "found"},
	}
}

func TestRefreshNVMLAndExit_ExitBranch_WithMockey(t *testing.T) {
	mockey.PatchConvey("refreshNVMLAndExit exits when NVML loads", t, func() {
		tickCh := make(chan time.Time, 1)
		tickCh <- time.Now()

		mockey.Mock(time.NewTicker).To(func(_ time.Duration) *time.Ticker {
			return &time.Ticker{C: tickCh}
		}).Build()
		mockey.Mock(nvmllib.New).To(func(opts ...nvmllib.OpOption) (nvmllib.Library, error) {
			return nil, nil
		}).Build()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		exitCode := -1
		mockey.Mock(os.Exit).To(func(code int) {
			exitCode = code
			cancel()
		}).Build()

		done := make(chan struct{})
		go func() {
			refreshNVMLAndExit(ctx)
			close(done)
		}()

		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("refreshNVMLAndExit did not exit")
		}
		assert.Equal(t, 0, exitCode)
	})
}

func TestRefreshNVMLAndExit_RetryFailureBranch_WithMockey(t *testing.T) {
	mockey.PatchConvey("refreshNVMLAndExit retries and logs failure when NVML load fails", t, func() {
		tickCh := make(chan time.Time, 1)
		tickCh <- time.Now()

		mockey.Mock(time.NewTicker).To(func(_ time.Duration) *time.Ticker {
			return &time.Ticker{C: tickCh}
		}).Build()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		calls := 0
		mockey.Mock(nvmllib.New).To(func(opts ...nvmllib.OpOption) (nvmllib.Library, error) {
			calls++
			cancel()
			return nil, errors.New("not ready")
		}).Build()

		refreshNVMLAndExit(ctx)
		assert.GreaterOrEqual(t, calls, 1)
	})
}

func TestNewInstance_ParseDriverVersionError_WithMockey(t *testing.T) {
	mockey.PatchConvey("newInstance returns parse error when driver version has invalid format", t, func() {
		lib := &fullMockLibWrapper{
			nvmlIface: &mock.Interface{
				SystemGetDriverVersionFunc: func() (string, nvml.Return) {
					return "bad.version", nvml.SUCCESS
				},
			},
			shutdownRet: nvml.SUCCESS,
			devIface:    &mockDevInterface{},
			infoIface:   &mockInfoWrapper{hasNvml: true, hasNvmlMsg: "found"},
		}

		mockey.Mock(nvmllib.New).To(func(opts ...nvmllib.OpOption) (nvmllib.Library, error) {
			return lib, nil
		}).Build()

		inst, err := newInstance(context.Background(), nil, nil)
		require.Error(t, err)
		assert.Nil(t, inst)
	})
}

func TestNewInstance_GetNameError_WithMockey(t *testing.T) {
	mockey.PatchConvey("newInstance returns error when first device name query fails", t, func() {
		dev := testutil.NewMockDevice(
			&mock.Device{
				GetNameFunc: func() (string, nvml.Return) {
					return "", nvml.ERROR_GPU_IS_LOST
				},
				GetUUIDFunc: func() (string, nvml.Return) {
					return "GPU-1", nvml.SUCCESS
				},
			},
			"hopper", "Tesla", "9.0", "0000:01:00.0",
		)
		lib := newMockLibrary("550.120.05", 12040, []nvlibdevice.Device{dev}, nil)

		mockey.Mock(nvmllib.New).To(func(opts ...nvmllib.OpOption) (nvmllib.Library, error) {
			return lib, nil
		}).Build()

		inst, err := newInstance(context.Background(), nil, nil)
		require.Error(t, err)
		assert.Nil(t, inst)
		assert.Contains(t, err.Error(), "failed to get device name")
	})
}

func TestNewInstance_FailureInjectorAndOverrideBranches_WithMockey(t *testing.T) {
	mockey.PatchConvey("newInstance covers product override and failure injector option branches", t, func() {
		dev := testutil.NewMockDeviceWithIDs(
			&mock.Device{
				GetNameFunc: func() (string, nvml.Return) {
					return "NVIDIA H100 80GB HBM3", nvml.SUCCESS
				},
				GetUUIDFunc: func() (string, nvml.Return) {
					return "GPU-INJECT-1", nvml.SUCCESS
				},
				GetCudaComputeCapabilityFunc: func() (int, int, nvml.Return) {
					return 9, 0, nvml.SUCCESS
				},
				GetBrandFunc: func() (nvml.BrandType, nvml.Return) {
					return nvml.BRAND_TESLA, nvml.SUCCESS
				},
			},
			"hopper",
			"Tesla",
			"9.0",
			"0000:01:00.0",
			"GPU-INJECT-1",
			"SERIAL-1",
			1,
			11,
		)

		lib := newMockLibrary("550.120.05", 12040, []nvlibdevice.Device{dev}, nil)
		mockey.Mock(nvmllib.New).To(func(opts ...nvmllib.OpOption) (nvmllib.Library, error) {
			return lib, nil
		}).Build()

		inst, err := newInstance(context.Background(), nil, &FailureInjectorConfig{
			GPUUUIDsWithGPULost:                           []string{"GPU-INJECT-1"},
			GPUUUIDsWithGPURequiresReset:                  []string{"GPU-INJECT-1"},
			GPUUUIDsWithFabricStateHealthSummaryUnhealthy: []string{"GPU-INJECT-1"},
			GPUProductNameOverride:                        "H100-SXM",
		})
		require.NoError(t, err)
		require.NotNil(t, inst)
		assert.Equal(t, "H100-SXM", inst.ProductName())
		assert.Equal(t, 1, len(inst.Devices()))
		assert.Equal(t, "hopper", inst.Architecture())
		assert.Equal(t, "Tesla", inst.Brand())
	})
}

func TestNewInstance_GetUUIDError_WithMockey(t *testing.T) {
	mockey.PatchConvey("newInstance returns error when device UUID query fails", t, func() {
		dev := testutil.NewMockDevice(
			&mock.Device{
				GetNameFunc: func() (string, nvml.Return) {
					return "NVIDIA H100", nvml.SUCCESS
				},
				GetUUIDFunc: func() (string, nvml.Return) {
					return "", nvml.ERROR_GPU_IS_LOST
				},
			},
			"hopper", "Tesla", "9.0", "0000:01:00.0",
		)
		lib := newMockLibrary("550.120.05", 12040, []nvlibdevice.Device{dev}, nil)

		mockey.Mock(nvmllib.New).To(func(opts ...nvmllib.OpOption) (nvmllib.Library, error) {
			return lib, nil
		}).Build()

		inst, err := newInstance(context.Background(), nil, nil)
		require.Error(t, err)
		assert.Nil(t, inst)
		assert.Contains(t, err.Error(), "failed to get device uuid")
	})
}

func TestNewInstance_GetPCIBusIDError_WithMockey(t *testing.T) {
	mockey.PatchConvey("newInstance returns error when device PCI bus ID query fails", t, func() {
		base := testutil.NewMockDevice(
			&mock.Device{
				GetNameFunc: func() (string, nvml.Return) {
					return "NVIDIA H100", nvml.SUCCESS
				},
				GetUUIDFunc: func() (string, nvml.Return) {
					return "GPU-BUSID-ERR", nvml.SUCCESS
				},
			},
			"hopper", "Tesla", "9.0", "0000:01:00.0",
		)

		dev := &busIDErrorDevice{MockDevice: base}
		lib := newMockLibrary("550.120.05", 12040, []nvlibdevice.Device{dev}, nil)

		mockey.Mock(nvmllib.New).To(func(opts ...nvmllib.OpOption) (nvmllib.Library, error) {
			return lib, nil
		}).Build()

		inst, err := newInstance(context.Background(), nil, nil)
		require.Error(t, err)
		assert.Nil(t, inst)
		assert.Contains(t, err.Error(), "failed to get pci bus id")
	})
}

func TestNewInstance_GetArchFamilyError_WithMockey(t *testing.T) {
	mockey.PatchConvey("newInstance returns error when GetArchFamily fails", t, func() {
		dev := testutil.NewMockDevice(
			&mock.Device{
				GetNameFunc: func() (string, nvml.Return) {
					return "NVIDIA H100", nvml.SUCCESS
				},
				GetUUIDFunc: func() (string, nvml.Return) {
					return "GPU-ARCH-ERR", nvml.SUCCESS
				},
			},
			"hopper", "Tesla", "9.0", "0000:01:00.0",
		)
		lib := newMockLibrary("550.120.05", 12040, []nvlibdevice.Device{dev}, nil)

		mockey.Mock(nvmllib.New).To(func(opts ...nvmllib.OpOption) (nvmllib.Library, error) {
			return lib, nil
		}).Build()
		mockey.Mock(GetArchFamily).To(func(_ device.Device) (string, error) {
			return "", errors.New("arch failed")
		}).Build()

		inst, err := newInstance(context.Background(), nil, nil)
		require.Error(t, err)
		assert.Nil(t, inst)
		assert.Contains(t, err.Error(), "arch failed")
	})
}

func TestNewInstance_GetBrandError_WithMockey(t *testing.T) {
	mockey.PatchConvey("newInstance returns error when GetBrand fails", t, func() {
		dev := testutil.NewMockDevice(
			&mock.Device{
				GetNameFunc: func() (string, nvml.Return) {
					return "NVIDIA H100", nvml.SUCCESS
				},
				GetUUIDFunc: func() (string, nvml.Return) {
					return "GPU-BRAND-ERR", nvml.SUCCESS
				},
				GetCudaComputeCapabilityFunc: func() (int, int, nvml.Return) {
					return 9, 0, nvml.SUCCESS
				},
			},
			"hopper", "Tesla", "9.0", "0000:01:00.0",
		)
		lib := newMockLibrary("550.120.05", 12040, []nvlibdevice.Device{dev}, nil)

		mockey.Mock(nvmllib.New).To(func(opts ...nvmllib.OpOption) (nvmllib.Library, error) {
			return lib, nil
		}).Build()
		mockey.Mock(GetBrand).To(func(_ device.Device) (string, error) {
			return "", errors.New("brand failed")
		}).Build()

		inst, err := newInstance(context.Background(), nil, nil)
		require.Error(t, err)
		assert.Nil(t, inst)
		assert.Contains(t, err.Error(), "brand failed")
	})
}

func TestNewInstance_FailureInjectorDeviceGetDevicesOption_WithMockey(t *testing.T) {
	mockey.PatchConvey("newInstance passes library option when NVMLDeviceGetDevicesError is enabled", t, func() {
		called := false
		mockey.Mock(nvmllib.New).To(func(opts ...nvmllib.OpOption) (nvmllib.Library, error) {
			called = true
			return nil, nvmllib.ErrNVMLNotFound
		}).Build()

		inst, err := newInstance(context.Background(), nil, &FailureInjectorConfig{
			NVMLDeviceGetDevicesError: true,
		})
		require.NoError(t, err)
		require.NotNil(t, inst)
		assert.False(t, inst.NVMLExists())
		assert.True(t, called)
	})
}

func TestNewInstance_RefreshCallbackSpawned_OnNVMLNotFound_WithMockey(t *testing.T) {
	mockey.PatchConvey("newInstance invokes refresh callback when NVML is not found", t, func() {
		mockey.Mock(nvmllib.New).To(func(opts ...nvmllib.OpOption) (nvmllib.Library, error) {
			return nil, nvmllib.ErrNVMLNotFound
		}).Build()

		refreshCalled := make(chan struct{}, 1)
		refreshFn := func(_ context.Context) {
			refreshCalled <- struct{}{}
		}

		inst, err := newInstance(context.Background(), refreshFn, nil)
		require.NoError(t, err)
		require.NotNil(t, inst)

		select {
		case <-refreshCalled:
		case <-time.After(2 * time.Second):
			t.Fatal("expected refresh callback to be invoked")
		}
	})
}

func TestRefreshNVMLAndExit_CoversFailedLoadLogPath_WithMockey(t *testing.T) {
	mockey.PatchConvey("refreshNVMLAndExit covers failed load debug branch", t, func() {
		tickCh := make(chan time.Time, 1)
		tickCh <- time.Now()

		mockey.Mock(time.NewTicker).To(func(_ time.Duration) *time.Ticker {
			return &time.Ticker{C: tickCh}
		}).Build()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		callCount := 0
		mockey.Mock(nvmllib.New).To(func(opts ...nvmllib.OpOption) (nvmllib.Library, error) {
			callCount++
			cancel()
			return nil, io.EOF
		}).Build()

		refreshNVMLAndExit(ctx)
		assert.Equal(t, 1, callCount)
	})
}
