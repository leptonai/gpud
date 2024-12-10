package state

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

const (
	TableNameMachineBootIDs = "machine_boot_ids"

	ColumnBootID          = "boot_id"
	ColumnBootUnixSeconds = "boot_unix_seconds"
)

func CreateTableBootIDs(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
	%s TEXT PRIMARY KEY,
	%s INTEGER
);`, TableNameMachineBootIDs, ColumnBootID, ColumnBootUnixSeconds))
	return err
}

// Returns an empty string and no error if no boot ID is found.
func GetLastBootID(ctx context.Context, db *sql.DB) (string, error) {
	row := db.QueryRowContext(ctx, fmt.Sprintf("SELECT %s FROM %s ORDER BY %s DESC LIMIT 1", ColumnBootID, TableNameMachineBootIDs, ColumnBootUnixSeconds))
	var bootID string
	err := row.Scan(&bootID)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return bootID, err
}

func GetFirstBootID(ctx context.Context, db *sql.DB) (string, error) {
	row := db.QueryRowContext(ctx, fmt.Sprintf("SELECT %s FROM %s ORDER BY %s ASC LIMIT 1", ColumnBootID, TableNameMachineBootIDs, ColumnBootUnixSeconds))
	var bootID string
	err := row.Scan(&bootID)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return bootID, err
}

func InsertBootID(ctx context.Context, db *sql.DB, bootID string, time time.Time) error {
	_, err := db.ExecContext(ctx, fmt.Sprintf("INSERT INTO %s (%s, %s) VALUES (?, ?)",
		TableNameMachineBootIDs,
		ColumnBootID,
		ColumnBootUnixSeconds),
		bootID, time.UTC().Unix())
	return err
}

type RebootEvent struct {
	BootID      string
	UnixSeconds int64
}

// Reboot events are events where the boot ID changed.
func GetRebootEvents(ctx context.Context, db *sql.DB, since time.Time) ([]RebootEvent, error) {
	firstBootID, err := GetFirstBootID(ctx, db)
	if err != nil {
		return nil, err
	}
	// exclude the first boot ID if it does not exist
	// since this is the initialization of the db
	// no boot ID change is expected
	if firstBootID == "" {
		return nil, nil
	}

	query := fmt.Sprintf("SELECT %s, %s FROM %s WHERE %s > ? AND %s != ?", ColumnBootID, ColumnBootUnixSeconds, TableNameMachineBootIDs, ColumnBootUnixSeconds, ColumnBootID)
	params := []any{since.UTC().Unix(), firstBootID}

	rows, err := db.QueryContext(ctx, query, params...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	events := []RebootEvent{}
	for rows.Next() {
		var event RebootEvent
		if err := rows.Scan(&event.BootID, &event.UnixSeconds); err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	if len(events) == 0 {
		return nil, nil
	}
	return events, nil
}
