package nscale

import (
	"context"
	"fmt"
	"net"
	"net/netip"

	"github.com/leptonai/gpud/pkg/log"
	"github.com/leptonai/gpud/pkg/providers"
	"github.com/leptonai/gpud/pkg/providers/nscale/imds"
)

const Name = "nscale"

// New returns a Detector that implements RegionDetector.
// It uses the OpenStack metadata JSON to detect the nscale provider
// and to read the provider region.
// Every other provider (AWS, GCP, Azure, OCI, Nebius) already uses
// NewWithRegion. This change keeps nscale consistent with the rest
// of the provider registry.
// ref. https://docs.openstack.org/nova/latest/user/metadata.html
func New() providers.Detector {
	return providers.NewIMDSWithRegion(
		Name,
		detectProvider,
		imds.FetchPublicIPv4,
		fetchPrivateIPv4,
		imds.FetchRegion,
		fetchVMEnvironment,
		imds.FetchInstanceID,
	)
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

// imdsHost is the link-local address of the nscale metadata service.
// It is only used to resolve the host's source IPv4 toward the service
// (no traffic is sent) when the metadata itself is unavailable.
const imdsHost = "169.254.169.254"

func fetchPrivateIPv4(ctx context.Context) (string, error) {
	addr, err := imds.FetchLocalIPv4(ctx)
	if err == nil {
		ip, perr := netip.ParseAddr(addr)
		if perr == nil && ip.Is4() {
			// On nscale, local-ipv4 is the authoritative host-local source IP and may be routable.
			return ip.String(), nil
		}
	}

	// The metadata service intermittently does not answer local-ipv4 in time
	// (e.g., the first connection after an idle period may exceed the client
	// timeout), so fall back to the source address the host routing stack
	// would use to reach the metadata service. On nscale that source is the
	// fabric IP of the primary interface and may be publicly routable — that
	// is expected and matches what local-ipv4 would have returned.
	src, serr := routeSourceIPv4()
	if serr != nil {
		log.Logger.Warnw("failed to get route source address as private IP fallback", "error", serr)
		if err != nil {
			return "", err
		}
		return "", nil
	}
	if err != nil {
		log.Logger.Warnw("metadata local-ipv4 unavailable, using route source address as private IP", "error", err, "privateIP", src)
	} else {
		log.Logger.Warnw("metadata local-ipv4 is not a valid IPv4 address, using route source address as private IP", "value", addr, "privateIP", src)
	}
	return src, nil
}

// routeSourceIPv4 returns the host IPv4 address that the kernel would use as
// the source when sending to the metadata service. A UDP "dial" performs no
// network I/O; it only resolves the route and its source address.
func routeSourceIPv4() (string, error) {
	conn, err := net.Dial("udp4", net.JoinHostPort(imdsHost, "80"))
	if err != nil {
		return "", fmt.Errorf("failed to resolve route to metadata service: %w", err)
	}
	defer func() {
		_ = conn.Close()
	}()

	la, ok := conn.LocalAddr().(*net.UDPAddr)
	if !ok || la.IP == nil {
		return "", fmt.Errorf("no local UDP address for metadata service route")
	}
	return parseRouteSourceIPv4(la.IP.String())
}

// parseRouteSourceIPv4 accepts only a usable host IPv4 address: the route
// source is never the loopback, unspecified, or link-local address.
// Unlike the NIC-based fallback in the login request, publicly routable
// addresses are accepted on purpose: nscale assigns fabric IPs from a public
// range, and this address is only used when the metadata service did not
// return local-ipv4.
func parseRouteSourceIPv4(s string) (string, error) {
	ip, err := netip.ParseAddr(s)
	if err != nil || !ip.Is4() || ip.IsLoopback() || ip.IsUnspecified() || ip.IsLinkLocalUnicast() {
		return "", fmt.Errorf("invalid route source IPv4 %q", s)
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
