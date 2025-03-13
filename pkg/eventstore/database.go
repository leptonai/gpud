package eventstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/common"
	"github.com/leptonai/gpud/pkg/log"
	"github.com/leptonai/gpud/pkg/sqlite"

	_ "github.com/mattn/go-sqlite3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	schemaVersion = "v0_4_0"
)

const (
	// columnTimestamp represents the event timestamp in unix seconds.
	columnTimestamp = "timestamp"

	// columnName represents the event name
	// e.g., "memory_oom", "memory_oom_kill_constraint", "memory_oom_cgroup", "memory_edac_correctable_errors".
	columnName = "name"

	// columnType represents event type
	// e.g., "Unknown", "Info", "Warning", "Critical", "Fatal".
	columnType = "type"

	// columnMessage represents event message
	// e.g., "VFS file-max limit reached"
	columnMessage = "message"

	// columnExtraInfo represents event extra info
	// e.g.,
	// data source: "dmesg", "nvml", "nvidia-smi".
	// event target: "gpu_id", "gpu_uuid".
	// log detail: "oom_reaper: reaped process 345646 (vector), now anon-rss:0kB, file-rss:0kB, shmem-rss:0".
	columnExtraInfo = "extra_info"

	// columnSuggestedActions represents event suggested actions
	// e.g., "reboot"
	columnSuggestedActions = "suggested_actions"
)

var (
	_ Store  = &database{}
	_ Bucket = &table{}
)

type database struct {
	dbRW      *sql.DB
	dbRO      *sql.DB
	retention time.Duration
}

type table struct {
	rootCtx       context.Context
	rootCancel    context.CancelFunc
	retention     time.Duration
	checkInterval time.Duration

	table string
	dbRW  *sql.DB
	dbRO  *sql.DB
}

func New(dbRW *sql.DB, dbRO *sql.DB, retention time.Duration) (Store, error) {
	return &database{
		dbRW:      dbRW,
		dbRO:      dbRO,
		retention: retention,
	}, nil
}

func (d *database) Bucket(name string) (Bucket, error) {
	// actual check interval should be lower than the retention period
	// in case of GPUd restarts
	checkInterval := d.retention / 5
	if checkInterval < time.Second {
		checkInterval = time.Second
	}
	return newTable(d.dbRW, d.dbRO, name, d.retention, checkInterval)
}

func newTable(dbRW *sql.DB, dbRO *sql.DB, name string, retention time.Duration, checkInterval time.Duration) (*table, error) {
	tableName := defaultTableName(name)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	err := createTable(ctx, dbRW, tableName)
	cancel()
	if err != nil {
		return nil, err
	}

	rootCtx, rootCancel := context.WithCancel(context.Background())
	t := &table{
		rootCtx:       rootCtx,
		rootCancel:    rootCancel,
		table:         tableName,
		dbRW:          dbRW,
		dbRO:          dbRO,
		retention:     retention,
		checkInterval: checkInterval,
	}
	if retention > time.Second {
		go t.runPurge()
	}
	return t, nil
}

// defaultTableName creates the default table name for the component.
// The table name is in the format of "components_{component_name}_events_v0_4_0".
// Suffix with the version, in case we change the table schema.
func defaultTableName(componentName string) string {
	c := strings.ReplaceAll(componentName, " ", "_")
	c = strings.ReplaceAll(c, "-", "_")
	c = strings.ReplaceAll(c, "__", "_")
	c = strings.ToLower(c)
	tableName := fmt.Sprintf("components_%s_events_%s", c, schemaVersion)
	return tableName
}

func (t *table) Name() string {
	return t.table
}

func (t *table) runPurge() {
	log.Logger.Infow("start purging", "table", t.table, "retention", t.retention, "checkInterval", t.checkInterval)
	for {
		select {
		case <-t.rootCtx.Done():
			return
		case <-time.After(t.checkInterval):
		}

		now := time.Now().UTC()
		purged, err := t.Purge(t.rootCtx, now.Add(-t.retention).Unix())
		if err != nil {
			log.Logger.Errorw("failed to purge data", "table", t.table, "retention", t.retention, "error", err)
		} else {
			log.Logger.Infow("purged data", "table", t.table, "retention", t.retention, "purged", purged)
		}
	}
}

func (t *table) Close() {
	log.Logger.Infow("closing the store", "table", t.table)
	if t.rootCancel != nil {
		t.rootCancel()
	}
}

func (t *table) Insert(ctx context.Context, ev components.Event) error {
	return insertEvent(ctx, t.dbRW, t.table, ev)
}

// Returns nil if the event is not found.
func (t *table) Find(ctx context.Context, ev components.Event) (*components.Event, error) {
	return findEvent(ctx, t.dbRO, t.table, ev)
}

// Get queries the event in the descending order of timestamp (latest event first).
func (t *table) Get(ctx context.Context, since time.Time) ([]components.Event, error) {
	return getEvents(ctx, t.dbRO, t.table, since)
}

// Latest queries the latest event, returns nil if no event found.
func (t *table) Latest(ctx context.Context) (*components.Event, error) {
	return lastEvent(ctx, t.dbRO, t.table)
}

func (t *table) Purge(ctx context.Context, beforeTimestamp int64) (int, error) {
	return purgeEvents(ctx, t.dbRW, t.table, beforeTimestamp)
}

func createTable(ctx context.Context, db *sql.DB, tableName string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}

	// create table
	_, err = tx.ExecContext(ctx, fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
	%s INTEGER NOT NULL,
	%s TEXT NOT NULL,
	%s TEXT NOT NULL,
	%s TEXT,
	%s TEXT,
	%s TEXT
);`, tableName,
		columnTimestamp,
		columnName,
		columnType,
		columnMessage,
		columnExtraInfo,
		columnSuggestedActions,
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
		tableName, columnName, tableName, columnName))
	if err != nil {
		_ = tx.Rollback()
		return err
	}

	_, err = tx.ExecContext(ctx, fmt.Sprintf(`CREATE INDEX IF NOT EXISTS idx_%s_%s ON %s(%s);`,
		tableName, columnType, tableName, columnType))
	if err != nil {
		_ = tx.Rollback()
		return err
	}

	return tx.Commit()
}

func insertEvent(ctx context.Context, db *sql.DB, tableName string, ev components.Event) error {
	start := time.Now()
	var extraInfoJSON, suggestedActionsJSON []byte
	var err error
	if ev.ExtraInfo != nil {
		extraInfoJSON, err = json.Marshal(ev.ExtraInfo)
		if err != nil {
			return fmt.Errorf("failed to marshal extra info: %w", err)
		}
	}
	if ev.SuggestedActions != nil {
		suggestedActionsJSON, err = json.Marshal(ev.SuggestedActions)
	}
	if err != nil {
		return fmt.Errorf("failed to marshal suggested actions: %w", err)
	}

	_, err = db.ExecContext(ctx, fmt.Sprintf("INSERT INTO %s (%s, %s, %s, %s, %s, %s) VALUES (?, ?, ?, NULLIF(?, ''), NULLIF(?, ''), NULLIF(?, ''))",
		tableName,
		columnTimestamp,
		columnName,
		columnType,
		columnMessage,
		columnExtraInfo,
		columnSuggestedActions,
	),
		ev.Time.Unix(),
		ev.Name,
		ev.Type,
		ev.Message,
		string(extraInfoJSON),
		string(suggestedActionsJSON),
	)
	sqlite.RecordInsertUpdate(time.Since(start).Seconds())

	return err
}

func findEvent(ctx context.Context, db *sql.DB, tableName string, ev components.Event) (*components.Event, error) {
	selectStatement := fmt.Sprintf(`
SELECT %s, %s, %s, %s, %s, %s FROM %s WHERE %s = ? AND %s = ? AND %s = ?`,
		columnTimestamp,
		columnName,
		columnType,
		columnMessage,
		columnExtraInfo,
		columnSuggestedActions,
		tableName,
		columnTimestamp,
		columnName,
		columnType,
	)
	if ev.Message != "" {
		selectStatement += fmt.Sprintf(" AND %s = ?", columnMessage)
	}
	if ev.SuggestedActions != nil {
		selectStatement += fmt.Sprintf(" AND %s = ?", columnSuggestedActions)
	}

	params := []any{ev.Time.Unix(), ev.Name, ev.Type}
	if ev.Message != "" {
		params = append(params, ev.Message)
	}
	if ev.SuggestedActions != nil {
		suggestedActionsJSON, err := json.Marshal(ev.SuggestedActions)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal suggested actions: %w", err)
		}
		params = append(params, string(suggestedActionsJSON))
	}

	start := time.Now()
	rows, err := db.QueryContext(ctx, selectStatement, params...)
	sqlite.RecordSelect(time.Since(start).Seconds())

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		event, err := scanRows(rows)
		if err != nil {
			return nil, err
		}
		if compareEvent(event, ev) {
			return &event, nil
		}
	}
	return nil, nil
}

// Returns the event in the descending order of timestamp (latest event first).
func getEvents(ctx context.Context, db *sql.DB, tableName string, since time.Time) ([]components.Event, error) {
	query := fmt.Sprintf(`SELECT %s, %s, %s, %s, %s, %s
FROM %s
WHERE %s > ?
ORDER BY %s DESC`,
		columnTimestamp, columnName, columnType, columnMessage, columnExtraInfo, columnSuggestedActions,
		tableName,
		columnTimestamp,
		columnTimestamp,
	)
	params := []any{since.UTC().Unix()}

	start := time.Now()
	rows, err := db.QueryContext(ctx, query, params...)
	sqlite.RecordSelect(time.Since(start).Seconds())

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()

	var events []components.Event
	for rows.Next() {
		event, err := scanRows(rows)
		if err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	if len(events) == 0 {
		return nil, nil
	}
	return events, nil
}

func lastEvent(ctx context.Context, db *sql.DB, tableName string) (*components.Event, error) {
	query := fmt.Sprintf(`SELECT %s, %s, %s, %s, %s, %s FROM %s ORDER BY %s DESC LIMIT 1`,
		columnTimestamp, columnName, columnType, columnMessage, columnExtraInfo, columnSuggestedActions, tableName, columnTimestamp)

	start := time.Now()
	row := db.QueryRowContext(ctx, query)
	sqlite.RecordSelect(time.Since(start).Seconds())

	foundEvent, err := scanRow(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	return &foundEvent, nil
}

func scanRow(row *sql.Row) (components.Event, error) {
	var event components.Event
	var timestamp int64
	var msg sql.NullString
	var extraInfo sql.NullString
	var suggestedActions sql.NullString
	err := row.Scan(
		&timestamp,
		&event.Name,
		&event.Type,
		&msg,
		&extraInfo,
		&suggestedActions,
	)
	if err != nil {
		return event, err
	}

	event.Time = metav1.Time{Time: time.Unix(timestamp, 0)}
	if msg.Valid {
		event.Message = msg.String
	}
	if extraInfo.Valid && len(extraInfo.String) > 0 && extraInfo.String != "null" {
		var extraInfoMap map[string]string
		if err := json.Unmarshal([]byte(extraInfo.String), &extraInfoMap); err != nil {
			return event, fmt.Errorf("failed to unmarshal extra info: %w", err)
		}
		event.ExtraInfo = extraInfoMap
	}
	if suggestedActions.Valid && len(suggestedActions.String) > 0 && suggestedActions.String != "null" {
		var suggestedActionsObj common.SuggestedActions
		if err := json.Unmarshal([]byte(suggestedActions.String), &suggestedActionsObj); err != nil {
			return event, fmt.Errorf("failed to unmarshal suggested actions: %w", err)
		}
		event.SuggestedActions = &suggestedActionsObj
	}
	return event, nil
}

func scanRows(rows *sql.Rows) (components.Event, error) {
	var event components.Event
	var timestamp int64
	var msg sql.NullString
	var extraInfo sql.NullString
	var suggestedActions sql.NullString
	err := rows.Scan(
		&timestamp,
		&event.Name,
		&event.Type,
		&msg,
		&extraInfo,
		&suggestedActions,
	)
	if err != nil {
		return event, err
	}

	event.Time = metav1.Time{Time: time.Unix(timestamp, 0)}
	if msg.Valid {
		event.Message = msg.String
	}
	if extraInfo.Valid {
		var extraInfoMap map[string]string
		if err := json.Unmarshal([]byte(extraInfo.String), &extraInfoMap); err != nil {
			return event, fmt.Errorf("failed to unmarshal extra info: %w", err)
		}
		event.ExtraInfo = extraInfoMap
	}
	if suggestedActions.Valid && suggestedActions.String != "" {
		var suggestedActionsObj common.SuggestedActions
		if err := json.Unmarshal([]byte(suggestedActions.String), &suggestedActionsObj); err != nil {
			return event, fmt.Errorf("failed to unmarshal suggested actions: %w", err)
		}
		event.SuggestedActions = &suggestedActionsObj
	}
	return event, nil
}

func purgeEvents(ctx context.Context, db *sql.DB, tableName string, beforeTimestamp int64) (int, error) {
	deleteStatement := fmt.Sprintf(`DELETE FROM %s WHERE %s < ?`, tableName, columnTimestamp)

	start := time.Now()
	rs, err := db.ExecContext(ctx, deleteStatement, beforeTimestamp)
	if err != nil {
		return 0, err
	}
	sqlite.RecordDelete(time.Since(start).Seconds())

	affected, err := rs.RowsAffected()
	if err != nil {
		return 0, err
	}
	return int(affected), nil
}

func compareEvent(eventA, eventB components.Event) bool {
	if len(eventA.ExtraInfo) != len(eventB.ExtraInfo) {
		return false
	}
	for key, value := range eventA.ExtraInfo {
		if val, ok := eventB.ExtraInfo[key]; !ok || val != value {
			return false
		}
	}
	return true
}
