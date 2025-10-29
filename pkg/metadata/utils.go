package metadata

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"

	pkgmetricsrecorder "github.com/leptonai/gpud/pkg/metrics/recorder"
)

// ReadMachineID returns the machine ID from the metadata table.
// Returns an empty string and no error if the machine ID is not present.
func ReadMachineID(ctx context.Context, dbRO *sql.DB) (string, error) {
	return ReadMetadata(ctx, dbRO, MetadataKeyMachineID)
}

// ReadToken returns the token from the metadata table.
// Returns an empty string and no error if the token is not present.
func ReadToken(ctx context.Context, dbRO *sql.DB) (string, error) {
	return ReadMetadata(ctx, dbRO, MetadataKeyToken)
}

// DeleteAllMetadata purges all metadata entries from the primary metadata table.
func DeleteAllMetadata(ctx context.Context, dbRW *sql.DB) error {
	start := time.Now()
	_, err := dbRW.ExecContext(ctx, fmt.Sprintf(`
DELETE FROM %s`, tableNameGPUdMetadata))
	pkgmetricsrecorder.RecordSQLiteDelete(time.Since(start).Seconds())

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
