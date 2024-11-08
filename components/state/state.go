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

func CreateTable(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS %s (
	%s TEXT PRIMARY KEY,
	%s INTEGER,
	%s TEXT,
	%s TEXT
);`, TableNameMachineMetadata, ColumnMachineID, ColumnUnixSeconds, ColumnToken, ColumnComponents))
	return err
}

func CreateMachineIDIfNotExist(ctx context.Context, db *sql.DB, providedUID string) (string, time.Time, error) {
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
	if err := db.QueryRowContext(ctx, query).Scan(&machineID, &unixSeconds); err == nil {
		return machineID, time.Unix(unixSeconds, 0), nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return "", time.Time{}, err
	}

	insertTime := time.Now()
	var uid string
	if providedUID != "" {
		uid = providedUID
	} else if dmiUUID, err := host.UUID(ctx); err == nil {
		uid = dmiUUID
	} else {
		generateUUID, err := uuid.NewUUID()
		if err != nil {
			return "", time.Time{}, err
		}
		uid = generateUUID.String()
	}
	query = fmt.Sprintf(`
INSERT OR REPLACE INTO %s (%s, %s) VALUES (?, ?);
`,
		TableNameMachineMetadata,
		ColumnMachineID,
		ColumnUnixSeconds,
	)
	if _, err := db.ExecContext(ctx, query, uid, insertTime.UTC().Unix()); err != nil {
		return "", time.Time{}, err
	}
	return uid, insertTime, nil
}

func GetLoginInfo(ctx context.Context, db *sql.DB, machineID string) (string, error) {
	query := fmt.Sprintf(`
SELECT %s FROM %s WHERE %s = '%s'
LIMIT 1;
`,
		ColumnToken,
		TableNameMachineMetadata,
		ColumnMachineID,
		machineID,
	)
	var token string
	err := db.QueryRowContext(ctx, query).Scan(&token)
	return token, err
}

func UpdateLoginInfo(ctx context.Context, db *sql.DB, machineID string, token string) error {
	query := fmt.Sprintf(`
UPDATE %s SET %s = '%s' WHERE %s = '%s';
`,
		TableNameMachineMetadata,
		ColumnToken,
		token,
		ColumnMachineID,
		machineID,
	)
	if _, err := db.ExecContext(ctx, query); err != nil {
		return err
	}
	return nil
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
	var components string
	err := db.QueryRowContext(ctx, query).Scan(&components)
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
	if _, err := db.ExecContext(ctx, query); err != nil {
		return err
	}
	return nil
}

var (
	currentPages = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "gpud",
			Subsystem: "state_sqlite",
			Name:      "current_pages",
			Help:      "current number of pages",
		},
	)
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
	if err := reg.Register(currentPages); err != nil {
		return err
	}
	if err := reg.Register(currentSize); err != nil {
		return err
	}
	return nil
}

func RecordMetrics(ctx context.Context, db *sql.DB) error {
	var pageCount uint64
	err := db.QueryRowContext(ctx, "PRAGMA page_count").Scan(&pageCount)
	if err == sql.ErrNoRows {
		return errors.New("no page count")
	}
	if err != nil {
		return err
	}
	currentPages.Set(float64(pageCount))

	var pageSize uint64
	err = db.QueryRowContext(ctx, "PRAGMA page_size").Scan(&pageSize)
	if err == sql.ErrNoRows {
		return errors.New("no page size")
	}
	if err != nil {
		return err
	}
	currentSize.Set(float64(pageCount * pageSize))

	return nil
}

func Compact(ctx context.Context, db *sql.DB) error {
	log.Logger.Debugw("compacting state database")
	_, err := db.ExecContext(ctx, "VACUUM;")
	if err != nil {
		return err
	}
	log.Logger.Debugw("successfully compacted state database")
	return nil
}
