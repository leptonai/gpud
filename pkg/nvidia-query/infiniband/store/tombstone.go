package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/leptonai/gpud/pkg/log"
	pkgmetricsrecorder "github.com/leptonai/gpud/pkg/metrics/recorder"
)

const (
	// metadataKeyTombstoneTimestamp represents the tombstone timestamp in unix seconds.
	metadataKeyTombstoneTimestamp = "tombstone_timestamp"
)

// Tombstone marks the given timestamp as tombstone,
// where all the events before the given timestamp are discarded.
// Useful when a component needs to discard (and purge) old events
// after the admin action (e.g. reboot).
func (s *ibPortsStore) Tombstone(timestamp time.Time) error {
	cctx, cancel := context.WithTimeout(s.rootCtx, 30*time.Second)
	defer cancel()

	if err := setTombstoneTimestamp(cctx, s.dbRW, s.metadataTable, timestamp); err != nil {
		return err
	}

	return nil
}

func setTombstoneTimestamp(ctx context.Context, dbRW *sql.DB, metadataTable string, timestamp time.Time) error {
	query := fmt.Sprintf(`INSERT OR REPLACE INTO %s (%s, %s) VALUES (?, ?);`, metadataTable, metadataColumnKey, metadataColumnValue)

	start := time.Now()
	_, err := dbRW.ExecContext(ctx, query, metadataKeyTombstoneTimestamp, fmt.Sprintf("%d", timestamp.Unix()))
	pkgmetricsrecorder.RecordSQLiteInsertUpdate(time.Since(start).Seconds())

	if err == nil {
		log.Logger.Infow("successfully set tombstone timestamp", "table", metadataTable, "timestamp", timestamp)
	}
	return err
}

func getTombstoneTimestamp(ctx context.Context, dbRO *sql.DB, metadataTable string) (time.Time, error) {
	query := fmt.Sprintf(`SELECT %s FROM %s WHERE %s = ? LIMIT 1;`, metadataColumnValue, metadataTable, metadataColumnKey)

	start := time.Now()
	row := dbRO.QueryRowContext(ctx, query, metadataKeyTombstoneTimestamp)
	pkgmetricsrecorder.RecordSQLiteSelect(time.Since(start).Seconds())

	var unixTSStr string
	err := row.Scan(&unixTSStr)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return time.Time{}, nil
		}
		return time.Time{}, err
	}

	// Parse the string to int64 since the column is TEXT type
	unixTS, err := strconv.ParseInt(unixTSStr, 10, 64)
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to parse timestamp %q: %w", unixTSStr, err)
	}

	return time.Unix(unixTS, 0).UTC(), nil
}
