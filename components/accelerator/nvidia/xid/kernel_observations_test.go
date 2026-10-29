package xid

import (
	"context"
	"testing"
	"time"

	"github.com/dsx-ai-factory/health-validation/collect/observation"
	xidsensor "github.com/dsx-ai-factory/health-validation/collect/sensors/xid"
	kmsgsignals "github.com/dsx-ai-factory/health-validation/collect/signals/kmsg"
	xidsignals "github.com/dsx-ai-factory/health-validation/collect/signals/xid"
	kmsgsource "github.com/dsx-ai-factory/health-validation/collect/source/kmsg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
)

func xidFixtureBatch(t *testing.T) *observation.ObservationBatch {
	t.Helper()
	sensor, err := xidsensor.NewKmsg(func(context.Context) ([]kmsgsource.Message, error) {
		return []kmsgsource.Message{
			{BootID: "boot-a", Sequence: 71, Timestamp: time.Unix(1700000000, 0), Priority: 3, Body: "NVRM: Xid (PCI:0018:01:00): 149, NETIR_LINK_EVT Fatal XC0 i0 Link 08 (0x004505c6 0x00000000)"},
			{BootID: "boot-b", Sequence: 71, Timestamp: time.Unix(1700000010, 0), Priority: 6, Body: "ordinary record"},
		}, nil
	})
	require.NoError(t, err)
	var ids []string
	for _, sig := range sensor.Signals() {
		ids = append(ids, sig.ID())
	}
	batch, err := components.MeasureSensor(t.Context(), sensor, ids...)
	require.NoError(t, err)
	return batch
}

func TestXidShimUsesSensorFactsAndKernelIdentity(t *testing.T) {
	batch := xidFixtureBatch(t)
	// Change raw text after collection. The shim must use parsed observations,
	// not a second parser of the raw body, for Xid and NVLink fields.
	for _, o := range batch.Observations {
		if o.SignalId == kmsgsignals.Message.ID() && o.Target.GetKernelRecord().BootId == "boot-a" {
			o.GetValue().Kind = &observation.Value_StringValue{StringValue: "stored original kernel payload"}
		}
	}
	records, err := kernelRecords(batch)
	require.NoError(t, err)
	require.Len(t, records, 2)
	require.NotNil(t, records[0].xid)
	assert.Equal(t, 149, records[0].xid.Xid)
	assert.Equal(t, "PCI:0018:01:00", records[0].xid.DeviceUUID)
	assert.Equal(t, 4, records[0].xid.Detail.SubCode)
	assert.Equal(t, apiv1.EventTypeFatal, records[0].xid.Detail.EventType)
	assert.Equal(t, time.Unix(1700000000, 0).UTC(), records[0].Timestamp.Time)
	assert.Equal(t, 71, records[0].SequenceNumber)
	assert.Equal(t, 3, records[0].Priority)
	assert.Equal(t, "stored original kernel payload", records[0].Message)
	assert.Nil(t, records[1].xid, "same sequence in a different boot must not join facts")
}

func TestXidShimRejectsInvalidFacts(t *testing.T) {
	for _, name := range []string{"duplicate", "missing address", "wrong code type", "parse error"} {
		t.Run(name, func(t *testing.T) {
			batch := xidFixtureBatch(t)
			var filtered []*observation.Observation
			for _, o := range batch.Observations {
				if name == "missing address" && o.SignalId == xidsignals.PCIAddress.ID() {
					continue
				}
				if o.SignalId == xidsignals.Code.ID() {
					switch name {
					case "duplicate":
						filtered = append(filtered, proto.Clone(o).(*observation.Observation))
					case "wrong code type":
						o.GetValue().Kind = &observation.Value_StringValue{StringValue: "149"}
					case "parse error":
						o.Outcome = &observation.Observation_CollectionError{CollectionError: &observation.CollectionError{Category: observation.CollectionErrorCategoryParseError, Detail: "invalid Xid"}}
					}
				}
				filtered = append(filtered, o)
			}
			batch.Observations = filtered
			records, err := kernelRecords(batch)
			require.Error(t, err)
			require.Len(t, records, 2)
			assert.Nil(t, records[0].xid)
			assert.Nil(t, records[1].xid)
		})
	}
}
