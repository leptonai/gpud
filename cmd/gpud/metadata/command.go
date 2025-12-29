package metadata

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/urfave/cli"

	cmdcommon "github.com/leptonai/gpud/cmd/common"
	gpudcommon "github.com/leptonai/gpud/cmd/gpud/common"
	"github.com/leptonai/gpud/pkg/eventstore"
	pkghost "github.com/leptonai/gpud/pkg/host"
	"github.com/leptonai/gpud/pkg/log"
	pkgmetadata "github.com/leptonai/gpud/pkg/metadata"
	"github.com/leptonai/gpud/pkg/osutil"
	"github.com/leptonai/gpud/pkg/sqlite"
)

var requireRoot = osutil.RequireRoot

func Command(cliContext *cli.Context) error {
	logLevel := cliContext.String("log-level")
	zapLvl, err := log.ParseLogLevel(logLevel)
	if err != nil {
		return err
	}
	log.SetLogger(log.CreateLogger(zapLvl, ""))

	log.Logger.Debugw("starting metadata command")

	if err := requireRoot(); err != nil {
		return err
	}

	rootCtx, rootCancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer rootCancel()

	log.Logger.Debugw("getting state file")
	stateFile, err := gpudcommon.StateFileFromContext(cliContext)
	if err != nil {
		return fmt.Errorf("failed to get state file: %w", err)
	}
	log.Logger.Debugw("successfully got state file")

	log.Logger.Debugw("opening state file for reading")
	dbRO, err := sqlite.Open(stateFile, sqlite.WithReadOnly(true))
	if err != nil {
		return fmt.Errorf("failed to open state file: %w", err)
	}
	defer func() {
		_ = dbRO.Close()
	}()
	log.Logger.Debugw("successfully opened state file for reading")

	metadata, err := pkgmetadata.ReadAllMetadata(rootCtx, dbRO)
	if err != nil {
		return fmt.Errorf("failed to read metadata: %w", err)
	}
	log.Logger.Debugw("successfully read metadata")

	for k, v := range metadata {
		if k == pkgmetadata.MetadataKeyToken {
			v = pkgmetadata.MaskToken(v)
		}
		fmt.Printf("%s: %s\n", k, v)
	}

	showRebootHistory := cliContext.Bool("reboot-history")
	if showRebootHistory {
		if err := displayRebootHistory(rootCtx, dbRO, stateFile); err != nil {
			return err
		}
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
	defer func() {
		_ = dbRW.Close()
	}()
	log.Logger.Debugw("successfully opened state file for writing")

	log.Logger.Debugw("setting metadata", "key", setKey, "value", setValue)
	if err := pkgmetadata.SetMetadata(rootCtx, dbRW, setKey, setValue); err != nil {
		return fmt.Errorf("failed to update metadata: %w", err)
	}
	log.Logger.Debugw("successfully updated metadata")

	fmt.Printf("%s successfully updated metadata\n", cmdcommon.CheckMark)
	return nil
}

func displayRebootHistory(ctx context.Context, dbRO *sql.DB, stateFile string) error {
	log.Logger.Debugw("opening state file for reboot history")
	dbRW, err := sqlite.Open(stateFile)
	if err != nil {
		return fmt.Errorf("failed to open state file for reboot history: %w", err)
	}
	defer func() {
		_ = dbRW.Close()
	}()

	eventStore, err := eventstore.New(dbRW, dbRO, eventstore.DefaultRetention)
	if err != nil {
		return fmt.Errorf("failed to open event store: %w", err)
	}

	rebootStore := pkghost.NewRebootEventStore(eventStore)
	log.Logger.Debugw("loading reboot history")
	events, err := rebootStore.GetRebootEvents(ctx, time.Time{})
	if err != nil {
		return fmt.Errorf("failed to load reboot events: %w", err)
	}

	fmt.Println()
	fmt.Println("Reboot History:")
	if len(events) == 0 {
		fmt.Println("  (no reboot events recorded)")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(w, "TIME (UTC)\tAGE\tMESSAGE"); err != nil {
		return err
	}

	now := time.Now()
	for _, ev := range events {
		age := humanize.RelTime(ev.Time, now, "ago", "from now")
		if _, err := fmt.Fprintf(w, "%s\t%s\t%s\n", ev.Time.UTC().Format(time.RFC3339), age, ev.Message); err != nil {
			return err
		}
	}

	if err := w.Flush(); err != nil {
		return fmt.Errorf("failed to flush reboot history table: %w", err)
	}

	fmt.Println()
	return nil
}
