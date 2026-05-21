package nfs

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/leptonai/gpud/pkg/eventstore"
)

// collectNFSHangEvents applies the conjunctive evidence rules (design doc
// §3.2.2) to filter true hang events out of raw kmsg events.
//
// Rules:
//   - nfs_lock_reclaim_failed: any single occurrence counts.
//   - nfs_server_not_responding: counts only when no later nfs_server_ok
//     cancels it out (compare the latest of each kind).
//   - nfs_writeback_hang: counts only when there are ≥ 2 occurrences in the
//     input window.
//
// The input events come from Bucket.Get (descending by Time). The function
// does not assume any particular order — internally it inspects each event
// independently. The returned hang slice is sorted ascending by Time so it
// can be fed directly into eventstore.EvaluateSuggestedActions (which
// expects ascending order per design doc §9 U2). The reason is a
// human-readable summary; when no hang is detected, ("", nil) is returned
// so the caller can decide whether to fall back to the prober.
func collectNFSHangEvents(events eventstore.Events) (hang eventstore.Events, reason string) {
	var (
		lockReclaim   eventstore.Events
		notResponding eventstore.Events
		ok            eventstore.Events
		writeback     eventstore.Events
	)

	for _, ev := range events {
		switch ev.Name {
		case eventNFSLockReclaimFailed:
			lockReclaim = append(lockReclaim, ev)
		case eventNFSServerNotResponding:
			notResponding = append(notResponding, ev)
		case eventNFSServerOK:
			ok = append(ok, ev)
		case eventNFSWritebackHang:
			writeback = append(writeback, ev)
		}
	}

	var reasonParts []string

	// Rule B: lock reclaim — any single occurrence is a hang.
	if len(lockReclaim) > 0 {
		hang = append(hang, lockReclaim...)
		reasonParts = append(reasonParts, fmt.Sprintf("%d lock reclaim failures", len(lockReclaim)))
	}

	// Rule A: server not responding — only counts if not cancelled by a
	// later "OK". Compare the latest of each.
	if len(notResponding) > 0 {
		latestNR := latestTime(notResponding)
		cancelled := false
		if len(ok) > 0 {
			latestOK := latestTime(ok)
			if latestOK.After(latestNR) {
				cancelled = true
			}
		}
		if !cancelled {
			hang = append(hang, notResponding...)
			reasonParts = append(reasonParts, fmt.Sprintf("%d NFS server not-responding events", len(notResponding)))
		}
	}

	// Rule C: writeback stack hints — require ≥ 2 occurrences.
	if len(writeback) >= 2 {
		hang = append(hang, writeback...)
		reasonParts = append(reasonParts, fmt.Sprintf("%d NFS writeback stack hints", len(writeback)))
	}

	if len(hang) == 0 {
		return nil, ""
	}

	sort.Slice(hang, func(i, j int) bool {
		return hang[i].Time.Before(hang[j].Time)
	})

	reason = "NFS hang detected: " + strings.Join(reasonParts, "; ")
	return hang, reason
}

// latestTime returns the maximum Time across the given events. The slice
// must be non-empty.
func latestTime(events eventstore.Events) time.Time {
	t := events[0].Time
	for _, ev := range events[1:] {
		if ev.Time.After(t) {
			t = ev.Time
		}
	}
	return t
}
