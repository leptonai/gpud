// Package imds provides functions for interacting with the nscale instance metadata service.
package imds

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	imdsMetadataURL          = "http://169.254.169.254/latest/meta-data"
	openStackMetadataJSONURL = "http://169.254.169.254/openstack/latest/meta_data.json"
)

type OpenStackMetadataResponse struct {
	UUID             string                `json:"uuid"`
	AvailabilityZone string                `json:"availability_zone"`
	Meta             OpenStackMetadataMeta `json:"meta"`
}

type OpenStackMetadataMeta struct {
	OrganizationID string `json:"organizationID"`
	ProjectID      string `json:"projectID"`
	RegionName     string `json:"regionName"`
	Region         string `json:"region"`
	RegionID       string `json:"regionID"`
	RegionId       string `json:"regionId"`
}

// BestRegion returns the most human-readable region value available.
// Only name-like fields are considered.
// regionID/regionId are intentionally excluded because they are opaque IDs.
// On current nscale nodes, metadata commonly exposes regionID without regionName.
// In that case we return empty so callers can keep their own region fallback
// (e.g., DERP/latency-based region selection).
func (m OpenStackMetadataMeta) BestRegion() string {
	if s := strings.TrimSpace(m.RegionName); s != "" {
		return s
	}
	if s := strings.TrimSpace(m.Region); s != "" {
		return s
	}
	return ""
}

// FetchMetadata fetches metadata from the nscale metadata service at the specified path.
// e.g., http://169.254.169.254/latest/meta-data/instance-id
func FetchMetadata(ctx context.Context, path string) (string, error) {
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return fetchMetadataByPath(ctx, imdsMetadataURL+path)
}

func fetchMetadataByPath(ctx context.Context, metadataURL string) (string, error) {
	s, _, err := fetchMetadataByPathWithStatusCode(ctx, metadataURL)
	if err != nil {
		return "", err
	}
	return s, nil
}

func fetchMetadataByPathWithStatusCode(ctx context.Context, metadataURL string) (string, int, error) {
	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, metadataURL, nil)
	if err != nil {
		return "", 0, fmt.Errorf("failed to create metadata request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", 0, fmt.Errorf("failed to fetch metadata: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		return "", resp.StatusCode, fmt.Errorf("failed to fetch metadata: received status code %d", resp.StatusCode)
	}

	metadataBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", resp.StatusCode, fmt.Errorf("failed to read metadata response body: %w", err)
	}

	return strings.TrimSpace(string(metadataBytes)), resp.StatusCode, nil
}

func FetchPublicIPv4(ctx context.Context) (string, error) {
	return fetchPublicIPv4(ctx, imdsMetadataURL)
}

func fetchPublicIPv4(ctx context.Context, metadataURL string) (string, error) {
	s, code, err := fetchMetadataByPathWithStatusCode(ctx, metadataURL+"/public-ipv4")
	if err != nil {
		if code == http.StatusNotFound {
			return "", nil
		}
		return "", err
	}
	return s, nil
}

func FetchLocalIPv4(ctx context.Context) (string, error) {
	return fetchMetadataByPath(ctx, imdsMetadataURL+"/local-ipv4")
}

func FetchInstanceID(ctx context.Context) (string, error) {
	return fetchMetadataByPath(ctx, imdsMetadataURL+"/instance-id")
}

func FetchOpenStackMetadata(ctx context.Context) (*OpenStackMetadataResponse, error) {
	return fetchOpenStackMetadata(ctx, openStackMetadataJSONURL)
}

func fetchOpenStackMetadata(ctx context.Context, metadataURL string) (*OpenStackMetadataResponse, error) {
	d, err := fetchMetadataByPath(ctx, metadataURL)
	if err != nil {
		return nil, err
	}

	resp := &OpenStackMetadataResponse{}
	if err := json.Unmarshal([]byte(d), resp); err != nil {
		return nil, fmt.Errorf("failed to parse OpenStack metadata: %w", err)
	}
	return resp, nil
}
