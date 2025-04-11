// Package state provides the persistent storage layer for the metrics.
package state

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/mattn/go-sqlite3"

	components "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/pkg/sqlite"
)

const DefaultTableName = "components_metrics"

const (
	ColumnUnixSeconds         = "unix_seconds"
	ColumnMetricName          = "metric_name"
	ColumnMetricSecondaryName = "metric_secondary_name"
	ColumnMetricValue         = "metric_value"
)

func CreateTableMetrics(ctx context.Context, db *sql.DB, tableName string) error {
	_, err := db.ExecContext(ctx, fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS %s (
	%s INTEGER NOT NULL,
	%s TEXT NOT NULL,
	%s TEXT,
	%s REAL NOT NULL,
	PRIMARY KEY (%s, %s, %s)
) WITHOUT ROWID;`,
		tableName,
		ColumnUnixSeconds, ColumnMetricName, ColumnMetricSecondaryName, ColumnMetricValue, // columns
		ColumnUnixSeconds, ColumnMetricName, ColumnMetricSecondaryName, // primary keys
	))
	return err
}

func InsertMetric(ctx context.Context, db *sql.DB, tableName string, metric components.Metric) error {
	query := fmt.Sprintf(`
INSERT OR REPLACE INTO %s (%s, %s, %s, %s) VALUES (?, ?, ?, ?);
`,
		tableName,
		ColumnUnixSeconds,
		ColumnMetricName,
		ColumnMetricSecondaryName,
		ColumnMetricValue,
	)

	start := time.Now()
	_, err := db.ExecContext(ctx, query, metric.UnixSeconds, metric.MetricName, metric.MetricSecondaryName, metric.Value)
	sqlite.RecordInsertUpdate(time.Since(start).Seconds())

	return err
}

// Reads the last metric.
// Returns nil if no record is found ("database/sql.ErrNoRows").
func ReadLastMetric(ctx context.Context, db *sql.DB, tableName string, name string, secondaryName string) (*components.Metric, error) {
	if secondaryName == "" {
		return readLastWithAllSecondaryNames(ctx, db, tableName, name)
	}

	query := fmt.Sprintf(`
SELECT %s, %s
FROM %s
WHERE %s = ? AND %s = ?
ORDER BY %s DESC
LIMIT 1;
`,
		ColumnUnixSeconds,
		ColumnMetricValue,
		tableName,
		ColumnMetricName,
		ColumnMetricSecondaryName,
		ColumnUnixSeconds,
	)

	metric := components.Metric{
		MetricName:          name,
		MetricSecondaryName: secondaryName,
	}

	start := time.Now()
	err := db.QueryRowContext(ctx, query, name, secondaryName).Scan(&metric.UnixSeconds, &metric.Value)
	sqlite.RecordSelect(time.Since(start).Seconds())

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	return &metric, nil
}

func readLastWithAllSecondaryNames(ctx context.Context, db *sql.DB, tableName string, name string) (*components.Metric, error) {
	query := fmt.Sprintf(`
SELECT %s, %s, %s
FROM %s
WHERE %s = ?
ORDER BY %s DESC
LIMIT 1;
`,
		ColumnUnixSeconds,
		ColumnMetricSecondaryName,
		ColumnMetricValue,
		tableName,
		ColumnMetricName,
		ColumnUnixSeconds,
	)

	metric := components.Metric{
		MetricName: name,
	}

	start := time.Now()
	err := db.QueryRowContext(ctx, query, name).Scan(&metric.UnixSeconds, &metric.MetricSecondaryName, &metric.Value)
	sqlite.RecordSelect(time.Since(start).Seconds())

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	return &metric, nil
}

// Returns nil if no record is found ("database/sql.ErrNoRows").
func ReadMetricsSince(ctx context.Context, db *sql.DB, tableName string, name string, secondaryName string, since time.Time) (components.Metrics, error) {
	if secondaryName == "" {
		return readSinceWithAllSecondaryNames(ctx, db, tableName, name, since)
	}

	query := fmt.Sprintf(`
SELECT %s, %s
FROM %s
WHERE %s >= ? AND %s = ? AND %s = ?
ORDER BY %s ASC;`,
		ColumnUnixSeconds,
		ColumnMetricValue,
		tableName,
		ColumnUnixSeconds,
		ColumnMetricName,
		ColumnMetricSecondaryName,
		ColumnUnixSeconds,
	)

	start := time.Now()
	defer func() {
		sqlite.RecordSelect(time.Since(start).Seconds())
	}()

	queryRows, err := db.QueryContext(ctx, query, since.Unix(), name, secondaryName)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	defer queryRows.Close()

	rows := make(components.Metrics, 0)
	for queryRows.Next() {
		var unixSeconds int64
		metric := components.Metric{
			MetricName:          name,
			MetricSecondaryName: secondaryName,
		}
		if err := queryRows.Scan(&unixSeconds, &metric.Value); err != nil {
			return nil, err
		}
		rows = append(rows, metric)
	}
	if err := queryRows.Err(); err != nil {
		return nil, err
	}
	return rows, nil
}

func readSinceWithAllSecondaryNames(ctx context.Context, db *sql.DB, tableName string, name string, since time.Time) (components.Metrics, error) {
	query := fmt.Sprintf(`
SELECT %s, %s, %s
FROM %s
WHERE %s >= ? AND %s = ?
ORDER BY %s ASC;`,
		ColumnUnixSeconds,
		ColumnMetricSecondaryName,
		ColumnMetricValue,
		tableName,
		ColumnUnixSeconds,
		ColumnMetricName,
		ColumnUnixSeconds,
	)

	start := time.Now()
	defer func() {
		sqlite.RecordSelect(time.Since(start).Seconds())
	}()

	queryRows, err := db.QueryContext(ctx, query, since.Unix(), name)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	defer queryRows.Close()

	rows := make(components.Metrics, 0)
	for queryRows.Next() {
		metric := components.Metric{
			MetricName: name,
		}
		if err := queryRows.Scan(&metric.UnixSeconds, &metric.MetricSecondaryName, &metric.Value); err != nil {
			return nil, err
		}
		rows = append(rows, metric)
	}
	if err := queryRows.Err(); err != nil {
		return nil, err
	}
	return rows, nil
}

// Computes the average of the last metrics.
// If the since is zero, all metrics are used.
// Returns zero if no record is found ("database/sql.ErrNoRows").
func AvgSince(ctx context.Context, db *sql.DB, tableName string, name string, secondaryName string, since time.Time) (float64, error) {
	var sinceUnix int64
	if !since.IsZero() {
		sinceUnix = since.Unix()
	}

	query := fmt.Sprintf(`
SELECT AVG(%s)
FROM %s
WHERE %s = ? AND %s >= ?`,
		ColumnMetricValue,
		tableName,
		ColumnMetricName,
		ColumnUnixSeconds,
	)
	args := []any{name, sinceUnix}
	if secondaryName != "" {
		query += fmt.Sprintf(` AND %s = ?`, ColumnMetricSecondaryName)
		args = append(args, secondaryName)
	}

	start := time.Now()
	var avg sql.NullFloat64
	err := db.QueryRowContext(ctx, query, args...).Scan(&avg)
	sqlite.RecordSelect(time.Since(start).Seconds())

	if err != nil {
		if err == sql.ErrNoRows {
			return 0.0, nil
		}
		return 0.0, err
	}

	if !avg.Valid {
		return 0.0, nil
	}

	return avg.Float64, nil
}

func PurgeMetrics(ctx context.Context, db *sql.DB, tableName string, before time.Time) (int, error) {
	query := fmt.Sprintf(`
DELETE FROM %s WHERE %s < ?;`, tableName, ColumnUnixSeconds)

	start := time.Now()
	rs, err := db.ExecContext(ctx, query, before.Unix())
	sqlite.RecordDelete(time.Since(start).Seconds())

	if err != nil {
		return 0, err
	}
	affected, err := rs.RowsAffected()
	if err != nil {
		return 0, err
	}
	return int(affected), nil
}
