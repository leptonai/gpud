// Package oci implements the Oracle Cloud Infrastructure provider detector.
package oci

import (
	"context"
	"net/netip"

	"github.com/leptonai/gpud/pkg/providers"
	"github.com/leptonai/gpud/pkg/providers/oci/imds"
)

const Name = "oci"

func New() providers.Detector {
	return providers.NewWithRegion(
		Name,
		detectProvider,
		nil,
		fetchPrivateIPv4,
		imds.FetchCanonicalRegionName,
		nil,
		imds.FetchInstanceID,
	)
}

func detectProvider(ctx context.Context) (string, error) {
	instanceID, err := imds.FetchInstanceID(ctx)
	if err != nil {
		return "", err
	}
	if instanceID != "" {
		return Name, nil
	}
	return "", nil
}

func fetchPrivateIPv4(ctx context.Context) (string, error) {
	addr, err := imds.FetchPrimaryVNICPrivateIPv4(ctx)
	if err != nil {
		return "", err
	}

	ip, err := netip.ParseAddr(addr)
	if err != nil || !ip.Is4() {
		return "", nil
	}
	return ip.String(), nil
}
