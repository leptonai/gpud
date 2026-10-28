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
// The "/host" entries are for the read-only host root mount used by the GPUd
// DaemonSet (helm value gpud.mountHostRoot); the plain paths cover systemd
// installations.
//
// "/var/lib/cloud/data/firebird-instance-id" is checked before
// "/var/lib/cloud/data/instance-id": Firebird bare-metal hosts also boot with
// an OpenStack config drive whose meta_data.json holds a Metal3 UUID, so
// "/var/lib/cloud/data/instance-id" is populated with that UUID, whereas
// Firebird's topology API identifies nodes by their canonical "bm.<ULID>" (or
// "vm.<ULID>") ID written to "/var/lib/cloud/data/firebird-instance-id".
var instanceIDFallbackPaths = []string{
	"/var/lib/cloud/data/firebird-instance-id",
	"/host/var/lib/cloud/data/firebird-instance-id",
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
		log.Logger.Infow("found instance ID in host file", "path", p, "instanceID", id)
		return id
	}
	return ""
}

// parseFallbackInstanceID trims an opaque provider instance ID and rejects
// cloud-init's empty or placeholder values. Recognized Firebird and UUID IDs
// retain their existing case normalization; other provider IDs are unchanged.
func parseFallbackInstanceID(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	switch trimmed {
	case "", "iid-datasource-none", "iid-datasource", "nocloud", "iid-dsconfigdrive", "iid-dsovf", "iid-dsopennebula":
		return "", fmt.Errorf("invalid instance ID %q", raw)
	}
	if id, ok := parseFirebirdInstanceID(trimmed); ok {
		return id, nil
	}

	// Normalize only the UUID forms cloud-init datasources commonly write.
	if len(trimmed) == 36 || len(trimmed) == 32 {
		if _, err := uuid.Parse(trimmed); err == nil {
			return strings.ToLower(trimmed), nil
		}
	}
	return trimmed, nil
}

// parseFirebirdInstanceID validates a Firebird canonical node ID of the form
// "bm.<ULID>" or "vm.<ULID>", where <ULID> is a 26-character Crockford Base32
// string (digits 0-9 and letters A-Z excluding I, L, O, U).
func parseFirebirdInstanceID(s string) (string, bool) {
	// "bm." / "vm." (3 bytes) + 26-char ULID = 29 bytes.
	if len(s) != 29 {
		return "", false
	}
	prefix := strings.ToLower(s[:3])
	if prefix != "bm." && prefix != "vm." {
		return "", false
	}
	ulid := strings.ToUpper(s[3:])
	// A 128-bit ULID encodes into 26 Crockford Base32 characters whose leading
	// character covers the top 2 bits and therefore cannot exceed '7'.
	if ulid[0] < '0' || ulid[0] > '7' {
		return "", false
	}
	for i := range len(ulid) {
		if !isCrockfordBase32(ulid[i]) {
			return "", false
		}
	}
	return prefix + ulid, true
}

func isCrockfordBase32(c byte) bool {
	if c >= '0' && c <= '9' {
		return true
	}
	if c >= 'A' && c <= 'Z' {
		return c != 'I' && c != 'L' && c != 'O' && c != 'U'
	}
	return false
}
