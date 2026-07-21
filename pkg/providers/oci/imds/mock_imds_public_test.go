package imds

import (
	"context"
	"testing"

	"github.com/bytedance/mockey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFetchPrimaryVNICPrivateIPv4_Public_WithMockey(t *testing.T) {
	mockey.PatchConvey("FetchPrimaryVNICPrivateIPv4 uses the default OCI metadata URL", t, func() {
		mockey.Mock(fetchPrimaryVNICPrivateIPv4).To(func(ctx context.Context, metadataURL string) (string, error) {
			assert.Equal(t, imdsMetadataURL, metadataURL)
			return "203.0.113.10", nil
		}).Build()

		got, err := FetchPrimaryVNICPrivateIPv4(context.Background())
		require.NoError(t, err)
		assert.Equal(t, "203.0.113.10", got)
	})
}
