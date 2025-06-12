package netutil

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/leptonai/gpud/pkg/log"
)

func PublicIP() (string, error) {
	var ip string
	var err error
	for _, url := range publicIPDiscoverURLs {
		ip, err = discoverPublicIP(url)
		if err == nil {
			break
		}
		log.Logger.Warnw("failed to discover public IP", "url", url, "error", err)
	}
	return ip, err
}

var publicIPDiscoverURLs = []string{
	"https://ifconfig.me/ip",
	"https://icanhazip.com",
	// "https://ifconfig.io/ip",
}

func discoverPublicIP(url string) (string, error) {
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

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "curl/7.64.1")

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	ip := strings.TrimSpace(string(body))

	// check if ip is valid
	parsed := net.ParseIP(ip)
	if len(parsed) == 0 {
		return "", fmt.Errorf("invalid IP address: %s", ip)
	}

	return ip, nil
}
