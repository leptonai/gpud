// Package metadata provides the persistent storage layer for component states.
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
	deprecatedTableNameMachineMetadata = "machine_metadata"
	deprecatedColumnMachineID          = "machine_id"
	deprecatedColumnUnixSeconds        = "unix_seconds"
	deprecatedColumnToken              = "token"
	deprecatedColumnComponents         = "components"
)

// readMachineIDFromDeprecatedTable reads the machine ID from the database.
// Returns an empty string and no error, if the machine ID is not found.
func readMachineIDFromDeprecatedTable(ctx context.Context, dbRO *sql.DB) (string, error) {
	query := fmt.Sprintf(`
SELECT %s FROM %s
LIMIT 1;
`,
		deprecatedColumnMachineID,
		deprecatedTableNameMachineMetadata,
	)

	var machineID string

	start := time.Now()
	err := dbRO.QueryRowContext(ctx, query).Scan(&machineID)
	pkgmetricsrecorder.RecordSQLiteSelect(time.Since(start).Seconds())

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			err = nil
		}
	}
	return machineID, err
}

func readTokenFromDeprecatedTable(ctx context.Context, dbRO *sql.DB, machineID string) (string, error) {
	query := fmt.Sprintf(`
SELECT COALESCE(%s, '') FROM %s WHERE %s = ?
LIMIT 1;
`,
		deprecatedColumnToken,
		deprecatedTableNameMachineMetadata,
		deprecatedColumnMachineID,
	)

	var token string

	start := time.Now()
	err := dbRO.QueryRowContext(ctx, query, machineID).Scan(&token)
	pkgmetricsrecorder.RecordSQLiteSelect(time.Since(start).Seconds())

	return token, err
}
