package machineinfo

import (
	"fmt"
	"os"
	"strings"

	"github.com/google/uuid"

	"github.com/leptonai/gpud/pkg/log"
)

// instanceIDFallbackPaths lists host files that may hold an instance ID when
// no provider metadata service (IMDS) supplied one, in probe order. The first
// file with a valid ID wins.
//
// cloud-init records the ID of the instance it was provisioned as, from
// whichever datasource it found at boot: the OpenStack config drive
// (e.g. Mistral bare metal provisioned by Metal3, where the value is the
// Metal3 host UUID that also appears in the Node's spec.providerID) or a
// NoCloud seed disk (e.g. Lambda). Neither provider exposes a link-local
// metadata service, so this file is the only host-local source of the ID.
//
// The "/host" entry is the read-only host root mount used by the GPUd
// DaemonSet (helm value gpud.mountHostRoot); the plain path covers systemd
// installations. Add a path here when another provider is found to keep its
// instance ID in a different host file.
var instanceIDFallbackPaths = []string{
	"/var/lib/cloud/data/instance-id",
	"/host/var/lib/cloud/data/instance-id",
}

// readInstanceIDFallback returns the first valid instance ID found in paths,
// or "" when none of them holds one. Missing files are expected on hosts
// without cloud-init and are not errors.
func readInstanceIDFallback(paths []string) string {
	for _, p := range paths {
		b, err := os.ReadFile(p)
		if err != nil {
			if !os.IsNotExist(err) {
				log.Logger.Warnw("failed to read instance ID fallback file", "path", p, "error", err)
			}
			continue
		}
		id, err := parseFallbackInstanceID(string(b))
		if err != nil {
			log.Logger.Warnw("ignoring invalid instance ID fallback file", "path", p, "error", err)
			continue
		}
		log.Logger.Infow("using instance ID from host file", "path", p, "instanceID", id)
		return id
	}
	return ""
}

// parseFallbackInstanceID accepts a UUID (36 characters) or 32 hex characters
// and returns it lowercased without changing its form. Anything else is
// rejected, including cloud-init's "iid-datasource-none" placeholder, which
// cloud-init writes when it found no datasource.
func parseFallbackInstanceID(raw string) (string, error) {
	s := strings.ToLower(strings.TrimSpace(raw))
	// uuid.Parse also accepts the braced and "urn:uuid:" forms; keep only the
	// two forms cloud-init datasources write so the ID is stored as found.
	if len(s) != 36 && len(s) != 32 {
		return "", fmt.Errorf("invalid instance ID %q", raw)
	}
	if _, err := uuid.Parse(s); err != nil {
		return "", fmt.Errorf("invalid instance ID %q: %w", raw, err)
	}
	return s, nil
}
