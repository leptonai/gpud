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

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/pkg/eventstore"
	"github.com/leptonai/gpud/pkg/kmsg"
	"github.com/leptonai/gpud/pkg/nvidia/nvml/device"
)

func TestUpdateCurrentStateRestoresPostRxDetectFailureFromCurrentBoot(t *testing.T) {
	now := time.Date(2026, 7, 15, 3, 0, 0, 0, time.UTC)
	bootTime := now.Add(-24 * time.Hour)
	bucket := &memoryEventBucket{events: eventstore.Events{
		{Time: bootTime.Add(-time.Hour), Name: EventNamePostRxDetectFailure, Message: "old boot"},
		{Time: bootTime.Add(time.Hour), Name: EventNamePostRxDetectFailure, Message: postRxDetectFailureMessage + " (boot ID: boot-1)"},
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
	assert.Equal(t, postRxDetectFailureMessage, state.Reason)
	assert.Equal(t, bootTime.Add(time.Hour), state.Time.Time)
}

func TestUpdateCurrentStateIgnoresPreviousBoot(t *testing.T) {
	now := time.Date(2026, 7, 15, 3, 0, 0, 0, time.UTC)
	bootTime := now.Add(-24 * time.Hour)
	bucket := &memoryEventBucket{events: eventstore.Events{
		{Time: bootTime.Add(-time.Hour), Name: EventNamePostRxDetectFailure, Message: postRxDetectFailureMessage},
		{Time: bootTime.Add(time.Hour), Name: "unrelated-event", Message: "ignored"},
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

func TestUpdateCurrentStateHandlesUnavailableState(t *testing.T) {
	now := time.Date(2026, 7, 15, 3, 0, 0, 0, time.UTC)

	tests := []struct {
		name     string
		bucket   eventstore.Bucket
		bootTime time.Time
		wantErr  string
	}{
		{
			name:     "event bucket is disabled",
			bootTime: now.Add(-time.Hour),
		},
		{
			name:     "boot time is invalid",
			bucket:   &memoryEventBucket{},
			bootTime: time.Time{},
		},
		{
			name:     "boot time is in the future",
			bucket:   &memoryEventBucket{},
			bootTime: now.Add(time.Hour),
		},
		{
			name:     "event query fails",
			bucket:   &memoryEventBucket{getErr: errors.New("query failed")},
			bootTime: now.Add(-time.Hour),
			wantErr:  "query failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &component{
				ctx:             context.Background(),
				getTimeNowFunc:  func() time.Time { return now },
				getBootTimeFunc: func() time.Time { return tt.bootTime },
				eventBucket:     tt.bucket,
			}

			err := c.updateCurrentState()
			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
		})
	}
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

func TestHealthStateFailsWhenNoCheckCompletesAfterMonitoringStarts(t *testing.T) {
	now := time.Date(2026, 7, 15, 3, 0, 0, 0, time.UTC)
	c := &component{
		getTimeNowFunc:      func() time.Time { return now },
		monitoringStartedAt: now.Add(-defaultCheckStaleAfter),
	}

	state := c.LastHealthStates()[0]
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, state.Health)
	assert.Contains(t, state.Reason, "has not refreshed")
}

func TestCheckDetectsPostRxDetectFailureForOneShotScan(t *testing.T) {
	getNVLinkCalled := false
	c := mustComponent(t, MockNVLinkComponent(
		context.Background(),
		func() map[string]device.Device { return map[string]device.Device{"gpu-0": nil} },
		func(_ string, _ device.Device) (NVLink, error) {
			getNVLinkCalled = true
			return NVLink{}, nil
		},
	))
	c.readAllKmsg = func(context.Context) ([]kmsg.Message, error) {
		return []kmsg.Message{{
			Message: "NVRM: knvlinkDiscoverPostRxDetLinks_GH100: Getting peer0's postRxDetLinkMask failed!",
		}}, nil
	}

	result := c.Check()
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, result.HealthStateType())
	assert.Contains(t, result.Summary(), "scanned kmsg(s)")
	assert.Contains(t, result.Summary(), postRxDetectFailureMessage)
	assert.Equal(t, "matched 1 kmsg(s)", result.String())
	assert.False(t, getNVLinkCalled, "kmsg failure should short-circuit NVML probing")
	state := result.HealthStates()[0]
	require.NotNil(t, state.SuggestedActions)
	assert.Equal(t, []apiv1.RepairActionType{apiv1.RepairActionTypeRebootSystem}, state.SuggestedActions.RepairActions)
}

func TestCheckHandlesOneShotKmsgReadResult(t *testing.T) {
	tests := []struct {
		name       string
		readAll    func(context.Context) ([]kmsg.Message, error)
		wantHealth apiv1.HealthStateType
		wantReason string
		wantString string
	}{
		{
			name: "read failure",
			readAll: func(context.Context) ([]kmsg.Message, error) {
				return nil, errors.New("read failed")
			},
			wantHealth: apiv1.HealthStateTypeUnhealthy,
			wantReason: "failed to read kmsg",
			wantString: "no data",
		},
		{
			name: "no matching message",
			readAll: func(context.Context) ([]kmsg.Message, error) {
				return []kmsg.Message{{Message: "unrelated kernel message"}}, nil
			},
			wantHealth: apiv1.HealthStateTypeHealthy,
			wantReason: "all 0 GPU(s) were checked, no nvlink issue found",
			wantString: "matched 0 kmsg(s)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := mustComponent(t, MockNVLinkComponent(
				context.Background(),
				func() map[string]device.Device { return map[string]device.Device{} },
				nil,
			))
			c.readAllKmsg = tt.readAll

			result := c.Check()
			assert.Equal(t, tt.wantHealth, result.HealthStateType())
			assert.Contains(t, result.Summary(), tt.wantReason)
			assert.Equal(t, tt.wantString, result.String())
		})
	}
}

type memoryEventBucket struct {
	mu     sync.Mutex
	events eventstore.Events
	getErr error
	closed atomic.Bool
}

func (b *memoryEventBucket) Name() string { return Name }

func (b *memoryEventBucket) Insert(_ context.Context, event eventstore.Event) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.events = append(b.events, event)
	return nil
}

func (b *memoryEventBucket) Find(_ context.Context, _ eventstore.Event) (*eventstore.Event, error) {
	return nil, nil
}

func (b *memoryEventBucket) Get(_ context.Context, since time.Time) (eventstore.Events, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.getErr != nil {
		return nil, b.getErr
	}
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

func (b *memoryEventBucket) Close() { b.closed.Store(true) }

var _ eventstore.Bucket = (*memoryEventBucket)(nil)

type memoryEventStore struct {
	bucket eventstore.Bucket
	err    error
}

func (s *memoryEventStore) Bucket(_ string, _ ...eventstore.OpOption) (eventstore.Bucket, error) {
	return s.bucket, s.err
}

var _ eventstore.Store = (*memoryEventStore)(nil)
