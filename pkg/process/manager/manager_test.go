package manager

import (
	"bufio"
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/leptonai/gpud/pkg/process/manager/state"

	_ "github.com/mattn/go-sqlite3"
)

func TestManagerStartScriptWithNoRateLimit(t *testing.T) {
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

	mngr, err := New(cfg)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	if _, err := mngr.Get(ctx, "echo 12345"); err != state.ErrNotFound {
		t.Fatalf("expected error, got %v", err)
	}

	id, p, err := mngr.StartBashScript(ctx, "echo 12345")
	if err != nil {
		t.Fatal("failed to start script:", err)
	}
	t.Logf("started script: %s", id)

	outputs := []string{}
	scanner := bufio.NewScanner(p.StdoutReader())
	for scanner.Scan() { // returns false at the end of the output
		line := scanner.Text()
		outputs = append(outputs, line)
		select {
		case err := <-p.Wait():
			if err != nil {
				panic(err)
			}
		default:
		}
	}
	if serr := scanner.Err(); serr != nil {
		// process already dead, thus ignore
		// e.g., "read |0: file already closed"
		if !strings.Contains(serr.Error(), "file already closed") {
			panic(serr)
		}
	}
	select {
	case err := <-p.Wait():
		if err != nil {
			panic(err)
		}
	case <-time.After(2 * time.Second):
		panic("timeout")
	}
	if err := mngr.UpdateOutput(ctx, id, strings.Join(outputs, "\n")); err != nil {
		t.Fatalf("failed to update output: %v", err)
	}

	status, err := mngr.Get(ctx, id)
	if err != nil {
		t.Fatalf("failed to get script: %v", err)
	} else {
		t.Logf("script status: %+v", status)
		t.Logf("script output: %s", *status.LastOutput)
	}

	if *status.LastOutput != strings.Join(outputs, "\n") {
		t.Fatalf("expected output %q, got %q", strings.Join(outputs, "\n"), *status.LastOutput)
	}

	if _, _, err = mngr.StartBashScript(ctx, "echo 12345"); err != nil {
		t.Fatal("failed to start script:", err)
	}
}

func TestManagerStartScriptWithMinimumRetryIntervalSeconds(t *testing.T) {
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
		SQLite:              db,
		TableName:           tableName,
		QPS:                 1,
		MinimumRetrySeconds: 60,
	}

	mngr, err := New(cfg)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	id, _, err := mngr.StartBashScript(ctx, "echo 12345")
	if err != nil {
		t.Fatal("failed to start script:", err)
	}
	t.Logf("started script: %s", id)

	if _, _, err = mngr.StartBashScript(ctx, "echo 12345"); err != ErrQPSLimitExceeded {
		t.Fatalf("expected error, got %v", err)
	}

	time.Sleep(time.Second)

	if _, _, err = mngr.StartBashScript(ctx, "echo 12345"); err != ErrMinimumRetryInterval {
		t.Fatalf("expected error, got %v", err)
	}
}

func TestManagerPreventSameCommandsAfterReboot(t *testing.T) {
	t.Parallel()

	tableName := "test"
	dbFile := filepath.Join(t.TempDir(), "test.db")

	db1, err := openDB(dbFile)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	mngr1, err := New(Config{
		SQLite:    db1,
		TableName: tableName,
	})
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}
	if _, _, err := mngr1.StartBashScript(ctx, "echo 12345"); err != nil {
		t.Fatal("failed to start script:", err)
	}
	db1.Close()

	db2, err := openDB(dbFile)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db2.Close()

	mngr2, err := New(Config{
		SQLite:              db2,
		TableName:           tableName,
		MinimumRetrySeconds: 120, // add this new requirement
	})
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}
	if _, _, err = mngr2.StartBashScript(ctx, "echo 12345"); err != ErrMinimumRetryInterval {
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
