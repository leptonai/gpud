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

type gossipPayload struct {
	Name          string `json:"name"`
	ID            string `json:"id"`
	Provider      string `json:"provider"`
	DaemonVersion string `json:"daemon_version"`
	Components    string `json:"components"`
}
type gossipRespErr struct {
	Error  string `json:"error"`
	Status string `json:"status"`
}

func Gossip(endpoint string, uid string, address string, components []string) error {
	if os.Getenv("GPUD_NO_USAGE_STATS") == "true" {
		log.Logger.Debug("gossip skipped since GPUD_NO_USAGE_STATS=true specified")
		return nil
	}

	content := gossipPayload{
		Name:          uid,
		ID:            uid,
		Provider:      "personal",
		DaemonVersion: version.Version,
		Components:    strings.Join(components, ","),
	}
	rawPayload, _ := json.Marshal(&content)
	response, err := http.Post(fmt.Sprintf("https://%s/api/v1/gossip", endpoint), "application/json", bytes.NewBuffer(rawPayload))
	if err != nil {
		return err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return fmt.Errorf("error reading response body: %w", err)
	}
	if response.StatusCode != http.StatusOK {
		var errorResponse gossipRespErr
		err = json.Unmarshal(body, &errorResponse)
		if err != nil {
			return fmt.Errorf("Error parsing error response: %v\nResponse body: %s", err, body)
		}
	}
	return nil
}
