package errors

import (
	"errors"
	"testing"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/stretchr/testify/assert"
)

func TestIsNotSupportError(t *testing.T) {
	// Basic constant tests
	tests := []struct {
		name     string
		ret      nvml.Return
		expected bool
	}{
		{
			name:     "Direct ERROR_NOT_SUPPORTED match",
			ret:      nvml.ERROR_NOT_SUPPORTED,
			expected: true,
		},
		{
			name:     "Success is not a not-supported error",
			ret:      nvml.SUCCESS,
			expected: false,
		},
		{
			name:     "Unknown error is not a not-supported error",
			ret:      nvml.ERROR_UNKNOWN,
			expected: false,
		},
		{
			name:     "Version mismatch error is not a not-supported error",
			ret:      nvml.ERROR_ARGUMENT_VERSION_MISMATCH,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsNotSupportError(tt.ret)
			assert.Equal(t, tt.expected, result)
		})
	}

	// String-based matching tests
	t.Run("String-based matches", func(t *testing.T) {
		mockErrorString := func(ret nvml.Return) string {
			switch ret {
			case nvml.Return(1000):
				return "operation is not supported on this device"
			case nvml.Return(1001):
				return "THIS OPERATION IS NOT SUPPORTED"
			case nvml.Return(1002):
				return "Feature Not Supported"
			case nvml.Return(1003):
				return "  not supported  "
			case nvml.Return(1004):
				return "The requested operation is not supported on device 0"
			case nvml.Return(1005):
				return "Some other error"
			case nvml.Return(1006):
				return ""
			case nvml.Return(1007):
				return "notsupported"
			default:
				return nvml.ErrorString(ret)
			}
		}

		stringTests := []struct {
			name     string
			ret      nvml.Return
			expected bool
		}{
			{
				name:     "String contains 'not supported' (lowercase)",
				ret:      nvml.Return(1000),
				expected: true,
			},
			{
				name:     "String contains 'NOT SUPPORTED' (uppercase)",
				ret:      nvml.Return(1001),
				expected: true,
			},
			{
				name:     "String contains 'Not Supported' (mixed case)",
				ret:      nvml.Return(1002),
				expected: true,
			},
			{
				name:     "String contains 'not supported' with leading/trailing spaces",
				ret:      nvml.Return(1003),
				expected: true,
			},
			{
				name:     "String contains 'not supported' within a longer message",
				ret:      nvml.Return(1004),
				expected: true,
			},
			{
				name:     "String does not contain 'not supported'",
				ret:      nvml.Return(1005),
				expected: false,
			},
			{
				name:     "Empty string",
				ret:      nvml.Return(1006),
				expected: false,
			},
			{
				name:     "String with similar but not exact match",
				ret:      nvml.Return(1007),
				expected: false,
			},
		}

		for _, tt := range stringTests {
			t.Run(tt.name, func(t *testing.T) {
				result := isNotSupportError(tt.ret, mockErrorString)
				assert.Equal(t, tt.expected, result)
			})
		}
	})
}

func TestIsVersionMismatchError(t *testing.T) {
	// Basic constant tests
	tests := []struct {
		name     string
		ret      nvml.Return
		expected bool
	}{
		{
			name:     "Direct ERROR_ARGUMENT_VERSION_MISMATCH match",
			ret:      nvml.ERROR_ARGUMENT_VERSION_MISMATCH,
			expected: true,
		},
		{
			name:     "Success is not a version mismatch error",
			ret:      nvml.SUCCESS,
			expected: false,
		},
		{
			name:     "Unknown error is not a version mismatch error",
			ret:      nvml.ERROR_UNKNOWN,
			expected: false,
		},
		{
			name:     "Not supported error is not a version mismatch error",
			ret:      nvml.ERROR_NOT_SUPPORTED,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsVersionMismatchError(tt.ret)
			assert.Equal(t, tt.expected, result)
		})
	}

	// String-based matching tests
	t.Run("String-based matches", func(t *testing.T) {
		mockErrorString := func(ret nvml.Return) string {
			switch ret {
			case nvml.Return(1000):
				return "operation failed due to version mismatch"
			case nvml.Return(1001):
				return "ERROR: VERSION MISMATCH DETECTED"
			case nvml.Return(1002):
				return "The API call failed: Version Mismatch between components"
			default:
				return nvml.ErrorString(ret)
			}
		}

		stringTests := []struct {
			name     string
			ret      nvml.Return
			expected bool
		}{
			{
				name:     "String contains 'version mismatch' (lowercase)",
				ret:      nvml.Return(1000),
				expected: true,
			},
			{
				name:     "String contains 'VERSION MISMATCH' (uppercase)",
				ret:      nvml.Return(1001),
				expected: true,
			},
			{
				name:     "String contains 'Version Mismatch' within message",
				ret:      nvml.Return(1002),
				expected: true,
			},
		}

		for _, tt := range stringTests {
			t.Run(tt.name, func(t *testing.T) {
				result := isVersionMismatchError(tt.ret, mockErrorString)
				assert.Equal(t, tt.expected, result)
			})
		}
	})
}

func TestIsNotReadyError(t *testing.T) {
	// Basic constant tests
	tests := []struct {
		name     string
		ret      nvml.Return
		expected bool
	}{
		{
			name:     "Direct ERROR_NOT_READY match",
			ret:      nvml.ERROR_NOT_READY,
			expected: true,
		},
		{
			name:     "Success is not a not-ready error",
			ret:      nvml.SUCCESS,
			expected: false,
		},
		{
			name:     "Unknown error is not a not-ready error",
			ret:      nvml.ERROR_UNKNOWN,
			expected: false,
		},
		{
			name:     "Not supported error is not a not-ready error",
			ret:      nvml.ERROR_NOT_SUPPORTED,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsNotReadyError(tt.ret)
			assert.Equal(t, tt.expected, result)
		})
	}

	// String-based matching tests
	t.Run("String-based matches", func(t *testing.T) {
		mockErrorString := func(ret nvml.Return) string {
			switch ret {
			case nvml.Return(1000):
				return "System is not in ready state"
			case nvml.Return(1001):
				return "SYSTEM IS NOT IN READY STATE"
			case nvml.Return(1002):
				return "nvml.CLOCK_GRAPHICS: System is not in ready state"
			case nvml.Return(1003):
				return "  not in ready  "
			case nvml.Return(1004):
				return "The system is not in ready state for this operation"
			case nvml.Return(1005):
				return "Some other error"
			case nvml.Return(1006):
				return ""
			case nvml.Return(1007):
				return "notinready"
			default:
				return nvml.ErrorString(ret)
			}
		}

		stringTests := []struct {
			name     string
			ret      nvml.Return
			expected bool
		}{
			{
				name:     "String contains 'not in ready' (lowercase)",
				ret:      nvml.Return(1000),
				expected: true,
			},
			{
				name:     "String contains 'NOT IN READY' (uppercase)",
				ret:      nvml.Return(1001),
				expected: true,
			},
			{
				name:     "String contains 'not in ready' with prefix",
				ret:      nvml.Return(1002),
				expected: true,
			},
			{
				name:     "String contains 'not in ready' with spaces",
				ret:      nvml.Return(1003),
				expected: true,
			},
			{
				name:     "String contains 'not in ready' within message",
				ret:      nvml.Return(1004),
				expected: true,
			},
			{
				name:     "String does not contain 'not in ready'",
				ret:      nvml.Return(1005),
				expected: false,
			},
			{
				name:     "Empty string",
				ret:      nvml.Return(1006),
				expected: false,
			},
			{
				name:     "String with similar but not exact match",
				ret:      nvml.Return(1007),
				expected: false,
			},
		}

		for _, tt := range stringTests {
			t.Run(tt.name, func(t *testing.T) {
				result := isNotReadyError(tt.ret, mockErrorString)
				assert.Equal(t, tt.expected, result)
			})
		}
	})
}

func TestIsNotFoundError(t *testing.T) {
	// Basic constant tests
	tests := []struct {
		name     string
		ret      nvml.Return
		expected bool
	}{
		{
			name:     "Direct ERROR_NOT_FOUND match",
			ret:      nvml.ERROR_NOT_FOUND,
			expected: true,
		},
		{
			name:     "Success is not a not-found error",
			ret:      nvml.SUCCESS,
			expected: false,
		},
		{
			name:     "Unknown error is not a not-found error",
			ret:      nvml.ERROR_UNKNOWN,
			expected: false,
		},
		{
			name:     "Not supported error is not a not-found error",
			ret:      nvml.ERROR_NOT_SUPPORTED,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsNotFoundError(tt.ret)
			assert.Equal(t, tt.expected, result)
		})
	}

	// String-based matching tests
	t.Run("String-based matches", func(t *testing.T) {
		mockErrorString := func(ret nvml.Return) string {
			switch ret {
			case nvml.Return(1000):
				return "process not found"
			case nvml.Return(1001):
				return "PROCESS NOT FOUND"
			case nvml.Return(1002):
				return "Device Not Found"
			case nvml.Return(1003):
				return "  not found  "
			case nvml.Return(1004):
				return "The requested object was not found on device 0"
			case nvml.Return(1005):
				return "Object not_found in database"
			case nvml.Return(1006):
				return "Some other error"
			case nvml.Return(1007):
				return ""
			case nvml.Return(1008):
				return "notfound"
			default:
				return nvml.ErrorString(ret)
			}
		}

		stringTests := []struct {
			name     string
			ret      nvml.Return
			expected bool
		}{
			{
				name:     "String contains 'not found' (lowercase)",
				ret:      nvml.Return(1000),
				expected: true,
			},
			{
				name:     "String contains 'NOT FOUND' (uppercase)",
				ret:      nvml.Return(1001),
				expected: true,
			},
			{
				name:     "String contains 'Not Found' (mixed case)",
				ret:      nvml.Return(1002),
				expected: true,
			},
			{
				name:     "String contains 'not found' with leading/trailing spaces",
				ret:      nvml.Return(1003),
				expected: true,
			},
			{
				name:     "String contains 'not found' within a longer message",
				ret:      nvml.Return(1004),
				expected: true,
			},
			{
				name:     "String contains 'not_found'",
				ret:      nvml.Return(1005),
				expected: true,
			},
			{
				name:     "String does not contain 'not found' or 'not_found'",
				ret:      nvml.Return(1006),
				expected: false,
			},
			{
				name:     "Empty string",
				ret:      nvml.Return(1007),
				expected: false,
			},
			{
				name:     "String with similar but not exact match",
				ret:      nvml.Return(1008),
				expected: false,
			},
		}

		for _, tt := range stringTests {
			t.Run(tt.name, func(t *testing.T) {
				result := isNotFoundError(tt.ret, mockErrorString)
				assert.Equal(t, tt.expected, result)
			})
		}
	})
}

func TestIsGPULostError(t *testing.T) {
	// Basic constant tests
	tests := []struct {
		name     string
		ret      nvml.Return
		expected bool
	}{
		{
			name:     "ERROR_GPU_IS_LOST constant",
			ret:      nvml.ERROR_GPU_IS_LOST,
			expected: true,
		},
		{
			name:     "Success is not a GPU lost error",
			ret:      nvml.SUCCESS,
			expected: false,
		},
		{
			name:     "Unknown error is not a GPU lost error",
			ret:      nvml.ERROR_UNKNOWN,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsGPULostError(tt.ret)
			assert.Equal(t, tt.expected, result)
		})
	}

	// String-based matching tests
	t.Run("String-based matches", func(t *testing.T) {
		mockErrorString := func(ret nvml.Return) string {
			switch ret {
			case nvml.Return(9999):
				return "the GPU lost error occurred"
			case nvml.Return(9998):
				return "the gpu is lost error message"
			case nvml.Return(9997):
				return "gpu_is_lost encountered"
			case nvml.Return(9996):
				return "this is an unrelated error"
			default:
				return nvml.ErrorString(ret)
			}
		}

		stringTests := []struct {
			name     string
			ret      nvml.Return
			expected bool
		}{
			{
				name:     "String contains 'GPU lost'",
				ret:      nvml.Return(9999),
				expected: true,
			},
			{
				name:     "String contains 'gpu is lost'",
				ret:      nvml.Return(9998),
				expected: true,
			},
			{
				name:     "String contains 'gpu_is_lost'",
				ret:      nvml.Return(9997),
				expected: true,
			},
			{
				name:     "Unrelated error",
				ret:      nvml.Return(9996),
				expected: false,
			},
		}

		for _, tt := range stringTests {
			t.Run(tt.name, func(t *testing.T) {
				result := isGPULostError(tt.ret, mockErrorString)
				assert.Equal(t, tt.expected, result)
			})
		}
	})
}

func TestIsGPURequiresReset(t *testing.T) {
	// Basic constant tests
	tests := []struct {
		name     string
		ret      nvml.Return
		expected bool
	}{
		{
			name:     "Direct ERROR_RESET_REQUIRED match",
			ret:      nvml.ERROR_RESET_REQUIRED,
			expected: true,
		},
		{
			name:     "Success is not a reset-required error",
			ret:      nvml.SUCCESS,
			expected: false,
		},
		{
			name:     "GPU_LOST is not a reset-required error",
			ret:      nvml.ERROR_GPU_IS_LOST,
			expected: false,
		},
		{
			name:     "Unknown error is not a reset-required error",
			ret:      nvml.ERROR_UNKNOWN,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsGPURequiresReset(tt.ret)
			assert.Equal(t, tt.expected, result)
		})
	}

	// String-based matching tests
	t.Run("String-based matches", func(t *testing.T) {
		mockErrorString := func(ret nvml.Return) string {
			switch ret {
			case nvml.Return(2000):
				return "GPU requires reset"
			case nvml.Return(2001):
				return "gpu reset detected"
			case nvml.Return(2002):
				return "GPU REQUIRES RESET"
			case nvml.Return(2003):
				return "  gpu reset  "
			case nvml.Return(2004):
				return "Some other error"
			default:
				return nvml.ErrorString(ret)
			}
		}

		stringTests := []struct {
			name     string
			ret      nvml.Return
			expected bool
		}{
			{
				name:     "String contains 'GPU requires reset'",
				ret:      nvml.Return(2000),
				expected: true,
			},
			{
				name:     "String contains 'gpu reset'",
				ret:      nvml.Return(2001),
				expected: true,
			},
			{
				name:     "Uppercase 'GPU REQUIRES RESET'",
				ret:      nvml.Return(2002),
				expected: true,
			},
			{
				name:     "With whitespace",
				ret:      nvml.Return(2003),
				expected: true,
			},
			{
				name:     "No match",
				ret:      nvml.Return(2004),
				expected: false,
			},
		}

		for _, tt := range stringTests {
			t.Run(tt.name, func(t *testing.T) {
				result := isGPURequiresReset(tt.ret, mockErrorString)
				assert.Equal(t, tt.expected, result)
			})
		}
	})
}

func TestIsNoSuchFileOrDirectoryError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "not found error",
			err:      errors.New("file not found"),
			expected: true,
		},
		{
			name:     "no such file or directory error",
			err:      errors.New("no such file or directory"),
			expected: true,
		},
		{
			name:     "mixed case error",
			err:      errors.New("No SuCh FiLe Or DiReCtoRy"),
			expected: true,
		},
		{
			name:     "different error",
			err:      errors.New("permission denied"),
			expected: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := IsNoSuchFileOrDirectoryError(test.err)
			assert.Equal(t, test.expected, result)
		})
	}
}
