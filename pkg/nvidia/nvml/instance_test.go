package nvml

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	nvmllib "github.com/leptonai/gpud/pkg/nvidia/nvml/lib"
	nvidiaproduct "github.com/leptonai/gpud/pkg/nvidia/product"
)

func TestInstanceV2(t *testing.T) {
	inst, err := New()
	if errors.Is(err, nvmllib.ErrNVMLNotFound) {
		t.Skipf("nvml not installed, skipping")
	}
	if err != nil {
		t.Fatalf("failed to create instance: %v", err)
	}
	t.Logf("instance mem cap %+v", inst.GetMemoryErrorManagementCapabilities())
}

func TestNewWithFailureInjector_NVMLDeviceGetDevicesError(t *testing.T) {
	// Test that enabling NVMLDeviceGetDevicesError returns an erroredInstance
	inst, err := NewWithFailureInjector(&FailureInjectorConfig{
		NVMLDeviceGetDevicesError: true,
	})

	if errors.Is(err, nvmllib.ErrNVMLNotFound) {
		t.Skipf("nvml not installed, skipping")
	}

	// Should not return an error from NewWithFailureInjector - instead, it returns an erroredInstance
	if err != nil {
		t.Fatalf("failed to create instance: %v", err)
	}

	// If NVML is not installed, the function returns a noOpInstance (NVMLExists=false)
	// which is expected - the error injection only works when NVML is actually loaded
	if !inst.NVMLExists() {
		t.Skipf("nvml not installed (noOpInstance returned), skipping")
	}

	// InitError() should return our injected error
	initErr := inst.InitError()
	if initErr == nil {
		t.Fatal("expected InitError() to return the injected error, got nil")
	}

	// Verify it's our injected error
	if !errors.Is(initErr, ErrDeviceGetDevicesInjected) {
		t.Fatalf("expected InitError() to return ErrDeviceGetDevicesInjected, got: %v", initErr)
	}

	// Devices should be nil for erroredInstance
	if inst.Devices() != nil {
		t.Fatalf("expected Devices() to return nil for erroredInstance, got: %v", inst.Devices())
	}

	t.Logf("successfully tested NVMLDeviceGetDevicesError injection: %v", initErr)
}

func TestErroredInstance(t *testing.T) {
	// Test the erroredInstance directly
	testErr := errors.New("test error")
	inst := NewErrored(testErr)

	// NVMLExists returns true because the library loaded
	if !inst.NVMLExists() {
		t.Error("expected NVMLExists() to return true for erroredInstance")
	}

	// InitError returns the error
	if inst.InitError() != testErr {
		t.Errorf("expected InitError() to return %v, got %v", testErr, inst.InitError())
	}

	// All data methods return nil/zero values
	if inst.Devices() != nil {
		t.Error("expected Devices() to return nil for erroredInstance")
	}
	if inst.Library() != nil {
		t.Error("expected Library() to return nil for erroredInstance")
	}
	if inst.ProductName() != "" {
		t.Error("expected ProductName() to return empty string for erroredInstance")
	}
	if inst.DriverVersion() != "" {
		t.Error("expected DriverVersion() to return empty string for erroredInstance")
	}
	if inst.DriverMajor() != 0 {
		t.Error("expected DriverMajor() to return 0 for erroredInstance")
	}
	if inst.CUDAVersion() != "" {
		t.Error("expected CUDAVersion() to return empty string for erroredInstance")
	}
	if inst.FabricManagerSupported() {
		t.Error("expected FabricManagerSupported() to return false for erroredInstance")
	}
	if inst.FabricStateSupported() {
		t.Error("expected FabricStateSupported() to return false for erroredInstance")
	}

	// Shutdown should not error
	if err := inst.Shutdown(); err != nil {
		t.Errorf("expected Shutdown() to return nil, got %v", err)
	}
}

func TestNewNoOp(t *testing.T) {
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
	assert.Empty(t, inst.GetMemoryErrorManagementCapabilities())
	assert.NoError(t, inst.Shutdown())
	assert.NoError(t, inst.InitError())
}

func TestNewErrored(t *testing.T) {
	expectedErr := errors.New("device enumeration failed")
	inst := NewErrored(expectedErr)

	// NVMLExists returns true because library loaded, but InitError is set
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
	assert.Empty(t, inst.GetMemoryErrorManagementCapabilities())
	assert.NoError(t, inst.Shutdown())
	assert.Error(t, inst.InitError())
	assert.Equal(t, expectedErr, inst.InitError())
}

func TestNewErrored_WithDeviceGetDevicesError(t *testing.T) {
	inst := NewErrored(ErrDeviceGetDevicesInjected)

	assert.True(t, inst.NVMLExists())
	assert.Error(t, inst.InitError())
	assert.Contains(t, inst.InitError().Error(), "Unknown Error")
	assert.Contains(t, inst.InitError().Error(), "injected for testing")
}

func TestInstance_GetMemoryErrorManagementCapabilities(t *testing.T) {
	tests := []struct {
		name                   string
		productName            string
		expectedErrorContain   bool
		expectedDynPageOffline bool
		expectedRowRemap       bool
	}{
		{
			name:                   "H100 with all RAS capabilities",
			productName:            "H100-SXM5-80GB",
			expectedErrorContain:   true,
			expectedDynPageOffline: true,
			expectedRowRemap:       true,
		},
		{
			name:                   "H100 PCIe",
			productName:            "H100-PCIE-80GB",
			expectedErrorContain:   true,
			expectedDynPageOffline: true,
			expectedRowRemap:       true,
		},
		{
			name:                   "A10 with row remapping only",
			productName:            "A10",
			expectedErrorContain:   false,
			expectedDynPageOffline: false,
			expectedRowRemap:       true,
		},
		{
			name:                   "unsupported GPU",
			productName:            "UNKNOWN-GPU",
			expectedErrorContain:   false,
			expectedDynPageOffline: false,
			expectedRowRemap:       false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Create an instance with the test product name
			inst := &instance{
				sanitizedProductName: nvidiaproduct.SanitizeProductName(tc.productName),
				memMgmtCaps:          nvidiaproduct.SupportedMemoryMgmtCapsByGPUProduct(tc.productName),
			}

			caps := inst.GetMemoryErrorManagementCapabilities()
			assert.Equal(t, tc.expectedErrorContain, caps.ErrorContainment)
			assert.Equal(t, tc.expectedDynPageOffline, caps.DynamicPageOfflining)
			assert.Equal(t, tc.expectedRowRemap, caps.RowRemapping)
		})
	}
}

func TestInstance_Architecture(t *testing.T) {
	tests := []struct {
		name         string
		architecture string
	}{
		{
			name:         "Hopper architecture",
			architecture: "hopper",
		},
		{
			name:         "Ampere architecture",
			architecture: "ampere",
		},
		{
			name:         "Blackwell architecture",
			architecture: "blackwell",
		},
		{
			name:         "empty architecture",
			architecture: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			inst := &instance{
				architecture: tc.architecture,
			}

			result := inst.Architecture()
			assert.Equal(t, tc.architecture, result)
		})
	}
}

func TestInstance_Brand(t *testing.T) {
	tests := []struct {
		name  string
		brand string
	}{
		{
			name:  "Tesla brand",
			brand: "Tesla",
		},
		{
			name:  "Quadro brand",
			brand: "Quadro",
		},
		{
			name:  "GeForce brand",
			brand: "GeForce",
		},
		{
			name:  "empty brand",
			brand: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			inst := &instance{
				brand: tc.brand,
			}

			result := inst.Brand()
			assert.Equal(t, tc.brand, result)
		})
	}
}
