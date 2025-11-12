package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/leptonai/gpud/pkg/log"
	pkgmetricsrecorder "github.com/leptonai/gpud/pkg/metrics/recorder"
)

const (
	// defaultRetentionPeriod is the default retention period for the data.
	defaultRetentionPeriod = 5 * 24 * time.Hour

	// defaultPurgeInterval is the default interval to purge the data.
	defaultPurgeInterval = 10 * time.Hour
)

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
	// Read configuration fields under mutex to avoid race conditions
	s.configMu.RLock()
	rootCtx := s.rootCtx
	metadataTable := s.metadataTable
	retentionPeriod := s.retentionPeriod
	s.configMu.RUnlock()

	cctx, cancel := context.WithTimeout(rootCtx, 15*time.Second)
	tombstoneTS, err := getTombstoneTimestamp(cctx, s.dbRO, metadataTable)
	cancel()
	if err != nil {
		return time.Time{}, false
	}

	purgeBefore := s.getTimeNow().Add(-retentionPeriod)
	purgeAllEvents := false
	if !tombstoneTS.IsZero() && tombstoneTS.After(purgeBefore) {
		purgeBefore = tombstoneTS
		purgeAllEvents = true
	}

	return purgeBefore, purgeAllEvents
}

func (s *ibPortsStore) runPurge() {
	// Read configuration fields under mutex to avoid race conditions
	s.configMu.RLock()
	historyTable := s.historyTable
	purgeInterval := s.purgeInterval
	rootCtx := s.rootCtx
	s.configMu.RUnlock()

	log.Logger.Infow("start purging", "table", historyTable, "checkInterval", purgeInterval)
	for {
		select {
		case <-rootCtx.Done():
			return
		case <-time.After(purgeInterval):
		}

		purgeBefore, purgeAllEvents := s.getPurgeBeforeTimestamp()
		if purgeBefore.IsZero() {
			continue
		}

		// Read configuration fields under mutex for each iteration
		s.configMu.RLock()
		currentHistoryTable := s.historyTable
		currentRootCtx := s.rootCtx
		s.configMu.RUnlock()

		cctx, cancel := context.WithTimeout(currentRootCtx, 15*time.Second)
		purged, err := purge(cctx, s.dbRW, currentHistoryTable, purgeBefore.Unix(), purgeAllEvents)
		cancel()
		if err != nil {
			log.Logger.Warnw("failed to purge", "table", currentHistoryTable, "error", err)
		} else {
			log.Logger.Infow("purged", "table", currentHistoryTable, "purged", purged)
		}
	}
}
