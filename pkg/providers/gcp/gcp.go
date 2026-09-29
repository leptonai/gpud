package gcp

import (
	"context"
	"net/netip"

	"github.com/leptonai/gpud/pkg/providers"
	"github.com/leptonai/gpud/pkg/providers/gcp/imds"
)

const Name = "gcp"

func New() providers.Detector {
	return providers.NewIMDSWithRegion(Name, detectProvider, imds.FetchPublicIPv4, fetchPrivateIPv4, imds.FetchRegion, nil, imds.FetchInstanceID)
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

// fetchPrivateIPv4 returns nic0's VPC-internal IPv4. It is authoritative even
// when the subnet uses a non-RFC1918 range, which the local NIC scan rejects.
func fetchPrivateIPv4(ctx context.Context) (string, error) {
	addr, err := imds.FetchPrimaryPrivateIPv4(ctx)
	if err != nil {
		return "", err
	}

	ip, err := netip.ParseAddr(addr)
	if err != nil || !ip.Is4() {
		return "", nil
	}
	return ip.String(), nil
}
