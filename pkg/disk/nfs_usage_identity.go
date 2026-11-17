package disk

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
)

func readNFSUsageIdentity(part Partition) (string, error) {
	f, err := os.Open("/proc/self/mountinfo")
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	return nfsUsageIdentity(f, part)
}

func nfsUsageIdentity(r io.Reader, part Partition) (string, error) {
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
		// Include mount ID, and reject stacked targets rather than guessing which
		// filesystem a pathname syscall will reach. No mounted filesystem I/O here.
		if identity != "" || b[0] != part.Fstype || unescape.Replace(b[1]) != part.Device {
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
