// Package store provides the persistent storage layer for the metrics.
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
	pkgmetrics "github.com/leptonai/gpud/pkg/metrics"
	pkgsqlite "github.com/leptonai/gpud/pkg/sqlite"
)

var _ pkgmetrics.Store = &sqliteStore{}

type sqliteStore struct {
	dbRW  *sql.DB
	dbRO  *sql.DB
	table string
}

func NewSQLiteStore(ctx context.Context, dbRW *sql.DB, dbRO *sql.DB, table string) (pkgmetrics.Store, error) {
	if err := CreateTable(ctx, dbRW, table); err != nil {
		return nil, err
	}
	return &sqliteStore{
		dbRW:  dbRW,
		dbRO:  dbRO,
		table: table,
	}, nil
}

func (s *sqliteStore) Record(ctx context.Context, ms ...pkgmetrics.Metric) error {
	return insert(ctx, s.dbRW, s.table, ms...)
}

func (s *sqliteStore) Read(ctx context.Context, since time.Time) (pkgmetrics.Metrics, error) {
	return read(ctx, s.dbRO, s.table, since)
}

func (s *sqliteStore) Purge(ctx context.Context, before time.Time) (int, error) {
	return purge(ctx, s.dbRW, s.table, before)
}

const (
	// DefaultTableName is the default table name for the metrics.
	DefaultTableName = "gpud_metrics"

	// ColumnUnixMilliseconds represents the Unix timestamp of the metric.
	ColumnUnixMilliseconds = "unix_milliseconds"

	// ColumnComponentName represents the name of the component this metric
	// belongs to.
	ColumnComponentName = "component_name"

	// ColumnMetricName represents the name of the metric.
	ColumnMetricName = "metric_name"

	// ColumnMetricLabel represents the label of the metric
	// such as GPU ID, etc. (as a secondary metric name).
	ColumnMetricLabel = "metric_label"

	// ColumnMetricValue represents the numeric value of the metric.
	ColumnMetricValue = "metric_value"
)

var (
	ErrEmptyTableName     = errors.New("table name is empty")
	ErrEmptyComponentName = errors.New("component name is empty")
	ErrEmptyMetricName    = errors.New("metric name is empty")
)

func CreateTable(ctx context.Context, dbRW *sql.DB, table string) error {
	if table == "" {
		return ErrEmptyTableName
	}

	_, err := dbRW.ExecContext(ctx, fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS %s (
	%s INTEGER NOT NULL,
	%s TEXT NOT NULL,
	%s TEXT NOT NULL,
	%s TEXT,
	%s REAL NOT NULL,
	PRIMARY KEY (%s, %s, %s, %s)
) WITHOUT ROWID;`,
		table,
		ColumnUnixMilliseconds, ColumnComponentName, ColumnMetricName, ColumnMetricLabel, ColumnMetricValue, // columns
		ColumnUnixMilliseconds, ColumnComponentName, ColumnMetricName, ColumnMetricLabel, // primary keys
	))
	return err
}

func insert(ctx context.Context, dbRW *sql.DB, table string, ms ...pkgmetrics.Metric) error {
	if table == "" {
		return ErrEmptyTableName
	}

	if len(ms) == 0 {
		return nil
	}

	// Validate all metrics first
	for _, m := range ms {
		if m.Component == "" {
			return ErrEmptyComponentName
		}
		if m.Name == "" {
			return ErrEmptyMetricName
		}
	}

	// Build the query with placeholders for all metrics
	query := fmt.Sprintf(
		"INSERT OR REPLACE INTO %s (%s, %s, %s, %s, %s) VALUES ",
		table,
		ColumnUnixMilliseconds,
		ColumnComponentName,
		ColumnMetricName,
		ColumnMetricLabel,
		ColumnMetricValue,
	)

	// Create proper placeholders with commas between value sets
	placeholders := make([]string, len(ms))
	for i := range placeholders {
		placeholders[i] = "(?, ?, ?, ?, ?)"
	}
	query += strings.Join(placeholders, ", ")

	args := make([]interface{}, 0, len(ms)*5)
	for _, m := range ms {
		args = append(args, m.UnixMilliseconds, m.Component, m.Name, m.Label, m.Value)
	}

	log.Logger.Infow("inserting metrics", "metrics", len(ms))
	start := time.Now()
	_, err := dbRW.ExecContext(ctx, query, args...)
	pkgsqlite.RecordInsertUpdate(time.Since(start).Seconds())

	return err
}

// read returns the metric data in the ascending order of unix seconds
// meaning the first element is the oldest event.
// It returns nil if no record is found ("database/sql.ErrNoRows").
func read(ctx context.Context, dbRO *sql.DB, table string, since time.Time) (pkgmetrics.Metrics, error) {
	if table == "" {
		return nil, ErrEmptyTableName
	}

	query := fmt.Sprintf(`
SELECT %s, %s, %s, %s, %s
FROM %s
WHERE %s >= ?
ORDER BY %s ASC;`,
		ColumnUnixMilliseconds,
		ColumnComponentName,
		ColumnMetricName,
		ColumnMetricLabel,
		ColumnMetricValue,
		table,
		ColumnUnixMilliseconds,
		ColumnUnixMilliseconds,
	)

	start := time.Now()
	defer func() {
		pkgsqlite.RecordSelect(time.Since(start).Seconds())
	}()

	queryRows, err := dbRO.QueryContext(ctx, query, since.UnixMilli())
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	defer queryRows.Close()

	rows := make(pkgmetrics.Metrics, 0)
	for queryRows.Next() {
		m := pkgmetrics.Metric{}
		var label sql.NullString
		if err := queryRows.Scan(&m.UnixMilliseconds, &m.Component, &m.Name, &label, &m.Value); err != nil {
			return nil, err
		}
		if label.Valid && label.String != "" {
			m.Label = label.String
		}
		rows = append(rows, m)
	}
	if err := queryRows.Err(); err != nil {
		return nil, err
	}
	return rows, nil
}

// purge purges the data for the corresponding component that is older
// than the given time.
func purge(ctx context.Context, dbRW *sql.DB, table string, before time.Time) (int, error) {
	if table == "" {
		return 0, ErrEmptyTableName
	}

	query := fmt.Sprintf(`
DELETE FROM %s WHERE %s < ?;`, table, ColumnUnixMilliseconds)

	start := time.Now()
	rs, err := dbRW.ExecContext(ctx, query, before.UnixMilli())
	pkgsqlite.RecordDelete(time.Since(start).Seconds())

	if err != nil {
		return 0, err
	}

	affected, err := rs.RowsAffected()
	if err != nil {
		return 0, err
	}
	return int(affected), nil
}
