// Package imds provides functions for interacting with the Google Cloud Platform Instance Metadata Service.
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
	// ref. https://cloud.google.com/compute/docs/metadata/predefined-metadata-keys
	imdsMetadataURL = "http://metadata.google.internal/computeMetadata/v1"

	headerMetadataFlavor = "Metadata-Flavor"
	metadataFlavorGoogle = "Google"
)

// FetchMetadata fetches Google Cloud instance metadata using IMDS at the specified path.
// ref. https://cloud.google.com/compute/docs/metadata/predefined-metadata-keys
// ref. https://cloud.google.com/compute/docs/metadata/querying-metadata
// e.g., curl "http://metadata.google.internal/computeMetadata/v1/instance/image" -H "Metadata-Flavor: Google"
func FetchMetadata(ctx context.Context, path string) (string, error) {
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return fetchMetadataByPath(ctx, imdsMetadataURL+path)
}

// fetchMetadataByPath retrieves Google Cloud instance metadata from the specified path using IMDS.
// ref. https://cloud.google.com/compute/docs/metadata/querying-metadata
func fetchMetadataByPath(ctx context.Context, metadataURL string) (string, error) {
	client := &http.Client{
		Timeout: 5 * time.Second, // Set a reasonable timeout
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, metadataURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create metadata request: %w", err)
	}

	// ref. https://cloud.google.com/compute/docs/metadata/querying-metadata#rest
	req.Header.Set(headerMetadataFlavor, metadataFlavorGoogle)

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

// FetchAvailabilityZone fetches Google Cloud instance availability zone using IMDS.
func FetchAvailabilityZone(ctx context.Context) (string, error) {
	return fetchAvailabilityZone(ctx, imdsMetadataURL)
}

// fetchAvailabilityZone retrieves Google Cloud instance availability zone from the specified path using IMDS.
func fetchAvailabilityZone(ctx context.Context, metadataURL string) (string, error) {
	// ref. https://cloud.google.com/compute/docs/metadata/predefined-metadata-keys#instance-metadata
	zone, err := fetchMetadataByPath(ctx, metadataURL+"/instance/zone")
	if err != nil {
		return "", err
	}
	return extractZoneFromPath(zone), nil
}

// extractZoneFromPath extracts the zone from a full zone path string.
// For example, "projects/980931390107/zones/us-east5-c" becomes "us-east5-c".
// If the path does not contain '/', it returns the original path.
func extractZoneFromPath(zonePath string) string {
	parts := strings.Split(zonePath, "/")
	return parts[len(parts)-1]
}

// FetchPublicIPv4 fetches Google Cloud instance public IPv4 using IMDS.
func FetchPublicIPv4(ctx context.Context) (string, error) {
	return fetchPublicIPv4(ctx, imdsMetadataURL)
}

// fetchPublicIPv4 retrieves Google Cloud instance public IPv4 from the specified path using IMDS.
func fetchPublicIPv4(ctx context.Context, metadataURL string) (string, error) {
	// ref. https://cloud.google.com/compute/docs/metadata/predefined-metadata-keys#instance-metadata
	// ref. https://cloud.google.com/compute/docs/metadata/querying-metadata (recursive example)
	data, err := fetchMetadataByPath(ctx, metadataURL+"/instance/network-interfaces/?recursive=true")
	if err != nil {
		return "", err
	}

	// Parse the JSON response which is an array of network interface objects
	var interfaces []gcpNetworkInterface
	if err := json.Unmarshal([]byte(data), &interfaces); err != nil {
		return "", fmt.Errorf("failed to parse network interface metadata: %w", err)
	}

	// Look for a public IP address in the response
	for _, iface := range interfaces {
		for _, ac := range iface.AccessConfigs {
			if ac.ExternalIP != "" {
				return ac.ExternalIP, nil
			}
		}
	}

	// no public IPv4 address found in instance metadata
	return "", nil
}

// gcpNetworkInterface represents a single network interface as returned by GCP IMDS.
// Based on the structure from instance/network-interfaces/?recursive=true
type gcpNetworkInterface struct {
	AccessConfigs []gcpAccessConfig `json:"accessConfigs"`
	// Other fields like 'ip', 'mac', 'name' can be added here if needed.
}

// gcpAccessConfig represents an access configuration for a GCP network interface.
type gcpAccessConfig struct {
	ExternalIP string `json:"externalIp"`
	Type       string `json:"type"`
}
