package metadata

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"github.com/leptonai/gpud/pkg/log"
	pkgmetricsrecorder "github.com/leptonai/gpud/pkg/metrics/recorder"
	"github.com/leptonai/gpud/pkg/sqlite"
)

// ReadMachineIDWithFallback reads the machine ID from the metadata table.
// Returns an empty string and no error, if the machine ID is not found in the new table.
// For compatibility with older versions of GPUd, it also checks the deprecated table.
func ReadMachineIDWithFallback(ctx context.Context, dbRW *sql.DB, dbRO *sql.DB) (string, error) {
	machineID, err := ReadMetadata(ctx, dbRO, MetadataKeyMachineID)
	if err != nil {
		return "", err
	}
	if machineID != "" {
		return machineID, nil
	}

	// not found in the new table
	// TODO: remove this once we have migrated all users to the new table
	log.Logger.Debugw("machine_id not found in the new table, checking the deprecated table")
	ok, err := sqlite.TableExists(ctx, dbRW, deprecatedTableNameMachineMetadata)
	if err != nil {
		return "", err
	}
	if !ok {
		// no old table either (first run)
		return "", nil
	}

	// old table exists, read the token from it
	machineID, err = readMachineIDFromDeprecatedTable(ctx, dbRO)
	if err != nil {
		return "", err
	}
	if machineID != "" {
		log.Logger.Debugw("machine_id found in the deprecated table, migrating to the new table for next reads")
		if err := SetMetadata(ctx, dbRW, MetadataKeyMachineID, machineID); err != nil {
			return "", err
		}
		return machineID, nil
	}
	return "", nil
}

// ReadTokenWithFallback reads the token from the metadata table.
// Returns an empty string and no error, if the token is not found in the new table.
// For compatibility with older versions of GPUd, it also checks the deprecated table.
func ReadTokenWithFallback(ctx context.Context, dbRW *sql.DB, dbRO *sql.DB, machineID string) (string, error) {
	token, err := ReadMetadata(ctx, dbRO, MetadataKeyToken)
	if err != nil {
		return "", err
	}
	if token != "" {
		return token, nil
	}

	// not found in the new table
	// TODO: remove this once we have migrated all users to the new table
	log.Logger.Debugw("token not found in the new table, checking the deprecated table", "machine_id", machineID)
	ok, err := sqlite.TableExists(ctx, dbRW, deprecatedTableNameMachineMetadata)
	if err != nil {
		return "", err
	}
	if !ok {
		// no old table either (first run)
		return "", nil
	}

	// old table exists, read the token from it
	token, err = readTokenFromDeprecatedTable(ctx, dbRO, machineID)
	if err != nil {
		return "", err
	}
	if token != "" {
		log.Logger.Debugw("token found in the deprecated table, migrating to the new table for next reads", "machine_id", machineID)
		if err := SetMetadata(ctx, dbRW, MetadataKeyToken, token); err != nil {
			return "", err
		}
		return token, nil
	}
	return "", nil
}

// DeleteAllMetadata purges all metadata entries.
func DeleteAllMetadata(ctx context.Context, dbRW *sql.DB) error {
	start := time.Now()
	_, err := dbRW.ExecContext(ctx, fmt.Sprintf(`
DELETE FROM %s`, tableNameGPUdMetadata))
	pkgmetricsrecorder.RecordSQLiteDelete(time.Since(start).Seconds())

	if err != nil {
		return err
	}

	if ok, err := sqlite.TableExists(ctx, dbRW, deprecatedTableNameMachineMetadata); ok && err == nil {
		start := time.Now()
		_, err = dbRW.ExecContext(ctx, fmt.Sprintf(`
DELETE FROM %s`, deprecatedTableNameMachineMetadata))
		pkgmetricsrecorder.RecordSQLiteDelete(time.Since(start).Seconds())

		if err != nil {
			return err
		}
	}

	return err
}

func MaskToken(token string) string {
	trimmed := token
	prefix := ""
	if strings.HasPrefix(token, "nvapi-stg-") {
		prefix = token[:10]
		trimmed = token[10:]
	} else if strings.HasPrefix(token, "nvapi-") {
		prefix = token[:6]
		trimmed = token[6:]
	}

	if len(trimmed) < 10 {
		// should never happen
		return prefix + "..."
	}

	return prefix + trimmed[:4] + "..." + token[len(token)-4:]
}
