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

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/pkg/httputil"
	"github.com/leptonai/gpud/pkg/log"
)

// ErrEmptyMachineID reports a successful login response that omitted the assigned machine ID.
var ErrEmptyMachineID = errors.New("login request failed with empty machine ID")

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
func SendRequest(ctx context.Context, endpoint string, req apiv1.LoginRequest) (*apiv1.LoginResponse, error) {
	url, err := httputil.CreateURL("https", endpoint, "/api/v1/login")
	if err != nil {
		return nil, fmt.Errorf("error creating URL: %w", err)
	}
	return sendRequest(ctx, url, req)
}

func sendRequest(ctx context.Context, endpointURL string, req apiv1.LoginRequest) (*apiv1.LoginResponse, error) {
	log.Logger.Debugw("sending login request", "endpointURL", endpointURL)

	u, err := url.Parse(endpointURL)
	if err != nil {
		return nil, err
	}
	host := u.Hostname()
	if host == "" {
		return nil, fmt.Errorf("no host in endpoint URL: %s", endpointURL)
	}

	b, err := marshalLoginRequest(req)
	if err != nil {
		return nil, fmt.Errorf("error marshaling login request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpointURL, bytes.NewBuffer(b))
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", req.Token))
	httpReq.Header.Set("Origin", host)

	client := &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
		},
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

func marshalLoginRequest(req apiv1.LoginRequest) ([]byte, error) {
	type loginRequestPayload struct {
		Token              string                 `json:"token"`
		MachineID          string                 `json:"machineID"`
		NodeGroup          string                 `json:"nodeGroup,omitempty"`
		NodeLabels         *map[string]string     `json:"nodeLabels,omitempty"`
		Network            *apiv1.MachineNetwork  `json:"network,omitempty"`
		Location           *apiv1.MachineLocation `json:"location,omitempty"`
		Provider           string                 `json:"provider"`
		ProviderInstanceID string                 `json:"providerInstanceID"`
		MachineInfo        *apiv1.MachineInfo     `json:"machineInfo,omitempty"`
		Resources          map[string]string      `json:"resources,omitempty"`
	}

	payload := loginRequestPayload{
		Token:              req.Token,
		MachineID:          req.MachineID,
		NodeGroup:          req.NodeGroup,
		Network:            req.Network,
		Location:           req.Location,
		Provider:           req.Provider,
		ProviderInstanceID: req.ProviderInstanceID,
		MachineInfo:        req.MachineInfo,
		Resources:          req.Resources,
	}
	if req.NodeLabels != nil {
		nodeLabels := req.NodeLabels
		payload.NodeLabels = &nodeLabels
	}

	return json.Marshal(payload)
}
