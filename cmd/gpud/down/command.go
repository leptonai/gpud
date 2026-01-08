package down

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/urfave/cli"

	cmdcommon "github.com/leptonai/gpud/cmd/common"
	gpudcommon "github.com/leptonai/gpud/cmd/gpud/common"
	"github.com/leptonai/gpud/pkg/log"
	pkgmetadata "github.com/leptonai/gpud/pkg/metadata"
	"github.com/leptonai/gpud/pkg/osutil"
	"github.com/leptonai/gpud/pkg/sqlite"
	pkgsystemd "github.com/leptonai/gpud/pkg/systemd"
	pkgupdate "github.com/leptonai/gpud/pkg/update"
)

func Command(cliContext *cli.Context) error {
	logLevel := cliContext.String("log-level")
	zapLvl, err := log.ParseLogLevel(logLevel)
	if err != nil {
		return err
	}
	log.SetLogger(log.CreateLogger(zapLvl, ""))

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

	if cliContext.Bool("reset-state") {
		log.Logger.Warnw("resetting state")

		log.Logger.Debugw("getting state file")
		stateFile, err := gpudcommon.StateFileFromContext(cliContext)
		if err != nil {
			return fmt.Errorf("failed to get state file: %w", err)
		}
		log.Logger.Debugw("successfully got state file", "file", stateFile)

		log.Logger.Debugw("opening state file for writing")
		dbRW, err := sqlite.Open(stateFile)
		if err != nil {
			return fmt.Errorf("failed to open state file %q: %w", stateFile, err)
		}
		defer func() {
			_ = dbRW.Close()
		}()
		log.Logger.Debugw("successfully opened state file for writing")

		rootCtx, rootCancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer rootCancel()

		log.Logger.Debugw("deleting metadata data")
		if err := pkgmetadata.DeleteAllMetadata(rootCtx, dbRW); err != nil {
			return fmt.Errorf("failed to delete metadata: %w", err)
		}
		log.Logger.Warnw("successfully deleted metadata")

		// TODO: clean up other login related files
		// /etc/systemd/system/gpud.service
		// /etc/systemd/system/kubelet.service
		// /etc/systemd/system/tailscaled.service
	}

	return nil
}
