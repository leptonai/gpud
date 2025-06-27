package session

import (
	pkdpackages "github.com/leptonai/gpud/pkg/gpud-manager/packages"
	"github.com/leptonai/gpud/pkg/log"
)

func (s *Session) processInstallAddon(req *pkdpackages.InstallAddonRequest, resp *Response) {
	if req == nil {
		resp.Error = "install addon request is nil"
		return
	}

	log.Logger.Info("installing addon", "name", req.Name, "dir", req.PackagesDir)

	if err := req.Install(); err != nil {
		resp.Error = err.Error()
	} else {
		log.Logger.Info("successfully installed addon", "name", req.Name, "dir", req.PackagesDir)
	}
}
