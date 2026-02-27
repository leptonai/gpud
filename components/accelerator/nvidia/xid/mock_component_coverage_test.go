package xid

import (
	"context"
	"errors"
	"os"
	"runtime"
	"testing"
	"time"

	"github.com/bytedance/mockey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/eventstore"
	"github.com/leptonai/gpud/pkg/kmsg"
	"github.com/leptonai/gpud/pkg/nvidia/nvml/device"
)

type configurableNVML struct {
	*mockNVMLInstance
	nvmlExists  bool
	productName string
	initErr     error
}

func (m *configurableNVML) NVMLExists() bool    { return m.nvmlExists }
func (m *configurableNVML) ProductName() string { return m.productName }
func (m *configurableNVML) InitError() error    { return m.initErr }

type stubEventBucket struct {
	getErr    error
	getEvents eventstore.Events
	insertErr error
	findErr   error
	findEvent *eventstore.Event
}

func (s *stubEventBucket) Name() string                                          { return "stub" }
func (s *stubEventBucket) Insert(ctx context.Context, ev eventstore.Event) error { return s.insertErr }
func (s *stubEventBucket) Find(ctx context.Context, ev eventstore.Event) (*eventstore.Event, error) {
	return s.findEvent, s.findErr
}
func (s *stubEventBucket) Get(ctx context.Context, since time.Time) (eventstore.Events, error) {
	return s.getEvents, s.getErr
}
func (s *stubEventBucket) Latest(ctx context.Context) (*eventstore.Event, error) { return nil, nil }
func (s *stubEventBucket) Purge(ctx context.Context, beforeTimestamp int64) (int, error) {
	return 0, nil
}
func (s *stubEventBucket) Close() {}

type stubEventStore struct {
	bucket eventstore.Bucket
	err    error
}

func (s *stubEventStore) Bucket(name string, opts ...eventstore.OpOption) (eventstore.Bucket, error) {
	return s.bucket, s.err
}

type stubRebootStore struct {
	events eventstore.Events
	err    error
}

func (s *stubRebootStore) RecordReboot(ctx context.Context) error { return nil }
func (s *stubRebootStore) GetRebootEvents(ctx context.Context, since time.Time) (eventstore.Events, error) {
	return s.events, s.err
}

type stubWatcher struct {
	watchErr error
	ch       chan kmsg.Message
	closeErr error
}

func (s *stubWatcher) Watch() (<-chan kmsg.Message, error) {
	if s.watchErr != nil {
		return nil, s.watchErr
	}
	if s.ch == nil {
		s.ch = make(chan kmsg.Message, 1)
	}
	return s.ch, nil
}

func (s *stubWatcher) Close() error { return s.closeErr }

func TestCheckResult_Coverage_WithMockey(t *testing.T) {
	var nilCR *checkResult
	assert.Equal(t, Name, nilCR.ComponentName())
	assert.Equal(t, apiv1.HealthStateType(""), nilCR.HealthStateType())
	hs := nilCR.HealthStates()
	require.Len(t, hs, 1)
	assert.Equal(t, Name, hs[0].Component)
	assert.Equal(t, "no data yet", hs[0].Reason)

	cr := &checkResult{
		ts:     time.Now().UTC(),
		err:    errors.New("test-error"),
		health: apiv1.HealthStateTypeUnhealthy,
		reason: "reason",
	}
	assert.Equal(t, Name, cr.ComponentName())
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.HealthStateType())
	hs2 := cr.HealthStates()
	require.Len(t, hs2, 1)
	assert.Equal(t, "test-error", hs2[0].Error)
	assert.Equal(t, "reason", hs2[0].Reason)
}

func TestNew_Branches_WithMockey(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mockey.PatchConvey("New returns event bucket error", t, func() {
		gpudInstance := &components.GPUdInstance{
			RootCtx:    ctx,
			EventStore: &stubEventStore{err: errors.New("bucket failed")},
		}
		comp, err := New(gpudInstance)
		require.Error(t, err)
		assert.Nil(t, comp)
		assert.Contains(t, err.Error(), "bucket failed")
	})

	mockey.PatchConvey("New returns watcher error when running as root", t, func() {
		mockey.Mock(os.Geteuid).To(func() int { return 0 }).Build()
		mockey.Mock(kmsg.NewWatcher).To(func(opts ...kmsg.OpOption) (kmsg.Watcher, error) {
			return nil, errors.New("watcher failed")
		}).Build()

		gpudInstance := &components.GPUdInstance{
			RootCtx:    ctx,
			EventStore: &stubEventStore{bucket: &stubEventBucket{}},
		}
		comp, err := New(gpudInstance)
		require.Error(t, err)
		assert.Nil(t, comp)
		assert.Contains(t, err.Error(), "watcher failed")
	})
}

func TestComponent_Start_Events_Close_Coverage_WithMockey(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	t.Run("Start exits on context canceled update", func(t *testing.T) {
		c := &component{ctx: ctx}
		mockey.PatchConvey("start exits when update returns context.Canceled", t, func() {
			mockey.Mock((*component).updateCurrentState).To(func(_ *component) error {
				return context.Canceled
			}).Build()

			err := c.Start()
			require.NoError(t, err)
		})
	})

	t.Run("Start returns watcher watch error", func(t *testing.T) {
		c := &component{
			ctx:         ctx,
			kmsgWatcher: &stubWatcher{watchErr: errors.New("watch failed")},
		}
		mockey.PatchConvey("start returns watch error after successful state update", t, func() {
			mockey.Mock((*component).updateCurrentState).To(func(_ *component) error {
				return nil
			}).Build()

			err := c.Start()
			require.Error(t, err)
			assert.Contains(t, err.Error(), "watch failed")
		})
	})

	t.Run("Events returns bucket get error", func(t *testing.T) {
		c := &component{
			eventBucket: &stubEventBucket{getErr: errors.New("get failed")},
			devices:     map[string]device.Device{},
		}
		events, err := c.Events(ctx, time.Now().Add(-time.Hour))
		require.Error(t, err)
		assert.Nil(t, events)
		assert.Contains(t, err.Error(), "get failed")
	})

	t.Run("Close handles watcher close error", func(t *testing.T) {
		c := &component{
			cancel:      func() {},
			kmsgWatcher: &stubWatcher{closeErr: errors.New("close failed")},
			eventBucket: &stubEventBucket{},
		}
		err := c.Close()
		require.NoError(t, err)
	})
}

func TestComponent_start_Branches_WithMockey(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	extraCh := make(chan *eventstore.Event, 2)
	kmsgCh := make(chan kmsg.Message, 2)

	c := &component{
		ctx:          ctx,
		extraEventCh: extraCh,
		eventBucket:  &stubEventBucket{insertErr: errors.New("insert failed")},
		nvmlInstance: &mockNVMLInstance{devices: map[string]device.Device{}},
		devices:      map[string]device.Device{},
	}

	mockey.PatchConvey("start handles nil extra events and non-xid messages", t, func() {
		mockey.Mock((*component).updateCurrentState).To(func(_ *component) error { return nil }).Build()

		done := make(chan struct{})
		go func() {
			c.start(kmsgCh, 5*time.Millisecond)
			close(done)
		}()

		extraCh <- nil
		extraCh <- &eventstore.Event{Name: "test", Time: time.Now()}
		kmsgCh <- kmsg.Message{
			Timestamp: metav1.Now(),
			Message:   "this is not an xid message",
		}
		time.Sleep(20 * time.Millisecond)
		cancel()

		select {
		case <-done:
		case <-time.After(1 * time.Second):
			t.Fatal("component.start did not exit on context cancel")
		}
	})
}

func TestComponent_UpdateCurrentState_RebootErrorBranch_WithMockey(t *testing.T) {
	now := time.Now().UTC()
	c := &component{
		ctx: context.Background(),
		rebootEventStore: &stubRebootStore{
			err: errors.New("reboot events failed"),
		},
		eventBucket: &stubEventBucket{
			getEvents: eventstore.Events{
				{
					Time:    now,
					Name:    EventNameErrorXid,
					Type:    string(apiv1.EventTypeWarning),
					Message: "warn",
				},
			},
		},
		getTimeNowFunc: func() time.Time { return now },
		getThresholdFunc: func() RebootThreshold {
			return RebootThreshold{Threshold: 1}
		},
		devices: map[string]device.Device{},
	}

	err := c.updateCurrentState()
	require.NoError(t, err)
	states := c.LastHealthStates()
	require.Len(t, states, 1)
	assert.Contains(t, states[0].Error, "failed to get reboot events")
}

func TestCheck_EarlyBranches_WithMockey(t *testing.T) {
	now := time.Now().UTC()
	newNVML := func(exists bool, product string, initErr error) *configurableNVML {
		return &configurableNVML{
			mockNVMLInstance: &mockNVMLInstance{
				devices: map[string]device.Device{},
			},
			nvmlExists:  exists,
			productName: product,
			initErr:     initErr,
		}
	}

	t.Run("nvml not loaded", func(t *testing.T) {
		c := &component{
			ctx:          context.Background(),
			nvmlInstance: newNVML(false, "GPU", nil),
			getTimeNowFunc: func() time.Time {
				return now
			},
		}
		cr := c.Check().(*checkResult)
		assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
		assert.Contains(t, cr.reason, "NVML library is not loaded")
	})

	t.Run("nvml init error", func(t *testing.T) {
		c := &component{
			ctx:          context.Background(),
			nvmlInstance: newNVML(true, "GPU", errors.New("init failed")),
			getTimeNowFunc: func() time.Time {
				return now
			},
		}
		cr := c.Check().(*checkResult)
		assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
		assert.Contains(t, cr.reason, "NVML initialization error")
		require.NotNil(t, cr.suggestedActions)
		assert.Contains(t, cr.suggestedActions.RepairActions, apiv1.RepairActionTypeRebootSystem)
	})

	t.Run("missing product name", func(t *testing.T) {
		c := &component{
			ctx:          context.Background(),
			nvmlInstance: newNVML(true, "", nil),
			getTimeNowFunc: func() time.Time {
				return now
			},
		}
		cr := c.Check().(*checkResult)
		assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
		assert.Contains(t, cr.reason, "missing product name")
	})

	t.Run("read kmsg with no xid match", func(t *testing.T) {
		c := &component{
			ctx:          context.Background(),
			nvmlInstance: newNVML(true, "GPU", nil),
			readAllKmsg: func(context.Context) ([]kmsg.Message, error) {
				return []kmsg.Message{
					{
						Timestamp: metav1.Now(),
						Message:   "this line has no xid content",
					},
				}, nil
			},
			getTimeNowFunc: func() time.Time {
				return now
			},
		}
		cr := c.Check().(*checkResult)
		assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
		assert.Equal(t, "matched 0 xid errors from 1 kmsg(s)", cr.reason)
	})
}

func TestStart_RetryContextDoneBranch_WithMockey(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	c := &component{ctx: ctx}

	mockey.PatchConvey("start retries until context is canceled", t, func() {
		mockey.Mock((*component).updateCurrentState).To(func(_ *component) error {
			return errors.New("retry state")
		}).Build()

		errCh := make(chan error, 1)
		go func() {
			errCh <- c.Start()
		}()

		time.Sleep(30 * time.Millisecond)
		cancel()

		select {
		case err := <-errCh:
			require.NoError(t, err)
		case <-time.After(2 * time.Second):
			t.Fatal("Start did not exit after context cancellation")
		}
	})
}

func TestStart_KmsgEventBranches_WithMockey(t *testing.T) {
	run := func(t *testing.T, c *component, msg kmsg.Message) {
		t.Helper()

		kmsgCh := make(chan kmsg.Message, 1)
		done := make(chan struct{})
		go func() {
			c.start(kmsgCh, time.Hour)
			close(done)
		}()

		kmsgCh <- msg
		time.Sleep(40 * time.Millisecond)
		c.cancel()

		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("component.start did not exit")
		}
	}

	xidMsg := kmsg.Message{
		Timestamp: metav1.Now(),
		Message:   "NVRM: Xid (PCI:0000:01:00): 31, pid=123",
	}

	t.Run("find error branch", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		c := &component{
			ctx:          ctx,
			cancel:       cancel,
			nvmlInstance: createMockNVMLInstance(),
			devices:      map[string]device.Device{},
			eventBucket: &stubEventBucket{
				findErr: errors.New("find failed"),
			},
			extraEventCh: make(chan *eventstore.Event, 1),
		}
		run(t, c, xidMsg)
	})

	t.Run("same event branch", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		c := &component{
			ctx:          ctx,
			cancel:       cancel,
			nvmlInstance: createMockNVMLInstance(),
			devices:      map[string]device.Device{},
			eventBucket: &stubEventBucket{
				findEvent: &eventstore.Event{Name: EventNameErrorXid},
			},
			extraEventCh: make(chan *eventstore.Event, 1),
		}
		run(t, c, xidMsg)
	})

	t.Run("insert error branch", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		c := &component{
			ctx:          ctx,
			cancel:       cancel,
			nvmlInstance: createMockNVMLInstance(),
			devices:      map[string]device.Device{},
			eventBucket: &stubEventBucket{
				insertErr: errors.New("insert failed"),
			},
			extraEventCh: make(chan *eventstore.Event, 1),
		}
		run(t, c, xidMsg)
	})

	t.Run("post-insert state update error branch", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		c := &component{
			ctx:          ctx,
			cancel:       cancel,
			nvmlInstance: createMockNVMLInstance(),
			devices:      map[string]device.Device{},
			eventBucket:  &stubEventBucket{},
			extraEventCh: make(chan *eventstore.Event, 1),
		}

		mockey.PatchConvey("updateCurrentState returns error after insert", t, func() {
			mockey.Mock((*component).updateCurrentState).To(func(_ *component) error {
				return errors.New("update failed")
			}).Build()
			run(t, c, xidMsg)
		})
	})
}

func TestNew_ReadAllKmsgSetOnLinuxRoot_WithMockey(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("this branch is only reachable on linux")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mockey.PatchConvey("new sets readAllKmsg when running as root on linux", t, func() {
		mockey.Mock(os.Geteuid).To(func() int { return 0 }).Build()

		comp, err := New(&components.GPUdInstance{
			RootCtx: ctx,
		})
		require.NoError(t, err)
		require.NotNil(t, comp)

		c := comp.(*component)
		require.NotNil(t, c.readAllKmsg)
		_ = c.Close()
	})
}
