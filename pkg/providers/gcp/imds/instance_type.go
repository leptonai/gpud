package imds

import (
	"context"
	"fmt"
	"strings"
)

// FetchInstanceType returns the machine-type name without its GCP resource path.
func FetchInstanceType(ctx context.Context) (string, error) {
	return fetchInstanceType(ctx, imdsMetadataURL)
}

func fetchInstanceType(ctx context.Context, metadataURL string) (string, error) {
	value, err := fetchMetadataByPath(ctx, metadataURL+"/instance/machine-type")
	if err != nil {
		return "", err
	}
	// IMDS returns projects/PROJECT/machineTypes/TYPE, not a zone or instance ID.
	if strings.Contains(value, "/") {
		prefix, name, ok := strings.Cut(value, "/machineTypes/")
		if !ok || prefix == "" || name == "" || strings.Contains(name, "/") {
			return "", fmt.Errorf("invalid machine-type resource path %q", value)
		}
		return name, nil
	}
	return value, nil
}
