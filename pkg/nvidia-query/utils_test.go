package query

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetSystemResourceGPUCount(t *testing.T) {
	devCnt, err := CountAllDevicesFromDevDir()
	assert.NoError(t, err)

	gpuCnt, err := GetSystemResourceGPUCount()
	assert.NoError(t, err)
	assert.NotEmpty(t, gpuCnt)

	if devCnt == 0 {
		assert.Equal(t, gpuCnt, "0")
	} else {
		assert.Equal(t, gpuCnt, strconv.Itoa(devCnt))
	}
}
