package lib

import (
	"testing"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/stretchr/testify/assert"
)

func TestIsLibraryNotFoundError(t *testing.T) {
	assert.Equal(t, IsLibraryNotFoundError(nvml.ERROR_LIBRARY_NOT_FOUND), true)
	assert.Equal(t, IsLibraryNotFoundError(nvml.SUCCESS), false)
}

func Test_doesLibraryExist(t *testing.T) {
	assert.Equal(t, true, doesLibraryExist(New(WithInitReturn(nvml.SUCCESS))))
	assert.Equal(t, false, doesLibraryExist(New(WithInitReturn(nvml.ERROR_LIBRARY_NOT_FOUND))))
}
