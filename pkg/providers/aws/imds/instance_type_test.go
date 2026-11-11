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
	for _, status := range []int{http.StatusOK, http.StatusNotFound, http.StatusForbidden} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/token" {
					assert.Equal(t, http.MethodPut, r.Method)
					assert.NotEmpty(t, r.Header.Get("X-aws-ec2-metadata-token-ttl-seconds"))
					_, _ = w.Write([]byte("test-token"))
					return
				}
				assert.Equal(t, "/meta-data/instance-type", r.URL.Path)
				assert.Equal(t, "test-token", r.Header.Get("X-aws-ec2-metadata-token"))
				w.WriteHeader(status)
				_, _ = w.Write([]byte(" p5.48xlarge\n"))
			}))
			defer server.Close()
			value, err := fetchInstanceType(context.Background(), server.URL+"/token", server.URL+"/meta-data")
			if status != http.StatusOK {
				require.Error(t, err)
				assert.Empty(t, value)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, "p5.48xlarge", value)
		})
	}
}
