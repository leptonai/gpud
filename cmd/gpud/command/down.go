package command

import (
	"fmt"
	"os"

	gpud_update "github.com/leptonai/gpud/pkg/gpud-update"
	pkg_systemd "github.com/leptonai/gpud/pkg/systemd"

	"github.com/urfave/cli"
)

func cmdDown(cliContext *cli.Context) error {
	bin, err := os.Executable()
	if err != nil {
		return err
	}
	if err := gpud_update.RequireRoot(); err != nil {
		fmt.Printf("%s %q requires root to stop gpud (if not run by systemd, manually kill the process with 'pidof gpud')\n", warningSign, bin)
		os.Exit(1)
	}
	if !pkg_systemd.SystemctlExists() {
		fmt.Printf("%s requires systemd, if not run by systemd, manually kill the process with 'pidof gpud'\n", warningSign)
		os.Exit(1)
	}

	active, err := pkg_systemd.IsActive("gpud.service")
	if err != nil {
		fmt.Printf("%s failed to check if gpud is running: %v\n", warningSign, err)
		os.Exit(1)
	}
	if !active {
		fmt.Printf("%s gpud is not running (no-op)\n", checkMark)
		os.Exit(0)
	}

	if err := gpud_update.StopSystemdUnit(); err != nil {
		fmt.Printf("%s failed to stop systemd unit 'gpud.service': %v\n", warningSign, err)
		os.Exit(1)
	}

	if err := gpud_update.DisableSystemdUnit(); err != nil {
		fmt.Printf("%s failed to disable systemd unit 'gpud.service': %v\n", warningSign, err)
		os.Exit(1)
	}

	fmt.Printf("%s successfully stopped gpud\n", checkMark)
	return nil
}
