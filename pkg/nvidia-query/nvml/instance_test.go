package nvml

import (
	"context"
	"errors"
	"testing"

	"github.com/NVIDIA/go-nvml/pkg/nvml"

	nvmllib "github.com/leptonai/gpud/pkg/nvidia-query/nvml/lib"
	nvmlmock "github.com/leptonai/gpud/pkg/nvidia-query/nvml/lib/mock"
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

func TestNewNoOpInstance(t *testing.T) {
	inst := NewNoOp()

	if inst.NVMLExists() {
		t.Fatalf("expected NVMLExists to be false")
	}
	if inst.Library() != nil {
		t.Fatalf("expected nil library")
	}
	if inst.Devices() != nil {
		t.Fatalf("expected nil devices map")
	}
	if inst.ProductName() != "" {
		t.Fatalf("expected empty product name")
	}
	if inst.Architecture() != "" {
		t.Fatalf("expected empty architecture")
	}
	if inst.Brand() != "" {
		t.Fatalf("expected empty brand")
	}
	if inst.DriverVersion() != "" {
		t.Fatalf("expected empty driver version")
	}
	if inst.DriverMajor() != 0 {
		t.Fatalf("expected driver major 0")
	}
	if inst.CUDAVersion() != "" {
		t.Fatalf("expected empty CUDA version")
	}
	if inst.FabricManagerSupported() {
		t.Fatalf("expected FabricManagerSupported false")
	}
	if inst.FabricStateSupported() {
		t.Fatalf("expected FabricStateSupported false")
	}
	if inst.GetMemoryErrorManagementCapabilities() != (nvidiaproduct.MemoryErrorManagementCapabilities{}) {
		t.Fatalf("expected empty memory error management capabilities")
	}
	if err := inst.Shutdown(); err != nil {
		t.Fatalf("expected Shutdown to return nil, got %v", err)
	}
	if inst.InitError() != nil {
		t.Fatalf("expected InitError nil")
	}
}

func TestNewErroredInstance(t *testing.T) {
	initErr := errors.New("nvml init failed")
	inst := NewErrored(initErr)

	if !inst.NVMLExists() {
		t.Fatalf("expected NVMLExists to be true")
	}
	if inst.Library() != nil {
		t.Fatalf("expected nil library")
	}
	if inst.Devices() != nil {
		t.Fatalf("expected nil devices map")
	}
	if inst.ProductName() != "" {
		t.Fatalf("expected empty product name")
	}
	if inst.Architecture() != "" {
		t.Fatalf("expected empty architecture")
	}
	if inst.Brand() != "" {
		t.Fatalf("expected empty brand")
	}
	if inst.DriverVersion() != "" {
		t.Fatalf("expected empty driver version")
	}
	if inst.DriverMajor() != 0 {
		t.Fatalf("expected driver major 0")
	}
	if inst.CUDAVersion() != "" {
		t.Fatalf("expected empty CUDA version")
	}
	if inst.FabricManagerSupported() {
		t.Fatalf("expected FabricManagerSupported false")
	}
	if inst.FabricStateSupported() {
		t.Fatalf("expected FabricStateSupported false")
	}
	if inst.GetMemoryErrorManagementCapabilities() != (nvidiaproduct.MemoryErrorManagementCapabilities{}) {
		t.Fatalf("expected empty memory error management capabilities")
	}
	if err := inst.Shutdown(); err != nil {
		t.Fatalf("expected Shutdown to return nil, got %v", err)
	}
	if !errors.Is(inst.InitError(), initErr) {
		t.Fatalf("expected InitError %v, got %v", initErr, inst.InitError())
	}
}

func TestNewInstanceInitErrorReturnsErroredInstance(t *testing.T) {
	t.Setenv(nvmllib.EnvMockAllSuccess, "true")

	originalDeviceGetCount := nvmlmock.AllSuccessInterface.DeviceGetCountFunc
	t.Cleanup(func() {
		nvmlmock.AllSuccessInterface.DeviceGetCountFunc = originalDeviceGetCount
	})
	nvmlmock.AllSuccessInterface.DeviceGetCountFunc = func() (int, nvml.Return) {
		return 0, nvml.ERROR_UNKNOWN
	}

	inst, err := newInstance(context.Background(), nil, nil)
	if err != nil {
		t.Fatalf("failed to create instance: %v", err)
	}
	if inst.InitError() == nil {
		t.Fatalf("expected init error")
	}
	if !inst.NVMLExists() {
		t.Fatalf("expected NVMLExists to be true")
	}
}
