package server

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

func TestServerWithoutMinimumRetryIntervalSeconds(t *testing.T) {
	t.Parallel()

	db, err := openDB(":memory:")
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	tableName := "test"

	cfg := Config{
		SQLite:    db,
		TableName: tableName,
		QPS:       5,
	}

	srv, err := New(cfg)
	if err != nil {
		t.Fatal("failed to create server:", err)
	}

	id, err := srv.Start(ctx, "echo 12345")
	if err != nil {
		t.Fatal("failed to start script:", err)
	}
	t.Logf("started script: %s", id)

	id, err = srv.Start(ctx, "echo 12345")
	if err != nil {
		t.Fatal("failed to start script:", err)
	}
}

func TestServerWithMinimumRetryIntervalSeconds(t *testing.T) {
	t.Parallel()

	db, err := openDB(":memory:")
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	tableName := "test"

	cfg := Config{
		SQLite:                      db,
		TableName:                   tableName,
		QPS:                         1,
		MinimumRetryIntervalSeconds: 60,
	}

	srv, err := New(cfg)
	if err != nil {
		t.Fatal("failed to create server:", err)
	}

	id, err := srv.Start(ctx, "echo 12345")
	if err != nil {
		t.Fatal("failed to start script:", err)
	}
	t.Logf("started script: %s", id)

	id, err = srv.Start(ctx, "echo 12345")
	if err != ErrQPSLimitExceeded {
		t.Fatalf("expected error, got %v", err)
	}

	time.Sleep(time.Second)

	id, err = srv.Start(ctx, "echo 12345")
	if err != ErrMinimumRetryInterval {
		t.Fatalf("expected error, got %v", err)
	}
}

func openDB(file string) (*sql.DB, error) {
	// no need to run separate PRAGMA commands
	// ref. https://www.sqlite.org/pragma.html#pragma_busy_timeout
	// ref. https://www.sqlite.org/pragma.html#pragma_journal_mode
	// ref. https://www.sqlite.org/pragma.html#pragma_synchronous
	conns := fmt.Sprintf("%s?_busy_timeout=5000&_journal_mode=WAL&_synchronous=NORMAL", file)
	db, err := sql.Open("sqlite3", conns)
	if err != nil {
		return nil, err
	}
	return db, nil
}
