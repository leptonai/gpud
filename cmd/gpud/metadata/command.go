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

type rebootHistoryEntry struct {
	TimeUTC string `json:"time_utc"`
	Age     string `json:"age"`
	Message string `json:"message"`
}

type metadataUpdatedEntry struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type metadataJSONOutput struct {
	Metadata      map[string]string     `json:"metadata"`
	RebootHistory *[]rebootHistoryEntry `json:"reboot_history,omitempty"`
	Updated       *metadataUpdatedEntry `json:"updated,omitempty"`
}

func Command(cliContext *cli.Context) error {
	outputFormat, err := gpudcommon.ParseOutputFormat(cliContext.String("output-format"))
	if err != nil {
		return err
	}
	wrapErr := func(code string, srcErr error) error {
		return gpudcommon.WrapOutputError(outputFormat, code, srcErr)
	}

	logLevel := cliContext.String("log-level")
	zapLvl, err := log.ParseLogLevel(logLevel)
	if err != nil {
		return wrapErr("invalid_log_level", err)
	}
	if outputFormat == gpudcommon.OutputFormatJSON {
		log.SetLogger(nil)
	} else {
		log.SetLogger(log.CreateLogger(zapLvl, ""))
	}

	log.Logger.Debugw("starting metadata command")

	if err := osutil.RequireRoot(); err != nil {
		return wrapErr("require_root", err)
	}

	rootCtx, rootCancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer rootCancel()

	log.Logger.Debugw("getting state file")
	stateFile, err := gpudcommon.StateFileFromContext(cliContext)
	if err != nil {
		return wrapErr("failed_to_get_state_file", fmt.Errorf("failed to get state file: %w", err))
	}
	log.Logger.Debugw("successfully got state file")

	log.Logger.Debugw("opening state file for reading")
	dbRO, err := sqlite.Open(stateFile, sqlite.WithReadOnly(true))
	if err != nil {
		return wrapErr("failed_to_open_state_file", fmt.Errorf("failed to open state file: %w", err))
	}
	defer func() {
		_ = dbRO.Close()
	}()
	log.Logger.Debugw("successfully opened state file for reading")

	metadata, err := pkgmetadata.ReadAllMetadata(rootCtx, dbRO)
	if err != nil {
		return wrapErr("failed_to_read_metadata", fmt.Errorf("failed to read metadata: %w", err))
	}
	log.Logger.Debugw("successfully read metadata")

	maskedMetadata := make(map[string]string, len(metadata))
	for k, v := range metadata {
		if k == pkgmetadata.MetadataKeyToken {
			v = pkgmetadata.MaskToken(v)
		}
		maskedMetadata[k] = v
		if outputFormat == gpudcommon.OutputFormatPlain {
			fmt.Printf("%s: %s\n", k, v)
		}
	}

	showRebootHistory := cliContext.Bool("reboot-history")
	var rebootHistory []rebootHistoryEntry
	if showRebootHistory {
		rebootHistory, err = loadRebootHistory(rootCtx, dbRO, stateFile)
		if err != nil {
			return wrapErr("failed_to_load_reboot_history", err)
		}
		if outputFormat == gpudcommon.OutputFormatPlain {
			if err := displayRebootHistory(rebootHistory); err != nil {
				return wrapErr("failed_to_display_reboot_history", err)
			}
		}
	}

	setKey := cliContext.String("set-key")
	setValue := cliContext.String("set-value")
	var updated *metadataUpdatedEntry
	if setKey != "" && setValue != "" {
		log.Logger.Debugw("opening state file for writing")
		dbRW, err := sqlite.Open(stateFile)
		if err != nil {
			return wrapErr("failed_to_open_state_file_for_write", fmt.Errorf("failed to open state file: %w", err))
		}
		defer func() {
			_ = dbRW.Close()
		}()
		log.Logger.Debugw("successfully opened state file for writing")

		log.Logger.Debugw("setting metadata", "key", setKey, "value", setValue)
		if err := pkgmetadata.SetMetadata(rootCtx, dbRW, setKey, setValue); err != nil {
			return wrapErr("failed_to_update_metadata", fmt.Errorf("failed to update metadata: %w", err))
		}
		log.Logger.Debugw("successfully updated metadata")

		updated = &metadataUpdatedEntry{Key: setKey, Value: setValue}
		if outputFormat == gpudcommon.OutputFormatPlain {
			fmt.Printf("%s successfully updated metadata\n", cmdcommon.CheckMark)
		}
	}

	if outputFormat == gpudcommon.OutputFormatJSON {
		out := metadataJSONOutput{
			Metadata: maskedMetadata,
			Updated:  updated,
		}
		if showRebootHistory {
			history := rebootHistory
			out.RebootHistory = &history
		}
		return wrapErr("failed_to_write_json_output", gpudcommon.WriteJSON(out))
	}

	return nil
}

func loadRebootHistory(ctx context.Context, dbRO *sql.DB, stateFile string) ([]rebootHistoryEntry, error) {
	log.Logger.Debugw("opening state file for reboot history")
	dbRW, err := sqlite.Open(stateFile)
	if err != nil {
		return nil, fmt.Errorf("failed to open state file for reboot history: %w", err)
	}
	defer func() {
		_ = dbRW.Close()
	}()

	eventStore, err := eventstore.New(dbRW, dbRO, eventstore.DefaultRetention)
	if err != nil {
		return nil, fmt.Errorf("failed to open event store: %w", err)
	}

	rebootStore := pkghost.NewRebootEventStore(eventStore)
	log.Logger.Debugw("loading reboot history")
	events, err := rebootStore.GetRebootEvents(ctx, time.Time{})
	if err != nil {
		return nil, fmt.Errorf("failed to load reboot events: %w", err)
	}

	now := time.Now()
	history := make([]rebootHistoryEntry, 0, len(events))
	for _, ev := range events {
		history = append(history, rebootHistoryEntry{
			TimeUTC: ev.Time.UTC().Format(time.RFC3339),
			Age:     humanize.RelTime(ev.Time, now, "ago", "from now"),
			Message: ev.Message,
		})
	}

	return history, nil
}

func displayRebootHistory(events []rebootHistoryEntry) error {
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

	for _, ev := range events {
		if _, err := fmt.Fprintf(w, "%s\t%s\t%s\n", ev.TimeUTC, ev.Age, ev.Message); err != nil {
			return err
		}
	}

	if err := w.Flush(); err != nil {
		return fmt.Errorf("failed to flush reboot history table: %w", err)
	}

	fmt.Println()
	return nil
}
