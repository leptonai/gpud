// Package sqlite provides a SQLite3 database utils.
package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/dustin/go-humanize"
	_ "github.com/mattn/go-sqlite3"

	"github.com/leptonai/gpud/pkg/log"
)

// BuildConnectionString builds a SQLite connection string based on the file path and options.
// This is exported for testing purposes.
// ref. https://www.sqlite.org/c3ref/open.html
// ref. https://www.sqlite.org/uri.html
// ref. https://github.com/mattn/go-sqlite3?tab=readme-ov-file#connection-string
// ref. https://github.com/mattn/go-sqlite3?tab=readme-ov-file#faq
func BuildConnectionString(file string, opts ...OpOption) (string, error) {
	op := &Op{}
	if err := op.applyOpts(opts); err != nil {
		return "", err
	}

	var conns string

	// Handle in-memory database with shared cache
	// ref. https://github.com/mattn/go-sqlite3?tab=readme-ov-file#faq
	// For shared in-memory database, use "file::memory:?cache=shared"
	if file == ":memory:" && op.cache != "" {
		conns = "file::memory:?cache=" + op.cache
	} else if file == ":memory:" {
		// Standard in-memory database without shared cache
		conns = "file::memory:"
	} else {
		// File-based database
		conns = "file:" + file
	}

	// Determine the separator for additional parameters
	separator := "?"
	if strings.Contains(conns, "?") {
		separator = "&"
	}

	// Add URI parameters
	// ref. https://www.sqlite.org/pragma.html#pragma_busy_timeout
	// ref. https://www.sqlite.org/pragma.html#pragma_journal_mode
	// ref. https://www.sqlite.org/pragma.html#pragma_synchronous
	// ref. https://github.com/mattn/go-sqlite3/blob/7658c06970ecf5588d8cd930ed1f2de7223f1010/sqlite3.go#L975
	// Note: WAL mode is ignored for in-memory databases (SQLite uses default mode), but including it
	// for consistency and to handle any edge cases where file might not be ":memory:".
	conns += separator + "_busy_timeout=5000&_journal_mode=WAL&_synchronous=NORMAL"

	if op.readOnly {
		conns += "&mode=ro"
	} else {
		// ref. https://github.com/mattn/go-sqlite3/issues/1179#issuecomment-1638083995
		conns += "&_txlock=immediate"
	}

	return conns, nil
}

// Helper function to open a SQLite3 database.
func Open(file string, opts ...OpOption) (*sql.DB, error) {
	conns, err := BuildConnectionString(file, opts...)
	if err != nil {
		return nil, err
	}

	db, err := sql.Open("sqlite3", conns)
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite3 database: %w (%q)", err, conns)
	}

	// Check if this is a read-only connection by checking the options
	op := &Op{}
	_ = op.applyOpts(opts)

	if !op.readOnly {
		// single connection for writing
		db.SetMaxOpenConns(1)
		db.SetMaxIdleConns(1)

		// to not close
		db.SetConnMaxLifetime(0)
		db.SetConnMaxIdleTime(0)
	}

	return db, nil
}

// ReadDBSize reads the size of the database in bytes.
// It fails if the database file does not exist.
func ReadDBSize(ctx context.Context, dbRO *sql.DB) (uint64, error) {
	var pageCount uint64
	err := dbRO.QueryRowContext(ctx, "PRAGMA page_count").Scan(&pageCount)
	if err == sql.ErrNoRows {
		return 0, errors.New("no page count")
	}
	if err != nil {
		return 0, err
	}

	var pageSize uint64
	err = dbRO.QueryRowContext(ctx, "PRAGMA page_size").Scan(&pageSize)
	if err == sql.ErrNoRows {
		return 0, errors.New("no page size")
	}
	if err != nil {
		return 0, err
	}

	return pageCount * pageSize, nil
}

// Compact compacts the database by running the VACUUM command.
func Compact(ctx context.Context, db *sql.DB) error {
	log.Logger.Infow("compacting state database")
	_, err := db.ExecContext(ctx, "VACUUM;")
	if err != nil {
		return err
	}
	log.Logger.Infow("successfully compacted state database")
	return nil
}

// RunCompact compacts the database by running the VACUUM command,
// and prints the size before and after the compact.
func RunCompact(ctx context.Context, dbFile string) error {
	dbRW, err := Open(dbFile)
	if err != nil {
		return fmt.Errorf("failed to open state file: %w", err)
	}
	defer func() {
		_ = dbRW.Close()
	}()

	dbRO, err := Open(dbFile, WithReadOnly(true))
	if err != nil {
		return fmt.Errorf("failed to open state file: %w", err)
	}
	defer func() {
		_ = dbRO.Close()
	}()

	dbSize, err := ReadDBSize(ctx, dbRO)
	if err != nil {
		return fmt.Errorf("failed to read state file size: %w", err)
	}
	log.Logger.Infow("state file size before compact", "size", humanize.IBytes(dbSize))

	if err := Compact(ctx, dbRW); err != nil {
		return fmt.Errorf("failed to compact state file: %w", err)
	}

	dbSize, err = ReadDBSize(ctx, dbRO)
	if err != nil {
		return fmt.Errorf("failed to read state file size: %w", err)
	}
	log.Logger.Infow("state file size after compact", "size", humanize.IBytes(dbSize))

	return nil
}

func TableExists(ctx context.Context, db *sql.DB, name string) (bool, error) {
	var exists int
	err := db.QueryRowContext(ctx, "SELECT 1 FROM sqlite_master WHERE type='table' AND name=?", name).Scan(&exists)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}
