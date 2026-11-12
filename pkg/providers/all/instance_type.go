package all

import (
	"context"
	"time"

	pkgproviders "github.com/leptonai/gpud/pkg/providers"
)

// DetectInstanceType refreshes only machine-type metadata, leaving enrollment's
// required identity checks to DetectWithOverrides.
func DetectInstanceType(ctx context.Context) string {
	return detectInstanceType(ctx, All)
}

func detectInstanceType(ctx context.Context, detectors []pkgproviders.Detector) string {
	for _, detector := range detectors {
		if ctx.Err() != nil {
			return ""
		}
		cctx, cancel := context.WithTimeout(ctx, 15*time.Second)
		provider, err := detector.Provider(cctx)
		cancel()
		if err == nil && provider != "" {
			// Preserve provider ordering even for providers without a type reader:
			// their metadata service may resemble another provider's IMDS.
			return fetchInstanceType(ctx, detector)
		}
	}
	return ""
}

func fetchInstanceType(ctx context.Context, detector pkgproviders.Detector) string {
	if !pkgproviders.SupportsInstanceType(detector) {
		return ""
	}
	reader, ok := detector.(pkgproviders.InstanceTypeDetector)
	if !ok {
		return ""
	}
	cctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	value, err := fetchProviderMetadata(cctx, detector, "instance type", reader.InstanceType, false)
	if err != nil {
		return ""
	}
	return value
}
