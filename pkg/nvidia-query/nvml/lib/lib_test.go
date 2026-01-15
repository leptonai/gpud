package lib

import (
	"errors"
	"testing"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/stretchr/testify/assert"
)

func Test_createLibrary(t *testing.T) {
	nv := createLibrary(
		WithInitReturn(nvml.SUCCESS),
	)
	assert.Equal(t, nv.NVML().Init(), nvml.SUCCESS)
}

func Test_WithDeviceGetDevicesError(t *testing.T) {
	testErr := errors.New("error getting device handle for index '0': Unknown Error (injected for testing)")

	nv := createLibrary(
		WithInitReturn(nvml.SUCCESS),
		WithDeviceGetDevicesError(testErr),
	)

	// GetDevices should return the injected error
	devices, err := nv.Device().GetDevices()
	assert.Nil(t, devices)
	assert.Equal(t, testErr, err)
	assert.ErrorIs(t, err, testErr)
}
