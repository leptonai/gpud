package lib

import (
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
