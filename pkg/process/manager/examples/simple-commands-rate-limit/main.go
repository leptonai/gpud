package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/leptonai/gpud/pkg/process/manager"
)

func main() {
	db, err := openDB(":memory:")
	if err != nil {
		panic(err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	mngr, err := manager.New(manager.Config{
		SQLite:              db,
		TableName:           "test",
		QPS:                 1,
		MinimumRetrySeconds: 30,
	})
	if err != nil {
		panic(err)
	}

	for i := 0; i < 10; i++ {
		id, _, err := mngr.StartBashScript(ctx, fmt.Sprintf("echo %d", i))
		if err != nil {
			fmt.Println(i, "error:", err)
			if errors.Is(err, manager.ErrQPSLimitExceeded) {
				continue
			}
			panic(err)
		}
		fmt.Println(i, "successfully started:", id)
	}

	time.Sleep(2 * time.Second)

	if _, _, err := mngr.StartBashScript(ctx, "echo 0"); !errors.Is(err, manager.ErrMinimumRetryInterval) {
		panic(err)
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
