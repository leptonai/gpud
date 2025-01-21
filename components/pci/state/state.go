// Package state provides the persistent storage layer for the PCI query results.
package state

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"time"

	"github.com/leptonai/gpud/log"
	"github.com/leptonai/gpud/pkg/sqlite"

	_ "github.com/mattn/go-sqlite3"
)

const TableNamePCIEvents = "components_pci_events"

const (
	// unix timestamp in seconds when the event was observed
	ColumnUnixSeconds = "unix_seconds"

	// "lspci -vv"
	ColumnDataSource = "data_source"

	// "acs_enabled"
	ColumnEventType = "event_type"

	// reasons for the events
	ColumnReasons = "reasons"
)

// retain up to 3 days of events
const DefaultRetentionPeriod = 3 * 24 * time.Hour

type Event struct {
	UnixSeconds int64
	DataSource  string
	EventType   string
	Reasons     []string
}

func CreateTable(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS %s (
	%s INTEGER NOT NULL,
	%s TEXT NOT NULL,
	%s TEXT NOT NULL,
	%s TEXT NOT NULL
);`, TableNamePCIEvents,
		ColumnUnixSeconds,
		ColumnDataSource,
		ColumnEventType,
		ColumnReasons,
	))
	return err
}

func InsertEvent(ctx context.Context, db *sql.DB, event Event) error {
	log.Logger.Debugw("inserting event", "dataSource", event.DataSource, "eventType", event.EventType, "reasons", event.Reasons)

	insertStatement := fmt.Sprintf(`
INSERT OR REPLACE INTO %s (%s, %s, %s, %s) VALUES (?, ?, ?, NULLIF(?, ''));
`,
		TableNamePCIEvents,
		ColumnUnixSeconds,
		ColumnDataSource,
		ColumnEventType,
		ColumnReasons,
	)

	reasonsBytes, err := json.Marshal(event.Reasons)
	if err != nil {
		return err
	}

	start := time.Now()
	_, err = db.ExecContext(
		ctx,
		insertStatement,
		event.UnixSeconds,
		event.DataSource,
		event.EventType,
		string(reasonsBytes),
	)
	sqlite.RecordInsertUpdate(time.Since(start).Seconds())

	return err
}

func FindEvent(ctx context.Context, db *sql.DB, event Event) (bool, error) {
	selectStatement := fmt.Sprintf(`
SELECT %s, %s, %s, %s FROM %s WHERE %s = ? AND %s = ? AND %s = ?;
`,
		ColumnUnixSeconds,
		ColumnDataSource,
		ColumnEventType,
		ColumnReasons,
		TableNamePCIEvents,
		ColumnUnixSeconds,
		ColumnDataSource,
		ColumnEventType,
	)

	start := time.Now()
	var foundEvent Event
	var reasonsRaw string
	err := db.QueryRowContext(
		ctx,
		selectStatement,
		event.UnixSeconds,
		event.DataSource,
		event.EventType,
	).Scan(
		&foundEvent.UnixSeconds,
		&foundEvent.DataSource,
		&foundEvent.EventType,
		&reasonsRaw,
	)
	sqlite.RecordSelect(time.Since(start).Seconds())

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	if err := json.Unmarshal([]byte(reasonsRaw), &foundEvent.Reasons); err != nil {
		return false, err
	}

	sort.Strings(foundEvent.Reasons)
	sort.Strings(event.Reasons)

	// event at the same time but with different details
	if len(foundEvent.Reasons) > 0 && !reflect.DeepEqual(foundEvent.Reasons, event.Reasons) {
		return false, nil
	}

	// found event
	// e.g., same messages in dmesg
	return true, nil
}

// Returns nil if no event is found.
func ReadEvents(ctx context.Context, db *sql.DB, opts ...OpOption) ([]Event, error) {
	selectStatement, args, err := createSelectStatementAndArgs(opts...)
	if err != nil {
		return nil, err
	}

	start := time.Now()
	defer func() {
		sqlite.RecordSelect(time.Since(start).Seconds())
	}()

	rows, err := db.QueryContext(ctx, selectStatement, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	op := &Op{}
	if err := op.applyOpts(opts); err != nil {
		return nil, err
	}

	events := []Event{}
	for rows.Next() {
		var event Event
		var reasonsRaw string
		if err := rows.Scan(
			&event.UnixSeconds,
			&event.DataSource,
			&event.EventType,
			&reasonsRaw,
		); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(reasonsRaw), &event.Reasons); err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(events) == 0 {
		return nil, nil
	}

	return events, nil
}

func createSelectStatementAndArgs(opts ...OpOption) (string, []any, error) {
	op := &Op{}
	if err := op.applyOpts(opts); err != nil {
		return "", nil, err
	}

	selectStatement := fmt.Sprintf(`SELECT %s, %s, %s, %s
FROM %s`,
		ColumnUnixSeconds,
		ColumnDataSource,
		ColumnEventType,
		ColumnReasons,
		TableNamePCIEvents,
	)

	args := []any{}

	if op.sinceUnixSeconds > 0 {
		selectStatement += "\nWHERE "
		selectStatement += fmt.Sprintf("%s >= ?", ColumnUnixSeconds)
		args = append(args, op.sinceUnixSeconds)
	}

	// sort by unix seconds and data source
	// data source is sorted in reverse order so that "nvml" returns before "nvidia-smi"
	// for the same unix second and event type
	selectStatement += "\nORDER BY " + ColumnUnixSeconds
	if op.sortUnixSecondsAscOrder {
		selectStatement += " ASC"
	} else {
		selectStatement += " DESC"
	}
	selectStatement += ", " + ColumnDataSource + " DESC"

	if op.limit > 0 {
		selectStatement += fmt.Sprintf("\nLIMIT %d", op.limit)
	}

	if len(args) == 0 {
		return selectStatement, nil, nil
	}
	return selectStatement, args, nil
}

func Purge(ctx context.Context, db *sql.DB, opts ...OpOption) (int, error) {
	log.Logger.Debugw("purging pci events")
	deleteStatement, args, err := createDeleteStatementAndArgs(opts...)
	if err != nil {
		return 0, err
	}

	start := time.Now()
	rs, err := db.ExecContext(ctx, deleteStatement, args...)
	sqlite.RecordDelete(time.Since(start).Seconds())

	if err != nil {
		return 0, err
	}

	affected, err := rs.RowsAffected()
	if err != nil {
		return 0, err
	}
	return int(affected), nil
}

// ignores order by and limit
func createDeleteStatementAndArgs(opts ...OpOption) (string, []any, error) {
	op := &Op{}
	if err := op.applyOpts(opts); err != nil {
		return "", nil, err
	}

	deleteStatement := fmt.Sprintf(`DELETE FROM %s`,
		TableNamePCIEvents,
	)

	args := []any{}

	if op.beforeUnixSeconds > 0 {
		deleteStatement += " WHERE "
		deleteStatement += fmt.Sprintf("%s < ?", ColumnUnixSeconds)
		args = append(args, op.beforeUnixSeconds)
	}

	if len(args) == 0 {
		return deleteStatement, nil, nil
	}
	return deleteStatement, args, nil
}
