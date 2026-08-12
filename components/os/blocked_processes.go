//nolint:revive // package name follows the directory import path used across the codebase.
package os

import (
	"sort"
	"sync"
	"time"

	"github.com/leptonai/gpud/pkg/log"
	"github.com/leptonai/gpud/pkg/process"
)

// This file implements tracking of processes stuck in uninterruptible sleep
// (Linux "D" state, surfaced by gopsutil as "blocked") for LEP-6029.
//
// Design constraints validated against production clusters (2026-08-12):
//   - Detection reuses the existing per-minute CountProcessesByStatus scan; no
//     additional process enumeration, no external commands (nvidia-smi, NVML).
//   - Healthy GPU nodes showed zero D-state processes; a real D-state process
//     persists for minutes, so a persistence threshold of consecutive checks
//     (not a single observation) is required to avoid false flags.
//   - A tracked PID that is absent from a single check (PID churn between
//     enumeration and metadata reads, transient /proc read errors) must not
//     reset its persistence counter.
//   - Diagnostic payloads must be bounded and must not contain environment
//     variables or unbounded command lines.
//
// The configurable thresholds (persistence, process-name regexes) live in
// threshold.go, following the gpu-counts threshold pattern.

const (
	// blockedProcessAbsenceGrace is the number of consecutive checks a tracked
	// process may be missing (PID reuse/churn, transient /proc read errors)
	// before it is dropped from tracking. One missed check neither extends nor
	// resets the persistence counter.
	blockedProcessAbsenceGrace = 1

	// maxBlockedProcessesOutput bounds the number of persistent blocked
	// processes persisted in the check result payload.
	maxBlockedProcessesOutput = 20

	// checkInterval is the cadence the persistence threshold is calibrated
	// against: Start runs Check once per minute. The duration gate in update
	// makes the ticket's "five consecutive ONE-MINUTE checks" robust to
	// out-of-band Check() invocations (/v1/components/trigger-check, session
	// triggerComponent call comp.Check() directly, ungated): without it, a
	// burst of manual triggers would inflate consecutiveChecks within seconds
	// and flag a seconds-old D-state process as persistent.
	checkInterval = time.Minute

	// DefaultBlockedProcessRebootThreshold is the number of reboot events (with
	// no recovery in between) after which a persistent blocked-process
	// condition escalates from reboot suggestion to hardware inspection.
	// Kept at parity with the XID component's DefaultRebootThreshold
	// (components/accelerator/nvidia/xid).
	DefaultBlockedProcessRebootThreshold = 2
)

// BlockedProcess describes one process observed in uninterruptible sleep.
type BlockedProcess struct {
	// PID is the process ID.
	PID int32 `json:"pid"`
	// Name is the process name (Linux comm, truncated to 15 chars by the kernel).
	Name string `json:"name"`
	// FirstSeenUnixSeconds is when the process was first observed blocked.
	FirstSeenUnixSeconds int64 `json:"first_seen_unix_seconds"`
	// LastSeenUnixSeconds is when the process was most recently observed blocked.
	LastSeenUnixSeconds int64 `json:"last_seen_unix_seconds"`
	// BlockedSeconds is the observed blocked duration (last seen - first seen).
	BlockedSeconds int64 `json:"blocked_seconds"`
	// ConsecutiveChecks is the number of consecutive checks the process was blocked.
	ConsecutiveChecks int `json:"consecutive_checks"`
}

// BlockedProcesses summarizes the blocked-process state of one check.
type BlockedProcesses struct {
	// CurrentCount is the number of processes blocked at this check.
	CurrentCount int `json:"current_count"`
	// PersistentCount is the number of processes that met the persistence
	// threshold (may exceed len(Persistent) when the output is capped).
	PersistentCount int `json:"persistent_count"`
	// Persistent is the bounded list of persistent blocked processes.
	Persistent []BlockedProcess `json:"persistent,omitempty"`
}

type blockedProcessEntry struct {
	name              string
	firstSeen         time.Time
	lastSeen          time.Time
	consecutiveChecks int
	absentChecks      int
}

// blockedProcessTracker tracks blocked PIDs across checks.
type blockedProcessTracker struct {
	mu      sync.Mutex
	entries map[int32]*blockedProcessEntry
}

func newBlockedProcessTracker() *blockedProcessTracker {
	return &blockedProcessTracker{
		entries: make(map[int32]*blockedProcessEntry),
	}
}

// reset drops all tracked processes, e.g. when an operator marks the
// component healthy: a process still blocked must re-earn the persistence
// threshold before being flagged again.
func (t *blockedProcessTracker) reset() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.entries = make(map[int32]*blockedProcessEntry)
}

// update ingests the currently blocked processes observed at "now" and returns
// the number of persistent blocked processes along with a bounded list of their
// details (sorted by first-seen time, then PID). A process is persistent once
// it has been blocked for persistenceThreshold consecutive checks; a
// non-positive persistenceThreshold falls back to the default.
//
// Tolerates processes disappearing between enumeration and metadata reads:
// a process absent for at most blockedProcessAbsenceGrace consecutive checks is
// kept (its persistence counter is preserved, not extended); longer absence
// drops the entry (recovered).
func (t *blockedProcessTracker) update(now time.Time, blocked []process.ProcessStatus, persistenceThreshold int) (int, []BlockedProcess) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if persistenceThreshold <= 0 {
		persistenceThreshold = DefaultBlockedProcessPersistenceThreshold
	}

	seen := make(map[int32]struct{}, len(blocked))
	for _, p := range blocked {
		if p == nil {
			continue
		}
		pid := p.PID()
		seen[pid] = struct{}{}

		name, err := p.Name()
		if err != nil {
			log.Logger.Warnw("failed to read blocked process name", "pid", pid, "error", err)
			name = ""
		}

		e, ok := t.entries[pid]
		if !ok {
			t.entries[pid] = &blockedProcessEntry{
				name:              name,
				firstSeen:         now,
				lastSeen:          now,
				consecutiveChecks: 1,
			}
			continue
		}
		if name != "" {
			e.name = name
		}
		e.lastSeen = now
		e.consecutiveChecks++
		e.absentChecks = 0
	}

	for pid, e := range t.entries {
		if _, ok := seen[pid]; ok {
			continue
		}
		e.absentChecks++
		if e.absentChecks > blockedProcessAbsenceGrace {
			delete(t.entries, pid)
		}
	}

	persistentCount := 0
	persistent := make([]BlockedProcess, 0)
	// A process must persist for at least (threshold-1) check intervals in
	// wall time, in addition to the consecutive-check count. At the normal
	// one-minute cadence the two gates coincide (5 checks span 4 minutes);
	// the duration gate only binds when Check is invoked faster than that.
	minDuration := time.Duration(persistenceThreshold-1) * checkInterval
	for pid, e := range t.entries {
		if e.consecutiveChecks < persistenceThreshold {
			continue
		}
		if now.Sub(e.firstSeen) < minDuration {
			continue
		}
		persistentCount++
		persistent = append(persistent, BlockedProcess{
			PID:                  pid,
			Name:                 e.name,
			FirstSeenUnixSeconds: e.firstSeen.Unix(),
			LastSeenUnixSeconds:  e.lastSeen.Unix(),
			BlockedSeconds:       int64(e.lastSeen.Sub(e.firstSeen).Seconds()),
			ConsecutiveChecks:    e.consecutiveChecks,
		})
	}

	sort.Slice(persistent, func(i, j int) bool {
		if persistent[i].FirstSeenUnixSeconds == persistent[j].FirstSeenUnixSeconds {
			return persistent[i].PID < persistent[j].PID
		}
		return persistent[i].FirstSeenUnixSeconds < persistent[j].FirstSeenUnixSeconds
	})

	if len(persistent) > maxBlockedProcessesOutput {
		persistent = persistent[:maxBlockedProcessesOutput]
	}

	return persistentCount, persistent
}
