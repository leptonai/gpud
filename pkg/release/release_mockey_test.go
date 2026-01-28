package release

import (
	"errors"
	"testing"

	"github.com/bytedance/mockey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/leptonai/gpud/pkg/release/distsign"
	"github.com/leptonai/gpud/version"
)

// TestGetLatestVersion_Success tests successful GetLatestVersion with stable version.
func TestGetLatestVersion_Success(t *testing.T) {
	mockey.PatchConvey("get latest version success", t, func() {
		originalVersion := version.Version
		defer func() {
			version.Version = originalVersion
		}()
		version.Version = "v0.1.0" // Odd minor = stable

		mockey.Mock(distsign.Fetch).To(func(url string, limit int64) ([]byte, error) {
			// Verify the URL is constructed correctly
			if url != "https://pkg.gpud.dev/stable_latest.txt" {
				return nil, errors.New("unexpected URL: " + url)
			}
			return []byte("v0.1.5\n"), nil
		}).Build()

		ver, err := GetLatestVersion()
		require.NoError(t, err)
		assert.Equal(t, "v0.1.5", ver)
	})
}

// TestGetLatestVersion_UnstableVersion tests GetLatestVersion with unstable version.
func TestGetLatestVersion_UnstableVersion(t *testing.T) {
	mockey.PatchConvey("get latest version unstable", t, func() {
		originalVersion := version.Version
		defer func() {
			version.Version = originalVersion
		}()
		version.Version = "v0.0.1" // Even minor = unstable

		mockey.Mock(distsign.Fetch).To(func(url string, limit int64) ([]byte, error) {
			// Verify the URL is constructed correctly for unstable
			if url != "https://pkg.gpud.dev/unstable_latest.txt" {
				return nil, errors.New("unexpected URL: " + url)
			}
			return []byte("v0.0.5\n"), nil
		}).Build()

		ver, err := GetLatestVersion()
		require.NoError(t, err)
		assert.Equal(t, "v0.0.5", ver)
	})
}

// TestGetLatestVersion_SanitizeVersionError tests GetLatestVersion when version parsing fails.
func TestGetLatestVersion_SanitizeVersionError(t *testing.T) {
	mockey.PatchConvey("sanitize version error", t, func() {
		originalVersion := version.Version
		defer func() {
			version.Version = originalVersion
		}()
		version.Version = "invalid-version" // Malformed version

		ver, err := GetLatestVersion()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "malformed version")
		assert.Equal(t, "", ver)
	})
}

// TestGetLatestVersion_FetchError tests GetLatestVersion when fetch fails.
func TestGetLatestVersion_FetchError(t *testing.T) {
	mockey.PatchConvey("fetch error", t, func() {
		originalVersion := version.Version
		defer func() {
			version.Version = originalVersion
		}()
		version.Version = "v0.1.0"

		mockey.Mock(distsign.Fetch).To(func(url string, limit int64) ([]byte, error) {
			return nil, errors.New("network error")
		}).Build()

		ver, err := GetLatestVersion()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "network error")
		assert.Equal(t, "", ver)
	})
}

// TestGetLatestVersionByURL_Success tests successful GetLatestVersionByURL.
func TestGetLatestVersionByURL_Success(t *testing.T) {
	mockey.PatchConvey("get latest version by url success", t, func() {
		mockey.Mock(distsign.Fetch).To(func(url string, limit int64) ([]byte, error) {
			return []byte("v1.2.3\n"), nil
		}).Build()

		ver, err := GetLatestVersionByURL("https://example.com/version.txt")
		require.NoError(t, err)
		assert.Equal(t, "v1.2.3", ver)
	})
}

// TestGetLatestVersionByURL_WithWhitespace tests GetLatestVersionByURL with whitespace.
func TestGetLatestVersionByURL_WithWhitespace(t *testing.T) {
	mockey.PatchConvey("get latest version by url with whitespace", t, func() {
		mockey.Mock(distsign.Fetch).To(func(url string, limit int64) ([]byte, error) {
			return []byte("  v1.2.3  \n\t"), nil
		}).Build()

		ver, err := GetLatestVersionByURL("https://example.com/version.txt")
		require.NoError(t, err)
		assert.Equal(t, "v1.2.3", ver)
	})
}

// TestGetLatestVersionByURL_FetchError tests GetLatestVersionByURL when fetch fails.
func TestGetLatestVersionByURL_FetchError(t *testing.T) {
	mockey.PatchConvey("get latest version by url fetch error", t, func() {
		mockey.Mock(distsign.Fetch).To(func(url string, limit int64) ([]byte, error) {
			return nil, errors.New("connection timeout")
		}).Build()

		ver, err := GetLatestVersionByURL("https://example.com/version.txt")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "connection timeout")
		assert.Equal(t, "", ver)
	})
}

// TestGetLatestVersionByURL_EmptyResponse tests GetLatestVersionByURL with empty response.
func TestGetLatestVersionByURL_EmptyResponse(t *testing.T) {
	mockey.PatchConvey("get latest version by url empty response", t, func() {
		mockey.Mock(distsign.Fetch).To(func(url string, limit int64) ([]byte, error) {
			return []byte(""), nil
		}).Build()

		ver, err := GetLatestVersionByURL("https://example.com/version.txt")
		require.NoError(t, err)
		assert.Equal(t, "", ver)
	})
}

// TestGetLatestVersionByURL_WhitespaceOnlyResponse tests GetLatestVersionByURL with whitespace only.
func TestGetLatestVersionByURL_WhitespaceOnlyResponse(t *testing.T) {
	mockey.PatchConvey("get latest version by url whitespace only response", t, func() {
		mockey.Mock(distsign.Fetch).To(func(url string, limit int64) ([]byte, error) {
			return []byte("   \n\t  "), nil
		}).Build()

		ver, err := GetLatestVersionByURL("https://example.com/version.txt")
		require.NoError(t, err)
		assert.Equal(t, "", ver)
	})
}

// TestSanitizeVersion_StableVersions tests sanitizeVersion with various stable versions.
func TestSanitizeVersion_StableVersions(t *testing.T) {
	tests := []struct {
		name    string
		version string
		want    string
	}{
		{"v0.1.0", "v0.1.0", "stable"},
		{"v0.3.0", "v0.3.0", "stable"},
		{"v0.5.0", "v0.5.0", "stable"},
		{"v0.7.0", "v0.7.0", "stable"},
		{"v0.9.0", "v0.9.0", "stable"},
		{"v1.1.0", "v1.1.0", "stable"},
		{"v1.3.0", "v1.3.0", "stable"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := sanitizeVersion(tt.version)
			require.NoError(t, err)
			assert.Equal(t, tt.want, result)
		})
	}
}

// TestSanitizeVersion_UnstableVersions tests sanitizeVersion with various unstable versions.
func TestSanitizeVersion_UnstableVersions(t *testing.T) {
	tests := []struct {
		name    string
		version string
		want    string
	}{
		{"v0.0.0", "v0.0.0", "unstable"},
		{"v0.2.0", "v0.2.0", "unstable"},
		{"v0.4.0", "v0.4.0", "unstable"},
		{"v0.6.0", "v0.6.0", "unstable"},
		{"v0.8.0", "v0.8.0", "unstable"},
		{"v1.0.0", "v1.0.0", "unstable"},
		{"v1.2.0", "v1.2.0", "unstable"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := sanitizeVersion(tt.version)
			require.NoError(t, err)
			assert.Equal(t, tt.want, result)
		})
	}
}

// TestSanitizeVersion_MalformedVersions tests sanitizeVersion with malformed versions.
func TestSanitizeVersion_MalformedVersions(t *testing.T) {
	tests := []struct {
		name    string
		version string
	}{
		{"no dots", "v100"},
		{"single dot", "v1.0"},
		{"non-numeric minor", "va.b.c"},
		{"empty", ""},
		{"just v", "v"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := sanitizeVersion(tt.version)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "malformed version")
		})
	}
}

// TestGetLatestVersion_StableTrack tests GetLatestVersion uses stable track for odd minor versions.
func TestGetLatestVersion_StableTrack(t *testing.T) {
	oddMinorVersions := []string{"v0.1.0", "v0.3.5", "v0.5.10", "v1.1.0", "v2.3.4"}

	for _, ver := range oddMinorVersions {
		t.Run(ver, func(t *testing.T) {
			mockey.PatchConvey("stable track", t, func() {
				originalVersion := version.Version
				defer func() {
					version.Version = originalVersion
				}()
				version.Version = ver

				var requestedURL string
				mockey.Mock(distsign.Fetch).To(func(url string, limit int64) ([]byte, error) {
					requestedURL = url
					return []byte("v9.9.9"), nil
				}).Build()

				_, err := GetLatestVersion()
				require.NoError(t, err)
				assert.Contains(t, requestedURL, "stable_latest.txt")
			})
		})
	}
}

// TestGetLatestVersion_UnstableTrack tests GetLatestVersion uses unstable track for even minor versions.
func TestGetLatestVersion_UnstableTrack(t *testing.T) {
	evenMinorVersions := []string{"v0.0.0", "v0.2.5", "v0.4.10", "v1.0.0", "v2.2.4"}

	for _, ver := range evenMinorVersions {
		t.Run(ver, func(t *testing.T) {
			mockey.PatchConvey("unstable track", t, func() {
				originalVersion := version.Version
				defer func() {
					version.Version = originalVersion
				}()
				version.Version = ver

				var requestedURL string
				mockey.Mock(distsign.Fetch).To(func(url string, limit int64) ([]byte, error) {
					requestedURL = url
					return []byte("v9.9.9"), nil
				}).Build()

				_, err := GetLatestVersion()
				require.NoError(t, err)
				assert.Contains(t, requestedURL, "unstable_latest.txt")
			})
		})
	}
}

// TestGetLatestVersionByURL_VerifiesLimit tests that GetLatestVersionByURL passes correct limit.
func TestGetLatestVersionByURL_VerifiesLimit(t *testing.T) {
	mockey.PatchConvey("verifies limit", t, func() {
		var capturedLimit int64
		mockey.Mock(distsign.Fetch).To(func(url string, limit int64) ([]byte, error) {
			capturedLimit = limit
			return []byte("v1.0.0"), nil
		}).Build()

		_, err := GetLatestVersionByURL("https://example.com/version.txt")
		require.NoError(t, err)
		assert.Equal(t, int64(100), capturedLimit)
	})
}
