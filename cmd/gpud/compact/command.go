package compact

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/urfave/cli"

	cmdcommon "github.com/leptonai/gpud/cmd/common"
	"github.com/leptonai/gpud/pkg/config"
	"github.com/leptonai/gpud/pkg/eventstore"
	pkghost "github.com/leptonai/gpud/pkg/host"
	"github.com/leptonai/gpud/pkg/log"
	pkgmetricsstore "github.com/leptonai/gpud/pkg/metrics/store"
	"github.com/leptonai/gpud/pkg/netutil"
	"github.com/leptonai/gpud/pkg/sqlite"
	"github.com/leptonai/gpud/pkg/systemd"
)

func Command(cliContext *cli.Context) error {
	logLevel := cliContext.String("log-level")
	zapLvl, err := log.ParseLogLevel(logLevel)
	if err != nil {
		return err
	}
	log.Logger = log.CreateLogger(zapLvl, "")

	log.Logger.Debugw("starting compact command")

	if systemd.SystemctlExists() {
		active, err := systemd.IsActive("gpud.service")
		if err != nil {
			return err
		}
		if active {
			return fmt.Errorf("gpud is running (must be stopped before running compact)")
		}
	}

	portOpen := netutil.IsPortOpen(config.DefaultGPUdPort)
	if portOpen {
		return fmt.Errorf("gpud is running on port %d (must be stopped before running compact)", config.DefaultGPUdPort)
	}

	log.Logger.Infow("successfully checked gpud is not running")

	rootCtx, rootCancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer rootCancel()

	stateFile, err := config.DefaultStateFile()
	if err != nil {
		return fmt.Errorf("failed to get state file: %w", err)
	}

	dbRW, err := sqlite.Open(stateFile)
	if err != nil {
		return fmt.Errorf("failed to open state file: %w", err)
	}
	defer dbRW.Close()

	dbRO, err := sqlite.Open(stateFile, sqlite.WithReadOnly(true))
	if err != nil {
		return fmt.Errorf("failed to open state file: %w", err)
	}
	defer dbRO.Close()

	dbSize, err := sqlite.ReadDBSize(rootCtx, dbRO)
	if err != nil {
		return fmt.Errorf("failed to read state file size: %w", err)
	}
	log.Logger.Infow("state file size before compact", "size", humanize.Bytes(dbSize))

	purgeEvents := cliContext.String("purge-events")
	if purgeEvents != "" {
		componentNames := strings.Split(purgeEvents, ",")
		if purgeEvents == "*" {
			componentNames = []string{}
		}

		now := time.Now().UTC()
		now = now.Add(24 * time.Hour)

		if err := eventstore.PurgeByComponents(rootCtx, dbRW, dbRO, now.Unix(), componentNames...); err != nil {
			return fmt.Errorf("failed to purge events: %w", err)
		}
		log.Logger.Infow("purged events", "componentNames", componentNames)
	}

	purgeRebootEvents := cliContext.Bool("purge-reboot-events")
	if purgeRebootEvents {
		eventStore, err := eventstore.New(dbRW, dbRO, 0)
		if err != nil {
			return fmt.Errorf("failed to create event store: %w", err)
		}
		rebootEventStore := pkghost.NewRebootEventStore(eventStore)
		if err := rebootEventStore.PurgeAll(rootCtx); err != nil {
			return fmt.Errorf("failed to purge reboot events: %w", err)
		}
		log.Logger.Infow("purged reboot events")
	}

	purgeMetrics := cliContext.Bool("purge-metrics")
	if purgeMetrics {
		metricsSQLiteStore, err := pkgmetricsstore.NewSQLiteStore(rootCtx, dbRW, dbRO, pkgmetricsstore.DefaultTableName)
		if err != nil {
			return fmt.Errorf("failed to create metrics store: %w", err)
		}
		purged, err := metricsSQLiteStore.Purge(rootCtx, time.Now())
		if err != nil {
			return fmt.Errorf("failed to purge metrics: %w", err)
		}
		log.Logger.Infow("purged metrics", "purged", purged)
	}

	if err := sqlite.Compact(rootCtx, dbRW); err != nil {
		return fmt.Errorf("failed to compact state file: %w", err)
	}

	dbSize, err = sqlite.ReadDBSize(rootCtx, dbRO)
	if err != nil {
		return fmt.Errorf("failed to read state file size: %w", err)
	}
	log.Logger.Infow("state file size after compact", "size", humanize.Bytes(dbSize))

	fmt.Printf("%s successfully compacted state file\n", cmdcommon.CheckMark)
	return nil
}
