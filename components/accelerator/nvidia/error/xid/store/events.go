package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"sync"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/components/common"
	"github.com/leptonai/gpud/log"
	"github.com/leptonai/gpud/pkg/sqlite"
)

const (
	ColumnID              = "id"
	ColumnTimestamp       = "timestamp"
	ColumnName            = "name"
	ColumnType            = "type"
	ColumnMessage         = "message"
	ColumnExtraInfo       = "extra_info"
	ColumnSuggestedAction = "suggested_action"

	DefaultRetentionPeriod = 3 * 24 * time.Hour
)

type eventKey struct {
	timestamp time.Time
	name      string
	eventType string
}

type eventCache struct {
	sync.RWMutex
	events    map[eventKey]components.Event
	timeIndex []time.Time
	dirty     bool
}

func newEventCache() *eventCache {
	return &eventCache{
		events: make(map[eventKey]components.Event),
	}
}

func (c *eventCache) add(event components.Event) bool {
	c.Lock()
	defer c.Unlock()

	key := eventKey{
		timestamp: event.Time.Time,
		name:      event.Name,
		eventType: string(event.Type),
	}

	if _, exists := c.events[key]; exists {
		return false
	}

	c.events[key] = event
	c.dirty = true
	return true
}

func (c *eventCache) rebuildTimeIndex() {
	if !c.dirty {
		return
	}

	c.timeIndex = make([]time.Time, 0, len(c.events))
	for k := range c.events {
		c.timeIndex = append(c.timeIndex, k.timestamp)
	}

	sort.Slice(c.timeIndex, func(i, j int) bool {
		return c.timeIndex[i].Before(c.timeIndex[j])
	})

	c.dirty = false
}

type Store struct {
	db              *sql.DB
	retentionPeriod time.Duration
	cache           *eventCache
	tableName       string
}

func New(ctx context.Context, db *sql.DB, tableName string) (*Store, error) {
	store := &Store{
		db:              db,
		retentionPeriod: DefaultRetentionPeriod,
		cache:           newEventCache(),
		tableName:       tableName,
	}

	if err := store.createTable(ctx); err != nil {
		return nil, fmt.Errorf("create table failed: %w", err)
	}

	if err := store.warmupCache(ctx); err != nil {
		return nil, fmt.Errorf("warmup cache failed: %w", err)
	}

	go store.purgeRoutine(ctx)

	return store, nil
}

func (s *Store) createTable(ctx context.Context) error {
	query := fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
		%s INTEGER PRIMARY KEY AUTOINCREMENT,
		%s INTEGER NOT NULL,
		%s TEXT NOT NULL,
		%s TEXT NOT NULL,
		%s TEXT,
		%s TEXT,
		%s TEXT
	);`, s.tableName,
		ColumnID, ColumnTimestamp, ColumnName, ColumnType,
		ColumnMessage, ColumnExtraInfo, ColumnSuggestedAction)

	_, err := s.db.ExecContext(ctx, query)
	return err
}

func (s *Store) CreateEvent(ctx context.Context, event components.Event) (int, error) {
	if !s.cache.add(event) {
		return 0, nil
	}

	query := fmt.Sprintf(`INSERT INTO %s (%s, %s, %s, %s, %s, %s)
		VALUES (?, ?, ?, ?, ?, ?)`,
		s.tableName,
		ColumnTimestamp, ColumnName, ColumnType,
		ColumnMessage, ColumnExtraInfo, ColumnSuggestedAction)

	extraInfoJSON, err := json.Marshal(event.ExtraInfo)
	if err != nil {
		return 0, fmt.Errorf("marshal extra info failed: %w", err)
	}

	var suggestedActionJSON []byte
	if event.SuggestedActions != nil {
		suggestedActionJSON, err = json.Marshal(event.SuggestedActions)
		if err != nil {
			return 0, fmt.Errorf("marshal suggested actions failed: %w", err)
		}
	}

	start := time.Now()
	_, err = s.db.ExecContext(ctx, query,
		event.Time.UnixNano(),
		event.Name,
		event.Type,
		event.Message,
		string(extraInfoJSON),
		string(suggestedActionJSON),
	)
	if err != nil {
		return 0, fmt.Errorf("insert event failed: %w", err)
	}
	sqlite.RecordInsertUpdate(time.Since(start).Seconds())
	return 1, nil
}

func (s *Store) GetEvents(ctx context.Context, since time.Time) ([]components.Event, error) {
	s.cache.RLock()
	defer s.cache.RUnlock()

	s.cache.rebuildTimeIndex()

	var result []components.Event
	for _, ts := range s.cache.timeIndex {
		if ts.After(since) {
			for k, v := range s.cache.events {
				if k.timestamp == ts {
					result = append(result, v)
				}
			}
		}
	}

	return result, nil
}

func (s *Store) GetAllEvents(ctx context.Context) ([]components.Event, error) {
	return s.GetEvents(ctx, time.Time{})
}

func (s *Store) warmupCache(ctx context.Context) error {
	start := time.Now()
	query := fmt.Sprintf(`SELECT %s, %s, %s, %s, %s, %s 
		FROM %s 
		WHERE %s >= ?
		ORDER BY %s ASC`,
		ColumnTimestamp, ColumnName, ColumnType,
		ColumnMessage, ColumnExtraInfo, ColumnSuggestedAction,
		s.tableName,
		ColumnTimestamp,
		ColumnTimestamp)

	rows, err := s.db.QueryContext(ctx, query, time.Now().Add(-s.retentionPeriod).UnixNano())
	if err != nil {
		return fmt.Errorf("query events failed: %w", err)
	}
	sqlite.RecordSelect(time.Since(start).Seconds())
	defer rows.Close()

	for rows.Next() {
		var (
			event           components.Event
			timestampNs     int64
			extraInfoStr    string
			suggestedAction string
		)

		if err := rows.Scan(
			&timestampNs,
			&event.Name,
			&event.Type,
			&event.Message,
			&extraInfoStr,
			&suggestedAction,
		); err != nil {
			return fmt.Errorf("scan row failed: %w", err)
		}

		event.Time = metav1.Time{Time: time.Unix(0, timestampNs)}

		if extraInfoStr != "" {
			if err := json.Unmarshal([]byte(extraInfoStr), &event.ExtraInfo); err != nil {
				return fmt.Errorf("unmarshal extra info failed: %w", err)
			}
		}

		if suggestedAction != "" {
			var actions common.SuggestedActions
			if err := json.Unmarshal([]byte(suggestedAction), &actions); err != nil {
				return fmt.Errorf("unmarshal suggested actions failed: %w", err)
			}
			event.SuggestedActions = &actions
		}

		s.cache.add(event)
	}

	return rows.Err()
}

func (s *Store) purge(ctx context.Context) (int, error) {
	start := time.Now()

	query := fmt.Sprintf(`DELETE FROM %s WHERE %s < ?`,
		s.tableName, ColumnTimestamp)

	result, err := s.db.ExecContext(ctx, query, start.UTC().Add(-s.retentionPeriod).UnixNano())
	if err != nil {
		return 0, fmt.Errorf("delete events failed: %w", err)
	}
	sqlite.RecordDelete(time.Since(start).Seconds())

	deleted, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("get deleted rows failed: %w", err)
	}

	s.cache.Lock()
	for k, v := range s.cache.events {
		if v.Time.Time.Before(start.Add(-s.retentionPeriod)) {
			delete(s.cache.events, k)
		}
	}
	s.cache.dirty = true
	s.cache.Unlock()

	return int(deleted), nil
}

func (s *Store) purgeRoutine(ctx context.Context) {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if _, err := s.purge(ctx); err != nil {
				log.Logger.Errorw("purge events failed", "error", err)
			}
		}
	}
}
