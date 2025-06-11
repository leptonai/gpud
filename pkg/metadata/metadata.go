// Package metadata provides the persistent storage layer for GPUd metadata.
package metadata

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	_ "github.com/mattn/go-sqlite3"

	pkgmetricsrecorder "github.com/leptonai/gpud/pkg/metrics/recorder"
)

const (
	tableNameGPUdMetadata = "gpud_metadata"
	columnKey             = "key"
	columnValue           = "value"
)

// CreateTableMetadata creates the table for the metadata.
func CreateTableMetadata(ctx context.Context, dbRW *sql.DB) error {
	_, err := dbRW.ExecContext(ctx, fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS %s (
	%s TEXT PRIMARY KEY,
	%s TEXT
) WITHOUT ROWID;`, tableNameGPUdMetadata, columnKey, columnValue))
	return err
}

const (
	MetadataKeyMachineID = "machine_id"
	MetadataKeyToken     = "token"
	MetadataKeyEndpoint  = "endpoint"
	MetadataKeyPublicIP  = "public_ip"
	MetadataKeyPrivateIP = "private_ip"
	MetadataKeyProvider  = "provider"
	MetadataKeyNodeGroup = "node_group"
	MetadataKeyRegion    = "region"
	MetadataKeyExtraInfo = "extra_info"

	// MetadataKeyControlPlaneLoginSuccess represents the timestamp in unix seconds
	// when the control plane login was successful.
	MetadataKeyControlPlaneLoginSuccess = "control_plane_login_success"
)

// SetMetadata sets the value of a metadata entry.
// If the metadata entry is not found, it is created.
// If the metadata entry is found and the value is the same, it is not updated.
// If the metadata entry is found and the value is different, it is updated.
func SetMetadata(ctx context.Context, dbRW *sql.DB, key string, value string) error {
	prev, err := ReadMetadata(ctx, dbRW, key)
	if err != nil {
		return err
	}

	if prev == value {
		return nil
	}

	start := time.Now()
	if prev == "" {
		// the "name" is not in the table, so we need to insert it
		_, err = dbRW.ExecContext(ctx, fmt.Sprintf(`
INSERT INTO %s (%s, %s) VALUES (?, ?)`, tableNameGPUdMetadata, columnKey, columnValue), key, value)
	} else {
		// the "name" is already in the table, so we need to update it
		_, err = dbRW.ExecContext(ctx, fmt.Sprintf(`
UPDATE %s SET %s = ? WHERE %s = ?`, tableNameGPUdMetadata, columnValue, columnKey), value, key)
	}
	pkgmetricsrecorder.RecordSQLiteInsertUpdate(time.Since(start).Seconds())

	return err
}

// ReadMetadata reads the value of a metadata entry.
// Returns an empty string and no error, if the metadata entry is not found.
func ReadMetadata(ctx context.Context, dbRO *sql.DB, key string) (string, error) {
	var value string

	start := time.Now()
	err := dbRO.QueryRowContext(ctx, fmt.Sprintf(`
SELECT %s FROM %s WHERE %s = ?`, columnValue, tableNameGPUdMetadata, columnKey), key).Scan(&value)
	pkgmetricsrecorder.RecordSQLiteSelect(time.Since(start).Seconds())

	if err == sql.ErrNoRows {
		return "", nil
	}
	return value, err
}

// ReadAllMetadata reads all the metadata entries.
// Returns an empty string and no error, if the metadata entry is not found.
func ReadAllMetadata(ctx context.Context, dbRO *sql.DB) (map[string]string, error) {
	selectQuery := fmt.Sprintf("SELECT %s, %s FROM %s", columnKey, columnValue, tableNameGPUdMetadata)

	start := time.Now()
	rows, err := dbRO.QueryContext(ctx, selectQuery)
	defer func() {
		pkgmetricsrecorder.RecordSQLiteSelect(time.Since(start).Seconds())
	}()
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()

	metadata := make(map[string]string)
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return nil, err
		}
		metadata[key] = value
	}
	return metadata, nil
}
