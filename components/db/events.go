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

	// event name
	// e.g., "memory_oom", "memory_oom_kill_constraint", "memory_oom_cgroup", "memory_edac_correctable_errors".
	ColumnName = "name"

	// event type
	// e.g., "Unknown", "Info", "Warning", "Critical", "Fatal".
	ColumnType = "type"

	// event message
	// e.g., "VFS file-max limit reached"
	ColumnMessage = "message"

	// event extra info
	// e.g.,
	// data source: "dmesg", "nvml", "nvidia-smi".
	// event target: "gpu_id", "gpu_uuid".
	// log detail: "oom_reaper: reaped process 345646 (vector), now anon-rss:0kB, file-rss:0kB, shmem-rss:0".
	ColumnExtraInfo = "extra_info"

	// event suggested actions
	// e.g., "reboot"
	ColumnSuggestedActions = "suggested_actions"
)

type Event struct {
	// Event timestamp in unix seconds.
	Timestamp int64 `json:"timestamp"`

	// event name
	// e.g., "memory_oom", "memory_oom_kill_constraint", "memory_oom_cgroup", "memory_edac_correctable_errors".
	Name string `json:"name"`

	// event type
	// e.g., "Unknown", "Info", "Warning", "Critical", "Fatal".
	Type string `json:"type"`

	// event message
	// e.g., "VFS file-max limit reached"
	Message string `json:"message"`

	// event extra info
	// e.g.,
	// data source: "dmesg", "nvml", "nvidia-smi".
	// event target: "gpu_id", "gpu_uuid".
	// log detail: "oom_reaper: reaped process 345646 (vector), now anon-rss:0kB, file-rss:0kB, shmem-rss:0".
	ExtraInfo string `json:"extra_info"`

	// event suggested actions
	// e.g., "reboot"
	SuggestedActions string `json:"suggested_actions"`
}

type storeImpl struct {
	table string
	dbRW  *sql.DB
	dbRO  *sql.DB
}

var (
	ErrNoDBRWSet = errors.New("no writable db set")
	ErrNoDBROSet = errors.New("no read-only db set")
)

// Creates a new DB instance with the table created.
// Requires write-only and read-only instances for minimize conflicting writes/reads.
// ref. https://github.com/mattn/go-sqlite3/issues/1179#issuecomment-1638083995
func NewStore(ctx context.Context, dbRW *sql.DB, dbRO *sql.DB, tableName string) (*storeImpl, error) {
	if dbRW == nil {
		return nil, ErrNoDBRWSet
	}
	if dbRO == nil {
		return nil, ErrNoDBROSet
	}

	if err := createTable(ctx, dbRW, tableName); err != nil {
		return nil, err
	}
	return &storeImpl{
		table: tableName,
		dbRW:  dbRW,
		dbRO:  dbRO,
	}, nil
}

func (s *storeImpl) Insert(ctx context.Context, ev Event) error {
	return insertEvent(ctx, s.dbRW, s.table, ev)
}

func (s *storeImpl) Find(ctx context.Context, ev Event) (*Event, error) {
	return findEvent(ctx, s.dbRO, s.table, ev)
}

// Returns the event in the descending order of timestamp (latest event first).
func (s *storeImpl) Get(ctx context.Context, since time.Time) ([]Event, error) {
	return getEvents(ctx, s.dbRO, s.table, since)
}

func (s *storeImpl) Purge(ctx context.Context, beforeTimestamp int64) (int, error) {
	return purgeEvents(ctx, s.dbRW, s.table, beforeTimestamp)
}

func createTable(ctx context.Context, db *sql.DB, tableName string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}

	// create table
	_, err = tx.ExecContext(ctx, fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
	%s INTEGER NOT NULL,
	%s TEXT NOT NULL,
	%s TEXT NOT NULL,
	%s TEXT,
	%s TEXT,
	%s TEXT
);`, tableName,
		ColumnTimestamp,
		ColumnName,
		ColumnType,
		ColumnMessage,
		ColumnExtraInfo,
		ColumnSuggestedActions,
	))
	if err != nil {
		_ = tx.Rollback()
		return err
	}

	// create index on timestamp column
	_, err = tx.ExecContext(ctx, fmt.Sprintf(`CREATE INDEX IF NOT EXISTS idx_%s_%s ON %s(%s);`,
		tableName, ColumnTimestamp, tableName, ColumnTimestamp))
	if err != nil {
		_ = tx.Rollback()
		return err
	}

	return tx.Commit()
}

func insertEvent(ctx context.Context, db *sql.DB, tableName string, ev Event) error {
	start := time.Now()
	_, err := db.ExecContext(ctx, fmt.Sprintf("INSERT INTO %s (%s, %s, %s, %s, %s, %s) VALUES (?, ?, ?, NULLIF(?, ''), NULLIF(?, ''), NULLIF(?, ''))",
		tableName,
		ColumnTimestamp,
		ColumnName,
		ColumnType,
		ColumnMessage,
		ColumnExtraInfo,
		ColumnSuggestedActions,
	),
		ev.Timestamp,
		ev.Name,
		ev.Type,
		ev.Message,
		ev.ExtraInfo,
		ev.SuggestedActions,
	)
	sqlite.RecordInsertUpdate(time.Since(start).Seconds())

	return err
}

func findEvent(ctx context.Context, db *sql.DB, tableName string, ev Event) (*Event, error) {
	selectStatement := fmt.Sprintf(`
SELECT %s, %s, %s, %s, %s, %s FROM %s WHERE %s = ? AND %s = ? AND %s = ?`,
		ColumnTimestamp,
		ColumnName,
		ColumnType,
		ColumnMessage,
		ColumnExtraInfo,
		ColumnSuggestedActions,
		tableName,
		ColumnTimestamp,
		ColumnName,
		ColumnType,
	)
	if ev.Message != "" {
		selectStatement += fmt.Sprintf(" AND %s = ?", ColumnMessage)
	}
	if ev.ExtraInfo != "" {
		selectStatement += fmt.Sprintf(" AND %s = ?", ColumnExtraInfo)
	}
	if ev.SuggestedActions != "" {
		selectStatement += fmt.Sprintf(" AND %s = ?", ColumnSuggestedActions)
	}

	params := []any{ev.Timestamp, ev.Name, ev.Type}
	if ev.Message != "" {
		params = append(params, ev.Message)
	}
	if ev.ExtraInfo != "" {
		params = append(params, ev.ExtraInfo)
	}
	if ev.SuggestedActions != "" {
		params = append(params, ev.SuggestedActions)
	}

	row := db.QueryRowContext(ctx, selectStatement, params...)

	foundEvent, err := scanRow(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	return &foundEvent, nil
}

// Returns the event in the descending order of timestamp (latest event first).
func getEvents(ctx context.Context, db *sql.DB, tableName string, since time.Time) ([]Event, error) {
	query := fmt.Sprintf(`SELECT %s, %s, %s, %s, %s, %s
FROM %s
WHERE %s > ?
ORDER BY %s DESC`,
		ColumnTimestamp, ColumnName, ColumnType, ColumnMessage, ColumnExtraInfo, ColumnSuggestedActions,
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
		event, err := scanRows(rows)
		if err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	if len(events) == 0 {
		return nil, nil
	}
	return events, nil
}

func scanRow(row *sql.Row) (Event, error) {
	var event Event
	var msg sql.NullString
	var extraInfo sql.NullString
	var suggestedActions sql.NullString
	err := row.Scan(
		&event.Timestamp,
		&event.Name,
		&event.Type,
		&msg,
		&extraInfo,
		&suggestedActions,
	)
	if err == nil {
		if msg.Valid {
			event.Message = msg.String
		}
		if extraInfo.Valid {
			event.ExtraInfo = extraInfo.String
		}
		if suggestedActions.Valid {
			event.SuggestedActions = suggestedActions.String
		}
	}
	return event, err
}

func scanRows(rows *sql.Rows) (Event, error) {
	var event Event
	var msg sql.NullString
	var extraInfo sql.NullString
	var suggestedActions sql.NullString
	err := rows.Scan(
		&event.Timestamp,
		&event.Name,
		&event.Type,
		&msg,
		&extraInfo,
		&suggestedActions,
	)
	if err == nil {
		if msg.Valid {
			event.Message = msg.String
		}
		if extraInfo.Valid {
			event.ExtraInfo = extraInfo.String
		}
		if suggestedActions.Valid {
			event.SuggestedActions = suggestedActions.String
		}
	}
	return event, err
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
