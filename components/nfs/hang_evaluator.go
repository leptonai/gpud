package nfs

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/leptonai/gpud/pkg/eventstore"
)

// collectNFSHangEvents selects NFS issue signals from kernel messages.
// They identify failures to report, not proof that the client needs a reboot.
//
// Rules:
//   - nfs_lock_reclaim_failed: any single occurrence counts.
//   - nfs_server_not_responding: counts per server only when it happened after
//     that server's latest nfs_server_ok.
//   - nfs_writeback_hang: counts only when there are ≥ 2 occurrences in the
//     input window.
//
// The input order does not matter. The reason summarizes the selected
// signals; no selected signals lets the caller fall back to the prober.
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

	// A lock-reclaim failure is worth reporting, but does not prove a hang.
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

	reason = "NFS issue detected: " + strings.Join(reasonParts, "; ")
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

// nfsEventsOnOrAfter includes messages at the reboot timestamp because
// timestamp precision may place a new-boot message in the same second.
func nfsEventsOnOrAfter(events eventstore.Events, since time.Time) eventstore.Events {
	filtered := make(eventstore.Events, 0, len(events))
	for _, ev := range events {
		if !ev.Time.Before(since) {
			filtered = append(filtered, ev)
		}
	}
	return filtered
}
