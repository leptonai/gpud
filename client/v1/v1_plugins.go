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

	"sigs.k8s.io/yaml"

	pkgcustomplugins "github.com/leptonai/gpud/pkg/custom-plugins"
	"github.com/leptonai/gpud/pkg/errdefs"
	"github.com/leptonai/gpud/pkg/httputil"
)

// GetPluginSpecs returns the custom plugins registered in the server.
func GetPluginSpecs(ctx context.Context, addr string, opts ...OpOption) (pkgcustomplugins.Specs, error) {
	op := &Op{}
	if err := op.applyOpts(opts); err != nil {
		return nil, err
	}

	reqURL, err := url.Parse(fmt.Sprintf("%s/v1/plugins", addr))
	if err != nil {
		return nil, err
	}

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
		if resp.StatusCode == http.StatusNotFound {
			return nil, errdefs.ErrNotFound
		}
		return nil, errors.New("server not ready, response not 200")
	}

	return ReadPluginSpecs(resp.Body, opts...)
}

// ReadPluginSpecs reads the custom plugin specs from the server.
func ReadPluginSpecs(rd io.Reader, opts ...OpOption) (pkgcustomplugins.Specs, error) {
	op := &Op{}
	if err := op.applyOpts(opts); err != nil {
		return nil, err
	}

	var specs pkgcustomplugins.Specs
	switch op.requestAcceptEncoding {
	case httputil.RequestHeaderEncodingGzip:
		gr, err := gzip.NewReader(rd)
		if err != nil {
			return nil, fmt.Errorf("failed to create gzip reader: %w", err)
		}
		defer gr.Close()

		switch op.requestContentType {
		case httputil.RequestHeaderJSON, "":
			if err := json.NewDecoder(gr).Decode(&specs); err != nil {
				return nil, fmt.Errorf("failed to decode json: %w", err)
			}
		case httputil.RequestHeaderYAML:
			b, err := io.ReadAll(gr)
			if err != nil {
				return nil, fmt.Errorf("failed to read yaml: %w", err)
			}
			if err := yaml.Unmarshal(b, &specs); err != nil {
				return nil, fmt.Errorf("failed to unmarshal yaml: %w", err)
			}
		default:
			return nil, fmt.Errorf("unsupported content type: %s", op.requestContentType)
		}

	default:
		switch op.requestContentType {
		case httputil.RequestHeaderJSON, "":
			if err := json.NewDecoder(rd).Decode(&specs); err != nil {
				return nil, fmt.Errorf("failed to decode json: %w", err)
			}
		case httputil.RequestHeaderYAML:
			b, err := io.ReadAll(rd)
			if err != nil {
				return nil, fmt.Errorf("failed to read yaml: %w", err)
			}
			if err := yaml.Unmarshal(b, &specs); err != nil {
				return nil, fmt.Errorf("failed to unmarshal yaml: %w", err)
			}
		default:
			return nil, fmt.Errorf("unsupported content type: %s", op.requestContentType)
		}
	}

	return specs, nil
}
