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

const TableNameEvents = "components_events_v0"

const (
	// unix timestamp in seconds when the event was observed
	EventsTableColumnTimestamp = "timestamp"

	// "vfs_file_max_limit_reached", "xid", "sxid"
	EventsTableColumnEventType = "event_type"

	// "dmesg", "nvml", "nvidia-smi"
	EventsTableColumnDataSource = "data_source"

	// "GPU UUID"
	EventsTableColumnTarget = "target"

	// "dmesg log line"
	EventsTableColumnEventDetails = "event_details"
)

// 3 days
const DefaultRetentionPeriodForEvents = 3 * 24 * time.Hour

type Event struct {
	Timestamp    int64
	EventType    string
	DataSource   string
	Target       string
	EventDetails string
}

func CreateEventsTable(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, fmt.Sprintf(`
BEGIN;
CREATE TABLE IF NOT EXISTS %s (
	%s INTEGER NOT NULL,
	%s TEXT NOT NULL,
	%s TEXT NOT NULL,
	%s TEXT,
	%s TEXT
);
CREATE INDEX IF NOT EXISTS idx_%s ON %s (%s);
CREATE INDEX IF NOT EXISTS idx_%s ON %s (%s);
CREATE INDEX IF NOT EXISTS idx_%s ON %s (%s);
COMMIT;
`, TableNameEvents,
		EventsTableColumnTimestamp,
		EventsTableColumnEventType,
		EventsTableColumnDataSource,
		EventsTableColumnTarget,
		EventsTableColumnEventDetails,

		EventsTableColumnTimestamp,
		TableNameEvents,
		EventsTableColumnTimestamp,

		EventsTableColumnEventType,
		TableNameEvents,
		EventsTableColumnEventType,

		EventsTableColumnDataSource,
		TableNameEvents,
		EventsTableColumnDataSource,
	))
	return err
}

func InsertEvent(ctx context.Context, db *sql.DB, event Event) error {
	log.Logger.Debugw("inserting event", "dataSource", event.DataSource, "eventType", event.EventType, "details", event.EventDetails)

	insertStatement := fmt.Sprintf(`
INSERT OR REPLACE INTO %s (%s, %s, %s, %s, %s) VALUES (?, ?, ?, NULLIF(?, ''), NULLIF(?, ''));
`,
		TableNameEvents,
		EventsTableColumnTimestamp,
		EventsTableColumnEventType,
		EventsTableColumnDataSource,
		EventsTableColumnTarget,
		EventsTableColumnEventDetails,
	)
	_, err := db.ExecContext(
		ctx,
		insertStatement,
		event.Timestamp,
		event.EventType,
		event.DataSource,
		event.Target,
		event.EventDetails,
	)
	return err
}

func FindEvent(ctx context.Context, db *sql.DB, event Event) (bool, error) {
	selectStatement := fmt.Sprintf(`
SELECT %s, %s, %s, %s, %s FROM %s WHERE %s = ? AND %s = ? AND %s = ? AND %s = ?;
`,
		EventsTableColumnTimestamp,
		EventsTableColumnEventType,
		EventsTableColumnDataSource,
		EventsTableColumnTarget,
		EventsTableColumnEventDetails,
		TableNameEvents,
		EventsTableColumnTimestamp,
		EventsTableColumnEventType,
		EventsTableColumnDataSource,
		EventsTableColumnTarget,
	)

	var foundEvent Event
	if err := db.QueryRowContext(
		ctx,
		selectStatement,
		event.Timestamp,
		event.EventType,
		event.DataSource,
		event.Target,
	).Scan(
		&foundEvent.Timestamp,
		&foundEvent.EventType,
		&foundEvent.DataSource,
		&foundEvent.Target,
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

		var target sql.NullString
		var eventDetails sql.NullString
		if err := rows.Scan(
			&event.Timestamp,
			&event.EventType,
			&event.DataSource,
			&target,
			&eventDetails,
		); err != nil {
			return nil, err
		}

		if target.Valid {
			event.Target = target.String
		}
		if eventDetails.Valid {
			event.EventDetails = eventDetails.String
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

	selectStatement := fmt.Sprintf(`SELECT %s, %s, %s, %s, %s
FROM %s`,
		EventsTableColumnTimestamp,
		EventsTableColumnEventType,
		EventsTableColumnDataSource,
		EventsTableColumnTarget,
		EventsTableColumnEventDetails,
		TableNameEvents,
	)

	args := []any{}

	if op.sinceUnixSeconds > 0 {
		selectStatement += "\nWHERE "
		selectStatement += fmt.Sprintf("%s >= ?", EventsTableColumnTimestamp)
		args = append(args, op.sinceUnixSeconds)
	}

	if op.eventType != "" {
		if !strings.Contains(selectStatement, "WHERE") {
			selectStatement += "\nWHERE "
		} else {
			selectStatement += " AND "
		}
		selectStatement += fmt.Sprintf("%s = ?", EventsTableColumnEventType)
		args = append(args, op.eventType)
	}

	if op.dataSource != "" {
		if !strings.Contains(selectStatement, "WHERE") {
			selectStatement += "\nWHERE "
		} else {
			selectStatement += " AND "
		}
		selectStatement += fmt.Sprintf("%s = ?", EventsTableColumnDataSource)
		args = append(args, op.dataSource)
	}

	if op.target != "" {
		if !strings.Contains(selectStatement, "WHERE") {
			selectStatement += "\nWHERE "
		} else {
			selectStatement += " AND "
		}
		selectStatement += fmt.Sprintf("%s = ?", EventsTableColumnTarget)
		args = append(args, op.target)
	}

	if op.sortTimestampAscOrder {
		selectStatement += "\nORDER BY " + EventsTableColumnTimestamp + " ASC"
	} else {
		selectStatement += "\nORDER BY " + EventsTableColumnTimestamp + " DESC"
	}

	if op.limit > 0 {
		selectStatement += fmt.Sprintf("\nLIMIT %d", op.limit)
	}

	if len(args) == 0 {
		return selectStatement, nil, nil
	}
	return selectStatement, args, nil
}

func PurgeEvents(ctx context.Context, db *sql.DB, opts ...OpOption) (int, error) {
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
		TableNameEvents,
	)

	args := []any{}

	if op.beforeUnixSeconds > 0 {
		deleteStatement += " WHERE "
		deleteStatement += fmt.Sprintf("%s < ?", EventsTableColumnTimestamp)
		args = append(args, op.beforeUnixSeconds)
	}

	if len(args) == 0 {
		return deleteStatement, nil, nil
	}
	return deleteStatement, args, nil
}
