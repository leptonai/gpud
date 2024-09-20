package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/leptonai/gpud/pkg/process"
	"github.com/leptonai/gpud/pkg/process/manager"
)

func main() {
	// create a temporary file
	tmpFile, err := os.CreateTemp("", "process-manager-test-*.txt")
	if err != nil {
		panic(err)
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	tableName := "test"
	dbFile := filepath.Join(os.TempDir(), "test.db")

	db, err := openDB(dbFile)
	if err != nil {
		panic(err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	mngr, err := manager.New(manager.Config{
		SQLite:    db,
		TableName: tableName,
	})
	if err != nil {
		panic(err)
	}

	id, proc, err := mngr.StartBashScript(ctx, "echo 12345", process.WithOutputFile(tmpFile))
	if err != nil {
		panic(err)
	}
	select {
	case err := <-proc.Wait():
		if err != nil {
			panic(err)
		}
	case <-time.After(2 * time.Second):
		panic("timeout")
	}
	fmt.Println("script finished, id", id)

	status, err := mngr.Get(ctx, id)
	if err != nil {
		panic(err)
	}
	fmt.Printf("status: %+v\n", status)

	content, err := os.ReadFile(tmpFile.Name())
	if err != nil {
		panic(err)
	}
	fmt.Printf("content: %q\n", string(content))
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
