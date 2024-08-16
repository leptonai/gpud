package tailscale

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"time"
)

func TailscaleExists() bool {
	p, err := exec.LookPath("tailscale")
	if err != nil {
		return false
	}
	return p != ""
}

// CheckVersion returns the tailscale version by running `tailscale version --json`.
func CheckVersion() (*VersionInfo, error) {
	p, err := exec.LookPath("tailscale")
	if err != nil {
		return nil, fmt.Errorf("tailscale version check requires tailscale (%w)", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	b, err := exec.CommandContext(ctx, p, "version", "--json").CombinedOutput()
	cancel()
	if err != nil {
		return nil, err
	}

	return ParseVersion(b)
}

// VersionInfo represents the JSON structure of the Tailscale version information.
type VersionInfo struct {
	MajorMinorPatch string `json:"majorMinorPatch"`
	Short           string `json:"short"`
	Long            string `json:"long"`
	UnstableBranch  bool   `json:"unstableBranch"`
	Cap             int    `json:"cap"`
}

// ParseVersion parses the JSON-encoded version information from a byte slice.
func ParseVersion(b []byte) (*VersionInfo, error) {
	var v VersionInfo
	err := json.Unmarshal(b, &v)
	if err != nil {
		return nil, fmt.Errorf("error parsing version info: %w", err)
	}
	return &v, nil
}
