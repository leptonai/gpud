package lib

import "github.com/NVIDIA/go-nvml/pkg/nvml"

// IsLibraryNotFoundError returns true if the error is due to the NVML library not being found.
func IsLibraryNotFoundError(ret nvml.Return) bool {
	return ret == nvml.ERROR_LIBRARY_NOT_FOUND
}

// DoesLibraryExist returns true if the NVML library exists.
func DoesLibraryExist() bool {
	lib := NewDefault()
	defer func() {
		_ = lib.Shutdown()
	}()
	return doesLibraryExist(lib)
}

func doesLibraryExist(lib Library) bool {
	ret := lib.NVML().Init()
	return ret == nvml.SUCCESS
}
