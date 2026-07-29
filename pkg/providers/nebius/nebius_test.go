package nebius

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/bytedance/mockey"
	"github.com/stretchr/testify/require"

	"github.com/leptonai/gpud/pkg/providers"
	"github.com/leptonai/gpud/pkg/providers/nebius/imds"
)

func TestNewAndDetectProvider_WithMockey(t *testing.T) {
	mockey.PatchConvey("New returns a region-capable Nebius detector", t, func() {
		mockey.Mock(imds.FetchRegion).To(func(context.Context) (string, error) {
			return "eu-west1", nil
		}).Build()
		mockey.Mock(imds.FetchInstanceData).To(func(context.Context) (*imds.InstanceData, error) {
			return &imds.InstanceData{
				ID:           "computeinstance-inst789",
				ParentID:     "project-test123",
				GPUClusterID: "computegpucluster-gpu456",
			}, nil
		}).Build()

		detector := New()
		require.Equal(t, Name, detector.Name())

		provider, err := detector.Provider(context.Background())
		require.NoError(t, err)
		require.Equal(t, Name, provider)

		regionDetector, ok := detector.(providers.RegionDetector)
		require.True(t, ok)
		region, err := regionDetector.Region(context.Background())
		require.NoError(t, err)
		require.Equal(t, "eu-west1", region)

		instanceID, err := detector.InstanceID(context.Background())
		require.NoError(t, err)
		require.Equal(t, "project-test123/computegpucluster-gpu456/computeinstance-inst789", instanceID)
	})
}

func TestGetInstanceID(t *testing.T) {
	// Save original metadataPath
	originalMetadataPath := metadataPath
	defer func() {
		metadataPath = originalMetadataPath
	}()

	tests := []struct {
		name           string
		setupFiles     map[string]string
		expectedResult string
		expectError    bool
	}{
		{
			name: "success with gpu cluster",
			setupFiles: map[string]string{
				"parent-id":      "project-test123",
				"gpu-cluster-id": "computegpucluster-gpu456",
				"instance-id":    "computeinstance-inst789",
			},
			expectedResult: "project-test123/computegpucluster-gpu456/computeinstance-inst789",
			expectError:    false,
		},
		{
			name: "success without gpu cluster",
			setupFiles: map[string]string{
				"parent-id":   "project-test456",
				"instance-id": "computeinstance-inst123",
			},
			expectedResult: "project-test456/computeinstance-inst123",
			expectError:    false,
		},
		{
			name: "success with empty gpu cluster id",
			setupFiles: map[string]string{
				"parent-id":      "project-test789",
				"gpu-cluster-id": "",
				"instance-id":    "computeinstance-inst456",
			},
			expectedResult: "project-test789/computeinstance-inst456",
			expectError:    false,
		},
		{
			name: "error: missing parent-id file",
			setupFiles: map[string]string{
				"instance-id": "computeinstance-inst789",
			},
			expectError: true,
		},
		{
			name: "error: missing instance-id file",
			setupFiles: map[string]string{
				"parent-id": "project-test123",
			},
			expectError: true,
		},
		{
			name:        "error: missing all files",
			setupFiles:  map[string]string{},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary directory using t.TempDir()
			tempDir := t.TempDir()

			// Set metadataPath to temp directory
			metadataPath = tempDir

			// Create test files
			for filename, content := range tt.setupFiles {
				filePath := filepath.Join(tempDir, filename)
				if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
					t.Fatalf("Failed to create test file %s: %v", filename, err)
				}
			}

			// Run the test
			result, err := GetInstanceID()

			// Check results
			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				if result != tt.expectedResult {
					t.Errorf("Expected result %q, got %q", tt.expectedResult, result)
				}
			}
		})
	}
}
