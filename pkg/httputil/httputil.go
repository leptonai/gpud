// Package httputil provides utilities for HTTP requests.
package httputil

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
)

// CreateURL creates a URL from a scheme, host, and path.
// If the scheme is empty, it will be inferred from the endpoint.
// If the endpoint does not have a scheme, it will default to "http".
// If the endpoint does not have a host, it will default to "localhost".
func CreateURL(scheme string, endpoint string, path string) (string, error) {
	if strings.HasPrefix(endpoint, ":") {
		// e.g., ":12345" becomes "localhost:12345"
		endpoint = "localhost" + endpoint
	}
	if scheme == "" {
		scheme = "http"
	}
	if !strings.HasPrefix(endpoint, scheme+"://") {
		endpoint = scheme + "://" + endpoint
	}

	url, err := url.Parse(endpoint)
	if err != nil {
		return "", err
	}
	if url == nil {
		return "", errors.New("invalid endpoint")
	}

	hostWithoutScheme := endpoint
	if url.Host != "" {
		hostWithoutScheme = url.Host
	}

	return strings.TrimSpace(fmt.Sprintf("%s://%s%s", scheme, hostWithoutScheme, path)), nil
}
