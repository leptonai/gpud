package kubelet

import (
	"fmt"
	"os/exec"
	"regexp"
	"strings"

	pkgfile "github.com/leptonai/gpud/pkg/file"
	"github.com/leptonai/gpud/pkg/log"
)

func checkKubeletInstalled() bool {
	p, err := pkgfile.LocateExecutable("kubelet")
	if err == nil {
		log.Logger.Debugw("kubelet found in PATH", "path", p)
		return true
	}
	log.Logger.Debugw("kubelet not found in PATH", "error", err)
	return false
}

// Example output:
//
//	$ kubelet --version
//	Kubernetes v1.33.4
func getKubeletVersion() (string, error) {
	p, err := pkgfile.LocateExecutable("kubelet") // e.g., "/usr/bin/kubelet"
	if err != nil {
		return "", err
	}
	out, err := exec.Command(p, "--version").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("kubelet --version failed: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return parseKubeletVersion(string(out))
}

var kubeletVersionRegex = regexp.MustCompile(`Kubernetes\s+(v?\d+\.\d+\.\d+)`)

func parseKubeletVersion(out string) (string, error) {
	matches := kubeletVersionRegex.FindStringSubmatch(out)
	if len(matches) < 2 {
		return "", fmt.Errorf("invalid kubelet version output: %s", strings.TrimSpace(out))
	}
	return matches[1], nil
}
