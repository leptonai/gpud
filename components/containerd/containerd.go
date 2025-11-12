package containerd

import (
	"fmt"
	"os/exec"
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

// Example output:
//
//	$ containerd --version
//	containerd github.com/containerd/containerd v1.7.20 7ab40557b20c33567c74c661d548661b631bbb37
func getContainerdVersion() (string, error) {
	p, err := pkgfile.LocateExecutable("containerd") // e.g.,"/usr/bin/containerd"
	if err != nil {
		return "", err
	}
	out, err := exec.Command(p, "--version").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("containerd --version failed: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return parseContainerdVersion(string(out))
}

var containerdVersionRegex = regexp.MustCompile(`\s+(v?\d+\.\d+\.\d+)(?:\s+|$)`)

func parseContainerdVersion(out string) (string, error) {
	matches := containerdVersionRegex.FindStringSubmatch(out)
	if len(matches) < 2 {
		return "", fmt.Errorf("invalid containerd version output: %s", strings.TrimSpace(out))
	}
	return matches[1], nil
}
