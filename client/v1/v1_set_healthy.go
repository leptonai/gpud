package v1

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/leptonai/gpud/pkg/httputil"
	"github.com/leptonai/gpud/pkg/log"
)

// SetHealthyComponents sets specified components to healthy state
func SetHealthyComponents(ctx context.Context, addr string, components []string, opts ...OpOption) error {
	op := &Op{}
	if err := op.applyOpts(opts); err != nil {
		return err
	}

	reqURL, err := url.Parse(fmt.Sprintf("%s/v1/health-states/set-healthy", addr))
	if err != nil {
		return err
	}

	// Add components to query parameters if specified
	if len(components) > 0 {
		q := reqURL.Query()
		q.Add("components", strings.Join(components, ","))
		reqURL.RawQuery = q.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL.String(), nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	if op.requestContentType != "" {
		req.Header.Set(httputil.RequestHeaderContentType, op.requestContentType)
	}
	if op.requestAcceptEncoding != "" {
		req.Header.Set(httputil.RequestHeaderAcceptEncoding, op.requestAcceptEncoding)
	}

	resp, err := createDefaultHTTPClient().Do(req)
	if err != nil {
		return fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("server returned %d: %s", resp.StatusCode, string(body))
	}

	var response struct {
		Success []string          `json:"success,omitempty"`
		Failed  map[string]string `json:"failed,omitempty"`
		Message string            `json:"message,omitempty"`
		Code    string            `json:"code,omitempty"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		log.Logger.Warnf("failed to decode response: %v", err)
		// Not a critical error if we got 200 OK
		return nil
	}

	if len(response.Failed) > 0 {
		return fmt.Errorf("some components failed to set healthy: %v", response.Failed)
	}

	return nil
}
