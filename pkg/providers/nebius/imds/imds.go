// Package imds provides functions for interacting with the Nebius Instance Metadata Service.
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
	// ref. https://docs.nebius.com/compute/virtual-machines/instance-metadata
	imdsMetadataURL = "http://metadata.nebius.internal/v1"

	headerMetadata = "Metadata"
	metadataTrue   = "true"

	maxMetadataRetries     = 3
	metadataRetryBaseDelay = 100 * time.Millisecond
)

// InstanceData contains the fields returned by Nebius instance-data IMDS.
type InstanceData struct {
	ID                     string            `json:"id"`
	ParentID               string            `json:"parent_id"`
	Name                   string            `json:"name"`
	Hostname               string            `json:"hostname"`
	Platform               string            `json:"platform"`
	Preset                 string            `json:"preset"`
	Labels                 map[string]string `json:"labels"`
	ResourceVersion        int64             `json:"resource_version"`
	CreatedAt              string            `json:"created_at"`
	ServiceAccountID       string            `json:"service_account_id"`
	GPUClusterID           string            `json:"gpu_cluster_id"`
	InfinibandFabric       string            `json:"infiniband_fabric"`
	InfinibandTopologyPath []string          `json:"infiniband_topology_path"`
	Region                 string            `json:"region"`
}

// FetchInstanceData fetches the Nebius VM instance metadata.
func FetchInstanceData(ctx context.Context) (*InstanceData, error) {
	return fetchInstanceData(ctx, imdsMetadataURL)
}

func fetchInstanceData(ctx context.Context, metadataURL string) (*InstanceData, error) {
	data, err := fetchMetadataByPath(ctx, metadataURL+"/instance-data")
	if err != nil {
		return nil, err
	}

	var instanceData InstanceData
	if err := json.Unmarshal([]byte(data), &instanceData); err != nil {
		return nil, fmt.Errorf("failed to parse Nebius instance metadata: %w", err)
	}
	return &instanceData, nil
}

// FetchRegion fetches the Nebius region identifier.
func FetchRegion(ctx context.Context) (string, error) {
	return fetchMetadataByPath(ctx, imdsMetadataURL+"/instance-data/region")
}

func fetchMetadataByPath(ctx context.Context, metadataURL string) (string, error) {
	client := &http.Client{Timeout: 5 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, metadataURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create Nebius metadata request: %w", err)
	}
	req.Header.Set(headerMetadata, metadataTrue)

	for attempt := 0; ; attempt++ {
		resp, err := client.Do(req)
		if err != nil {
			return "", fmt.Errorf("failed to fetch Nebius metadata: %w", err)
		}

		if resp.StatusCode == http.StatusOK {
			metadataBytes, readErr := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			if readErr != nil {
				return "", fmt.Errorf("failed to read Nebius metadata response body: %w", readErr)
			}
			return strings.TrimSpace(string(metadataBytes)), nil
		}

		statusCode := resp.StatusCode
		_ = resp.Body.Close()
		if !retryableStatus(statusCode) || attempt == maxMetadataRetries {
			return "", fmt.Errorf("failed to fetch Nebius metadata: received status code %d", statusCode)
		}

		select {
		case <-ctx.Done():
			return "", fmt.Errorf("failed to fetch Nebius metadata: %w", ctx.Err())
		case <-time.After(metadataRetryBaseDelay << attempt):
		}
	}
}

func retryableStatus(statusCode int) bool {
	return statusCode == http.StatusTooManyRequests ||
		statusCode == http.StatusInternalServerError ||
		statusCode == http.StatusServiceUnavailable
}
