package netutil

import (
	"os/exec"
	"strings"
)

func PublicIP() (string, error) {
	cmd := exec.Command("curl", "-4", "ifconfig.me")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	ip := strings.TrimSpace(string(output))
	return ip, nil
}
