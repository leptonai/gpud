package netutil

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

func PublicIP() (string, error) {
	return publicIP("https://ifconfig.me")
}

func publicIP(target string) (string, error) {
	// Create a transport that forces IPv4
	transport := &http.Transport{
		// Force IPv4 by using tcp4 network
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return (&net.Dialer{
				Timeout:   5 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext(ctx, "tcp4", addr)
		},
	}

	client := &http.Client{
		Timeout:   10 * time.Second,
		Transport: transport,
	}

	// Create a request to mimic curl behavior
	req, err := http.NewRequest("GET", target, nil)
	if err != nil {
		return "", err
	}
	// Add User-Agent to ensure we get a plain response, not HTML
	req.Header.Set("User-Agent", "curl/7.64.1")

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	ip := strings.TrimSpace(string(body))
	return ip, nil
}

// IsPortOpen checks if the TCP port is open/used.
// It returns true if the port is open/used, otherwise false.
func IsPortOpen(port int) bool {
	// check if the TCP port is open/used
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("localhost:%d", port), 3*time.Second)
	if err != nil {
		return false
	}
	defer conn.Close()

	return true
}
