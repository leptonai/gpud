package store

import (
	"context"
	"database/sql"
	"fmt"
)

var defaultMetadataTable = "infiniband_metadata_" + schemaVersion

const (
	// metadataColumnKey represents the key of the metadata.
	metadataColumnKey = "k"
	// metadataColumnValue represents the value of the metadata.
	metadataColumnValue = "v"
)

func createMetadataTable(ctx context.Context, dbRW *sql.DB, tableName string) error {
	tx, err := dbRW.BeginTx(ctx, nil)
	if err != nil {
		return err
	}

	// create table
	_, err = tx.ExecContext(ctx, fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS %s (
	%s TEXT PRIMARY KEY NOT NULL,
	%s TEXT NOT NULL
);`, tableName,
		metadataColumnKey,
		metadataColumnValue,
	))
	if err != nil {
		_ = tx.Rollback()
		return err
	}

	return tx.Commit()
}
