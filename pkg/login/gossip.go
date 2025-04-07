package login

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/leptonai/gpud/pkg/log"
	"github.com/leptonai/gpud/version"
)

type GossipRequest struct {
	Name          string `json:"name"`
	ID            string `json:"id"`
	Provider      string `json:"provider"`
	DaemonVersion string `json:"daemon_version"`
	Components    string `json:"components"`
}

type GossipErrorResponse struct {
	Error  string `json:"error"`
	Status string `json:"status"`
}

func createGossipURL(endpoint string) string {
	return fmt.Sprintf("https://%s/api/v1/gossip", endpoint)
}

func Gossip(uid string, endpoint string, components []string) error {
	url := createGossipURL(endpoint)
	return gossip(uid, url, components)
}

func gossip(uid string, url string, components []string) error {
	if os.Getenv("GPUD_NO_USAGE_STATS") == "true" {
		log.Logger.Debug("gossip skipped since GPUD_NO_USAGE_STATS=true specified")
		return nil
	}

	log.Logger.Infow("gossiping", "url", url, "uid", uid, "components", components)
	req := GossipRequest{
		Name:          uid,
		ID:            uid,
		Provider:      "personal",
		DaemonVersion: version.Version,
		Components:    strings.Join(components, ","),
	}
	rawPayload, err := json.Marshal(&req)
	if err != nil {
		return fmt.Errorf("error marshaling gossip request: %w", err)
	}

	resp, err := http.Post(url, "application/json", bytes.NewBuffer(rawPayload))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("error reading response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var errorResponse GossipErrorResponse
		err = json.Unmarshal(body, &errorResponse)
		if err != nil {
			return fmt.Errorf("Error parsing error response: %v\nResponse body: %s", err, body)
		}
	}
	return nil
}
