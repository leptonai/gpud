package infiniband

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/dustin/go-humanize"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/pkg/log"
	"github.com/leptonai/gpud/pkg/nvidia-query/infiniband"
)

var (
	// nothing specified for this machine, gpud MUST skip the ib check
	reasonNoThreshold   = "ports or rate threshold not set (skipped evaluation)"
	reasonNoEventBucket = "no event storage (skipped evaluation)"
	reasonNoIbPortData  = "no infiniband port data (skipped evaluation)"
	reasonNoIbPortIssue = "ok; no infiniband port issue"
)

// evaluateHealthStateWithThresholds evaluates the current infiniband port states against the thresholds
// and it DOES NOT take historical states into account
func evaluateHealthStateWithThresholds(thresholds infiniband.ExpectedPortStates, ibports []infiniband.IBPort, cr *checkResult) {
	// DO NOT auto-detect infiniband devices/PCI buses
	// strictly rely on the user-specified config.
	if thresholds.IsZero() {
		cr.health = apiv1.HealthStateTypeHealthy
		cr.suggestedActions = nil
		cr.reason = reasonNoThreshold

		cr.unhealthyIBPorts = nil

		cr.err = nil
		return
	}

	// neither "ibstat" nor "ibstatus" command returned any data
	// then we just skip the evaluation
	if len(ibports) == 0 {
		cr.health = apiv1.HealthStateTypeHealthy
		cr.reason = reasonNoIbPortData
		log.Logger.Warnw(cr.reason)
		return
	}

	// Link down/drop -> hardware inspection
	// Link port flap -> hardware inspection
	atLeastPorts := thresholds.AtLeastPorts
	atLeastRate := thresholds.AtLeastRate

	unhealthy, err := infiniband.EvaluatePortsAndRate(ibports, atLeastPorts, atLeastRate)
	if err != nil {
		cr.health = apiv1.HealthStateTypeUnhealthy
		cr.suggestedActions = &apiv1.SuggestedActions{
			RepairActions: []apiv1.RepairActionType{apiv1.RepairActionTypeHardwareInspection},
		}
		cr.reason = err.Error()

		cr.unhealthyIBPorts = unhealthy
		return
	}

	cr.health = apiv1.HealthStateTypeHealthy
	cr.suggestedActions = nil
	cr.reason = reasonNoIbPortIssue

	cr.unhealthyIBPorts = nil

	// whether ibstat command failed or not (e.g., one port device is wrongly mapped), we use the entire/partial output
	// but we got the entire/partial output from "ibstat" command
	// thus we use the data from "ibstat" command to evaluate
	// ok to error as long as it meets the thresholds
	// which means we may overwrite the error above
	// (e.g., "ibstat" command exited 255 but still meets the thresholds)
	// TODO: do not need this logic once we deprecate the "ibstat" command
	if cr.err != nil && cr.health == apiv1.HealthStateTypeHealthy {
		// partial output from "ibstat" command worked
		log.Logger.Debugw("ibstat command returned partial output -- discarding error", "error", cr.err, "reason", cr.reason)
		cr.err = nil
	}
}

// evaluateIbSwitchFault evaluates whether the check result is caused by
// the ib switch fault, where all ports are down
// if that's the case, it sets the field [checkResult.reasonIbSwitchFault]
func evaluateIBSwitchFault(currentPorts []infiniband.IBPort, cr *checkResult) {
	if len(currentPorts) == 0 {
		return
	}
	if cr == nil {
		return
	}

	total := 0
	for _, port := range currentPorts {
		if !port.IsIBPort() {
			continue
		}
		total++
	}

	if total == len(cr.unhealthyIBPorts) {
		cr.reasonIbSwitchFault = "ib switch fault, all ports down"
	}
}

// IB port drop when a port has been down for more than 4-minute
// assumes the snapshots are sorted in the ascending order of [ibPortsSnapshot.ts]
func evaluateIBPortsDrop(ibportsSnapshots ibPortsSnapshots, dropThreshold time.Duration, cr *checkResult) {
	if len(ibportsSnapshots) == 0 {
		return
	}
	if cr == nil {
		return
	}
	if cr.ts.IsZero() {
		// current check result timestamp is unknown, can't evaluate
		return
	}

	// maps from port device name to the time when the port first dropped and is still down!
	remainDroppedSince := make(map[string]time.Time)
	for _, snapshot := range ibportsSnapshots {
		for _, port := range snapshot.unhealthy {
			if !port.IsIBPort() {
				continue
			}

			// delete in for-loop, because the later one in the entry
			// is the latest one, thus, if the latest event says this port is up
			// we should delete the entry from the map since it's not down anymore
			if strings.Contains(strings.ToLower(port.State), "active") {
				delete(remainDroppedSince, port.Device)
				continue
			}

			// only track the first time the port dropped
			// in order to not overwrite the timestamp with the later timestamp
			// even if there are more subsequent down events for the same device/port
			if _, ok := remainDroppedSince[port.Device]; !ok {
				remainDroppedSince[port.Device] = snapshot.ts
			}
		}
	}

	// now "remainDroppedSince" only contains the ports that are STILL down
	// up to the current/latest results
	// now double-check the entries by their timestamps
	msgs := make([]string, 0)
	for device, ts := range remainDroppedSince {
		elapsed := cr.ts.Sub(ts)
		if elapsed < 0 {
			// something wrong... clock drift?
			log.Logger.Warnw("unexpected ib ports snapshots ordering", "checkResultTimestamp", cr.ts, "snapshotTimestamp", ts)
			continue
		}

		if elapsed < dropThreshold {
			// some ports are down, but only down for less than 4 minutes (too recent!)
			// thus safe to assume no ib port drop
			// even if we have more events, all only elapsed less than 4 minutes
			// thus safe to assume no ib port drop
			// may come back later!
			log.Logger.Warnw("ib port drop too recent", "device", device, "elapsed", elapsed)
			continue
		}

		sinceDesc := humanize.RelTime(ts, cr.ts, "ago", "from now")
		msgs = append(msgs, fmt.Sprintf("%s dropped %s", device, sinceDesc))
	}
	if len(msgs) == 0 {
		// no ib port drop, no unhealthy ib ports
		return
	}
	sort.Strings(msgs)

	cr.reasonIbPortsDrop = "ib port drop -- " + strings.Join(msgs, ", ")
}

// IB port flap when a port is down and back to active for the last 4-minute
// assumes the snapshots are sorted in the ascending order of [ibPortsSnapshot.ts]
// assumes the snapshots are already truncated to its retention period (e.g., 5 minutes)
func evaluateIBPortFlap(ibportsSnapshots ibPortsSnapshots, flapEvaluatePeriod time.Duration, cr *checkResult) {
	if len(ibportsSnapshots) == 0 {
		return
	}
	if cr == nil {
		return
	}
	if cr.ts.IsZero() {
		// current check result timestamp is unknown, can't evaluate
		return
	}

	// map from device name -> port number -> state transition from down to active, or vice versa
	// first [0]string is the oldest down state (if any)
	// second [1]string is the latest active state (if any)
	// if first is Down, second is Active, there's a ib port flap event
	stateTransitions := make(map[string]map[uint][2]string)

	// map from device name -> port number -> total link downed min/max
	// first [0]uint64 is the min
	// second [1]uint64 is the max
	// if min < max, there's a ib port drop event
	// if min < max AND state transitioned from down to active, there's a ib port flap event
	linkDowned := make(map[string]map[uint][2]uint64)
	for _, snapshot := range ibportsSnapshots {
		// even when the current check result is healthy
		// if the old results were unhealthy
		// we still need to evaluate the ib port flap
		for _, port := range snapshot.all {
			if !port.IsIBPort() {
				continue
			}

			elapsed := cr.ts.Sub(snapshot.ts)
			if elapsed < 0 {
				// something wrong... clock drift?
				log.Logger.Warnw("unexpected ib ports snapshots ordering", "checkResultTimestamp", cr.ts, "snapshotTimestamp", snapshot.ts)
				continue
			}

			// only evaluate the latest port states
			if elapsed > flapEvaluatePeriod {
				continue
			}

			if _, ok := stateTransitions[port.Device]; !ok {
				stateTransitions[port.Device] = make(map[uint][2]string)
			}
			if _, ok := stateTransitions[port.Device][port.Port]; !ok {
				stateTransitions[port.Device][port.Port] = [2]string{port.State, port.State}
			}
			stateTransitionPair := stateTransitions[port.Device][port.Port]
			if strings.Contains(strings.ToLower(port.State), "down") {
				stateTransitionPair[0] = port.State
			} else {
				stateTransitionPair[1] = port.State
			}
			stateTransitions[port.Device][port.Port] = stateTransitionPair

			// update the link downed min/max
			if _, ok := linkDowned[port.Device]; !ok {
				linkDowned[port.Device] = make(map[uint][2]uint64)
			}
			if _, ok := linkDowned[port.Device][port.Port]; !ok {
				linkDowned[port.Device][port.Port] = [2]uint64{port.TotalLinkDowned, port.TotalLinkDowned}
			}
			linkDownedPair := linkDowned[port.Device][port.Port]
			if port.TotalLinkDowned < linkDownedPair[0] {
				linkDownedPair[0] = port.TotalLinkDowned
			}
			if port.TotalLinkDowned > linkDownedPair[1] {
				linkDownedPair[1] = port.TotalLinkDowned
			}
			linkDowned[port.Device][port.Port] = linkDownedPair
		}
	}

	// if first is Down, second is Active, there's a ib port flap event
	// if min < max AND state transitioned from down to active, there's a ib port flap event
	msgs := make([]string, 0)
	for dev, portStates := range stateTransitions {
		for port, statePair := range portStates {
			if !strings.Contains(strings.ToLower(statePair[0]), "down") {
				continue
			}
			if !strings.Contains(strings.ToLower(statePair[1]), "active") {
				continue
			}

			linkDownedPair := linkDowned[dev][port]
			linkDownedDelta := linkDownedPair[1] - linkDownedPair[0]

			msg := fmt.Sprintf("%s port %d flapped %d time(s) from %s to %s", dev, port, linkDownedDelta, statePair[0], statePair[1])
			msgs = append(msgs, msg)
		}
	}

	if len(msgs) == 0 {
		// no ib port state flapped
		return
	}
	sort.Strings(msgs)

	cr.reasonIbPortsFlap = "ib port flap -- " + strings.Join(msgs, ", ")
}
