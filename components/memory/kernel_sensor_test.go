package memory

import (
	"context"
	"testing"
	"time"

	"github.com/dsx-ai-factory/health-validation/collect/observation"
	kmsgsensor "github.com/dsx-ai-factory/health-validation/collect/sensors/kmsg"
	kmsgsignals "github.com/dsx-ai-factory/health-validation/collect/signals/kmsg"
	kmsgsource "github.com/dsx-ai-factory/health-validation/collect/source/kmsg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/eventstore"
	"github.com/leptonai/gpud/pkg/sqlite"
)

func TestKernelSensorShimPreservesMultirecordOOM(t *testing.T) {
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()
	store, err := eventstore.New(dbRW, dbRO, eventstore.DefaultRetention)
	require.NoError(t, err)
	bucket, err := store.Bucket(Name)
	require.NoError(t, err)
	defer bucket.Close()
	c := &component{ctx: t.Context(), eventBucket: bucket}
	var consume func(context.Context, *observation.ObservationBatch)
	stop, err := c.observeKernel(func(_ context.Context, callback func(context.Context, *observation.ObservationBatch)) (func(), error) {
		consume = callback
		return func() {}, nil
	}, createMatchFunc(), eventstore.DedupPolicy{})
	require.NoError(t, err)
	defer stop()
	var input []kmsgsource.Message
	sensor, err := kmsgsensor.New(func(context.Context) ([]kmsgsource.Message, error) { return input, nil }, map[string]string{"gpud.kernel_record": `(?s).*`})
	require.NoError(t, err)
	at := time.Date(2026, 10, 29, 1, 0, 0, 0, time.UTC)
	// Each line arrives in a different collection cycle, including a continuation
	// that is not itself an OOM start. The component owns reconstruction state.
	for i, line := range []string{
		"apache2 invoked oom-killer: gfp_mask=0x201d2, order=0, oomkilladj=0",
		"Task in /docker/container456 killed as a result of limit of /docker/container456",
		"Killed process 9876 (apache2)",
	} {
		input = []kmsgsource.Message{{BootID: "boot", Sequence: int64(i), Timestamp: at, Priority: 6, Body: line}}
		batch, err := components.MeasureSensor(t.Context(), sensor, kmsgsignals.Message.ID(), kmsgsignals.Priority.ID())
		require.NoError(t, err)
		consume(t.Context(), batch)
	}
	events, err := bucket.Get(t.Context(), time.Unix(0, 0))
	require.NoError(t, err)
	require.Len(t, events, 1)
	assert.Equal(t, OOMEventName, events[0].Name)
	assert.Equal(t, "OOM encountered, victim process: apache2, pid: 9876", events[0].Message)
	assert.Equal(t, string(apiv1.EventTypeWarning), events[0].Type)
	assert.True(t, at.Equal(events[0].Time))
}
