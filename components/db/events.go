package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/leptonai/gpud/log"
	"github.com/leptonai/gpud/pkg/sqlite"

	_ "github.com/mattn/go-sqlite3"
)

const (
	// Event timestamp in unix seconds.
	ColumnTimestamp = "timestamp"

	// e.g., "dmesg", "nvml", "nvidia-smi".
	ColumnDataSource = "data_source"

	// e.g., "memory_oom", "memory_oom_kill_constraint", "memory_oom_cgroup", "memory_edac_correctable_errors".
	ColumnEventType = "event_type"

	// e.g., "xid", "sxid".
	ColumnEventID1 = "event_id_1"

	// e.g., "gpu_id", "gpu_uuid".
	ColumnEventID2 = "event_id_2"

	// e.g., "oom_reaper: reaped process 345646 (vector), now anon-rss:0kB, file-rss:0kB, shmem-rss:0".
	ColumnEventDetails = "event_details"
)

type Event struct {
	// unix seconds
	Timestamp    int64  `json:"timestamp"`
	DataSource   string `json:"data_source"`
	EventType    string `json:"event_type"`
	EventID1     string `json:"event_id_1"`
	EventID2     string `json:"event_id_2"`
	EventDetails string `json:"event_details"`
}

type DB struct {
	table string
	dbRW  *sql.DB
	dbRO  *sql.DB
}

// Creates a new DB instance with the table created.
func NewDB(ctx context.Context, dbRW *sql.DB, dbRO *sql.DB, tableName string) (*DB, error) {
	if err := createTable(ctx, dbRW, tableName); err != nil {
		return nil, err
	}
	return &DB{
		table: tableName,
		dbRW:  dbRW,
		dbRO:  dbRO,
	}, nil
}

func (db *DB) Insert(ctx context.Context, ev Event) error {
	return insertEvent(ctx, db.dbRW, db.table, ev)
}

func (db *DB) Find(ctx context.Context, ev Event) (*Event, error) {
	return findEvent(ctx, db.dbRO, db.table, ev)
}

func (db *DB) Get(ctx context.Context, since time.Time) ([]Event, error) {
	return getEvents(ctx, db.dbRO, db.table, since)
}

func (db *DB) Purge(ctx context.Context, beforeTimestamp int64) (int, error) {
	return purgeEvents(ctx, db.dbRW, db.table, beforeTimestamp)
}

func createTable(ctx context.Context, db *sql.DB, tableName string) error {
	_, err := db.ExecContext(ctx, fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
	%s INTEGER NOT NULL,
	%s TEXT NOT NULL,
	%s TEXT NOT NULL,
	%s TEXT,
	%s TEXT,
	%s TEXT NOT NULL
);`, tableName, ColumnTimestamp, ColumnDataSource, ColumnEventType, ColumnEventID1, ColumnEventID2, ColumnEventDetails))
	if err != nil {
		return err
	}

	return nil
}

func insertEvent(ctx context.Context, db *sql.DB, tableName string, ev Event) error {
	start := time.Now()
	_, err := db.ExecContext(ctx, fmt.Sprintf("INSERT INTO %s (%s, %s, %s, %s, %s, %s) VALUES (?, ?, ?, NULLIF(?, ''), NULLIF(?, ''), ?)",
		tableName,
		ColumnTimestamp,
		ColumnDataSource,
		ColumnEventType,
		ColumnEventID1,
		ColumnEventID2,
		ColumnEventDetails,
	),
		ev.Timestamp,
		ev.DataSource,
		ev.EventType,
		ev.EventID1,
		ev.EventID2,
		ev.EventDetails,
	)
	sqlite.RecordInsertUpdate(time.Since(start).Seconds())

	return err
}

func findEvent(ctx context.Context, db *sql.DB, tableName string, ev Event) (*Event, error) {
	selectStatement := fmt.Sprintf(`
SELECT %s, %s, %s, %s, %s, %s FROM %s WHERE %s = ? AND %s = ? AND %s = ?`,
		ColumnTimestamp,
		ColumnDataSource,
		ColumnEventType,
		ColumnEventID1,
		ColumnEventID2,
		ColumnEventDetails,
		tableName,
		ColumnTimestamp,
		ColumnDataSource,
		ColumnEventType,
	)
	if ev.EventID1 != "" {
		selectStatement += fmt.Sprintf(" AND %s = ?", ColumnEventID1)
	}
	if ev.EventID2 != "" {
		selectStatement += fmt.Sprintf(" AND %s = ?", ColumnEventID2)
	}

	params := []any{ev.Timestamp, ev.DataSource, ev.EventType}
	if ev.EventID1 != "" {
		params = append(params, ev.EventID1)
	}
	if ev.EventID2 != "" {
		params = append(params, ev.EventID2)
	}

	var foundEvent Event
	var eventID1 sql.NullString
	var eventID2 sql.NullString
	if err := db.QueryRowContext(ctx, selectStatement, params...).Scan(
		&foundEvent.Timestamp,
		&foundEvent.DataSource,
		&foundEvent.EventType,
		&eventID1,
		&eventID2,
		&foundEvent.EventDetails,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	if eventID1.Valid {
		foundEvent.EventID1 = eventID1.String
	}
	if eventID2.Valid {
		foundEvent.EventID2 = eventID2.String
	}

	return &foundEvent, nil
}

// Returns the event in the descending order of timestamp (latest event first).
func getEvents(ctx context.Context, db *sql.DB, tableName string, since time.Time) ([]Event, error) {
	query := fmt.Sprintf(`SELECT %s, %s, %s, %s, %s, %s
FROM %s
WHERE %s > ?
ORDER BY %s DESC`,
		ColumnTimestamp, ColumnDataSource, ColumnEventType, ColumnEventID1, ColumnEventID2, ColumnEventDetails,
		tableName,
		ColumnTimestamp,
		ColumnTimestamp,
	)
	params := []any{since.UTC().Unix()}

	start := time.Now()
	rows, err := db.QueryContext(ctx, query, params...)
	sqlite.RecordSelect(time.Since(start).Seconds())

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()

	events := []Event{}
	for rows.Next() {
		var event Event
		var eventID1 sql.NullString
		var eventID2 sql.NullString
		if err := rows.Scan(
			&event.Timestamp,
			&event.DataSource,
			&event.EventType,
			&eventID1,
			&eventID2,
			&event.EventDetails,
		); err != nil {
			return nil, err
		}
		if eventID1.Valid {
			event.EventID1 = eventID1.String
		}
		if eventID2.Valid {
			event.EventID2 = eventID2.String
		}
		events = append(events, event)
	}
	if len(events) == 0 {
		return nil, nil
	}
	return events, nil
}

func purgeEvents(ctx context.Context, db *sql.DB, tableName string, beforeTimestamp int64) (int, error) {
	log.Logger.Debugw("purging events")
	deleteStatement := fmt.Sprintf(`DELETE FROM %s WHERE %s < ?`,
		tableName,
		ColumnTimestamp,
	)

	start := time.Now()
	rs, err := db.ExecContext(ctx, deleteStatement, beforeTimestamp)
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
