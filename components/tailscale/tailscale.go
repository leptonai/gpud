package tailscale

import (
	"fmt"
	"os/exec"
	"regexp"
	"strings"

	pkgfile "github.com/leptonai/gpud/pkg/file"
	"github.com/leptonai/gpud/pkg/log"
)

func checkTailscaleInstalled() bool {
	p, err := pkgfile.LocateExecutable("tailscale") // e.g., "/usr/bin/tailscale"
	if err == nil {
		log.Logger.Debugw("tailscale found in PATH", "path", p)
		return true
	}
	log.Logger.Debugw("tailscale not found in PATH", "error", err)
	return false
}

func checkTailscaledInstalled() bool {
	p, err := pkgfile.LocateExecutable("tailscaled") // e.g., "/usr/bin/tailscaled"
	if err == nil {
		log.Logger.Debugw("tailscaled found in PATH", "path", p)
		return true
	}
	log.Logger.Debugw("tailscaled not found in PATH", "error", err)
	return false
}

// Example output:
//
//	$ tailscale --version
//
//	1.80.0
//		tailscale commit: 4f4686503ae930740854e71efef4baa4ac815844
//		other commit: ccb3ce01b143ca5d39bf8eed68601d827d547718
//		go version: go1.23.5
func getTailscaleVersion() (string, error) {
	p, err := pkgfile.LocateExecutable("tailscale")
	if err != nil {
		return "", err
	}
	out, err := exec.Command(p, "--version").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("tailscale --version failed: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return parseTailscaleVersion(string(out))
}

var tailscaleVersionRegex = regexp.MustCompile(`(?m)^\s*(v?\d+(?:\.\d+){1,3})\s*$`)

func parseTailscaleVersion(out string) (string, error) {
	return parseVersionFromOutput(out, "tailscale")
}

func parseVersionFromOutput(out, name string) (string, error) {
	matches := tailscaleVersionRegex.FindStringSubmatch(out)
	if len(matches) < 2 {
		return "", fmt.Errorf("invalid %s version output: %s", name, strings.TrimSpace(out))
	}
	return matches[1], nil
}

// CheckTailscaleInstalled checks if the tailscale binary is installed.
func CheckTailscaleInstalled() bool {
	return checkTailscaleInstalled()
}

// GetTailscaleVersion returns the version of the tailscale binary.
func GetTailscaleVersion() (string, error) {
	return getTailscaleVersion()
}
