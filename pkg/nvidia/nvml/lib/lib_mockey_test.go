package lib

import (
	"testing"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/NVIDIA/go-nvml/pkg/nvml/mock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNvmlInterface_NVML tests that NVML() returns the interface itself.
func TestNvmlInterface_NVML(t *testing.T) {
	mockInterface := &mock.Interface{}
	lib := createLibrary(WithNVML(mockInterface))

	nvmlLib := lib.NVML()
	assert.NotNil(t, nvmlLib)
}

// TestNvmlInterface_Device tests that Device() returns the device interface.
func TestNvmlInterface_Device(t *testing.T) {
	mockInterface := &mock.Interface{}
	lib := createLibrary(WithNVML(mockInterface))

	devLib := lib.Device()
	assert.NotNil(t, devLib)
}

// TestNvmlInterface_Info tests that Info() returns the info interface.
func TestNvmlInterface_Info(t *testing.T) {
	mockInterface := &mock.Interface{}
	lib := createLibrary(WithNVML(mockInterface))

	infoLib := lib.Info()
	assert.NotNil(t, infoLib)
}

// TestNvmlInterface_Shutdown tests the Shutdown method.
func TestNvmlInterface_Shutdown(t *testing.T) {
	tests := []struct {
		name           string
		shutdownReturn nvml.Return
		expectReturn   nvml.Return
	}{
		{
			name:           "shutdown success",
			shutdownReturn: nvml.SUCCESS,
			expectReturn:   nvml.SUCCESS,
		},
		{
			name:           "shutdown error",
			shutdownReturn: nvml.ERROR_UNINITIALIZED,
			expectReturn:   nvml.ERROR_UNINITIALIZED,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mockInterface := &mock.Interface{
				ShutdownFunc: func() nvml.Return {
					return tc.shutdownReturn
				},
			}
			lib := createLibrary(WithNVML(mockInterface))

			ret := lib.Shutdown()
			assert.Equal(t, tc.expectReturn, ret)
		})
	}
}

// TestNvmlInterface_Init tests the Init method with custom initReturn.
func TestNvmlInterface_Init(t *testing.T) {
	tests := []struct {
		name         string
		initReturn   nvml.Return
		expectReturn nvml.Return
	}{
		{
			name:         "injected success",
			initReturn:   nvml.SUCCESS,
			expectReturn: nvml.SUCCESS,
		},
		{
			name:         "injected error",
			initReturn:   nvml.ERROR_LIBRARY_NOT_FOUND,
			expectReturn: nvml.ERROR_LIBRARY_NOT_FOUND,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mockInterface := &mock.Interface{
				InitFunc: func() nvml.Return {
					return nvml.ERROR_UNKNOWN // This should not be called if initReturn is set
				},
			}
			lib := createLibrary(WithNVML(mockInterface), WithInitReturn(tc.initReturn))

			// Access the underlying nvmlInterface to call Init
			nvmlIface, ok := lib.(*nvmlInterface)
			require.True(t, ok)

			ret := nvmlIface.Init()
			assert.Equal(t, tc.expectReturn, ret)
		})
	}
}

// TestNvmlInterface_Init_NoInjection tests the Init method without injection.
func TestNvmlInterface_Init_NoInjection(t *testing.T) {
	mockInterface := &mock.Interface{
		InitFunc: func() nvml.Return {
			return nvml.SUCCESS
		},
	}
	lib := createLibrary(WithNVML(mockInterface))

	nvmlIface, ok := lib.(*nvmlInterface)
	require.True(t, ok)

	ret := nvmlIface.Init()
	assert.Equal(t, nvml.SUCCESS, ret)
}

// TestDevInterface_GetDevices_WithError tests the GetDevices method with error injection.
func TestDevInterface_GetDevices_WithError(t *testing.T) {
	mockInterface := &mock.Interface{
		DeviceGetCountFunc: func() (int, nvml.Return) {
			return 0, nvml.SUCCESS
		},
	}

	lib := createLibrary(
		WithNVML(mockInterface),
		WithDeviceGetDevicesError(ErrNVMLNotFound),
	)
	devLib := lib.Device()

	_, err := devLib.GetDevices()
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNVMLNotFound)
}

// TestDevInterface_GetDevices_NoError tests the GetDevices method without error injection.
func TestDevInterface_GetDevices_NoError(t *testing.T) {
	mockInterface := &mock.Interface{
		DeviceGetCountFunc: func() (int, nvml.Return) {
			return 0, nvml.SUCCESS
		},
	}

	lib := createLibrary(WithNVML(mockInterface))
	devLib := lib.Device()

	devices, err := devLib.GetDevices()
	require.NoError(t, err)
	assert.Empty(t, devices)
}

// TestErrNVMLNotFound tests that the error constant exists and has a meaningful message.
func TestErrNVMLNotFound(t *testing.T) {
	assert.NotNil(t, ErrNVMLNotFound)
	assert.Contains(t, ErrNVMLNotFound.Error(), "NVML")
}

// TestCreateLibrary_WithAllOptions tests createLibrary with multiple options.
func TestCreateLibrary_WithAllOptions(t *testing.T) {
	mockInterface := &mock.Interface{
		ShutdownFunc: func() nvml.Return {
			return nvml.SUCCESS
		},
	}

	lib := createLibrary(
		WithNVML(mockInterface),
		WithInitReturn(nvml.SUCCESS),
		WithDeviceGetRemappedRowsForAllDevs(func() (int, int, bool, bool, nvml.Return) {
			return 1, 2, true, false, nvml.SUCCESS
		}),
		WithDeviceGetCurrentClocksEventReasonsForAllDevs(func() (uint64, nvml.Return) {
			return 0x8, nvml.SUCCESS
		}),
		WithDeviceGetDevicesError(ErrNVMLNotFound),
	)

	assert.NotNil(t, lib)
	assert.NotNil(t, lib.Device())
	assert.NotNil(t, lib.Info())
}

// TestOp_ApplyOpts_DefaultNVML tests that applyOpts sets default NVML library.
func TestOp_ApplyOpts_DefaultNVML(t *testing.T) {
	op := &Op{}
	// When we call applyOpts with no options and nvmlLib is nil,
	// it should set nvmlLib to nvml.New()
	// Note: This may fail if NVML is not available, so we just test that it doesn't panic
	op.applyOpts(nil)
	assert.NotNil(t, op.nvmlLib)
}

// TestOp_ApplyOpts_WithNVML tests that applyOpts respects provided NVML library.
func TestOp_ApplyOpts_WithNVML(t *testing.T) {
	mockInterface := &mock.Interface{}
	op := &Op{}
	op.applyOpts([]OpOption{WithNVML(mockInterface)})
	assert.Equal(t, mockInterface, op.nvmlLib)
}
