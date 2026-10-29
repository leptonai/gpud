package xid

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/dsx-ai-factory/health-validation/collect/observation"
	kmsgsensor "github.com/dsx-ai-factory/health-validation/collect/sensors/kmsg"
	xidsignals "github.com/dsx-ai-factory/health-validation/collect/signals/xid"
	kmsgsource "github.com/dsx-ai-factory/health-validation/collect/source/kmsg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apiv1 "github.com/leptonai/gpud/api/v1"
	componentkmsg "github.com/leptonai/gpud/components/kmsg"
)

func xidFixtureBatch(t *testing.T, extra ...kmsgsource.Message) *observation.ObservationBatch {
	t.Helper()
	sensor, err := kmsgsensor.New(func(context.Context) ([]kmsgsource.Message, error) {
		return append([]kmsgsource.Message{
			{BootID: "boot-a", Sequence: 71, Timestamp: time.Unix(1700000000, 0), Priority: 3, Body: "NVRM: Xid (PCI:0018:01:00): 149, NETIR_LINK_EVT Fatal XC0 i0 Link 08 (0x004505c6 0x00000000)"},
			{BootID: "boot-b", Sequence: 71, Timestamp: time.Unix(1700000010, 0), Priority: 6, Body: "ordinary record"},
		}, extra...), nil
	}, map[string]string{"gpud.kernel_record": `(?s).*`})
	require.NoError(t, err)
	batch, err := componentkmsg.Measure(t.Context(), sensor)
	require.NoError(t, err)
	return batch
}

func TestXidShimUsesLibraryParserAndPreservesKernelIdentity(t *testing.T) {
	batch := xidFixtureBatch(t)
	original := proto.Clone(batch)
	records, err := kernelRecords(batch)
	require.NoError(t, err)
	require.Len(t, records, 2)
	require.NotNil(t, records[0].xid)
	assert.Equal(t, 149, records[0].xid.Xid)
	assert.Equal(t, "PCI:0018:01:00", records[0].xid.DeviceUUID)
	assert.Equal(t, 4, records[0].xid.Detail.SubCode)
	assert.Equal(t, apiv1.EventTypeFatal, records[0].xid.Detail.EventType)
	assert.Equal(t, time.Unix(1700000000, 0).UTC(), records[0].Timestamp.Time)
	assert.Equal(t, "boot-a", records[0].BootID)
	assert.Equal(t, 71, records[0].SequenceNumber)
	assert.Equal(t, 3, records[0].Priority)
	assert.Equal(t, "NVRM: Xid (PCI:0018:01:00): 149, NETIR_LINK_EVT Fatal XC0 i0 Link 08 (0x004505c6 0x00000000)", records[0].Message)
	assert.Nil(t, records[1].xid)
	assert.True(t, proto.Equal(original, batch), "component interpretation must not mutate shared observations")
}

func TestXidParseFailureStaysLocalToComponent(t *testing.T) {
	const malformed = "NVRM: Xid (PCI:0000:01:00): invalid"
	batch := xidFixtureBatch(t, kmsgsource.Message{BootID: "boot-a", Sequence: 72, Timestamp: time.Unix(1700000020, 0), Priority: 3, Body: malformed})
	records, err := kernelRecords(batch)
	var parseErr *xidsignals.ParseError
	require.ErrorAs(t, err, &parseErr)
	require.Len(t, records, 3)
	require.NotNil(t, records[0].xid, "a malformed Xid must not discard other events")
	assert.Nil(t, records[2].xid)
	assert.Equal(t, malformed, records[2].Message)

	otherComponentRecords, err := componentkmsg.Messages(batch)
	require.NoError(t, err, "other components receive raw records without Xid parse failures")
	require.Len(t, otherComponentRecords, 3)
	assert.Equal(t, malformed, otherComponentRecords[2].Message)
}

func TestXidShimNumericNVLinkUnits(t *testing.T) {
	for i, unit := range []string{"TLW_RX/TLW_RX_PIPE0", "TLW_RX/TLW_RX_PIPE1", "TLW_TX/TLW_TX_PIPE0", "TLW_TX/TLW_TX_PIPE1"} {
		for _, fatal := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/fatal=%t", unit, fatal), func(t *testing.T) {
				severity, status := "Nonfatal", uint32(1)
				eventType, action := apiv1.EventTypeWarning, apiv1.RepairActionTypeIgnoreNoActionRequired
				if fatal {
					severity, status = "Fatal", 4
					eventType, action = apiv1.EventTypeFatal, apiv1.RepairActionTypeRebootSystem
				}
				line := fmt.Sprintf("NVRM: Xid (PCI:0000:04:00): 146, %s %s XC0 i0 Link 00 (0x%08x 0x%08x 0x00000000 0x00000000)", unit, severity, 10+i, status)
				read := kernelReaderForTest(func(context.Context) ([]apiv1.KernelMessage, error) {
					return []apiv1.KernelMessage{{Timestamp: metav1.NewTime(time.Unix(1700000000, 0)), Message: line}}, nil
				})
				records, err := read(t.Context())
				require.NoError(t, err)
				require.Len(t, records, 1)
				require.NotNil(t, records[0].xid)
				detail := records[0].xid.Detail
				require.NotNil(t, detail)
				assert.Equal(t, unit, detail.SubCodeDescription)
				assert.Equal(t, status, detail.ErrorStatus)
				assert.Equal(t, eventType, detail.EventType)
				require.NotNil(t, detail.SuggestedActionsByGPUd)
				assert.Equal(t, []apiv1.RepairActionType{action}, detail.SuggestedActionsByGPUd.RepairActions)

				// Default mode retains the old parser's base policy for numeric units.
				legacy := Match(line)
				require.NotNil(t, legacy)
				assert.Equal(t, apiv1.EventTypeWarning, legacy.Detail.EventType)
				assert.Equal(t, []apiv1.RepairActionType{apiv1.RepairActionTypeRebootSystem}, legacy.Detail.SuggestedActionsByGPUd.RepairActions)
			})
		}
	}
}
