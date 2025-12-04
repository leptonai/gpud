package store

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"
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

	// historyTableColumnEventType represents the event type (e.g., "ib flap").
	historyTableColumnEventType = "event_type"

	// historyTableColumnEventReason represents more detailed information of the event.
	historyTableColumnEventReason = "event_reason"

	// historyTableColumnExtraInfo represents the extra information of the port.
	historyTableColumnExtraInfo = "extra_info"
)

var _ Store = &ibPortsStore{}

type ibPortsStore struct {
	// configMu protects configuration fields that can be modified by tests
	configMu sync.RWMutex

	rootCtx context.Context

	historyTable  string
	metadataTable string

	dbRW *sql.DB
	dbRO *sql.DB

	getTimeNow func() time.Time

	// minInsertInterval is the minimum interval between inserts
	// to prevent excessive inserts.
	minInsertInterval time.Duration

	// retentionPeriod is the period to retain the data when purging
	// to be overridden by tombstone timestamp if the tombstone is set
	// to a later timestamp
	// and also used for scanning the data since we do want to scan
	// all the timeseries data to evaluate with 12-hour window
	retentionPeriod time.Duration
	// purgeInterval is the interval to purge the data with retention
	purgeInterval time.Duration

	// minimum duration between down events until it's marked as "drop" candidates
	// (e.g., 4 minutes means that if a port is down for more than 4 minutes,
	// it's marked as "drop" candidates, and 3 minutes is not long enough)
	ibPortDropThreshold time.Duration

	// ibPortFlapDownIntervalThreshold is the minimum duration between down events
	// until it's marked as "flap" candidates
	ibPortFlapDownIntervalThreshold time.Duration
	// ibPortFlapBackToActiveThreshold is the minimum number of times that "down" ports
	// need to revert back to active to be considered as "flap"
	ibPortFlapBackToActiveThreshold int

	lastInsertedTsMu sync.RWMutex
	lastInsertedTs   time.Time

	allMu sync.RWMutex
	// tracks all available device name values
	allDeviceValues map[string]any
	// tracks all available port values
	allPortValues map[uint]any
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

		minInsertInterval: defaultMinInsertInterval,

		retentionPeriod: defaultRetentionPeriod,
		purgeInterval:   defaultPurgeInterval,

		ibPortDropThreshold: defaultIbPortDropThreshold,

		ibPortFlapDownIntervalThreshold: defaultIbPortFlapDownIntervalThreshold,
		ibPortFlapBackToActiveThreshold: defaultIbPortFlapBackToActiveThreshold,
	}

	if err := s.init(); err != nil {
		return nil, err
	}

	if s.purgeInterval > 0 {
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
	s.allDeviceValues, err = readAllDeviceValues(ctx, s.dbRO, s.historyTable)
	if err != nil {
		return err
	}
	s.allPortValues, err = readAllPortValues(ctx, s.dbRO, s.historyTable)
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
		historyTableColumnEventType,
		historyTableColumnEventReason,
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
		tableName, historyTableColumnEventType, tableName, historyTableColumnEventType))
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

func (s *ibPortsStore) updateAllDeviceValues(devs map[string]any) {
	s.allMu.Lock()
	for device := range devs {
		_, ok := s.allDeviceValues[device]
		if !ok {
			s.allDeviceValues[device] = nil
		}
	}
	s.allMu.Unlock()
}

func (s *ibPortsStore) getAllDeviceValues() map[string]any {
	s.allMu.RLock()
	defer s.allMu.RUnlock()

	// Create a copy to prevent race conditions when the caller iterates over the map
	copy := make(map[string]any, len(s.allDeviceValues))
	for k, v := range s.allDeviceValues {
		copy[k] = v
	}
	return copy
}

// readAllDeviceValues selects all distinct device names from the table.
func readAllDeviceValues(ctx context.Context, dbRO *sql.DB, tableName string) (map[string]any, error) {
	query := fmt.Sprintf(`SELECT DISTINCT %s FROM %s;`, historyTableColumnDevice, tableName)

	rows, err := dbRO.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = rows.Close()
	}()

	devices := make(map[string]any)
	for rows.Next() {
		var device string
		if err := rows.Scan(&device); err != nil {
			return nil, err
		}
		devices[device] = nil
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return devices, nil
}

func (s *ibPortsStore) updateAllPortValues(ports map[uint]any) {
	s.allMu.Lock()
	for port := range ports {
		_, ok := s.allPortValues[port]
		if !ok {
			s.allPortValues[port] = nil
		}
	}
	s.allMu.Unlock()
}

func (s *ibPortsStore) getAllPortValues() map[uint]any {
	s.allMu.RLock()
	defer s.allMu.RUnlock()

	// Create a copy to prevent race conditions when the caller iterates over the map
	copy := make(map[uint]any, len(s.allPortValues))
	for k, v := range s.allPortValues {
		copy[k] = v
	}
	return copy
}

// readAllPortValues selects all distinct port numbers from the table.
func readAllPortValues(ctx context.Context, dbRO *sql.DB, tableName string) (map[uint]any, error) {
	query := fmt.Sprintf(`SELECT DISTINCT %s FROM %s;`, historyTableColumnPort, tableName)

	rows, err := dbRO.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = rows.Close()
	}()

	ports := make(map[uint]any)
	for rows.Next() {
		var port uint
		if err := rows.Scan(&port); err != nil {
			return nil, err
		}
		ports[port] = nil
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return ports, nil
}
