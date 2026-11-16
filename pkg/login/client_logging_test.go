package login

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/pkg/log"
)

func TestLoginCredentialsStayOutOfLogsAndErrors(t *testing.T) {
	path := filepath.Join(t.TempDir(), "login.log")
	cfg := log.DefaultLoggerConfig()
	cfg.Level = zap.NewAtomicLevelAt(zap.DebugLevel)
	cfg.OutputPaths = []string{path}
	previous := log.Logger
	log.Logger = log.CreateLoggerWithConfig(cfg)
	t.Cleanup(func() { log.Logger = previous })

	const enrollment = "synthetic-enrollment-secret"
	const session = "synthetic-session-secret"
	const proof = "synthetic-machine-proof"
	for _, status := range []int{http.StatusOK, http.StatusUnauthorized} {
		server := newLocalHTTPServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var request apiv1.LoginRequest
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Error(err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			if request.Token != enrollment || r.Header.Get("Authorization") != "Bearer "+enrollment {
				t.Error("authentication credential changed")
			}
			w.WriteHeader(status)
			_ = json.NewEncoder(w).Encode(apiv1.LoginResponse{
				MachineID: "test-machine", Token: session, MachineProof: proof,
				Code: "test-status", Message: "test-message",
			})
		}))
		response, err := sendRequest(context.Background(), server.URL, apiv1.LoginRequest{Token: enrollment}, proof)
		require.NotNil(t, response)
		require.Equal(t, session, response.Token)
		require.Equal(t, proof, response.MachineProof)
		if status == http.StatusOK {
			require.NoError(t, err)
		} else {
			require.EqualError(t, err, "unexpected status code 401")
			require.Equal(t, "test-message", response.Message)
		}
	}
	output, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Contains(t, string(output), "login request processed")
	for _, secret := range []string{enrollment, session, proof} {
		require.NotContains(t, string(output), secret)
	}
}
