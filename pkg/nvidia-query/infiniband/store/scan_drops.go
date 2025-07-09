package store

import (
	"fmt"
	"time"

	"github.com/leptonai/gpud/pkg/log"
)

const (
	defaultIbPortDropLookbackPeriod = 10 * time.Minute
	defaultIbPortDropThreshold      = 4 * time.Minute

	EventTypeIbPortDrop = "ib_port_drop"
)

// scanIBPortDrops scans the ib port drops for the past period since "since"
// and returns the snapshots with the drop events.
func (s *ibPortsStore) scanIBPortDrops(device string, port uint, since time.Time) ([]devPortSnapshotWithReason, error) {
	ss, err := s.readDevPortSnapshots(device, port, since)
	if err != nil {
		return nil, err
	}
	return ss.findDrops(device, port, s.ibPortDropThreshold), nil
}

// findDrops finds the ib port drop events in the snapshots,
// and return the snapshots with the drop events.
//
// assume "devPortSnapshots" already ranges enough time to evaluate the events with thresholds
// (if threshold is 4-minute, assume the snapshots already ranges >4-minute)
// assume "devPortSnapshots" are sorted by timestamp in ascending order
// assume "devPortSnapshots" are ALREADY grouped by the same device and port
//
// ib port is marked "drop" when
// 1. [devPortSnapshot.state] has been "down" and has not changed, for the period "threshold"
// 2. [devPortSnapshot.totalLinkDowned] has not changed, for the period "threshold"
func (ss devPortSnapshots) findDrops(device string, port uint, threshold time.Duration) []devPortSnapshotWithReason {
	// need at least 2 snapshots to evaluate ib port drop
	if len(ss) <= 1 {
		return nil
	}

	var downOldest *devPortSnapshot
	var downLatest *devPortSnapshot
	for _, snapshot := range ss {
		// only "drop" when the "down" events are consecutive
		if snapshot.state == "active" {
			downOldest = nil
			downLatest = nil
			continue
		}

		// the for-loop is sorted by timestamp in ascending order
		// thus only set once!
		if downOldest == nil {
			downOldest = &snapshot
		}

		// keeps overwriting to the latest down event
		downLatest = &snapshot

		// we DO NOT stop iterating here because
		// we want the latest down events
	}

	// no ib port down event, thus no ib port drop
	if downOldest == nil || downLatest == nil {
		return nil
	}

	// now we know that
	// [devPortSnapshot.state] has been "down" and has not changed

	if downOldest.totalLinkDowned != downLatest.totalLinkDowned {
		// the total link downed count has changed, thus no ib port drop
		// possible flap event
		log.Logger.Warnw("persistent ib port down but different total link downed count (potential ib port flap)",
			"device", device,
			"port", port,
			"oldest_down", downOldest.ts,
			"latest_down", downLatest.ts,
			"oldest_down_total_link_downed", downOldest.totalLinkDowned,
			"latest_down_total_link_downed", downLatest.totalLinkDowned,
		)
		return nil
	}

	// now we know that
	// [devPortSnapshot.totalLinkDowned] has not changed

	// now make sure the drop state persisted long enough for the period "threshold"
	elapsed := downLatest.ts.Sub(downOldest.ts)
	if elapsed < threshold {
		// the drop state did NOT persist long enough for the period X
		log.Logger.Warnw("persistent ib port down but did not persist long enough for the period X",
			"device", device,
			"port", port,
			"oldest_down", downOldest.ts,
			"latest_down", downLatest.ts,
			"elapsed", elapsed,
		)
		return nil
	}

	return []devPortSnapshotWithReason{
		{
			reason:          fmt.Sprintf("%s port %d down since %s", device, port, downOldest.ts.UTC().Format(time.RFC3339)),
			devPortSnapshot: *downLatest,
		},
	}
}
