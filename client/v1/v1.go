package v1

import (
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
	"github.com/leptonai/gpud/pkg/errdefs"
	"github.com/leptonai/gpud/pkg/httputil"
	"github.com/leptonai/gpud/pkg/log"
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
		req.Header.Set(httputil.RequestHeaderContentType, op.requestContentType)
	}
	if op.requestAcceptEncoding != "" {
		req.Header.Set(httputil.RequestHeaderAcceptEncoding, op.requestAcceptEncoding)
	}

	resp, err := createDefaultHTTPClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()
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
	case httputil.RequestHeaderEncodingGzip:
		gr, err := gzip.NewReader(rd)
		if err != nil {
			return nil, fmt.Errorf("failed to create gzip reader: %w", err)
		}
		defer func() {
			_ = gr.Close()
		}()

		switch op.requestContentType {
		case httputil.RequestHeaderJSON, "":
			if err := json.NewDecoder(gr).Decode(&components); err != nil {
				return nil, fmt.Errorf("failed to decode json: %w", err)
			}
		case httputil.RequestHeaderYAML:
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
		case httputil.RequestHeaderJSON, "":
			if err := json.NewDecoder(rd).Decode(&components); err != nil {
				return nil, fmt.Errorf("failed to decode json: %w", err)
			}
		case httputil.RequestHeaderYAML:
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
		req.Header.Set(httputil.RequestHeaderContentType, op.requestContentType)
	}
	if op.requestAcceptEncoding != "" {
		req.Header.Set(httputil.RequestHeaderAcceptEncoding, op.requestAcceptEncoding)
	}

	resp, err := createDefaultHTTPClient().Do(req)
	if err != nil {
		return fmt.Errorf("failed to make request: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

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
		req.Header.Set(httputil.RequestHeaderContentType, op.requestContentType)
	}
	if op.requestAcceptEncoding != "" {
		req.Header.Set(httputil.RequestHeaderAcceptEncoding, op.requestAcceptEncoding)
	}

	resp, err := createDefaultHTTPClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()
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
	case httputil.RequestHeaderEncodingGzip:
		gr, err := gzip.NewReader(rd)
		if err != nil {
			return nil, fmt.Errorf("failed to create gzip reader: %w", err)
		}
		defer func() {
			_ = gr.Close()
		}()

		switch op.requestContentType {
		case httputil.RequestHeaderJSON, "":
			if err := json.NewDecoder(gr).Decode(&info); err != nil {
				return nil, fmt.Errorf("failed to decode json: %w", err)
			}
		case httputil.RequestHeaderYAML:
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
		case httputil.RequestHeaderJSON, "":
			if err := json.NewDecoder(rd).Decode(&info); err != nil {
				return nil, fmt.Errorf("failed to decode json: %w", err)
			}
		case httputil.RequestHeaderYAML:
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
		req.Header.Set(httputil.RequestHeaderContentType, op.requestContentType)
	}
	if op.requestAcceptEncoding != "" {
		req.Header.Set(httputil.RequestHeaderAcceptEncoding, op.requestAcceptEncoding)
	}

	resp, err := createDefaultHTTPClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

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
	case httputil.RequestHeaderEncodingGzip:
		gr, err := gzip.NewReader(rd)
		if err != nil {
			return nil, fmt.Errorf("failed to create gzip reader: %w", err)
		}
		defer func() {
			_ = gr.Close()
		}()

		switch op.requestContentType {
		case httputil.RequestHeaderJSON, "":
			if err := json.NewDecoder(gr).Decode(&states); err != nil {
				return nil, fmt.Errorf("failed to decode json: %w", err)
			}
		case httputil.RequestHeaderYAML:
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
		case httputil.RequestHeaderJSON, "":
			if err := json.NewDecoder(rd).Decode(&states); err != nil {
				return nil, fmt.Errorf("failed to decode json: %w", err)
			}
		case httputil.RequestHeaderYAML:
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
		req.Header.Set(httputil.RequestHeaderContentType, op.requestContentType)
	}
	if op.requestAcceptEncoding != "" {
		req.Header.Set(httputil.RequestHeaderAcceptEncoding, op.requestAcceptEncoding)
	}

	resp, err := createDefaultHTTPClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()
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
	case httputil.RequestHeaderEncodingGzip:
		gr, err := gzip.NewReader(rd)
		if err != nil {
			return nil, fmt.Errorf("failed to create gzip reader: %w", err)
		}
		defer func() {
			_ = gr.Close()
		}()

		switch op.requestContentType {
		case httputil.RequestHeaderJSON, "":
			if err := json.NewDecoder(gr).Decode(&evs); err != nil {
				return nil, fmt.Errorf("failed to decode json: %w", err)
			}
		case httputil.RequestHeaderYAML:
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
		case httputil.RequestHeaderJSON, "":
			if err := json.NewDecoder(rd).Decode(&evs); err != nil {
				return nil, fmt.Errorf("failed to decode json: %w", err)
			}
		case httputil.RequestHeaderYAML:
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
		req.Header.Set(httputil.RequestHeaderContentType, op.requestContentType)
	}
	if op.requestAcceptEncoding != "" {
		req.Header.Set(httputil.RequestHeaderAcceptEncoding, op.requestAcceptEncoding)
	}

	resp, err := createDefaultHTTPClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()
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
	case httputil.RequestHeaderEncodingGzip:
		gr, err := gzip.NewReader(rd)
		if err != nil {
			return nil, fmt.Errorf("failed to create gzip reader: %w", err)
		}
		defer func() {
			_ = gr.Close()
		}()

		switch op.requestContentType {
		case httputil.RequestHeaderJSON, "":
			if err := json.NewDecoder(gr).Decode(&metrics); err != nil {
				return nil, fmt.Errorf("failed to decode json: %w", err)
			}
		case httputil.RequestHeaderYAML:
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
		case httputil.RequestHeaderJSON, "":
			if err := json.NewDecoder(rd).Decode(&metrics); err != nil {
				return nil, fmt.Errorf("failed to decode json: %w", err)
			}
		case httputil.RequestHeaderYAML:
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

func createDefaultHTTPClient() *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			Proxy:           http.ProxyFromEnvironment,
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}
}
