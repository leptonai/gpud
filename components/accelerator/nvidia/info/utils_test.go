package info

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"

	nvidiaquery "github.com/leptonai/gpud/pkg/nvidia-query"
)

func TestGetSystemResourceGPUCount(t *testing.T) {
	devCnt, err := nvidiaquery.CountAllDevicesFromDevDir()
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
