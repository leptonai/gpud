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
	"slices"
	"strings"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/pkg/httputil"
	"github.com/leptonai/gpud/pkg/log"
)

// ErrEmptyMachineID reports a successful login response that omitted the assigned machine ID.
var ErrEmptyMachineID = errors.New("login request failed with empty machine ID")

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
		redirect.Header.Del(httputil.RequestHeaderMachineProof)
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
// sent in the X-GPUD-Machine-Proof header rather than the JSON body. Neither
// credential-bearing payload is logged.
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
		httpReq.Header.Set(httputil.RequestHeaderMachineProof, machineProof)
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
		// Keep the HTTP status useful even when an upstream returns malformed JSON.
		// Decoder details are untrusted response data, just like the body itself.
		return nil, fmt.Errorf("error unmarshaling login response (HTTP %d)", httpResp.StatusCode)
	}

	if httpResp.StatusCode != http.StatusOK || resp.MachineID == "" {
		redactLoginErrorResponse(&resp, req.Token, machineProof)
	}

	if httpResp.StatusCode != http.StatusOK {
		// Response bodies can contain credentials; callers still receive the structured response.
		return &resp, fmt.Errorf("unexpected status code %d", httpResp.StatusCode)
	}

	if resp.MachineID == "" {
		return &resp, ErrEmptyMachineID
	}

	// The serialized request contains the enrollment token and must never reach logs.
	log.Logger.Debugw("login request processed", "endpointURL", endpointURL, "machineID", resp.MachineID)
	return &resp, nil
}

// redactLoginErrorResponse preserves server diagnostics without letting echoed
// credentials reach the caller's logs or CLI output, including legacy fields.
func redactLoginErrorResponse(resp *apiv1.LoginResponse, credentials ...string) {
	credentials = append(credentials, resp.Token, resp.MachineProof)
	// A rotated credential can extend an old one; match the longest value first
	// so replacing the old prefix cannot leave the new credential's suffix behind.
	slices.SortFunc(credentials, func(a, b string) int { return len(b) - len(a) })
	replacements := make([]string, 0, 2*len(credentials))
	for _, credential := range credentials {
		if credential != "" {
			replacements = append(replacements, credential, "<redacted>")
		}
	}
	replacer := strings.NewReplacer(replacements...)
	resp.Code = replacer.Replace(resp.Code)
	resp.Message = replacer.Replace(resp.Message)
	//nolint:staticcheck // Older servers still populate these error fields.
	resp.Status = replacer.Replace(resp.Status)
	//nolint:staticcheck // Older servers still populate these error fields.
	resp.Error = replacer.Replace(resp.Error)
}
