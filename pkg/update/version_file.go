package update

import (
	"bytes"
	"errors"
	"os"

	"github.com/leptonai/gpud/pkg/config"
	"github.com/leptonai/gpud/pkg/log"
	"github.com/leptonai/gpud/pkg/systemd"
	"github.com/leptonai/gpud/version"
)

var DefaultVersionFile = config.VersionFilePath(config.DefaultDataDir)

func UpdateTargetVersion(versionFile string, autoExitCode int) error {
	return updateTargetVersion(
		versionFile,
		autoExitCode,
		func() bool {
			systemdManaged, _ := systemd.IsActive("gpud.service")
			return systemdManaged
		},
		UpdateExecutable,
		os.Exit,
	)
}

func updateTargetVersion(
	versionFile string,
	autoExitCode int,
	checkGPUdSystemdServiceFunc func() bool,
	updateExecutableFunc func(targetVersion string, url string, requireRoot bool) error,
	osExitFunc func(code int),
) error {
	targetVer, needUpdate, err := checkVersionFileForUpdate(versionFile)
	if err != nil {
		return err
	}
	if !needUpdate {
		log.Logger.Debugw("no need to update GPUd to target version", "currentVersion", version.Version, "targetVersion", targetVer)
		return nil
	}

	log.Logger.Infow("need to update GPUd to target version", "currentVersion", version.Version, "targetVersion", targetVer)

	systemdManaged := checkGPUdSystemdServiceFunc()

	// even if it's systemd managed, it's using "Restart=always"
	// thus we simply exit the process to trigger the restart
	// do not use "systemctl restart gpud.service"
	// as it immediately restarts the service,
	// failing to respond to the control plane
	uerr := updateExecutableFunc(targetVer, DefaultUpdateURL, systemdManaged)
	if uerr != nil {
		return uerr
	}

	if autoExitCode != -1 {
		log.Logger.Infow("exiting with code after update with version file", "code", autoExitCode)
		osExitFunc(autoExitCode)
	}

	return nil
}

// checkVersionFileForUpdate checks if the specified version in the file
// is different from the current version, and returns true if the
// version is different (meaning we need update).
// If the file does not exist, it returns false and no error.
func checkVersionFileForUpdate(versionFile string) (string, bool, error) {
	if versionFile == "" {
		return "", false, nil
	}

	content, err := os.ReadFile(versionFile)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", false, nil
		}
		return "", false, err
	}
	ver := string(bytes.TrimSpace(content))
	curVer := version.Version
	return ver, ver != curVer, nil
}
