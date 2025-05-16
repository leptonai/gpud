package v1

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"sigs.k8s.io/yaml"

	v1 "github.com/leptonai/gpud/api/v1"
	pkgcustomplugins "github.com/leptonai/gpud/pkg/custom-plugins"
	"github.com/leptonai/gpud/pkg/errdefs"
	"github.com/leptonai/gpud/pkg/log"
)

const (
	RequestHeaderContentType = "Content-Type"
	RequestHeaderJSON        = "application/json"
	RequestHeaderYAML        = "application/yaml"
	RequestHeaderJSONIndent  = "json-indent"

	RequestHeaderAcceptEncoding = "Accept-Encoding"
	RequestHeaderEncodingGzip   = "gzip"
)

func GetComponents(ctx context.Context, addr string, opts ...OpOption) ([]string, error) {
	op := &Op{}
	if err := op.applyOpts(opts); err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/v1/components", addr), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	if op.requestContentType != "" {
		req.Header.Set(RequestHeaderContentType, op.requestContentType)
	}
	if op.requestAcceptEncoding != "" {
		req.Header.Set(RequestHeaderAcceptEncoding, op.requestAcceptEncoding)
	}

	resp, err := createDefaultHTTPClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("server not ready, response not 200")
	}

	return ReadComponents(resp.Body, opts...)
}

func ReadComponents(rd io.Reader, opts ...OpOption) ([]string, error) {
	op := &Op{}
	if err := op.applyOpts(opts); err != nil {
		return nil, err
	}

	var components []string
	switch op.requestAcceptEncoding {
	case RequestHeaderEncodingGzip:
		gr, err := gzip.NewReader(rd)
		if err != nil {
			return nil, fmt.Errorf("failed to create gzip reader: %w", err)
		}
		defer gr.Close()

		switch op.requestContentType {
		case RequestHeaderJSON, "":
			if err := json.NewDecoder(gr).Decode(&components); err != nil {
				return nil, fmt.Errorf("failed to decode json: %w", err)
			}
		case RequestHeaderYAML:
			b, err := io.ReadAll(gr)
			if err != nil {
				return nil, fmt.Errorf("failed to read yaml: %w", err)
			}
			if err := yaml.Unmarshal(b, &components); err != nil {
				return nil, fmt.Errorf("failed to unmarshal yaml: %w", err)
			}
		default:
			return nil, fmt.Errorf("unsupported content type: %s", op.requestContentType)
		}

	default:
		switch op.requestContentType {
		case RequestHeaderJSON, "":
			if err := json.NewDecoder(rd).Decode(&components); err != nil {
				return nil, fmt.Errorf("failed to decode json: %w", err)
			}
		case RequestHeaderYAML:
			b, err := io.ReadAll(rd)
			if err != nil {
				return nil, fmt.Errorf("failed to read yaml: %w", err)
			}
			if err := yaml.Unmarshal(b, &components); err != nil {
				return nil, fmt.Errorf("failed to unmarshal yaml: %w", err)
			}
		default:
			return nil, fmt.Errorf("unsupported content type: %s", op.requestContentType)
		}
	}

	return components, nil
}

// DeregisterComponent deregisters the component from the server, by using the component name.
// It fails if the component has not been registered yet or is not deregisterable.
func DeregisterComponent(ctx context.Context, addr string, componentName string, opts ...OpOption) error {
	op := &Op{}
	if err := op.applyOpts(opts); err != nil {
		return err
	}

	if componentName == "" {
		return errors.New("component name is required")
	}

	reqURL, err := url.Parse(fmt.Sprintf("%s/v1/components", addr))
	if err != nil {
		return err
	}

	q := reqURL.Query()
	q.Add("componentName", componentName)
	reqURL.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, reqURL.String(), nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	if op.requestContentType != "" {
		req.Header.Set(RequestHeaderContentType, op.requestContentType)
	}
	if op.requestAcceptEncoding != "" {
		req.Header.Set(RequestHeaderAcceptEncoding, op.requestAcceptEncoding)
	}

	resp, err := createDefaultHTTPClient().Do(req)
	if err != nil {
		return fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return errors.New("server not ready, response not 200")
	}

	rb, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	log.Logger.Infow("deregistered custom plugin", "component", componentName, "response", string(rb))

	return nil
}

// TriggerComponentCheck manually triggers a component check.
func TriggerComponentCheck(ctx context.Context, addr string, componentName string, opts ...OpOption) (v1.HealthStates, error) {
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
		req.Header.Set(RequestHeaderContentType, op.requestContentType)
	}
	if op.requestAcceptEncoding != "" {
		req.Header.Set(RequestHeaderAcceptEncoding, op.requestAcceptEncoding)
	}

	resp, err := createDefaultHTTPClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("server not ready, response not 200")
	}

	var healthStates v1.HealthStates
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
		req.Header.Set(RequestHeaderContentType, op.requestContentType)
	}
	if op.requestAcceptEncoding != "" {
		req.Header.Set(RequestHeaderAcceptEncoding, op.requestAcceptEncoding)
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

// GetInfo returns component information from the server.
//
// Example:
//
//	baseURL := "https://localhost:15132"
//	componentName := "" // Leave empty to query all components
//
//	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
//	defer cancel()
//	info, err := GetInfo(ctx, baseURL, WithComponent(componentName))
//	if err != nil {
//		fmt.Println("Error fetching component info:", err)
//		return
//	}
//
//	fmt.Println("Component Information:")
//	for _, i := range info {
//		fmt.Printf("Component: %s\n", i.Component)
//		for _, event := range i.Info.Events {
//			fmt.Printf("  Event: %s - %s\n", event.Name, event.Message)
//		}
//		for _, metric := range i.Info.Metrics {
//			fmt.Printf("  Metric: %s (labels: %q) - Value: %f\n", metric.Name, metric.Labels, metric.Value)
//		}
//		for _, state := range i.Info.States {
//			fmt.Printf("  State: %s - Health: %s\n", state.Name, state.Health)
//		}
//	}
func GetInfo(ctx context.Context, addr string, opts ...OpOption) (v1.GPUdComponentInfos, error) {
	op := &Op{}
	if err := op.applyOpts(opts); err != nil {
		return nil, err
	}

	reqURL, err := url.Parse(fmt.Sprintf("%s/v1/info", addr))
	if err != nil {
		return nil, err
	}
	q := reqURL.Query()
	if len(op.components) > 0 {
		components := make([]string, 0, len(op.components))
		for component := range op.components {
			components = append(components, component)
		}
		q.Add("components", strings.Join(components, ","))
	}
	reqURL.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	if op.requestContentType != "" {
		req.Header.Set(RequestHeaderContentType, op.requestContentType)
	}
	if op.requestAcceptEncoding != "" {
		req.Header.Set(RequestHeaderAcceptEncoding, op.requestAcceptEncoding)
	}

	resp, err := createDefaultHTTPClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("server not ready, response not 200")
	}

	return ReadInfo(resp.Body, opts...)
}

func ReadInfo(rd io.Reader, opts ...OpOption) (v1.GPUdComponentInfos, error) {
	op := &Op{}
	if err := op.applyOpts(opts); err != nil {
		return nil, err
	}

	var info v1.GPUdComponentInfos
	switch op.requestAcceptEncoding {
	case RequestHeaderEncodingGzip:
		gr, err := gzip.NewReader(rd)
		if err != nil {
			return nil, fmt.Errorf("failed to create gzip reader: %w", err)
		}
		defer gr.Close()

		switch op.requestContentType {
		case RequestHeaderJSON, "":
			if err := json.NewDecoder(gr).Decode(&info); err != nil {
				return nil, fmt.Errorf("failed to decode json: %w", err)
			}
		case RequestHeaderYAML:
			b, err := io.ReadAll(gr)
			if err != nil {
				return nil, fmt.Errorf("failed to read yaml: %w", err)
			}
			if err := yaml.Unmarshal(b, &info); err != nil {
				return nil, fmt.Errorf("failed to unmarshal yaml: %w", err)
			}
		default:
			return nil, fmt.Errorf("unsupported content type: %s", op.requestContentType)
		}

	default:
		switch op.requestContentType {
		case RequestHeaderJSON, "":
			if err := json.NewDecoder(rd).Decode(&info); err != nil {
				return nil, fmt.Errorf("failed to decode json: %w", err)
			}
		case RequestHeaderYAML:
			b, err := io.ReadAll(rd)
			if err != nil {
				return nil, fmt.Errorf("failed to read yaml: %w", err)
			}
			if err := yaml.Unmarshal(b, &info); err != nil {
				return nil, fmt.Errorf("failed to unmarshal yaml: %w", err)
			}
		default:
			return nil, fmt.Errorf("unsupported content type: %s", op.requestContentType)
		}
	}

	return info, nil
}

// GetHealthStates returns health states from the server for the given components.
//
// Example:
//
//	baseURL := "https://localhost:15132"
//	for _, componentName := range []string{"disk", "accelerator-nvidia-info"} {
//		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
//		defer cancel()
//		states, err := GetHealthStates(ctx, baseURL, WithComponent(componentName))
//		if err != nil {
//			if errdefs.IsNotFound(err) {
//				log.Logger.Warnw("component not found", "component", componentName)
//				return
//			}
//
//			log.Logger.Error("error fetching component info", "error", err)
//			return
//		}
//
//		for _, ss := range states {
//			for _, s := range ss.States {
//				log.Logger.Infof("state: %q, health: %s\n", s.Name, s.Health)
//			}
//		}
//	}
func GetHealthStates(ctx context.Context, addr string, opts ...OpOption) (v1.GPUdComponentHealthStates, error) {
	op := &Op{}
	if err := op.applyOpts(opts); err != nil {
		return nil, err
	}

	reqURL, err := url.Parse(fmt.Sprintf("%s/v1/states", addr))
	if err != nil {
		return nil, err
	}
	q := reqURL.Query()
	if len(op.components) > 0 {
		components := make([]string, 0, len(op.components))
		for component := range op.components {
			components = append(components, component)
		}
		q.Add("components", strings.Join(components, ","))
	}
	reqURL.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	if op.requestContentType != "" {
		req.Header.Set(RequestHeaderContentType, op.requestContentType)
	}
	if op.requestAcceptEncoding != "" {
		req.Header.Set(RequestHeaderAcceptEncoding, op.requestAcceptEncoding)
	}

	resp, err := createDefaultHTTPClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusNotFound {
			return nil, errdefs.ErrNotFound
		}
		return nil, errors.New("server not ready, response not 200")
	}

	return ReadHealthStates(resp.Body, opts...)
}

func ReadHealthStates(rd io.Reader, opts ...OpOption) (v1.GPUdComponentHealthStates, error) {
	op := &Op{}
	if err := op.applyOpts(opts); err != nil {
		return nil, err
	}

	var states v1.GPUdComponentHealthStates
	switch op.requestAcceptEncoding {
	case RequestHeaderEncodingGzip:
		gr, err := gzip.NewReader(rd)
		if err != nil {
			return nil, fmt.Errorf("failed to create gzip reader: %w", err)
		}
		defer gr.Close()

		switch op.requestContentType {
		case RequestHeaderJSON, "":
			if err := json.NewDecoder(gr).Decode(&states); err != nil {
				return nil, fmt.Errorf("failed to decode json: %w", err)
			}
		case RequestHeaderYAML:
			b, err := io.ReadAll(gr)
			if err != nil {
				return nil, fmt.Errorf("failed to read yaml: %w", err)
			}
			if err := yaml.Unmarshal(b, &states); err != nil {
				return nil, fmt.Errorf("failed to unmarshal yaml: %w", err)
			}
		default:
			return nil, fmt.Errorf("unsupported content type: %s", op.requestContentType)
		}

	default:
		switch op.requestContentType {
		case RequestHeaderJSON, "":
			if err := json.NewDecoder(rd).Decode(&states); err != nil {
				return nil, fmt.Errorf("failed to decode json: %w", err)
			}
		case RequestHeaderYAML:
			b, err := io.ReadAll(rd)
			if err != nil {
				return nil, fmt.Errorf("failed to read yaml: %w", err)
			}
			if err := yaml.Unmarshal(b, &states); err != nil {
				return nil, fmt.Errorf("failed to unmarshal yaml: %w", err)
			}
		default:
			return nil, fmt.Errorf("unsupported content type: %s", op.requestContentType)
		}
	}

	return states, nil
}

func GetEvents(ctx context.Context, addr string, opts ...OpOption) (v1.GPUdComponentEvents, error) {
	op := &Op{}
	if err := op.applyOpts(opts); err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/v1/events", addr), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	if op.requestContentType != "" {
		req.Header.Set(RequestHeaderContentType, op.requestContentType)
	}
	if op.requestAcceptEncoding != "" {
		req.Header.Set(RequestHeaderAcceptEncoding, op.requestAcceptEncoding)
	}

	resp, err := createDefaultHTTPClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("server not ready, response not 200")
	}

	return ReadEvents(resp.Body, opts...)
}

func ReadEvents(rd io.Reader, opts ...OpOption) (v1.GPUdComponentEvents, error) {
	op := &Op{}
	if err := op.applyOpts(opts); err != nil {
		return nil, err
	}

	var evs v1.GPUdComponentEvents
	switch op.requestAcceptEncoding {
	case RequestHeaderEncodingGzip:
		gr, err := gzip.NewReader(rd)
		if err != nil {
			return nil, fmt.Errorf("failed to create gzip reader: %w", err)
		}
		defer gr.Close()

		switch op.requestContentType {
		case RequestHeaderJSON, "":
			if err := json.NewDecoder(gr).Decode(&evs); err != nil {
				return nil, fmt.Errorf("failed to decode json: %w", err)
			}
		case RequestHeaderYAML:
			b, err := io.ReadAll(gr)
			if err != nil {
				return nil, fmt.Errorf("failed to read yaml: %w", err)
			}
			if err := yaml.Unmarshal(b, &evs); err != nil {
				return nil, fmt.Errorf("failed to unmarshal yaml: %w", err)
			}
		default:
			return nil, fmt.Errorf("unsupported content type: %s", op.requestContentType)
		}

	default:
		switch op.requestContentType {
		case RequestHeaderJSON, "":
			if err := json.NewDecoder(rd).Decode(&evs); err != nil {
				return nil, fmt.Errorf("failed to decode json: %w", err)
			}
		case RequestHeaderYAML:
			b, err := io.ReadAll(rd)
			if err != nil {
				return nil, fmt.Errorf("failed to read yaml: %w", err)
			}
			if err := yaml.Unmarshal(b, &evs); err != nil {
				return nil, fmt.Errorf("failed to unmarshal yaml: %w", err)
			}
		default:
			return nil, fmt.Errorf("unsupported content type: %s", op.requestContentType)
		}
	}

	return evs, nil
}

func GetMetrics(ctx context.Context, addr string, opts ...OpOption) (v1.GPUdComponentMetrics, error) {
	op := &Op{}
	if err := op.applyOpts(opts); err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/v1/metrics", addr), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	if op.requestContentType != "" {
		req.Header.Set(RequestHeaderContentType, op.requestContentType)
	}
	if op.requestAcceptEncoding != "" {
		req.Header.Set(RequestHeaderAcceptEncoding, op.requestAcceptEncoding)
	}

	resp, err := createDefaultHTTPClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("server not ready, response not 200")
	}

	return ReadMetrics(resp.Body, opts...)
}

func ReadMetrics(rd io.Reader, opts ...OpOption) (v1.GPUdComponentMetrics, error) {
	op := &Op{}
	if err := op.applyOpts(opts); err != nil {
		return nil, err
	}

	var metrics v1.GPUdComponentMetrics
	switch op.requestAcceptEncoding {
	case RequestHeaderEncodingGzip:
		gr, err := gzip.NewReader(rd)
		if err != nil {
			return nil, fmt.Errorf("failed to create gzip reader: %w", err)
		}
		defer gr.Close()

		switch op.requestContentType {
		case RequestHeaderJSON, "":
			if err := json.NewDecoder(gr).Decode(&metrics); err != nil {
				return nil, fmt.Errorf("failed to decode json: %w", err)
			}
		case RequestHeaderYAML:
			b, err := io.ReadAll(gr)
			if err != nil {
				return nil, fmt.Errorf("failed to read yaml: %w", err)
			}
			if err := yaml.Unmarshal(b, &metrics); err != nil {
				return nil, fmt.Errorf("failed to unmarshal yaml: %w", err)
			}
		default:
			return nil, fmt.Errorf("unsupported content type: %s", op.requestContentType)
		}

	default:
		switch op.requestContentType {
		case RequestHeaderJSON, "":
			if err := json.NewDecoder(rd).Decode(&metrics); err != nil {
				return nil, fmt.Errorf("failed to decode json: %w", err)
			}
		case RequestHeaderYAML:
			b, err := io.ReadAll(rd)
			if err != nil {
				return nil, fmt.Errorf("failed to read yaml: %w", err)
			}
			if err := yaml.Unmarshal(b, &metrics); err != nil {
				return nil, fmt.Errorf("failed to unmarshal yaml: %w", err)
			}
		default:
			return nil, fmt.Errorf("unsupported content type: %s", op.requestContentType)
		}
	}

	return metrics, nil
}

// GetCustomPlugins returns the custom plugins registered in the server.
func GetCustomPlugins(ctx context.Context, addr string, opts ...OpOption) (map[string]pkgcustomplugins.Spec, error) {
	op := &Op{}
	if err := op.applyOpts(opts); err != nil {
		return nil, err
	}

	reqURL, err := url.Parse(fmt.Sprintf("%s/v1/components/custom-plugin", addr))
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	if op.requestContentType != "" {
		req.Header.Set(RequestHeaderContentType, op.requestContentType)
	}
	if op.requestAcceptEncoding != "" {
		req.Header.Set(RequestHeaderAcceptEncoding, op.requestAcceptEncoding)
	}

	resp, err := createDefaultHTTPClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusNotFound {
			return nil, errdefs.ErrNotFound
		}
		return nil, errors.New("server not ready, response not 200")
	}

	return ReadCustomPluginSpecs(resp.Body, opts...)
}

// ReadCustomPluginSpecs reads the custom plugin specs from the server.
func ReadCustomPluginSpecs(rd io.Reader, opts ...OpOption) (map[string]pkgcustomplugins.Spec, error) {
	op := &Op{}
	if err := op.applyOpts(opts); err != nil {
		return nil, err
	}

	var csPlugins map[string]pkgcustomplugins.Spec
	switch op.requestAcceptEncoding {
	case RequestHeaderEncodingGzip:
		gr, err := gzip.NewReader(rd)
		if err != nil {
			return nil, fmt.Errorf("failed to create gzip reader: %w", err)
		}
		defer gr.Close()

		switch op.requestContentType {
		case RequestHeaderJSON, "":
			if err := json.NewDecoder(gr).Decode(&csPlugins); err != nil {
				return nil, fmt.Errorf("failed to decode json: %w", err)
			}
		case RequestHeaderYAML:
			b, err := io.ReadAll(gr)
			if err != nil {
				return nil, fmt.Errorf("failed to read yaml: %w", err)
			}
			if err := yaml.Unmarshal(b, &csPlugins); err != nil {
				return nil, fmt.Errorf("failed to unmarshal yaml: %w", err)
			}
		default:
			return nil, fmt.Errorf("unsupported content type: %s", op.requestContentType)
		}

	default:
		switch op.requestContentType {
		case RequestHeaderJSON, "":
			if err := json.NewDecoder(rd).Decode(&csPlugins); err != nil {
				return nil, fmt.Errorf("failed to decode json: %w", err)
			}
		case RequestHeaderYAML:
			b, err := io.ReadAll(rd)
			if err != nil {
				return nil, fmt.Errorf("failed to read yaml: %w", err)
			}
			if err := yaml.Unmarshal(b, &csPlugins); err != nil {
				return nil, fmt.Errorf("failed to unmarshal yaml: %w", err)
			}
		default:
			return nil, fmt.Errorf("unsupported content type: %s", op.requestContentType)
		}
	}

	return csPlugins, nil
}

// RegisterCustomPlugin registers a new custom plugin.
// It fails if the custom plugin has already been registered.
func RegisterCustomPlugin(ctx context.Context, addr string, spec pkgcustomplugins.Spec, opts ...OpOption) error {
	return registerOrUpdateCustomPlugin(ctx, addr, spec, http.MethodPost, opts...)
}

// UpdateCustomPlugin updates a custom plugin.
// It fails if the custom plugin has not been registered yet.
func UpdateCustomPlugin(ctx context.Context, addr string, spec pkgcustomplugins.Spec, opts ...OpOption) error {
	return registerOrUpdateCustomPlugin(ctx, addr, spec, http.MethodPut, opts...)
}

// registerOrUpdateCustomPlugin is a helper function to register or update a custom plugin.
func registerOrUpdateCustomPlugin(ctx context.Context, addr string, spec pkgcustomplugins.Spec, method string, opts ...OpOption) error {
	op := &Op{}
	if err := op.applyOpts(opts); err != nil {
		return err
	}

	if err := spec.Validate(); err != nil {
		return fmt.Errorf("invalid spec: %w", err)
	}

	b, err := json.Marshal(spec)
	if err != nil {
		return fmt.Errorf("failed to marshal spec: %w", err)
	}

	reqURL, err := url.Parse(fmt.Sprintf("%s/v1/components/custom-plugin", addr))
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, method, reqURL.String(), bytes.NewReader(b))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	if op.requestContentType != "" {
		req.Header.Set(RequestHeaderContentType, op.requestContentType)
	}
	if op.requestAcceptEncoding != "" {
		req.Header.Set(RequestHeaderAcceptEncoding, op.requestAcceptEncoding)
	}

	switch method {
	case http.MethodPost:
		log.Logger.Infow("registering custom plugin", "component", spec.ComponentName())
	case http.MethodPut:
		log.Logger.Infow("updating custom plugin", "component", spec.ComponentName())
	default:
		return fmt.Errorf("unsupported method: %s", method)
	}

	resp, err := createDefaultHTTPClient().Do(req)
	if err != nil {
		return fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return errors.New("server not ready, response not 200")
	}

	rb, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	switch method {
	case http.MethodPost:
		log.Logger.Infow("registered custom plugin", "component", spec.ComponentName(), "response", string(rb))
	case http.MethodPut:
		log.Logger.Infow("updated custom plugin", "component", spec.ComponentName(), "response", string(rb))
	default:
		return fmt.Errorf("unsupported method: %s", method)
	}

	return nil
}

func createDefaultHTTPClient() *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}
}
