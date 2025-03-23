package testutil

import (
	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/NVIDIA/go-nvml/pkg/nvml/mock"
)

var _ nvml.Device = (*MockDevice)(nil)

type MockDevice struct {
	*mock.Device
	Architecture          string
	Brand                 string
	CudaComputeCapability string
	PCIBusID              string
}

// NewMockDevice creates a new mock device with the given parameters
func NewMockDevice(device *mock.Device, architecture, brand, cudaComputeCapability, pciBusID string) *MockDevice {
	return &MockDevice{
		Device:                device,
		Architecture:          architecture,
		Brand:                 brand,
		CudaComputeCapability: cudaComputeCapability,
		PCIBusID:              pciBusID,
	}
}

func (d *MockDevice) GetArchitectureAsString() (string, error) {
	return d.Architecture, nil
}

func (d *MockDevice) GetBrandAsString() (string, error) {
	return d.Brand, nil
}

func (d *MockDevice) GetCudaComputeCapabilityAsString() (string, error) {
	return d.CudaComputeCapability, nil
}

func (d *MockDevice) GetPCIBusID() (string, error) {
	return d.PCIBusID, nil
}

func (d *MockDevice) IsFabricAttached() (bool, error) {
	return false, nil
}

func (d *MockDevice) IsMigCapable() (bool, error) {
	return false, nil
}

func (d *MockDevice) IsMigEnabled() (bool, error) {
	return false, nil
}
