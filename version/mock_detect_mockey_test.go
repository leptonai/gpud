package version

import (
	"errors"
	"testing"

	"github.com/bytedance/mockey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/leptonai/gpud/pkg/release/distsign"
)

func TestDetectLatestVersion_WithMockeySuccess(t *testing.T) {
	mockey.PatchConvey("DetectLatestVersion success with mocked fetch", t, func() {
		originalVersion := Version
		defer func() {
			Version = originalVersion
		}()
		Version = "v1.3.7" // odd minor => stable

		mockey.Mock(distsign.Fetch).To(func(url string, limit int64) ([]byte, error) {
			assert.Equal(t, DefaultURLPrefix+"stable_latest.txt", url)
			return []byte(" v1.3.9 \n"), nil
		}).Build()

		ver, err := DetectLatestVersion()
		require.NoError(t, err)
		assert.Equal(t, "v1.3.9", ver)
	})
}

func TestDetectLatestVersion_WithMockeyFetchError(t *testing.T) {
	mockey.PatchConvey("DetectLatestVersion fetch error", t, func() {
		originalVersion := Version
		defer func() {
			Version = originalVersion
		}()
		Version = "v0.2.1" // even minor => unstable

		mockey.Mock(distsign.Fetch).To(func(url string, limit int64) ([]byte, error) {
			assert.Equal(t, DefaultURLPrefix+"unstable_latest.txt", url)
			return nil, errors.New("fetch failed")
		}).Build()

		ver, err := DetectLatestVersion()
		require.Error(t, err)
		assert.Empty(t, ver)
	})
}
