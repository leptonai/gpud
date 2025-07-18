package update

import (
	"net/url"
	"os"
	"testing"
)

// TestUpdate tests the Update function signature
func TestUpdate(t *testing.T) {
	// Skip if running as root to avoid actual updates
	if os.Geteuid() == 0 {
		t.Skip("Skipping test when running as root")
	}

	// Should fail with non-root error
	err := UpdateExecutable("0.0.1", DefaultUpdateURL, true)
	if err == nil {
		t.Error("Expected error when running Update as non-root")
	}
}

// TestUpdateOnlyBinary tests the UpdateOnlyBinary function signature
func TestUpdateOnlyBinary(t *testing.T) {
	// This should fail with download error
	err := UpdateExecutable("0.0.1", DefaultUpdateURL, false)
	if err == nil {
		t.Error("Expected error when running UpdateOnlyBinary")
	}
}

// TestPackageUpdate tests the PackageUpdate function
func TestPackageUpdate(t *testing.T) {
	// Create a temp directory for testing
	tempDir, err := os.MkdirTemp(t.TempDir(), "gpud-package-update-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Test error path for non-existent directory
	err = packageUpdate("/non-existent/dir", "test-pkg", "0.0.1", DefaultUpdateURL)
	if err == nil {
		t.Error("Expected error for non-existent directory")
	}

	// Test with valid temp directory
	err = packageUpdate(tempDir, "test-pkg", "0.0.1", DefaultUpdateURL)
	// This will fail due to download failing, but we're testing the logic flow
	if err == nil {
		t.Error("Expected error for failed download")
	}

	// Test with package that would need root to install
	if os.Geteuid() != 0 {
		err = PackageUpdate("test-pkg", "0.0.1", DefaultUpdateURL)
		if err == nil {
			t.Error("Expected error when running PackageUpdate as non-root")
		}
	}
}

func TestURLJoin(t *testing.T) {
	tests := []struct {
		baseURL string
		paths   []string
		want    string
		wantErr bool
	}{
		{
			baseURL: "https://example.com",
			paths:   []string{"packages", "test-pkg", "v1.0"},
			want:    "https://example.com/packages/test-pkg/v1.0",
			wantErr: false,
		},
		{
			baseURL: "https://example.com/",
			paths:   []string{"packages", "test-pkg", "v1.0"},
			want:    "https://example.com/packages/test-pkg/v1.0",
			wantErr: false,
		},
		{
			baseURL: "",
			paths:   []string{"packages", "test-pkg", "v1.0"},
			want:    "packages/test-pkg/v1.0", // url.JoinPath returns a relative URL for empty base
			wantErr: false,                    // url.JoinPath doesn't actually return an error for empty base URL
		},
	}

	for _, tc := range tests {
		got, err := url.JoinPath(tc.baseURL, tc.paths...)

		if (err != nil) != tc.wantErr {
			t.Errorf("url.JoinPath(%q, %v) error = %v, wantErr %v",
				tc.baseURL, tc.paths, err, tc.wantErr)
		}

		if err == nil && got != tc.want {
			t.Errorf("url.JoinPath(%q, %v) = %v, want %v",
				tc.baseURL, tc.paths, got, tc.want)
		}
	}
}
