package compact

import (
	"context"
	"fmt"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/urfave/cli"

	cmdcommon "github.com/leptonai/gpud/cmd/common"
	gpudcommon "github.com/leptonai/gpud/cmd/gpud/common"
	"github.com/leptonai/gpud/pkg/config"
	"github.com/leptonai/gpud/pkg/log"
	"github.com/leptonai/gpud/pkg/netutil"
	"github.com/leptonai/gpud/pkg/sqlite"
	"github.com/leptonai/gpud/pkg/systemd"
)

var (
	systemctlExists       = systemd.SystemctlExists
	systemdIsActive       = systemd.IsActive
	isPortOpen            = netutil.IsPortOpen
	stateFileFromContext  = gpudcommon.StateFileFromContext
	sqliteOpen            = sqlite.Open
	sqliteReadDBSize      = sqlite.ReadDBSize
	sqliteCompactDatabase = sqlite.Compact
)

func Command(cliContext *cli.Context) error {
	logLevel := cliContext.String("log-level")
	zapLvl, err := log.ParseLogLevel(logLevel)
	if err != nil {
		return err
	}
	log.SetLogger(log.CreateLogger(zapLvl, ""))

	log.Logger.Debugw("starting compact command")

	if systemctlExists() {
		active, err := systemdIsActive("gpud.service")
		if err != nil {
			return err
		}
		if active {
			return fmt.Errorf("gpud is running (must be stopped before running compact)")
		}
	}

	portOpen := isPortOpen(config.DefaultGPUdPort)
	if portOpen {
		return fmt.Errorf("gpud is running on port %d (must be stopped before running compact)", config.DefaultGPUdPort)
	}

	log.Logger.Infow("successfully checked gpud is not running")

	rootCtx, rootCancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer rootCancel()

	stateFile, err := stateFileFromContext(cliContext)
	if err != nil {
		return fmt.Errorf("failed to get state file: %w", err)
	}

	dbRW, err := sqliteOpen(stateFile)
	if err != nil {
		return fmt.Errorf("failed to open state file: %w", err)
	}
	defer func() {
		_ = dbRW.Close()
	}()

	dbRO, err := sqliteOpen(stateFile, sqlite.WithReadOnly(true))
	if err != nil {
		return fmt.Errorf("failed to open state file: %w", err)
	}
	defer func() {
		_ = dbRO.Close()
	}()

	dbSize, err := sqliteReadDBSize(rootCtx, dbRO)
	if err != nil {
		return fmt.Errorf("failed to read state file size: %w", err)
	}
	log.Logger.Infow("state file size before compact", "size", humanize.IBytes(dbSize))

	if err := sqliteCompactDatabase(rootCtx, dbRW); err != nil {
		return fmt.Errorf("failed to compact state file: %w", err)
	}

	dbSize, err = sqliteReadDBSize(rootCtx, dbRO)
	if err != nil {
		return fmt.Errorf("failed to read state file size: %w", err)
	}
	log.Logger.Infow("state file size after compact", "size", humanize.IBytes(dbSize))

	fmt.Printf("%s successfully compacted state file\n", cmdcommon.CheckMark)
	return nil
}
