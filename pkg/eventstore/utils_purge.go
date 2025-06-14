package eventstore

import (
	"context"
	"database/sql"
	"strings"

	"github.com/leptonai/gpud/pkg/log"
)

// PurgeByComponents purges events by the component names.
// If the component names are specified, it only purges the event tables for the specified components.
// If the component names are not specified, it purges all event tables.
func PurgeByComponents(ctx context.Context, dbRW *sql.DB, dbRO *sql.DB, beforeTimestamp int64, componentNames ...string) error {
	tableNames := make([]string, 0)
	for _, componentName := range componentNames {
		tableNames = append(tableNames, TableName(componentName))
	}

	if len(tableNames) == 0 {
		var err error
		tableNames, err = listTables(ctx, dbRO)
		if err != nil {
			return err
		}
		log.Logger.Infow("purging all event tables", "tableNames", tableNames)
	}

	for _, tableName := range tableNames {
		purged, err := purgeEvents(ctx, dbRW, tableName, beforeTimestamp)
		if err != nil {
			return err
		}
		log.Logger.Infow("purged events", "tableName", tableName, "purged", purged)
	}

	return nil
}

func listTables(ctx context.Context, dbRO *sql.DB) ([]string, error) {
	rows, err := dbRO.QueryContext(ctx, "SELECT name FROM sqlite_master WHERE type='table'")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	names := make([]string, 0)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}

		if !strings.HasPrefix(name, "components_") || !strings.Contains(name, "_events") {
			continue
		}

		names = append(names, name)
	}

	return names, nil
}
