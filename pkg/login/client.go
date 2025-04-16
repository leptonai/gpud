package login

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/pkg/log"
)

var ErrEmptyMachineID = errors.New("login request failed with empty machine ID")

// SendRequest sends a login request and blocks until the login request is processed.
// It also validates the response field to ensure the login request is processed successfully.
func SendRequest(ctx context.Context, endpoint string, req apiv1.LoginRequest) (*apiv1.LoginResponse, error) {
	url := createURL(endpoint)
	return sendRequest(ctx, url, req)
}

func sendRequest(ctx context.Context, url string, req apiv1.LoginRequest) (*apiv1.LoginResponse, error) {
	log.Logger.Infow("sending login request", "url", url)

	b, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("error marshaling login request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(b))
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	httpResp, err := client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer httpResp.Body.Close()

	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response body: %w", err)
	}

	var resp apiv1.LoginResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("error unmarshalling login response: %w", err)
	}

	if httpResp.StatusCode != http.StatusOK {
		return &resp, fmt.Errorf("login request failed with status code %d (%+v)", httpResp.StatusCode, resp)
	}

	if resp.MachineID == "" {
		return &resp, ErrEmptyMachineID
	}

	log.Logger.Infow("login request processed", "url", url, "machineID", resp.MachineID)
	return &resp, nil
}

// createURL creates a URL for the login endpoint
func createURL(endpoint string) string {
	return fmt.Sprintf("https://%s/api/v1/login", endpoint)
}
