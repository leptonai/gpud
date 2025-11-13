package errors

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
	ErrGPULost = errors.New("GPU lost")
	// ErrGPURequiresReset is an error that indicates the GPU requires reset.
	// This typically appears when NVML reports "GPU requires reset".
	ErrGPURequiresReset = errors.New("GPU requires reset")
)

// errorStringFunc is the type signature for error string functions.
// Used for dependency injection in tests.
type errorStringFunc func(nvml.Return) string

// normalizeNVMLReturnString normalizes an NVML return to a string using the provided function.
func normalizeNVMLReturnString(ret nvml.Return, errStrFunc errorStringFunc) string {
	s := errStrFunc(ret)
	return strings.ToLower(strings.TrimSpace(s))
}

// isGPULostError is the internal implementation that accepts a custom errorStringFunc.
// Used for testing with mocked error string functions.
func isGPULostError(ret nvml.Return, errStrFunc errorStringFunc) bool {
	if ret == nvml.ERROR_GPU_IS_LOST {
		return true
	}

	e := normalizeNVMLReturnString(ret, errStrFunc)
	return strings.Contains(e, "gpu lost") || strings.Contains(e, "gpu is lost") || strings.Contains(e, "gpu_is_lost")
}

// IsGPULostError returns true if the error indicates that the GPU is lost.
// "if the target GPU has fallen off the bus or is otherwise inaccessible".
func IsGPULostError(ret nvml.Return) bool {
	return isGPULostError(ret, nvml.ErrorString)
}

// isGPURequiresReset is the internal implementation that accepts a custom errorStringFunc.
func isGPURequiresReset(ret nvml.Return, errStrFunc errorStringFunc) bool {
	if ret == nvml.ERROR_RESET_REQUIRED {
		return true
	}

	e := normalizeNVMLReturnString(ret, errStrFunc)
	return strings.Contains(e, "gpu requires reset") || strings.Contains(e, "gpu reset")
}

// IsGPURequiresReset returns true if nvml.ErrorString(ret) indicates that the GPU requires reset.
// e.g., "GPU requires reset".
func IsGPURequiresReset(ret nvml.Return) bool {
	return isGPURequiresReset(ret, nvml.ErrorString)
}

// isVersionMismatchError is the internal implementation that accepts a custom errorStringFunc.
func isVersionMismatchError(ret nvml.Return, errStrFunc errorStringFunc) bool {
	if ret == nvml.ERROR_ARGUMENT_VERSION_MISMATCH {
		return true
	}

	e := normalizeNVMLReturnString(ret, errStrFunc)
	return strings.Contains(e, "version mismatch")
}

// IsVersionMismatchError returns true if the error indicates a version mismatch.
func IsVersionMismatchError(ret nvml.Return) bool {
	return isVersionMismatchError(ret, nvml.ErrorString)
}

// isNotSupportError is the internal implementation that accepts a custom errorStringFunc.
func isNotSupportError(ret nvml.Return, errStrFunc errorStringFunc) bool {
	if ret == nvml.ERROR_NOT_SUPPORTED {
		return true
	}

	// e.g., "Not Supported"
	e := normalizeNVMLReturnString(ret, errStrFunc)
	return strings.Contains(e, "not supported")
}

// IsNotSupportError returns true if the error indicates that the operation is not supported.
func IsNotSupportError(ret nvml.Return) bool {
	return isNotSupportError(ret, nvml.ErrorString)
}

// isNotReadyError is the internal implementation that accepts a custom errorStringFunc.
func isNotReadyError(ret nvml.Return, errStrFunc errorStringFunc) bool {
	if ret == nvml.ERROR_NOT_READY {
		return true
	}

	e := normalizeNVMLReturnString(ret, errStrFunc)
	return strings.Contains(e, "not in ready")
}

// IsNotReadyError returns true if the error indicates that the system is not ready,
// meaning that the GPU is not yet initialized.
// e.g.,
// "nvml.CLOCK_GRAPHICS: System is not in ready state"
func IsNotReadyError(ret nvml.Return) bool {
	return isNotReadyError(ret, nvml.ErrorString)
}

// isNotFoundError is the internal implementation that accepts a custom errorStringFunc.
func isNotFoundError(ret nvml.Return, errStrFunc errorStringFunc) bool {
	if ret == nvml.ERROR_NOT_FOUND {
		return true
	}

	e := normalizeNVMLReturnString(ret, errStrFunc)
	return strings.Contains(e, "not found") || strings.Contains(e, "not_found")
}

// IsNotFoundError returns true if the error indicates that the object/instance is not found.
// e.g., process not found from nvml
func IsNotFoundError(ret nvml.Return) bool {
	return isNotFoundError(ret, nvml.ErrorString)
}

func IsNoSuchFileOrDirectoryError(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "not found") || strings.Contains(s, "no such file or directory")
}
