package nvml

import (
	"errors"
	"strings"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
)

var (
	// ErrGPULost is an error that indicates the GPU is lost.
	// Likely due to the GPU is physically removed from the machine.
	// Also manifested as Xid 79 (GPU has fallen off the bus).
	// ref. https://github.com/leptonai/gpud/issues/604
	ErrGPULost = errors.New("gpu lost")
	// ErrGPURequiresReset is an error that indicates the GPU requires reset.
	// This typically appears when NVML reports "GPU requires reset".
	ErrGPURequiresReset = errors.New("gpu requires reset")
)

// IsVersionMismatchError returns true if the error indicates a version mismatch.
func IsVersionMismatchError(ret nvml.Return) bool {
	if ret == nvml.ERROR_ARGUMENT_VERSION_MISMATCH {
		return true
	}

	e := normalizeNVMLReturnString(ret)
	return strings.Contains(e, "version mismatch")
}

// IsNotSupportError returns true if the error indicates that the operation is not supported.
func IsNotSupportError(ret nvml.Return) bool {
	if ret == nvml.ERROR_NOT_SUPPORTED {
		return true
	}

	// e.g., "Not Supported"
	e := normalizeNVMLReturnString(ret)
	return strings.Contains(e, "not supported")
}

// IsNotReadyError returns true if the error indicates that the system is not ready,
// meaning that the GPU is not yet initialized.
// e.g.,
// "nvml.CLOCK_GRAPHICS: System is not in ready state"
func IsNotReadyError(ret nvml.Return) bool {
	if ret == nvml.ERROR_NOT_READY {
		return true
	}

	e := normalizeNVMLReturnString(ret)
	return strings.Contains(e, "not in ready")
}

// IsNotFoundError returns true if the error indicates that the object/instance is not found.
// e.g., process not found from nvml
func IsNotFoundError(ret nvml.Return) bool {
	if ret == nvml.ERROR_NOT_FOUND {
		return true
	}

	e := normalizeNVMLReturnString(ret)
	return strings.Contains(e, "not found") || strings.Contains(e, "not_found")
}

func IsNoSuchFileOrDirectoryError(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "not found") || strings.Contains(s, "no such file or directory")
}

// IsGPULostError returns true if the error indicates that the GPU is lost.
// "if the target GPU has fallen off the bus or is otherwise inaccessible".
func IsGPULostError(ret nvml.Return) bool {
	if ret == nvml.ERROR_GPU_IS_LOST {
		return true
	}

	e := normalizeNVMLReturnString(ret)
	return strings.Contains(e, "gpu lost") || strings.Contains(e, "gpu is lost") || strings.Contains(e, "gpu_is_lost")
}

// IsGPURequiresReset returns true if nvml.ErrorString(ret) indicates that the GPU requires reset.
// e.g., "GPU requires reset".
func IsGPURequiresReset(ret nvml.Return) bool {
	e := normalizeNVMLReturnString(ret)
	return strings.Contains(e, "gpu requires reset")
}

// normalizeNVMLReturnString normalizes an NVML return to a string.
func normalizeNVMLReturnString(ret nvml.Return) string {
	s := nvml.ErrorString(ret)
	return strings.ToLower(strings.TrimSpace(s))
}
