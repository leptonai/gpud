// Package gpudstate provides the persistent storage layer for component states.
package gpudstate

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/prometheus/client_golang/prometheus"

	pkgmetrics "github.com/leptonai/gpud/pkg/metrics"
	"github.com/leptonai/gpud/pkg/sqlite"
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

// ReadMachineID reads the machine ID from the database.
// Returns an empty string and no error, if the machine ID is not found.
func ReadMachineID(ctx context.Context, dbRO *sql.DB) (string, error) {
	query := fmt.Sprintf(`
SELECT %s FROM %s
LIMIT 1;
`,
		ColumnMachineID,
		TableNameMachineMetadata,
	)

	var machineID string
	err := dbRO.QueryRowContext(ctx, query).Scan(&machineID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			err = nil
		}
	}
	return machineID, err
}

// RecordMachineID records the machine ID in the database.
// Returns no error if the machine ID is already assigned with the same value.
// Returns an error if the machine ID is already assigned with a different value.
func RecordMachineID(ctx context.Context, dbRW *sql.DB, dbRO *sql.DB, uid string) error {
	existingID, err := ReadMachineID(ctx, dbRO)
	if err != nil {
		return err
	}
	if existingID == uid {
		return nil
	}
	if existingID != "" {
		return fmt.Errorf("machine ID %s already assigned", existingID)
	}

	// was never inserted, thus insert the one now
	query := fmt.Sprintf(`
INSERT OR REPLACE INTO %s (%s, %s) VALUES (?, ?);
`,
		TableNameMachineMetadata,
		ColumnMachineID,
		ColumnUnixSeconds,
	)

	start := time.Now()
	_, err = dbRW.ExecContext(ctx, query, uid, time.Now().Unix())
	sqlite.RecordInsertUpdate(time.Since(start).Seconds())

	return err
}

func GetLoginInfo(ctx context.Context, dbRO *sql.DB, machineID string) (string, error) {
	query := fmt.Sprintf(`
SELECT COALESCE(%s, '') FROM %s WHERE %s = ?
LIMIT 1;
`,
		ColumnToken,
		TableNameMachineMetadata,
		ColumnMachineID,
	)

	start := time.Now()
	var token string
	err := dbRO.QueryRowContext(ctx, query, machineID).Scan(&token)
	sqlite.RecordSelect(time.Since(start).Seconds())

	return token, err
}

func UpdateLoginInfo(ctx context.Context, dbRW *sql.DB, machineID string, token string) error {
	query := fmt.Sprintf(`
UPDATE %s SET %s = ? WHERE %s = ?;
`,
		TableNameMachineMetadata,
		ColumnToken,
		ColumnMachineID,
	)

	start := time.Now()
	_, err := dbRW.ExecContext(ctx, query, token, machineID)
	sqlite.RecordInsertUpdate(time.Since(start).Seconds())

	return err
}

func GetComponents(ctx context.Context, db *sql.DB, machineID string) (string, error) {
	query := fmt.Sprintf(`
SELECT COALESCE(%s, '') FROM %s WHERE %s = ?
LIMIT 1;
`,
		ColumnComponents,
		TableNameMachineMetadata,
		ColumnMachineID,
	)

	start := time.Now()
	var components string
	err := db.QueryRowContext(ctx, query, machineID).Scan(&components)
	sqlite.RecordSelect(time.Since(start).Seconds())

	return components, err
}

func UpdateComponents(ctx context.Context, db *sql.DB, machineID string, components string) error {
	query := fmt.Sprintf(`
UPDATE %s SET %s = ? WHERE %s = ?;
`,
		TableNameMachineMetadata,
		ColumnComponents,
		ColumnMachineID,
	)

	start := time.Now()
	_, err := db.ExecContext(ctx, query, components, machineID)
	sqlite.RecordInsertUpdate(time.Since(start).Seconds())

	return err
}

var (
	metricCurrentSize = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "gpud",
			Subsystem: "state_sqlite",
			Name:      "current_size",
			Help:      "current size of the database file (number of pages * size of page)",
		},
	)
)

func init() {
	pkgmetrics.MustRegister(metricCurrentSize)
}

func RecordMetrics(ctx context.Context, db *sql.DB) error {
	dbSize, err := sqlite.ReadDBSize(ctx, db)
	if err != nil {
		return err
	}
	metricCurrentSize.Set(float64(dbSize))

	return nil
}
