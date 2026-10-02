package containerd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	pkgfile "github.com/leptonai/gpud/pkg/file"
	"github.com/leptonai/gpud/pkg/log"
)

// hostRoot is where the gpud DaemonSet mounts the node's root filesystem
// (gpud.mountHostRoot, default on).
const hostRoot = "/host"

var errContainerdNotFound = errors.New("containerd binary not found")

// locateContainerd returns the path of the node's containerd binary. The gpud
// image does not bundle containerd, and inside a container PATH describes the
// image rather than the node, so with the host root mounted it searches the
// host's standard binary directories. Without the host-root mount
// (bare-metal/systemd installs) it falls back to the PATH lookup.
func locateContainerd(root string) (string, error) {
	st, err := os.Stat(root)
	if err != nil || !st.IsDir() {
		return pkgfile.LocateExecutable("containerd")
	}
	for _, dir := range []string{"usr/bin", "usr/local/bin", "usr/sbin", "bin", "sbin"} {
		p := filepath.Join(root, dir, "containerd")
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p, nil
		}
	}
	return "", fmt.Errorf("%w under %s", errContainerdNotFound, root)
}

func checkContainerdInstalled() bool {
	return checkContainerdInstalledOnHost(hostRoot, defaultSocketFile)
}

// checkContainerdInstalledOnHost reports whether containerd is installed on
// the node (see locateContainerd). With the host root mounted, a present
// containerd socket also counts, covering host binaries installed outside the
// standard directories (the image used to bundle containerd, so such nodes
// were monitored before).
func checkContainerdInstalledOnHost(root string, socketFile string) bool {
	p, err := locateContainerd(root)
	if err == nil {
		log.Logger.Debugw("containerd binary found", "path", p)
		return true
	}
	if errors.Is(err, errContainerdNotFound) && checkSocketExists(socketFile) {
		return true
	}
	log.Logger.Debugw("containerd not found", "error", err)
	return false
}

// CheckContainerdInstalled returns true if containerd is installed on the node.
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
