package session

import (
	"context"
	"strings"

	pkdsystemd "github.com/leptonai/gpud/pkg/gpud-manager/systemd"
	"github.com/leptonai/gpud/pkg/log"
	"github.com/leptonai/gpud/pkg/systemd"
	"github.com/leptonai/gpud/pkg/update"
)

// processUpdate handles the update request
func (s *Session) processUpdate(_ context.Context, payload Request, response *Response, restartExitCode *int) {
	if targetVersion := strings.Split(payload.UpdateVersion, ":"); len(targetVersion) == 2 {
		err := update.PackageUpdate(targetVersion[0], targetVersion[1], update.DefaultUpdateURL, s.dataDir)
		log.Logger.Infow("update received for machine", "version", targetVersion[1], "package", targetVersion[0], "error", err)
	} else {
		if !s.enableAutoUpdate {
			log.Logger.Warnw("auto update is disabled -- skipping update")
			response.Error = "auto update is disabled"
			return
		}

		systemdManaged, _ := systemd.IsActive("gpud.service")
		if s.autoUpdateExitCode == -1 && !systemdManaged {
			log.Logger.Warnw("gpud is not managed with systemd and auto update by exit code is not set -- skipping update")
			response.Error = "gpud is not managed with systemd"
			return
		}

		nextVersion := payload.UpdateVersion
		if nextVersion == "" {
			log.Logger.Warnw("target update_version is empty -- skipping update")
			response.Error = "update_version is empty"
			return
		}

		if systemdManaged {
			if uerr := pkdsystemd.CreateDefaultEnvFile(""); uerr != nil {
				response.Error = uerr.Error()
				return
			}
		}

		// even if it's systemd managed, it's using "Restart=always"
		// thus we simply exit the process to trigger the restart
		// do not use "systemctl restart gpud.service"
		// as it immediately restarts the service,
		// failing to respond to the control plane
		uerr := update.UpdateExecutable(nextVersion, update.DefaultUpdateURL, systemdManaged)
		if uerr != nil {
			response.Error = uerr.Error()
		} else {
			*restartExitCode = s.autoUpdateExitCode
			log.Logger.Infow("scheduled process exit for auto update", "code", *restartExitCode)
		}
	}
}
