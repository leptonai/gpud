// Package xidsxidstate provides the persistent storage layer for the nvidia query results.
package xidsxidstate

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	nvidia_query_sxid "github.com/leptonai/gpud/components/accelerator/nvidia/query/sxid"
	nvidia_query_xid "github.com/leptonai/gpud/components/accelerator/nvidia/query/xid"
	"github.com/leptonai/gpud/log"

	_ "github.com/mattn/go-sqlite3"
)

const TableNameXidSXidEventHistory = "components_accelerator_nvidia_query_xid_sxid_event_history"

const (
	// unix timestamp in seconds when the event was observed
	ColumnUnixSeconds = "unix_seconds"

	// either "nvml" or "dmesg"
	ColumnDataSource = "data_source"

	// either "xid" or "sxid"
	ColumnEventType = "event_type"

	// event id; xid or sxid
	ColumnEventID = "event_id"

	// event details; dmesg log line
	ColumnEventDetails = "event_details"
)

type Event struct {
	UnixSeconds  int64
	DataSource   string
	EventType    string
	EventID      int64
	EventDetails string
}

func (e Event) ToXidDetail() *nvidia_query_xid.Detail {
	if e.EventType != "xid" {
		return nil
	}
	d, ok := nvidia_query_xid.GetDetail(int(e.EventID))
	if !ok {
		return nil
	}
	return d
}

func (e Event) ToSXidDetail() *nvidia_query_sxid.Detail {
	if e.EventType != "sxid" {
		return nil
	}
	d, ok := nvidia_query_sxid.GetDetail(int(e.EventID))
	if !ok {
		return nil
	}
	return d
}

func CreateTableXidSXidEventHistory(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS %s (
	%s INTEGER NOT NULL,
	%s TEXT NOT NULL,
	%s TEXT NOT NULL,
	%s INTEGER NOT NULL,
	%s TEXT
);`, TableNameXidSXidEventHistory,
		ColumnUnixSeconds,
		ColumnDataSource,
		ColumnEventType,
		ColumnEventID,
		ColumnEventDetails,
	))
	return err
}

func InsertEvent(ctx context.Context, db *sql.DB, event Event) error {
	log.Logger.Infow("inserting event", "dataSource", event.DataSource, "eventType", event.EventType, "eventID", event.EventID, "details", event.EventDetails)

	insertStatement := fmt.Sprintf(`
INSERT OR REPLACE INTO %s (%s, %s, %s, %s, %s) VALUES (?, ?, ?, ?, NULLIF(?, ''));
`,
		TableNameXidSXidEventHistory,
		ColumnUnixSeconds,
		ColumnDataSource,
		ColumnEventType,
		ColumnEventID,
		ColumnEventDetails,
	)
	_, err := db.ExecContext(
		ctx,
		insertStatement,
		event.UnixSeconds,
		event.DataSource,
		event.EventType,
		event.EventID,
		event.EventDetails,
	)
	return err
}

func FindEvent(ctx context.Context, db *sql.DB, event Event) (bool, error) {
	selectStatement := fmt.Sprintf(`
SELECT %s, %s, %s, %s, %s FROM %s WHERE %s = ? AND %s = ? AND %s = ? AND %s = ?;
`,
		ColumnUnixSeconds,
		ColumnDataSource,
		ColumnEventType,
		ColumnEventID,
		ColumnEventDetails,
		TableNameXidSXidEventHistory,
		ColumnUnixSeconds,
		ColumnDataSource,
		ColumnEventType,
		ColumnEventID,
	)

	var foundEvent Event
	if err := db.QueryRowContext(
		ctx,
		selectStatement,
		event.UnixSeconds,
		event.DataSource,
		event.EventType,
		event.EventID,
	).Scan(
		&foundEvent.UnixSeconds,
		&foundEvent.DataSource,
		&foundEvent.EventType,
		&foundEvent.EventID,
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
		if err := rows.Scan(
			&event.UnixSeconds,
			&event.DataSource,
			&event.EventType,
			&event.EventID,
			&event.EventDetails,
		); err != nil {
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

	selectStatement := fmt.Sprintf(`SELECT %s, %s, %s, %s, %s
FROM %s`,
		ColumnUnixSeconds,
		ColumnDataSource,
		ColumnEventType,
		ColumnEventID,
		ColumnEventDetails,
		TableNameXidSXidEventHistory,
	)

	args := []any{}

	if op.sinceUnixSeconds > 0 {
		selectStatement += "\nWHERE "
		selectStatement += fmt.Sprintf("%s >= ?", ColumnUnixSeconds)
		args = append(args, op.sinceUnixSeconds)
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
		TableNameXidSXidEventHistory,
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
