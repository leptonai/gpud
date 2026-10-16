// Package oci implements the Oracle Cloud Infrastructure provider detector.
package oci

import (
	"context"
	"net/netip"
	"os"
	"strings"

	"github.com/leptonai/gpud/pkg/log"
	"github.com/leptonai/gpud/pkg/providers"
	"github.com/leptonai/gpud/pkg/providers/oci/imds"
)

const Name = "oci"

// instanceIDFallbackPaths lists host files where cloud-init records the OCI
// instance OCID ("ocid1.instance.<realm>.<region-key>.<unique-id>") at boot.
// The "/host" path covers the GPUd DaemonSet read-only host root mount
// (gpud.mountHostRoot); the plain path covers systemd installations.
var instanceIDFallbackPaths = []string{
	"/var/lib/cloud/data/instance-id",
	"/host/var/lib/cloud/data/instance-id",
}

// ociRegionKeyToIdentifier maps OCI 3-letter region keys (as embedded in the
// 4th dot-separated field of an OCI OCID) to their canonical OCI region
// identifiers when IMDS is unreachable.
// ref. https://docs.oracle.com/en-us/iaas/Content/General/Concepts/regions.htm
var ociRegionKeyToIdentifier = map[string]string{
	"ams":       "eu-amsterdam-1",
	"arn":       "eu-stockholm-1",
	"auh":       "me-abudhabi-1",
	"bog":       "sa-bogota-1",
	"bom":       "ap-mumbai-1",
	"cdg":       "eu-paris-1",
	"chuncheon": "ap-chuncheon-1",
	"cul":       "mx-culiacan-1",
	"cwl":       "uk-cardiff-1",
	"dub":       "eu-dublin-1",
	"dxb":       "me-dubai-1",
	"fra":       "eu-frankfurt-1",
	"gru":       "sa-saopaulo-1",
	"hyd":       "ap-hyderabad-1",
	"iad":       "us-ashburn-1",
	"icn":       "ap-seoul-1",
	"jed":       "me-jeddah-1",
	"jnb":       "af-johannesburg-1",
	"kix":       "ap-osaka-1",
	"lhr":       "uk-london-1",
	"lin":       "eu-milan-1",
	"mad":       "eu-madrid-1",
	"mel":       "ap-melbourne-1",
	"mrs":       "eu-marseille-1",
	"mtz":       "il-jerusalem-1",
	"mty":       "mx-monterrey-1",
	"nrt":       "ap-tokyo-1",
	"ord":       "us-chicago-1",
	"phx":       "us-phoenix-1",
	"qro":       "mx-queretaro-1",
	"riy":       "me-riyadh-1",
	"scl":       "sa-santiago-1",
	"sin":       "ap-singapore-1",
	"sjc":       "us-sanjose-1",
	"syd":       "ap-sydney-1",
	"vap":       "sa-valparaiso-1",
	"vcp":       "sa-vinhedo-1",
	"yny":       "ap-chuncheon-1",
	"yul":       "ca-montreal-1",
	"yyz":       "ca-toronto-1",
	"zrh":       "eu-zurich-1",
}

func New() providers.Detector {
	return providers.NewIMDSWithRegion(
		Name,
		detectProvider,
		nil,
		fetchPrivateIPv4,
		fetchRegion,
		nil,
		fetchInstanceID,
	)
}

func detectProvider(ctx context.Context) (string, error) {
	instanceID, err := fetchInstanceID(ctx)
	if err != nil {
		return "", err
	}
	if instanceID != "" {
		return Name, nil
	}
	return "", nil
}

func fetchInstanceID(ctx context.Context) (string, error) {
	instanceID, err := imds.FetchInstanceID(ctx)
	if err == nil && instanceID != "" {
		return instanceID, nil
	}
	if fallbackID := readInstanceOCIDFallback(instanceIDFallbackPaths); fallbackID != "" {
		return fallbackID, nil
	}
	return instanceID, err
}

func fetchRegion(ctx context.Context) (string, error) {
	region, err := imds.FetchCanonicalRegionName(ctx)
	if err == nil && region != "" {
		return region, nil
	}
	if fallbackID := readInstanceOCIDFallback(instanceIDFallbackPaths); fallbackID != "" {
		if fallbackRegion := regionFromInstanceOCID(fallbackID); fallbackRegion != "" {
			log.Logger.Infow("using OCI region derived from fallback instance OCID", "region", fallbackRegion, "instanceID", fallbackID)
			return fallbackRegion, nil
		}
	}
	return region, err
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

func readInstanceOCIDFallback(paths []string) string {
	for _, p := range paths {
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		if id, ok := parseInstanceOCID(string(b)); ok {
			log.Logger.Infow("using OCI instance ID from host file", "path", p, "instanceID", id)
			return id
		}
	}
	return ""
}

// parseInstanceOCID validates an OCI instance OCID of the form:
// "ocid1.instance.<realm>.<region>.<unique-id>".
// ref. https://docs.oracle.com/en-us/iaas/Content/General/Concepts/identifiers.htm
func parseInstanceOCID(raw string) (string, bool) {
	s := strings.ToLower(strings.TrimSpace(raw))
	parts := strings.Split(s, ".")
	if len(parts) != 5 || parts[0] != "ocid1" || parts[1] != "instance" {
		return "", false
	}
	for _, part := range parts[2:] {
		if part == "" || !isLowerAlnumOrHyphen(part) {
			return "", false
		}
	}
	return s, true
}

func isLowerAlnumOrHyphen(s string) bool {
	for i := range len(s) {
		c := s[i]
		if (c < 'a' || c > 'z') && (c < '0' || c > '9') && c != '-' {
			return false
		}
	}
	return true
}

func regionFromInstanceOCID(ocid string) string {
	parts := strings.Split(strings.ToLower(strings.TrimSpace(ocid)), ".")
	if len(parts) != 5 || parts[0] != "ocid1" || parts[1] != "instance" {
		return ""
	}
	regionField := parts[3]
	if canonical, ok := ociRegionKeyToIdentifier[regionField]; ok {
		return canonical
	}
	// Dedicated/realm regions may already embed the full hyphenated region
	// identifier (e.g. "us-ashburn-1") in the region field.
	if strings.Contains(regionField, "-") {
		return regionField
	}
	return ""
}
