package command

import (
	"errors"
	"fmt"
	"os"

	"github.com/leptonai/gpud/pkg/gpud-manager/systemd"
	pkdsystemd "github.com/leptonai/gpud/pkg/systemd"
	pkgupdate "github.com/leptonai/gpud/pkg/update"

	"github.com/urfave/cli"
)

func cmdUp(cliContext *cli.Context) (retErr error) {
	if cliContext.String("token") != "" {
		if lerr := cmdLogin(cliContext); lerr != nil {
			fmt.Printf("%s failed to login (%v)\n", warningSign, lerr)
			return lerr
		}
		fmt.Printf("%s successfully logged in\n", checkMark)
	} else {
		fmt.Printf("\nvisit https://localhost:15132 to view the dashboard\n\n")
	}

	bin, err := os.Executable()
	if err != nil {
		return err
	}
	if err := pkgupdate.RequireRoot(); err != nil {
		fmt.Printf("%s %q requires root to run with systemd: %v (to run without systemd, '%s run')\n", warningSign, bin, err, bin)
		return err
	}
	if !pkdsystemd.SystemctlExists() {
		fmt.Printf("%s requires systemd, to run without systemd, '%s run'\n", warningSign, bin)
		return errors.ErrUnsupported
	}

	if !systemd.DefaultBinExists() {
		return fmt.Errorf("gpud binary not found at %s (you may run 'cp %s %s' to fix the installation)", systemd.DefaultBinPath, bin, systemd.DefaultBinPath)
	}

	endpoint := cliContext.String("endpoint")
	if err := systemdInit(endpoint); err != nil {
		fmt.Printf("%s failed to initialize systemd files\n", warningSign)
		return err
	}

	if err := pkgupdate.EnableGPUdSystemdUnit(); err != nil {
		fmt.Printf("%s failed to enable systemd unit 'gpud.service'\n", warningSign)
		return err
	}

	if err := pkgupdate.RestartGPUdSystemdUnit(); err != nil {
		fmt.Printf("%s failed to restart systemd unit 'gpud.service'\n", warningSign)
		return err
	}

	fmt.Printf("%s successfully started gpud (run 'gpud status' for checking status)\n", checkMark)
	return nil
}

func systemdInit(endpoint string) error {
	if err := systemd.CreateDefaultEnvFile(endpoint); err != nil {
		return err
	}
	systemdUnitFileData := systemd.GPUDService
	return os.WriteFile(systemd.DefaultUnitFile, []byte(systemdUnitFileData), 0644)
}
