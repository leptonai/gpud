package xid

import (
	"context"
	"testing"
	"time"

	"github.com/bytedance/mockey"
	"github.com/dsx-ai-factory/health-validation/collect/observation"
	kmsgsource "github.com/dsx-ai-factory/health-validation/collect/source/kmsg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apiv1 "github.com/leptonai/gpud/api/v1"
)

func recurringRecoveryRecords() []kmsgsource.Message {
	at := time.Now().UTC().Truncate(time.Minute).Add(-time.Minute)
	request := "NVRM: Xid (PCI:0000:01:00): 154, GPU recovery action changed from 0x0 (None) to 0x1 (GPU Reset Required)"
	none := "NVRM: Xid (PCI:0000:01:00): 154, GPU recovery action changed from 0x1 (GPU Reset Required) to 0x0 (None)"
	return []kmsgsource.Message{
		{BootID: "boot", Sequence: 1, Timestamp: at, Priority: 3, Body: request},
		{BootID: "boot", Sequence: 2, Timestamp: at.Add(time.Second), Priority: 3, Body: none},
		{BootID: "boot", Sequence: 3, Timestamp: at.Add(2 * time.Second), Priority: 3, Body: request},
	}
}

func TestKernelSensorPreservesRecurringRecoveryTransitions(t *testing.T) {
	c, _ := newTestComponentWithNVSentinel(t, nil)
	var consume func(context.Context, *observation.ObservationBatch)
	messages, stop, err := c.observeKernel(func(_ context.Context, callback func(context.Context, *observation.ObservationBatch), _ func()) (func(), error) {
		consume = callback
		return func() {}, nil
	})
	require.NoError(t, err)
	defer stop()
	records := recurringRecoveryRecords()
	batch := kernelDeliveryBatch(t, records)
	consume(t.Context(), batch)
	consume(t.Context(), batch)
	require.Len(t, messages, 3, "recurring transitions survive; replayed records do not duplicate them")
	go c.start(messages, time.Hour)
	waitForRecoveryBatch(t, c, records[0].Timestamp.Add(-time.Second), 3)
	require.Eventually(t, func() bool {
		return c.LastHealthStates()[0].Health == apiv1.HealthStateTypeUnhealthy
	}, time.Second, 10*time.Millisecond)
	state := c.LastHealthStates()[0]
	require.NotNil(t, state.SuggestedActions)
	assert.Contains(t, state.SuggestedActions.RepairActions, apiv1.RepairActionTypeRebootSystem)
}

func TestKernelSnapshotPreservesRecurringRecoveryTransitions(t *testing.T) {
	t.Setenv("KMSG_FILE_PATH", "")
	mockey.PatchConvey("snapshot preserves recurring recovery transitions", t, func() {
		source := recurringRecoveryRecords()
		mockey.Mock(kmsgsource.ReadAll).To(func(context.Context) ([]kmsgsource.Message, error) {
			return source, nil
		}).Build()
		records, err := readKernelMessages(t.Context())
		require.NoError(t, err)
		require.Len(t, records, 3)
		for i, record := range records {
			require.NotNil(t, record.xid)
			require.NotNil(t, record.xid.RecoveryAction)
			assert.True(t, record.xid.RecoveryAction.Valid)
			assert.Equal(t, source[i].Body, record.xid.RecoveryAction.Raw)
			assert.Equal(t, int(source[i].Sequence), record.SequenceNumber)
			assert.Equal(t, Match(source[i].Body), record.xid)
		}
		assert.Equal(t, uint32(1), records[0].xid.RecoveryAction.To)
		assert.Equal(t, uint32(0), records[1].xid.RecoveryAction.To)
		assert.Equal(t, uint32(1), records[2].xid.RecoveryAction.To)
	})
}
