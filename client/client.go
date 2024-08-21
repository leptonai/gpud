// Package client provides the gpud client for the server.
package client

import (
	"crypto/tls"
	"net/http"
	"time"
)

type Op struct {
	httpClient    *http.Client
	checkInterval time.Duration
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
