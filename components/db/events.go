package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/components/common"
	"github.com/leptonai/gpud/log"
	"github.com/leptonai/gpud/pkg/sqlite"

	_ "github.com/mattn/go-sqlite3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const schemaVersion = "v0_4_0"

// Creates the default table name for the component.
// The table name is in the format of "components_{component_name}_events_v0_4_0".
// Suffix with the version, in case we change the table schema.
func CreateDefaultTableName(componentName string) string {
	c := strings.ReplaceAll(componentName, " ", "_")
	c = strings.ReplaceAll(c, "-", "_")
	c = strings.ReplaceAll(c, "__", "_")
	c = strings.ToLower(c)
	tableName := fmt.Sprintf("components_%s_events_%s", c, schemaVersion)
	return tableName
}

const (
	// Event timestamp in unix seconds.
	ColumnTimestamp = "timestamp"

	// event name
	// e.g., "memory_oom", "memory_oom_kill_constraint", "memory_oom_cgroup", "memory_edac_correctable_errors".
	ColumnName = "name"

	// event type
	// e.g., "Unknown", "Info", "Warning", "Critical", "Fatal".
	ColumnType = "type"

	// event message
	// e.g., "VFS file-max limit reached"
	ColumnMessage = "message"

	// event extra info
	// e.g.,
	// data source: "dmesg", "nvml", "nvidia-smi".
	// event target: "gpu_id", "gpu_uuid".
	// log detail: "oom_reaper: reaped process 345646 (vector), now anon-rss:0kB, file-rss:0kB, shmem-rss:0".
	ColumnExtraInfo = "extra_info"

	// event suggested actions
	// e.g., "reboot"
	ColumnSuggestedActions = "suggested_actions"
)

type storeImpl struct {
	rootCtx    context.Context
	rootCancel context.CancelFunc

	table     string
	dbRW      *sql.DB
	dbRO      *sql.DB
	retention time.Duration
}

var (
	ErrNoDBRWSet = errors.New("no writable db set")
	ErrNoDBROSet = errors.New("no read-only db set")
)

type Store interface {
	Insert(ctx context.Context, ev components.Event) error
	Find(ctx context.Context, ev components.Event) (*components.Event, error)

	// Returns the event in the descending order of timestamp (latest event first).
	Get(ctx context.Context, since time.Time) ([]components.Event, error)

	// Returns the latest event.
	// Returns nil if no event found.
	Latest(ctx context.Context) (*components.Event, error)

	Purge(ctx context.Context, beforeTimestamp int64) (int, error)
	Close()
}

var _ Store = (*storeImpl)(nil)

// Creates a new DB instance with the table created.
// Requires write-only and read-only instances for minimize conflicting writes/reads.
// ref. https://github.com/mattn/go-sqlite3/issues/1179#issuecomment-1638083995
func NewStore(dbRW *sql.DB, dbRO *sql.DB, tableName string, retention time.Duration) (Store, error) {
	if dbRW == nil {
		return nil, ErrNoDBRWSet
	}
	if dbRO == nil {
		return nil, ErrNoDBROSet
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	err := createTable(ctx, dbRW, tableName)
	cancel()
	if err != nil {
		return nil, err
	}

	rootCtx, rootCancel := context.WithCancel(context.Background())
	s := &storeImpl{
		rootCtx:    rootCtx,
		rootCancel: rootCancel,
		table:      tableName,
		dbRW:       dbRW,
		dbRO:       dbRO,
		retention:  retention,
	}
	go s.runPurge()

	return s, nil
}

func (s *storeImpl) runPurge() {
	if s.retention < time.Second {
		return
	}

	// actual check interval should be lower than the retention period
	// in case of GPUd restarts
	checkInterval := s.retention / 5
	if checkInterval < time.Second {
		checkInterval = time.Second
	}

	log.Logger.Infow("start purging", "table", s.table, "retention", s.retention)
	for {
		select {
		case <-s.rootCtx.Done():
			return
		case <-time.After(checkInterval):
		}

		now := time.Now().UTC()
		purged, err := s.Purge(s.rootCtx, now.Add(-s.retention).Unix())
		if err != nil {
			log.Logger.Errorw("failed to purge data", "table", s.table, "retention", s.retention, "error", err)
		} else {
			log.Logger.Infow("purged data", "table", s.table, "retention", s.retention, "purged", purged)
		}
	}
}

func (s *storeImpl) Close() {
	log.Logger.Infow("closing the store", "table", s.table)
	if s.rootCancel != nil {
		s.rootCancel()
	}
}

func (s *storeImpl) Insert(ctx context.Context, ev components.Event) error {
	return insertEvent(ctx, s.dbRW, s.table, ev)
}

func (s *storeImpl) Find(ctx context.Context, ev components.Event) (*components.Event, error) {
	return findEvent(ctx, s.dbRO, s.table, ev)
}

// Returns the event in the descending order of timestamp (latest event first).
func (s *storeImpl) Get(ctx context.Context, since time.Time) ([]components.Event, error) {
	return getEvents(ctx, s.dbRO, s.table, since)
}

// Returns the latest event.
// Returns nil if no event found.
func (s *storeImpl) Latest(ctx context.Context) (*components.Event, error) {
	return lastEvent(ctx, s.dbRO, s.table)
}

func (s *storeImpl) Purge(ctx context.Context, beforeTimestamp int64) (int, error) {
	return purgeEvents(ctx, s.dbRW, s.table, beforeTimestamp)
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
		ColumnTimestamp,
		ColumnName,
		ColumnType,
		ColumnMessage,
		ColumnExtraInfo,
		ColumnSuggestedActions,
	))
	if err != nil {
		_ = tx.Rollback()
		return err
	}

	_, err = tx.ExecContext(ctx, fmt.Sprintf(`CREATE INDEX IF NOT EXISTS idx_%s_%s ON %s(%s);`,
		tableName, ColumnTimestamp, tableName, ColumnTimestamp))
	if err != nil {
		_ = tx.Rollback()
		return err
	}

	_, err = tx.ExecContext(ctx, fmt.Sprintf(`CREATE INDEX IF NOT EXISTS idx_%s_%s ON %s(%s);`,
		tableName, ColumnName, tableName, ColumnName))
	if err != nil {
		_ = tx.Rollback()
		return err
	}

	_, err = tx.ExecContext(ctx, fmt.Sprintf(`CREATE INDEX IF NOT EXISTS idx_%s_%s ON %s(%s);`,
		tableName, ColumnType, tableName, ColumnType))
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
		ColumnTimestamp,
		ColumnName,
		ColumnType,
		ColumnMessage,
		ColumnExtraInfo,
		ColumnSuggestedActions,
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
		ColumnTimestamp,
		ColumnName,
		ColumnType,
		ColumnMessage,
		ColumnExtraInfo,
		ColumnSuggestedActions,
		tableName,
		ColumnTimestamp,
		ColumnName,
		ColumnType,
	)
	if ev.Message != "" {
		selectStatement += fmt.Sprintf(" AND %s = ?", ColumnMessage)
	}
	if ev.SuggestedActions != nil {
		selectStatement += fmt.Sprintf(" AND %s = ?", ColumnSuggestedActions)
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
		ColumnTimestamp, ColumnName, ColumnType, ColumnMessage, ColumnExtraInfo, ColumnSuggestedActions,
		tableName,
		ColumnTimestamp,
		ColumnTimestamp,
	)
	params := []any{since.UTC().Unix()}

	start := time.Now()
	rows, err := db.QueryContext(ctx, query, params...)
	sqlite.RecordSelect(time.Since(start).Seconds())

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()

	events := []components.Event{}
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
		ColumnTimestamp, ColumnName, ColumnType, ColumnMessage, ColumnExtraInfo, ColumnSuggestedActions, tableName, ColumnTimestamp)

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
	deleteStatement := fmt.Sprintf(`DELETE FROM %s WHERE %s < ?`,
		tableName,
		ColumnTimestamp,
	)

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
