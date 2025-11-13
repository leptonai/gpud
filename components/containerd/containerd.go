package containerd

import (
	"fmt"
	"regexp"
	"strings"

	pkgfile "github.com/leptonai/gpud/pkg/file"
	"github.com/leptonai/gpud/pkg/log"
)

func checkContainerdInstalled() bool {
	p, err := pkgfile.LocateExecutable("containerd")
	if err == nil {
		log.Logger.Debugw("containerd found in PATH", "path", p)
		return true
	}
	log.Logger.Debugw("containerd not found in PATH", "error", err)
	return false
}

// CheckContainerdInstalled returns true if the containerd binary is available in PATH.
func CheckContainerdInstalled() bool {
	return checkContainerdInstalled()
}

var containerdVersionRegex = regexp.MustCompile(`\s+(v?\d+\.\d+\.\d+)(?:\s+|$)`)

func parseContainerdVersion(out string) (string, error) {
	matches := containerdVersionRegex.FindStringSubmatch(out)
	if len(matches) < 2 {
		return "", fmt.Errorf("invalid containerd version output: %s", strings.TrimSpace(out))
	}
	return matches[1], nil
}
