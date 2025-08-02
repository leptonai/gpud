package logout

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/urfave/cli"

	cmdcommon "github.com/leptonai/gpud/cmd/common"
	"github.com/leptonai/gpud/pkg/config"
	"github.com/leptonai/gpud/pkg/log"
	pkgmetadata "github.com/leptonai/gpud/pkg/metadata"
	"github.com/leptonai/gpud/pkg/osutil"
	"github.com/leptonai/gpud/pkg/sqlite"
)

func Command(cliContext *cli.Context) error {
	logLevel := cliContext.String("log-level")
	zapLvl, err := log.ParseLogLevel(logLevel)
	if err != nil {
		return err
	}
	log.Logger = log.CreateLogger(zapLvl, "")

	log.Logger.Debugw("starting logout command")

	if err := osutil.RequireRoot(); err != nil {
		return err
	}

	rootCtx, rootCancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer rootCancel()

	log.Logger.Debugw("getting state file")
	stateFile, err := config.DefaultStateFile()
	if err != nil {
		return fmt.Errorf("failed to get state file: %w", err)
	}
	log.Logger.Debugw("successfully got state file")

	log.Logger.Debugw("opening state file for writing")
	dbRW, err := sqlite.Open(stateFile)
	if err != nil {
		return fmt.Errorf("failed to open state file: %w", err)
	}
	defer dbRW.Close()
	log.Logger.Debugw("successfully opened state file for writing")

	log.Logger.Debugw("deleting metadata data")
	if err := pkgmetadata.DeleteAllMetadata(rootCtx, dbRW); err != nil {
		return fmt.Errorf("failed to delete metadata: %w", err)
	}
	log.Logger.Debugw("successfully deleted metadata")

	fmt.Printf("%s successfully logged out\n", cmdcommon.CheckMark)
	fmt.Printf("\nPlease run 'rm -rf %s/gpud*' to remove the state file (otherwise, re-login may contain stale health data)\n\n", filepath.Dir(stateFile))

	if cliContext.Bool("reset-state") {
		log.Logger.Warnw("deleting state files", "state-file", stateFile)
		if err := os.RemoveAll(stateFile); err != nil {
			return err
		}
		if err := os.RemoveAll(stateFile + "-wal"); err != nil {
			return err
		}
		if err := os.RemoveAll(stateFile + "-shm"); err != nil {
			return err
		}
		if err := os.RemoveAll(filepath.Join(filepath.Dir(stateFile), "packages")); err != nil {
			return err
		}
		log.Logger.Infow("successfully deleted state files", "state-file", stateFile)
	}

	return nil
}
