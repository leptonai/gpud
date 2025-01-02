// Package sqlite provides a SQLite3 database utils.
package sqlite

import (
	"database/sql"
	"fmt"

	_ "github.com/mattn/go-sqlite3"
)

// Helper function to open a SQLite3 database.
// ref. https://github.com/mattn/go-sqlite3/blob/7658c06970ecf5588d8cd930ed1f2de7223f1010/sqlite3.go#L975
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
	conns += "?_busy_timeout=5000&_journal_mode=WAL&_synchronous=NORMAL&"

	if op.readOnly {
		conns += "&mode=ro"
	}

	fmt.Println(conns)

	// Open with URI format enabled
	db, err := sql.Open("sqlite3", conns)
	if err != nil {
		return nil, err
	}
	return db, nil
}
