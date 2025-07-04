package testutil

import (
	"testing"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/NVIDIA/go-nvml/pkg/nvml/mock"
	"github.com/stretchr/testify/assert"
)

func TestMockDevice(t *testing.T) {
	tests := []struct {
		name         string
		architecture string
		brand        string
		computeCap   string
		pciBusID     string
		uuid         string
	}{
		{
			name:         "default values",
			architecture: "Test Architecture",
			brand:        "Test Brand",
			computeCap:   "8.0",
			pciBusID:     "0000:00:00.0",
			uuid:         "test-uuid-1",
		},
		{
			name:         "custom values",
			architecture: "Ampere",
			brand:        "NVIDIA A100",
			computeCap:   "8.6",
			pciBusID:     "0000:af:00.0",
			uuid:         "test-uuid-2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockDevice := &MockDevice{
				Device: &mock.Device{
					GetUUIDFunc: func() (string, nvml.Return) {
						return tt.uuid, nvml.SUCCESS
					},
				},
				Architecture:          tt.architecture,
				Brand:                 tt.brand,
				CudaComputeCapability: tt.computeCap,
				BusID:                 tt.pciBusID,
			}

			assert.NotNil(t, mockDevice)

			// Test direct field access
			assert.Equal(t, tt.architecture, mockDevice.Architecture)
			assert.Equal(t, tt.brand, mockDevice.Brand)
			assert.Equal(t, tt.computeCap, mockDevice.CudaComputeCapability)
			assert.Equal(t, tt.pciBusID, mockDevice.BusID)

			// Test device methods
			arch, err := mockDevice.GetArchitectureAsString()
			assert.NoError(t, err)
			assert.Equal(t, tt.architecture, arch)

			brand, err := mockDevice.GetBrandAsString()
			assert.NoError(t, err)
			assert.Equal(t, tt.brand, brand)

			computeCap, err := mockDevice.GetCudaComputeCapabilityAsString()
			assert.NoError(t, err)
			assert.Equal(t, tt.computeCap, computeCap)

			pciBusID, err := mockDevice.GetPCIBusID()
			assert.NoError(t, err)
			assert.Equal(t, tt.pciBusID, pciBusID)

			// Test UUID from mock device
			uuid, ret := mockDevice.Device.GetUUID()
			assert.Equal(t, tt.uuid, uuid)
			assert.Equal(t, nvml.SUCCESS, ret)

			// Test MIG-related methods
			migDevices, err := mockDevice.GetMigDevices()
			assert.NoError(t, err)
			assert.Empty(t, migDevices)

			migProfiles, err := mockDevice.GetMigProfiles()
			assert.NoError(t, err)
			assert.Empty(t, migProfiles)

			fabricAttached, err := mockDevice.IsFabricAttached()
			assert.NoError(t, err)
			assert.False(t, fabricAttached)

			migCapable, err := mockDevice.IsMigCapable()
			assert.NoError(t, err)
			assert.False(t, migCapable)

			migEnabled, err := mockDevice.IsMigEnabled()
			assert.NoError(t, err)
			assert.False(t, migEnabled)

			// Test visitor methods
			err = mockDevice.VisitMigDevices(nil)
			assert.NoError(t, err)

			err = mockDevice.VisitMigProfiles(nil)
			assert.NoError(t, err)
		})
	}
}
