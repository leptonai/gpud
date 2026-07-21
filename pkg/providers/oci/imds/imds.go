// Package imds provides functions for interacting with the OCI Instance Metadata Service v2.
package imds

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	imdsMetadataURL = "http://169.254.169.254/opc/v2"

	headerAuthorization = "Authorization"
	bearerOracle        = "Bearer Oracle"
)

// FetchPrimaryVNICPrivateIPv4 fetches the primary VNIC's primary private IPv4 address.
// ref. https://docs.oracle.com/en-us/iaas/Content/Compute/Tasks/gettingmetadata.htm
// e.g., curl -H "Authorization: Bearer Oracle" http://169.254.169.254/opc/v2/vnics/0/privateIp
func FetchPrimaryVNICPrivateIPv4(ctx context.Context) (string, error) {
	return fetchPrimaryVNICPrivateIPv4(ctx, imdsMetadataURL)
}

func fetchPrimaryVNICPrivateIPv4(ctx context.Context, metadataURL string) (string, error) {
	return fetchMetadataByPath(ctx, metadataURL+"/vnics/0/privateIp")
}

func fetchMetadataByPath(ctx context.Context, metadataURL string) (string, error) {
	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, metadataURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create OCI metadata request: %w", err)
	}
	req.Header.Set(headerAuthorization, bearerOracle)

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to fetch OCI metadata: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to fetch OCI metadata: received status code %d", resp.StatusCode)
	}

	metadataBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read OCI metadata response body: %w", err)
	}

	return strings.TrimSpace(string(metadataBytes)), nil
}
