package nvml

import (
	"context"
	"errors"
	"testing"

	nvlibdevice "github.com/NVIDIA/go-nvlib/pkg/nvlib/device"
	nvinfo "github.com/NVIDIA/go-nvlib/pkg/nvlib/info"
	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/NVIDIA/go-nvml/pkg/nvml/mock"
	"github.com/bytedance/mockey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/leptonai/gpud/pkg/nvidia/nvml/device"
	nvmllib "github.com/leptonai/gpud/pkg/nvidia/nvml/lib"
	"github.com/leptonai/gpud/pkg/nvidia/nvml/testutil"
)

// TestGetArchFamily_WithMockedDevice tests GetArchFamily with various compute capabilities.
// This test uses mockey to avoid requiring actual NVIDIA hardware.
func TestGetArchFamily_WithMockedDevice(t *testing.T) {
	tests := []struct {
		name          string
		computeMajor  int
		computeMinor  int
		expectedArch  string
		expectedError bool
	}{
		{
			name:         "Tesla architecture (major 1)",
			computeMajor: 1,
			computeMinor: 0,
			expectedArch: "tesla",
		},
		{
			name:         "Fermi architecture (major 2)",
			computeMajor: 2,
			computeMinor: 0,
			expectedArch: "fermi",
		},
		{
			name:         "Kepler architecture (major 3)",
			computeMajor: 3,
			computeMinor: 5,
			expectedArch: "kepler",
		},
		{
			name:         "Maxwell architecture (major 5)",
			computeMajor: 5,
			computeMinor: 2,
			expectedArch: "maxwell",
		},
		{
			name:         "Pascal architecture (major 6)",
			computeMajor: 6,
			computeMinor: 1,
			expectedArch: "pascal",
		},
		{
			name:         "Volta architecture (major 7, minor < 5)",
			computeMajor: 7,
			computeMinor: 0,
			expectedArch: "volta",
		},
		{
			name:         "Turing architecture (major 7, minor >= 5)",
			computeMajor: 7,
			computeMinor: 5,
			expectedArch: "turing",
		},
		{
			name:         "Ampere architecture (major 8, minor < 9)",
			computeMajor: 8,
			computeMinor: 0,
			expectedArch: "ampere",
		},
		{
			name:         "Ada Lovelace architecture (major 8, minor >= 9)",
			computeMajor: 8,
			computeMinor: 9,
			expectedArch: "ada-lovelace",
		},
		{
			name:         "Hopper architecture (major 9)",
			computeMajor: 9,
			computeMinor: 0,
			expectedArch: "hopper",
		},
		{
			name:         "Blackwell architecture (major 10)",
			computeMajor: 10,
			computeMinor: 0,
			expectedArch: "blackwell",
		},
		{
			name:         "Blackwell architecture (major 12)",
			computeMajor: 12,
			computeMinor: 0,
			expectedArch: "blackwell",
		},
		{
			name:         "Undefined architecture (unknown major)",
			computeMajor: 99,
			computeMinor: 0,
			expectedArch: "undefined",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mockDevice := &mock.Device{
				GetCudaComputeCapabilityFunc: func() (int, int, nvml.Return) {
					return tc.computeMajor, tc.computeMinor, nvml.SUCCESS
				},
			}

			dev := testutil.NewMockDevice(mockDevice, tc.expectedArch, "Tesla", "8.0", "0000:00:1e.0")

			arch, err := GetArchFamily(dev)
			require.NoError(t, err)
			assert.Equal(t, tc.expectedArch, arch)
		})
	}
}

// TestGetArchFamily_DeviceError tests GetArchFamily when device returns an error.
func TestGetArchFamily_DeviceError(t *testing.T) {
	mockDevice := &mock.Device{
		GetCudaComputeCapabilityFunc: func() (int, int, nvml.Return) {
			return 0, 0, nvml.ERROR_GPU_IS_LOST
		},
	}

	dev := testutil.NewMockDevice(mockDevice, "", "Tesla", "8.0", "0000:00:1e.0")

	_, err := GetArchFamily(dev)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get device compute capability")
}

// TestGetBrand_WithMockedDevice tests GetBrand with various brand types.
func TestGetBrand_WithMockedDevice(t *testing.T) {
	tests := []struct {
		name          string
		brandType     nvml.BrandType
		expectedBrand string
	}{
		{
			name:          "Unknown brand",
			brandType:     nvml.BRAND_UNKNOWN,
			expectedBrand: "Unknown",
		},
		{
			name:          "Quadro brand",
			brandType:     nvml.BRAND_QUADRO,
			expectedBrand: "Quadro",
		},
		{
			name:          "Tesla brand",
			brandType:     nvml.BRAND_TESLA,
			expectedBrand: "Tesla",
		},
		{
			name:          "NVS brand",
			brandType:     nvml.BRAND_NVS,
			expectedBrand: "NVS",
		},
		{
			name:          "GRID brand",
			brandType:     nvml.BRAND_GRID,
			expectedBrand: "GRID",
		},
		{
			name:          "GeForce brand",
			brandType:     nvml.BRAND_GEFORCE,
			expectedBrand: "GeForce",
		},
		{
			name:          "TITAN brand",
			brandType:     nvml.BRAND_TITAN,
			expectedBrand: "TITAN",
		},
		{
			name:          "NVIDIA vApps brand",
			brandType:     nvml.BRAND_NVIDIA_VAPPS,
			expectedBrand: "NVIDIA vApps",
		},
		{
			name:          "NVIDIA Virtual PC brand",
			brandType:     nvml.BRAND_NVIDIA_VPC,
			expectedBrand: "NVIDIA Virtual PC",
		},
		{
			name:          "NVIDIA Virtual Compute Server brand",
			brandType:     nvml.BRAND_NVIDIA_VCS,
			expectedBrand: "NVIDIA Virtual Compute Server",
		},
		{
			name:          "NVIDIA Virtual Workstation brand",
			brandType:     nvml.BRAND_NVIDIA_VWS,
			expectedBrand: "NVIDIA Virtual Workstation",
		},
		{
			name:          "NVIDIA Cloud Gaming brand",
			brandType:     nvml.BRAND_NVIDIA_CLOUD_GAMING,
			expectedBrand: "NVIDIA Cloud Gaming",
		},
		{
			name:          "Quadro RTX brand",
			brandType:     nvml.BRAND_QUADRO_RTX,
			expectedBrand: "Quadro RTX",
		},
		{
			name:          "NVIDIA RTX brand",
			brandType:     nvml.BRAND_NVIDIA_RTX,
			expectedBrand: "NVIDIA RTX",
		},
		{
			name:          "NVIDIA brand",
			brandType:     nvml.BRAND_NVIDIA,
			expectedBrand: "NVIDIA",
		},
		{
			name:          "GeForce RTX brand",
			brandType:     nvml.BRAND_GEFORCE_RTX,
			expectedBrand: "GeForce RTX",
		},
		{
			name:          "TITAN RTX brand",
			brandType:     nvml.BRAND_TITAN_RTX,
			expectedBrand: "TITAN RTX",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mockDevice := &mock.Device{
				GetBrandFunc: func() (nvml.BrandType, nvml.Return) {
					return tc.brandType, nvml.SUCCESS
				},
			}

			dev := testutil.NewMockDevice(mockDevice, "ampere", tc.expectedBrand, "8.0", "0000:00:1e.0")

			brand, err := GetBrand(dev)
			require.NoError(t, err)
			assert.Equal(t, tc.expectedBrand, brand)
		})
	}
}

// TestGetBrand_UnknownBrandType tests GetBrand with an unknown brand type.
func TestGetBrand_UnknownBrandType(t *testing.T) {
	mockDevice := &mock.Device{
		GetBrandFunc: func() (nvml.BrandType, nvml.Return) {
			return nvml.BrandType(9999), nvml.SUCCESS // Unknown brand type
		},
	}

	dev := testutil.NewMockDevice(mockDevice, "ampere", "Unknown", "8.0", "0000:00:1e.0")

	brand, err := GetBrand(dev)
	require.NoError(t, err)
	assert.Contains(t, brand, "UnknownBrand(9999)")
}

// TestGetBrand_DeviceError tests GetBrand when device returns an error.
func TestGetBrand_DeviceError(t *testing.T) {
	mockDevice := &mock.Device{
		GetBrandFunc: func() (nvml.BrandType, nvml.Return) {
			return nvml.BRAND_UNKNOWN, nvml.ERROR_GPU_IS_LOST
		},
	}

	dev := testutil.NewMockDevice(mockDevice, "ampere", "Unknown", "8.0", "0000:00:1e.0")

	_, err := GetBrand(dev)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get device brand")
}

// TestGetSystemDriverVersion_WithMockedInterface tests GetSystemDriverVersion with mocked nvml.Interface.
func TestGetSystemDriverVersion_WithMockedInterface(t *testing.T) {
	tests := []struct {
		name            string
		driverVersion   string
		returnCode      nvml.Return
		expectError     bool
		expectedVersion string
	}{
		{
			name:            "successful driver version retrieval",
			driverVersion:   "550.120.05",
			returnCode:      nvml.SUCCESS,
			expectError:     false,
			expectedVersion: "550.120.05",
		},
		{
			name:            "driver version with two parts",
			driverVersion:   "535.161",
			returnCode:      nvml.SUCCESS,
			expectError:     false,
			expectedVersion: "535.161",
		},
		{
			name:        "NVML error",
			returnCode:  nvml.ERROR_UNINITIALIZED,
			expectError: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mockInterface := &mock.Interface{
				SystemGetDriverVersionFunc: func() (string, nvml.Return) {
					return tc.driverVersion, tc.returnCode
				},
			}

			version, err := GetSystemDriverVersion(mockInterface)
			if tc.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.expectedVersion, version)
			}
		})
	}
}

// TestGetCUDAVersion_WithMockedInterface tests getCUDAVersion with mocked nvml.Interface.
func TestGetCUDAVersion_WithMockedInterface(t *testing.T) {
	tests := []struct {
		name            string
		cudaVersion     int
		returnCode      nvml.Return
		expectError     bool
		expectedVersion string
	}{
		{
			name:            "CUDA 12.0",
			cudaVersion:     12000,
			returnCode:      nvml.SUCCESS,
			expectError:     false,
			expectedVersion: "12.0",
		},
		{
			name:            "CUDA 11.8",
			cudaVersion:     11080,
			returnCode:      nvml.SUCCESS,
			expectError:     false,
			expectedVersion: "11.8",
		},
		{
			name:            "CUDA 12.4",
			cudaVersion:     12040,
			returnCode:      nvml.SUCCESS,
			expectError:     false,
			expectedVersion: "12.4",
		},
		{
			name:        "NVML error",
			returnCode:  nvml.ERROR_UNINITIALIZED,
			expectError: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mockInterface := &mock.Interface{
				SystemGetCudaDriverVersion_v2Func: func() (int, nvml.Return) {
					return tc.cudaVersion, tc.returnCode
				},
			}

			version, err := getCUDAVersion(mockInterface)
			if tc.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.expectedVersion, version)
			}
		})
	}
}

// TestGetProductName_WithMockedDevice tests GetProductName with mocked device.
func TestGetProductName_WithMockedDevice(t *testing.T) {
	tests := []struct {
		name         string
		productName  string
		returnCode   nvml.Return
		expectError  bool
		expectedName string
	}{
		{
			name:         "H100 SXM",
			productName:  "NVIDIA H100 80GB HBM3",
			returnCode:   nvml.SUCCESS,
			expectedName: "NVIDIA H100 80GB HBM3",
		},
		{
			name:         "A100",
			productName:  "NVIDIA A100-SXM4-40GB",
			returnCode:   nvml.SUCCESS,
			expectedName: "NVIDIA A100-SXM4-40GB",
		},
		{
			name:        "device error",
			returnCode:  nvml.ERROR_GPU_IS_LOST,
			expectError: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mockDevice := &mock.Device{
				GetNameFunc: func() (string, nvml.Return) {
					return tc.productName, tc.returnCode
				},
			}

			dev := testutil.NewMockDevice(mockDevice, "hopper", "Tesla", "9.0", "0000:00:1e.0")

			name, err := GetProductName(dev)
			if tc.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "failed to get device name")
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.expectedName, name)
			}
		})
	}
}

// TestClockEventsSupportedVersion tests ClockEventsSupportedVersion with various driver versions.
func TestClockEventsSupportedVersion(t *testing.T) {
	tests := []struct {
		name      string
		major     int
		supported bool
	}{
		{
			name:      "driver 525 - not supported",
			major:     525,
			supported: false,
		},
		{
			name:      "driver 530 - not supported",
			major:     530,
			supported: false,
		},
		{
			name:      "driver 534 - not supported",
			major:     534,
			supported: false,
		},
		{
			name:      "driver 535 - supported (boundary)",
			major:     535,
			supported: true,
		},
		{
			name:      "driver 550 - supported",
			major:     550,
			supported: true,
		},
		{
			name:      "driver 560 - supported",
			major:     560,
			supported: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := ClockEventsSupportedVersion(tc.major)
			assert.Equal(t, tc.supported, result)
		})
	}
}

// TestGetArchFamily_InternalFunction tests the internal getArchFamily function.
func TestGetArchFamily_InternalFunction(t *testing.T) {
	tests := []struct {
		name         string
		computeMajor int
		computeMinor int
		expected     string
	}{
		{"tesla 1.0", 1, 0, "tesla"},
		{"tesla 1.3", 1, 3, "tesla"},
		{"fermi 2.0", 2, 0, "fermi"},
		{"fermi 2.1", 2, 1, "fermi"},
		{"kepler 3.0", 3, 0, "kepler"},
		{"kepler 3.5", 3, 5, "kepler"},
		{"kepler 3.7", 3, 7, "kepler"},
		{"maxwell 5.0", 5, 0, "maxwell"},
		{"maxwell 5.2", 5, 2, "maxwell"},
		{"maxwell 5.3", 5, 3, "maxwell"},
		{"pascal 6.0", 6, 0, "pascal"},
		{"pascal 6.1", 6, 1, "pascal"},
		{"pascal 6.2", 6, 2, "pascal"},
		{"volta 7.0", 7, 0, "volta"},
		{"volta 7.2", 7, 2, "volta"},
		{"volta 7.4", 7, 4, "volta"},
		{"turing 7.5", 7, 5, "turing"},
		{"turing 7.6", 7, 6, "turing"},
		{"ampere 8.0", 8, 0, "ampere"},
		{"ampere 8.6", 8, 6, "ampere"},
		{"ampere 8.7", 8, 7, "ampere"},
		{"ampere 8.8", 8, 8, "ampere"},
		{"ada-lovelace 8.9", 8, 9, "ada-lovelace"},
		{"hopper 9.0", 9, 0, "hopper"},
		{"blackwell 10.0", 10, 0, "blackwell"},
		{"blackwell 12.0", 12, 0, "blackwell"},
		{"undefined 4.0", 4, 0, "undefined"},
		{"undefined 11.0", 11, 0, "undefined"},
		{"undefined 99.0", 99, 0, "undefined"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := getArchFamily(tc.computeMajor, tc.computeMinor)
			assert.Equal(t, tc.expected, result)
		})
	}
}

// TestInstanceShutdown_Error tests Instance.Shutdown when nvml.Shutdown returns an error.
func TestInstanceShutdown_Error(t *testing.T) {
	mockey.PatchConvey("shutdown returns error", t, func() {
		// Create a mock library that returns an error on shutdown
		mockLib := &mockLibWrapper{
			shutdownRet: nvml.ERROR_UNINITIALIZED,
		}

		// Create a test instance with the mock library
		inst := &instance{
			nvmlLib: mockLib,
		}

		err := inst.Shutdown()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to shutdown nvml library")
	})
}

// TestInstanceShutdown_Success tests Instance.Shutdown when nvml.Shutdown succeeds.
func TestInstanceShutdown_Success(t *testing.T) {
	mockey.PatchConvey("shutdown succeeds", t, func() {
		mockLib := &mockLibWrapper{
			shutdownRet: nvml.SUCCESS,
		}

		inst := &instance{
			nvmlLib: mockLib,
		}

		err := inst.Shutdown()
		assert.NoError(t, err)
	})
}

// mockLibWrapper implements lib.Library for testing
type mockLibWrapper struct {
	nvmlInterface nvml.Interface
	shutdownRet   nvml.Return
	devInterface  nvlibdevice.Interface
	infoInterface nvinfo.Interface
}

func (m *mockLibWrapper) NVML() nvml.Interface {
	return m.nvmlInterface
}

func (m *mockLibWrapper) Device() nvlibdevice.Interface {
	return m.devInterface
}

func (m *mockLibWrapper) Info() nvinfo.Interface {
	return m.infoInterface
}

func (m *mockLibWrapper) Shutdown() nvml.Return {
	return m.shutdownRet
}

// --- Instance getter method tests ---

// TestInstance_ProductName tests the ProductName getter on a real instance struct.
func TestInstance_ProductName(t *testing.T) {
	tests := []struct {
		name     string
		product  string
		expected string
	}{
		{"H100 product name", "H100-SXM", "H100-SXM"},
		{"A100 product name", "A100-SXM4-40GB", "A100-SXM4-40GB"},
		{"empty product name", "", ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			inst := &instance{
				sanitizedProductName: tc.product,
			}
			assert.Equal(t, tc.expected, inst.ProductName())
		})
	}
}

// TestInstance_DriverVersion tests the DriverVersion getter.
func TestInstance_DriverVersion(t *testing.T) {
	tests := []struct {
		name    string
		version string
	}{
		{"standard version", "550.120.05"},
		{"two-part version", "535.161"},
		{"empty version", ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			inst := &instance{
				driverVersion: tc.version,
			}
			assert.Equal(t, tc.version, inst.DriverVersion())
		})
	}
}

// TestInstance_DriverMajor tests the DriverMajor getter.
func TestInstance_DriverMajor(t *testing.T) {
	tests := []struct {
		name  string
		major int
	}{
		{"driver 550", 550},
		{"driver 535", 535},
		{"driver 0", 0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			inst := &instance{
				driverMajor: tc.major,
			}
			assert.Equal(t, tc.major, inst.DriverMajor())
		})
	}
}

// TestInstance_CUDAVersion tests the CUDAVersion getter.
func TestInstance_CUDAVersion(t *testing.T) {
	tests := []struct {
		name    string
		version string
	}{
		{"CUDA 12.0", "12.0"},
		{"CUDA 11.8", "11.8"},
		{"empty", ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			inst := &instance{
				cudaVersion: tc.version,
			}
			assert.Equal(t, tc.version, inst.CUDAVersion())
		})
	}
}

// TestInstance_NVMLExists tests the NVMLExists getter.
func TestInstance_NVMLExists(t *testing.T) {
	tests := []struct {
		name   string
		exists bool
	}{
		{"nvml exists", true},
		{"nvml not exists", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			inst := &instance{
				nvmlExists: tc.exists,
			}
			assert.Equal(t, tc.exists, inst.NVMLExists())
		})
	}
}

// TestInstance_FabricManagerSupported tests the FabricManagerSupported getter.
func TestInstance_FabricManagerSupported(t *testing.T) {
	tests := []struct {
		name      string
		supported bool
	}{
		{"fabric manager supported", true},
		{"fabric manager not supported", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			inst := &instance{
				fabricMgrSupported: tc.supported,
			}
			assert.Equal(t, tc.supported, inst.FabricManagerSupported())
		})
	}
}

// TestInstance_FabricStateSupported tests the FabricStateSupported getter.
func TestInstance_FabricStateSupported(t *testing.T) {
	tests := []struct {
		name      string
		supported bool
	}{
		{"fabric state supported", true},
		{"fabric state not supported", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			inst := &instance{
				fabricStateSupported: tc.supported,
			}
			assert.Equal(t, tc.supported, inst.FabricStateSupported())
		})
	}
}

// TestInstance_Library tests the Library getter.
func TestInstance_Library(t *testing.T) {
	mockey.PatchConvey("Library getter returns correct library", t, func() {
		mockLib := &mockLibWrapper{
			shutdownRet: nvml.SUCCESS,
		}

		inst := &instance{
			nvmlLib: mockLib,
		}

		assert.Equal(t, mockLib, inst.Library())
	})
}

// TestInstance_Devices tests the Devices getter.
func TestInstance_Devices(t *testing.T) {
	mockey.PatchConvey("Devices getter returns correct device map", t, func() {
		devs := map[string]device.Device{
			"GPU-1234": testutil.NewMockDevice(
				&mock.Device{
					GetCudaComputeCapabilityFunc: func() (int, int, nvml.Return) {
						return 9, 0, nvml.SUCCESS
					},
				},
				"hopper", "Tesla", "9.0", "0000:00:1e.0",
			),
		}

		inst := &instance{
			devices: devs,
		}

		result := inst.Devices()
		assert.Len(t, result, 1)
		assert.Contains(t, result, "GPU-1234")
	})
}

// TestInstance_DevicesNil tests the Devices getter with nil devices.
func TestInstance_DevicesNil(t *testing.T) {
	mockey.PatchConvey("Devices getter returns nil when no devices", t, func() {
		inst := &instance{
			devices: nil,
		}

		result := inst.Devices()
		assert.Nil(t, result)
	})
}

// TestInstance_InitError tests that a regular instance returns nil InitError.
func TestInstance_InitError(t *testing.T) {
	mockey.PatchConvey("regular instance has nil InitError", t, func() {
		inst := &instance{}

		assert.NoError(t, inst.InitError())
	})
}

// --- NoOp instance additional tests ---

// TestNoOpInstance_AllMethods tests all methods of the noOpInstance.
func TestNoOpInstance_AllMethods(t *testing.T) {
	mockey.PatchConvey("noOp instance returns zero values for all methods", t, func() {
		inst := NewNoOp()

		assert.False(t, inst.NVMLExists())
		assert.Nil(t, inst.Library())
		assert.Nil(t, inst.Devices())
		assert.Empty(t, inst.ProductName())
		assert.Empty(t, inst.Architecture())
		assert.Empty(t, inst.Brand())
		assert.Empty(t, inst.DriverVersion())
		assert.Equal(t, 0, inst.DriverMajor())
		assert.Empty(t, inst.CUDAVersion())
		assert.False(t, inst.FabricManagerSupported())
		assert.False(t, inst.FabricStateSupported())
		assert.NoError(t, inst.Shutdown())
		assert.NoError(t, inst.InitError())
	})
}

// --- Errored instance additional tests ---

// TestErroredInstance_AllMethods tests all methods of the erroredInstance.
func TestErroredInstance_AllMethods(t *testing.T) {
	mockey.PatchConvey("errored instance returns correct values", t, func() {
		testErr := errors.New("GPU device enumeration failed")
		inst := NewErrored(testErr)

		assert.True(t, inst.NVMLExists())
		assert.Nil(t, inst.Library())
		assert.Nil(t, inst.Devices())
		assert.Empty(t, inst.ProductName())
		assert.Empty(t, inst.Architecture())
		assert.Empty(t, inst.Brand())
		assert.Empty(t, inst.DriverVersion())
		assert.Equal(t, 0, inst.DriverMajor())
		assert.Empty(t, inst.CUDAVersion())
		assert.False(t, inst.FabricManagerSupported())
		assert.False(t, inst.FabricStateSupported())
		assert.NoError(t, inst.Shutdown())
		assert.Error(t, inst.InitError())
		assert.Equal(t, testErr, inst.InitError())
	})
}

// TestErroredInstance_GetMemoryErrorManagementCapabilities tests memory capabilities for errored instance.
func TestErroredInstance_GetMemoryErrorManagementCapabilities(t *testing.T) {
	mockey.PatchConvey("errored instance returns empty memory capabilities", t, func() {
		inst := NewErrored(errors.New("test error"))

		caps := inst.GetMemoryErrorManagementCapabilities()
		assert.False(t, caps.ErrorContainment)
		assert.False(t, caps.DynamicPageOfflining)
		assert.False(t, caps.RowRemapping)
	})
}

// --- FailureInjectorConfig tests ---

// TestFailureInjectorConfig_Fields tests all fields of FailureInjectorConfig.
func TestFailureInjectorConfig_Fields(t *testing.T) {
	mockey.PatchConvey("FailureInjectorConfig fields", t, func() {
		config := &FailureInjectorConfig{
			GPUUUIDsWithGPULost:                           []string{"GPU-1", "GPU-2"},
			GPUUUIDsWithGPURequiresReset:                  []string{"GPU-3"},
			GPUUUIDsWithFabricStateHealthSummaryUnhealthy: []string{"GPU-4"},
			GPUProductNameOverride:                        "H100-SXM",
			NVMLDeviceGetDevicesError:                     true,
		}

		assert.Len(t, config.GPUUUIDsWithGPULost, 2)
		assert.Len(t, config.GPUUUIDsWithGPURequiresReset, 1)
		assert.Len(t, config.GPUUUIDsWithFabricStateHealthSummaryUnhealthy, 1)
		assert.Equal(t, "H100-SXM", config.GPUProductNameOverride)
		assert.True(t, config.NVMLDeviceGetDevicesError)
	})
}

// TestErrDeviceGetDevicesInjected tests the injected error constant.
func TestErrDeviceGetDevicesInjected(t *testing.T) {
	assert.NotNil(t, ErrDeviceGetDevicesInjected)
	assert.Contains(t, ErrDeviceGetDevicesInjected.Error(), "Unknown Error")
	assert.Contains(t, ErrDeviceGetDevicesInjected.Error(), "injected for testing")
}

// --- mockInfoWrapper implements nvinfo.Interface for testing ---

type mockInfoWrapper struct {
	hasNvml      bool
	hasNvmlMsg   string
	hasDXCore    bool
	hasDXCoreMsg string
	hasTegra     bool
	hasTegraMsg  string
	platform     nvinfo.Platform
}

func (m *mockInfoWrapper) HasNvml() (bool, string)               { return m.hasNvml, m.hasNvmlMsg }
func (m *mockInfoWrapper) HasDXCore() (bool, string)             { return m.hasDXCore, m.hasDXCoreMsg }
func (m *mockInfoWrapper) HasTegraFiles() (bool, string)         { return m.hasTegra, m.hasTegraMsg }
func (m *mockInfoWrapper) IsTegraSystem() (bool, string)         { return m.hasTegra, m.hasTegraMsg }
func (m *mockInfoWrapper) ResolvePlatform() nvinfo.Platform      { return m.platform }
func (m *mockInfoWrapper) UsesOnlyNVGPUModule() (bool, string)   { return false, "" }
func (m *mockInfoWrapper) HasOnlyIntegratedGPUs() (bool, string) { return false, "" }

// --- mockDevInterface implements nvlibdevice.Interface for testing ---

type mockDevInterface struct {
	nvlibdevice.Interface
	devices []nvlibdevice.Device
	err     error
}

func (m *mockDevInterface) GetDevices() ([]nvlibdevice.Device, error) {
	return m.devices, m.err
}

// --- fullMockLibWrapper is a mock that supports all Library methods for nvmllib.New mocking ---

type fullMockLibWrapper struct {
	nvmlIface   nvml.Interface
	shutdownRet nvml.Return
	devIface    nvlibdevice.Interface
	infoIface   nvinfo.Interface
}

func (m *fullMockLibWrapper) NVML() nvml.Interface          { return m.nvmlIface }
func (m *fullMockLibWrapper) Device() nvlibdevice.Interface { return m.devIface }
func (m *fullMockLibWrapper) Info() nvinfo.Interface        { return m.infoIface }
func (m *fullMockLibWrapper) Shutdown() nvml.Return         { return m.shutdownRet }

// --- Tests for GetDriverVersion via mocked nvmllib.New ---

func TestGetDriverVersion_Success(t *testing.T) {
	mockey.PatchConvey("GetDriverVersion succeeds with mocked lib", t, func() {
		mockIface := &mock.Interface{
			SystemGetDriverVersionFunc: func() (string, nvml.Return) {
				return "550.120.05", nvml.SUCCESS
			},
		}

		mockey.Mock(nvmllib.New).To(func(opts ...nvmllib.OpOption) (nvmllib.Library, error) {
			return &fullMockLibWrapper{
				nvmlIface:   mockIface,
				shutdownRet: nvml.SUCCESS,
			}, nil
		}).Build()

		version, err := GetDriverVersion()
		require.NoError(t, err)
		assert.Equal(t, "550.120.05", version)
	})
}

func TestGetDriverVersion_NewError(t *testing.T) {
	mockey.PatchConvey("GetDriverVersion fails when nvmllib.New returns error", t, func() {
		mockey.Mock(nvmllib.New).To(func(opts ...nvmllib.OpOption) (nvmllib.Library, error) {
			return nil, errors.New("NVML init failed")
		}).Build()

		_, err := GetDriverVersion()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "NVML init failed")
	})
}

// --- Tests for GetCUDAVersion via mocked nvmllib.New ---

func TestGetCUDAVersion_Success(t *testing.T) {
	mockey.PatchConvey("GetCUDAVersion succeeds with mocked lib", t, func() {
		mockIface := &mock.Interface{
			SystemGetCudaDriverVersion_v2Func: func() (int, nvml.Return) {
				return 12040, nvml.SUCCESS
			},
		}

		mockey.Mock(nvmllib.New).To(func(opts ...nvmllib.OpOption) (nvmllib.Library, error) {
			return &fullMockLibWrapper{
				nvmlIface:   mockIface,
				shutdownRet: nvml.SUCCESS,
			}, nil
		}).Build()

		version, err := GetCUDAVersion()
		require.NoError(t, err)
		assert.Equal(t, "12.4", version)
	})
}

func TestGetCUDAVersion_NewError(t *testing.T) {
	mockey.PatchConvey("GetCUDAVersion fails when nvmllib.New returns error", t, func() {
		mockey.Mock(nvmllib.New).To(func(opts ...nvmllib.OpOption) (nvmllib.Library, error) {
			return nil, errors.New("NVML not available")
		}).Build()

		_, err := GetCUDAVersion()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "NVML not available")
	})
}

// --- Tests for LoadGPUDeviceName via mocked nvmllib.New ---

func TestLoadGPUDeviceName_Success(t *testing.T) {
	mockey.PatchConvey("LoadGPUDeviceName succeeds with mocked lib", t, func() {
		mockIface := &mock.Interface{}

		mockDevs := []nvlibdevice.Device{
			testutil.NewMockDevice(
				&mock.Device{
					GetNameFunc: func() (string, nvml.Return) {
						return "NVIDIA H100 80GB HBM3", nvml.SUCCESS
					},
				},
				"hopper", "Tesla", "9.0", "0000:00:1e.0",
			),
		}

		mockey.Mock(nvmllib.New).To(func(opts ...nvmllib.OpOption) (nvmllib.Library, error) {
			return &fullMockLibWrapper{
				nvmlIface:   mockIface,
				shutdownRet: nvml.SUCCESS,
				devIface:    &mockDevInterface{devices: mockDevs},
				infoIface:   &mockInfoWrapper{hasNvml: true, hasNvmlMsg: "found"},
			}, nil
		}).Build()

		name, err := LoadGPUDeviceName()
		require.NoError(t, err)
		assert.Equal(t, "NVIDIA H100 80GB HBM3", name)
	})
}

func TestLoadGPUDeviceName_NewError(t *testing.T) {
	mockey.PatchConvey("LoadGPUDeviceName fails when nvmllib.New returns error", t, func() {
		mockey.Mock(nvmllib.New).To(func(opts ...nvmllib.OpOption) (nvmllib.Library, error) {
			return nil, errors.New("NVML init failed")
		}).Build()

		_, err := LoadGPUDeviceName()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "NVML init failed")
	})
}

func TestLoadGPUDeviceName_NvmlNotFound(t *testing.T) {
	mockey.PatchConvey("LoadGPUDeviceName fails when NVML not found", t, func() {
		mockIface := &mock.Interface{}

		mockey.Mock(nvmllib.New).To(func(opts ...nvmllib.OpOption) (nvmllib.Library, error) {
			return &fullMockLibWrapper{
				nvmlIface:   mockIface,
				shutdownRet: nvml.SUCCESS,
				devIface:    &mockDevInterface{},
				infoIface:   &mockInfoWrapper{hasNvml: false, hasNvmlMsg: "nvml library not found"},
			}, nil
		}).Build()

		_, err := LoadGPUDeviceName()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "NVML not found")
	})
}

func TestLoadGPUDeviceName_GetDevicesError(t *testing.T) {
	mockey.PatchConvey("LoadGPUDeviceName fails when GetDevices returns error", t, func() {
		mockIface := &mock.Interface{}

		mockey.Mock(nvmllib.New).To(func(opts ...nvmllib.OpOption) (nvmllib.Library, error) {
			return &fullMockLibWrapper{
				nvmlIface:   mockIface,
				shutdownRet: nvml.SUCCESS,
				devIface:    &mockDevInterface{err: errors.New("device enumeration failed")},
				infoIface:   &mockInfoWrapper{hasNvml: true, hasNvmlMsg: "found"},
			}, nil
		}).Build()

		_, err := LoadGPUDeviceName()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "device enumeration failed")
	})
}

func TestLoadGPUDeviceName_NoDevices(t *testing.T) {
	mockey.PatchConvey("LoadGPUDeviceName returns empty when no devices", t, func() {
		mockIface := &mock.Interface{}

		mockey.Mock(nvmllib.New).To(func(opts ...nvmllib.OpOption) (nvmllib.Library, error) {
			return &fullMockLibWrapper{
				nvmlIface:   mockIface,
				shutdownRet: nvml.SUCCESS,
				devIface:    &mockDevInterface{devices: []nvlibdevice.Device{}},
				infoIface:   &mockInfoWrapper{hasNvml: true, hasNvmlMsg: "found"},
			}, nil
		}).Build()

		name, err := LoadGPUDeviceName()
		require.NoError(t, err)
		assert.Empty(t, name)
	})
}

func TestLoadGPUDeviceName_GetNameError(t *testing.T) {
	mockey.PatchConvey("LoadGPUDeviceName fails when GetName returns error", t, func() {
		mockIface := &mock.Interface{}

		mockDevs := []nvlibdevice.Device{
			testutil.NewMockDevice(
				&mock.Device{
					GetNameFunc: func() (string, nvml.Return) {
						return "", nvml.ERROR_GPU_IS_LOST
					},
				},
				"hopper", "Tesla", "9.0", "0000:00:1e.0",
			),
		}

		mockey.Mock(nvmllib.New).To(func(opts ...nvmllib.OpOption) (nvmllib.Library, error) {
			return &fullMockLibWrapper{
				nvmlIface:   mockIface,
				shutdownRet: nvml.SUCCESS,
				devIface:    &mockDevInterface{devices: mockDevs},
				infoIface:   &mockInfoWrapper{hasNvml: true, hasNvmlMsg: "found"},
			}, nil
		}).Build()

		_, err := LoadGPUDeviceName()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get device name")
	})
}

// --- Tests for newInstance via mocked nvmllib.New ---

func TestNewInstance_NVMLNotFound(t *testing.T) {
	mockey.PatchConvey("newInstance returns noOp when NVML not found", t, func() {
		mockey.Mock(nvmllib.New).To(func(opts ...nvmllib.OpOption) (nvmllib.Library, error) {
			return nil, nvmllib.ErrNVMLNotFound
		}).Build()

		inst, err := New()
		require.NoError(t, err)
		assert.NotNil(t, inst)
		assert.False(t, inst.NVMLExists())
	})
}

func TestNewInstance_NVMLNotFoundWithRefresh(t *testing.T) {
	mockey.PatchConvey("newInstance with refresh callback when NVML not found", t, func() {
		mockey.Mock(nvmllib.New).To(func(opts ...nvmllib.OpOption) (nvmllib.Library, error) {
			return nil, nvmllib.ErrNVMLNotFound
		}).Build()

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // cancel immediately so the refresh goroutine exits

		inst, err := NewWithExitOnSuccessfulLoad(ctx)
		require.NoError(t, err)
		assert.NotNil(t, inst)
		assert.False(t, inst.NVMLExists())
	})
}

func TestNewInstance_NonNVMLError(t *testing.T) {
	mockey.PatchConvey("newInstance fails with non-NVML error", t, func() {
		mockey.Mock(nvmllib.New).To(func(opts ...nvmllib.OpOption) (nvmllib.Library, error) {
			return nil, errors.New("unexpected initialization error")
		}).Build()

		inst, err := New()
		require.Error(t, err)
		assert.Nil(t, inst)
		assert.Contains(t, err.Error(), "unexpected initialization error")
	})
}

func TestNewInstance_NVMLExistsButNotFound(t *testing.T) {
	mockey.PatchConvey("newInstance fails when HasNvml returns false", t, func() {
		mockIface := &mock.Interface{}

		mockey.Mock(nvmllib.New).To(func(opts ...nvmllib.OpOption) (nvmllib.Library, error) {
			return &fullMockLibWrapper{
				nvmlIface:   mockIface,
				shutdownRet: nvml.SUCCESS,
				devIface:    &mockDevInterface{},
				infoIface:   &mockInfoWrapper{hasNvml: false, hasNvmlMsg: "library not found on system"},
			}, nil
		}).Build()

		inst, err := New()
		require.Error(t, err)
		assert.Nil(t, inst)
		assert.Contains(t, err.Error(), "nvml not found")
	})
}

func TestNewInstance_DriverVersionError(t *testing.T) {
	mockey.PatchConvey("newInstance fails when driver version cannot be retrieved", t, func() {
		mockIface := &mock.Interface{
			SystemGetDriverVersionFunc: func() (string, nvml.Return) {
				return "", nvml.ERROR_UNINITIALIZED
			},
		}

		mockey.Mock(nvmllib.New).To(func(opts ...nvmllib.OpOption) (nvmllib.Library, error) {
			return &fullMockLibWrapper{
				nvmlIface:   mockIface,
				shutdownRet: nvml.SUCCESS,
				devIface:    &mockDevInterface{},
				infoIface:   &mockInfoWrapper{hasNvml: true, hasNvmlMsg: "found"},
			}, nil
		}).Build()

		inst, err := New()
		require.Error(t, err)
		assert.Nil(t, inst)
		assert.Contains(t, err.Error(), "failed to get driver version")
	})
}

func TestNewInstance_CUDAVersionError(t *testing.T) {
	mockey.PatchConvey("newInstance fails when CUDA version cannot be retrieved", t, func() {
		mockIface := &mock.Interface{
			SystemGetDriverVersionFunc: func() (string, nvml.Return) {
				return "550.120.05", nvml.SUCCESS
			},
			SystemGetCudaDriverVersion_v2Func: func() (int, nvml.Return) {
				return 0, nvml.ERROR_UNINITIALIZED
			},
		}

		mockey.Mock(nvmllib.New).To(func(opts ...nvmllib.OpOption) (nvmllib.Library, error) {
			return &fullMockLibWrapper{
				nvmlIface:   mockIface,
				shutdownRet: nvml.SUCCESS,
				devIface:    &mockDevInterface{},
				infoIface:   &mockInfoWrapper{hasNvml: true, hasNvmlMsg: "found"},
			}, nil
		}).Build()

		inst, err := New()
		require.Error(t, err)
		assert.Nil(t, inst)
		assert.Contains(t, err.Error(), "failed to get driver version")
	})
}

func TestNewInstance_DeviceEnumerationFailed(t *testing.T) {
	mockey.PatchConvey("newInstance returns errored instance when device enumeration fails", t, func() {
		mockIface := &mock.Interface{
			SystemGetDriverVersionFunc: func() (string, nvml.Return) {
				return "550.120.05", nvml.SUCCESS
			},
			SystemGetCudaDriverVersion_v2Func: func() (int, nvml.Return) {
				return 12040, nvml.SUCCESS
			},
		}

		mockey.Mock(nvmllib.New).To(func(opts ...nvmllib.OpOption) (nvmllib.Library, error) {
			return &fullMockLibWrapper{
				nvmlIface:   mockIface,
				shutdownRet: nvml.SUCCESS,
				devIface:    &mockDevInterface{err: errors.New("Unknown Error: device handle failed")},
				infoIface:   &mockInfoWrapper{hasNvml: true, hasNvmlMsg: "found"},
			}, nil
		}).Build()

		inst, err := New()
		require.NoError(t, err)
		assert.NotNil(t, inst)
		// Should be an errored instance
		assert.True(t, inst.NVMLExists())
		assert.Error(t, inst.InitError())
		assert.Contains(t, inst.InitError().Error(), "Unknown Error")
	})
}

func TestNewInstance_NoDevices(t *testing.T) {
	mockey.PatchConvey("newInstance succeeds with no devices", t, func() {
		mockIface := &mock.Interface{
			SystemGetDriverVersionFunc: func() (string, nvml.Return) {
				return "550.120.05", nvml.SUCCESS
			},
			SystemGetCudaDriverVersion_v2Func: func() (int, nvml.Return) {
				return 12040, nvml.SUCCESS
			},
		}

		mockey.Mock(nvmllib.New).To(func(opts ...nvmllib.OpOption) (nvmllib.Library, error) {
			return &fullMockLibWrapper{
				nvmlIface:   mockIface,
				shutdownRet: nvml.SUCCESS,
				devIface:    &mockDevInterface{devices: []nvlibdevice.Device{}},
				infoIface:   &mockInfoWrapper{hasNvml: true, hasNvmlMsg: "found"},
			}, nil
		}).Build()

		inst, err := New()
		require.NoError(t, err)
		assert.NotNil(t, inst)
		assert.True(t, inst.NVMLExists())
		assert.NoError(t, inst.InitError())
		assert.Equal(t, "550.120.05", inst.DriverVersion())
		assert.Equal(t, "12.4", inst.CUDAVersion())
		assert.Equal(t, 550, inst.DriverMajor())
		assert.Empty(t, inst.ProductName())
	})
}

// --- refreshNVMLAndExit context cancellation test ---

func TestRefreshNVMLAndExit_ContextCancel(t *testing.T) {
	mockey.PatchConvey("refreshNVMLAndExit exits on context cancellation", t, func() {
		mockey.Mock(nvmllib.New).To(func(opts ...nvmllib.OpOption) (nvmllib.Library, error) {
			return nil, nvmllib.ErrNVMLNotFound
		}).Build()

		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan struct{})

		go func() {
			refreshNVMLAndExit(ctx)
			close(done)
		}()

		// Cancel context to stop the loop
		cancel()

		// Wait for the goroutine to finish
		<-done
	})
}

// --- ParseDriverVersion additional edge case tests ---

// TestParseDriverVersion_AdditionalCases tests additional edge cases for ParseDriverVersion.
func TestParseDriverVersion_AdditionalCases(t *testing.T) {
	tests := []struct {
		name        string
		version     string
		wantMajor   int
		wantMinor   int
		wantPatch   int
		expectError bool
	}{
		{
			name:      "three parts",
			version:   "550.120.05",
			wantMajor: 550,
			wantMinor: 120,
			wantPatch: 5,
		},
		{
			name:      "two parts",
			version:   "535.161",
			wantMajor: 535,
			wantMinor: 161,
			wantPatch: 0,
		},
		{
			name:        "too few parts",
			version:     "550",
			expectError: true,
		},
		{
			name:        "too many parts",
			version:     "550.120.05.01",
			expectError: true,
		},
		{
			name:        "invalid major",
			version:     "abc.120.05",
			expectError: true,
		},
		{
			name:        "invalid minor",
			version:     "550.xyz.05",
			expectError: true,
		},
		{
			name:        "invalid patch",
			version:     "550.120.abc",
			expectError: true,
		},
		{
			name:        "empty string",
			version:     "",
			expectError: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			major, minor, patch, err := ParseDriverVersion(tc.version)
			if tc.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.wantMajor, major)
				assert.Equal(t, tc.wantMinor, minor)
				assert.Equal(t, tc.wantPatch, patch)
			}
		})
	}
}
