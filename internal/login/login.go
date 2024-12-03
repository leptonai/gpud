package login

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"

	"github.com/leptonai/gpud/config"
	"github.com/leptonai/gpud/log"
	"github.com/leptonai/gpud/version"
)

func Login(name string, token string, endpoint string, components string, uid string) error {
	ip, err := PublicIP()
	if err != nil {
		return fmt.Errorf("failed to fetch public ip: %w", err)
	}

	type payload struct {
		Name       string `json:"name"`
		ID         string `json:"id"`
		PublicIP   string `json:"public_ip"`
		Provider   string `json:"provider"`
		Components string `json:"components"`
		Token      string `json:"token"`
	}
	type RespErr struct {
		Error  string `json:"error"`
		Status string `json:"status"`
	}
	content := payload{
		Name:       name,
		ID:         uid,
		PublicIP:   ip,
		Provider:   "personal",
		Components: components,
		Token:      token,
	}
	rawPayload, _ := json.Marshal(&content)
	response, err := http.Post(fmt.Sprintf("https://%s/api/v1/login", endpoint), "application/json", bytes.NewBuffer(rawPayload))
	if err != nil {
		return err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return fmt.Errorf("error reading response body: %w", err)
	}
	if response.StatusCode != http.StatusOK {
		var errorResponse RespErr
		err = json.Unmarshal(body, &errorResponse)
		if err != nil {
			return fmt.Errorf("Error parsing error response: %v\nResponse body: %s", err, body)
		}
		if strings.Contains(errorResponse.Status, "invalid workspace token") {
			return fmt.Errorf("invalid token provided, please use the workspace token under Setting/Tokens and execute\n    gpud login --token yourToken")
		}
		return fmt.Errorf("\nCurrently, we only support machines with a public IP address. Please ensure that your public IP and port combination (%s:%d) is reachable.\nerror: %v", ip, config.DefaultGPUdPort, errorResponse)
	}
	return nil
}

func Gossip(endpoint string, uid string, address string, components []string) error {
	if os.Getenv("GPUD_NO_USAGE_STATS") == "true" {
		log.Logger.Debug("gossip skipped since GPUD_NO_USAGE_STATS=true specified")
		return nil
	}

	type payload struct {
		Name          string `json:"name"`
		ID            string `json:"id"`
		Provider      string `json:"provider"`
		DaemonVersion string `json:"daemon_version"`
		Components    string `json:"components"`
	}
	type RespErr struct {
		Error  string `json:"error"`
		Status string `json:"status"`
	}
	content := payload{
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
		var errorResponse RespErr
		err = json.Unmarshal(body, &errorResponse)
		if err != nil {
			return fmt.Errorf("Error parsing error response: %v\nResponse body: %s", err, body)
		}
	}
	return nil
}

func PublicIP() (string, error) {
	cmd := exec.Command("curl", "-4", "ifconfig.me")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	ip := strings.TrimSpace(string(output))
	return ip, nil
}
