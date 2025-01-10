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
	e := nvml.ErrorString(ret)
	return strings.Contains(strings.ToLower(strings.TrimSpace(e)), "not supported")
}
