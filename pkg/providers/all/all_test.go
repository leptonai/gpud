package all

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/leptonai/gpud/pkg/providers"
)

// mockDetector implements the providers.Detector interface for testing
type mockDetector struct {
	name          string
	provider      string
	publicIP      string
	privateIP     string
	provErr       error
	publicErr     error
	privateErr    error
	vmEnv         string
	vmEnvErr      error
	instanceID    string
	instanceIDErr error
	delay         time.Duration
}

func (m *mockDetector) Name() string {
	return m.name
}

func (m *mockDetector) Provider(ctx context.Context) (string, error) {
	if m.delay > 0 {
		select {
		case <-time.After(m.delay):
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	return m.provider, m.provErr
}

func (m *mockDetector) PublicIPv4(ctx context.Context) (string, error) {
	return m.publicIP, m.publicErr
}

func (m *mockDetector) PrivateIPv4(ctx context.Context) (string, error) {
	return m.privateIP, m.privateErr
}

func (m *mockDetector) VMEnvironment(ctx context.Context) (string, error) {
	return m.vmEnv, m.vmEnvErr
}

func (m *mockDetector) InstanceID(ctx context.Context) (string, error) {
	return m.instanceID, m.instanceIDErr
}

// withTemporaryDetectors runs the provided function with a temporary replacement for All
// and restores the original value when done
func withTemporaryDetectors(tempDetectors []providers.Detector, fn func()) {
	orig := All
	All = tempDetectors
	defer func() {
		All = orig
	}()
	fn()
}

func TestDetect_Success(t *testing.T) {
	testDetectors := []providers.Detector{
		&mockDetector{
			name:       "aws",
			provider:   "aws",
			publicIP:   "1.2.3.4",
			privateIP:  "10.0.1.100",
			vmEnv:      "AWS",
			instanceID: "i-abc",
		},
	}

	withTemporaryDetectors(testDetectors, func() {
		info, err := Detect(context.Background())
		assert.NoError(t, err)
		assert.Equal(t, "aws", info.Provider)
		assert.Equal(t, "1.2.3.4", info.PublicIP)
		assert.Equal(t, "10.0.1.100", info.PrivateIP)
		assert.Equal(t, "AWS", info.VMEnvironment)
		assert.Equal(t, "i-abc", info.InstanceID)
	})
}

func TestDetect_SkipOnEmptyProvider(t *testing.T) {
	testDetectors := []providers.Detector{
		&mockDetector{
			name:      "aws",
			provider:  "", // Empty provider name
			publicIP:  "1.2.3.4",
			privateIP: "10.0.1.100",
			vmEnv:     "AWS",
		},
		&mockDetector{
			name:       "azure",
			provider:   "azure",
			publicIP:   "5.6.7.8",
			privateIP:  "10.0.2.200",
			vmEnv:      "AZURE",
			instanceID: "az-vm-1",
		},
	}

	withTemporaryDetectors(testDetectors, func() {
		info, err := Detect(context.Background())
		assert.NoError(t, err)
		assert.Equal(t, "azure", info.Provider)
		assert.Equal(t, "5.6.7.8", info.PublicIP)
		assert.Equal(t, "10.0.2.200", info.PrivateIP)
		assert.Equal(t, "AZURE", info.VMEnvironment)
		assert.Equal(t, "az-vm-1", info.InstanceID)
	})
}

func TestDetect_ProviderError(t *testing.T) {
	testDetectors := []providers.Detector{
		&mockDetector{
			name:      "aws",
			provErr:   errors.New("provider error"),
			publicIP:  "1.2.3.4",
			privateIP: "10.0.1.100",
			vmEnv:     "AWS",
		},
		&mockDetector{
			name:      "azure",
			provider:  "azure",
			publicIP:  "5.6.7.8",
			privateIP: "10.0.2.200",
			vmEnv:     "AZURE",
		},
	}

	withTemporaryDetectors(testDetectors, func() {
		info, err := Detect(context.Background())
		assert.NoError(t, err)
		assert.Equal(t, "azure", info.Provider)
		assert.Equal(t, "5.6.7.8", info.PublicIP)
		assert.Equal(t, "10.0.2.200", info.PrivateIP)
		assert.Equal(t, "AZURE", info.VMEnvironment)
	})
}

func TestDetect_NoProviderDetected(t *testing.T) {
	testDetectors := []providers.Detector{
		&mockDetector{
			name:     "aws",
			provider: "", // Empty provider, not error
		},
		&mockDetector{
			name:     "azure",
			provider: "", // Empty provider, not error
		},
	}

	withTemporaryDetectors(testDetectors, func() {
		info, err := Detect(context.Background())
		assert.NoError(t, err)
		assert.Equal(t, "unknown", info.Provider)
	})
}

func TestDetect_PublicIPError(t *testing.T) {
	testDetectors := []providers.Detector{
		&mockDetector{
			name:       "aws",
			provider:   "aws",
			publicErr:  errors.New("public IP error"),
			privateIP:  "10.0.1.100",
			vmEnv:      "AWS",
			instanceID: "i-abc",
		},
	}

	withTemporaryDetectors(testDetectors, func() {
		info, err := Detect(context.Background())
		assert.NoError(t, err)
		assert.Equal(t, "aws", info.Provider)
		assert.Empty(t, info.PublicIP)
		assert.Equal(t, "10.0.1.100", info.PrivateIP)
		assert.Equal(t, "AWS", info.VMEnvironment)
		assert.Equal(t, "i-abc", info.InstanceID)
	})
}

func TestDetect_VMEnvironmentError(t *testing.T) {
	testDetectors := []providers.Detector{
		&mockDetector{
			name:       "aws",
			provider:   "aws",
			publicIP:   "1.2.3.4",
			privateIP:  "10.0.1.100",
			vmEnvErr:   errors.New("VM environment error"),
			instanceID: "i-abc",
		},
	}

	withTemporaryDetectors(testDetectors, func() {
		info, err := Detect(context.Background())
		assert.NoError(t, err)
		assert.Equal(t, "aws", info.Provider)
		assert.Equal(t, "1.2.3.4", info.PublicIP)
		assert.Equal(t, "10.0.1.100", info.PrivateIP)
		assert.Empty(t, info.VMEnvironment)
		assert.Equal(t, "i-abc", info.InstanceID)
	})
}

func TestDetect_PrivateIPError(t *testing.T) {
	testDetectors := []providers.Detector{
		&mockDetector{
			name:       "aws",
			provider:   "aws",
			publicIP:   "1.2.3.4",
			privateErr: errors.New("private IP error"),
			vmEnv:      "AWS",
			instanceID: "i-abc",
		},
	}

	withTemporaryDetectors(testDetectors, func() {
		info, err := Detect(context.Background())
		assert.NoError(t, err)
		assert.Equal(t, "aws", info.Provider)
		assert.Equal(t, "1.2.3.4", info.PublicIP)
		assert.Empty(t, info.PrivateIP)
		assert.Equal(t, "AWS", info.VMEnvironment)
		assert.Equal(t, "i-abc", info.InstanceID)
	})
}

func TestDetect_InstanceIDError(t *testing.T) {
	testDetectors := []providers.Detector{
		&mockDetector{
			name:          "aws",
			provider:      "aws",
			publicIP:      "1.2.3.4",
			privateIP:     "10.0.1.100",
			vmEnv:         "AWS",
			instanceIDErr: errors.New("instance ID error"),
		},
	}

	withTemporaryDetectors(testDetectors, func() {
		info, err := Detect(context.Background())
		assert.NoError(t, err)
		assert.Equal(t, "aws", info.Provider)
		assert.Equal(t, "1.2.3.4", info.PublicIP)
		assert.Equal(t, "10.0.1.100", info.PrivateIP)
		assert.Equal(t, "AWS", info.VMEnvironment)
		assert.Empty(t, info.InstanceID)
	})
}
