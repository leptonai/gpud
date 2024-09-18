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

// Returns the latest fabric manager output using journalctl.
// Equivalent to "journalctl -xeu nvidia-fabricmanager.service --no-pager".
func GetLatestFabricManagerOutput(ctx context.Context) (string, error) {
	return systemd.GetLatestJournalctlOutput(ctx, "nvidia-fabricmanager")
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

	// Stores the output of "journalctl -xeu nvidia-fabricmanager.service --no-pager".
	// Useful for debugging fabric manager fails to start (e.g., "Error: Fabric Manager already running with pid 7388").
	JournalOutput string `json:"journal_output,omitempty"`
}
