package v1

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	v1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/errdefs"
	"github.com/leptonai/gpud/internal/server"
	"sigs.k8s.io/yaml"
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
		req.Header.Set(server.RequestHeaderContentType, op.requestContentType)
	}
	if op.requestAcceptEncoding != "" {
		req.Header.Set(server.RequestHeaderAcceptEncoding, op.requestAcceptEncoding)
	}

	resp, err := op.httpClient.Do(req)
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
	case server.RequestHeaderEncodingGzip:
		gr, err := gzip.NewReader(rd)
		if err != nil {
			return nil, fmt.Errorf("failed to create gzip reader: %w", err)
		}
		defer gr.Close()

		switch op.requestContentType {
		case server.RequestHeaderJSON, "":
			if err := json.NewDecoder(gr).Decode(&components); err != nil {
				return nil, fmt.Errorf("failed to decode json: %w", err)
			}
		case server.RequestHeaderYAML:
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
		case server.RequestHeaderJSON, "":
			if err := json.NewDecoder(rd).Decode(&components); err != nil {
				return nil, fmt.Errorf("failed to decode json: %w", err)
			}
		case server.RequestHeaderYAML:
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

func GetInfo(ctx context.Context, addr string, opts ...OpOption) (v1.LeptonInfo, error) {
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
		req.Header.Set(server.RequestHeaderContentType, op.requestContentType)
	}
	if op.requestAcceptEncoding != "" {
		req.Header.Set(server.RequestHeaderAcceptEncoding, op.requestAcceptEncoding)
	}

	resp, err := op.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("server not ready, response not 200")
	}

	return ReadInfo(resp.Body, opts...)
}

func ReadInfo(rd io.Reader, opts ...OpOption) (v1.LeptonInfo, error) {
	op := &Op{}
	if err := op.applyOpts(opts); err != nil {
		return nil, err
	}

	var info v1.LeptonInfo
	switch op.requestAcceptEncoding {
	case server.RequestHeaderEncodingGzip:
		gr, err := gzip.NewReader(rd)
		if err != nil {
			return nil, fmt.Errorf("failed to create gzip reader: %w", err)
		}
		defer gr.Close()

		switch op.requestContentType {
		case server.RequestHeaderJSON, "":
			if err := json.NewDecoder(gr).Decode(&info); err != nil {
				return nil, fmt.Errorf("failed to decode json: %w", err)
			}
		case server.RequestHeaderYAML:
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
		case server.RequestHeaderJSON, "":
			if err := json.NewDecoder(rd).Decode(&info); err != nil {
				return nil, fmt.Errorf("failed to decode json: %w", err)
			}
		case server.RequestHeaderYAML:
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

func GetStates(ctx context.Context, addr string, opts ...OpOption) (v1.LeptonStates, error) {
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
		req.Header.Set(server.RequestHeaderContentType, op.requestContentType)
	}
	if op.requestAcceptEncoding != "" {
		req.Header.Set(server.RequestHeaderAcceptEncoding, op.requestAcceptEncoding)
	}

	resp, err := op.httpClient.Do(req)
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

	return nil, nil
}

func ReadStates(rd io.Reader, opts ...OpOption) (v1.LeptonStates, error) {
	op := &Op{}
	if err := op.applyOpts(opts); err != nil {
		return nil, err
	}

	var states v1.LeptonStates
	switch op.requestAcceptEncoding {
	case server.RequestHeaderEncodingGzip:
		gr, err := gzip.NewReader(rd)
		if err != nil {
			return nil, fmt.Errorf("failed to create gzip reader: %w", err)
		}
		defer gr.Close()

		switch op.requestContentType {
		case server.RequestHeaderJSON, "":
			if err := json.NewDecoder(gr).Decode(&states); err != nil {
				return nil, fmt.Errorf("failed to decode json: %w", err)
			}
		case server.RequestHeaderYAML:
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
		case server.RequestHeaderJSON, "":
			if err := json.NewDecoder(rd).Decode(&states); err != nil {
				return nil, fmt.Errorf("failed to decode json: %w", err)
			}
		case server.RequestHeaderYAML:
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

func GetEvents(ctx context.Context, addr string, opts ...OpOption) (v1.LeptonEvents, error) {
	op := &Op{}
	if err := op.applyOpts(opts); err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/v1/events", addr), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	if op.requestContentType != "" {
		req.Header.Set(server.RequestHeaderContentType, op.requestContentType)
	}
	if op.requestAcceptEncoding != "" {
		req.Header.Set(server.RequestHeaderAcceptEncoding, op.requestAcceptEncoding)
	}

	resp, err := op.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("server not ready, response not 200")
	}

	return ReadEvents(resp.Body, opts...)
}

func ReadEvents(rd io.Reader, opts ...OpOption) (v1.LeptonEvents, error) {
	op := &Op{}
	if err := op.applyOpts(opts); err != nil {
		return nil, err
	}

	var evs v1.LeptonEvents
	switch op.requestAcceptEncoding {
	case server.RequestHeaderEncodingGzip:
		gr, err := gzip.NewReader(rd)
		if err != nil {
			return nil, fmt.Errorf("failed to create gzip reader: %w", err)
		}
		defer gr.Close()

		switch op.requestContentType {
		case server.RequestHeaderJSON, "":
			if err := json.NewDecoder(gr).Decode(&evs); err != nil {
				return nil, fmt.Errorf("failed to decode json: %w", err)
			}
		case server.RequestHeaderYAML:
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
		case server.RequestHeaderJSON, "":
			if err := json.NewDecoder(rd).Decode(&evs); err != nil {
				return nil, fmt.Errorf("failed to decode json: %w", err)
			}
		case server.RequestHeaderYAML:
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

func GetMetrics(ctx context.Context, addr string, opts ...OpOption) (v1.LeptonMetrics, error) {
	op := &Op{}
	if err := op.applyOpts(opts); err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/v1/metrics", addr), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	if op.requestContentType != "" {
		req.Header.Set(server.RequestHeaderContentType, op.requestContentType)
	}
	if op.requestAcceptEncoding != "" {
		req.Header.Set(server.RequestHeaderAcceptEncoding, op.requestAcceptEncoding)
	}

	resp, err := op.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("server not ready, response not 200")
	}

	return ReadMetrics(resp.Body, opts...)
}

func ReadMetrics(rd io.Reader, opts ...OpOption) (v1.LeptonMetrics, error) {
	op := &Op{}
	if err := op.applyOpts(opts); err != nil {
		return nil, err
	}

	var metrics v1.LeptonMetrics
	switch op.requestAcceptEncoding {
	case server.RequestHeaderEncodingGzip:
		gr, err := gzip.NewReader(rd)
		if err != nil {
			return nil, fmt.Errorf("failed to create gzip reader: %w", err)
		}
		defer gr.Close()

		switch op.requestContentType {
		case server.RequestHeaderJSON, "":
			if err := json.NewDecoder(gr).Decode(&metrics); err != nil {
				return nil, fmt.Errorf("failed to decode json: %w", err)
			}
		case server.RequestHeaderYAML:
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
		case server.RequestHeaderJSON, "":
			if err := json.NewDecoder(rd).Decode(&metrics); err != nil {
				return nil, fmt.Errorf("failed to decode json: %w", err)
			}
		case server.RequestHeaderYAML:
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
