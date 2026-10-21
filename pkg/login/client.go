package login

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/pkg/httputil"
	"github.com/leptonai/gpud/pkg/log"
)

// ErrEmptyMachineID reports a successful login response that omitted the assigned machine ID.
var ErrEmptyMachineID = errors.New("login request failed with empty machine ID")

// machineProofHeader carries the proof from this machine's previous login, the
// same header session requests use.
const machineProofHeader = "X-GPUD-Machine-Proof"

// dropMachineProofOnRedirect keeps the default redirect limit but removes the
// proof unless a redirect keeps the login request's scheme and host. Go
// forwards custom headers on any redirect, including to another host or from
// HTTPS to HTTP.
func dropMachineProofOnRedirect(redirect *http.Request, via []*http.Request) error {
	if len(via) >= 10 {
		return errors.New("stopped after 10 redirects")
	}
	original := via[0].URL
	if redirect.URL.Scheme != original.Scheme || !strings.EqualFold(redirect.URL.Hostname(), original.Hostname()) {
		redirect.Header.Del(machineProofHeader)
	}
	return nil
}

// SendRequest sends a login request and blocks until the login request is processed.
// It also validates the response field to ensure the login request is processed successfully.
//
// The server responds with the following status codes and messages:
//
// Success:
// - 200 OK: Login successful. Returns Machine ID and Token.
//
// Failures:
// - 400 Bad Request:
//   - Invalid JSON
//   - Missing Machine Info
//   - Missing Token
//   - Missing ID/NodeGroup
//   - Node Group Mismatch
//
// - 401 Unauthorized: Invalid Token
// - 403 Forbidden:
//   - Forbidden Access (machine not owned by workspace)
//   - Forbidden Node Group (node group not owned by workspace)
//
// - 404 Not Found:
//   - Machine Not Found
//   - Node Group Not Found
//
// - 500 Internal Server Error:
//   - Token Validation Failed
//   - Session Token Error
//   - Machine Retrieval/Creation/Update Errors
//   - Node Group Error
//   - ID Generation Error
//
// machineProof is the proof from this machine's previous login, or empty. It is
// sent in the X-GPUD-Machine-Proof header and never in the JSON body, which is
// debug-logged.
func SendRequest(ctx context.Context, endpoint string, req apiv1.LoginRequest, machineProof string) (*apiv1.LoginResponse, error) {
	url, err := httputil.CreateURL("https", endpoint, "/api/v1/login")
	if err != nil {
		return nil, fmt.Errorf("error creating URL: %w", err)
	}
	return sendRequest(ctx, url, req, machineProof)
}

func sendRequest(ctx context.Context, endpointURL string, req apiv1.LoginRequest, machineProof string) (*apiv1.LoginResponse, error) {
	log.Logger.Debugw("sending login request", "endpointURL", endpointURL)

	u, err := url.Parse(endpointURL)
	if err != nil {
		return nil, err
	}
	host := u.Hostname()
	if host == "" {
		return nil, fmt.Errorf("no host in endpoint URL: %s", endpointURL)
	}

	b, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("error marshaling login request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpointURL, bytes.NewBuffer(b))
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", req.Token))
	if machineProof != "" {
		httpReq.Header.Set(machineProofHeader, machineProof)
	}
	httpReq.Header.Set("Origin", host)

	client := &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
		},
		CheckRedirect: dropMachineProofOnRedirect,
	}
	httpResp, err := client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = httpResp.Body.Close()
	}()

	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response body: %w", err)
	}

	var resp apiv1.LoginResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("error unmarshaling login response: %w", err)
	}

	if httpResp.StatusCode != http.StatusOK {
		return &resp, fmt.Errorf("unexpected status code %d (%s)", httpResp.StatusCode, string(body))
	}

	if resp.MachineID == "" {
		return &resp, ErrEmptyMachineID
	}

	log.Logger.Debugw("login request processed", "data", string(b), "endpointURL", endpointURL, "machineID", resp.MachineID)
	return &resp, nil
}
