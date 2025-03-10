package tailscale

import (
	pkg_file "github.com/leptonai/gpud/pkg/file"
	"github.com/leptonai/gpud/pkg/log"
)

func checkTailscaledInstalled() bool {
	p, err := pkg_file.LocateExecutable("tailscaled")
	if err == nil {
		log.Logger.Debugw("tailscaled found in PATH", "path", p)
		return true
	}
	log.Logger.Debugw("tailscaled not found in PATH", "error", err)
	return false
}
