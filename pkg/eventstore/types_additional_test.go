package eventstore

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apiv1 "github.com/leptonai/gpud/api/v1"
)

func TestEventsConversion(t *testing.T) {
	base := time.Unix(1_700_000_000, 0).UTC()
	evs := Events{
		{
			Component: "component-a",
			Time:      base,
			Name:      "event-a",
			Type:      string(apiv1.EventTypeWarning),
			Message:   "warning message",
		},
		{
			Component: "component-b",
			Time:      base.Add(time.Second),
			Name:      "event-b",
			Type:      string(apiv1.EventTypeInfo),
			Message:   "info message",
		},
	}

	got := evs.Events()
	require.Len(t, got, 2)

	assert.Equal(t, "component-a", got[0].Component)
	assert.Equal(t, base.Unix(), got[0].Time.Unix())
	assert.Equal(t, "event-a", got[0].Name)
	assert.Equal(t, apiv1.EventTypeWarning, got[0].Type)
	assert.Equal(t, "warning message", got[0].Message)

	assert.Equal(t, "component-b", got[1].Component)
	assert.Equal(t, base.Add(time.Second).Unix(), got[1].Time.Unix())
	assert.Equal(t, "event-b", got[1].Name)
	assert.Equal(t, apiv1.EventTypeInfo, got[1].Type)
	assert.Equal(t, "info message", got[1].Message)
}

func TestEventToEvent(t *testing.T) {
	ev := Event{
		Component: "component-c",
		Time:      time.Unix(1_700_000_123, 0).UTC(),
		Name:      "event-c",
		Type:      string(apiv1.EventTypeCritical),
		Message:   "critical message",
	}

	got := ev.ToEvent()
	assert.Equal(t, ev.Component, got.Component)
	assert.Equal(t, ev.Time.Unix(), got.Time.Unix())
	assert.Equal(t, ev.Name, got.Name)
	assert.Equal(t, apiv1.EventTypeCritical, got.Type)
	assert.Equal(t, ev.Message, got.Message)
}

func TestOpApplyOptsAndWithDisablePurge(t *testing.T) {
	op := &Op{}
	require.NoError(t, op.applyOpts(nil))
	assert.False(t, op.disablePurge)

	require.NoError(t, op.applyOpts([]OpOption{WithDisablePurge()}))
	assert.True(t, op.disablePurge)
}
