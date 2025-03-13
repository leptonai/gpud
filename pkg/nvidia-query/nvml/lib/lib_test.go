package lib

import (
	"testing"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/stretchr/testify/assert"
)

func TestLibrary(t *testing.T) {
	nv := New(
		WithInitReturn(nvml.SUCCESS),
	)
	assert.Equal(t, nv.NVML().Init(), nvml.SUCCESS)
}
