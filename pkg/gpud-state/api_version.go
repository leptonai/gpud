package gpudstate

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/leptonai/gpud/pkg/log"
	"github.com/leptonai/gpud/pkg/sqlite"
)

const (
	TableNameAPIVersion = "api_version"

	ColumnAPIVersion = "version"
)

func CreateTableAPIVersion(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS %s (
	%s TEXT PRIMARY KEY
);`, TableNameAPIVersion, ColumnAPIVersion))
	return err
}

var ErrEmptyAPIVersion = errors.New("api version is empty")

func UpdateAPIVersionIfNotExists(ctx context.Context, db *sql.DB, apiVersion string) (string, error) {
	if apiVersion == "" {
		return "", ErrEmptyAPIVersion
	}

	// Start a transaction to ensure atomicity
	tx, err := db.BeginTx(ctx, &sql.TxOptions{
		Isolation: sql.LevelSerializable,
	})
	if err != nil {
		return "", fmt.Errorf("failed to begin transaction: %v", err)
	}
	defer func() {
		if err := tx.Rollback(); err != nil {
			log.Logger.Errorw("failed to rollback transaction", "errror", err)
		}
	}()

	// Read current version within transaction
	var version sql.NullString
	err = tx.QueryRowContext(ctx, fmt.Sprintf(`
SELECT %s FROM %s ORDER BY rowid DESC LIMIT 1
`, ColumnAPIVersion, TableNameAPIVersion)).Scan(&version)

	if err != nil && err != sql.ErrNoRows {
		return "", fmt.Errorf("failed to read version: %v", err)
	}

	if version.Valid {
		return version.String, nil
	}

	// No version exists, insert new one
	query := fmt.Sprintf(`
INSERT INTO %s (%s) VALUES (?)
`, TableNameAPIVersion, ColumnAPIVersion)

	start := time.Now()
	_, err = tx.ExecContext(ctx, query, apiVersion)
	if err != nil {
		return "", fmt.Errorf("failed to insert version: %v", err)
	}
	sqlite.RecordInsertUpdate(time.Since(start).Seconds())

	// Commit the transaction
	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("failed to commit transaction: %v", err)
	}

	return apiVersion, nil
}

// ReadLatestAPIVersion reads the latest API version from the database.
func ReadLatestAPIVersion(ctx context.Context, db *sql.DB) (string, error) {
	// the last inserted row is the latest version, thus using the internal sqlite rowid index
	selectQuery := fmt.Sprintf(`SELECT %s FROM %s ORDER BY rowid DESC LIMIT 1`, ColumnAPIVersion, TableNameAPIVersion)
	var apiVersion sql.NullString
	if err := db.QueryRowContext(ctx, selectQuery).Scan(&apiVersion); err != nil {
		return "", err
	}

	if !apiVersion.Valid {
		return "", sql.ErrNoRows
	}

	return apiVersion.String, nil
}
