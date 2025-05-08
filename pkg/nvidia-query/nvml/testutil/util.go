package testutil

import (
	"github.com/NVIDIA/go-nvlib/pkg/nvlib/device"
	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/NVIDIA/go-nvml/pkg/nvml/mock"
)

var _ device.Device = (*MockDevice)(nil)

type MockDevice struct {
	*mock.Device
	Architecture          string
	Brand                 string
	CudaComputeCapability string
	PCIBusID              string
	Serial                string
	MinorNumber           int
	BoardID               uint32
}

// NewMockDevice creates a new mock device with the given parameters
func NewMockDevice(device *mock.Device, architecture, brand, cudaComputeCapability, pciBusID string) *MockDevice {
	return NewMockDeviceWithIDs(device, architecture, brand, cudaComputeCapability, pciBusID, "MOCK-GPU-SERIAL", 0, 0)
}

// NewMockDeviceWithIDs creates a new mock device with the given parameters including serial and minor number
func NewMockDeviceWithIDs(device *mock.Device, architecture, brand, cudaComputeCapability, pciBusID, serial string, minorNumber int, boardID uint32) *MockDevice {
	return &MockDevice{
		Device:                device,
		Architecture:          architecture,
		Brand:                 brand,
		CudaComputeCapability: cudaComputeCapability,
		PCIBusID:              pciBusID,
		Serial:                serial,
		MinorNumber:           minorNumber,
		BoardID:               boardID,
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

func (d *MockDevice) GetMigDevices() ([]device.MigDevice, error) {
	return nil, nil
}

func (d *MockDevice) GetMigProfiles() ([]device.MigProfile, error) {
	return nil, nil
}

func (d *MockDevice) GetPCIBusID() (string, error) {
	return d.PCIBusID, nil
}

func (d *MockDevice) GetSerial() (string, nvml.Return) {
	return d.Serial, nvml.SUCCESS
}

func (d *MockDevice) GetMinorNumber() (int, nvml.Return) {
	return d.MinorNumber, nvml.SUCCESS
}

func (d *MockDevice) GetBoardId() (uint32, nvml.Return) {
	return d.BoardID, nvml.SUCCESS
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

func (d *MockDevice) VisitMigDevices(func(j int, m device.MigDevice) error) error {
	return nil
}

func (d *MockDevice) VisitMigProfiles(func(p device.MigProfile) error) error {
	return nil
}
