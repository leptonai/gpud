// Package imds provides functions for interacting with the AWS Instance Metadata Service.
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
	imdsTokenURL    = "http://169.254.169.254/latest/api/token"
	imdsMetadataURL = "http://169.254.169.254/latest/meta-data"

	headerToken = "X-aws-ec2-metadata-token"
	headerTTL   = "X-aws-ec2-metadata-token-ttl-seconds"

	defaultTokenTTL = 21600 // 6 hours in seconds
)

// FetchToken fetches a session token for instance metadata service v2.
// ref. https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/configuring-instance-metadata-service.html
// e.g., curl -X PUT "http://169.254.169.254/latest/api/token" -H "X-aws-ec2-metadata-token-ttl-seconds: 21600"
func FetchToken(ctx context.Context) (string, error) {
	return fetchToken(ctx, imdsTokenURL)
}

// fetchToken retrieves a session token from the IMDSv2 endpoint.
func fetchToken(ctx context.Context, url string) (string, error) {
	client := &http.Client{
		Timeout: 5 * time.Second, // Set a reasonable timeout
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create IMDS token request: %w", err)
	}
	req.Header.Set(headerTTL, fmt.Sprintf("%d", defaultTokenTTL))

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to fetch IMDS token: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to fetch IMDS token: received status code %d", resp.StatusCode)
	}

	tokenBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read IMDS token response body: %w", err)
	}

	return strings.TrimSpace(string(tokenBytes)), nil
}

// FetchMetadata fetches EC2 instance metadata using IMDSv2 at the specified path.
// ref. https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/instancedata-data-retrieval.html
// e.g., curl -H "X-aws-ec2-metadata-token: $TOKEN" http://169.254.169.254/latest/meta-data/instance-id
func FetchMetadata(ctx context.Context, path string) (string, error) {
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return fetchMetadataByPath(ctx, imdsTokenURL, imdsMetadataURL+path)
}

// fetchMetadataByPath retrieves EC2 instance metadata from the specified path using IMDSv2.
func fetchMetadataByPath(ctx context.Context, tokenURL string, metadataURL string) (string, error) {
	token, err := fetchToken(ctx, tokenURL)
	if err != nil {
		return "", err
	}

	client := &http.Client{
		Timeout: 5 * time.Second, // Set a reasonable timeout
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, metadataURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create metadata request: %w", err)
	}
	req.Header.Set(headerToken, token)

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to fetch metadata: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to fetch metadata: received status code %d", resp.StatusCode)
	}

	metadataBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read metadata response body: %w", err)
	}

	return strings.TrimSpace(string(metadataBytes)), nil
}

// FetchAvailabilityZone fetches EC2 instance availability zone using IMDSv2.
// ref. https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/ec2-instance-metadata.html#instancedata-data-categories
// e.g., curl -H "X-aws-ec2-metadata-token: $TOKEN" http://169.254.169.254/latest/meta-data/placement/availability-zone
func FetchAvailabilityZone(ctx context.Context) (string, error) {
	return fetchAvailabilityZone(ctx, imdsTokenURL, imdsMetadataURL)
}

// fetchAvailabilityZone retrieves EC2 instance availability zone from the specified path using IMDSv2.
func fetchAvailabilityZone(ctx context.Context, tokenURL string, metadataURL string) (string, error) {
	// ref. https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/ec2-instance-metadata.html#instancedata-data-categories
	return fetchMetadataByPath(ctx, tokenURL, metadataURL+"/placement/availability-zone")
}

// FetchPublicIPv4 fetches EC2 instance public IPv4 using IMDSv2.
// e.g., curl -H "X-aws-ec2-metadata-token: $TOKEN" http://169.254.169.254/latest/meta-data/placement/availability-zone
func FetchPublicIPv4(ctx context.Context) (string, error) {
	return fetchPublicIPv4(ctx, imdsTokenURL, imdsMetadataURL)
}

// fetchPublicIPv4 retrieves EC2 instance public IPv4 from the specified path using IMDSv2.
func fetchPublicIPv4(ctx context.Context, tokenURL string, metadataURL string) (string, error) {
	return fetchMetadataByPath(ctx, tokenURL, metadataURL+"/public-ipv4")
}

func FetchInstanceID(ctx context.Context) (string, error) {
	return fetchMetadataByPath(ctx, imdsTokenURL, imdsMetadataURL+"/instance-id")
}
