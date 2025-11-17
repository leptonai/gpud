// Package states provides tracking of login success and failure events as well as
// the state of ongoing session loops (token expiration, etc.).
package states

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/mattn/go-sqlite3"

	pkgmetricsrecorder "github.com/leptonai/gpud/pkg/metrics/recorder"
)

const (
	tableNameSessionStates = "session_states"
	columnTimestamp        = "timestamp"
	columnSuccess          = "success"
	columnMessage          = "message"
)

// CreateTable creates the table for session state tracking.
func CreateTable(ctx context.Context, dbRW *sql.DB) error {
	_, err := dbRW.ExecContext(ctx, fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS %s (
	%s INTEGER NOT NULL,
	%s INTEGER NOT NULL,
	%s TEXT
);`, tableNameSessionStates, columnTimestamp, columnSuccess, columnMessage))
	return err
}

// State represents a single login/session status entry.
type State struct {
	Timestamp int64
	Success   bool
	Message   string
}

// Insert inserts a new login status entry.
func Insert(ctx context.Context, dbRW *sql.DB, timestamp int64, success bool, message string) error {
	successInt := 0
	if success {
		successInt = 1
	}

	start := time.Now()
	_, err := dbRW.ExecContext(ctx, fmt.Sprintf(`
INSERT INTO %s (%s, %s, %s) VALUES (?, ?, ?)`,
		tableNameSessionStates, columnTimestamp, columnSuccess, columnMessage),
		timestamp, successInt, message)
	pkgmetricsrecorder.RecordSQLiteInsertUpdate(time.Since(start).Seconds())
	if err != nil {
		return err
	}

	start = time.Now()
	_, err = dbRW.ExecContext(ctx, fmt.Sprintf(`
DELETE FROM %s 
WHERE %s NOT IN (
	SELECT %s FROM %s 
	ORDER BY %s DESC 
	LIMIT 10
)`, tableNameSessionStates, columnTimestamp, columnTimestamp, tableNameSessionStates, columnTimestamp))
	pkgmetricsrecorder.RecordSQLiteInsertUpdate(time.Since(start).Seconds())

	return err
}

// ReadLast returns the most recent login status entry.
// Returns nil if no entries exist.
func ReadLast(ctx context.Context, dbRO *sql.DB) (*State, error) {
	var timestamp int64
	var successInt int
	var message string

	start := time.Now()
	err := dbRO.QueryRowContext(ctx, fmt.Sprintf(`
SELECT %s, %s, %s FROM %s 
ORDER BY %s DESC 
LIMIT 1`, columnTimestamp, columnSuccess, columnMessage, tableNameSessionStates, columnTimestamp)).Scan(&timestamp, &successInt, &message)
	pkgmetricsrecorder.RecordSQLiteSelect(time.Since(start).Seconds())

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return &State{
		Timestamp: timestamp,
		Success:   successInt == 1,
		Message:   message,
	}, nil
}

// HasAnyFailures checks if there are any failure entries in the table.
func HasAnyFailures(ctx context.Context, dbRO *sql.DB) (bool, error) {
	var count int

	start := time.Now()
	err := dbRO.QueryRowContext(ctx, fmt.Sprintf(`
SELECT COUNT(*) FROM %s WHERE %s = 0`, tableNameSessionStates, columnSuccess)).Scan(&count)
	pkgmetricsrecorder.RecordSQLiteSelect(time.Since(start).Seconds())

	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	return count > 0, nil
}
