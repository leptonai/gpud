package store

import (
	"fmt"
	"time"

	"github.com/leptonai/gpud/pkg/log"
)

const (
	defaultIbPortFlapLookbackPeriod = 12*time.Hour + 5*time.Minute

	// subtract some buffer seconds for edge cases
	// when delta is exact 30-second and check interval is also 30 seconds
	defaultIbPortFlapDownIntervalThreshold = 25 * time.Second

	// requires at least 3 flap events (in the last 12 hours)
	// to be considered as a flap
	defaultIbPortFlapBackToActiveThreshold = 3

	EventTypeIbPortFlap = "ib_port_flap"
)

// scanIBPortFlaps scans the ib port flaps for the past period since "since"
// and returns the snapshots with the flap events.
func (s *ibPortsStore) scanIBPortFlaps(device string, port uint, since time.Time) ([]devPortSnapshotWithReason, error) {
	ss, err := s.readDevPortSnapshots(device, port, since)
	if err != nil {
		return nil, err
	}
	return ss.findFlaps(device, port, s.ibPortFlapDownIntervalThreshold, s.ibPortFlapBackToActiveThreshold), nil
}

// findFlaps finds the ib port flap events in the snapshots,
// and return the snapshots with the flap events.
//
// assume "devPortSnapshots" already ranges enough time to evaluate the events with thresholds
// (if threshold is 12-hour, assume the snapshots already ranges >12-hour)
// assume "devPortSnapshots" are sorted by timestamp in ascending order
// assume "devPortSnapshots" are ALREADY grouped by the same device and port
//
// ib port is marked "flap" when
// 1. [devPortSnapshot.state] has been "down" and has not changed, for the period "downIntervalThreshold"
// 2. after such persistent "down" events, [devPortSnapshot.state] flapped back to "active"
// 3. such "flap" events happened more than "flapBackToActiveThreshold" times
//
// e.g., "down for more than 30s and flap back for more than 2 times for the past 12 hours"
func (ss devPortSnapshots) findFlaps(device string, port uint, downIntervalThreshold time.Duration, flapBackToActiveThreshold int) []devPortSnapshotWithReason {
	// need at least 3 snapshots to evaluate ib port flaps
	// in case "flapBackToActiveThreshold" is 1
	if len(ss) < 3 || len(ss) < flapBackToActiveThreshold {
		return nil
	}

	// find the first persistent "down" event
	var down1 *devPortSnapshot
	var down2 *devPortSnapshot
	revertsToActive := make([]devPortSnapshotWithReason, 0)
	for _, snapshot := range ss {
		// potential "flap" events
		// if and only if we already have preceding persistent "down" events
		// only persistent when the "down" events are consecutive
		if snapshot.state == "active" {
			if down1 != nil && down2 != nil {
				// this can be a "flap" instance
				rs := devPortSnapshotWithReason{
					reason:          fmt.Sprintf("%s port %d down since %s (and flapped back to active)", device, port, down1.ts.UTC().Format(time.RFC3339)),
					devPortSnapshot: snapshot,
				}
				revertsToActive = append(revertsToActive, rs)

				log.Logger.Warnw("ib port reverted back to active (potential flap)",
					"device", device,
					"port", port,
					"down1", down1.ts,
					"down2", down2.ts,
					"revertToActive", snapshot.ts,
				)
			}

			// whether flap or not, since we revert back to "active"
			// we need to start over
			// either "down" events are not consecutive (not persistent) or flap events happened
			down1 = nil
			down2 = nil
			continue
		}

		if down1 == nil {
			down1 = &snapshot
		} else if down2 == nil {
			elapsed := snapshot.ts.Sub(down1.ts)
			if elapsed < downIntervalThreshold {
				// consecutive/persistent "down" events
				// but not long enough
				continue
			}

			// now we have the occurrence of the persistent "down" events
			// first and second down events persisted long enough
			down2 = &snapshot

			// we DO NOT stop iterating here because
			// we need to find out more than "flapBackToActiveThreshold" "flap" events
		}
	}

	// e.g., only 2 "flap" events happened when the threshold is 3
	// thus no ib port flap
	if len(revertsToActive) < flapBackToActiveThreshold {
		log.Logger.Warnw("ib port reverted back to active but not enough times (not a flap)",
			"device", device,
			"port", port,
			"revertsToActive", revertsToActive,
			"threshold", flapBackToActiveThreshold,
		)
		return nil
	}

	// now we have the "flap" events
	// only return the first "flap" event
	// the first time it breached the threshold
	// because we only need to save the first time it breached the threshold
	// and we don't need to save the subsequent "flap" events
	// e.g., "len(revertsToActive)" is 5 and "flapBackToActiveThreshold" is 3,
	// then return "revertsToActive[2]" because the third time is when it breached the threshold 3 (more than 2 times)
	return []devPortSnapshotWithReason{revertsToActive[flapBackToActiveThreshold-1]}
}
