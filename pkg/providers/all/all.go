// Package all provides a list of known providers.
package all

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/leptonai/gpud/pkg/log"
	pkgproviders "github.com/leptonai/gpud/pkg/providers"
	pkgprovidersaws "github.com/leptonai/gpud/pkg/providers/aws"
	pkgprovidersazure "github.com/leptonai/gpud/pkg/providers/azure"
	pkgprovidersgcp "github.com/leptonai/gpud/pkg/providers/gcp"
	pkgprovidersnebius "github.com/leptonai/gpud/pkg/providers/nebius"
	pkgprovidersnscale "github.com/leptonai/gpud/pkg/providers/nscale"
	pkgprovidersoci "github.com/leptonai/gpud/pkg/providers/oci"
)

var imdsRetryBackoffs = []time.Duration{0, 100 * time.Millisecond, 200 * time.Millisecond, 400 * time.Millisecond}

var All = []pkgproviders.Detector{
	pkgprovidersnscale.New(),
	pkgprovidersnebius.New(),
	pkgprovidersaws.New(),
	pkgprovidersazure.New(),
	pkgprovidersgcp.New(),
	pkgprovidersoci.New(),
}

// Detect detects the provider and returns the provider info.
func Detect(ctx context.Context) (*pkgproviders.Info, error) {
	return DetectWithRegionOverride(ctx, "")
}

// DetectWithRegionOverride skips provider-region metadata when regionOverride is set.
func DetectWithRegionOverride(ctx context.Context, regionOverride string) (*pkgproviders.Info, error) {
	var detector pkgproviders.Detector
	for _, d := range All {
		cctx, cancel := context.WithTimeout(ctx, 15*time.Second)
		provider, err := d.Provider(cctx)
		cancel()
		if err != nil {
			if d != nil {
				log.Logger.Debugw("failed to get provider", "name", d.Name(), "error", err)
			} else {
				log.Logger.Debugw("failed to get provider", "error", err)
			}
			continue
		}

		if provider != "" {
			detector = d
			log.Logger.Infow("detected provider", "provider", provider)
			break
		}
	}

	if detector == nil {
		return &pkgproviders.Info{
			Provider: "unknown",
			Region:   strings.TrimSpace(regionOverride),
		}, nil
	}

	info := &pkgproviders.Info{
		Provider:     detector.Name(),
		IMDSDetected: pkgproviders.SupportsIMDS(detector),
	}

	// Metadata fields are best-effort: provider detection has already succeeded,
	// so one failed optional fetch should not discard provider identity.
	publicIP, err := detector.PublicIPv4(ctx)
	if err != nil {
		log.Logger.Warnw("failed to get public IP", "provider", detector.Name(), "error", err)
	} else {
		info.PublicIP = publicIP
	}

	privateIP, err := detector.PrivateIPv4(ctx)
	if err != nil {
		log.Logger.Warnw("failed to get private IP", "provider", detector.Name(), "error", err)
	} else {
		info.PrivateIP = privateIP
		log.Logger.Infow("successfully detected private IP", "provider", detector.Name(), "privateIP", privateIP)
	}

	if regionOverride = strings.TrimSpace(regionOverride); regionOverride != "" {
		info.Region = regionOverride
	} else if regionDetector, ok := detector.(pkgproviders.RegionDetector); ok {
		region, err := fetchRequiredMetadata(ctx, detector, "region", regionDetector.Region)
		if err != nil {
			log.Logger.Warnw("failed to get region", "provider", detector.Name(), "error", err)
		} else {
			info.Region = region
		}
	}

	vmEnvironment, err := detector.VMEnvironment(ctx)
	if err != nil {
		log.Logger.Warnw("failed to get VM environment", "provider", detector.Name(), "error", err)
	} else {
		info.VMEnvironment = vmEnvironment
	}

	instanceID, err := fetchRequiredMetadata(ctx, detector, "instance ID", detector.InstanceID)
	if err != nil {
		log.Logger.Warnw("failed to get instance ID", "provider", detector.Name(), "error", err)
	} else {
		info.InstanceID = instanceID
		log.Logger.Infow("successfully detected instance ID", "provider", detector.Name(), "instanceID", instanceID)
	}

	return info, nil
}

func fetchRequiredMetadata(ctx context.Context, detector pkgproviders.Detector, field string, fetch func(context.Context) (string, error)) (string, error) {
	if !pkgproviders.SupportsIMDS(detector) {
		return fetch(ctx)
	}

	var lastErr error
	for attempt, backoff := range imdsRetryBackoffs {
		if backoff > 0 {
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(backoff):
			}
		}

		value, err := fetch(ctx)
		value = strings.TrimSpace(value)
		if err == nil && value != "" && !strings.EqualFold(value, "unknown") {
			return value, nil
		}
		if err == nil {
			err = fmt.Errorf("empty or invalid %s", field)
		}
		lastErr = err
		log.Logger.Warnw("failed to get required provider metadata",
			"provider", detector.Name(),
			"field", field,
			"attempt", attempt+1,
			"maxAttempts", len(imdsRetryBackoffs),
			"error", err,
		)
	}

	return "", lastErr
}
