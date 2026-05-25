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
//   - nfs_server_not_responding: counts per server only when it happened after
//     that server's latest nfs_server_ok.
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
		lockReclaim    eventstore.Events
		serverResponse = make(map[string]*nfsServerResponseEvents)
		writeback      eventstore.Events
	)

	for _, ev := range events {
		switch ev.Name {
		case eventNFSLockReclaimFailed:
			lockReclaim = append(lockReclaim, ev)
		case eventNFSServerNotResponding:
			response := getNFSServerResponseEvents(serverResponse, ev)
			response.notResponding = append(response.notResponding, ev)
		case eventNFSServerOK:
			response := getNFSServerResponseEvents(serverResponse, ev)
			response.ok = append(response.ok, ev)
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

	// Rule A: server not responding — evaluate each server independently and
	// keep only events that happened after that server's latest "OK".
	notRespondingCount := 0
	for _, response := range serverResponse {
		if len(response.notResponding) == 0 {
			continue
		}

		unresolved := response.notResponding
		if len(response.ok) > 0 {
			latestOK := latestTime(response.ok)
			unresolved = nil
			for _, ev := range response.notResponding {
				if ev.Time.After(latestOK) {
					unresolved = append(unresolved, ev)
				}
			}
		}

		if len(unresolved) > 0 {
			hang = append(hang, unresolved...)
			notRespondingCount += len(unresolved)
		}
	}
	if notRespondingCount > 0 {
		reasonParts = append(reasonParts, fmt.Sprintf("%d NFS server not-responding events", notRespondingCount))
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

type nfsServerResponseEvents struct {
	notResponding eventstore.Events
	ok            eventstore.Events
}

func getNFSServerResponseEvents(responses map[string]*nfsServerResponseEvents, ev eventstore.Event) *nfsServerResponseEvents {
	server := nfsServerFromEventMessage(ev.Message)
	if responses[server] == nil {
		responses[server] = &nfsServerResponseEvents{}
	}
	return responses[server]
}

func nfsServerFromEventMessage(message string) string {
	for _, prefix := range []string{
		messageNFSServerNotResponding + ": ",
		messageNFSServerOK + ": ",
	} {
		if server := strings.TrimPrefix(message, prefix); server != message {
			return server
		}
	}
	return ""
}

// nfsEventsOnOrAfter returns events whose timestamps are on or after since,
// matching eventstore.EvaluateSuggestedActions' treatment of equal timestamps.
func nfsEventsOnOrAfter(events eventstore.Events, since time.Time) eventstore.Events {
	filtered := make(eventstore.Events, 0, len(events))
	for _, ev := range events {
		if !ev.Time.Before(since) {
			filtered = append(filtered, ev)
		}
	}
	return filtered
}
