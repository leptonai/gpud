package kmsg

import (
	"bufio"
	"context"
	"errors"
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/bytedance/mockey"
	"github.com/dsx-ai-factory/health-validation/collect/observation"
	kmsgsignals "github.com/dsx-ai-factory/health-validation/collect/signals/kmsg"
	kmsgsource "github.com/dsx-ai-factory/health-validation/collect/source/kmsg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestKernelSensorFixture(t *testing.T) {
	file, err := os.Open("testdata/kmsg.1.log")
	require.NoError(t, err)
	defer func() { _ = file.Close() }()
	var input []kmsgsource.Message
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		if scanner.Text() == "" {
			continue
		}
		message, err := kmsgsource.Parse(time.Unix(1700000000, 0), scanner.Text())
		require.NoError(t, err)
		message.BootID = "fixture-boot"
		input = append(input, message)
	}
	require.NoError(t, scanner.Err())
	// Continuation lines, empty bodies, and repeated bodies must survive collection.
	input = append(input, sourceRecord(100000, ""), sourceRecord(100001, "line one\nline two"), sourceRecord(100002, "repeat"), sourceRecord(100003, "repeat"))
	otherBoot := sourceRecord(100003, "same sequence in a different boot")
	otherBoot.BootID = "boot-b"
	input = append(input, otherBoot)
	sensor, err := newSensor(func(context.Context) ([]kmsgsource.Message, error) { return input, nil })
	require.NoError(t, err)
	records, err := collectMessages(t.Context(), sensor)
	require.NoError(t, err)
	require.Len(t, records, len(input))
	for i, record := range records {
		assert.Equal(t, int(input[i].Sequence), record.SequenceNumber)
		assert.Equal(t, input[i].Body, record.Message)
		assert.Equal(t, input[i].Priority, record.Priority)
		assert.Equal(t, input[i].Timestamp.UTC(), record.Timestamp.Time)
	}
}

func TestKernelSensorPartialFailure(t *testing.T) {
	for _, failure := range []error{syscall.EPIPE, kmsgsource.ErrMalformedRecord, errors.New("I/O error")} {
		sensor, err := newSensor(func(context.Context) ([]kmsgsource.Message, error) {
			return []kmsgsource.Message{sourceRecord(4, "retained")}, failure
		})
		require.NoError(t, err)
		records, err := collectMessages(t.Context(), sensor)
		require.ErrorContains(t, err, failure.Error())
		require.Len(t, records, 1)
		assert.Equal(t, "retained", records[0].Message)
	}
}

func TestReadAllUsesSharedReader(t *testing.T) {
	mockey.PatchConvey("snapshot reader and GPUd content dedup", t, func() {
		mockey.Mock(kmsgsource.ReadAll).To(func(context.Context) ([]kmsgsource.Message, error) {
			return []kmsgsource.Message{sourceRecord(1, "same"), sourceRecord(2, "same"), sourceRecord(3, "different")}, nil
		}).Build()
		messages, err := ReadAll(t.Context())
		require.NoError(t, err)
		require.Len(t, messages, 2)
		assert.Equal(t, []int{1, 3}, []int{messages[0].SequenceNumber, messages[1].SequenceNumber})
	})
}

func TestNewWatcherUsesSharedStream(t *testing.T) {
	mockey.PatchConvey("shared stream constructor failure is returned", t, func() {
		failure := errors.New("cannot open kernel stream")
		mockey.Mock(kmsgsource.NewStream).To(func(int) (*kmsgsource.Stream, error) { return nil, failure }).Build()
		watcher, err := NewWatcher()
		require.ErrorIs(t, err, failure)
		assert.True(t, watcher == nil, "constructor failures must return a nil interface")
	})
}

func TestCollectKernelRecordsCanceled(t *testing.T) {
	sensor, err := newSensor(func(context.Context) ([]kmsgsource.Message, error) {
		t.Error("canceled collection must not read")
		return nil, nil
	})
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = collectMessages(ctx, sensor)
	require.ErrorIs(t, err, context.Canceled)
}

func TestRecordsFromObservations(t *testing.T) {
	target := observation.KernelRecordTarget("boot-a", 1)
	at := timestamppb.New(time.Unix(1700000000, 0))
	message := &observation.Observation{SignalId: kmsgsignals.Message.ID(), Target: target, ObservedAt: at,
		Outcome: &observation.Observation_Value{Value: &observation.Value{Kind: &observation.Value_StringValue{StringValue: "body"}}}}
	priority := &observation.Observation{SignalId: kmsgsignals.Priority.ID(), Target: target, ObservedAt: at,
		Outcome: &observation.Observation_Value{Value: &observation.Value{Kind: &observation.Value_IntValue{IntValue: 6}}}}
	for _, tc := range []struct {
		name  string
		batch *observation.ObservationBatch
		fail  bool
		count int
	}{
		{name: "nil batch", fail: true},
		{name: "empty", batch: &observation.ObservationBatch{}},
		{name: "nil observation", batch: &observation.ObservationBatch{Observations: []*observation.Observation{nil}}, fail: true},
		{name: "missing priority", batch: &observation.ObservationBatch{Observations: []*observation.Observation{message}}, fail: true},
		{name: "missing body", batch: &observation.ObservationBatch{Observations: []*observation.Observation{priority}}, fail: true},
		{name: "duplicate body", batch: &observation.ObservationBatch{Observations: []*observation.Observation{message, message, priority}}, fail: true},
		{name: "priority first", batch: &observation.ObservationBatch{Observations: []*observation.Observation{priority, message}}, count: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			records, err := messagesFromObservations(tc.batch)
			if tc.fail {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			assert.Len(t, records, tc.count)
		})
	}
}
