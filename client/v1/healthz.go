package v1

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

var ErrServerNotReady = errors.New("server not ready, timeout waiting")

func CheckHealthz(ctx context.Context, addr string, opts ...OpOption) error {
	op := &Op{}
	if err := op.applyOpts(opts); err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/healthz", addr), nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	return checkHealthz(createDefaultHTTPClient(), req)
}

func checkHealthz(cli *http.Client, req *http.Request) error {
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

	var healthz Healthz
	if err := json.Unmarshal(b, &healthz); err != nil {
		return fmt.Errorf("failed to unmarshal healthz response: %w", err)
	}

	if healthz.Status != "ok" {
		return errors.New("server not ready, status is not ok")
	}

	return nil
}

// copied from "github.com/leptonai/gpud/pkg/server.Healthz"
// to avoid import cycle
type Healthz struct {
	Status  string `json:"status"`
	Version string `json:"version"`
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

	httpClient := createDefaultHTTPClient()

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for range 30 {
		select {
		case <-ticker.C:
			if err := checkHealthz(httpClient, req); err == nil {
				return nil
			}
		case <-ctx.Done():
			return fmt.Errorf("context done: %w", ctx.Err())
		}
	}
	return ErrServerNotReady
}
