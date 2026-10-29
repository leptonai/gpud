package sxid

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/bytedance/mockey"
	kmsgsource "github.com/dsx-ai-factory/health-validation/collect/source/kmsg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestKernelSnapshotUsesHealthValidation(t *testing.T) {
	t.Setenv("KMSG_FILE_PATH", "")
	mockey.PatchConvey("full history, partial errors, and GPUd content dedup", t, func() {
		failure := errors.New("partial kernel read")
		mockey.Mock(kmsgsource.ReadAll).To(func(context.Context) ([]kmsgsource.Message, error) {
			records := make([]kmsgsource.Message, 0, 5001)
			for i := range 5000 {
				records = append(records, kmsgsource.Message{BootID: "boot", Sequence: int64(i), Timestamp: time.Unix(int64(1700000000+i*60), 0), Priority: 6, Body: "record"})
			}
			records = append(records, kmsgsource.Message{BootID: "boot", Sequence: 5000, Timestamp: records[4999].Timestamp, Priority: 6, Body: "record"})
			return records, failure
		}).Build()
		records, err := readKernelMessages(t.Context())
		require.ErrorContains(t, err, failure.Error())
		require.Len(t, records, 5000, "snapshots must not be limited to the streaming window")
		assert.Equal(t, 0, records[0].SequenceNumber)
		assert.Equal(t, 4999, records[len(records)-1].SequenceNumber)
	})
}

func TestKernelSnapshotRejectsUnsupportedSourceAndCancellation(t *testing.T) {
	t.Setenv("KMSG_FILE_PATH", "/tmp/legacy-kmsg")
	records, err := readKernelMessages(t.Context())
	require.ErrorContains(t, err, "unsupported")
	require.Empty(t, records)

	t.Setenv("KMSG_FILE_PATH", "/dev/kmsg")
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	records, err = readKernelMessages(ctx)
	require.ErrorIs(t, err, context.Canceled)
	require.Empty(t, records)
}
