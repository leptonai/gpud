package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/leptonai/gpud/components/accelerator/nvidia/infiniband/types"
	"github.com/leptonai/gpud/pkg/log"
	pkgmetricsrecorder "github.com/leptonai/gpud/pkg/metrics/recorder"
)

// SetEventType sets the event id for the given timestamp, device, and port.
func (s *ibPortsStore) SetEventType(device string, port uint, timestamp time.Time, eventType string, eventReason string) error {
	// Read configuration fields under mutex to avoid race conditions
	s.configMu.RLock()
	historyTable := s.historyTable
	rootCtx := s.rootCtx
	s.configMu.RUnlock()

	query := fmt.Sprintf(`UPDATE %s SET %s = ?, %s = ? WHERE %s = ? AND %s = ? AND %s = ?;`,
		historyTable,
		historyTableColumnEventType, historyTableColumnEventReason,
		historyTableColumnTimestamp, historyTableColumnDevice, historyTableColumnPort,
	)

	cctx, cancel := context.WithTimeout(rootCtx, 30*time.Second)
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
		log.Logger.Warnw("no row affected", "table", historyTable, "timestamp", timestamp, "device", device, "port", port, "event", eventType, "reason", eventReason)
	} else {
		log.Logger.Infow("set event", "table", historyTable, "timestamp", timestamp, "device", device, "port", port, "event", eventType, "reason", eventReason)
	}

	return nil
}

// Event represents an event for a port.
type Event struct {
	Time        time.Time
	Port        types.IBPort
	EventType   string
	EventReason string
}

type Events []Event

// LastEvents only returns the last event per device and per port,
// since the given timestamp.
// The events are sorted by timestamp in ascending order.
func (s *ibPortsStore) LastEvents(since time.Time) ([]Event, error) {
	// Read configuration fields under mutex to avoid race conditions
	s.configMu.RLock()
	rootCtx := s.rootCtx
	metadataTable := s.metadataTable
	s.configMu.RUnlock()

	cctx, cancel := context.WithTimeout(rootCtx, 30*time.Second)
	tombstoneTs, err := getTombstoneTimestamp(cctx, s.dbRO, metadataTable)
	cancel()
	if err != nil {
		return nil, err
	}
	if !tombstoneTs.IsZero() && tombstoneTs.After(since) {
		log.Logger.Infow("tombstone is after since, using tombstone as since", "tombstone", tombstoneTs, "since", since)
		since = tombstoneTs
	}

	allDevs := s.getAllDeviceValues()
	allPorts := s.getAllPortValues()

	events := make([]Event, 0)
	for dev := range allDevs {
		for port := range allPorts {
			evs, err := s.lastEvents(dev, port, since)
			if err != nil {
				return nil, err
			}
			if len(evs) > 0 {
				events = append(events, evs...)
			}
		}
	}
	sort.Slice(events, func(i, j int) bool {
		return events[i].Time.Before(events[j].Time)
	})
	return events, nil
}

func (s *ibPortsStore) lastEvents(device string, port uint, since time.Time) ([]Event, error) {
	query := fmt.Sprintf(`SELECT %s, %s, %s, %s, %s, %s, %s, %s, %s, %s FROM %s WHERE %s = ? AND %s = ? AND %s != ""`,
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
		historyTableColumnDevice,
		historyTableColumnPort,
		historyTableColumnEventType,
	)
	params := []any{device, port}

	if !since.IsZero() {
		query += fmt.Sprintf(` AND %s >= ?`, historyTableColumnTimestamp)
		params = append(params, since.Unix())
	}

	// limit 1 is fine, since ib flap vs. drop events are mutually exclusive
	query += fmt.Sprintf(` ORDER BY %s DESC LIMIT 1;`, historyTableColumnTimestamp)

	// Read configuration fields under mutex to avoid race conditions
	s.configMu.RLock()
	rootCtx := s.rootCtx
	s.configMu.RUnlock()

	cctx, cancel := context.WithTimeout(rootCtx, 30*time.Second)
	defer cancel()

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
		var port types.IBPort
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
