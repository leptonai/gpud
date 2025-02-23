package nvml

import (
	"strings"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
)

// Returns true if the error indicates that the operation is not supported.
func IsNotSupportError(ret nvml.Return) bool {
	if ret == nvml.ERROR_NOT_SUPPORTED {
		return true
	}

	// "Argument version mismatch" indicates that the NVML library
	// is not compatible with the corresponding API call
	// thus marking the operation as not supported.
	if ret == nvml.ERROR_ARGUMENT_VERSION_MISMATCH {
		return true
	}

	e := nvml.ErrorString(ret)
	e = strings.ToLower(strings.TrimSpace(e))

	return strings.Contains(e, "not supported") || strings.Contains(e, "version mismatch")
}
