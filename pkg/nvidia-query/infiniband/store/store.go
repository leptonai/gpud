package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"github.com/leptonai/gpud/pkg/log"
	pkgmetricsrecorder "github.com/leptonai/gpud/pkg/metrics/recorder"
	"github.com/leptonai/gpud/pkg/nvidia-query/infiniband"
)

const schemaVersion = "v0_5_1"

var defaultHistoryTable = "infiniband_device_port_history_" + schemaVersion

const (
	// historyTableColumnTimestamp represents the event timestamp in unix seconds.
	historyTableColumnTimestamp = "timestamp"

	// historyTableColumnDevice represents the device name (e.g., "mlx5_0").
	historyTableColumnDevice = "device"

	// historyTableColumnPort represents the port number (e.g., "1").
	historyTableColumnPort = "port"

	// historyTableColumnLinkLayer represents the link layer of the port (e.g., "Infiniband").
	historyTableColumnLinkLayer = "link_layer"

	// historyTableColumnState represents the state of the port (e.g., "Active").
	historyTableColumnState = "state"

	// historyTableColumnPhysicalState represents the physical state of the port
	// (e.g., "LinkUp").
	historyTableColumnPhysicalState = "physical_state"

	// historyTableColumnRateGBSec represents the rate of the port in GB/s.
	historyTableColumnRateGBSec = "rate_gb_sec"

	// historyTableColumnTotalLinkDowned represents the total number of link downed events.
	historyTableColumnTotalLinkDowned = "total_link_downed"

	// historyTableColumnEventName represents the event type (e.g., "ib flap").
	historyTableColumnEventName = "event_name"

	// historyTableColumnExtraInfo represents the extra information of the port.
	historyTableColumnExtraInfo = "extra_info"
)

var defaultMetadataTable = "infiniband_metadata_" + schemaVersion

const (
	// metadataColumnKey represents the key of the metadata.
	metadataColumnKey = "k"
	// metadataColumnValue represents the value of the metadata.
	metadataColumnValue = "v"

	// metadataKeyLastScanTimestamp represents the timestamp of the last scan in unix seconds.
	metadataKeyLastScanTimestamp = "last_scan_timestamp"
)

const (
	defaultMinInsertInterval  = 15 * time.Second
	defaultScanLookbackPeriod = 10 * time.Minute
	defaultRetentionPeriod    = 5 * 24 * time.Hour
	defaultPurgeInterval      = 10 * time.Hour
)

// Store defines the interface for storing IB ports states.
type Store interface {
	// Insert inserts the IB ports into the store.
	// The timestamp is the time when the IB ports were queried,
	// and all ports are inserted with the same timestamp.
	// Only stores the "Infiniband" link layer ports (not "Ethernet" or "Unknown").
	Insert(eventTime time.Time, ibPorts []infiniband.IBPort) error
	// SetEventName sets the event id for the given timestamp, device, and port.
	SetEventName(timestamp time.Time, device string, port uint, eventName string) error

	// Scan scans the recent events to mark any events
	// (such as "ib port drop").
	Scan() error
}

var _ Store = &ibPortsStore{}

type ibPortsStore struct {
	rootCtx context.Context

	historyTable  string
	metadataTable string

	dbRW *sql.DB
	dbRO *sql.DB

	getTimeNow func() time.Time

	// minInsertInterval is the minimum interval between inserts
	// to prevent excessive inserts.
	minInsertInterval time.Duration

	// scanLookbackPeriod is the lookback period for the scan.
	scanLookbackPeriod time.Duration

	retention     time.Duration
	purgeInterval time.Duration

	lastInsertedTsMu sync.RWMutex
	lastInsertedTs   time.Time

	allMu sync.RWMutex
	// tracks all available device name values
	allDevices map[string]any
	// tracks all available port values
	allPorts map[uint]any
}

// New creates a new IB ports store.
func New(rootCtx context.Context, dbRW *sql.DB, dbRO *sql.DB) (Store, error) {
	s := &ibPortsStore{
		rootCtx: rootCtx,

		historyTable:  defaultHistoryTable,
		metadataTable: defaultMetadataTable,

		dbRW: dbRW,
		dbRO: dbRO,

		getTimeNow: func() time.Time { return time.Now().UTC() },

		minInsertInterval:  defaultMinInsertInterval,
		scanLookbackPeriod: defaultScanLookbackPeriod,

		retention:     defaultRetentionPeriod,
		purgeInterval: defaultPurgeInterval,
	}

	if err := s.init(); err != nil {
		return nil, err
	}

	if s.retention > 0 && s.purgeInterval > 0 {
		go s.runPurge()
	}

	return s, nil
}

func (s *ibPortsStore) init() error {
	ctx, cancel := context.WithTimeout(s.rootCtx, 10*time.Second)
	defer cancel()

	if err := createHistoryTable(ctx, s.dbRW, s.historyTable); err != nil {
		return err
	}
	if err := createMetadataTable(ctx, s.dbRW, s.metadataTable); err != nil {
		return err
	}

	var err error
	s.lastInsertedTs, err = readLastTimestamp(ctx, s.dbRO, s.historyTable)
	if err != nil {
		return err
	}

	// query the existing devices and ports
	s.allDevices, err = readAllDevices(ctx, s.dbRO, s.historyTable)
	if err != nil {
		return err
	}
	s.allPorts, err = readAllPorts(ctx, s.dbRO, s.historyTable)
	if err != nil {
		return err
	}

	return nil
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
		historyTableColumnTimestamp,
		historyTableColumnDevice,
		historyTableColumnPort,
		historyTableColumnLinkLayer,
		historyTableColumnState,
		historyTableColumnPhysicalState,
		historyTableColumnRateGBSec,
		historyTableColumnTotalLinkDowned,
		historyTableColumnEventName,
		historyTableColumnExtraInfo,
	))
	if err != nil {
		_ = tx.Rollback()
		return err
	}

	_, err = tx.ExecContext(ctx, fmt.Sprintf(`CREATE INDEX IF NOT EXISTS idx_%s_%s ON %s(%s);`,
		tableName, historyTableColumnTimestamp, tableName, historyTableColumnTimestamp))
	if err != nil {
		_ = tx.Rollback()
		return err
	}

	_, err = tx.ExecContext(ctx, fmt.Sprintf(`CREATE INDEX IF NOT EXISTS idx_%s_%s ON %s(%s);`,
		tableName, historyTableColumnDevice, tableName, historyTableColumnDevice))
	if err != nil {
		_ = tx.Rollback()
		return err
	}

	_, err = tx.ExecContext(ctx, fmt.Sprintf(`CREATE INDEX IF NOT EXISTS idx_%s_%s ON %s(%s);`,
		tableName, historyTableColumnPort, tableName, historyTableColumnPort))
	if err != nil {
		_ = tx.Rollback()
		return err
	}

	_, err = tx.ExecContext(ctx, fmt.Sprintf(`CREATE INDEX IF NOT EXISTS idx_%s_%s ON %s(%s);`,
		tableName, historyTableColumnEventName, tableName, historyTableColumnEventName))
	if err != nil {
		_ = tx.Rollback()
		return err
	}

	_, err = tx.ExecContext(ctx, fmt.Sprintf(`CREATE INDEX IF NOT EXISTS idx_%s_%s ON %s(%s);`,
		tableName, historyTableColumnState, tableName, historyTableColumnState))
	if err != nil {
		_ = tx.Rollback()
		return err
	}

	return tx.Commit()
}

func createMetadataTable(ctx context.Context, dbRW *sql.DB, tableName string) error {
	tx, err := dbRW.BeginTx(ctx, nil)
	if err != nil {
		return err
	}

	// create table
	_, err = tx.ExecContext(ctx, fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS %s (
	%s TEXT PRIMARY KEY NOT NULL,
	%s TEXT NOT NULL
);`, tableName,
		metadataColumnKey,
		metadataColumnValue,
	))
	if err != nil {
		_ = tx.Rollback()
		return err
	}

	return tx.Commit()
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
	return time.Unix(unixTS, 0), nil
}

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
		historyTableColumnEventName,
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
			"", // no event name when we first insert
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
	s.updateAllDevices(allDevs)
	s.updateAllPorts(allPorts)

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

func (s *ibPortsStore) updateAllDevices(devs map[string]any) {
	s.allMu.Lock()
	for device := range devs {
		_, ok := s.allDevices[device]
		if !ok {
			s.allDevices[device] = nil
		}
	}
	s.allMu.Unlock()
}

func (s *ibPortsStore) getAllDevices() map[string]any {
	s.allMu.RLock()
	all := s.allDevices
	s.allMu.RUnlock()
	return all
}

// readAllDevices selects all distinct device names from the table.
func readAllDevices(ctx context.Context, dbRO *sql.DB, tableName string) (map[string]any, error) {
	query := fmt.Sprintf(`SELECT DISTINCT %s FROM %s;`, historyTableColumnDevice, tableName)

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

func (s *ibPortsStore) updateAllPorts(ports map[uint]any) {
	s.allMu.Lock()
	for port := range ports {
		_, ok := s.allPorts[port]
		if !ok {
			s.allPorts[port] = nil
		}
	}
	s.allMu.Unlock()
}

// readAllPorts selects all distinct port numbers from the table.
func readAllPorts(ctx context.Context, dbRO *sql.DB, tableName string) (map[uint]any, error) {
	query := fmt.Sprintf(`SELECT DISTINCT %s FROM %s;`, historyTableColumnPort, tableName)

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

func (s *ibPortsStore) getAllPorts() map[uint]any {
	s.allMu.RLock()
	all := s.allPorts
	s.allMu.RUnlock()
	return all
}

func (s *ibPortsStore) SetEventName(timestamp time.Time, device string, port uint, eventName string) error {
	query := fmt.Sprintf(`UPDATE %s SET %s = ? WHERE %s = ? AND %s = ? AND %s = ?;`,
		s.historyTable, historyTableColumnEventName,
		historyTableColumnTimestamp,
		historyTableColumnDevice,
		historyTableColumnPort,
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

func (s *ibPortsStore) Scan() error {
	cctx, cancel := context.WithTimeout(s.rootCtx, 30*time.Second)
	lastScanTs, err := getLastScanTimestamp(cctx, s.dbRO, s.metadataTable)
	cancel()
	if err != nil {
		return err
	}

	allDevs := s.getAllDevices()
	allPorts := s.getAllPorts()

	for dev := range allDevs {
		for port := range allPorts {
			if err := s.scanIBPortDrop(dev, port, lastScanTs); err != nil {
				return err
			}
		}
	}

	cctx, cancel = context.WithTimeout(s.rootCtx, 30*time.Second)
	defer cancel()
	return setLastScanTimestamp(cctx, s.dbRW, s.metadataTable, s.getTimeNow())
}

type ibPortSnapshot struct {
	ts              time.Time
	device          string
	port            uint
	state           string
	totalLinkDowned uint64
	eventName       string
}

func (s *ibPortsStore) scanIBPortDrop(device string, port uint, since time.Time) error {
	query := fmt.Sprintf(`SELECT %s, %s, %s, %s FROM %s WHERE %s = ? AND %s = ?`,
		historyTableColumnTimestamp,
		historyTableColumnState,
		historyTableColumnTotalLinkDowned,
		historyTableColumnEventName,
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
			log.Logger.Debugw("no row found during ib port drop scan", "device", device, "port", port, "since", since)
			return nil
		}
		return err
	}
	defer rows.Close()

	snapshots := make([]ibPortSnapshot, 0)
	for rows.Next() {
		var ts int64
		var state string
		var totalLinkDowned int
		var eventName sql.NullString
		if err := rows.Scan(&ts, &state, &totalLinkDowned, &eventName); err != nil {
			return err
		}
		snapshot := ibPortSnapshot{
			ts:              time.Unix(ts, 0),
			device:          device,
			port:            port,
			state:           state,
			totalLinkDowned: uint64(totalLinkDowned),
		}
		if eventName.Valid && eventName.String != "" {
			snapshot.eventName = eventName.String
		}

		snapshots = append(snapshots, snapshot)
	}
	if err = rows.Err(); err != nil {
		return err
	}

	_ = snapshots

	return nil
}

func getLastScanTimestamp(ctx context.Context, dbRO *sql.DB, tableName string) (time.Time, error) {
	query := fmt.Sprintf(`SELECT %s FROM %s WHERE %s = ? LIMIT 1;`, metadataColumnValue, tableName, metadataColumnKey)

	start := time.Now()
	row := dbRO.QueryRowContext(ctx, query, metadataKeyLastScanTimestamp)
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

func setLastScanTimestamp(ctx context.Context, dbRW *sql.DB, tableName string, timestamp time.Time) error {
	query := fmt.Sprintf(`INSERT OR REPLACE INTO %s (%s, %s) VALUES (?, ?);`, tableName, metadataColumnKey, metadataColumnValue)

	start := time.Now()
	_, err := dbRW.ExecContext(ctx, query, metadataKeyLastScanTimestamp, timestamp.Unix())
	pkgmetricsrecorder.RecordSQLiteInsertUpdate(time.Since(start).Seconds())

	if err == nil {
		log.Logger.Infow("successfully set last scan timestamp", "table", tableName, "timestamp", timestamp)
	}
	return err
}

// purge purges old entries from the table
// but retain the rows with non-empty event name,
// because we do not want to lose important events.
func purge(ctx context.Context, dbRW *sql.DB, tableName string, beforeTimestamp int64) (int, error) {
	deleteStatement := fmt.Sprintf(`DELETE FROM %s WHERE %s < ? AND %s = ''`, tableName, historyTableColumnTimestamp, historyTableColumnEventName)

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
		purgeBefore := now.Add(-s.retention)

		cctx, cancel := context.WithTimeout(s.rootCtx, 10*time.Second)
		lastScanTs, err := getLastScanTimestamp(cctx, s.dbRO, s.metadataTable)
		cancel()
		if err != nil {
			log.Logger.Warnw("failed to get last scan timestamp", "table", s.metadataTable, "error", err)
		}
		if !lastScanTs.IsZero() && lastScanTs.Before(purgeBefore) {
			log.Logger.Warnw("scan is still behind, overwriting purge timestamp to save next scan", "table", s.metadataTable, "lastScanTs", lastScanTs, "purgeBefore", purgeBefore)
			purgeBefore = lastScanTs.Add(-2 * s.scanLookbackPeriod)
		}

		cctx, cancel = context.WithTimeout(s.rootCtx, 10*time.Second)
		purged, err := purge(cctx, s.dbRW, s.historyTable, purgeBefore.Unix())
		cancel()
		if err != nil {
			log.Logger.Warnw("failed to purge", "table", s.historyTable, "retention", s.retention, "error", err)
		} else {
			log.Logger.Infow("purged", "table", s.historyTable, "retention", s.retention, "purged", purged)
		}
	}
}
