package providers

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDetector_PrivateIPv4(t *testing.T) {
	const testProviderName = "test-provider-for-private-ip"
	tests := []struct {
		name               string
		fetchPrivateIPFunc func(ctx context.Context) (string, error)
		expectedResult     string
		expectedError      error
	}{
		{
			name: "successful private IP fetch",
			fetchPrivateIPFunc: func(ctx context.Context) (string, error) {
				return "10.0.1.5", nil
			},
			expectedResult: "10.0.1.5",
			expectedError:  nil,
		},
		{
			name: "private IP fetch error",
			fetchPrivateIPFunc: func(ctx context.Context) (string, error) {
				return "", errors.New("private IP fetch failed")
			},
			expectedResult: "",
			expectedError:  errors.New("private IP fetch failed"),
		},
		{
			name:               "nil fetch function",
			fetchPrivateIPFunc: nil,
			expectedResult:     "", // PrivateIPv4 method should handle nil fetchPrivateIPFunc gracefully
			expectedError:      nil,
		},
		{
			name: "empty private IP",
			fetchPrivateIPFunc: func(ctx context.Context) (string, error) {
				return "", nil
			},
			expectedResult: "",
			expectedError:  nil,
		},
		{
			name: "IPv6 private address",
			fetchPrivateIPFunc: func(ctx context.Context) (string, error) {
				return "fd00::1", nil
			},
			expectedResult: "fd00::1",
			expectedError:  nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Use the New function from the package
			d := New(testProviderName, nil, nil, tc.fetchPrivateIPFunc, nil, nil)

			result, err := d.PrivateIPv4(context.Background())

			if tc.expectedError != nil {
				assert.EqualError(t, err, tc.expectedError.Error())
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tc.expectedResult, result)
		})
	}
}

func TestDetector_InstanceID(t *testing.T) {
	const testProviderName = "test-provider-for-instance-id"
	tests := []struct {
		name                string
		fetchInstanceIDFunc func(ctx context.Context) (string, error)
		expectedResult      string
		expectedError       error
	}{
		{
			name: "successful instance ID fetch",
			fetchInstanceIDFunc: func(ctx context.Context) (string, error) {
				return "i-0123456789abcdef", nil
			},
			expectedResult: "i-0123456789abcdef",
			expectedError:  nil,
		},
		{
			name: "instance ID fetch error",
			fetchInstanceIDFunc: func(ctx context.Context) (string, error) {
				return "", errors.New("instance ID fetch failed")
			},
			expectedResult: "",
			expectedError:  errors.New("instance ID fetch failed"),
		},
		{
			name:                "nil fetch function",
			fetchInstanceIDFunc: nil,
			expectedResult:      "", // InstanceID method should handle nil fetchInstanceIDFunc gracefully
			expectedError:       nil,
		},
		{
			name: "empty instance ID",
			fetchInstanceIDFunc: func(ctx context.Context) (string, error) {
				return "", nil
			},
			expectedResult: "",
			expectedError:  nil,
		},
		{
			name: "AWS instance ID format",
			fetchInstanceIDFunc: func(ctx context.Context) (string, error) {
				return "i-1234567890abcdef0", nil
			},
			expectedResult: "i-1234567890abcdef0",
			expectedError:  nil,
		},
		{
			name: "Azure instance ID format",
			fetchInstanceIDFunc: func(ctx context.Context) (string, error) {
				return "12345678-1234-1234-1234-123456789abc", nil
			},
			expectedResult: "12345678-1234-1234-1234-123456789abc",
			expectedError:  nil,
		},
		{
			name: "GCP instance ID format",
			fetchInstanceIDFunc: func(ctx context.Context) (string, error) {
				return "1234567890123456789", nil
			},
			expectedResult: "1234567890123456789",
			expectedError:  nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Use the New function from the package
			d := New(testProviderName, nil, nil, nil, nil, tc.fetchInstanceIDFunc)

			result, err := d.InstanceID(context.Background())

			if tc.expectedError != nil {
				assert.EqualError(t, err, tc.expectedError.Error())
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tc.expectedResult, result)
		})
	}
}

func TestDetector_AllMethodsCombined(t *testing.T) {
	// Test a detector with all methods configured
	testName := "complete-provider"

	d := New(
		testName,
		func(ctx context.Context) (string, error) { return "provider-token", nil },
		func(ctx context.Context) (string, error) { return "203.0.113.1", nil },
		func(ctx context.Context) (string, error) { return "10.0.1.5", nil },
		func(ctx context.Context) (string, error) { return "production", nil },
		func(ctx context.Context) (string, error) { return "i-abc123", nil },
	)

	ctx := context.Background()

	// Test Name
	assert.Equal(t, testName, d.Name())

	// Test Provider
	provider, err := d.Provider(ctx)
	require.NoError(t, err)
	assert.Equal(t, testName, provider)

	// Test PublicIPv4
	publicIP, err := d.PublicIPv4(ctx)
	require.NoError(t, err)
	assert.Equal(t, "203.0.113.1", publicIP)

	// Test PrivateIPv4
	privateIP, err := d.PrivateIPv4(ctx)
	require.NoError(t, err)
	assert.Equal(t, "10.0.1.5", privateIP)

	// Test VMEnvironment
	vmEnv, err := d.VMEnvironment(ctx)
	require.NoError(t, err)
	assert.Equal(t, "production", vmEnv)

	// Test InstanceID
	instanceID, err := d.InstanceID(ctx)
	require.NoError(t, err)
	assert.Equal(t, "i-abc123", instanceID)
}

func TestDetector_ContextCancellation(t *testing.T) {
	// Test that context cancellation is properly propagated
	testName := "context-test-provider"

	callCount := 0
	d := New(
		testName,
		func(ctx context.Context) (string, error) {
			callCount++
			if err := ctx.Err(); err != nil {
				return "", err
			}
			return "token", nil
		},
		nil, nil, nil, nil,
	)

	// Create a canceled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Call Provider with canceled context
	_, err := d.Provider(ctx)
	require.Error(t, err)
	assert.Equal(t, context.Canceled, err)
	assert.Equal(t, 1, callCount)
}
