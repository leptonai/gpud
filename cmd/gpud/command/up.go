package command

import (
	"fmt"
	"os"

	pkgupdate "github.com/leptonai/gpud/pkg/update"
	"github.com/leptonai/gpud/systemd"

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
	if err := pkgupdate.RequireRoot(); err != nil {
		fmt.Printf("%s %q requires root to run with systemd: %v (to run without systemd, '%s run')\n", warningSign, bin, err, bin)
		return err
	}
	if err := pkgupdate.SystemctlExists(); err != nil {
		fmt.Printf("%s requires systemd: %v (to run without systemd, '%s run')\n", warningSign, err, bin)
		return err
	}

	if !systemd.DefaultBinExists() {
		return fmt.Errorf("gpud binary not found at %s (you may run 'cp %s %s' to fix the installation)", systemd.DefaultBinPath, bin, systemd.DefaultBinPath)
	}

	if err := systemdInit(); err != nil {
		fmt.Printf("%s failed to initialize systemd files\n", warningSign)
		return err
	}

	if err := systemd.LogrotateInit(); err != nil {
		fmt.Printf("%s failed to initialize logrotate for gpud log\n", warningSign)
		return err
	}

	if err := pkgupdate.RestartSystemdUnit(); err != nil {
		fmt.Printf("%s failed to restart systemd unit 'gpud.service'\n", warningSign)
		return err
	}

	fmt.Printf("%s successfully started gpud (run 'gpud status' or 'gpud logs' for checking status)\n", checkMark)
	return nil
}

func systemdInit() error {
	if err := systemd.CreateDefaultEnvFile(); err != nil {
		return err
	}
	systemdUnitFileData := systemd.GPUDService
	return os.WriteFile(systemd.DefaultUnitFile, []byte(systemdUnitFileData), 0644)
}
