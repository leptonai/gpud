// Package sqlite provides a SQLite implementation of the state.Interface.
package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/leptonai/gpud/pkg/process/manager/state"
	"github.com/leptonai/gpud/pkg/process/manager/state/schema"

	_ "github.com/mattn/go-sqlite3"
)

var _ state.Interface = (*State)(nil)

type State struct {
	db        *sql.DB
	tableName string
}

func New(ctx context.Context, db *sql.DB, tableName string) (state.Interface, error) {
	if err := CreateTable(ctx, db, tableName); err != nil {
		return nil, err
	}
	return &State{
		db:        db,
		tableName: tableName,
	}, nil
}

// RecordStart records the start of a script in UTC time.
func (s *State) RecordStart(ctx context.Context, scriptID string, opts ...state.OpOption) error {
	op := state.Op{}
	if err := op.ApplyOpts(opts); err != nil {
		return err
	}

	if op.StartTimeUnixSeconds == 0 {
		op.StartTimeUnixSeconds = time.Now().UTC().Unix()
	}
	return RecordStart(ctx, s.db, s.tableName, scriptID, op.ScriptName, op.StartTimeUnixSeconds)
}

func (s *State) UpdateExitCode(ctx context.Context, scriptID string, scriptExitCode int) error {
	return UpdateExitCode(ctx, s.db, s.tableName, scriptID, scriptExitCode)
}

func (s *State) UpdateOutput(ctx context.Context, scriptID string, scriptOutput string) error {
	return UpdateOutput(ctx, s.db, s.tableName, scriptID, scriptOutput)
}

// Returns status nil, error ErrNotFound if the row does not exist.
func (s *State) Get(ctx context.Context, scriptID string) (*schema.Status, error) {
	return Get(ctx, s.db, s.tableName, scriptID)
}

const (
	ColumnScriptHash             = "script_hash"
	ColumnLastStartedUnixSeconds = "last_started_unix_seconds"
	ColumnScriptName             = "script_name"
	ColumnLastExitCode           = "last_exit_code"
	ColumnLastOutput             = "last_output"
)

func CreateTable(ctx context.Context, db *sql.DB, tableName string) error {
	_, err := db.ExecContext(ctx, fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS %s (
	%s TEXT PRIMARY KEY,
	%s INTEGER,
	%s TEXT,
	%s INTEGER,
	%s TEXT
);`, tableName, ColumnScriptHash, ColumnLastStartedUnixSeconds, ColumnScriptName, ColumnLastExitCode, ColumnLastOutput))
	return err
}

// Records the start of a script execution in UTC time.
func RecordStart(ctx context.Context, db *sql.DB, tableName string, scriptID string, scriptName string, scriptStartUnixSeconds int64) error {
	insertQuery := fmt.Sprintf(`
INSERT OR REPLACE INTO %s (%s, %s) VALUES (?, ?);
`, tableName, ColumnScriptHash, ColumnLastStartedUnixSeconds)
	args := []any{scriptID, scriptStartUnixSeconds}

	if scriptName != "" {
		insertQuery = fmt.Sprintf(`
INSERT OR REPLACE INTO %s (%s, %s, %s) VALUES (?, ?, ?);
`, tableName, ColumnScriptHash, ColumnLastStartedUnixSeconds, ColumnScriptName)

		args = append(args, scriptName)
	}

	_, err := db.ExecContext(ctx, insertQuery, args...)
	return err
}

// Records the command exit code from a script execution.
func UpdateExitCode(ctx context.Context, db *sql.DB, tableName string, scriptID string, scriptExitCode int) error {
	updateQuery := fmt.Sprintf(`
UPDATE %s SET %s = ? WHERE %s = ?;
`, tableName, ColumnLastExitCode, ColumnScriptHash)
	result, err := db.ExecContext(ctx, updateQuery, scriptExitCode, scriptID)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rowsAffected == 0 {
		insertQuery := fmt.Sprintf(`
INSERT INTO %s (%s, %s) VALUES (?, ?);
`, tableName, ColumnScriptHash, ColumnLastExitCode)
		_, err = db.ExecContext(ctx, insertQuery, scriptID, scriptExitCode)
	}

	return err
}

// Records the command output from a script execution.
func UpdateOutput(ctx context.Context, db *sql.DB, tableName string, scriptID string, scriptOutput string) error {
	updateQuery := fmt.Sprintf(`
UPDATE %s SET %s = ? WHERE %s = ?;
`, tableName, ColumnLastOutput, ColumnScriptHash)
	result, err := db.ExecContext(ctx, updateQuery, scriptOutput, scriptID)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rowsAffected == 0 {
		insertQuery := fmt.Sprintf(`
INSERT INTO %s (%s, %s) VALUES (?, ?);
`, tableName, ColumnScriptHash, ColumnLastOutput)
		_, err = db.ExecContext(ctx, insertQuery, scriptID, scriptOutput)
	}

	return err
}

// Reads the status from the table using the script hash as the key.
// Returns status nil, error ErrNotFound if the row does not exist.
func Get(ctx context.Context, db *sql.DB, tableName string, scriptID string) (*schema.Status, error) {
	query := fmt.Sprintf(`
SELECT %s, %s, %s, %s, %s FROM %s WHERE %s = ?;
`,
		ColumnScriptHash,
		ColumnLastStartedUnixSeconds,
		ColumnScriptName,
		ColumnLastExitCode,
		ColumnLastOutput,
		tableName,
		ColumnScriptHash,
	)
	row := db.QueryRowContext(ctx, query, scriptID)

	var result schema.Status
	err := row.Scan(&result.ScriptHash, &result.LastStartedUnixSeconds, &result.ScriptName, &result.LastExitCode, &result.LastOutput)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, state.ErrNotFound
		}

		return nil, err
	}
	return &result, nil
}
