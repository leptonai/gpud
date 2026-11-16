package login

import (
	"context"
	"encoding/json"
	"fmt"
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
	// Keep this test sequential because it temporarily replaces the shared logger.
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

func TestMalformedLoginResponseKeepsHTTPStatus(t *testing.T) {
	const secret = "98765432109876543210"
	for _, body := range []string{
		`{"token":` + secret + `}`,
		`{"machineProof":` + secret + `}`,
		"upstream unavailable " + secret,
	} {
		for _, status := range []int{http.StatusOK, http.StatusUnauthorized} {
			server := newLocalHTTPServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(status)
				_, _ = w.Write([]byte(body))
			}))
			response, err := sendRequest(context.Background(), server.URL, apiv1.LoginRequest{}, "")
			require.Nil(t, response)
			require.Error(t, err)
			require.NotContains(t, err.Error(), secret)
			require.EqualError(t, err, fmt.Sprintf("error unmarshaling login response (HTTP %d)", status))
		}
	}
}

func TestLoginErrorResponseRedactsEchoedCredentials(t *testing.T) {
	const enrollment = "synthetic-enrollment"
	const previousProof = "synthetic-proof"
	// The returned proof extends the previous one to catch partial redaction.
	const newProof = previousProof + "-renewed"
	const session = "synthetic-session"
	for _, status := range []int{http.StatusUnauthorized, http.StatusOK} {
		server := newLocalHTTPServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(status)
			_, _ = w.Write([]byte(`{"code":"denied ` + enrollment + `","message":"retry ` + previousProof + ` ` + newProof + ` ` + session + `","status":"denied ` + session + `","error":"retry ` + enrollment + `","token":"` + session + `","machineProof":"` + newProof + `"}`))
		}))
		response, err := sendRequest(context.Background(), server.URL, apiv1.LoginRequest{Token: enrollment}, previousProof)
		require.Error(t, err)
		require.NotNil(t, response)
		require.Equal(t, "denied <redacted>", response.Code)
		require.Equal(t, "retry <redacted> <redacted> <redacted>", response.Message)
		// Marshal to also check legacy error fields without using deprecated Go fields.
		encoded, marshalErr := json.Marshal(response)
		require.NoError(t, marshalErr)
		var fields map[string]any
		require.NoError(t, json.Unmarshal(encoded, &fields))
		require.Equal(t, "denied <redacted>", fields["status"])
		require.Equal(t, "retry <redacted>", fields["error"])
		require.Equal(t, session, response.Token)
		require.Equal(t, newProof, response.MachineProof)
	}
}
