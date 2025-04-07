package login

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/leptonai/gpud/pkg/config"
	"github.com/leptonai/gpud/pkg/log"
	"github.com/leptonai/gpud/pkg/netutil"
)

// Login calls PublicIP and then delegates to the login function
func Login(name string, token string, endpoint string, components string, uid string) error {
	url := createLoginURL(endpoint)
	return login(name, token, url, components, uid, netutil.PublicIP)
}

func createLoginURL(endpoint string) string {
	return fmt.Sprintf("https://%s/api/v1/login", endpoint)
}

type LoginRequest struct {
	Name       string `json:"name"`
	ID         string `json:"id"`
	PublicIP   string `json:"public_ip"`
	Provider   string `json:"provider"`
	Components string `json:"components"`
	Token      string `json:"token"`
}

type LoginErrorResponse struct {
	Error  string `json:"error"`
	Status string `json:"status"`
}

// login is a private function that takes publicIP as a parameter for better testability
func login(name string, token string, url string, components string, uid string, getPublicIP func() (string, error)) error {
	publicIP, err := getPublicIP()
	if err != nil {
		return fmt.Errorf("failed to fetch public ip: %w", err)
	}

	log.Logger.Infow("logging in", "url", url, "uid", uid, "publicIP", publicIP)
	req := LoginRequest{
		Name:       name,
		ID:         uid,
		PublicIP:   publicIP,
		Provider:   "personal",
		Components: components,
		Token:      token,
	}
	rawPayload, err := json.Marshal(&req)
	if err != nil {
		return fmt.Errorf("error marshaling login request: %w", err)
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
		var errorResponse LoginErrorResponse
		err = json.Unmarshal(body, &errorResponse)
		if err != nil {
			return fmt.Errorf("Error parsing error response: %v\nResponse body: %s", err, body)
		}
		if strings.Contains(errorResponse.Status, "invalid workspace token") {
			return fmt.Errorf("invalid token provided, please use the workspace token under Setting/Tokens and execute\n    gpud login --token yourToken")
		}
		return fmt.Errorf("\nCurrently, we only support machines with a public IP address. Please ensure that your public IP and port combination (%s:%d) is reachable.\nerror: %v", publicIP, config.DefaultGPUdPort, errorResponse)
	}
	return nil
}
