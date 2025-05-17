// Package up implements the "up" command.
package up

import (
	"fmt"
	"os"

	"github.com/urfave/cli"

	cmdcommon "github.com/leptonai/gpud/cmd/common"
	cmdlogin "github.com/leptonai/gpud/cmd/gpud/login"
	"github.com/leptonai/gpud/pkg/gpud-manager/systemd"
	"github.com/leptonai/gpud/pkg/log"
	pkdsystemd "github.com/leptonai/gpud/pkg/systemd"
	pkgupdate "github.com/leptonai/gpud/pkg/update"
)

func Command(cliContext *cli.Context) (retErr error) {
	logLevel := cliContext.String("log-level")
	logFile := cliContext.String("log-file")
	zapLvl, err := log.ParseLogLevel(logLevel)
	if err != nil {
		return err
	}
	log.Logger = log.CreateLogger(zapLvl, logFile)

	if cliContext.String("token") != "" {
		if lerr := cmdlogin.Command(cliContext); lerr != nil {
			fmt.Printf("%s failed to login (%v)\n", cmdcommon.WarningSign, lerr)
			return lerr
		}
		fmt.Printf("%s successfully logged in\n", cmdcommon.CheckMark)
	}

	bin, err := os.Executable()
	if err != nil {
		return err
	}
	if err := pkgupdate.RequireRoot(); err != nil {
		return err
	}
	if !pkdsystemd.SystemctlExists() {
		return fmt.Errorf("requires systemd, to run without systemd, '%s run'", bin)
	}

	if !systemd.DefaultBinExists() {
		return fmt.Errorf("gpud binary not found at %s (you may run 'cp %s %s' to fix the installation)", systemd.DefaultBinPath, bin, systemd.DefaultBinPath)
	}

	endpoint := cliContext.String("endpoint")
	if err := systemdInit(endpoint); err != nil {
		return err
	}

	if err := pkgupdate.EnableGPUdSystemdUnit(); err != nil {
		return err
	}

	if err := pkgupdate.RestartGPUdSystemdUnit(); err != nil {
		return err
	}

	log.Logger.Debugw("successfully started gpud (run 'gpud status' for checking status)")
	return nil
}

func systemdInit(endpoint string) error {
	if err := systemd.CreateDefaultEnvFile(endpoint); err != nil {
		return err
	}
	systemdUnitFileData := systemd.GPUDService
	return os.WriteFile(systemd.DefaultUnitFile, []byte(systemdUnitFileData), 0644)
}
