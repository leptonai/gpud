// Package state provides the persistent storage layer for the events.
package state

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/leptonai/gpud/log"

	_ "github.com/mattn/go-sqlite3"
)

const (
	TableNameEventHistory = "components_file_descriptor_event_history"
)

const (
	// unix timestamp in seconds when the event was observed
	ColumnUnixSeconds = "unix_seconds"

	// "dmesg"
	ColumnDataSource = "data_source"

	// "vfs_file_max_limit_reached"
	ColumnEventType = "event_type"

	// dmesg log line
	ColumnEventDetails = "event_details"
)

const DefaultRetentionPeriod = 3 * time.Hour

type Event struct {
	UnixSeconds  int64
	DataSource   string
	EventType    string
	EventDetails string
}

func CreateTable(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS %s (
	%s INTEGER NOT NULL,
	%s TEXT NOT NULL,
	%s TEXT NOT NULL,
	%s TEXT
);`, TableNameEventHistory,
		ColumnUnixSeconds,
		ColumnDataSource,
		ColumnEventType,
		ColumnEventDetails,
	))
	return err
}

func InsertEvent(ctx context.Context, db *sql.DB, event Event) error {
	log.Logger.Debugw("inserting event", "dataSource", event.DataSource, "eventType", event.EventType, "details", event.EventDetails)

	insertStatement := fmt.Sprintf(`
INSERT OR REPLACE INTO %s (%s, %s, %s, %s) VALUES (?, ?, ?, NULLIF(?, ''));
`,
		TableNameEventHistory,
		ColumnUnixSeconds,
		ColumnDataSource,
		ColumnEventType,
		ColumnEventDetails,
	)
	_, err := db.ExecContext(
		ctx,
		insertStatement,
		event.UnixSeconds,
		event.DataSource,
		event.EventType,
		event.EventDetails,
	)
	return err
}

func FindEvent(ctx context.Context, db *sql.DB, event Event) (bool, error) {
	selectStatement := fmt.Sprintf(`
SELECT %s, %s, %s, %s FROM %s WHERE %s = ? AND %s = ? AND %s = ?;
`,
		ColumnUnixSeconds,
		ColumnDataSource,
		ColumnEventType,
		ColumnEventDetails,
		TableNameEventHistory,
		ColumnUnixSeconds,
		ColumnDataSource,
		ColumnEventType,
	)

	var foundEvent Event
	if err := db.QueryRowContext(
		ctx,
		selectStatement,
		event.UnixSeconds,
		event.DataSource,
		event.EventType,
	).Scan(
		&foundEvent.UnixSeconds,
		&foundEvent.DataSource,
		&foundEvent.EventType,
		&foundEvent.EventDetails,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}

	// event at the same time but with different details
	if foundEvent.EventDetails != "" && foundEvent.EventDetails != event.EventDetails {
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

	rows, err := db.QueryContext(ctx, selectStatement, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	events := []Event{}
	for rows.Next() {
		var event Event
		var details sql.NullString
		if err := rows.Scan(
			&event.UnixSeconds,
			&event.DataSource,
			&event.EventType,
			&details,
		); err != nil {
			return nil, err
		}
		if details.Valid {
			event.EventDetails = details.String
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
		ColumnEventDetails,
		TableNameEventHistory,
	)

	args := []any{}

	if op.sinceUnixSeconds > 0 {
		selectStatement += "\nWHERE "
		selectStatement += fmt.Sprintf("%s >= ?", ColumnUnixSeconds)
		args = append(args, op.sinceUnixSeconds)
	}

	if op.eventType != "" {
		if !strings.Contains(selectStatement, "WHERE") {
			selectStatement += "\nWHERE "
		} else {
			selectStatement += " AND "
		}
		selectStatement += fmt.Sprintf("%s = ?", ColumnEventType)
		args = append(args, op.eventType)
	}

	if op.sortUnixSecondsAscOrder {
		selectStatement += "\nORDER BY " + ColumnUnixSeconds + " ASC"
	} else {
		selectStatement += "\nORDER BY " + ColumnUnixSeconds + " DESC"
	}

	if op.limit > 0 {
		selectStatement += fmt.Sprintf("\nLIMIT %d", op.limit)
	}

	if len(args) == 0 {
		return selectStatement, nil, nil
	}
	return selectStatement, args, nil
}

func Purge(ctx context.Context, db *sql.DB, opts ...OpOption) (int, error) {
	log.Logger.Debugw("purging events")
	deleteStatement, args, err := createDeleteStatementAndArgs(opts...)
	if err != nil {
		return 0, err
	}
	rs, err := db.ExecContext(ctx, deleteStatement, args...)
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
		TableNameEventHistory,
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
