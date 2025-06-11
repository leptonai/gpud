package metadata

import (
	"context"
	"fmt"
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

	log.Logger.Debugw("starting metadata command")

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

	log.Logger.Debugw("opening state file for reading")
	dbRO, err := sqlite.Open(stateFile, sqlite.WithReadOnly(true))
	if err != nil {
		return fmt.Errorf("failed to open state file: %w", err)
	}
	defer dbRO.Close()
	log.Logger.Debugw("successfully opened state file for reading")

	metadata, err := pkgmetadata.ReadAllMetadata(rootCtx, dbRO)
	if err != nil {
		return fmt.Errorf("failed to read metadata: %w", err)
	}
	log.Logger.Debugw("successfully read metadata")

	for k, v := range metadata {
		if k == pkgmetadata.MetadataKeyToken {
			v = "[hidden]"
		}
		fmt.Printf("%s: %s\n", k, v)
	}

	setKey := cliContext.String("set-key")
	setValue := cliContext.String("set-value")
	if setKey == "" || setValue == "" { // no update/insert needed
		return nil
	}

	log.Logger.Debugw("opening state file for writing")
	dbRW, err := sqlite.Open(stateFile)
	if err != nil {
		return fmt.Errorf("failed to open state file: %w", err)
	}
	defer dbRW.Close()
	log.Logger.Debugw("successfully opened state file for writing")

	log.Logger.Debugw("deleting metadata data")
	if err := pkgmetadata.SetMetadata(rootCtx, dbRW, setKey, setValue); err != nil {
		return fmt.Errorf("failed to update metadata: %w", err)
	}
	log.Logger.Debugw("successfully updated metadata")

	fmt.Printf("%s successfully updated metadata\n", cmdcommon.CheckMark)
	return nil
}
