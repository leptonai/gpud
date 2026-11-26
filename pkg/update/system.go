package update

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"

	pkghost "github.com/leptonai/gpud/pkg/host"
	pkdsystemd "github.com/leptonai/gpud/pkg/systemd"
)

// osReleasePath is a variable so tests can use a fixture file.
var osReleasePath = pkghost.OSReleasePath

// detectUbuntuVersion returns the Ubuntu suffix of the release tarball for
// this system (for example "ubuntu22.04"), or "" for the generic tarball.
// It asks lsb_release first. When lsb_release cannot run, it reads
// /etc/os-release, as install.sh does. Container images such as the GPUd
// image do not install lsb-release, and the release workflow builds only
// the Ubuntu Linux tarballs.
func detectUbuntuVersion() string {
	id, version, err := lsbReleaseIDAndVersion()
	if err != nil {
		fields, err := pkghost.ReadOSRelease(osReleasePath)
		if err != nil {
			return ""
		}
		id, version = fields["ID"], fields["VERSION_ID"]
	}
	return ubuntuTarballSuffix(id, version)
}

func lsbReleaseIDAndVersion() (string, string, error) {
	idOut, err := exec.Command("lsb_release", "-i", "-s").Output()
	if err != nil {
		return "", "", err
	}
	id := strings.TrimSpace(string(idOut))
	if !strings.EqualFold(id, "ubuntu") {
		// Do not ask for the version of a distribution that has no Ubuntu tarball.
		return id, "", nil
	}
	versionOut, err := exec.Command("lsb_release", "-r", "-s").Output()
	if err != nil {
		return "", "", err
	}
	return id, strings.TrimSpace(string(versionOut)), nil
}

func ubuntuTarballSuffix(id, version string) string {
	if !strings.EqualFold(id, "ubuntu") {
		return ""
	}
	switch version {
	case "22.04", "24.04", "26.04":
		return "ubuntu" + version
	}
	return ""
}

func EnableGPUdSystemdUnit() error {
	if !pkdsystemd.SystemctlExists() {
		return errors.ErrUnsupported
	}
	if out, err := exec.Command("systemctl", "enable", "gpud.service").CombinedOutput(); err != nil {
		return fmt.Errorf("systemctl enable failed: %w output: %s", err, out)
	}
	return nil
}

func DisableGPUdSystemdUnit() error {
	if !pkdsystemd.SystemctlExists() {
		return errors.ErrUnsupported
	}
	if out, err := exec.Command("systemctl", "disable", "gpud.service").CombinedOutput(); err != nil {
		return fmt.Errorf("systemctl disable failed: %w output: %s", err, out)
	}
	return nil
}

func RestartGPUdSystemdUnit() error {
	if !pkdsystemd.SystemctlExists() {
		return errors.ErrUnsupported
	}
	if out, err := exec.Command("systemctl", "daemon-reload").CombinedOutput(); err != nil {
		return fmt.Errorf("systemctl daemon-reload failed: %w output: %s", err, out)
	}
	if out, err := exec.Command("systemctl", "restart", "gpud.service").CombinedOutput(); err != nil {
		return fmt.Errorf("systemctl restart failed: %w output: %s", err, out)
	}
	return nil
}

func StopSystemdUnit() error {
	if !pkdsystemd.SystemctlExists() {
		return errors.ErrUnsupported
	}
	if out, err := exec.Command("systemctl", "stop", "gpud.service").CombinedOutput(); err != nil {
		return fmt.Errorf("systemctl stop failed: %w output: %s", err, out)
	}
	return nil
}
