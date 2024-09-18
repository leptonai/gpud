package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"
)

func TestCommand(t *testing.T) {
	t.Parallel()

	db, err := openDB(":memory:")
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	tableName := "test"

	s, err := New(ctx, db, tableName)
	if err != nil {
		t.Fatal("failed to create state:", err)
	}

	scriptHash := "hash123"
	scriptName := "test_script"
	startTime := time.Now().Unix()

	if err := s.RecordStart(ctx, scriptHash, scriptName); err != nil {
		t.Fatal("failed to record start:", err)
	}

	row, err := s.Get(ctx, scriptHash)
	if err != nil {
		t.Fatal("failed to get row:", err)
	}
	if row.ScriptHash != scriptHash {
		t.Fatalf("script hash does not match: %s", row.ScriptHash)
	}
	if row.LastStartedUnixSeconds != startTime {
		t.Fatalf("start time does not match: %d", row.LastStartedUnixSeconds)
	}
	if *row.ScriptName != scriptName {
		t.Fatalf("script name does not match: %s", *row.ScriptName)
	}

	if err = s.UpdateExitCode(ctx, scriptHash, 0); err != nil {
		t.Fatal("failed to record exit code:", err)
	}
	row, err = s.Get(ctx, scriptHash)
	if err != nil {
		t.Fatal("failed to get row:", err)
	}
	if *row.LastExitCode != 0 {
		t.Fatalf("exit code does not match: %d", row.LastExitCode)
	}

	output := "Test output"
	if err = s.UpdateOutput(ctx, scriptHash, output); err != nil {
		t.Fatal("failed to record output:", err)
	}
	row, err = s.Get(ctx, scriptHash)
	if err != nil {
		t.Fatal("failed to get row:", err)
	}
	if *row.LastOutput != output {
		t.Fatalf("output does not match: %s", *row.LastOutput)
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
