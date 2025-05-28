package providers

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

// NOTE: The 'detector' struct and its methods (Name, Provider, PublicIPv4, VMEnvironment)
// as well as the 'New' constructor are defined in detect.go.

func TestDetector_Name(t *testing.T) {
	testProviderName := "test-provider"
	// Use the New function from the package, providing nil for functions not directly tested by Name()
	d := New(testProviderName, nil, nil, nil, nil)
	assert.Equal(t, testProviderName, d.Name())
}

func TestDetector_Provider(t *testing.T) {
	const testProviderName = "test-provider" // Consistent name for tests
	tests := []struct {
		name           string
		fetchTokenFunc func(ctx context.Context) (string, error)
		expectedResult string
		expectedError  error
	}{
		{
			name: "successful detection",
			fetchTokenFunc: func(ctx context.Context) (string, error) {
				return "token-value", nil
			},
			expectedResult: testProviderName, // Should match the name used to create the detector
			expectedError:  nil,
		},
		{
			name: "empty token",
			fetchTokenFunc: func(ctx context.Context) (string, error) {
				return "", nil
			},
			expectedResult: "",
			expectedError:  nil,
		},
		{
			name: "token fetch error",
			fetchTokenFunc: func(ctx context.Context) (string, error) {
				return "", errors.New("token fetch failed")
			},
			expectedResult: "",
			expectedError:  errors.New("token fetch failed"),
		},
		{
			name:           "nil fetch function",
			fetchTokenFunc: nil,
			expectedResult: "", // Provider method should handle nil fetchTokenFunc gracefully
			expectedError:  nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Use the New function from the package
			// Provide nil for functions not relevant to Provider method testing
			d := New(testProviderName, tc.fetchTokenFunc, nil, nil, nil)

			result, err := d.Provider(context.Background())

			if tc.expectedError != nil {
				assert.EqualError(t, err, tc.expectedError.Error())
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tc.expectedResult, result)
		})
	}
}

func TestDetector_PublicIPv4(t *testing.T) {
	const testProviderName = "test-provider-for-ip"
	tests := []struct {
		name              string
		fetchPublicIPFunc func(ctx context.Context) (string, error)
		expectedResult    string
		expectedError     error
	}{
		{
			name: "successful IP fetch",
			fetchPublicIPFunc: func(ctx context.Context) (string, error) {
				return "203.0.113.1", nil
			},
			expectedResult: "203.0.113.1",
			expectedError:  nil,
		},
		{
			name: "IP fetch error",
			fetchPublicIPFunc: func(ctx context.Context) (string, error) {
				return "", errors.New("IP fetch failed")
			},
			expectedResult: "",
			expectedError:  errors.New("IP fetch failed"),
		},
		{
			name:              "nil fetch function",
			fetchPublicIPFunc: nil,
			expectedResult:    "", // PublicIPv4 method should handle nil fetchPublicIPFunc gracefully
			expectedError:     nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Use the New function from the package
			d := New(testProviderName, nil, tc.fetchPublicIPFunc, nil, nil)

			result, err := d.PublicIPv4(context.Background())

			if tc.expectedError != nil {
				assert.EqualError(t, err, tc.expectedError.Error())
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tc.expectedResult, result)
		})
	}
}

func TestDetector_VMEnvironment(t *testing.T) {
	const testProviderName = "test-provider-for-vm-env"
	tests := []struct {
		name                   string
		fetchVMEnvironmentFunc func(ctx context.Context) (string, error)
		expectedResult         string
		expectedError          error
	}{
		{
			name: "successful vm environment fetch",
			fetchVMEnvironmentFunc: func(ctx context.Context) (string, error) {
				return "AZUREPUBLICCLOUD", nil
			},
			expectedResult: "AZUREPUBLICCLOUD",
			expectedError:  nil,
		},
		{
			name: "vm environment fetch error",
			fetchVMEnvironmentFunc: func(ctx context.Context) (string, error) {
				return "", errors.New("vm environment fetch failed")
			},
			expectedResult: "",
			expectedError:  errors.New("vm environment fetch failed"),
		},
		{
			name:                   "nil fetch function",
			fetchVMEnvironmentFunc: nil,
			expectedResult:         "", // VMEnvironment method should handle nil fetchVMEnvironmentFunc gracefully
			expectedError:          nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Use the New function from the package
			d := New(testProviderName, nil, nil, tc.fetchVMEnvironmentFunc, nil)

			result, err := d.VMEnvironment(context.Background())

			if tc.expectedError != nil {
				assert.EqualError(t, err, tc.expectedError.Error())
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tc.expectedResult, result)
		})
	}
}

func TestNew(t *testing.T) {
	testName := "test-new-provider"
	mockTokenFunc := func(ctx context.Context) (string, error) { return "test-token", nil }
	mockIPFunc := func(ctx context.Context) (string, error) { return "test-ip", nil }
	mockVMEnvFunc := func(ctx context.Context) (string, error) { return "test-env", nil }

	d := New(testName, mockTokenFunc, mockIPFunc, mockVMEnvFunc, nil)

	// Check that detector is properly initialized
	assert.NotNil(t, d)

	// We can't directly assert the type of 'd' to a private '*detector' struct from another file.
	// Instead, we check its behavior through the Detector interface.
	assert.Equal(t, testName, d.Name(), "Name() should return the name passed to New")

	// Verify that the functions are being used by calling the methods
	token, err := d.Provider(context.Background()) // Provider uses token func and name
	assert.NoError(t, err)
	assert.Equal(t, testName, token, "Provider() should use the provided name on successful token fetch")

	ip, err := d.PublicIPv4(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, "test-ip", ip, "PublicIPv4() should use the provided fetchPublicIPv4Func")

	vmEnv, err := d.VMEnvironment(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, "test-env", vmEnv, "VMEnvironment() should use the provided fetchVMEnvironmentFunc")

	// Test nil function cases for Provider, PublicIPv4, VMEnvironment
	dNilFuncs := New("niltest", nil, nil, nil, nil)
	provNil, errNil := dNilFuncs.Provider(context.Background())
	assert.NoError(t, errNil)
	assert.Equal(t, "", provNil)

	ipNil, errNil := dNilFuncs.PublicIPv4(context.Background())
	assert.NoError(t, errNil)
	assert.Equal(t, "", ipNil)

	vmEnvNil, errNil := dNilFuncs.VMEnvironment(context.Background())
	assert.NoError(t, errNil)
	assert.Equal(t, "", vmEnvNil)
}
