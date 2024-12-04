// Package clockeventsstate provides the persistent storage layer for the nvidia query results.
package clockeventsstate

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"time"

	"github.com/leptonai/gpud/log"

	_ "github.com/mattn/go-sqlite3"
)

const TableNameClockEvents = "components_accelerator_nvidia_query_clock_events"

const (
	// unix timestamp in seconds when the event was observed
	ColumnUnixSeconds = "unix_seconds"

	// either "nvml" or "dmesg"
	ColumnDataSource = "data_source"

	// either "xid" or "sxid"
	ColumnEventType = "event_type"

	// gpu uuid
	ColumnGPUUUID = "gpu_uuid"

	// reasons for clock events
	ColumnReasons = "reasons"
)

const DefaultRetentionPeriod = 3 * time.Hour

type Event struct {
	UnixSeconds int64
	DataSource  string
	EventType   string
	GPUUUID     string
	Reasons     []string
}

func CreateTable(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS %s (
	%s INTEGER NOT NULL,
	%s TEXT NOT NULL,
	%s TEXT NOT NULL,
	%s TEXT NOT NULL,
	%s TEXT NOT NULL
);`, TableNameClockEvents,
		ColumnUnixSeconds,
		ColumnDataSource,
		ColumnEventType,
		ColumnGPUUUID,
		ColumnReasons,
	))
	return err
}

func InsertEvent(ctx context.Context, db *sql.DB, event Event) error {
	log.Logger.Debugw("inserting event", "dataSource", event.DataSource, "eventType", event.EventType, "uuid", event.GPUUUID, "reasons", event.Reasons)

	insertStatement := fmt.Sprintf(`
INSERT OR REPLACE INTO %s (%s, %s, %s, %s, %s) VALUES (?, ?, ?, ?, NULLIF(?, ''));
`,
		TableNameClockEvents,
		ColumnUnixSeconds,
		ColumnDataSource,
		ColumnEventType,
		ColumnGPUUUID,
		ColumnReasons,
	)

	reasonsBytes, err := json.Marshal(event.Reasons)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(
		ctx,
		insertStatement,
		event.UnixSeconds,
		event.DataSource,
		event.EventType,
		event.GPUUUID,
		string(reasonsBytes),
	)
	return err
}

func FindEvent(ctx context.Context, db *sql.DB, event Event) (bool, error) {
	selectStatement := fmt.Sprintf(`
SELECT %s, %s, %s, %s, %s FROM %s WHERE %s = ? AND %s = ? AND %s = ? AND %s = ?;
`,
		ColumnUnixSeconds,
		ColumnDataSource,
		ColumnEventType,
		ColumnGPUUUID,
		ColumnReasons,
		TableNameClockEvents,
		ColumnUnixSeconds,
		ColumnDataSource,
		ColumnEventType,
		ColumnGPUUUID,
	)

	var foundEvent Event
	var reasonsRaw string
	if err := db.QueryRowContext(
		ctx,
		selectStatement,
		event.UnixSeconds,
		event.DataSource,
		event.EventType,
		event.GPUUUID,
	).Scan(
		&foundEvent.UnixSeconds,
		&foundEvent.DataSource,
		&foundEvent.EventType,
		&foundEvent.GPUUUID,
		&reasonsRaw,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	if err := json.Unmarshal([]byte(reasonsRaw), &foundEvent.Reasons); err != nil {
		return false, err
	}

	sort.Strings(foundEvent.Reasons)
	sort.Strings(event.Reasons)

	// event at the same time but with different details
	if len(foundEvent.Reasons) > 0 && !reflect.DeepEqual(foundEvent.Reasons, event.Reasons) {
		return false, nil
	}

	// found event
	// e.g., same messages in dmesg
	return true, nil
}

// Returns nil if no event is found.
func ReadEvents(ctx context.Context, db *sql.DB, opts ...OpOption) ([]Event, error) {
	selectStatement, args, err := createSelectStatementAndArgs(opts...)
	if err != nil {
		return nil, err
	}

	rows, err := db.QueryContext(ctx, selectStatement, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	op := &Op{}
	if err := op.applyOpts(opts); err != nil {
		return nil, err
	}

	// data sources can be multiple (e.g., nvml and nvidia-smi)
	// here, we prioritize nvml over nvidia-smi
	// for the same unix second, only the one from nvml will be kept
	unixToEventTypeToUUIDToDataSource := make(map[int64]map[string]map[string]string, 0)

	events := []Event{}
	for rows.Next() {
		var event Event
		var reasonsRaw string
		if err := rows.Scan(
			&event.UnixSeconds,
			&event.DataSource,
			&event.EventType,
			&event.GPUUUID,
			&reasonsRaw,
		); err != nil {
			return nil, err
		}

		_, ok := unixToEventTypeToUUIDToDataSource[event.UnixSeconds]
		if !ok {
			unixToEventTypeToUUIDToDataSource[event.UnixSeconds] = make(map[string]map[string]string, 0)
		}
		_, ok = unixToEventTypeToUUIDToDataSource[event.UnixSeconds][event.EventType]
		if !ok {
			unixToEventTypeToUUIDToDataSource[event.UnixSeconds][event.EventType] = make(map[string]string, 0)
		}

		prevDataSource, ok := unixToEventTypeToUUIDToDataSource[event.UnixSeconds][event.EventType][event.GPUUUID]
		currDataSource := event.DataSource

		toInclude := true
		if !ok {
			// this GPU had NO PREVIOUS event at this unix second and event type ("nvml" or "nvidia-smi")
			unixToEventTypeToUUIDToDataSource[event.UnixSeconds][event.EventType][event.GPUUUID] = currDataSource
		} else {
			// this GPU HAD A PREVIOUS event at this unix second and event type BUT with different data source

			// this works because we prioritize "nvml" over "nvidia-smi" in the SQL select statement
			// "ORDER BY data_source DESC" where "nvml" comes before "nvidia-smi"
			if op.dedupDataSource && prevDataSource == "nvml" {
				// already had an event with the prioritized "nvml" data source
				// thus we DO NOT need to return another event with the other data source ("nvidia-smi")
				toInclude = false
			}
		}

		if !toInclude {
			continue
		}

		if err := json.Unmarshal([]byte(reasonsRaw), &event.Reasons); err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(events) == 0 {
		return nil, nil
	}

	return events, nil
}

func createSelectStatementAndArgs(opts ...OpOption) (string, []any, error) {
	op := &Op{}
	if err := op.applyOpts(opts); err != nil {
		return "", nil, err
	}

	selectStatement := fmt.Sprintf(`SELECT %s, %s, %s, %s, %s
FROM %s`,
		ColumnUnixSeconds,
		ColumnDataSource,
		ColumnEventType,
		ColumnGPUUUID,
		ColumnReasons,
		TableNameClockEvents,
	)

	args := []any{}

	if op.sinceUnixSeconds > 0 {
		selectStatement += "\nWHERE "
		selectStatement += fmt.Sprintf("%s >= ?", ColumnUnixSeconds)
		args = append(args, op.sinceUnixSeconds)
	}

	// sort by unix seconds and data source
	// data source is sorted in reverse order so that "nvml" returns before "nvidia-smi"
	// for the same unix second and event type
	selectStatement += "\nORDER BY " + ColumnUnixSeconds
	if op.sortUnixSecondsAscOrder {
		selectStatement += " ASC"
	} else {
		selectStatement += " DESC"
	}
	selectStatement += ", " + ColumnDataSource + " DESC"

	if op.limit > 0 {
		selectStatement += fmt.Sprintf("\nLIMIT %d", op.limit)
	}

	if len(args) == 0 {
		return selectStatement, nil, nil
	}
	return selectStatement, args, nil
}

func Purge(ctx context.Context, db *sql.DB, opts ...OpOption) (int, error) {
	log.Logger.Debugw("purging nvidia clock events")
	deleteStatement, args, err := createDeleteStatementAndArgs(opts...)
	if err != nil {
		return 0, err
	}
	rs, err := db.ExecContext(ctx, deleteStatement, args...)
	if err != nil {
		return 0, err
	}
	affected, err := rs.RowsAffected()
	if err != nil {
		return 0, err
	}
	return int(affected), nil
}

// ignores order by and limit
func createDeleteStatementAndArgs(opts ...OpOption) (string, []any, error) {
	op := &Op{}
	if err := op.applyOpts(opts); err != nil {
		return "", nil, err
	}

	deleteStatement := fmt.Sprintf(`DELETE FROM %s`,
		TableNameClockEvents,
	)

	args := []any{}

	if op.beforeUnixSeconds > 0 {
		deleteStatement += " WHERE "
		deleteStatement += fmt.Sprintf("%s < ?", ColumnUnixSeconds)
		args = append(args, op.beforeUnixSeconds)
	}

	if len(args) == 0 {
		return deleteStatement, nil, nil
	}
	return deleteStatement, args, nil
}
