package nvml

import (
	"errors"
	"testing"

	nvmllib "github.com/leptonai/gpud/pkg/nvidia-query/nvml/lib"
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
