// Package all provides a list of known providers.
package all

import (
	"context"
	"time"

	"github.com/leptonai/gpud/pkg/log"
	pkgproviders "github.com/leptonai/gpud/pkg/providers"
	pkgprovidersaws "github.com/leptonai/gpud/pkg/providers/aws"
	pkgprovidersazure "github.com/leptonai/gpud/pkg/providers/azure"
	pkgprovidersgcp "github.com/leptonai/gpud/pkg/providers/gcp"
	pkgprovidersnscale "github.com/leptonai/gpud/pkg/providers/nscale"
)

var All = []pkgproviders.Detector{
	pkgprovidersnscale.New(),
	pkgprovidersaws.New(),
	pkgprovidersazure.New(),
	pkgprovidersgcp.New(),
}

// Detect detects the provider and returns the provider info.
func Detect(ctx context.Context) (*pkgproviders.Info, error) {
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
		}, nil
	}

	info := &pkgproviders.Info{
		Provider: detector.Name(),
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

	vmEnvironment, err := detector.VMEnvironment(ctx)
	if err != nil {
		log.Logger.Warnw("failed to get VM environment", "provider", detector.Name(), "error", err)
	} else {
		info.VMEnvironment = vmEnvironment
	}

	instanceID, err := detector.InstanceID(ctx)
	if err != nil {
		log.Logger.Warnw("failed to get instance ID", "provider", detector.Name(), "error", err)
	} else {
		info.InstanceID = instanceID
		log.Logger.Infow("successfully detected instance ID", "provider", detector.Name(), "instanceID", instanceID)
	}

	return info, nil
}
