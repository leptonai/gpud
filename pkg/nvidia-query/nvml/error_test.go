package nvml

import (
	"testing"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/stretchr/testify/assert"
)

func TestIsNotSupportError(t *testing.T) {
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

	// Override nvml.ErrorString for testing string-based matches
	originalErrorString := nvml.ErrorString
	defer func() {
		nvml.ErrorString = originalErrorString
	}()

	nvml.ErrorString = func(ret nvml.Return) string {
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
			return "notsupported" // No space between 'not' and 'supported'
		default:
			return originalErrorString(ret)
		}
	}

	// Add string-based test cases
	stringBasedTests := []struct {
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

	tests = append(tests, stringBasedTests...)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsNotSupportError(tt.ret)
			assert.Equal(t, tt.expected, result, "IsNotSupportError(%v) = %v, want %v", tt.ret, result, tt.expected)
		})
	}
}

// TestIsNotSupportErrorStringMatch tests the string-based matching of not supported errors
func TestIsNotSupportErrorStringMatch(t *testing.T) {
	// Create a custom Return type that will produce different error strings
	tests := []struct {
		name     string
		ret      nvml.Return
		expected bool
	}{
		{
			name:     "String contains 'not supported' (lowercase)",
			ret:      nvml.Return(1000), // This will produce "Unknown Error" which we'll handle in ErrorString
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

	// Override nvml.ErrorString for testing
	originalErrorString := nvml.ErrorString
	defer func() {
		nvml.ErrorString = originalErrorString
	}()

	nvml.ErrorString = func(ret nvml.Return) string {
		switch ret {
		case 1000:
			return "operation is not supported on this device"
		case 1001:
			return "THIS OPERATION IS NOT SUPPORTED"
		case 1002:
			return "Feature Not Supported"
		case 1003:
			return "  not supported  "
		case 1004:
			return "The requested operation is not supported on device 0"
		case 1005:
			return "Some other error"
		case 1006:
			return ""
		case 1007:
			return "notsupported" // No space between 'not' and 'supported'
		default:
			return originalErrorString(ret)
		}
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsNotSupportError(tt.ret)
			assert.Equal(t, tt.expected, result, "IsNotSupportError(%v) = %v, want %v", tt.ret, result, tt.expected)
		})
	}
}

func TestNormalizeErrorString(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Lowercase string",
			input:    "error message",
			expected: "error message",
		},
		{
			name:     "Uppercase string",
			input:    "ERROR MESSAGE",
			expected: "error message",
		},
		{
			name:     "Mixed case string",
			input:    "Error Message",
			expected: "error message",
		},
		{
			name:     "String with leading/trailing spaces",
			input:    "  Error Message  ",
			expected: "error message",
		},
		{
			name:     "String with multiple spaces",
			input:    "Error    Message",
			expected: "error    message",
		},
		{
			name:     "Empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "Only whitespace",
			input:    "   ",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizeErrorString(tt.input)
			assert.Equal(t, tt.expected, result, "normalizeErrorString(%q) = %q, want %q", tt.input, result, tt.expected)
		})
	}
}

func TestIsVersionMismatchError(t *testing.T) {
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

	// Override nvml.ErrorString for testing string-based matches
	originalErrorString := nvml.ErrorString
	defer func() {
		nvml.ErrorString = originalErrorString
	}()

	nvml.ErrorString = func(ret nvml.Return) string {
		if ret == nvml.Return(1000) {
			return "operation failed due to version mismatch"
		}
		if ret == nvml.Return(1001) {
			return "ERROR: VERSION MISMATCH DETECTED"
		}
		if ret == nvml.Return(1002) {
			return "The API call failed: Version Mismatch between components"
		}
		return originalErrorString(ret)
	}

	// Add string-based test cases
	stringBasedTests := []struct {
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

	tests = append(tests, stringBasedTests...)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsVersionMismatchError(tt.ret)
			assert.Equal(t, tt.expected, result, "IsVersionMismatchError(%v) = %v, want %v", tt.ret, result, tt.expected)
		})
	}
}
