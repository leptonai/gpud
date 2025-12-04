// Package imds provides functions for interacting with the Azure Instance Metadata Service.
package imds

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/leptonai/gpud/pkg/log"
)

const (
	imdsMetadataURL = "http://169.254.169.254/metadata"

	queryKeyAPIVersion = "api-version"

	// ref. https://learn.microsoft.com/en-us/azure/virtual-machines/instance-metadata-service?tabs=linux#supported-api-versions
	// ref. https://github.com/Azure/azure-rest-api-specs/blob/main/specification/imds/data-plane/Microsoft.InstanceMetadataService/stable/2021-12-13/imds.json
	defaultAPIVersion = "2023-11-15"

	headerMetadata = "Metadata"
)

// FetchMetadata fetches Azure instance metadata using IMDS at the specified path.
// ref. https://learn.microsoft.com/en-us/azure/virtual-machines/instance-metadata-service
// ref. https://learn.microsoft.com/en-us/azure/virtual-machines/instance-metadata-service?tabs=linux#instance-metadata
// e.g., curl -s -H Metadata:true --noproxy "*" "http://169.254.169.254/metadata/instance?api-version=2023-11-15" | jq
func FetchMetadata(ctx context.Context, path string) (string, error) {
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return fetchMetadataByPath(ctx, imdsMetadataURL+path)
}

// fetchMetadataByPath retrieves Azure instance metadata from the specified path using IMDS.
// ref. https://learn.microsoft.com/en-us/azure/virtual-machines/instance-metadata-service?tabs=linux#instance-metadata
func fetchMetadataByPath(ctx context.Context, metadataURL string) (string, error) {
	client := &http.Client{
		Timeout: 5 * time.Second, // Set a reasonable timeout
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, metadataURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create metadata request: %w", err)
	}

	// ref. https://learn.microsoft.com/en-us/azure/virtual-machines/instance-metadata-service?tabs=linux#security-and-authentication
	req.Header.Set(headerMetadata, "true")

	query := req.URL.Query()
	query.Add(queryKeyAPIVersion, defaultAPIVersion)
	req.URL.RawQuery = query.Encode()

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to fetch metadata: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to fetch metadata: received status code %d", resp.StatusCode)
	}

	metadataBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read metadata response body: %w", err)
	}

	return strings.TrimSpace(string(metadataBytes)), nil
}

// FetchPublicIPv4 fetches Azure instance public IPv4 using IMDS.
func FetchPublicIPv4(ctx context.Context) (string, error) {
	return fetchPublicIPv4(ctx, imdsMetadataURL)
}

// fetchPublicIPv4 retrieves Azure instance public IPv4 from the specified path using IMDS.
func fetchPublicIPv4(ctx context.Context, metadataURL string) (string, error) {
	// Get the raw network interface metadata
	// e.g., curl -s -H Metadata:true --noproxy "*" "http://169.254.169.254/metadata/instance/network/interface?api-version=2023-11-15" | jq
	data, err := fetchMetadataByPath(ctx, metadataURL+"/instance/network/interface")
	if err != nil {
		return "", err
	}

	// Parse the JSON response
	var interfaces networkInterfacesResponse
	if err := json.Unmarshal([]byte(data), &interfaces); err != nil {
		return "", fmt.Errorf("failed to parse network interface metadata: %w", err)
	}

	// Look for a public IP address in the response
	for _, iface := range interfaces {
		for _, addr := range iface.IPv4.IPAddress {
			if addr.PublicIPAddress != "" {
				return addr.PublicIPAddress, nil
			}
		}
	}

	// no public IPv4 address found in instance metadata
	return "", nil
}

// networkInterfacesResponse represents the array of network interfaces returned by IMDS
type networkInterfacesResponse []networkInterface

type networkInterface struct {
	IPv4       IPv4Info `json:"ipv4"`
	IPv6       IPv6Info `json:"ipv6"`
	MACAddress string   `json:"macAddress"`
}

type IPv4Info struct {
	IPAddress []IPv4Address `json:"ipAddress"`
	Subnet    []SubnetInfo  `json:"subnet"`
}

type IPv4Address struct {
	PrivateIPAddress string `json:"privateIpAddress"`
	PublicIPAddress  string `json:"publicIpAddress"`
}

type SubnetInfo struct {
	Address string `json:"address"`
	Prefix  string `json:"prefix"`
}

type IPv6Info struct {
	IPAddress []IPv6Address `json:"ipAddress"`
}

type IPv6Address struct {
	PrivateIPAddress string `json:"privateIpAddress,omitempty"`
	PublicIPAddress  string `json:"publicIpAddress,omitempty"`
}

type computeResponse struct {
	AZEnvironment string `json:"azEnvironment"`
	Location      string `json:"location"`
	PhysicalZone  string `json:"physicalZone"`
	ResourceID    string `json:"resourceId"`
}

// FetchAvailabilityZone fetches Azure instance environment using IMDS.
func FetchAvailabilityZone(ctx context.Context) (string, error) {
	resp, err := fetchComputeResponse(ctx, imdsMetadataURL)
	if err != nil {
		return "", err
	}
	return resp.Location, nil
}

// FetchAZEnvironment fetches Azure instance environment using IMDS, such as "AZUREPUBLICCLOUD" or "AzurePublicCloud".
func FetchAZEnvironment(ctx context.Context) (string, error) {
	resp, err := fetchComputeResponse(ctx, imdsMetadataURL)
	if err != nil {
		return "", err
	}
	if resp.AZEnvironment != "" && !strings.Contains(strings.ToLower(resp.AZEnvironment), "azurepublic") {
		log.Logger.Warnw("unexpected Azure AZ environment", "azEnvironment", resp.AZEnvironment)
	}
	return resp.AZEnvironment, nil
}

func FetchInstanceID(ctx context.Context) (string, error) {
	resp, err := fetchComputeResponse(ctx, imdsMetadataURL)
	if err != nil {
		return "", err
	}
	return resp.ResourceID, nil
}

func fetchComputeResponse(ctx context.Context, metadataURL string) (*computeResponse, error) {
	// ref. https://learn.microsoft.com/en-us/azure/virtual-machines/instance-metadata-service?tabs=linux#instance-metadata
	// e.g., curl -s -H Metadata:true --noproxy "*" "http://169.254.169.254/metadata/instance/compute?api-version=2023-11-15" | jq
	d, err := fetchMetadataByPath(ctx, metadataURL+"/instance/compute")
	if err != nil {
		return nil, err
	}

	var compute computeResponse
	if err := json.Unmarshal([]byte(d), &compute); err != nil {
		return nil, fmt.Errorf("failed to parse compute metadata: %w", err)
	}
	return &compute, nil
}
