// Package v1 provides the gpud v1 client for the server.
package v1

import (
	"crypto/tls"
	"net/http"
	"time"

	"github.com/leptonai/gpud/internal/server"
)

type Op struct {
	httpClient            *http.Client
	checkInterval         time.Duration
	requestContentType    string
	requestAcceptEncoding string
	components            map[string]any
}

type OpOption func(*Op)

func (op *Op) applyOpts(opts []OpOption) error {
	for _, opt := range opts {
		opt(op)
	}

	if op.httpClient == nil {
		op.httpClient = &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			},
		}
	}

	if op.checkInterval == 0 {
		op.checkInterval = time.Second
	}

	return nil
}

func WithHTTPClient(cli *http.Client) OpOption {
	return func(op *Op) {
		op.httpClient = cli
	}
}

func WithCheckInterval(interval time.Duration) OpOption {
	return func(op *Op) {
		op.checkInterval = interval
	}
}

// WithRequestContentTypeYAML sets the request content type to YAML.
func WithRequestContentTypeYAML() OpOption {
	return func(op *Op) {
		op.requestContentType = server.RequestHeaderYAML
	}
}

// WithRequestContentTypeJSON sets the request content type to JSON.
func WithRequestContentTypeJSON() OpOption {
	return func(op *Op) {
		op.requestContentType = server.RequestHeaderJSON
	}
}

// WithAcceptEncodingGzip requests gzip encoding for the response.
func WithAcceptEncodingGzip() OpOption {
	return func(op *Op) {
		op.requestAcceptEncoding = server.RequestHeaderEncodingGzip
	}
}

func WithComponent(component string) OpOption {
	return func(op *Op) {
		if op.components == nil {
			op.components = make(map[string]any)
		}
		op.components[component] = nil
	}
}
