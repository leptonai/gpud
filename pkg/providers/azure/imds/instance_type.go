package imds

import (
	"context"
	"strings"
)

// FetchInstanceType returns the Azure VM size from IMDS.
func FetchInstanceType(ctx context.Context) (string, error) {
	return fetchInstanceType(ctx, imdsMetadataURL)
}

func fetchInstanceType(ctx context.Context, metadataURL string) (string, error) {
	compute, err := fetchComputeResponse(ctx, metadataURL)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(compute.VMSize), nil
}
