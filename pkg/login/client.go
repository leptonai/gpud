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
	"os"
	"slices"
	"strings"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/pkg/httputil"
	"github.com/leptonai/gpud/pkg/log"
)

// ErrEmptyMachineID reports a successful login response that omitted the assigned machine ID.
var ErrEmptyMachineID = errors.New("login request failed with empty machine ID")

// A 307 or 308 can replay the enrollment credential in the POST body even
// after headers are stripped. Keep ordinary login redirects on the same origin.
func checkLoginRedirect(redirect *http.Request, via []*http.Request) error {
	if len(via) >= 10 {
		return errors.New("stopped after 10 redirects")
	}
	original := via[0].URL
	if redirect.URL.Scheme != original.Scheme || !strings.EqualFold(redirect.URL.Hostname(), original.Hostname()) ||
		loginPort(redirect.URL) != loginPort(original) {
		return http.ErrUseLastResponse
	}
	return nil
}

func loginPort(u *url.URL) string {
	if port := u.Port(); port != "" {
		return port
	}
	if u.Scheme == "https" {
		return "443"
	}
	return "80"
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
	return sendLoginRequest(ctx, endpoint, req, machineProof, "")
}

func sendLoginRequest(ctx context.Context, endpoint string, req apiv1.LoginRequest, machineProof, recoveryTokenFile string) (*apiv1.LoginResponse, error) {
	var recoveryToken string
	if req.MachineID != "" && machineProof == "" && recoveryTokenFile != "" {
		// Accept a host[:port] or an HTTPS origin. The legacy URL builder
		// prepends its scheme even when the input already has another one.
		origin, err := url.Parse("https://" + strings.TrimPrefix(endpoint, "https://"))
		if err != nil || origin.Hostname() == "" || origin.User != nil ||
			(origin.Path != "" && origin.Path != "/") || origin.RawQuery != "" || origin.Fragment != "" {
			return nil, errors.New("pod recovery requires a configured HTTPS login origin")
		}
		data, err := os.ReadFile(recoveryTokenFile)
		if err != nil {
			return nil, fmt.Errorf("read projected recovery token: %w", err)
		}
		recoveryToken = strings.TrimSpace(string(data))
		if recoveryToken == "" || strings.ContainsAny(recoveryToken, " \t\r\n") {
			return nil, errors.New("invalid projected recovery token")
		}
	}
	url, err := httputil.CreateURL("https", endpoint, "/api/v1/login")
	if err != nil {
		return nil, fmt.Errorf("error creating URL: %w", err)
	}
	return sendRequest(ctx, url, req, machineProof, recoveryToken)
}

func sendRequest(ctx context.Context, endpointURL string, req apiv1.LoginRequest, machineProof, recoveryToken string) (*apiv1.LoginResponse, error) {
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
		CheckRedirect: checkLoginRedirect,
	}
	if recoveryToken != "" {
		httpReq.Header.Set(httputil.RequestHeaderRecoveryToken, recoveryToken)
		// A redirect can forward both custom headers and the enrollment request body.
		client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	}
	httpResp, err := client.Do(httpReq)
	if err != nil {
		// net/http parses Location before CheckRedirect and can quote the
		// entire header in an error, including a reflected credential.
		return nil, redactLoginTransportError(err, req.Token, machineProof, recoveryToken)
	}
	defer func() {
		_ = httpResp.Body.Close()
	}()

	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response body: %w", redactLoginTransportError(err, req.Token, machineProof, recoveryToken))
	}

	var resp apiv1.LoginResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		// Keep the HTTP status useful even when an upstream returns malformed JSON.
		// Decoder details are untrusted response data, just like the body itself.
		return nil, fmt.Errorf("error unmarshaling login response (HTTP %d)", httpResp.StatusCode)
	}

	redactLoginDiagnostics(&resp, req.Token, machineProof, recoveryToken)

	if httpResp.StatusCode != http.StatusOK {
		// Response bodies can contain credentials; callers still receive the structured response.
		return &resp, fmt.Errorf("unexpected status code %d", httpResp.StatusCode)
	}

	if resp.MachineID == "" {
		return &resp, ErrEmptyMachineID
	}

	if req.MachineID != "" && resp.MachineID != req.MachineID {
		return nil, errors.New("login response contains an unexpected machine ID")
	}
	if recoveryToken != "" && (resp.MachineProof == "" || resp.Token == "") {
		return nil, errors.New("incomplete credentials in recovery response")
	}

	// The serialized request contains the enrollment token and must never reach logs.
	log.Logger.Debugw("login request processed", "endpointURL", endpointURL, "machineID", resp.MachineID)
	return &resp, nil
}

// Login prints validation diagnostics even when the request succeeds.
func redactLoginDiagnostics(resp *apiv1.LoginResponse, credentials ...string) {
	credentials = append(credentials, resp.Token, resp.MachineProof)
	replacer := loginCredentialReplacer(credentials...)
	resp.Code = replacer.Replace(resp.Code)
	resp.Message = replacer.Replace(resp.Message)
	//nolint:staticcheck // Older servers still populate these error fields.
	resp.Status = replacer.Replace(resp.Status)
	//nolint:staticcheck // Older servers still populate these error fields.
	resp.Error = replacer.Replace(resp.Error)
	for i := range resp.ValidationResults {
		result := &resp.ValidationResults[i]
		result.Name = replacer.Replace(result.Name)
		result.Reason = replacer.Replace(result.Reason)
		result.Suggestion = replacer.Replace(result.Suggestion)
	}
}

func redactLoginTransportError(err error, credentials ...string) error {
	if safe := loginCredentialReplacer(credentials...).Replace(err.Error()); safe != err.Error() {
		return errors.New(safe)
	}
	return err
}

func loginCredentialReplacer(credentials ...string) *strings.Replacer {
	// A rotated credential can extend an old one; match the longest value first
	// so replacing the old prefix cannot leave the new credential's suffix behind.
	slices.SortFunc(credentials, func(a, b string) int { return len(b) - len(a) })
	replacements := make([]string, 0, 2*len(credentials))
	for _, credential := range credentials {
		if credential != "" {
			replacements = append(replacements, credential, "<redacted>")
		}
	}
	return strings.NewReplacer(replacements...)
}
