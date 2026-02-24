package nscale

import (
	"context"
	"net/netip"

	"github.com/leptonai/gpud/pkg/providers"
	"github.com/leptonai/gpud/pkg/providers/nscale/imds"
)

const Name = "nscale"

func New() providers.Detector {
	return providers.New(Name, detectProvider, imds.FetchPublicIPv4, fetchPrivateIPv4, fetchVMEnvironment, imds.FetchInstanceID)
}

func detectProvider(ctx context.Context) (string, error) {
	resp, err := imds.FetchOpenStackMetadata(ctx)
	if err != nil {
		return "", err
	}
	if resp.UUID == "" {
		return "", nil
	}

	// nscale OpenStack metadata includes both org/project identifiers.
	if resp.Meta.OrganizationID == "" || resp.Meta.ProjectID == "" {
		return "", nil
	}

	return Name, nil
}

func fetchPrivateIPv4(ctx context.Context) (string, error) {
	addr, err := imds.FetchLocalIPv4(ctx)
	if err != nil {
		return "", err
	}

	ip, err := netip.ParseAddr(addr)
	if err != nil {
		return "", nil
	}
	// On nscale, local-ipv4 is the authoritative host-local source IP and may be routable.
	if !ip.Is4() {
		return "", nil
	}

	return ip.String(), nil
}

func fetchVMEnvironment(ctx context.Context) (string, error) {
	resp, err := imds.FetchOpenStackMetadata(ctx)
	if err != nil {
		return "", err
	}
	return resp.AvailabilityZone, nil
}
