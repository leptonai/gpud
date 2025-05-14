package gpudstate

import (
	"context"
	"database/sql"
	"fmt"

	_ "github.com/mattn/go-sqlite3"
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

	if prev == "" {
		// the "name" is not in the table, so we need to insert it
		_, err = dbRW.ExecContext(ctx, fmt.Sprintf(`
INSERT INTO %s (%s, %s) VALUES (?, ?)`, tableNameGPUdMetadata, columnKey, columnValue), key, value)
	} else {
		// the "name" is already in the table, so we need to update it
		_, err = dbRW.ExecContext(ctx, fmt.Sprintf(`
UPDATE %s SET %s = ? WHERE %s = ?`, tableNameGPUdMetadata, columnValue, columnKey), value, key)
	}

	return err
}

// ReadMetadata reads the value of a metadata entry.
// Returns an empty string and no error, if the metadata entry is not found.
func ReadMetadata(ctx context.Context, dbRO *sql.DB, key string) (string, error) {
	var value string
	err := dbRO.QueryRowContext(ctx, fmt.Sprintf(`
SELECT %s FROM %s WHERE %s = ?`, columnValue, tableNameGPUdMetadata, columnKey), key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return value, err
}
