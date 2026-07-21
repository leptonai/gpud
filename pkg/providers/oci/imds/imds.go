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

	maxMetadataRetries     = 3
	metadataRetryBaseDelay = 100 * time.Millisecond
)

// FetchInstanceID fetches the OCI instance OCID.
func FetchInstanceID(ctx context.Context) (string, error) {
	return fetchInstanceID(ctx, imdsMetadataURL)
}

func fetchInstanceID(ctx context.Context, metadataURL string) (string, error) {
	return fetchMetadataByPath(ctx, metadataURL+"/instance/id")
}

// FetchCanonicalRegionName fetches the canonical OCI region identifier.
func FetchCanonicalRegionName(ctx context.Context) (string, error) {
	return fetchCanonicalRegionName(ctx, imdsMetadataURL)
}

func fetchCanonicalRegionName(ctx context.Context, metadataURL string) (string, error) {
	return fetchMetadataByPath(ctx, metadataURL+"/instance/canonicalRegionName")
}

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

	for attempt := 0; ; attempt++ {
		resp, err := client.Do(req)
		if err != nil {
			return "", fmt.Errorf("failed to fetch OCI metadata: %w", err)
		}

		if resp.StatusCode == http.StatusOK {
			metadataBytes, readErr := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			if readErr != nil {
				return "", fmt.Errorf("failed to read OCI metadata response body: %w", readErr)
			}
			return strings.TrimSpace(string(metadataBytes)), nil
		}

		statusCode := resp.StatusCode
		_ = resp.Body.Close()
		if !retryableStatus(statusCode) || attempt == maxMetadataRetries {
			return "", fmt.Errorf("failed to fetch OCI metadata: received status code %d", statusCode)
		}

		select {
		case <-ctx.Done():
			return "", fmt.Errorf("failed to fetch OCI metadata: %w", ctx.Err())
		case <-time.After(metadataRetryBaseDelay << attempt):
		}
	}
}

func retryableStatus(statusCode int) bool {
	return statusCode == http.StatusNotFound ||
		statusCode == http.StatusTooManyRequests ||
		(statusCode >= http.StatusInternalServerError && statusCode < 600)
}
