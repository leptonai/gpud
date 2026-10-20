package nscale

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"time"

	"github.com/leptonai/gpud/pkg/log"
	"github.com/leptonai/gpud/pkg/providers"
	"github.com/leptonai/gpud/pkg/providers/nscale/imds"
)

const Name = "nscale"

var imdsRetryBackoffs = []time.Duration{0, 100 * time.Millisecond, 200 * time.Millisecond, 400 * time.Millisecond}

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
		// the nscale metadata service can be slow to accept the first
		// connection after an idle period (observed >5s on production nodes),
		// and its fabric IPs are not RFC1918 so a missed private-IP fetch has
		// no NIC-based fallback — retry it like region and instance ID
		providers.WithPrivateIPv4Retry(),
	)
}

func detectProvider(ctx context.Context) (string, error) {
	var lastErr error
	for attempt, backoff := range imdsRetryBackoffs {
		if backoff > 0 {
			select {
			case <-ctx.Done():
				return "", errors.Join(lastErr, ctx.Err())
			case <-time.After(backoff):
			}
		}

		resp, statusCode, err := imds.FetchOpenStackMetadataWithStatusCode(ctx)
		if err != nil {
			lastErr = err
			if !isRetryableOpenStackMetadataStatus(statusCode) {
				return "", err
			}
			log.Logger.Debugw("transient HTTP error fetching nscale OpenStack metadata during provider detection",
				"attempt", attempt+1,
				"maxAttempts", len(imdsRetryBackoffs),
				"statusCode", statusCode,
				"error", err,
			)
			continue
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

	return "", lastErr
}

// isRetryableOpenStackMetadataStatus reports whether an HTTP status code from
// /openstack/latest/meta_data.json warrants an inline retry during initial
// provider detection. Only HTTP 429 and 5xx are retried here so non-NScale
// hosts (which return 4xx, connection refused, or link-local timeout) do not
// pay extra detection latency; network timeouts on NScale are recovered by the
// ASN fallback in pkg/machine-info.
func isRetryableOpenStackMetadataStatus(statusCode int) bool {
	return statusCode == http.StatusTooManyRequests || statusCode >= http.StatusInternalServerError
}

// GetInstanceID fetches the nscale VM/bare-metal instance ID (e.g. "i-00014a3a")
// from the EC2-compatible metadata endpoint with retries.
func GetInstanceID(ctx context.Context) (string, error) {
	return retryIMDSFetch(ctx, "instance ID", imds.FetchInstanceID)
}

// PrivateIPv4 returns the machine's nscale private IPv4 address from metadata
// with retries, falling back to the route source address toward 169.254.169.254.
func PrivateIPv4(ctx context.Context) (string, error) {
	return retryIMDSFetch(ctx, "private IP", fetchPrivateIPv4)
}

// Region returns the nscale region from OpenStack metadata when present.
func Region(ctx context.Context) (string, error) {
	return imds.FetchRegion(ctx)
}

// VMEnvironment returns the OpenStack availability zone from nscale metadata.
func VMEnvironment(ctx context.Context) (string, error) {
	return fetchVMEnvironment(ctx)
}

func retryIMDSFetch(ctx context.Context, field string, fetch func(context.Context) (string, error)) (string, error) {
	var lastErr error
	for attempt, backoff := range imdsRetryBackoffs {
		if backoff > 0 {
			select {
			case <-ctx.Done():
				return "", errors.Join(lastErr, ctx.Err())
			case <-time.After(backoff):
			}
		}

		val, err := fetch(ctx)
		val = strings.TrimSpace(val)
		if err == nil && val != "" {
			return val, nil
		}
		if err == nil {
			err = fmt.Errorf("empty nscale %s", field)
		}
		lastErr = err
		log.Logger.Warnw("failed to get nscale metadata",
			"field", field,
			"attempt", attempt+1,
			"maxAttempts", len(imdsRetryBackoffs),
			"error", err,
		)
	}

	return "", lastErr
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
