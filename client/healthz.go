package client

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/leptonai/gpud/internal/server"
)

func CheckHealthz(ctx context.Context, addr string, opts ...OpOption) error {
	op := &Op{}
	if err := op.applyOpts(opts); err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/healthz", addr), nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	exp, err := server.DefaultHealthz.JSON()
	if err != nil {
		return fmt.Errorf("failed to marshal expected healthz response: %w", err)
	}

	return checkHealthz(op.httpClient, req, exp)
}

func checkHealthz(cli *http.Client, req *http.Request, exp []byte) error {
	resp, err := cli.Do(req)
	if err != nil {
		return fmt.Errorf("failed to make request to /healthz: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server not ready, response not 200")
	}

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read healthz response: %w", err)
	}

	if !bytes.Equal(b, exp) {
		return fmt.Errorf("unexpected healthz response: %s", string(b))
	}

	return nil
}

func BlockUntilServerReady(ctx context.Context, addr string, opts ...OpOption) error {
	op := &Op{}
	if err := op.applyOpts(opts); err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/healthz", addr), nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	exp, err := server.DefaultHealthz.JSON()
	if err != nil {
		return fmt.Errorf("failed to marshal expected healthz response: %w", err)
	}

	ticker := time.NewTicker(op.checkInterval)
	defer ticker.Stop()
	for range 30 {
		select {
		case <-ticker.C:
			if err := checkHealthz(op.httpClient, req, exp); err == nil {
				return nil
			}
		case <-ctx.Done():
			return fmt.Errorf("context done: %w", ctx.Err())
		}
	}
	return errors.New("server not ready, timeout waiting")
}
