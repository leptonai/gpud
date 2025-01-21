package tailscale

import (
	"os/exec"
)

func TailscaleExists() bool {
	p, err := exec.LookPath("tailscale")
	if err != nil {
		return false
	}
	return p != ""
}
