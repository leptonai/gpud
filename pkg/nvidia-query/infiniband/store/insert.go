package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"github.com/leptonai/gpud/pkg/log"
	pkgmetricsrecorder "github.com/leptonai/gpud/pkg/metrics/recorder"
	"github.com/leptonai/gpud/pkg/nvidia-query/infiniband"
)

const (
	defaultMinInsertInterval = 15 * time.Second
)

// Insert inserts the IB ports into the store.
// The timestamp is the time when the IB ports were queried,
// and all ports are inserted with the same timestamp.
// Only stores the "Infiniband" link layer ports (not "Ethernet" or "Unknown").
func (s *ibPortsStore) Insert(eventTime time.Time, ibPorts []infiniband.IBPort) error {
	lastTimestamp := s.getLastInsertTimestamp()
	if !lastTimestamp.IsZero() && eventTime.Before(lastTimestamp.Add(s.minInsertInterval)) {
		log.Logger.Warnw("skipping insert", "table", s.historyTable, "eventTime", eventTime, "lastTs", lastTimestamp, "minInsertInterval", s.minInsertInterval)
		return nil
	}

	// use the same timestamp for all
	start := s.getTimeNow()
	defer func() {
		pkgmetricsrecorder.RecordSQLiteInsertUpdate(s.getTimeNow().Sub(start).Seconds())
	}()

	cctx, cancel := context.WithTimeout(s.rootCtx, 30*time.Second)
	defer cancel()

	tx, err := s.dbRW.BeginTx(cctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	// Prepare statement once for all inserts
	insertStatement := fmt.Sprintf(`INSERT INTO %s (%s, %s, %s, %s, %s, %s, %s, %s, %s, %s) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?);`,
		s.historyTable,
		historyTableColumnTimestamp,
		historyTableColumnDevice,
		historyTableColumnPort,
		historyTableColumnLinkLayer,
		historyTableColumnState,
		historyTableColumnPhysicalState,
		historyTableColumnRateGBSec,
		historyTableColumnTotalLinkDowned,
		historyTableColumnEventType,
		historyTableColumnExtraInfo,
	)

	stmt, err := tx.PrepareContext(cctx, insertStatement)
	if err != nil {
		return err
	}
	defer stmt.Close()

	allDevs := make(map[string]any)
	allPorts := make(map[uint]any)
	for _, port := range ibPorts {
		if !port.IsIBPort() {
			continue
		}

		_, err = stmt.ExecContext(
			cctx,
			eventTime.Unix(), // use the same timestamp for all ports
			strings.TrimSpace(strings.ToLower(port.Device)),
			port.Port,
			strings.TrimSpace(strings.ToLower(port.LinkLayer)),
			strings.TrimSpace(strings.ToLower(port.State)),
			strings.TrimSpace(strings.ToLower(port.PhysicalState)),
			port.RateGBSec,
			port.TotalLinkDowned,
			"", // no event type when we first insert
			"", // no extra info for now
		)
		if err != nil {
			return err
		}

		allDevs[port.Device] = nil
		allPorts[port.Port] = nil
	}
	if err := tx.Commit(); err != nil {
		return err
	}

	s.updateLastInsertTimestamp(eventTime)
	s.updateAllDeviceValues(allDevs)
	s.updateAllPortValues(allPorts)

	return nil
}

func (s *ibPortsStore) updateLastInsertTimestamp(ts time.Time) {
	s.lastInsertedTsMu.Lock()
	s.lastInsertedTs = ts
	s.lastInsertedTsMu.Unlock()
}

func (s *ibPortsStore) getLastInsertTimestamp() time.Time {
	s.lastInsertedTsMu.RLock()
	ts := s.lastInsertedTs
	s.lastInsertedTsMu.RUnlock()
	return ts
}

// readLastTimestamp returns the last timestamp of the table.
// If the table is empty, it returns a zero time.
func readLastTimestamp(ctx context.Context, dbRO *sql.DB, tableName string) (time.Time, error) {
	query := fmt.Sprintf(`SELECT %s FROM %s ORDER BY %s DESC LIMIT 1;`, historyTableColumnTimestamp, tableName, historyTableColumnTimestamp)

	start := time.Now()
	row := dbRO.QueryRowContext(ctx, query)
	pkgmetricsrecorder.RecordSQLiteSelect(time.Since(start).Seconds())

	var unixTS int64
	err := row.Scan(&unixTS)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return time.Time{}, nil
		}
		return time.Time{}, err
	}
	return time.Unix(unixTS, 0).UTC(), nil
}
