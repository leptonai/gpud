package gossip

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/pkg/log"
)

// SendRequest sends a gossip request.
func SendRequest(ctx context.Context, endpoint string, req apiv1.GossipRequest) (*apiv1.GossipResponse, error) {
	url := createURL(endpoint)
	return sendRequest(ctx, url, req)
}

func sendRequest(ctx context.Context, url string, req apiv1.GossipRequest) (*apiv1.GossipResponse, error) {
	if os.Getenv("GPUD_NO_USAGE_STATS") == "true" {
		log.Logger.Debug("gossip skipped since GPUD_NO_USAGE_STATS=true specified")
		return nil, nil
	}

	log.Logger.Debugw("sending gossip request", "url", url)

	b, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("error marshaling gossip request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(b))
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	httpResp, err := client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer httpResp.Body.Close()

	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response body: %w", err)
	}

	var resp apiv1.GossipResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("error unmarshaling gossip response: %w", err)
	}

	if httpResp.StatusCode != http.StatusOK {
		return &resp, fmt.Errorf("login request failed with status code %d (%+v)", httpResp.StatusCode, resp)
	}

	log.Logger.Debugw("gossip request processed", "data", string(b), "url", url)
	return &resp, nil
}

// createURL creates a URL for the gossip endpoint
func createURL(endpoint string) string {
	host := endpoint
	url, _ := url.Parse(endpoint)
	if url.Host != "" {
		host = url.Host
	}
	return fmt.Sprintf("https://%s/api/v1/gossip", host)
}
