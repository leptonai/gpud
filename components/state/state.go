// Package state provides the persistent storage layer for component states.
package state

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/leptonai/gpud/log"
	"github.com/leptonai/gpud/pkg/host"
	"github.com/leptonai/gpud/pkg/sqlite"

	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	TableNameMachineMetadata = "machine_metadata"

	ColumnMachineID   = "machine_id"
	ColumnUnixSeconds = "unix_seconds"
	ColumnToken       = "token"
	ColumnComponents  = "components"
)

func CreateTableMachineMetadata(ctx context.Context, dbRW *sql.DB) error {
	_, err := dbRW.ExecContext(ctx, fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS %s (
	%s TEXT PRIMARY KEY,
	%s INTEGER,
	%s TEXT,
	%s TEXT
);`, TableNameMachineMetadata, ColumnMachineID, ColumnUnixSeconds, ColumnToken, ColumnComponents))
	return err
}

// Reads the machine ID from the database.
// Returns an empty string and sql.ErrNoRows if the machine ID is not found.
func GetMachineID(ctx context.Context, dbRO *sql.DB) (string, error) {
	query := fmt.Sprintf(`
SELECT %s FROM %s
LIMIT 1;
`,
		ColumnMachineID,
		TableNameMachineMetadata,
	)

	var machineID string
	err := dbRO.QueryRowContext(ctx, query).Scan(&machineID)
	return machineID, err
}

func CreateMachineIDIfNotExist(ctx context.Context, dbRW *sql.DB, dbRO *sql.DB, providedUID string) (string, error) {
	query := fmt.Sprintf(`
SELECT %s, %s FROM %s
LIMIT 1;
`,
		ColumnMachineID,
		ColumnUnixSeconds,
		TableNameMachineMetadata,
	)

	var (
		machineID   string
		unixSeconds int64
	)

	start := time.Now()
	err := dbRO.QueryRowContext(ctx, query).Scan(&machineID, &unixSeconds)
	sqlite.RecordSelect(time.Since(start).Seconds())

	if err == nil { // reuse existing machine ID
		return machineID, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}

	uid := providedUID
	if uid == "" {
		uid, err = host.GetMachineID(ctx)
		if err != nil {
			log.Logger.Warnw("failed to get machine ID", "error", err)
		}
	}
	if uid == "" { // fallback to random UUID
		u, err := uuid.NewUUID()
		if err != nil {
			return "", err
		}
		uid = u.String()
	}

	// was never inserted, thus insert the one now
	query = fmt.Sprintf(`
INSERT OR REPLACE INTO %s (%s, %s) VALUES (?, ?);
`,
		TableNameMachineMetadata,
		ColumnMachineID,
		ColumnUnixSeconds,
	)

	start = time.Now()
	_, err = dbRW.ExecContext(ctx, query, uid, time.Now().Unix())
	sqlite.RecordInsertUpdate(time.Since(start).Seconds())

	if err != nil {
		return "", err
	}
	return uid, nil
}

func GetLoginInfo(ctx context.Context, dbRO *sql.DB, machineID string) (string, error) {
	query := fmt.Sprintf(`
SELECT %s FROM %s WHERE %s = '%s'
LIMIT 1;
`,
		ColumnToken,
		TableNameMachineMetadata,
		ColumnMachineID,
		machineID,
	)

	start := time.Now()
	var token string
	err := dbRO.QueryRowContext(ctx, query).Scan(&token)
	sqlite.RecordSelect(time.Since(start).Seconds())

	return token, err
}

func UpdateLoginInfo(ctx context.Context, dbRW *sql.DB, machineID string, token string) error {
	query := fmt.Sprintf(`
UPDATE %s SET %s = '%s' WHERE %s = '%s';
`,
		TableNameMachineMetadata,
		ColumnToken,
		token,
		ColumnMachineID,
		machineID,
	)

	start := time.Now()
	_, err := dbRW.ExecContext(ctx, query)
	sqlite.RecordInsertUpdate(time.Since(start).Seconds())

	return err
}

func GetComponents(ctx context.Context, db *sql.DB, machineID string) (string, error) {
	query := fmt.Sprintf(`
SELECT %s FROM %s WHERE %s = '%s'
LIMIT 1;
`,
		ColumnComponents,
		TableNameMachineMetadata,
		ColumnMachineID,
		machineID,
	)

	start := time.Now()
	var components string
	err := db.QueryRowContext(ctx, query).Scan(&components)
	sqlite.RecordSelect(time.Since(start).Seconds())

	return components, err
}

func UpdateComponents(ctx context.Context, db *sql.DB, machineID string, components string) error {
	query := fmt.Sprintf(`
UPDATE %s SET %s = '%s' WHERE %s = '%s';
`,
		TableNameMachineMetadata,
		ColumnComponents,
		components,
		ColumnMachineID,
		machineID,
	)

	start := time.Now()
	_, err := db.ExecContext(ctx, query)
	sqlite.RecordInsertUpdate(time.Since(start).Seconds())

	return err
}

var (
	currentSize = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "gpud",
			Subsystem: "state_sqlite",
			Name:      "current_size",
			Help:      "current size of the database file (number of pages * size of page)",
		},
	)
)

func Register(reg *prometheus.Registry) error {
	if err := reg.Register(currentSize); err != nil {
		return err
	}
	return nil
}

func ReadDBSize(ctx context.Context, db *sql.DB) (uint64, error) {
	var pageCount uint64
	err := db.QueryRowContext(ctx, "PRAGMA page_count").Scan(&pageCount)
	if err == sql.ErrNoRows {
		return 0, errors.New("no page count")
	}
	if err != nil {
		return 0, err
	}

	var pageSize uint64
	err = db.QueryRowContext(ctx, "PRAGMA page_size").Scan(&pageSize)
	if err == sql.ErrNoRows {
		return 0, errors.New("no page size")
	}
	if err != nil {
		return 0, err
	}

	return pageCount * pageSize, nil
}

func RecordMetrics(ctx context.Context, db *sql.DB) error {
	dbSize, err := ReadDBSize(ctx, db)
	if err != nil {
		return err
	}
	currentSize.Set(float64(dbSize))

	return nil
}

func Compact(ctx context.Context, db *sql.DB) error {
	log.Logger.Infow("compacting state database")
	_, err := db.ExecContext(ctx, "VACUUM;")
	if err != nil {
		return err
	}
	log.Logger.Infow("successfully compacted state database")
	return nil
}
