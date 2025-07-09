package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/leptonai/gpud/pkg/log"
	pkgmetricsrecorder "github.com/leptonai/gpud/pkg/metrics/recorder"
)

// devPortSnapshot is a snapshot of the state
// and its total link downed count of an IB port.
type devPortSnapshot struct {
	ts              time.Time
	state           string
	totalLinkDowned uint64
}

// devPortSnapshots is a list of [ibPortSnapshot]s for the same device and port.
type devPortSnapshots []devPortSnapshot

type devPortSnapshotWithReason struct {
	reason string
	devPortSnapshot
}

func (s *ibPortsStore) readDevPortSnapshots(device string, port uint, since time.Time) (devPortSnapshots, error) {
	query := fmt.Sprintf(`SELECT %s, %s, %s FROM %s WHERE %s = ? AND %s = ?`,
		historyTableColumnTimestamp,
		historyTableColumnState,
		historyTableColumnTotalLinkDowned,
		s.historyTable,
		historyTableColumnDevice,
		historyTableColumnPort,
	)

	params := []any{device, port}

	if !since.IsZero() {
		query += fmt.Sprintf(` AND %s >= ?`, historyTableColumnTimestamp)
		params = append(params, since.Unix())
	}

	query += fmt.Sprintf(` ORDER BY %s ASC;`, historyTableColumnTimestamp)

	cctx, cancel := context.WithTimeout(s.rootCtx, 30*time.Second)
	defer cancel()

	start := time.Now()
	rows, err := s.dbRO.QueryContext(cctx, query, params...)
	pkgmetricsrecorder.RecordSQLiteSelect(time.Since(start).Seconds())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			log.Logger.Debugw("no row found", "device", device, "port", port, "since", since)
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()

	snapshots := make([]devPortSnapshot, 0)
	for rows.Next() {
		var ts int64
		var state string
		var totalLinkDowned int
		if err := rows.Scan(&ts, &state, &totalLinkDowned); err != nil {
			return nil, err
		}

		snapshot := devPortSnapshot{
			ts:              time.Unix(ts, 0).UTC(),
			state:           state,
			totalLinkDowned: uint64(totalLinkDowned),
		}
		snapshots = append(snapshots, snapshot)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return snapshots, nil
}
