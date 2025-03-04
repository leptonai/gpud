package command

import (
	"errors"
	"fmt"
	"os"

	"github.com/leptonai/gpud/pkg/gpud-manager/systemd"
	pkd_systemd "github.com/leptonai/gpud/pkg/systemd"
	pkg_update "github.com/leptonai/gpud/pkg/update"

	"github.com/urfave/cli"
)

func cmdUp(cliContext *cli.Context) (retErr error) {
	defer func() {
		if retErr != nil {
			return
		}
		if cliContext.String("token") != "" {
			if err := cmdLogin(cliContext); err != nil {
				retErr = err
			}
		} else {
			fmt.Printf("\nvisit https://localhost:15132 to view the dashboard\n\n")
		}
	}()

	bin, err := os.Executable()
	if err != nil {
		return err
	}
	if err := pkg_update.RequireRoot(); err != nil {
		fmt.Printf("%s %q requires root to run with systemd: %v (to run without systemd, '%s run')\n", warningSign, bin, err, bin)
		return err
	}
	if !pkd_systemd.SystemctlExists() {
		fmt.Printf("%s requires systemd, to run without systemd, '%s run'\n", warningSign, bin)
		return errors.ErrUnsupported
	}

	if !systemd.DefaultBinExists() {
		return fmt.Errorf("gpud binary not found at %s (you may run 'cp %s %s' to fix the installation)", systemd.DefaultBinPath, bin, systemd.DefaultBinPath)
	}

	if err := systemdInit(); err != nil {
		fmt.Printf("%s failed to initialize systemd files\n", warningSign)
		return err
	}

	if err := pkg_update.EnableGPUdSystemdUnit(); err != nil {
		fmt.Printf("%s failed to enable systemd unit 'gpud.service'\n", warningSign)
		return err
	}

	if err := pkg_update.RestartGPUdSystemdUnit(); err != nil {
		fmt.Printf("%s failed to restart systemd unit 'gpud.service'\n", warningSign)
		return err
	}

	fmt.Printf("%s successfully started gpud (run 'gpud status' for checking status)\n", checkMark)
	return nil
}

func systemdInit() error {
	if err := systemd.CreateDefaultEnvFile(); err != nil {
		return err
	}
	systemdUnitFileData := systemd.GPUDService
	return os.WriteFile(systemd.DefaultUnitFile, []byte(systemdUnitFileData), 0644)
}
