package nvlink

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/pkg/eventstore"
	"github.com/leptonai/gpud/pkg/kmsg"
	"github.com/leptonai/gpud/pkg/nvidia/nvml/device"
)

func TestRecordDriverWedgeEventPersistsOnceAndOverridesHealth(t *testing.T) {
	now := time.Date(2026, 7, 11, 4, 21, 32, 0, time.UTC)
	bucket := &memoryEventBucket{}
	c := &component{
		ctx:            context.Background(),
		getTimeNowFunc: func() time.Time { return now },
		eventBucket:    bucket,
		lastCheckResult: &checkResult{
			ts:     now.Add(-time.Minute),
			health: apiv1.HealthStateTypeHealthy,
			reason: "healthy before the driver wedge",
		},
	}
	message := kmsg.Message{
		Timestamp: metav1.NewTime(now),
		Message:   "NVRM: knvlinkDiscoverPostRxDetLinks_GH100: Getting peer0's postRxDetLinkMask failed!",
	}

	c.recordDriverWedgeEvent(message, EventNameDriverWedge, driverWedgeMessage)
	c.recordDriverWedgeEvent(message, EventNameDriverWedge, driverWedgeMessage)

	require.Len(t, bucket.snapshot(), 1)
	assert.Equal(t, EventNameDriverWedge, bucket.snapshot()[0].Name)
	state := c.LastHealthStates()[0]
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, state.Health)
	assert.Equal(t, driverWedgeMessage, state.Reason)
	require.NotNil(t, state.SuggestedActions)
	assert.Equal(t, []apiv1.RepairActionType{apiv1.RepairActionTypeRebootSystem}, state.SuggestedActions.RepairActions)
}

func TestRecordDriverWedgeEventRetriesPersistence(t *testing.T) {
	now := time.Date(2026, 7, 11, 4, 21, 32, 0, time.UTC)
	bucket := &memoryEventBucket{insertFailures: 1}
	c := &component{
		ctx:            context.Background(),
		getTimeNowFunc: func() time.Time { return now },
		eventBucket:    bucket,
	}
	message := kmsg.Message{Timestamp: metav1.NewTime(now)}

	c.recordDriverWedgeEvent(message, EventNameDriverWedge, driverWedgeMessage)
	c.recordDriverWedgeEvent(message, EventNameDriverWedge, driverWedgeMessage)

	require.Len(t, bucket.snapshot(), 1)
	assert.True(t, c.driverWedgeEventPersisted)
}

func TestUpdateCurrentStateRestoresDriverWedgeFromCurrentBoot(t *testing.T) {
	now := time.Date(2026, 7, 15, 3, 0, 0, 0, time.UTC)
	bootTime := now.Add(-24 * time.Hour)
	bucket := &memoryEventBucket{events: eventstore.Events{
		{Time: bootTime.Add(-time.Hour), Name: EventNameDriverWedge, Message: "old boot"},
		{Time: bootTime.Add(time.Hour), Name: EventNameDriverWedge, Message: driverWedgeMessage},
	}}
	c := &component{
		ctx:             context.Background(),
		getTimeNowFunc:  func() time.Time { return now },
		getBootTimeFunc: func() time.Time { return bootTime },
		eventBucket:     bucket,
	}

	require.NoError(t, c.updateCurrentState())

	state := c.LastHealthStates()[0]
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, state.Health)
	assert.Equal(t, driverWedgeMessage, state.Reason)
	assert.Equal(t, bootTime.Add(time.Hour), state.Time.Time)
}

func TestUpdateCurrentStateIgnoresPreviousBoot(t *testing.T) {
	now := time.Date(2026, 7, 15, 3, 0, 0, 0, time.UTC)
	bootTime := now.Add(-24 * time.Hour)
	bucket := &memoryEventBucket{events: eventstore.Events{
		{Time: bootTime.Add(-time.Hour), Name: EventNameDriverWedge, Message: driverWedgeMessage},
	}}
	c := &component{
		ctx:             context.Background(),
		getTimeNowFunc:  func() time.Time { return now },
		getBootTimeFunc: func() time.Time { return bootTime },
		eventBucket:     bucket,
	}

	require.NoError(t, c.updateCurrentState())
	assert.Equal(t, apiv1.HealthStateTypeHealthy, c.LastHealthStates()[0].Health)
}

func TestBlockedNVMLCheckBecomesUnhealthyWithoutReplacement(t *testing.T) {
	now := time.Date(2026, 7, 15, 3, 0, 0, 0, time.UTC)
	var clock atomic.Int64
	clock.Store(now.UnixNano())
	entered := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	releaseCheck := func() { releaseOnce.Do(func() { close(release) }) }
	defer releaseCheck()
	componentAny := MockNVLinkComponent(
		context.Background(),
		func() map[string]device.Device { return map[string]device.Device{"gpu-0": nil} },
		func(_ string, _ device.Device) (NVLink, error) {
			close(entered)
			<-release
			return NVLink{}, nil
		},
	)
	c := mustComponent(t, componentAny)
	c.getTimeNowFunc = func() time.Time { return time.Unix(0, clock.Load()).UTC() }
	done := make(chan struct{})
	go func() {
		_ = c.Check()
		close(done)
	}()
	<-entered
	clock.Store(now.Add(defaultCheckStaleAfter).UnixNano())

	state := c.LastHealthStates()[0]
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, state.Health)
	assert.Contains(t, state.Reason, "has not completed")
	require.NotNil(t, state.SuggestedActions)
	assert.Equal(t, []apiv1.RepairActionType{apiv1.RepairActionTypeRebootSystem}, state.SuggestedActions.RepairActions)
	c.lastMu.RLock()
	assert.Len(t, c.checksInFlight, 1)
	c.lastMu.RUnlock()

	releaseCheck()
	<-done
	assert.Equal(t, apiv1.HealthStateTypeHealthy, c.LastHealthStates()[0].Health)
}

func TestWatchdogTracksOnlyCurrentlyRunningChecks(t *testing.T) {
	now := time.Date(2026, 7, 15, 3, 0, 0, 0, time.UTC)
	c := &component{getTimeNowFunc: func() time.Time { return now }}
	completed := &checkResult{ts: now.Add(-3 * time.Minute), health: apiv1.HealthStateTypeHealthy}
	running := &checkResult{ts: now.Add(-time.Minute), health: apiv1.HealthStateTypeHealthy}
	c.beginCheck(completed)
	c.beginCheck(running)
	c.finishCheck(completed)

	assert.Equal(t, apiv1.HealthStateTypeHealthy, c.LastHealthStates()[0].Health)

	c.finishCheck(running)
}

func TestStaleCompletedHealthStateFailsAfterMonitoringStarts(t *testing.T) {
	now := time.Date(2026, 7, 15, 3, 0, 0, 0, time.UTC)
	c := &component{
		getTimeNowFunc: func() time.Time { return now },
		lastCheckResult: &checkResult{
			ts:     now.Add(-3 * time.Minute),
			health: apiv1.HealthStateTypeHealthy,
		},
		monitoringStartedAt:  now.Add(-4 * time.Minute),
		lastCheckCompletedAt: now.Add(-defaultCheckStaleAfter),
	}

	state := c.LastHealthStates()[0]
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, state.Health)
	assert.Contains(t, state.Reason, "has not refreshed")
}

func TestStartProcessesDriverWedgeEvent(t *testing.T) {
	now := time.Date(2026, 7, 11, 4, 21, 32, 0, time.UTC)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	kmsgCh := make(chan kmsg.Message, 1)
	c := &component{
		ctx:            ctx,
		getTimeNowFunc: func() time.Time { return now },
	}

	go c.start(kmsgCh)
	kmsgCh <- kmsg.Message{
		Timestamp: metav1.NewTime(now),
		Message:   "NVRM: knvlinkDiscoverPostRxDetLinks_GH100: Getting peer0's postRxDetLinkMask failed!",
	}
	require.Eventually(t, func() bool {
		return c.LastHealthStates()[0].Health == apiv1.HealthStateTypeUnhealthy
	}, time.Second, 10*time.Millisecond)
}

type memoryEventBucket struct {
	mu             sync.Mutex
	events         eventstore.Events
	insertFailures int
}

func (b *memoryEventBucket) Name() string { return Name }

func (b *memoryEventBucket) Insert(_ context.Context, event eventstore.Event) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.insertFailures > 0 {
		b.insertFailures--
		return errors.New("injected insert failure")
	}
	b.events = append(b.events, event)
	return nil
}

func (b *memoryEventBucket) Find(_ context.Context, _ eventstore.Event) (*eventstore.Event, error) {
	return nil, nil
}

func (b *memoryEventBucket) Get(_ context.Context, since time.Time) (eventstore.Events, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	var events eventstore.Events
	for _, event := range b.events {
		if event.Time.After(since) {
			events = append(events, event)
		}
	}
	return events, nil
}

func (b *memoryEventBucket) Latest(_ context.Context) (*eventstore.Event, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.events) == 0 {
		return nil, nil
	}
	event := b.events[len(b.events)-1]
	return &event, nil
}

func (b *memoryEventBucket) Purge(_ context.Context, beforeTimestamp int64) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	kept := b.events[:0]
	for _, event := range b.events {
		if event.Time.Unix() >= beforeTimestamp {
			kept = append(kept, event)
		}
	}
	purged := len(b.events) - len(kept)
	b.events = kept
	return purged, nil
}

func (b *memoryEventBucket) Close() {}

func (b *memoryEventBucket) snapshot() eventstore.Events {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append(eventstore.Events(nil), b.events...)
}

var _ eventstore.Bucket = (*memoryEventBucket)(nil)
