package down

import (
	"fmt"
	"os"

	"github.com/urfave/cli"

	cmdcommon "github.com/leptonai/gpud/cmd/common"
	"github.com/leptonai/gpud/pkg/log"
	"github.com/leptonai/gpud/pkg/osutil"
	pkgsystemd "github.com/leptonai/gpud/pkg/systemd"
	pkgupdate "github.com/leptonai/gpud/pkg/update"
)

func Command(cliContext *cli.Context) error {
	logLevel := cliContext.String("log-level")
	zapLvl, err := log.ParseLogLevel(logLevel)
	if err != nil {
		return err
	}
	log.Logger = log.CreateLogger(zapLvl, "")

	log.Logger.Debugw("starting down command")

	if err := osutil.RequireRoot(); err != nil {
		return err
	}

	if !pkgsystemd.SystemctlExists() {
		fmt.Printf("%s requires systemd, if not run by systemd, manually kill the process with 'pidof gpud'\n", cmdcommon.WarningSign)
		os.Exit(1)
	}

	active, err := pkgsystemd.IsActive("gpud.service")
	if err != nil {
		fmt.Printf("%s failed to check if gpud is running: %v\n", cmdcommon.WarningSign, err)
		os.Exit(1)
	}
	if !active {
		fmt.Printf("%s gpud is not running (no-op)\n", cmdcommon.CheckMark)
		os.Exit(0)
	}

	if err := pkgupdate.StopSystemdUnit(); err != nil {
		fmt.Printf("%s failed to stop systemd unit 'gpud.service': %v\n", cmdcommon.WarningSign, err)
		os.Exit(1)
	}

	if err := pkgupdate.DisableGPUdSystemdUnit(); err != nil {
		fmt.Printf("%s failed to disable systemd unit 'gpud.service': %v\n", cmdcommon.WarningSign, err)
		os.Exit(1)
	}

	fmt.Printf("%s successfully stopped gpud\n", cmdcommon.CheckMark)
	return nil
}
