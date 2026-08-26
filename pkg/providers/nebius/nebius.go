package nebius

import (
	"context"
	"errors"

	"github.com/leptonai/gpud/pkg/providers"
	"github.com/leptonai/gpud/pkg/providers/nebius/imds"
)

const Name = "nebius"

func New() providers.Detector {
	return providers.NewIMDSWithRegion(Name, detectProvider, nil, nil, imds.FetchRegion, nil, GetInstanceID)
}

func detectProvider(ctx context.Context) (string, error) {
	region, err := imds.FetchRegion(ctx)
	if err != nil {
		return "", err
	}
	if region != "" {
		return Name, nil
	}
	return "", nil
}

// GetInstanceID fetches the Nebius VM identity from HTTP IMDS.
func GetInstanceID(ctx context.Context) (string, error) {
	data, err := imds.FetchInstanceData(ctx)
	if err != nil {
		return "", err
	}
	if data.ID == "" {
		return "", errors.New("nebius instance metadata is missing id")
	}
	return data.ID, nil
}
