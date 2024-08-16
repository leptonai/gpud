package query

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/leptonai/gpud/pkg/systemd"
)

func FabricManagerExists() bool {
	p, err := exec.LookPath("nv-fabricmanager")
	if err != nil {
		return false
	}
	return p != ""
}

func CheckFabricManagerVersion(ctx context.Context) (string, error) {
	p, err := exec.LookPath("nv-fabricmanager")
	if err != nil {
		return "", fmt.Errorf("fabric manager version check requires 'nv-fabricmanager' (%w)", err)
	}
	b, err := exec.CommandContext(ctx, p, "--version").CombinedOutput()
	if err != nil {
		return "", err
	}
	return parseFabricManagerVersion(string(b)), nil
}

func CheckFabricManagerActive(ctx context.Context, conn *systemd.DbusConn) (bool, error) {
	active, err := conn.IsActive(ctx, "nvidia-fabricmanager")
	if err != nil {
		return false, err
	}
	return active, nil
}

// e.g.,
// /usr/bin/nv-fabricmanager --version
// "Fabric Manager version is : 535.161.08"
func parseFabricManagerVersion(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "Fabric Manager version is : ")
	return strings.TrimSpace(s)
}

type FabricManagerOutput struct {
	Version string `json:"version"`
	// Set true if the "nvidia-fabricmanager" systemd service is active.
	Active bool `json:"active"`
}
