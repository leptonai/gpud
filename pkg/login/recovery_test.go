package login

import (
	"context"
	"encoding/json"
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

func TestRecoveryHTTPResponse(t *testing.T) {
	const token = "synthetic-pod-token"
	const enrollment = "synthetic-enrollment"
	req := apiv1.LoginRequest{MachineID: "machine-a", Token: enrollment}
	for _, tc := range []struct {
		name     string
		response apiv1.LoginResponse
		ok       bool
	}{
		{name: "success", response: apiv1.LoginResponse{MachineID: req.MachineID, MachineProof: "replacement", Token: "session"}, ok: true},
		{name: "other machine", response: apiv1.LoginResponse{MachineID: "machine-b", MachineProof: "replacement", Token: "session"}},
		{name: "missing proof", response: apiv1.LoginResponse{MachineID: req.MachineID, Token: "session"}},
		{name: "missing session", response: apiv1.LoginResponse{MachineID: req.MachineID, MachineProof: "replacement"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := newLocalHTTPServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewEncoder(w).Encode(tc.response)
			}))
			resp, err := sendRequest(context.Background(), server.URL, req, "", token)
			if tc.ok {
				require.NoError(t, err)
				require.Equal(t, tc.response, *resp)
			} else {
				require.Error(t, err)
				require.Nil(t, resp)
			}
		})
	}
	t.Run("successful recovery redacts printed validation fields", func(t *testing.T) {
		server := newLocalHTTPServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(apiv1.LoginResponse{
				MachineID: req.MachineID, Token: "new-session", MachineProof: "new-proof",
				ValidationResults: []apiv1.ValidationResult{{
					Name: token, Reason: "failed " + enrollment, Suggestion: "retry new-session new-proof",
				}},
			})
		}))
		resp, err := sendRequest(t.Context(), server.URL, req, "", token)
		require.NoError(t, err)
		require.Equal(t, []apiv1.ValidationResult{{
			Name: "<redacted>", Reason: "failed <redacted>", Suggestion: "retry <redacted> <redacted>",
		}}, resp.ValidationResults)
		require.Equal(t, "new-proof", resp.MachineProof)
		require.Equal(t, "new-session", resp.Token)
	})
	t.Run("recovery does not follow even a same-origin redirect", func(t *testing.T) {
		server := newLocalHTTPServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/redirected" {
				t.Error("redirect received a recovery request")
				w.WriteHeader(http.StatusForbidden)
				return
			}
			http.Redirect(w, r, "/redirected", http.StatusTemporaryRedirect)
		}))
		_, err := sendRequest(t.Context(), server.URL, req, "", token)
		require.ErrorContains(t, err, "307")
	})
	t.Run("invalid redirect cannot echo credentials through the HTTP error", func(t *testing.T) {
		server := newLocalHTTPServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Location", "/%zz/"+r.Header.Get(httputil.RequestHeaderRecoveryToken))
			w.WriteHeader(http.StatusTemporaryRedirect)
		}))
		_, err := sendRequest(context.Background(), server.URL, req, "", token)
		require.Error(t, err)
		require.NotContains(t, err.Error(), token)
	})
	t.Run("invalid redirect cannot echo a stored proof", func(t *testing.T) {
		server := newLocalHTTPServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Location", "/%zz/"+r.Header.Get(httputil.RequestHeaderMachineProof))
			w.WriteHeader(http.StatusTemporaryRedirect)
		}))
		_, err := sendRequest(t.Context(), server.URL, req, "previous-proof", "")
		require.Error(t, err)
		require.NotContains(t, err.Error(), "previous-proof")
	})
}

func TestRecoveryRequiresConfiguredOrigin(t *testing.T) {
	req := apiv1.LoginRequest{MachineID: "machine-a"}
	for _, endpoint := range []string{"", "https://", "http://manager.example", "//manager.example", "https://user@manager.example"} {
		_, err := sendLoginRequest(context.Background(), endpoint, req, "", "/missing-token")
		require.EqualError(t, err, "pod recovery requires a configured HTTPS login origin")
	}
}

func TestStoredProofLoginRejectsDifferentMachine(t *testing.T) {
	server := newLocalHTTPServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(apiv1.LoginResponse{MachineID: "other-machine", Token: "session"})
	}))
	req := apiv1.LoginRequest{MachineID: "registered-machine", Token: "enrollment"}
	resp, err := sendRequest(t.Context(), server.URL, req, "stored-proof", "")
	require.Error(t, err)
	require.Nil(t, resp)
}
