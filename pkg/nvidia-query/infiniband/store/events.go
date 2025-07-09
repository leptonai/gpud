package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/leptonai/gpud/pkg/log"
	pkgmetricsrecorder "github.com/leptonai/gpud/pkg/metrics/recorder"
	"github.com/leptonai/gpud/pkg/nvidia-query/infiniband"
)

// SetEventType sets the event id for the given timestamp, device, and port.
func (s *ibPortsStore) SetEventType(device string, port uint, timestamp time.Time, eventType string, eventReason string) error {
	query := fmt.Sprintf(`UPDATE %s SET %s = ?, %s = ? WHERE %s = ? AND %s = ? AND %s = ?;`,
		s.historyTable,
		historyTableColumnEventType, historyTableColumnEventReason,
		historyTableColumnTimestamp, historyTableColumnDevice, historyTableColumnPort,
	)

	cctx, cancel := context.WithTimeout(s.rootCtx, 30*time.Second)
	defer cancel()

	now := s.getTimeNow()
	rs, err := s.dbRW.ExecContext(cctx, query, eventType, eventReason, timestamp.Unix(), device, port)
	pkgmetricsrecorder.RecordSQLiteInsertUpdate(s.getTimeNow().Sub(now).Seconds())
	if err != nil {
		return err
	}

	affected, err := rs.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		log.Logger.Warnw("no row affected", "table", s.historyTable, "timestamp", timestamp, "device", device, "port", port, "event", eventType, "reason", eventReason)
	} else {
		log.Logger.Infow("set event", "table", s.historyTable, "timestamp", timestamp, "device", device, "port", port, "event", eventType, "reason", eventReason)
	}

	return nil
}

// Event represents an event for a port.
type Event struct {
	Time        time.Time
	Port        infiniband.IBPort
	EventType   string
	EventReason string
}

type Events []Event

// Events returns the events since the given timestamp.
// The events are sorted by timestamp in ascending order.
func (s *ibPortsStore) Events(since time.Time) ([]Event, error) {
	query := fmt.Sprintf(`SELECT %s, %s, %s, %s, %s, %s, %s, %s, %s, %s FROM %s WHERE %s != ""`,
		historyTableColumnTimestamp,
		historyTableColumnDevice,
		historyTableColumnPort,
		historyTableColumnLinkLayer,
		historyTableColumnState,
		historyTableColumnPhysicalState,
		historyTableColumnRateGBSec,
		historyTableColumnTotalLinkDowned,
		historyTableColumnEventType,
		historyTableColumnEventReason,
		s.historyTable,
		historyTableColumnEventType,
	)

	params := make([]any, 0)

	cctx, cancel := context.WithTimeout(s.rootCtx, 30*time.Second)
	defer cancel()

	tombstoneTs, err := getTombstoneTimestamp(cctx, s.dbRO, s.metadataTable)
	if err != nil {
		return nil, err
	}
	if !tombstoneTs.IsZero() && tombstoneTs.After(since) {
		log.Logger.Infow("tombstone is after since, using tombstone as since", "tombstone", tombstoneTs, "since", since)
		since = tombstoneTs
	}
	if !since.IsZero() {
		query += fmt.Sprintf(` AND %s >= ?`, historyTableColumnTimestamp)
		params = append(params, since.Unix())
	}

	query += fmt.Sprintf(` ORDER BY %s ASC;`, historyTableColumnTimestamp)

	now := s.getTimeNow()
	rows, err := s.dbRO.QueryContext(cctx, query, params...)
	pkgmetricsrecorder.RecordSQLiteSelect(s.getTimeNow().Sub(now).Seconds())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			log.Logger.Debugw("no row found", "since", since)
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()

	events := make([]Event, 0)
	for rows.Next() {
		var ts int64
		var port infiniband.IBPort
		var eventType string
		var eventReason sql.NullString
		if err := rows.Scan(
			&ts,
			&port.Device,
			&port.Port,
			&port.LinkLayer,
			&port.State,
			&port.PhysicalState,
			&port.RateGBSec,
			&port.TotalLinkDowned,
			&eventType,
			&eventReason,
		); err != nil {
			return nil, err
		}
		ev := Event{
			Time:      time.Unix(ts, 0).UTC(),
			Port:      port,
			EventType: eventType,
		}
		if eventReason.Valid {
			ev.EventReason = eventReason.String
		}
		events = append(events, ev)
	}

	return events, nil
}
