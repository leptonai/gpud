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
	// metadataKeyLastScanTimestamp represents the timestamp of the last scan in unix seconds.
	metadataKeyLastScanTimestamp = "last_scan_timestamp"
)

// Scan scans the recent events to mark any events
// (such as "ib port drop").
func (s *ibPortsStore) Scan() error {
	cctx, cancel := context.WithTimeout(s.rootCtx, 30*time.Second)
	since, err := getLastScanTimestamp(cctx, s.dbRO, s.metadataTable)
	cancel()
	if err != nil {
		return err
	}

	cctx, cancel = context.WithTimeout(s.rootCtx, 30*time.Second)
	tombstone, err := getTombstoneTimestamp(cctx, s.dbRO, s.metadataTable)
	cancel()
	if err != nil {
		return err
	}

	if !tombstone.IsZero() && tombstone.After(since) {
		since = tombstone
	}

	sinceForDropScan := since
	sinceForFlapScan := since
	if !since.IsZero() {
		sinceForDropScan = sinceForDropScan.Add(-s.ibPortDropLookbackPeriod)
		sinceForFlapScan = sinceForFlapScan.Add(-s.ibPortFlapLookbackPeriod)
	}

	allDevs := s.getAllDeviceValues()
	allPorts := s.getAllPortValues()

	for dev := range allDevs {
		for port := range allPorts {
			drops, err := s.scanIBPortDrops(dev, port, sinceForDropScan)
			if err != nil {
				return err
			}
			for _, rs := range drops {
				if err := s.SetEventType(dev, port, rs.ts, EventTypeIbPortDrop, rs.reason); err != nil {
					return err
				}
			}

			flaps, err := s.scanIBPortFlaps(dev, port, sinceForFlapScan)
			if err != nil {
				return err
			}
			for _, rs := range flaps {
				if err := s.SetEventType(dev, port, rs.ts, EventTypeIbPortFlap, rs.reason); err != nil {
					return err
				}
			}
		}
	}

	cctx, cancel = context.WithTimeout(s.rootCtx, 30*time.Second)
	defer cancel()
	return setLastScanTimestamp(cctx, s.dbRW, s.metadataTable, s.getTimeNow())
}

func setLastScanTimestamp(ctx context.Context, dbRW *sql.DB, metadataTable string, timestamp time.Time) error {
	query := fmt.Sprintf(`INSERT OR REPLACE INTO %s (%s, %s) VALUES (?, ?);`, metadataTable, metadataColumnKey, metadataColumnValue)

	start := time.Now()
	_, err := dbRW.ExecContext(ctx, query, metadataKeyLastScanTimestamp, fmt.Sprintf("%d", timestamp.Unix()))
	pkgmetricsrecorder.RecordSQLiteInsertUpdate(time.Since(start).Seconds())

	if err == nil {
		log.Logger.Infow("successfully set last scan timestamp", "table", metadataTable, "timestamp", timestamp)
	}
	return err
}

func getLastScanTimestamp(ctx context.Context, dbRO *sql.DB, metadataTable string) (time.Time, error) {
	query := fmt.Sprintf(`SELECT %s FROM %s WHERE %s = ? LIMIT 1;`, metadataColumnValue, metadataTable, metadataColumnKey)

	start := time.Now()
	row := dbRO.QueryRowContext(ctx, query, metadataKeyLastScanTimestamp)
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
