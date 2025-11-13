package kubelet

import (
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
