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
		name, body, want string
		status           int
		wantError        bool
	}{
		{"vm size", `{"vmSize":"Standard_ND96isr_H100_v5","resourceId":"/subscriptions/s/vm/id"}`, "Standard_ND96isr_H100_v5", 200, false},
		{"missing size", `{"resourceId":"/subscriptions/s/vm/id"}`, "", 200, false},
		{"invalid json", `{`, "", 200, true},
		{"unavailable", `unavailable`, "", 503, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "/instance/compute", r.URL.Path)
				assert.Equal(t, "true", r.Header.Get("Metadata"))
				assert.Equal(t, defaultAPIVersion, r.URL.Query().Get("api-version"))
				w.WriteHeader(tc.status)
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
