// Package sqlite provides a SQLite3 database utils.
package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/leptonai/gpud/log"

	_ "github.com/mattn/go-sqlite3"
)

// Helper function to open a SQLite3 database.
func Open(file string, opts ...OpOption) (*sql.DB, error) {
	op := &Op{}
	if err := op.applyOpts(opts); err != nil {
		return nil, err
	}

	// Build connection string in URI format
	// ref. https://www.sqlite.org/c3ref/open.html
	// ref. https://www.sqlite.org/uri.html
	// ref. https://github.com/mattn/go-sqlite3?tab=readme-ov-file#connection-string
	conns := "file:" + file

	// Add URI parameters
	// ref. https://www.sqlite.org/pragma.html#pragma_busy_timeout
	// ref. https://www.sqlite.org/pragma.html#pragma_journal_mode
	// ref. https://www.sqlite.org/pragma.html#pragma_synchronous
	// ref. https://github.com/mattn/go-sqlite3/blob/7658c06970ecf5588d8cd930ed1f2de7223f1010/sqlite3.go#L975
	conns += "?_busy_timeout=5000&_journal_mode=WAL&_synchronous=NORMAL"

	if op.readOnly {
		conns += "&mode=ro"
	} else {
		// ref. https://github.com/mattn/go-sqlite3/issues/1179#issuecomment-1638083995
		conns += "&_txlock=immediate"
	}

	db, err := sql.Open("sqlite3", conns)
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite3 database: %w (%q)", err, conns)
	}

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

func ReadDBSize(ctx context.Context, db *sql.DB) (uint64, error) {
	var pageCount uint64
	err := db.QueryRowContext(ctx, "PRAGMA page_count").Scan(&pageCount)
	if err == sql.ErrNoRows {
		return 0, errors.New("no page count")
	}
	if err != nil {
		return 0, err
	}

	var pageSize uint64
	err = db.QueryRowContext(ctx, "PRAGMA page_size").Scan(&pageSize)
	if err == sql.ErrNoRows {
		return 0, errors.New("no page size")
	}
	if err != nil {
		return 0, err
	}

	return pageCount * pageSize, nil
}

func Compact(ctx context.Context, db *sql.DB) error {
	log.Logger.Infow("compacting state database")
	_, err := db.ExecContext(ctx, "VACUUM;")
	if err != nil {
		return err
	}
	log.Logger.Infow("successfully compacted state database")
	return nil
}
