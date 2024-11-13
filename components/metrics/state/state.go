// Package state provides the persistent storage layer for the metrics.
package state

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"text/template"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

type Metric struct {
	UnixSeconds         int64   `json:"unix_seconds"`
	MetricName          string  `json:"metric_name"`
	MetricSecondaryName string  `json:"metric_secondary_name,omitempty"`
	Value               float64 `json:"value"`
}

type Metrics []Metric

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

func InsertMetric(ctx context.Context, db *sql.DB, tableName string, metric Metric) error {
	query := fmt.Sprintf(`
INSERT OR REPLACE INTO %s (%s, %s, %s, %s) VALUES (?, ?, ?, ?);
`,
		tableName,
		ColumnUnixSeconds,
		ColumnMetricName,
		ColumnMetricSecondaryName,
		ColumnMetricValue,
	)
	_, err := db.ExecContext(ctx, query, metric.UnixSeconds, metric.MetricName, metric.MetricSecondaryName, metric.Value)
	return err
}

// Reads the last metric.
// Returns nil if no record is found ("database/sql.ErrNoRows").
func ReadLastMetric(ctx context.Context, db *sql.DB, tableName string, name string, secondaryName string) (*Metric, error) {
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

	metric := Metric{
		MetricName:          name,
		MetricSecondaryName: secondaryName,
	}
	err := db.QueryRowContext(ctx, query, name, secondaryName).Scan(&metric.UnixSeconds, &metric.Value)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	return &metric, nil
}

func readLastWithAllSecondaryNames(ctx context.Context, db *sql.DB, tableName string, name string) (*Metric, error) {
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

	metric := Metric{
		MetricName: name,
	}
	err := db.QueryRowContext(ctx, query, name).Scan(&metric.UnixSeconds, &metric.MetricSecondaryName, &metric.Value)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	return &metric, nil
}

// Returns nil if no record is found ("database/sql.ErrNoRows").
func ReadMetricsSince(ctx context.Context, db *sql.DB, tableName string, name string, secondaryName string, since time.Time) (Metrics, error) {
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

	queryRows, err := db.QueryContext(ctx, query, since.Unix(), name, secondaryName)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	defer queryRows.Close()

	rows := make(Metrics, 0)
	for queryRows.Next() {
		var unixSeconds int64
		metric := Metric{
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

func readSinceWithAllSecondaryNames(ctx context.Context, db *sql.DB, tableName string, name string, since time.Time) (Metrics, error) {
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

	queryRows, err := db.QueryContext(ctx, query, since.Unix(), name)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	defer queryRows.Close()

	rows := make(Metrics, 0)
	for queryRows.Next() {
		metric := Metric{
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
	query := fmt.Sprintf(`
SELECT AVG(%s)
FROM %s
WHERE %s = ? AND %s = ? AND %s >= ?;`,
		ColumnMetricValue,
		tableName,
		ColumnMetricName,
		ColumnMetricSecondaryName,
		ColumnUnixSeconds,
	)

	var sinceUnix int64
	if !since.IsZero() {
		sinceUnix = since.Unix()
	}

	var avg sql.NullFloat64
	err := db.QueryRowContext(ctx, query, name, secondaryName, sinceUnix).Scan(&avg)
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

const emaQueryTempl = `
WITH ranked_metrics AS (
	SELECT {{.ColumnUnixSeconds}}, {{.ColumnMetricValue}},
		ROW_NUMBER() OVER (ORDER BY {{.ColumnUnixSeconds}} ASC) AS row_num
	FROM {{.TableName}}
	WHERE {{.ColumnMetricName}} = ? AND {{.ColumnMetricSecondaryName}} = ? AND {{.ColumnUnixSeconds}} >= ?
	ORDER BY {{.ColumnUnixSeconds}} ASC
),
ema_calc AS (
	SELECT {{.ColumnUnixSeconds}},
		{{.ColumnMetricValue}},
		row_num,
		CASE
			WHEN row_num = 1 THEN {{.ColumnMetricValue}}
			ELSE (? * {{.ColumnMetricValue}}) + ((1 - ?) * LAG({{.ColumnMetricValue}}, 1) OVER (ORDER BY {{.ColumnUnixSeconds}}))
		END AS ema
	FROM ranked_metrics
)
SELECT ema
FROM ema_calc
ORDER BY {{.ColumnUnixSeconds}} DESC
LIMIT 1;
`

type emaQueryTemplInput struct {
	TableName                 string
	ColumnUnixSeconds         string
	ColumnMetricValue         string
	ColumnMetricName          string
	ColumnMetricSecondaryName string
}

// EMASince calculates the Exponential Moving Average since a given time
func EMASince(ctx context.Context, db *sql.DB, tableName string, name string, secondaryName string, period time.Duration, since time.Time) (float64, error) {
	tmpl, err := template.New("query").Parse(emaQueryTempl)
	if err != nil {
		return 0.0, fmt.Errorf("failed to parse query template: %w", err)
	}

	data := emaQueryTemplInput{
		TableName:                 tableName,
		ColumnUnixSeconds:         ColumnUnixSeconds,
		ColumnMetricValue:         ColumnMetricValue,
		ColumnMetricName:          ColumnMetricName,
		ColumnMetricSecondaryName: ColumnMetricSecondaryName,
	}
	var query bytes.Buffer
	if err := tmpl.Execute(&query, data); err != nil {
		return 0.0, fmt.Errorf("failed to execute query template: %w", err)
	}

	// calculate alpha (smoothing factor)
	alpha := 2.0 / (period.Minutes() + 1)

	var ema sql.NullFloat64
	err = db.QueryRowContext(ctx, query.String(), name, secondaryName, since.Unix(), alpha, alpha).Scan(&ema)
	if err != nil {
		if err == sql.ErrNoRows {
			return 0.0, nil
		}
		return 0.0, err
	}

	if !ema.Valid {
		return 0.0, nil
	}

	return ema.Float64, nil
}

func PurgeMetrics(ctx context.Context, db *sql.DB, tableName string, before time.Time) (int, error) {
	query := fmt.Sprintf(`
DELETE FROM %s WHERE %s < ?;`, tableName, ColumnUnixSeconds)
	rs, err := db.ExecContext(ctx, query, before.Unix())
	if err != nil {
		return 0, err
	}
	affected, err := rs.RowsAffected()
	if err != nil {
		return 0, err
	}
	return int(affected), nil
}
