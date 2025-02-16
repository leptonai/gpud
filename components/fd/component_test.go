package fd

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/leptonai/gpud/components"
	query_config "github.com/leptonai/gpud/pkg/query/config"
	"github.com/leptonai/gpud/pkg/sqlite"
)

func TestEvents(t *testing.T) {
	comp, ctx, cleanup := createTestComponent(t)
	defer cleanup()
	defer comp.Close()

	events, err := comp.Events(ctx, time.Now().Add(-query_config.DefaultPollInterval))
	require.NoError(t, err)
	assert.Empty(t, events)
}

func TestMetrics(t *testing.T) {
	comp, ctx, cleanup := createTestComponent(t)
	defer cleanup()
	defer comp.Close()

	metrics, err := comp.Metrics(ctx, time.Now().Add(-query_config.DefaultPollInterval))
	require.NoError(t, err)
	assert.NotNil(t, metrics)
}

func TestClose(t *testing.T) {
	comp, _, cleanup := createTestComponent(t)
	defer cleanup()

	err := comp.Close()
	require.NoError(t, err)
}

func createTestComponent(t *testing.T) (components.Component, context.Context, func()) {
	ctx := context.Background()
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)

	cfg := Config{
		Query: query_config.Config{
			State: &query_config.State{
				DBRW: dbRW,
				DBRO: dbRO,
			},
		},
	}

	comp, err := New(ctx, cfg)
	require.NoError(t, err)
	require.NotNil(t, comp)
	return comp, ctx, cleanup
}
