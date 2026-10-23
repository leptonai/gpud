package all

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/leptonai/gpud/pkg/providers"
	awsimds "github.com/leptonai/gpud/pkg/providers/aws/imds"
	azureimds "github.com/leptonai/gpud/pkg/providers/azure/imds"
	gcpimds "github.com/leptonai/gpud/pkg/providers/gcp/imds"
	"github.com/leptonai/gpud/pkg/providers/nebius"
	nscaleimds "github.com/leptonai/gpud/pkg/providers/nscale/imds"
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
		info, err := DetectWithOverrides(context.Background(), "eu-north-1", "")
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

func TestDetect_NscaleEmptyRegionDoesNotRetryFourTimes(t *testing.T) {
	originalBackoffs := imdsRetryBackoffs
	imdsRetryBackoffs = []time.Duration{0, time.Hour}
	defer func() { imdsRetryBackoffs = originalBackoffs }()

	regionCalls := 0
	detector := providers.NewIMDSWithRegion(
		"nscale",
		func(context.Context) (string, error) { return "nscale", nil },
		nil,
		nil,
		func(context.Context) (string, error) {
			regionCalls++
			return "", nil
		},
		nil,
		func(context.Context) (string, error) { return "i-00003b7a", nil },
	)

	withTemporaryDetectors([]providers.Detector{detector}, func() {
		info, err := Detect(context.Background())
		assert.NoError(t, err)
		assert.True(t, info.IMDSDetected)
		assert.Equal(t, "i-00003b7a", info.InstanceID)
		assert.Empty(t, info.Region)
		assert.Equal(t, 1, regionCalls)
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
		info, err := DetectWithOverrides(context.Background(), " eu-north-1 ", "")
		assert.NoError(t, err)
		assert.Equal(t, "eu-north-1", info.Region)
		assert.Equal(t, "instance-1", info.InstanceID)
		assert.Zero(t, regionCalls)
		assert.Equal(t, 1, instanceIDCalls)
	})
}

func TestDetect_InstanceIDOverride(t *testing.T) {
	originalBackoffs := imdsRetryBackoffs
	imdsRetryBackoffs = []time.Duration{0, 0, 0, 0}
	defer func() { imdsRetryBackoffs = originalBackoffs }()

	tests := []struct {
		name            string
		override        string
		instanceIDErr   error
		wantInstanceID  string
		wantInstanceIDs int
	}{
		{
			name:           "override wins over discovered ID",
			override:       "  explicit/ID  ",
			wantInstanceID: "  explicit/ID  ",
		},
		{
			name:           "override bypasses failing metadata",
			override:       "explicit-ID",
			instanceIDErr:  errors.New("instance ID unavailable"),
			wantInstanceID: "explicit-ID",
		},
		{
			name:            "empty override discovers ID",
			wantInstanceID:  "metadata-ID",
			wantInstanceIDs: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			providerCalls, instanceIDCalls, regionCalls := 0, 0, 0
			publicIPCalls, privateIPCalls, vmEnvironmentCalls := 0, 0, 0
			detector := providers.NewIMDSWithRegion(
				"test-cloud",
				func(context.Context) (string, error) {
					providerCalls++
					return "detected", nil
				},
				func(context.Context) (string, error) {
					publicIPCalls++
					return "1.2.3.4", nil
				},
				func(context.Context) (string, error) {
					privateIPCalls++
					return "10.0.1.100", nil
				},
				func(context.Context) (string, error) {
					regionCalls++
					return "metadata-region", nil
				},
				func(context.Context) (string, error) {
					vmEnvironmentCalls++
					return "TEST-CLOUD", nil
				},
				func(context.Context) (string, error) {
					instanceIDCalls++
					return "metadata-ID", tt.instanceIDErr
				},
			)

			withTemporaryDetectors([]providers.Detector{detector}, func() {
				info, err := DetectWithOverrides(context.Background(), "", tt.override)
				assert.NoError(t, err)
				assert.Equal(t, "test-cloud", info.Provider)
				assert.True(t, info.IMDSDetected)
				assert.Equal(t, tt.wantInstanceID, info.InstanceID)
				assert.Equal(t, "metadata-region", info.Region)
				assert.Equal(t, "1.2.3.4", info.PublicIP)
				assert.Equal(t, "10.0.1.100", info.PrivateIP)
				assert.Equal(t, "TEST-CLOUD", info.VMEnvironment)
				assert.Equal(t, tt.wantInstanceIDs, instanceIDCalls)
				assert.Equal(t, 1, providerCalls)
				assert.Equal(t, 1, regionCalls)
				assert.Equal(t, 1, publicIPCalls)
				assert.Equal(t, 1, privateIPCalls)
				assert.Equal(t, 1, vmEnvironmentCalls)
			})
		})
	}
}

func TestDetect_InstanceIDOverrideForUnknownProvider(t *testing.T) {
	for _, instanceID := range []string{"", "  explicit/ID  "} {
		t.Run(instanceID, func(t *testing.T) {
			withTemporaryDetectors(nil, func() {
				info, err := DetectWithOverrides(context.Background(), " eu-north-1 ", instanceID)
				assert.NoError(t, err)
				assert.Equal(t, "unknown", info.Provider)
				assert.Equal(t, instanceID, info.InstanceID)
				assert.Equal(t, "eu-north-1", info.Region)
				assert.False(t, info.IMDSDetected)
			})
		})
	}
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

func TestDetect_IMDSPrivateIPv4SingleAttemptWithoutOptIn(t *testing.T) {
	originalBackoffs := imdsRetryBackoffs
	imdsRetryBackoffs = []time.Duration{0, time.Hour}
	defer func() { imdsRetryBackoffs = originalBackoffs }()

	// The detector has a private IPv4 fetcher but did not opt into retries:
	// a second attempt would block for an hour, so completing proves the
	// fetch happened exactly once.
	privateCalls := 0
	detector := providers.NewIMDSWithRegion(
		"test-cloud",
		func(context.Context) (string, error) { return "detected", nil },
		nil,
		func(context.Context) (string, error) {
			privateCalls++
			return "", errors.New("metadata unavailable")
		},
		func(context.Context) (string, error) { return "eu-west-2", nil },
		nil,
		func(context.Context) (string, error) { return "instance-1", nil },
	)

	withTemporaryDetectors([]providers.Detector{detector}, func() {
		info, err := Detect(context.Background())
		assert.NoError(t, err)
		assert.Empty(t, info.PrivateIP)
		assert.Equal(t, 1, privateCalls)
	})
}

func TestDetect_IMDSRetriesPrivateIPv4(t *testing.T) {
	originalBackoffs := imdsRetryBackoffs
	imdsRetryBackoffs = []time.Duration{0, 0, 0, 0}
	defer func() { imdsRetryBackoffs = originalBackoffs }()

	privateCalls := 0
	detector := providers.NewIMDSWithRegion(
		"test-cloud",
		func(context.Context) (string, error) { return "detected", nil },
		nil,
		func(context.Context) (string, error) {
			privateCalls++
			if privateCalls < 3 {
				return "", errors.New("metadata unavailable")
			}
			return "7.247.35.250", nil
		},
		func(context.Context) (string, error) { return "eu-west-2", nil },
		nil,
		func(context.Context) (string, error) { return "instance-1", nil },
		providers.WithPrivateIPv4Retry(),
	)

	withTemporaryDetectors([]providers.Detector{detector}, func() {
		info, err := Detect(context.Background())
		assert.NoError(t, err)
		assert.True(t, info.IMDSDetected)
		assert.Equal(t, "7.247.35.250", info.PrivateIP)
		assert.Equal(t, 3, privateCalls)
	})
}

func TestDetect_PrivateIPv4RetryDoesNotStarveRequiredFields(t *testing.T) {
	// The private IPv4 fetch hangs until the shared context expires (a
	// metadata service that accepts no connections). The required identity
	// fields must still be populated because they are fetched first — their
	// mocks fail on a canceled context, so a wrong fetch order would leave
	// them empty and fail the test.
	detector := providers.NewIMDSWithRegion(
		"test-cloud",
		func(context.Context) (string, error) { return "detected", nil },
		nil,
		func(ctx context.Context) (string, error) {
			<-ctx.Done()
			return "", ctx.Err()
		},
		func(ctx context.Context) (string, error) {
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			default:
			}
			return "eu-west-2", nil
		},
		nil,
		func(ctx context.Context) (string, error) {
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			default:
			}
			return "instance-1", nil
		},
		providers.WithPrivateIPv4Retry(),
	)

	withTemporaryDetectors([]providers.Detector{detector}, func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		info, err := Detect(ctx)
		assert.NoError(t, err)
		assert.Equal(t, "instance-1", info.InstanceID)
		assert.Equal(t, "eu-west-2", info.Region)
		assert.Empty(t, info.PrivateIP)
	})
}

func TestDetect_IMDSPrivateIPv4RetryExhaustion(t *testing.T) {
	originalBackoffs := imdsRetryBackoffs
	imdsRetryBackoffs = []time.Duration{0, 0, 0, 0}
	defer func() { imdsRetryBackoffs = originalBackoffs }()

	privateCalls := 0
	detector := providers.NewIMDSWithRegion(
		"test-cloud",
		func(context.Context) (string, error) { return "detected", nil },
		nil,
		func(context.Context) (string, error) {
			privateCalls++
			return "", nil
		},
		func(context.Context) (string, error) { return "eu-west-2", nil },
		nil,
		func(context.Context) (string, error) { return "instance-1", nil },
		providers.WithPrivateIPv4Retry(),
	)

	withTemporaryDetectors([]providers.Detector{detector}, func() {
		info, err := Detect(context.Background())
		assert.NoError(t, err)
		assert.Empty(t, info.PrivateIP)
		assert.Equal(t, 4, privateCalls)
	})
}

func TestDetect_PublicIPv4HTTPOutcomes(t *testing.T) {
	cases := []struct {
		name      string
		fetch     func(context.Context) (string, error)
		path      string
		body      string
		statuses  []int
		wantIP    string
		wantCalls int32
	}{
		{name: "AWS recovers from 503", fetch: awsimds.FetchPublicIPv4, path: "/latest/meta-data/public-ipv4", body: "203.0.113.10", statuses: []int{503, 200}, wantIP: "203.0.113.10", wantCalls: 2},
		{name: "GCP recovers from 503", fetch: gcpimds.FetchPublicIPv4, path: "/computeMetadata/v1/instance/network-interfaces/", body: `[{"accessConfigs":[{"externalIp":"203.0.113.10"}]}]`, statuses: []int{503, 200}, wantIP: "203.0.113.10", wantCalls: 2},
		{name: "Azure recovers from 503", fetch: azureimds.FetchPublicIPv4, path: "/metadata/instance/network/interface", body: `[{"ipv4":{"ipAddress":[{"publicIpAddress":"203.0.113.10"}]}}]`, statuses: []int{503, 200}, wantIP: "203.0.113.10", wantCalls: 2},
		{name: "NScale recovers from 503", fetch: nscaleimds.FetchPublicIPv4, path: "/latest/meta-data/public-ipv4", body: "203.0.113.10", statuses: []int{503, 200}, wantIP: "203.0.113.10", wantCalls: 2},
		{name: "AWS 404 is terminal", fetch: awsimds.FetchPublicIPv4, path: "/latest/meta-data/public-ipv4", statuses: []int{404}, wantCalls: 1},
		{name: "NScale 404 is terminal", fetch: nscaleimds.FetchPublicIPv4, path: "/latest/meta-data/public-ipv4", statuses: []int{404}, wantCalls: 1},
		{name: "AWS empty answer is terminal", fetch: awsimds.FetchPublicIPv4, path: "/latest/meta-data/public-ipv4", statuses: []int{200}, wantCalls: 1},
		{name: "GCP no access configs is terminal", fetch: gcpimds.FetchPublicIPv4, path: "/computeMetadata/v1/instance/network-interfaces/", body: `[{"accessConfigs":[]}]`, statuses: []int{200}, wantCalls: 1},
		{name: "Azure empty instance-level public address is terminal", fetch: azureimds.FetchPublicIPv4, path: "/metadata/instance/network/interface", body: `[{"ipv4":{"ipAddress":[{"privateIpAddress":"10.0.0.4","publicIpAddress":""}]}}]`, statuses: []int{200}, wantCalls: 1},
		{name: "persistent 503 leaves public address unknown", fetch: awsimds.FetchPublicIPv4, path: "/latest/meta-data/public-ipv4", statuses: []int{503}, wantCalls: 4},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/latest/api/token" {
					_, _ = fmt.Fprint(w, "test-token")
					return
				}
				if r.URL.Path != tc.path {
					http.NotFound(w, r)
					return
				}
				index := min(int(calls.Add(1))-1, len(tc.statuses)-1)
				w.WriteHeader(tc.statuses[index])
				_, _ = fmt.Fprint(w, tc.body)
			}))
			defer server.Close()
			proxyURL, err := url.Parse(server.URL)
			require.NoError(t, err)
			transport := &http.Transport{Proxy: http.ProxyURL(proxyURL)}
			originalTransport := http.DefaultTransport
			http.DefaultTransport = transport
			defer func() {
				http.DefaultTransport = originalTransport
				transport.CloseIdleConnections()
			}()
			originalBackoffs := imdsRetryBackoffs
			imdsRetryBackoffs = []time.Duration{0, 0, 0, 0}
			defer func() { imdsRetryBackoffs = originalBackoffs }()
			detector := providers.NewIMDSWithRegion(
				"test-cloud", func(context.Context) (string, error) { return "test-cloud", nil },
				tc.fetch, nil,
				func(context.Context) (string, error) { return "region-1", nil }, nil,
				func(context.Context) (string, error) { return "instance-1", nil },
			)
			withTemporaryDetectors([]providers.Detector{detector}, func() {
				info, err := Detect(context.Background())
				require.NoError(t, err)
				assert.Equal(t, tc.wantIP, info.PublicIP)
				assert.Equal(t, tc.wantCalls, calls.Load())
			})
		})
	}
}

func TestDetect_OCIInstanceIDOverrideSkipsIMDSInstanceID(t *testing.T) {
	tests := []struct {
		name              string
		instanceID        string
		region            string
		wantInstanceID    string
		wantRegion        string
		wantInstanceCalls int32
		wantRegionCalls   int32
	}{
		{name: "empty override keeps instance discovery", wantInstanceID: "ocid1.instance.oc1.phx.example", wantRegion: "us-phoenix-1", wantInstanceCalls: 2, wantRegionCalls: 1},
		{name: "override bypasses failing instance endpoint", instanceID: "  explicit/ID  ", wantInstanceID: "  explicit/ID  ", wantRegion: "us-phoenix-1", wantRegionCalls: 2},
		{name: "both overrides retain OCI provider detection", instanceID: "explicit-ID", region: " eu-north-1 ", wantInstanceID: "explicit-ID", wantRegion: "eu-north-1", wantRegionCalls: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var instanceCalls, regionCalls, privateCalls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "Bearer Oracle", r.Header.Get("Authorization"))
				switch r.URL.Path {
				case "/opc/v2/instance/id":
					instanceCalls.Add(1)
					if tt.instanceID != "" {
						http.Error(w, "instance metadata unavailable", http.StatusServiceUnavailable)
						return
					}
					_, _ = fmt.Fprint(w, "ocid1.instance.oc1.phx.example")
				case "/opc/v2/instance/canonicalRegionName":
					regionCalls.Add(1)
					_, _ = fmt.Fprint(w, "us-phoenix-1")
				case "/opc/v2/vnics/0/privateIp":
					privateCalls.Add(1)
					_, _ = fmt.Fprint(w, "203.0.113.10")
				default:
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			proxyURL, err := url.Parse(server.URL)
			require.NoError(t, err)
			transport := &http.Transport{Proxy: http.ProxyURL(proxyURL)}
			originalTransport := http.DefaultTransport
			http.DefaultTransport = transport
			defer func() {
				http.DefaultTransport = originalTransport
				transport.CloseIdleConnections()
			}()
			originalBackoffs := imdsRetryBackoffs
			imdsRetryBackoffs = []time.Duration{0, 0, 0, 0}
			defer func() { imdsRetryBackoffs = originalBackoffs }()

			withTemporaryDetectors([]providers.Detector{ociDetector}, func() {
				info, err := DetectWithOverrides(context.Background(), tt.region, tt.instanceID)
				require.NoError(t, err)
				assert.Equal(t, oci.Name, info.Provider)
				assert.True(t, info.IMDSDetected)
				assert.Equal(t, tt.wantInstanceID, info.InstanceID)
				assert.Equal(t, tt.wantRegion, info.Region)
				assert.Equal(t, "203.0.113.10", info.PrivateIP)
				assert.Equal(t, tt.wantInstanceCalls, instanceCalls.Load())
				assert.Equal(t, tt.wantRegionCalls, regionCalls.Load())
				assert.Equal(t, int32(1), privateCalls.Load())
			})
		})
	}
}

func TestDetect_InstanceIDOverrideKeepsCustomOCIDetector(t *testing.T) {
	detector := &mockDetector{
		name:          oci.Name,
		provider:      oci.Name,
		region:        "custom-region",
		privateIP:     "10.0.1.100",
		instanceIDErr: errors.New("instance ID unavailable"),
	}
	withTemporaryDetectors([]providers.Detector{detector}, func() {
		info, err := DetectWithOverrides(context.Background(), "", "explicit-ID")
		require.NoError(t, err)
		assert.Equal(t, oci.Name, info.Provider)
		assert.Equal(t, "explicit-ID", info.InstanceID)
		assert.Equal(t, "custom-region", info.Region)
		assert.Equal(t, "10.0.1.100", info.PrivateIP)
	})
}

func TestDetect_PublicIPv4TimeoutRecovery(t *testing.T) {
	originalBackoffs := imdsRetryBackoffs
	imdsRetryBackoffs = []time.Duration{0, 0, 0, 0}
	defer func() { imdsRetryBackoffs = originalBackoffs }()
	calls := 0
	detector := providers.NewIMDSWithRegion(
		"test-cloud", func(context.Context) (string, error) { return "test-cloud", nil },
		func(context.Context) (string, error) {
			calls++
			if calls == 1 {
				return "", context.DeadlineExceeded
			}
			return "203.0.113.10", nil
		}, nil, nil, nil, nil,
	)
	withTemporaryDetectors([]providers.Detector{detector}, func() {
		info, err := Detect(context.Background())
		require.NoError(t, err)
		assert.Equal(t, "203.0.113.10", info.PublicIP)
		assert.Equal(t, 2, calls)
	})
}

func TestDetect_PublicIPv4CancellationPreservesIdentity(t *testing.T) {
	originalBackoffs := imdsRetryBackoffs
	imdsRetryBackoffs = []time.Duration{0, time.Hour}
	defer func() { imdsRetryBackoffs = originalBackoffs }()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	calls := 0
	detector := providers.NewIMDSWithRegion(
		"test-cloud", func(context.Context) (string, error) { return "test-cloud", nil },
		func(context.Context) (string, error) {
			calls++
			cancel()
			return "", context.DeadlineExceeded
		}, nil,
		func(ctx context.Context) (string, error) { return "region-1", ctx.Err() }, nil,
		func(ctx context.Context) (string, error) { return "instance-1", ctx.Err() },
	)
	withTemporaryDetectors([]providers.Detector{detector}, func() {
		info, err := Detect(ctx)
		require.NoError(t, err)
		assert.Empty(t, info.PublicIP)
		assert.Equal(t, "instance-1", info.InstanceID)
		assert.Equal(t, "region-1", info.Region)
		assert.Equal(t, 1, calls)
	})
}
