package nvlink

import (
	"context"
	"errors"
	"os"
	"reflect"
	"runtime"
	"testing"
	"time"

	"github.com/bytedance/mockey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/eventstore"
	pkghost "github.com/leptonai/gpud/pkg/host"
	"github.com/leptonai/gpud/pkg/kmsg"
)

func TestNew_WithEventStoreAsRoot_KmsgSyncer_WithMockey(t *testing.T) {
	mockey.PatchConvey("New creates and closes the event-backed kmsg syncer as root", t, func() {
		bootTime := time.Date(2026, 7, 15, 3, 0, 0, 0, time.UTC)
		mockey.Mock(os.Geteuid).Return(0).Build()
		mockey.Mock(pkghost.BootID).Return("").Build()
		mockey.Mock(pkghost.BootTime).Return(bootTime).Build()

		bucket := &memoryEventBucket{}
		var match kmsg.MatchFunc
		var syncerOpts []kmsg.OpOption
		mockey.Mock(kmsg.NewSyncer).To(func(
			_ context.Context,
			matchFunc kmsg.MatchFunc,
			eventBucket eventstore.Bucket,
			opts ...kmsg.OpOption,
		) (*kmsg.Syncer, error) {
			match = matchFunc
			syncerOpts = opts
			assert.Same(t, bucket, eventBucket)
			return &kmsg.Syncer{}, nil
		}).Build()
		mockey.Mock((*kmsg.Syncer).Close).Return().Build()

		created, err := New(&components.GPUdInstance{
			RootCtx:    context.Background(),
			EventStore: &memoryEventStore{bucket: bucket},
		})
		require.NoError(t, err)
		c := mustComponent(t, created)
		require.NotNil(t, c.kmsgSyncer)
		assert.Nil(t, c.readAllKmsg, "daemon components should rely on the kmsg syncer")
		require.NotNil(t, match)
		require.Len(t, syncerOpts, 1)
		assert.Equal(t, int(defaultKmsgEventDedupWindow.Seconds()), getKmsgCacheKeyTruncateSeconds(t, syncerOpts[0]))

		name, message := match("NVRM: knvlinkDiscoverPostRxDetLinks_GH100: Getting peer0's postRxDetLinkMask failed!")
		assert.Equal(t, EventNamePostRxDetectFailure, name)
		assert.Contains(t, message, bootTime.Format(time.RFC3339Nano))

		require.NoError(t, c.Close())
		assert.True(t, bucket.closed.Load())
		select {
		case <-c.ctx.Done():
		default:
			require.FailNow(t, "component context was not canceled")
		}
	})
}

func getKmsgCacheKeyTruncateSeconds(t *testing.T, opt kmsg.OpOption) int {
	t.Helper()
	op := &kmsg.Op{}
	opt(op)

	v := reflect.ValueOf(op).Elem().FieldByName("cacheKeyTruncateSeconds")
	require.True(t, v.IsValid(), "cacheKeyTruncateSeconds field must exist")
	require.Equal(t, reflect.Int, v.Kind(), "cacheKeyTruncateSeconds must be int")
	return int(v.Int())
}

func TestNew_WithEventStoreAsRoot_KmsgSyncerError_WithMockey(t *testing.T) {
	mockey.PatchConvey("New cleans up when the kmsg syncer cannot be created", t, func() {
		mockey.Mock(os.Geteuid).Return(0).Build()
		mockey.Mock(pkghost.BootID).Return("").Build()
		mockey.Mock(pkghost.BootTime).Return(time.Time{}).Build()
		mockey.Mock(kmsg.NewSyncer).To(func(
			_ context.Context,
			matchFunc kmsg.MatchFunc,
			_ eventstore.Bucket,
			_ ...kmsg.OpOption,
		) (*kmsg.Syncer, error) {
			name, message := matchFunc("NVRM: knvlinkDiscoverPostRxDetLinks_GH100: Getting peer0's postRxDetLinkMask failed!")
			assert.Equal(t, EventNamePostRxDetectFailure, name)
			assert.Contains(t, message, "boot ID: ")
			return nil, errors.New("failed to open /dev/kmsg")
		}).Build()

		bucket := &memoryEventBucket{}
		created, err := New(&components.GPUdInstance{
			RootCtx:    context.Background(),
			EventStore: &memoryEventStore{bucket: bucket},
		})

		require.EqualError(t, err, "failed to open /dev/kmsg")
		assert.Nil(t, created)
		assert.True(t, bucket.closed.Load())
	})
}

func TestNew_WithEventStoreBucketError(t *testing.T) {
	created, err := New(&components.GPUdInstance{
		RootCtx:    context.Background(),
		EventStore: &memoryEventStore{err: errors.New("bucket failed")},
	})

	require.EqualError(t, err, "bucket failed")
	assert.Nil(t, created)
}

func TestNew_OneShotKmsgReader_WithMockey(t *testing.T) {
	mockey.PatchConvey("New enables direct kmsg reads only for Linux root one-shot scans", t, func() {
		mockey.Mock(os.Geteuid).Return(0).Build()

		created, err := New(&components.GPUdInstance{RootCtx: context.Background()})
		require.NoError(t, err)
		c := mustComponent(t, created)
		if runtime.GOOS == "linux" {
			assert.NotNil(t, c.readAllKmsg)
		} else {
			assert.Nil(t, c.readAllKmsg)
		}
		require.NoError(t, c.Close())
	})
}
