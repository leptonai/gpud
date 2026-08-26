package all

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/leptonai/gpud/pkg/providers"
	"github.com/leptonai/gpud/pkg/providers/nebius"
	"github.com/leptonai/gpud/pkg/providers/oci"
)

func TestAllIncludesRequiredProviders(t *testing.T) {
	for _, name := range []string{nebius.Name, oci.Name} {
		t.Run(name, func(t *testing.T) {
			for _, detector := range All {
				if detector.Name() == name {
					return
				}
			}
			t.Fatalf("provider registry does not include %q", name)
		})
	}
}

func TestAllProvidersSupportIMDS(t *testing.T) {
	for _, detector := range All {
		assert.True(t, providers.SupportsIMDS(detector), detector.Name())
	}
}

// mockDetector implements the providers.Detector interface for testing
type mockDetector struct {
	name          string
	provider      string
	publicIP      string
	privateIP     string
	region        string
	provErr       error
	publicErr     error
	privateErr    error
	regionErr     error
	vmEnv         string
	vmEnvErr      error
	instanceID    string
	instanceIDErr error
	delay         time.Duration
}

type legacyDetector struct {
	name       string
	provider   string
	publicIP   string
	privateIP  string
	vmEnv      string
	instanceID string
}

func (m *mockDetector) Name() string {
	return m.name
}

func (m *legacyDetector) Name() string {
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

func (m *legacyDetector) Provider(ctx context.Context) (string, error) {
	return m.provider, nil
}

func (m *mockDetector) PublicIPv4(ctx context.Context) (string, error) {
	return m.publicIP, m.publicErr
}

func (m *legacyDetector) PublicIPv4(ctx context.Context) (string, error) {
	return m.publicIP, nil
}

func (m *mockDetector) PrivateIPv4(ctx context.Context) (string, error) {
	return m.privateIP, m.privateErr
}

func (m *legacyDetector) PrivateIPv4(ctx context.Context) (string, error) {
	return m.privateIP, nil
}

func (m *mockDetector) Region(ctx context.Context) (string, error) {
	return m.region, m.regionErr
}

func (m *mockDetector) VMEnvironment(ctx context.Context) (string, error) {
	return m.vmEnv, m.vmEnvErr
}

func (m *legacyDetector) VMEnvironment(ctx context.Context) (string, error) {
	return m.vmEnv, nil
}

func (m *mockDetector) InstanceID(ctx context.Context) (string, error) {
	return m.instanceID, m.instanceIDErr
}

func (m *legacyDetector) InstanceID(ctx context.Context) (string, error) {
	return m.instanceID, nil
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
			region:     "us-east-1",
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
		assert.Equal(t, "us-east-1", info.Region)
		assert.Equal(t, "AWS", info.VMEnvironment)
		assert.Equal(t, "i-abc", info.InstanceID)
	})
}

func TestDetect_SuccessWithoutRegionDetector(t *testing.T) {
	testDetectors := []providers.Detector{
		&legacyDetector{
			name:       "legacy",
			provider:   "legacy",
			publicIP:   "1.2.3.4",
			privateIP:  "10.0.1.100",
			vmEnv:      "LEGACY",
			instanceID: "i-legacy",
		},
	}

	withTemporaryDetectors(testDetectors, func() {
		info, err := Detect(context.Background())
		assert.NoError(t, err)
		assert.Equal(t, "legacy", info.Provider)
		assert.Equal(t, "1.2.3.4", info.PublicIP)
		assert.Equal(t, "10.0.1.100", info.PrivateIP)
		assert.Empty(t, info.Region)
		assert.Equal(t, "LEGACY", info.VMEnvironment)
		assert.Equal(t, "i-legacy", info.InstanceID)
	})
}

func TestDetect_RegionError(t *testing.T) {
	testDetectors := []providers.Detector{
		&mockDetector{
			name:       "aws",
			provider:   "aws",
			publicIP:   "1.2.3.4",
			privateIP:  "10.0.1.100",
			regionErr:  errors.New("region error"),
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
		assert.Empty(t, info.Region)
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

func TestDetect_RegionOverrideSkipsFallbackForUnknownProvider(t *testing.T) {
	withTemporaryDetectors(nil, func() {
		info, err := DetectWithRegionOverride(context.Background(), "eu-north-1")
		assert.NoError(t, err)
		assert.Equal(t, "unknown", info.Provider)
		assert.Equal(t, "eu-north-1", info.Region)
		assert.False(t, info.IMDSDetected)
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

func TestDetect_IMDSRetriesRequiredMetadata(t *testing.T) {
	originalBackoffs := imdsRetryBackoffs
	imdsRetryBackoffs = []time.Duration{0, 0, 0, 0}
	defer func() { imdsRetryBackoffs = originalBackoffs }()

	regionCalls := 0
	instanceIDCalls := 0
	detector := providers.NewIMDSWithRegion(
		"test-cloud",
		func(context.Context) (string, error) { return "detected", nil },
		nil,
		nil,
		func(context.Context) (string, error) {
			regionCalls++
			if regionCalls < 3 {
				return "", nil
			}
			return "eu-west-2", nil
		},
		nil,
		func(context.Context) (string, error) {
			instanceIDCalls++
			if instanceIDCalls < 2 {
				return "", errors.New("metadata unavailable")
			}
			return "instance-1", nil
		},
	)

	withTemporaryDetectors([]providers.Detector{detector}, func() {
		info, err := Detect(context.Background())
		assert.NoError(t, err)
		assert.True(t, info.IMDSDetected)
		assert.Equal(t, "eu-west-2", info.Region)
		assert.Equal(t, "instance-1", info.InstanceID)
		assert.Equal(t, 3, regionCalls)
		assert.Equal(t, 2, instanceIDCalls)
	})
}

func TestDetect_IMDSRetryExhaustion(t *testing.T) {
	originalBackoffs := imdsRetryBackoffs
	imdsRetryBackoffs = []time.Duration{0, 0, 0, 0}
	defer func() { imdsRetryBackoffs = originalBackoffs }()

	instanceIDCalls := 0
	detector := providers.NewIMDSWithRegion(
		"test-cloud",
		func(context.Context) (string, error) { return "detected", nil },
		nil,
		nil,
		func(context.Context) (string, error) { return "eu-west-2", nil },
		nil,
		func(context.Context) (string, error) {
			instanceIDCalls++
			return " ", nil
		},
	)

	withTemporaryDetectors([]providers.Detector{detector}, func() {
		info, err := Detect(context.Background())
		assert.NoError(t, err)
		assert.True(t, info.IMDSDetected)
		assert.Empty(t, info.InstanceID)
		assert.Equal(t, 4, instanceIDCalls)
	})
}

func TestDetect_RegionOverrideSkipsIMDSAndKeepsInstanceID(t *testing.T) {
	originalBackoffs := imdsRetryBackoffs
	imdsRetryBackoffs = []time.Duration{0, 0, 0, 0}
	defer func() { imdsRetryBackoffs = originalBackoffs }()

	regionCalls := 0
	instanceIDCalls := 0
	detector := providers.NewIMDSWithRegion(
		"test-cloud",
		func(context.Context) (string, error) { return "detected", nil },
		nil,
		nil,
		func(context.Context) (string, error) {
			regionCalls++
			return "metadata-region", nil
		},
		nil,
		func(context.Context) (string, error) {
			instanceIDCalls++
			return "instance-1", nil
		},
	)

	withTemporaryDetectors([]providers.Detector{detector}, func() {
		info, err := DetectWithRegionOverride(context.Background(), " eu-north-1 ")
		assert.NoError(t, err)
		assert.Equal(t, "eu-north-1", info.Region)
		assert.Equal(t, "instance-1", info.InstanceID)
		assert.Zero(t, regionCalls)
		assert.Equal(t, 1, instanceIDCalls)
	})
}

func TestDetect_IMDSRetryStopsOnContextCancellation(t *testing.T) {
	originalBackoffs := imdsRetryBackoffs
	imdsRetryBackoffs = []time.Duration{0, time.Hour}
	defer func() { imdsRetryBackoffs = originalBackoffs }()

	ctx, cancel := context.WithCancel(context.Background())
	instanceIDCalls := 0
	detector := providers.NewIMDSWithRegion(
		"test-cloud",
		func(context.Context) (string, error) { return "detected", nil },
		nil,
		nil,
		func(context.Context) (string, error) { return "eu-west-2", nil },
		nil,
		func(context.Context) (string, error) {
			instanceIDCalls++
			cancel()
			return "", errors.New("metadata unavailable")
		},
	)

	withTemporaryDetectors([]providers.Detector{detector}, func() {
		info, err := Detect(ctx)
		assert.NoError(t, err)
		assert.Empty(t, info.InstanceID)
		assert.Equal(t, 1, instanceIDCalls)
	})
}
