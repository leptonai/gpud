package nvml

import (
	"testing"

	"github.com/NVIDIA/go-nvlib/pkg/nvlib/device"
	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/NVIDIA/go-nvml/pkg/nvml/mock"
	"github.com/stretchr/testify/assert"

	"github.com/leptonai/gpud/pkg/nvidia-query/nvml/testutil"
)

func TestGetSerial(t *testing.T) {
	// Success case
	mockDevice := testutil.NewMockDeviceWithIDs(
		&mock.Device{},
		"test-arch",
		"test-brand",
		"test-cuda",
		"test-pci",
		"TEST-SERIAL-123",
		0,
		0,
	)

	serial, err := GetSerial("test-uuid", mockDevice)
	assert.NoError(t, err)
	assert.Equal(t, "TEST-SERIAL-123", serial)

	// Error case
	errorDevice := &mockErrorDevice{errorCode: nvml.ERROR_UNKNOWN}
	serial, err = GetSerial("test-uuid", errorDevice)
	assert.Error(t, err)
	assert.Equal(t, "", serial)
}

func TestGetMinorID(t *testing.T) {
	// Success case
	mockDevice := testutil.NewMockDeviceWithIDs(
		&mock.Device{},
		"test-arch",
		"test-brand",
		"test-cuda",
		"test-pci",
		"test-serial",
		42,
		0,
	)

	minorID, err := GetMinorID("test-uuid", mockDevice)
	assert.NoError(t, err)
	assert.Equal(t, 42, minorID)

	// Error case
	errorDevice := &mockErrorDevice{errorCode: nvml.ERROR_UNKNOWN}
	minorID, err = GetMinorID("test-uuid", errorDevice)
	assert.Error(t, err)
	assert.Equal(t, 0, minorID)
}

func TestGetBoardID(t *testing.T) {
	// Success case
	mockDevice := testutil.NewMockDeviceWithIDs(
		&mock.Device{},
		"test-arch",
		"test-brand",
		"test-cuda",
		"test-pci",
		"test-serial",
		0,
		12345,
	)

	boardID, err := GetBoardID("test-uuid", mockDevice)
	assert.NoError(t, err)
	assert.Equal(t, uint32(12345), boardID)

	// Error case
	errorDevice := &mockErrorDevice{errorCode: nvml.ERROR_UNKNOWN}
	boardID, err = GetBoardID("test-uuid", errorDevice)
	assert.Error(t, err)
	assert.Equal(t, uint32(0), boardID)
}

// mockErrorDevice implements device.Device and returns errors for the methods we're testing
type mockErrorDevice struct {
	device.Device
	errorCode nvml.Return
}

func (d *mockErrorDevice) GetSerial() (string, nvml.Return) {
	return "", d.errorCode
}

func (d *mockErrorDevice) GetMinorNumber() (int, nvml.Return) {
	return 0, d.errorCode
}

func (d *mockErrorDevice) GetBoardId() (uint32, nvml.Return) {
	return 0, d.errorCode
}
