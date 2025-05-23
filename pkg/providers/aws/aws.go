package aws

import (
	"context"

	"github.com/leptonai/gpud/pkg/providers"
	"github.com/leptonai/gpud/pkg/providers/aws/imds"
)

const Name = "aws"

func New() providers.Detector {
	return providers.New(Name, detectProvider, imds.FetchPublicIPv4, nil)
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
