package command

import (
	"fmt"
	"os"

	"github.com/urfave/cli"

	pkgupdate "github.com/leptonai/gpud/pkg/update"
)

func cmdDown(cliContext *cli.Context) error {
	bin, err := os.Executable()
	if err != nil {
		return err
	}
	if err := pkgupdate.RequireRoot(); err != nil {
		fmt.Printf("%s %q requires root to stop gpud (if not run by systemd, manually kill the process with 'pidof gpud')\n", warningSign, bin)
		return err
	}
	if err := pkgupdate.SystemctlExists(); err != nil {
		fmt.Printf("%s requires systemd: %v (if not run by systemd, manually kill the process with 'pidof gpud')\n", warningSign, err)
		return err
	}

	if err := pkgupdate.StopSystemdUnit(); err != nil {
		fmt.Printf("%s failed to stop systemd unit 'gpud.service': %v\n", warningSign, err)
		return err
	}

	fmt.Printf("%s successfully stopped gpud\n", checkMark)
	return nil
}
