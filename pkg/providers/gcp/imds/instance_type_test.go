package imds

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFetchInstanceType(t *testing.T) {
	for _, tc := range []struct {
		body, want string
		wantError  bool
	}{
		{"projects/123/machineTypes/a3-highgpu-8g\n", "a3-highgpu-8g", false},
		{"projects/123/zones/us-central1-a/machineTypes/a3-megagpu-8g", "a3-megagpu-8g", false},
		{"a3-highgpu-8g", "a3-highgpu-8g", false},
		{"", "", false},
		{"projects/123/machineTypes/", "", true},
		{"projects/123/zones/us-central1-a", "", true},
		{"projects/123/machineTypes/type/extra", "", true},
	} {
		t.Run(tc.body, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "/instance/machine-type", r.URL.Path)
				assert.Equal(t, "Google", r.Header.Get("Metadata-Flavor"))
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()
			value, err := fetchInstanceType(context.Background(), server.URL)
			if tc.wantError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tc.want, value)
		})
	}
}
