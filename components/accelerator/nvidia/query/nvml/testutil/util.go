package testutil

import (
	"github.com/NVIDIA/go-nvlib/pkg/nvlib/device"
	"github.com/NVIDIA/go-nvml/pkg/nvml/mock"
)

var _ device.Device = (*mockDevice)(nil)

type mockDevice struct {
	*mock.Device
}

func (d *mockDevice) GetArchitectureAsString() (string, error) {
	return "", nil
}
func (d *mockDevice) GetBrandAsString() (string, error) {
	return "", nil
}
func (d *mockDevice) GetCudaComputeCapabilityAsString() (string, error) {
	return "", nil
}
func (d *mockDevice) GetMigDevices() ([]device.MigDevice, error) {
	return nil, nil
}
func (d *mockDevice) GetMigProfiles() ([]device.MigProfile, error) {
	return nil, nil
}
func (d *mockDevice) GetPCIBusID() (string, error)                                { return "", nil }
func (d *mockDevice) IsFabricAttached() (bool, error)                             { return false, nil }
func (d *mockDevice) IsMigCapable() (bool, error)                                 { return false, nil }
func (d *mockDevice) IsMigEnabled() (bool, error)                                 { return false, nil }
func (d *mockDevice) VisitMigDevices(func(j int, m device.MigDevice) error) error { return nil }
func (d *mockDevice) VisitMigProfiles(func(p device.MigProfile) error) error      { return nil }

func CreateDevice(m *mock.Device) device.Device {
	return &mockDevice{Device: m}
}
