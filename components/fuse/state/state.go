package state

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/leptonai/gpud/log"
	"github.com/leptonai/gpud/pkg/sqlite"

	_ "github.com/mattn/go-sqlite3"
)

const TableNameFUSEConnectionsEventHistory = "components_fuse_connections_event_history"

const (
	// unix timestamp in seconds when the event was observed
	ColumnUnixSeconds = "unix_seconds"

	ColumnDeviceName                           = "device_name"
	ColumnCongestedPercentAgainstThreshold     = "congested_percent_against_threshold"
	ColumnMaxBackgroundPercentAgainstThreshold = "max_background_percent_against_threshold"
)

// retain up to 3 days of events
const DefaultRetentionPeriod = 3 * 24 * time.Hour

type Event struct {
	UnixSeconds                          int64   `json:"unix_seconds"`
	DeviceName                           string  `json:"device_name"`
	CongestedPercentAgainstThreshold     float64 `json:"congested_percent_against_threshold"`
	MaxBackgroundPercentAgainstThreshold float64 `json:"max_background_percent_against_threshold"`
}

func (ev Event) JSON() ([]byte, error) {
	return json.Marshal(ev)
}

func CreateTableFUSEConnectionsEventHistory(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS %s (
	%s INTEGER NOT NULL,
	%s TEXT NOT NULL,
	%s REAL,
	%s REAL
);`, TableNameFUSEConnectionsEventHistory,
		ColumnUnixSeconds,
		ColumnDeviceName,
		ColumnCongestedPercentAgainstThreshold,
		ColumnMaxBackgroundPercentAgainstThreshold,
	))
	return err
}

func InsertEvent(ctx context.Context, db *sql.DB, event Event) error {
	log.Logger.Debugw(
		"inserting event",
		"deviceName", event.DeviceName,
		"congestedPercentAgainstThreshold", event.CongestedPercentAgainstThreshold,
		"maxBackgroundPercentAgainstThreshold", event.MaxBackgroundPercentAgainstThreshold,
	)

	insertStatement := fmt.Sprintf(`
INSERT OR REPLACE INTO %s (%s, %s, %s, %s) VALUES (?, ?, ?, ?);
`,
		TableNameFUSEConnectionsEventHistory,
		ColumnUnixSeconds,
		ColumnDeviceName,
		ColumnCongestedPercentAgainstThreshold,
		ColumnMaxBackgroundPercentAgainstThreshold,
	)

	start := time.Now()
	_, err := db.ExecContext(
		ctx,
		insertStatement,
		event.UnixSeconds,
		event.DeviceName,
		event.CongestedPercentAgainstThreshold,
		event.MaxBackgroundPercentAgainstThreshold,
	)
	sqlite.RecordInsertUpdate(time.Since(start).Seconds())

	return err
}

func FindEvent(ctx context.Context, db *sql.DB, unixSeconds int64, devName string) (*Event, error) {
	selectStatement := fmt.Sprintf(`
SELECT %s, %s, %s, %s FROM %s WHERE %s = ? AND %s = ?;
`,
		ColumnUnixSeconds,
		ColumnDeviceName,
		ColumnCongestedPercentAgainstThreshold,
		ColumnMaxBackgroundPercentAgainstThreshold,
		TableNameFUSEConnectionsEventHistory,
		ColumnUnixSeconds,
		ColumnDeviceName,
	)

	var foundEvent Event
	if err := db.QueryRowContext(
		ctx,
		selectStatement,
		unixSeconds,
		devName,
	).Scan(
		&foundEvent.UnixSeconds,
		&foundEvent.DeviceName,
		&foundEvent.CongestedPercentAgainstThreshold,
		&foundEvent.MaxBackgroundPercentAgainstThreshold,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	return &foundEvent, nil
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
			&event.DeviceName,
			&event.CongestedPercentAgainstThreshold,
			&event.MaxBackgroundPercentAgainstThreshold,
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

	selectStatement := fmt.Sprintf(`SELECT %s, %s, %s, %s
FROM %s`,
		ColumnUnixSeconds,
		ColumnDeviceName,
		ColumnCongestedPercentAgainstThreshold,
		ColumnMaxBackgroundPercentAgainstThreshold,
		TableNameFUSEConnectionsEventHistory,
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
	log.Logger.Debugw("purging fuse connections events")
	deleteStatement, args, err := createDeleteStatementAndArgs(opts...)
	if err != nil {
		return 0, err
	}

	start := time.Now()
	rs, err := db.ExecContext(ctx, deleteStatement, args...)
	if err != nil {
		return 0, err
	}
	sqlite.RecordDelete(time.Since(start).Seconds())

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
		TableNameFUSEConnectionsEventHistory,
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
