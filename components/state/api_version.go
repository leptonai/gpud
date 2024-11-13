package state

import (
	"context"
	"database/sql"
	"fmt"
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

func UpdateAPIVersionIfNotExists(ctx context.Context, db *sql.DB, apiVersion string) (string, error) {
	ver, err := ReadAPIVersion(ctx, db)
	if err != nil {
		if err == sql.ErrNoRows {
			err = UpdateAPIVersion(ctx, db, apiVersion)
			return apiVersion, err
		}
		return "", err
	}
	return ver, nil
}

func UpdateAPIVersion(ctx context.Context, db *sql.DB, apiVersion string) error {
	query := fmt.Sprintf(`
INSERT OR REPLACE INTO %s (%s) VALUES (?)
`, TableNameAPIVersion, ColumnAPIVersion)
	_, err := db.ExecContext(ctx, query, apiVersion)
	return err
}

func ReadAPIVersion(ctx context.Context, db *sql.DB) (string, error) {
	row := db.QueryRowContext(ctx, fmt.Sprintf(`SELECT %s FROM %s`, ColumnAPIVersion, TableNameAPIVersion))
	var apiVersion string
	err := row.Scan(&apiVersion)
	return apiVersion, err
}
