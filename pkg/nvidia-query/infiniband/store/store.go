package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"github.com/leptonai/gpud/pkg/log"
	pkgmetricsrecorder "github.com/leptonai/gpud/pkg/metrics/recorder"
	"github.com/leptonai/gpud/pkg/nvidia-query/infiniband"
)

const schemaVersion = "v0_5_1"

var (
	defaultHistoryTable = "infiniband_device_port_history_" + schemaVersion
)

const (
	// columnTimestamp represents the event timestamp in unix seconds.
	columnTimestamp = "timestamp"

	// columnDevice represents the device name (e.g., "mlx5_0").
	columnDevice = "device"

	// columnPort represents the port number (e.g., "1").
	columnPort = "port"

	// columnLinkLayer represents the link layer of the port (e.g., "Infiniband").
	columnLinkLayer = "link_layer"

	// columnState represents the state of the port (e.g., "Active").
	columnState = "state"

	// columnPhysicalState represents the physical state of the port
	// (e.g., "LinkUp").
	columnPhysicalState = "physical_state"

	// columnRateGBSec represents the rate of the port in GB/s.
	columnRateGBSec = "rate_gb_sec"

	// columnTotalLinkDowned represents the total number of link downed events.
	columnTotalLinkDowned = "total_link_downed"

	// columnEventName represents the event type (e.g., "ib flap").
	columnEventName = "event_name"

	// columnExtraInfo represents the extra information of the port.
	columnExtraInfo = "extra_info"
)

const (
	defaultMinInsertInterval = 15 * time.Second
	defaultRetentionPeriod   = 5 * 24 * time.Hour
	defaultPurgeInterval     = 10 * time.Hour
)

// IBPortsEvent represents an IB ports event,
// which contains the time and the IB ports.
type IBPortsEvent struct {
	Time    time.Time
	IBPorts []infiniband.IBPort
}

// IBPortsEvents is a slice of IB ports events.
type IBPortsEvents []IBPortsEvent

// Store defines the interface for storing IB ports states.
type Store interface {
	// Insert inserts the IB ports into the store.
	// The timestamp is the time when the IB ports were queried.
	// Only stores the "Infiniband" link layer ports (not "Ethernet" or "Unknown").
	Insert(event *IBPortsEvent) error
	// SetEventName sets the event id for the given timestamp, device, and port.
	SetEventName(timestamp time.Time, device string, port uint, eventName string) error

	// Scan scans the recent events for the given lookback period,
	// and mark any events that are detected (such as "ib port drop").
	Scan(lookbackPeriod time.Duration) error

	// Read reads entries from the store since the given time.
	// The returned entries are sorted by timestamp in ascending order
	// (latest entry in the last order) and grouped by timestamp.
	Read(since time.Time) ([]IBPortEntries, error)
}

var _ Store = &ibPortsStore{}

type ibPortsStore struct {
	rootCtx context.Context

	historyTable string

	dbRW *sql.DB
	dbRO *sql.DB

	getTimeNow func() time.Time

	// minInsertInterval is the minimum interval between inserts
	// to prevent excessive inserts.
	minInsertInterval time.Duration

	retention     time.Duration
	purgeInterval time.Duration

	lastTimestampMu sync.RWMutex
	lastTimestamp   time.Time

	allMu sync.RWMutex
	// tracks all available device name values
	allDevices map[string]any
	// tracks all available port values
	allPorts map[uint]any
}

// New creates a new IB ports store.
func New(rootCtx context.Context, dbRW *sql.DB, dbRO *sql.DB) (Store, error) {
	ctx, cancel := context.WithTimeout(rootCtx, 10*time.Second)
	defer cancel()

	if err := createHistoryTable(ctx, dbRW, defaultHistoryTable); err != nil {
		return nil, err
	}

	lastTimestamp, err := getLastTimestamp(ctx, dbRO, defaultHistoryTable)
	if err != nil {
		return nil, err
	}

	// query the existing devices and ports
	allDevices, err := selectAllDevices(ctx, dbRO, defaultHistoryTable)
	if err != nil {
		return nil, err
	}
	allPorts, err := selectAllPorts(ctx, dbRO, defaultHistoryTable)
	if err != nil {
		return nil, err
	}

	s := &ibPortsStore{
		rootCtx:      rootCtx,
		historyTable: defaultHistoryTable,

		dbRW: dbRW,
		dbRO: dbRO,

		getTimeNow: func() time.Time { return time.Now().UTC() },

		minInsertInterval: defaultMinInsertInterval,
		retention:         defaultRetentionPeriod,
		purgeInterval:     defaultPurgeInterval,

		lastTimestamp: lastTimestamp,

		allDevices: allDevices,
		allPorts:   allPorts,
	}

	if s.retention > 0 && s.purgeInterval > 0 {
		go s.runPurge()
	}

	return s, nil
}

func createHistoryTable(ctx context.Context, dbRW *sql.DB, tableName string) error {
	tx, err := dbRW.BeginTx(ctx, nil)
	if err != nil {
		return err
	}

	// create table
	_, err = tx.ExecContext(ctx, fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS %s (
	%s TEXT NOT NULL,
	%s INTEGER NOT NULL,
	%s TEXT NOT NULL,
	%s TEXT NOT NULL,
	%s TEXT NOT NULL,
	%s TEXT NOT NULL,
	%s INTEGER NOT NULL,
	%s INTEGER NOT NULL,
	%s TEXT,
	%s TEXT
);`, tableName,
		columnTimestamp,
		columnDevice,
		columnPort,
		columnLinkLayer,
		columnState,
		columnPhysicalState,
		columnRateGBSec,
		columnTotalLinkDowned,
		columnEventName,
		columnExtraInfo,
	))
	if err != nil {
		_ = tx.Rollback()
		return err
	}

	_, err = tx.ExecContext(ctx, fmt.Sprintf(`CREATE INDEX IF NOT EXISTS idx_%s_%s ON %s(%s);`,
		tableName, columnTimestamp, tableName, columnTimestamp))
	if err != nil {
		_ = tx.Rollback()
		return err
	}

	_, err = tx.ExecContext(ctx, fmt.Sprintf(`CREATE INDEX IF NOT EXISTS idx_%s_%s ON %s(%s);`,
		tableName, columnDevice, tableName, columnDevice))
	if err != nil {
		_ = tx.Rollback()
		return err
	}

	_, err = tx.ExecContext(ctx, fmt.Sprintf(`CREATE INDEX IF NOT EXISTS idx_%s_%s ON %s(%s);`,
		tableName, columnPort, tableName, columnPort))
	if err != nil {
		_ = tx.Rollback()
		return err
	}

	_, err = tx.ExecContext(ctx, fmt.Sprintf(`CREATE INDEX IF NOT EXISTS idx_%s_%s ON %s(%s);`,
		tableName, columnEventName, tableName, columnEventName))
	if err != nil {
		_ = tx.Rollback()
		return err
	}

	_, err = tx.ExecContext(ctx, fmt.Sprintf(`CREATE INDEX IF NOT EXISTS idx_%s_%s ON %s(%s);`,
		tableName, columnState, tableName, columnState))
	if err != nil {
		_ = tx.Rollback()
		return err
	}

	return tx.Commit()
}

// getLastTimestamp returns the last timestamp of the table.
// If the table is empty, it returns a zero time.
func getLastTimestamp(ctx context.Context, dbRO *sql.DB, tableName string) (time.Time, error) {
	query := fmt.Sprintf(`SELECT %s FROM %s ORDER BY %s DESC LIMIT 1;`, columnTimestamp, tableName, columnTimestamp)

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
	return time.Unix(unixTS, 0), nil
}

// selectAllDevices selects all distinct device names from the table.
func selectAllDevices(ctx context.Context, dbRO *sql.DB, tableName string) (map[string]any, error) {
	query := fmt.Sprintf(`SELECT DISTINCT %s FROM %s;`, columnDevice, tableName)

	rows, err := dbRO.QueryContext(ctx, query)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return make(map[string]any), nil
		}
		return nil, err
	}
	defer rows.Close()

	devices := make(map[string]any)
	for rows.Next() {
		var device string
		if err := rows.Scan(&device); err != nil {
			return nil, err
		}
		devices[device] = nil
	}

	if err = rows.Err(); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return make(map[string]any), nil
		}
		return nil, err
	}

	return devices, nil
}

// selectAllPorts selects all distinct port numbers from the table.
func selectAllPorts(ctx context.Context, dbRO *sql.DB, tableName string) (map[uint]any, error) {
	query := fmt.Sprintf(`SELECT DISTINCT %s FROM %s;`, columnPort, tableName)

	rows, err := dbRO.QueryContext(ctx, query)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return make(map[uint]any), nil
		}
		return nil, err
	}
	defer rows.Close()

	ports := make(map[uint]any)
	for rows.Next() {
		var port uint
		if err := rows.Scan(&port); err != nil {
			return nil, err
		}
		ports[port] = nil
	}

	if err = rows.Err(); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return make(map[uint]any), nil
		}
		return nil, err
	}

	return ports, nil
}

func (s *ibPortsStore) Insert(event *IBPortsEvent) error {
	if event == nil {
		return nil
	}

	s.lastTimestampMu.RLock()
	lastTimestamp := s.lastTimestamp
	s.lastTimestampMu.RUnlock()

	if !lastTimestamp.IsZero() && event.Time.Before(lastTimestamp.Add(s.minInsertInterval)) {
		log.Logger.Warnw("skipping insert", "table", s.historyTable, "event", event, "lastTs", lastTimestamp, "minInsertInterval", s.minInsertInterval)
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
		columnTimestamp,
		columnDevice,
		columnPort,
		columnLinkLayer,
		columnState,
		columnPhysicalState,
		columnRateGBSec,
		columnTotalLinkDowned,
		columnEventName,
		columnExtraInfo,
	)

	stmt, err := tx.PrepareContext(cctx, insertStatement)
	if err != nil {
		return err
	}
	defer stmt.Close()

	allDevs := make(map[string]any)
	allPorts := make(map[uint]any)
	for _, port := range event.IBPorts {
		_, err = stmt.ExecContext(
			cctx,
			event.Time.Unix(),
			strings.TrimSpace(strings.ToLower(port.Device)),
			port.Port,
			strings.TrimSpace(strings.ToLower(port.LinkLayer)),
			strings.TrimSpace(strings.ToLower(port.State)),
			strings.TrimSpace(strings.ToLower(port.PhysicalState)),
			port.RateGBSec,
			port.TotalLinkDowned,
			"", // no event id when we first insert
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

	s.lastTimestampMu.Lock()
	s.lastTimestamp = event.Time
	s.lastTimestampMu.Unlock()

	s.allMu.Lock()
	for device := range allDevs {
		_, ok := s.allDevices[device]
		if !ok {
			s.allDevices[device] = nil
		}
	}
	for port := range allPorts {
		_, ok := s.allPorts[port]
		if !ok {
			s.allPorts[port] = nil
		}
	}
	s.allMu.Unlock()

	return nil
}

func (s *ibPortsStore) SetEventName(timestamp time.Time, device string, port uint, eventName string) error {
	query := fmt.Sprintf(`UPDATE %s SET %s = ? WHERE %s = ? AND %s = ? AND %s = ?;`,
		s.historyTable, columnEventName,
		columnTimestamp,
		columnDevice,
		columnPort,
	)

	cctx, cancel := context.WithTimeout(s.rootCtx, 30*time.Second)
	defer cancel()

	now := s.getTimeNow()
	rs, err := s.dbRW.ExecContext(cctx, query, eventName, timestamp.Unix(), device, port)
	pkgmetricsrecorder.RecordSQLiteInsertUpdate(s.getTimeNow().Sub(now).Seconds())
	if err != nil {
		return err
	}

	affected, err := rs.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		log.Logger.Warnw("no row affected", "table", s.historyTable, "timestamp", timestamp, "device", device, "port", port, "event", eventName)
	} else {
		log.Logger.Infow("set event", "table", s.historyTable, "timestamp", timestamp, "device", device, "port", port, "event", eventName)
	}

	return nil
}

// purge purges old entries from the table
// but retain the rows with non-empty event name,
// because we do not want to lose important events.
func purge(ctx context.Context, dbRW *sql.DB, tableName string, beforeTimestamp int64) (int, error) {
	deleteStatement := fmt.Sprintf(`DELETE FROM %s WHERE %s < ? AND %s = ''`, tableName, columnTimestamp, columnEventName)

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

func (s *ibPortsStore) runPurge() {
	log.Logger.Infow("start purging", "table", s.historyTable, "retention", s.retention, "checkInterval", s.purgeInterval)
	for {
		select {
		case <-s.rootCtx.Done():
			return
		case <-time.After(s.purgeInterval):
		}

		now := s.getTimeNow()
		cctx, cancel := context.WithTimeout(s.rootCtx, 10*time.Second)
		purged, err := purge(cctx, s.dbRW, s.historyTable, now.Add(-s.retention).Unix())
		cancel()
		if err != nil {
			log.Logger.Warnw("failed to purge", "table", s.historyTable, "retention", s.retention, "error", err)
		} else {
			log.Logger.Infow("purged", "table", s.historyTable, "retention", s.retention, "purged", purged)
		}
	}
}

// IBPortEntry represents an IB ports state in the database.
// All string fields are stored in lower case.
type IBPortEntry struct {
	Timestamp       time.Time
	Device          string
	Port            uint
	LinkLayer       string
	State           string
	PhysState       string
	RateGBSec       int
	TotalLinkDowned int
	Event           string
	ExtraInfo       map[string]string
}

type IBPortEntries struct {
	Timestamp time.Time
	Entries   []IBPortEntry
}

// Read reads entries from the store since the given time.
// The returned entries are sorted by timestamp in ascending order
// (latest entry in the last order) and grouped by timestamp.
func (s *ibPortsStore) Read(since time.Time) ([]IBPortEntries, error) {
	query := fmt.Sprintf(`SELECT %s, %s, %s, %s, %s, %s, %s, %s, %s, %s FROM %s WHERE %s >= ? ORDER BY %s ASC;`,
		columnTimestamp, columnDevice, columnPort, columnLinkLayer, columnState, columnPhysicalState, columnRateGBSec, columnTotalLinkDowned, columnEventName, columnExtraInfo,
		s.historyTable, columnTimestamp, columnTimestamp)

	cctx, cancel := context.WithTimeout(s.rootCtx, 30*time.Second)
	defer cancel()

	rows, err := s.dbRO.QueryContext(cctx, query, since.Unix())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	groupedByTs := make(map[time.Time][]IBPortEntry, 0)
	for rows.Next() {
		var unixTS int64
		var eventRaw sql.NullString
		var extraInfoRaw sql.NullString
		var ent IBPortEntry
		if err := rows.Scan(&unixTS, &ent.Device, &ent.Port, &ent.LinkLayer, &ent.State, &ent.PhysState, &ent.RateGBSec, &ent.TotalLinkDowned, &eventRaw, &extraInfoRaw); err != nil {
			return nil, err
		}

		ent.Timestamp = time.Unix(unixTS, 0)
		ent.Event = eventRaw.String

		if extraInfoRaw.Valid && extraInfoRaw.String != "" {
			ent.ExtraInfo = make(map[string]string)
			if err := json.Unmarshal([]byte(extraInfoRaw.String), &ent.ExtraInfo); err != nil {
				return nil, err
			}
		}

		if _, ok := groupedByTs[ent.Timestamp]; !ok {
			groupedByTs[ent.Timestamp] = make([]IBPortEntry, 0)
		}
		groupedByTs[ent.Timestamp] = append(groupedByTs[ent.Timestamp], ent)
	}

	all := make([]IBPortEntries, 0)
	for ts, entries := range groupedByTs {
		all = append(all, IBPortEntries{Timestamp: ts, Entries: entries})
	}
	sort.Slice(all, func(i, j int) bool {
		return all[i].Timestamp.Before(all[j].Timestamp)
	})

	return all, nil
}

// Scan scans the recent events for the given lookback period,
// and mark any events that are detected (such as "ib port drop").
func (s *ibPortsStore) Scan(lookbackPeriod time.Duration) error {
	cctx, cancel := context.WithTimeout(s.rootCtx, 30*time.Second)
	defer cancel()

	_ = cctx

	return nil
}
