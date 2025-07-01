package gcp

import (
	"context"

	"github.com/leptonai/gpud/pkg/providers"
	"github.com/leptonai/gpud/pkg/providers/gcp/imds"
)

const Name = "gcp"

func New() providers.Detector {
	return providers.New(Name, detectProvider, imds.FetchPublicIPv4, nil, nil, imds.FetchInstanceID)
}

func detectProvider(ctx context.Context) (string, error) {
	zone, err := imds.FetchAvailabilityZone(ctx)
	if err != nil {
		return "", err
	}
	if zone != "" {
		return Name, nil
	}
	return "", nil
}
