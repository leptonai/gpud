//nolint:forcetypeassert,revive // tests intentionally inspect concrete types; package name follows the directory import path.
package os

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	procs "github.com/shirou/gopsutil/v4/process"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/client_golang/prometheus"
	prometheusdto "github.com/prometheus/client_model/go"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/eventstore"
	pkghost "github.com/leptonai/gpud/pkg/host"
	"github.com/leptonai/gpud/pkg/process"
)

// mockProcessStatus implements process.ProcessStatus for tests.
type mockProcessStatus struct {
	pid     int32
	name    string
	nameErr error
}

func (m *mockProcessStatus) Name() (string, error)     { return m.name, m.nameErr }
func (m *mockProcessStatus) PID() int32                { return m.pid }
func (m *mockProcessStatus) Status() ([]string, error) { return []string{procs.Blocked}, nil }

func blockedList(procs ...process.ProcessStatus) []process.ProcessStatus {
	return procs
}

// mustBlockedProcessThresholds builds thresholds with compiled regexes for tests.
func mustBlockedProcessThresholds(t *testing.T, threshold int, regexes ...string) BlockedProcessThresholds {
	t.Helper()
	th, err := newBlockedProcessThresholds(threshold, regexes)
	require.NoError(t, err)
	return th
}

// withBlockedProcessThresholds makes the component read the given thresholds
// on every check, simulating a config applied via flags or session
// updateConfig.
func withBlockedProcessThresholds(comp *component, th BlockedProcessThresholds) {
	comp.getBlockedProcessThresholdsFunc = func() BlockedProcessThresholds { return th }
}

// recordingBucket records inserted events and serves seeded events.
type recordingBucket struct {
	mu        sync.Mutex
	inserted  []eventstore.Event
	getEvents eventstore.Events
	getError  error
}

func (m *recordingBucket) Name() string { return "mock-bucket" }
func (m *recordingBucket) Insert(_ context.Context, event eventstore.Event) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.inserted = append(m.inserted, event)
	return nil
}
func (m *recordingBucket) Find(_ context.Context, _ eventstore.Event) (*eventstore.Event, error) {
	return nil, nil
}
func (m *recordingBucket) Get(_ context.Context, _ time.Time) (eventstore.Events, error) {
	if m.getError != nil {
		return nil, m.getError
	}
	// serve inserted events as a real bucket would, plus any seeded events
	m.mu.Lock()
	defer m.mu.Unlock()
	ret := make(eventstore.Events, 0, len(m.getEvents)+len(m.inserted))
	ret = append(ret, m.getEvents...)
	ret = append(ret, m.inserted...)
	return ret, nil
}
func (m *recordingBucket) Latest(_ context.Context) (*eventstore.Event, error) { return nil, nil }
func (m *recordingBucket) Purge(_ context.Context, _ int64) (int, error)       { return 0, nil }
func (m *recordingBucket) Close()                                              {}

func (m *recordingBucket) insertedNames() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	names := make([]string, 0, len(m.inserted))
	for _, ev := range m.inserted {
		names = append(names, ev.Name)
	}
	return names
}

func (m *recordingBucket) insertedEvents() []eventstore.Event {
	m.mu.Lock()
	defer m.mu.Unlock()
	ret := make([]eventstore.Event, len(m.inserted))
	copy(ret, m.inserted)
	return ret
}

func TestBlockedProcessTracker_NoBlocked(t *testing.T) {
	tr := newBlockedProcessTracker()
	now := time.Unix(1000, 0)
	for i := 0; i < 10; i++ {
		count, persistent := tr.update(now.Add(time.Duration(i)*time.Minute), nil, 5)
		assert.Zero(t, count)
		assert.Empty(t, persistent)
	}
}

func TestBlockedProcessTracker_TransientNotFlagged(t *testing.T) {
	tr := newBlockedProcessTracker()
	now := time.Unix(1000, 0)

	// process blocked for 4 consecutive checks (< threshold of 5)
	for i := 0; i < 4; i++ {
		count, persistent := tr.update(now.Add(time.Duration(i)*time.Minute), blockedList(&mockProcessStatus{pid: 100, name: "nvidia-smi"}), 5)
		assert.Zero(t, count, "check %d must not be persistent yet", i)
		assert.Empty(t, persistent)
	}

	// then disappears for good (2 consecutive absences drop the entry);
	// when a process with the same PID appears later, the counter restarts
	tr.update(now.Add(4*time.Minute), nil, 5)
	tr.update(now.Add(5*time.Minute), nil, 5)
	for i := 6; i < 10; i++ {
		count, persistent := tr.update(now.Add(time.Duration(i)*time.Minute), blockedList(&mockProcessStatus{pid: 100, name: "nvidia-smi"}), 5)
		assert.Zero(t, count, "check %d must restart counting after recovery", i)
		assert.Empty(t, persistent)
	}
}

func TestBlockedProcessTracker_Persistent(t *testing.T) {
	tr := newBlockedProcessTracker()
	now := time.Unix(1000, 0)

	var count int
	var persistent []BlockedProcess
	for i := 0; i < 5; i++ {
		count, persistent = tr.update(now.Add(time.Duration(i)*time.Minute), blockedList(&mockProcessStatus{pid: 100, name: "nvidia-smi"}), 5)
	}
	require.Equal(t, 1, count)
	require.Len(t, persistent, 1)

	bp := persistent[0]
	assert.Equal(t, int32(100), bp.PID)
	assert.Equal(t, "nvidia-smi", bp.Name)
	assert.Equal(t, now.Unix(), bp.FirstSeenUnixSeconds)
	assert.Equal(t, now.Add(4*time.Minute).Unix(), bp.LastSeenUnixSeconds)
	assert.Equal(t, int64(240), bp.BlockedSeconds)
	assert.Equal(t, 5, bp.ConsecutiveChecks)

	// keeps being reported on the 6th check
	count, persistent = tr.update(now.Add(5*time.Minute), blockedList(&mockProcessStatus{pid: 100, name: "nvidia-smi"}), 5)
	require.Equal(t, 1, count)
	assert.Equal(t, 6, persistent[0].ConsecutiveChecks)
}

func TestBlockedProcessTracker_ThresholdOneFlagsImmediately(t *testing.T) {
	tr := newBlockedProcessTracker()
	now := time.Unix(1000, 0)

	// threshold 1 (one-off gpud scan): a single observation is persistent
	count, persistent := tr.update(now, blockedList(&mockProcessStatus{pid: 100, name: "nvidia-smi"}), 1)
	require.Equal(t, 1, count)
	require.Len(t, persistent, 1)
	assert.Equal(t, 1, persistent[0].ConsecutiveChecks)
	assert.Equal(t, int64(0), persistent[0].BlockedSeconds)
}

func TestBlockedProcessTracker_NonPositiveThresholdFallsBackToDefault(t *testing.T) {
	tr := newBlockedProcessTracker()
	now := time.Unix(1000, 0)

	// threshold 0 must behave as DefaultBlockedProcessPersistenceThreshold
	for i := 0; i < DefaultBlockedProcessPersistenceThreshold-1; i++ {
		count, _ := tr.update(now.Add(time.Duration(i)*time.Minute), blockedList(&mockProcessStatus{pid: 100, name: "nvidia-smi"}), 0)
		assert.Zero(t, count)
	}
	count, persistent := tr.update(now.Add((DefaultBlockedProcessPersistenceThreshold-1)*time.Minute), blockedList(&mockProcessStatus{pid: 100, name: "nvidia-smi"}), 0)
	require.Equal(t, 1, count)
	require.Len(t, persistent, 1)
}

func TestBlockedProcessTracker_Reset(t *testing.T) {
	tr := newBlockedProcessTracker()
	now := time.Unix(1000, 0)

	for i := 0; i < 5; i++ {
		tr.update(now.Add(time.Duration(i)*time.Minute), blockedList(&mockProcessStatus{pid: 100, name: "nvidia-smi"}), 5)
	}
	count, _ := tr.update(now.Add(5*time.Minute), blockedList(&mockProcessStatus{pid: 100, name: "nvidia-smi"}), 5)
	require.Equal(t, 1, count)

	tr.reset()

	// after reset the same process must re-earn the persistence threshold
	for i := 6; i < 10; i++ {
		count, _ := tr.update(now.Add(time.Duration(i)*time.Minute), blockedList(&mockProcessStatus{pid: 100, name: "nvidia-smi"}), 5)
		assert.Zero(t, count, "check %d after reset must not be persistent yet", i)
	}
	count, persistent := tr.update(now.Add(10*time.Minute), blockedList(&mockProcessStatus{pid: 100, name: "nvidia-smi"}), 5)
	require.Equal(t, 1, count)
	require.Len(t, persistent, 1)
	assert.Equal(t, 5, persistent[0].ConsecutiveChecks)
	assert.Equal(t, now.Add(6*time.Minute).Unix(), persistent[0].FirstSeenUnixSeconds,
		"first-seen must restart from the post-reset observation")
}

func TestBlockedProcessTracker_SingleCheckAbsenceDoesNotReset(t *testing.T) {
	tr := newBlockedProcessTracker()
	now := time.Unix(1000, 0)

	// present for 4 checks
	for i := 0; i < 4; i++ {
		tr.update(now.Add(time.Duration(i)*time.Minute), blockedList(&mockProcessStatus{pid: 100, name: "nvidia-smi"}), 5)
	}
	// absent for 1 check (PID churn / transient /proc read error)
	count, persistent := tr.update(now.Add(4*time.Minute), nil, 5)
	assert.Zero(t, count)
	assert.Empty(t, persistent)

	// present again: consecutive counter must NOT have reset; this is the 5th
	// consecutive observation and must become persistent
	count, persistent = tr.update(now.Add(5*time.Minute), blockedList(&mockProcessStatus{pid: 100, name: "nvidia-smi"}), 5)
	require.Equal(t, 1, count, "single-check absence must not reset persistence")
	require.Len(t, persistent, 1)
	assert.Equal(t, 5, persistent[0].ConsecutiveChecks)
}

func TestBlockedProcessTracker_RecoveryAfterTwoAbsences(t *testing.T) {
	tr := newBlockedProcessTracker()
	now := time.Unix(1000, 0)

	for i := 0; i < 5; i++ {
		tr.update(now.Add(time.Duration(i)*time.Minute), blockedList(&mockProcessStatus{pid: 100, name: "nvidia-smi"}), 5)
	}
	count, _ := tr.update(now.Add(5*time.Minute), blockedList(&mockProcessStatus{pid: 100, name: "nvidia-smi"}), 5)
	require.Equal(t, 1, count)

	// one absent check: still tracked and still reported persistent
	count, _ = tr.update(now.Add(6*time.Minute), nil, 5)
	assert.Equal(t, 1, count, "single absence is within grace and must not clear the condition")

	// two consecutive absent checks: dropped (recovered)
	count, persistent := tr.update(now.Add(7*time.Minute), nil, 5)
	assert.Zero(t, count)
	assert.Empty(t, persistent)

	// stays cleared
	count, _ = tr.update(now.Add(8*time.Minute), nil, 5)
	assert.Zero(t, count)
}

func TestBlockedProcessTracker_BurstChecksDoNotInflatePersistence(t *testing.T) {
	tr := newBlockedProcessTracker()
	now := time.Unix(1000, 0)

	// 10 checks only 1s apart (out-of-band trigger burst via
	// /v1/components/trigger-check or session triggerComponent): the
	// consecutive count reaches 10, but the wall-clock duration gate
	// (>= 4min for threshold 5) must not flag the process as persistent
	for i := 0; i < 10; i++ {
		count, persistent := tr.update(now.Add(time.Duration(i)*time.Second), blockedList(&mockProcessStatus{pid: 100, name: "nvidia-smi"}), 5)
		assert.Zero(t, count, "burst check %d must not be persistent", i)
		assert.Empty(t, persistent)
	}

	// once the wall-clock duration is reached, it fires
	count, persistent := tr.update(now.Add(4*time.Minute), blockedList(&mockProcessStatus{pid: 100, name: "nvidia-smi"}), 5)
	require.Equal(t, 1, count)
	require.Len(t, persistent, 1)
	assert.Equal(t, 11, persistent[0].ConsecutiveChecks)
}

func TestBlockedProcessTracker_NameErrorTolerated(t *testing.T) {
	tr := newBlockedProcessTracker()
	now := time.Unix(1000, 0)

	// name read fails (e.g., process disappeared between enumeration and metadata read)
	for i := 0; i < 5; i++ {
		tr.update(now.Add(time.Duration(i)*time.Minute), blockedList(&mockProcessStatus{pid: 100, name: "", nameErr: errors.New("no such file")}), 5)
	}
	count, persistent := tr.update(now.Add(5*time.Minute), blockedList(&mockProcessStatus{pid: 100, name: "", nameErr: errors.New("no such file")}), 5)
	require.Equal(t, 1, count)
	require.Len(t, persistent, 1)
	assert.Equal(t, "", persistent[0].Name, "name error must be tolerated and recorded as empty")
}

func TestBlockedProcessTracker_NilEntriesSkipped(t *testing.T) {
	tr := newBlockedProcessTracker()
	now := time.Unix(1000, 0)
	for i := 0; i < 6; i++ {
		count, _ := tr.update(now.Add(time.Duration(i)*time.Minute), blockedList(nil, &mockProcessStatus{pid: 100, name: "nvidia-smi"}, nil), 5)
		if i >= 4 {
			assert.Equal(t, 1, count)
		}
	}
}

func TestBlockedProcessTracker_BoundedOutput(t *testing.T) {
	tr := newBlockedProcessTracker()
	now := time.Unix(1000, 0)

	many := make([]process.ProcessStatus, 0, maxBlockedProcessesOutput+5)
	for i := 0; i < maxBlockedProcessesOutput+5; i++ {
		many = append(many, &mockProcessStatus{pid: int32(1000 + i), name: fmt.Sprintf("proc-%d", i)})
	}

	var count int
	var persistent []BlockedProcess
	for i := 0; i < 5; i++ {
		count, persistent = tr.update(now.Add(time.Duration(i)*time.Minute), many, 5)
	}
	assert.Equal(t, maxBlockedProcessesOutput+5, count, "true persistent count must exceed the bounded list")
	assert.Len(t, persistent, maxBlockedProcessesOutput, "output list must be bounded")

	// sorted by first-seen (same ts here) then PID
	for i := 1; i < len(persistent); i++ {
		assert.Less(t, persistent[i-1].PID, persistent[i].PID)
	}
}

// newTestComponent builds an os component with all external functions mocked
// and a controllable clock.
func newTestComponent(t *testing.T, ctx context.Context, rebootStore pkghost.RebootEventStore, bucket eventstore.Bucket) (*component, *time.Time) {
	t.Helper()

	c, err := New(&components.GPUdInstance{
		RootCtx:          ctx,
		RebootEventStore: rebootStore,
	})
	require.NoError(t, err)
	comp, ok := c.(*component)
	require.True(t, ok)

	if bucket != nil {
		comp.eventBucket = bucket
	}

	now := time.Unix(100000, 0).UTC()
	comp.getTimeNowFunc = func() time.Time { return now }

	comp.getHostUptimeFunc = func(ctx context.Context) (uint64, error) { return 1000, nil }
	comp.getFileHandlesFunc = func() (uint64, uint64, error) { return 1000, 0, nil }
	comp.countRunningPIDsFunc = func() (uint64, error) { return 100, nil }
	comp.getUsageFunc = func() (uint64, error) { return 1000, nil }
	comp.getLimitFunc = func() (uint64, error) { return 1000000, nil }
	comp.checkFileHandlesSupportedFunc = func() bool { return true }
	comp.checkFDLimitSupportedFunc = func() bool { return true }

	return comp, &now
}

// runChecks runs comp.Check() n times, advancing the mock clock by one minute
// between checks (mirroring the 1-minute check interval).
func runChecks(comp *component, now *time.Time, n int, blocked []process.ProcessStatus) components.CheckResult {
	comp.countProcessesByStatusFunc = func(ctx context.Context) (map[string][]process.ProcessStatus, error) {
		m := map[string][]process.ProcessStatus{
			procs.Running: make([]process.ProcessStatus, 10),
		}
		if len(blocked) > 0 {
			m[procs.Blocked] = blocked
		}
		return m, nil
	}

	var cr components.CheckResult
	for i := 0; i < n; i++ {
		cr = comp.Check()
		*now = now.Add(time.Minute)
	}
	return cr
}

func TestNewComponent_ReadsDefaultThresholds(t *testing.T) {
	original := GetDefaultBlockedProcessThresholds()
	t.Cleanup(func() {
		require.NoError(t, SetDefaultBlockedProcessThresholds(original))
	})

	require.NoError(t, SetDefaultBlockedProcessThresholds(BlockedProcessThresholds{
		PersistenceThreshold: 3,
		NameRegexes:          []string{"^dd$"},
	}))

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	comp, _ := newTestComponent(t, ctx, &mockRebootEventStore{}, nil)
	defer func() { _ = comp.Close() }()

	got := comp.getBlockedProcessThresholdsFunc()
	assert.Equal(t, 3, got.PersistenceThreshold)
	assert.Equal(t, []string{"^dd$"}, got.NameRegexes)
	assert.True(t, got.MatchesName("dd"))
	assert.False(t, got.MatchesName("nvidia-smi"))
}

// TestCheck_BlockedProcesses_RuntimeConfigUpdate verifies that thresholds
// updated at runtime (e.g., via session updateConfig for node-group config)
// take effect on the running component without recreating it.
func TestCheck_BlockedProcesses_RuntimeConfigUpdate(t *testing.T) {
	original := GetDefaultBlockedProcessThresholds()
	t.Cleanup(func() {
		require.NoError(t, SetDefaultBlockedProcessThresholds(original))
	})
	// start from the built-in default: D-state checking disabled
	require.NoError(t, SetDefaultBlockedProcessThresholds(BlockedProcessThresholds{}))

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	comp, now := newTestComponent(t, ctx, &mockRebootEventStore{}, &recordingBucket{})
	defer func() { _ = comp.Close() }()

	cr := runChecks(comp, now, 3, blockedList(&mockProcessStatus{pid: 42, name: "nvidia-smi"}))
	require.Equal(t, apiv1.HealthStateTypeHealthy, cr.HealthStateType(),
		"disabled D-state check must not react to blocked processes")

	// simulate a session updateConfig push enabling NVIDIA D-state detection
	require.NoError(t, SetDefaultBlockedProcessThresholds(BlockedProcessThresholds{
		PersistenceThreshold: 1,
		NameRegexes:          []string{"^nvidia"},
	}))

	cr = runChecks(comp, now, 1, blockedList(&mockProcessStatus{pid: 42, name: "nvidia-smi"}))
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.HealthStateType(),
		"runtime-updated thresholds must apply on the next check")
}

func TestCheck_BlockedProcesses_DisabledWithoutNameRegexes(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	bucket := &recordingBucket{}
	comp, now := newTestComponent(t, ctx, &mockRebootEventStore{}, bucket)
	defer func() { _ = comp.Close() }()

	// built-in default: empty regexes disable the D-state check entirely
	require.True(t, comp.getBlockedProcessThresholdsFunc().IsZero())

	cr := runChecks(comp, now, 6, blockedList(&mockProcessStatus{pid: 42, name: "nvidia-smi"}))
	assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.HealthStateType(),
		"disabled D-state check must never affect health")
	assert.Zero(t, comp.lastCheckResult.BlockedProcesses.CurrentCount)
	assert.Zero(t, comp.lastCheckResult.BlockedProcesses.PersistentCount)
	assert.Empty(t, bucket.insertedNames(), "disabled D-state check must not emit events")
}

func TestCheck_BlockedProcesses_TransientStaysHealthy(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	comp, now := newTestComponent(t, ctx, &mockRebootEventStore{}, nil)
	defer func() { _ = comp.Close() }()
	withBlockedProcessThresholds(comp, mustBlockedProcessThresholds(t, 5, "^nvidia"))

	// 4 consecutive checks with a blocked nvidia-smi (< threshold 5)
	cr := runChecks(comp, now, 4, blockedList(&mockProcessStatus{pid: 42, name: "nvidia-smi"}))
	assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.HealthStateType(), "transient D-state must not change health")
	assert.Zero(t, comp.lastCheckResult.BlockedProcesses.PersistentCount)
	assert.Equal(t, 1, comp.lastCheckResult.BlockedProcesses.CurrentCount)
}

func TestCheck_BlockedProcesses_PersistentUnmatchedDegraded(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	bucket := &recordingBucket{}
	comp, now := newTestComponent(t, ctx, &mockRebootEventStore{}, bucket)
	defer func() { _ = comp.Close() }()
	withBlockedProcessThresholds(comp, mustBlockedProcessThresholds(t, 5, "^nvidia"))

	// 5 consecutive checks with a blocked process NOT matching escalation regexes
	cr := runChecks(comp, now, 5, blockedList(&mockProcessStatus{pid: 42, name: "dd"}))
	assert.Equal(t, apiv1.HealthStateTypeDegraded, cr.HealthStateType())
	assert.Contains(t, cr.Summary(), "persistent D-state")
	assert.Contains(t, cr.Summary(), "name=dd")
	assert.Equal(t, 1, comp.lastCheckResult.BlockedProcesses.PersistentCount)

	// no repair suggestion for unmatched processes (LEP-6029 initial rollout)
	states := cr.HealthStates()
	require.Len(t, states, 1)
	assert.Nil(t, states[0].SuggestedActions)

	// episode start event recorded exactly once (rising edge), with a
	// non-empty message (regression: message was set before reason existed)
	names := bucket.insertedNames()
	assert.Equal(t, []string{EventNameBlockedProcessesPersistent}, names)
	events := bucket.insertedEvents()
	require.Len(t, events, 1)
	assert.NotEmpty(t, events[0].Message)
	assert.Contains(t, events[0].Message, "name=dd")
}

func TestCheck_BlockedProcesses_PersistentMatchedUnhealthyReboot(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	bucket := &recordingBucket{}
	comp, now := newTestComponent(t, ctx, &mockRebootEventStore{}, bucket)
	defer func() { _ = comp.Close() }()
	withBlockedProcessThresholds(comp, mustBlockedProcessThresholds(t, 5, "^nvidia"))

	cr := runChecks(comp, now, 5, blockedList(&mockProcessStatus{pid: 42, name: "nvidia-smi"}))
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.HealthStateType())
	assert.Contains(t, cr.Summary(), "nvidia-smi")

	states := cr.HealthStates()
	require.Len(t, states, 1)
	require.NotNil(t, states[0].SuggestedActions)
	assert.Equal(t, []apiv1.RepairActionType{apiv1.RepairActionTypeRebootSystem}, states[0].SuggestedActions.RepairActions)
}

func TestCheck_BlockedProcesses_ThresholdOneFlagsImmediately(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	comp, now := newTestComponent(t, ctx, &mockRebootEventStore{}, &recordingBucket{})
	defer func() { _ = comp.Close() }()
	// one-off gpud scan semantics: a single observation flags unhealthy
	withBlockedProcessThresholds(comp, mustBlockedProcessThresholds(t, 1, "^nvidia"))

	cr := runChecks(comp, now, 1, blockedList(&mockProcessStatus{pid: 42, name: "nvidia-smi"}))
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.HealthStateType(),
		"threshold 1 must flag a currently blocked nvidia process on the first check")
	assert.Contains(t, cr.Summary(), "nvidia-smi")

	states := cr.HealthStates()
	require.Len(t, states, 1)
	require.NotNil(t, states[0].SuggestedActions)
	assert.Equal(t, []apiv1.RepairActionType{apiv1.RepairActionTypeRebootSystem}, states[0].SuggestedActions.RepairActions)
}

func TestCheck_BlockedProcesses_EscalatesToHardwareInspectionAfterReboots(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	// two reboots since the condition was first observed, no recovery in between
	rebootStore := &mockRebootEventStore{
		events: eventstore.Events{
			{Time: time.Unix(99000, 0), Name: "reboot", Type: string(apiv1.EventTypeWarning)},
			{Time: time.Unix(99500, 0), Name: "reboot", Type: string(apiv1.EventTypeWarning)},
		},
	}

	comp, now := newTestComponent(t, ctx, rebootStore, &recordingBucket{})
	defer func() { _ = comp.Close() }()
	withBlockedProcessThresholds(comp, mustBlockedProcessThresholds(t, 5, "^nvidia"))

	cr := runChecks(comp, now, 5, blockedList(&mockProcessStatus{pid: 42, name: "nvidia-smi"}))
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.HealthStateType())

	states := cr.HealthStates()
	require.Len(t, states, 1)
	require.NotNil(t, states[0].SuggestedActions)
	assert.Equal(t, []apiv1.RepairActionType{apiv1.RepairActionTypeHardwareInspection}, states[0].SuggestedActions.RepairActions,
		"repeated reboots without recovery must escalate to hardware inspection (xid parity)")
}

func TestCheck_BlockedProcesses_RebootsBeforeRecoveryDoNotEscalate(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	// reboots happened, but a recovery event afterwards resets the escalation window
	rebootStore := &mockRebootEventStore{
		events: eventstore.Events{
			{Time: time.Unix(99000, 0), Name: "reboot", Type: string(apiv1.EventTypeWarning)},
			{Time: time.Unix(99500, 0), Name: "reboot", Type: string(apiv1.EventTypeWarning)},
		},
	}
	bucket := &recordingBucket{
		getEvents: eventstore.Events{
			{Time: time.Unix(99800, 0), Name: EventNameBlockedProcessesRecovered, Type: string(apiv1.EventTypeInfo)},
		},
	}

	comp, now := newTestComponent(t, ctx, rebootStore, bucket)
	defer func() { _ = comp.Close() }()
	withBlockedProcessThresholds(comp, mustBlockedProcessThresholds(t, 5, "^nvidia"))

	cr := runChecks(comp, now, 5, blockedList(&mockProcessStatus{pid: 42, name: "nvidia-smi"}))
	states := cr.HealthStates()
	require.Len(t, states, 1)
	require.NotNil(t, states[0].SuggestedActions)
	assert.Equal(t, []apiv1.RepairActionType{apiv1.RepairActionTypeRebootSystem}, states[0].SuggestedActions.RepairActions,
		"reboots followed by a recovery must not escalate to hardware inspection")
}

func TestCheck_BlockedProcesses_RecoveryClears(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	bucket := &recordingBucket{}
	comp, now := newTestComponent(t, ctx, &mockRebootEventStore{}, bucket)
	defer func() { _ = comp.Close() }()
	withBlockedProcessThresholds(comp, mustBlockedProcessThresholds(t, 5, "^nvidia"))

	// become persistent
	cr := runChecks(comp, now, 5, blockedList(&mockProcessStatus{pid: 42, name: "nvidia-smi"}))
	require.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.HealthStateType())

	// process disappears: 1st absent check keeps the condition (grace)...
	cr = runChecks(comp, now, 1, nil)
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.HealthStateType(), "single absence must not clear (grace)")

	// ...2nd consecutive absent check drops the tracker entry and clears
	cr = runChecks(comp, now, 1, nil)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.HealthStateType(), "condition must clear after the process is gone")
	assert.Zero(t, comp.lastCheckResult.BlockedProcesses.PersistentCount)

	// episode events: persistent + recovered, in order
	assert.Equal(t, []string{EventNameBlockedProcessesPersistent, EventNameBlockedProcessesRecovered}, bucket.insertedNames())

	// and it can fire again later (no sticky state)
	cr = runChecks(comp, now, 5, blockedList(&mockProcessStatus{pid: 43, name: "nvidia-smi"}))
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.HealthStateType())
	assert.Equal(t, []string{EventNameBlockedProcessesPersistent, EventNameBlockedProcessesRecovered, EventNameBlockedProcessesPersistent}, bucket.insertedNames())
}

func TestCheck_BlockedProcesses_BurstTriggersDoNotFlag(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	comp, now := newTestComponent(t, ctx, &mockRebootEventStore{}, &recordingBucket{})
	defer func() { _ = comp.Close() }()
	withBlockedProcessThresholds(comp, mustBlockedProcessThresholds(t, 5, "^nvidia"))

	comp.countProcessesByStatusFunc = func(ctx context.Context) (map[string][]process.ProcessStatus, error) {
		return map[string][]process.ProcessStatus{
			procs.Running: make([]process.ProcessStatus, 10),
			procs.Blocked: {&mockProcessStatus{pid: 42, name: "nvidia-smi"}},
		}, nil
	}

	// simulate a control-plane/HTTP trigger burst: 10 Check() calls 2s apart
	for i := 0; i < 10; i++ {
		cr := comp.Check()
		assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.HealthStateType(),
			"trigger burst check %d must not flag a seconds-old D-state process", i)
		*now = now.Add(2 * time.Second)
	}

	// the same process still blocked once 4+ minutes have elapsed fires
	*now = now.Add(4 * time.Minute)
	cr := comp.Check()
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.HealthStateType())
}

func TestCheck_BlockedProcesses_RebootStoreErrorFailsOpenToReboot(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	comp, now := newTestComponent(t, ctx, &errRebootEventStore{}, &recordingBucket{})
	defer func() { _ = comp.Close() }()
	withBlockedProcessThresholds(comp, mustBlockedProcessThresholds(t, 5, "^nvidia"))

	cr := runChecks(comp, now, 5, blockedList(&mockProcessStatus{pid: 42, name: "nvidia-smi"}))
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.HealthStateType())

	states := cr.HealthStates()
	require.Len(t, states, 1)
	require.NotNil(t, states[0].SuggestedActions)
	assert.Equal(t, []apiv1.RepairActionType{apiv1.RepairActionTypeRebootSystem}, states[0].SuggestedActions.RepairActions,
		"reboot-store errors must fail open to a reboot suggestion, never silently escalate")
}

func TestCheck_BlockedProcesses_BucketGetErrorCountsWindowReboots(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	// bucket.Get fails => lastRecovery stays zero => all window reboots count
	rebootStore := &mockRebootEventStore{
		events: eventstore.Events{
			{Time: time.Unix(99000, 0), Name: "reboot", Type: string(apiv1.EventTypeWarning)},
			{Time: time.Unix(99500, 0), Name: "reboot", Type: string(apiv1.EventTypeWarning)},
		},
	}
	bucket := &recordingBucket{getError: errors.New("mock bucket get error")}

	comp, now := newTestComponent(t, ctx, rebootStore, bucket)
	defer func() { _ = comp.Close() }()
	withBlockedProcessThresholds(comp, mustBlockedProcessThresholds(t, 5, "^nvidia"))

	cr := runChecks(comp, now, 5, blockedList(&mockProcessStatus{pid: 42, name: "nvidia-smi"}))
	states := cr.HealthStates()
	require.Len(t, states, 1)
	require.NotNil(t, states[0].SuggestedActions)
	assert.Equal(t, []apiv1.RepairActionType{apiv1.RepairActionTypeHardwareInspection}, states[0].SuggestedActions.RepairActions,
		"when recovery history is unreadable, window reboots are counted (2 here) and escalate")
}

func TestCheck_BlockedProcesses_ProcessScanErrorStillUnhealthy(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	comp, _ := newTestComponent(t, ctx, &mockRebootEventStore{}, nil)
	defer func() { _ = comp.Close() }()

	comp.countProcessesByStatusFunc = func(ctx context.Context) (map[string][]process.ProcessStatus, error) {
		return nil, errors.New("process count error")
	}
	cr := comp.Check()
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.HealthStateType())
	assert.Contains(t, cr.Summary(), "error getting process count")
}

func TestCheck_BlockedProcesses_DisableClearsStateAndGauges(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	bucket := &recordingBucket{}
	comp, now := newTestComponent(t, ctx, &mockRebootEventStore{}, bucket)
	defer func() { _ = comp.Close() }()

	// runtime-switchable thresholds, simulating a session updateConfig push
	thresholds := mustBlockedProcessThresholds(t, 5, "^nvidia")
	comp.getBlockedProcessThresholdsFunc = func() BlockedProcessThresholds { return thresholds }

	// 1. enabled: 5 consecutive checks with a blocked nvidia-smi => unhealthy episode
	cr := runChecks(comp, now, 5, blockedList(&mockProcessStatus{pid: 42, name: "nvidia-smi"}))
	require.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.HealthStateType())
	require.Equal(t, []string{EventNameBlockedProcessesPersistent}, bucket.insertedNames())
	assertGaugeValue(t, metricBlockedProcessesPersistent, 1)

	// 2. disabled mid-episode (empty regexes): one check clears health,
	// resets the tracker, closes the episode with a "disabled" recovery event,
	// and zeroes the gauges
	thresholds = mustBlockedProcessThresholds(t, 5)
	cr = runChecks(comp, now, 1, nil)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.HealthStateType())

	comp.blockedTracker.mu.Lock()
	tracked := len(comp.blockedTracker.entries)
	comp.blockedTracker.mu.Unlock()
	assert.Zero(t, tracked, "disable must reset the tracker so re-enable re-earns the threshold")

	names := bucket.insertedNames()
	require.Equal(t, []string{EventNameBlockedProcessesPersistent, EventNameBlockedProcessesRecovered}, names)
	events := bucket.insertedEvents()
	assert.Contains(t, events[1].Message, "disabled", "recovery event must say the check was disabled, not that the processes actually cleared")

	assertGaugeValue(t, metricBlockedProcesses, 0)
	assertGaugeValue(t, metricBlockedProcessesPersistent, 0)

	// 3. re-enabled while the same process is still blocked: must re-earn the
	// full persistence threshold (not resume the stale counter), then fire a
	// new episode
	thresholds = mustBlockedProcessThresholds(t, 5, "^nvidia")
	cr = runChecks(comp, now, 4, blockedList(&mockProcessStatus{pid: 42, name: "nvidia-smi"}))
	assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.HealthStateType(), "re-enabled check must re-earn the persistence threshold")
	cr = runChecks(comp, now, 1, blockedList(&mockProcessStatus{pid: 42, name: "nvidia-smi"}))
	assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.HealthStateType())
	assert.Equal(t, []string{EventNameBlockedProcessesPersistent, EventNameBlockedProcessesRecovered, EventNameBlockedProcessesPersistent}, bucket.insertedNames())
}

// assertGaugeValue asserts the current value of a component gauge.
func assertGaugeValue(t *testing.T, gaugeVec *prometheus.GaugeVec, want float64) {
	t.Helper()
	gauge, err := gaugeVec.GetMetricWith(prometheus.Labels{})
	require.NoError(t, err)
	dto := &prometheusdto.Metric{}
	require.NoError(t, gauge.Write(dto))
	assert.InDelta(t, want, dto.GetGauge().GetValue(), 0.0001)
}
