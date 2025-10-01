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
	"github.com/leptonai/gpud/pkg/server"
)

// SetHealthyComponents sets specified components to healthy state
func SetHealthyComponents(ctx context.Context, addr string, components []string, opts ...OpOption) ([]string, error) {
	op := &Op{}
	if err := op.applyOpts(opts); err != nil {
		return nil, err
	}

	reqURL, err := url.Parse(fmt.Sprintf("%s/v1/health-states/set-healthy", addr))
	if err != nil {
		return nil, err
	}

	// Add components to query parameters if specified
	if len(components) > 0 {
		q := reqURL.Query()
		q.Add("components", strings.Join(components, ","))
		reqURL.RawQuery = q.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	if op.requestContentType != "" {
		req.Header.Set(httputil.RequestHeaderContentType, op.requestContentType)
	}
	if op.requestAcceptEncoding != "" {
		req.Header.Set(httputil.RequestHeaderAcceptEncoding, op.requestAcceptEncoding)
	}

	resp, err := createDefaultHTTPClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("server returned %d: %s", resp.StatusCode, string(body))
	}

	var response server.SetHealthyStatesResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		log.Logger.Warnf("failed to decode response: %v", err)
		// Not a critical error if we got 200 OK
		return cloneStringSlice(components), nil
	}

	if len(response.Successful) == 0 && len(components) > 0 {
		response.Successful = cloneStringSlice(components)
	}

	if len(response.Failed) > 0 {
		return nil, fmt.Errorf("some components failed to set healthy: %v", response.Failed)
	}

	return response.Successful, nil
}

func cloneStringSlice(src []string) []string {
	if len(src) == 0 {
		return nil
	}
	dst := make([]string, len(src))
	copy(dst, src)
	return dst
}
