package login

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bytedance/mockey"
	"github.com/stretchr/testify/require"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/pkg/httputil"
)

func TestProjectedRecoveryToken(t *testing.T) {
	for _, tc := range []struct {
		name, machineID, proof, fileValue, wantHeader string
		wantErr                                       bool
	}{
		{name: "empty local state", machineID: "machine-a", fileValue: "synthetic-pod-token\n", wantHeader: "synthetic-pod-token"},
		{name: "stored proof takes precedence", machineID: "machine-a", proof: "synthetic-proof"},
		{name: "new machine does not read token"},
		{name: "missing projection", machineID: "machine-a", wantErr: true},
		{name: "empty projection", machineID: "machine-a", fileValue: "\n", wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mockey.PatchConvey(tc.name, t, func() {
				path := filepath.Join(t.TempDir(), "token")
				if tc.fileValue != "" {
					require.NoError(t, os.WriteFile(path, []byte(tc.fileValue), 0600))
				}
				var sent bool
				mockey.Mock((*http.Client).Do).To(func(client *http.Client, req *http.Request) (*http.Response, error) {
					sent = true
					require.Equal(t, "https", req.URL.Scheme)
					require.Equal(t, "Bearer enrollment", req.Header.Get("Authorization"))
					require.Equal(t, tc.proof, req.Header.Get(httputil.RequestHeaderMachineProof))
					require.Equal(t, tc.wantHeader, req.Header.Get(httputil.RequestHeaderRecoveryToken))
					body, err := io.ReadAll(req.Body)
					require.NoError(t, err)
					require.NotContains(t, string(body), "synthetic-pod-token")
					if tc.wantHeader != "" {
						require.ErrorIs(t, client.CheckRedirect(req, []*http.Request{req}), http.ErrUseLastResponse)
					}
					return &http.Response{StatusCode: 403, Body: io.NopCloser(strings.NewReader(`{"message":"denied synthetic-pod-token"}`))}, nil
				}).Build()
				res, err := sendLoginRequest(context.Background(), "manager.example", apiv1.LoginRequest{MachineID: tc.machineID, Token: "enrollment"}, tc.proof, path)
				require.Error(t, err)
				require.Equal(t, !tc.wantErr, sent)
				if tc.wantHeader != "" {
					require.Equal(t, "denied <redacted>", res.Message)
				}
			})
		})
	}
}
