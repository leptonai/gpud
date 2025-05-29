package v1

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"

	v1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/pkg/httputil"
	"github.com/leptonai/gpud/pkg/log"
)

// TriggerComponent manually triggers a component check.
func TriggerComponent(ctx context.Context, addr string, componentName string, opts ...OpOption) (v1.GPUdComponentHealthStates, error) {
	op := &Op{}
	if err := op.applyOpts(opts); err != nil {
		return nil, err
	}

	if componentName == "" {
		return nil, errors.New("component name is required")
	}

	reqURL, err := url.Parse(fmt.Sprintf("%s/v1/components/trigger-check", addr))
	if err != nil {
		return nil, err
	}

	q := reqURL.Query()
	q.Add("componentName", componentName)
	reqURL.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL.String(), nil)
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
		return nil, errors.New("server not ready, response not 200")
	}

	var healthStates v1.GPUdComponentHealthStates
	if err := json.NewDecoder(resp.Body).Decode(&healthStates); err != nil {
		return nil, fmt.Errorf("failed to decode json: %w", err)
	}

	log.Logger.Infow("triggered component check", "component", componentName, "healthStates", healthStates)
	return healthStates, nil
}

// TriggerComponentCheckByTag triggers all components that have the specified tag
func TriggerComponentCheckByTag(ctx context.Context, addr string, tagName string, opts ...OpOption) error {
	op := &Op{}
	if err := op.applyOpts(opts); err != nil {
		return err
	}

	if tagName == "" {
		return errors.New("tag name is required")
	}

	reqURL, err := url.Parse(fmt.Sprintf("%s/v1/components/trigger-tag", addr))
	if err != nil {
		return err
	}

	q := reqURL.Query()
	q.Add("tagName", tagName)
	reqURL.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL.String(), nil)
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
		return errors.New("server not ready, response not 200")
	}

	var response struct {
		Components []string `json:"components"`
		Exit       int      `json:"exit"`
		Success    bool     `json:"success"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return fmt.Errorf("failed to decode json: %w", err)
	}

	if !response.Success {
		return fmt.Errorf("health check failed for tag %s, components: %v", tagName, response.Components)
	}

	return nil
}
