package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/leptonai/gpud/pkg/log"
	pkgmetricsrecorder "github.com/leptonai/gpud/pkg/metrics/recorder"
)

// defaultPurgeInterval is the default interval to purge the data.
// Since events are critical, we do NOT purge until the admin action (e.g., set healthy)
// thus explicitly requested to tombstone with a timestamp.
const defaultPurgeInterval = 10 * time.Hour

// purge purges old entries from the table
// but retain the rows with non-empty event type,
// because we do not want to lose important events.
// If purgeAllEvents is true, it will purge all events.
func purge(ctx context.Context, dbRW *sql.DB, tableName string, beforeTimestamp int64, purgeAllEvents bool) (int, error) {
	deleteStatement := fmt.Sprintf(`DELETE FROM %s WHERE %s < ?`, tableName, historyTableColumnTimestamp)
	if !purgeAllEvents {
		deleteStatement += fmt.Sprintf(` AND %s = ''`, historyTableColumnEventType)
	}

	start := time.Now()
	rs, err := dbRW.ExecContext(ctx, deleteStatement, beforeTimestamp)
	pkgmetricsrecorder.RecordSQLiteDelete(time.Since(start).Seconds())
	if err != nil {
		return 0, err
	}

	affected, err := rs.RowsAffected()
	if err != nil {
		return 0, err
	}
	return int(affected), nil
}

// getPurgeBeforeTimestamp returns the timestamp to purge the records before,
// and whether to purge all events (false if we need to retain the events with non-empty event type)
func (s *ibPortsStore) getPurgeBeforeTimestamp() (time.Time, bool) {
	cctx, cancel := context.WithTimeout(s.rootCtx, 15*time.Second)
	tombstoneTs, err := getTombstoneTimestamp(cctx, s.dbRO, s.metadataTable)
	cancel()
	if err != nil {
		log.Logger.Warnw("failed to get tombstone timestamp", "table", s.metadataTable, "error", err)
	}

	cctx, cancel = context.WithTimeout(s.rootCtx, 15*time.Second)
	lastScanTs, err := getLastScanTimestamp(cctx, s.dbRO, s.metadataTable)
	cancel()
	if err != nil {
		log.Logger.Warnw("failed to get last scan timestamp", "table", s.metadataTable, "error", err)
	}

	// tombstone always overrides all watermarks
	purgeBefore := lastScanTs
	purgeAllEvents := false
	if !tombstoneTs.IsZero() {
		// Since events are critical, we do NOT purge until the admin action (e.g., set healthy)
		// thus explicitly requested to tombstone with a timestamp.
		purgeBefore = tombstoneTs
		purgeAllEvents = true
	}

	return purgeBefore, purgeAllEvents
}

func (s *ibPortsStore) runPurge() {
	log.Logger.Infow("start purging", "table", s.historyTable, "checkInterval", s.purgeInterval)
	for {
		select {
		case <-s.rootCtx.Done():
			return
		case <-time.After(s.purgeInterval):
		}

		purgeBefore, purgeAllEvents := s.getPurgeBeforeTimestamp()

		cctx, cancel := context.WithTimeout(s.rootCtx, 15*time.Second)
		purged, err := purge(cctx, s.dbRW, s.historyTable, purgeBefore.Unix(), purgeAllEvents)
		cancel()
		if err != nil {
			log.Logger.Warnw("failed to purge", "table", s.historyTable, "error", err)
		} else {
			log.Logger.Infow("purged", "table", s.historyTable, "purged", purged)
		}
	}
}
