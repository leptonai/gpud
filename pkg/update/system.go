package update

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"

	pkdsystemd "github.com/leptonai/gpud/pkg/systemd"
)

func detectUbuntuVersion() string {
	outputBytes, err := exec.Command("lsb_release", "-i", "-s").Output()
	if err != nil {
		return ""
	}
	osName := strings.TrimSpace(strings.ToLower(string(outputBytes)))
	if osName != "ubuntu" {
		return ""
	}
	outputBytes, err = exec.Command("lsb_release", "-r", "-s").Output()
	if err != nil {
		return ""
	}
	version := strings.TrimSpace(string(outputBytes))
	if version == "22.04" || version == "24.04" {
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
