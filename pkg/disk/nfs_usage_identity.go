package disk

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

func nfsUsageIdentity(r io.Reader, part Partition, mountID string) (string, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 4096), 1024*1024)
	unescape := strings.NewReplacer(`\040`, " ", `\011`, "\t", `\012`, "\n", `\134`, `\`)
	identity := ""
	for scanner.Scan() {
		line := scanner.Text()
		left, right, ok := strings.Cut(line, " - ")
		a, b := strings.Fields(left), strings.Fields(right)
		if !ok || len(a) < 6 || len(b) < 3 {
			return "", fmt.Errorf("invalid mountinfo record")
		}
		if unescape.Replace(a[4]) != part.MountPoint {
			continue
		}
		// An automounter can leave its trigger at the same path as the export.
		// Only the mount referenced by the open descriptor can supply usage.
		if b[0] == "autofs" {
			continue
		}
		if identity != "" || a[0] != mountID || b[0] != part.Fstype || unescape.Replace(b[1]) != part.Device {
			return "", fmt.Errorf("filesystem identity changed at %q", part.MountPoint)
		}
		identity = line
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	if identity == "" {
		return "", fmt.Errorf("mount %q disappeared", part.MountPoint)
	}
	return identity, nil
}
