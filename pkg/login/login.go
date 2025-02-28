package login

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/leptonai/gpud/pkg/config"
	"github.com/leptonai/gpud/pkg/netutil"
)

// Login calls PublicIP and then delegates to the login function
func Login(name string, token string, endpoint string, components string, uid string) error {
	ip, err := netutil.PublicIP()
	if err != nil {
		return fmt.Errorf("failed to fetch public ip: %w", err)
	}
	return login(name, token, endpoint, components, uid, ip)
}

type loginPayload struct {
	Name       string `json:"name"`
	ID         string `json:"id"`
	PublicIP   string `json:"public_ip"`
	Provider   string `json:"provider"`
	Components string `json:"components"`
	Token      string `json:"token"`
}

type loginRespErr struct {
	Error  string `json:"error"`
	Status string `json:"status"`
}

// login is a private function that takes publicIP as a parameter for better testability
func login(name string, token string, endpoint string, components string, uid string, publicIP string) error {
	content := loginPayload{
		Name:       name,
		ID:         uid,
		PublicIP:   publicIP,
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
		var errorResponse loginRespErr
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
