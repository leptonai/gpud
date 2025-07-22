package eventstore

import (
	"time"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/pkg/log"
)

// EvaluateSuggestedActions evaluates repair actions based on reboot history and failure events.
// It returns health state and suggested actions.
//
// The logic follows these rules:
// - If no failure events: panic (should never happen)
// - If the first reboot happened after the first failure: return nil (reboot may have fixed the issue)
// - If no reboots: suggest reboot
// - If only 1 reboot or 1 failure event: suggest reboot
// - Otherwise, count "reboot → failure" sequences (failures that occurred after reboots)
// - If reboot→failure sequences >= maxRebootsBeforeInspection: suggest hardware inspection
// - Otherwise: suggest reboot
//
// The function analyzes sequences where a failure occurs AFTER a reboot, indicating
// that the reboot did not resolve the issue.
//
// Parameters:
//   - rebootEvents: Historical reboot events
//   - failureEvents: Slice of failure events to evaluate (must not be empty, will panic if empty)
//   - maxRebootsBeforeInspection: Threshold for reboot→failure sequences before suggesting hardware inspection (typically 2)
//
// Returns:
//   - health: Health state (always Unhealthy when failures exist)
//   - suggestedActions: Suggested repair actions (REBOOT, HW_INSPECTION, or nil)
func EvaluateSuggestedActions(
	rebootEvents Events,
	failureEvents Events,
	maxRebootsBeforeInspection int,
) *apiv1.SuggestedActions {
	// if no failure event, return healthy
	if len(failureEvents) == 0 {
		panic("no failure event") // should never happen
	}

	// validate maxRebootsBeforeInspection
	if maxRebootsBeforeInspection < 1 {
		maxRebootsBeforeInspection = 2 // Default to 2 if invalid
	}

	if len(rebootEvents) == 0 {
		// first failure (no previous reboots) → suggest reboot
		log.Logger.Warnw("failure event found but no reboot event found -- suggesting reboot", "failureCount", len(failureEvents))
		return &apiv1.SuggestedActions{
			RepairActions: []apiv1.RepairActionType{
				apiv1.RepairActionTypeRebootSystem,
			},
		}
	}

	// edge case:
	// we just inserted above, before calling [EvaluateSuggestedActions]
	// assume reboot (if ever) happened before failure
	// AND there's no following failure event after reboot,
	// we should ignore (reboot could have been triggered just now!)
	firstRebootTime := rebootEvents[0].Time
	firstMismatchTime := failureEvents[0].Time
	if firstRebootTime.After(firstMismatchTime) {
		log.Logger.Warnw("no failure event found after reboot -- suggesting none")
		return nil
	}

	// now it's guaranteed that we have at least
	// one sequence of "failure event -> reboot"
	if len(rebootEvents) == 1 || len(failureEvents) == 1 {
		// case 1.
		// there's been only one reboot event,
		// so now we know there's only one possible sequence of
		// "failure event -> reboot"
		//
		// case 2.
		// failure event -> reboot
		// -> failure event; suggest second "reboot"
		// (after first reboot, we still get failure event)
		//
		// case 3.
		// there's been only one reboot + only one failure event; suggest second "reboot"
		// (after first reboot, we still get failure event)
		log.Logger.Warnw("failure event found -- suggesting reboot", "rebootCount", len(rebootEvents), "failureEvents", len(failureEvents))
		return &apiv1.SuggestedActions{
			RepairActions: []apiv1.RepairActionType{
				apiv1.RepairActionTypeRebootSystem,
			},
		}
	}

	// now that we have >=2 reboot events AND >=2 failure events
	// just check if there's been >=2 sequences of "reboot -> mismatch"
	// since it's possible that "reboot -> reboot -> mismatch -> mismatch"
	// which should only count as one sequence of "reboot -> mismatch"
	failureToRebootSequences := make(map[time.Time]time.Time)
	for i := 0; i < len(failureEvents); i++ {
		failureTime := failureEvents[i].Time

		for j := 0; j < len(rebootEvents); j++ {
			rebootTime := rebootEvents[j].Time

			if failureTime.Before(rebootTime) {
				continue
			}

			if _, ok := failureToRebootSequences[failureTime]; ok {
				// already seen this mismatch event with a corresponding reboot event
				continue
			}

			failureToRebootSequences[failureTime] = rebootTime
		}
	}

	// e.g.,
	// failure -> reboot
	// -> failure -> reboot
	// -> failure; suggest "hw inspection"
	// (after >=2 reboots, we still get failure)
	if len(failureToRebootSequences) >= maxRebootsBeforeInspection {
		log.Logger.Warnw("multiple reboot -> failures sequences found -- suggesting hw inspection", "rebootCount", len(rebootEvents), "failureEvents", len(failureEvents))
		return &apiv1.SuggestedActions{
			RepairActions: []apiv1.RepairActionType{
				apiv1.RepairActionTypeHardwareInspection,
			},
		}
	}

	// we do have valid sequence of "reboot -> failure"
	// but within threshold, still suggest "reboot"
	return &apiv1.SuggestedActions{
		RepairActions: []apiv1.RepairActionType{
			apiv1.RepairActionTypeRebootSystem,
		},
	}
}

// AggregateSuggestedActions combines multiple suggested actions with priority rules.
// If any action suggests HW_INSPECTION, that takes priority over SYSTEM_REBOOT.
// This allows components to evaluate multiple failure types and get a single recommendation.
func AggregateSuggestedActions(allActions []*apiv1.SuggestedActions) *apiv1.SuggestedActions {
	if len(allActions) == 0 {
		return nil
	}

	// Check if any action suggests hardware inspection
	hasHardwareInspection := false
	hasReboot := false

	for _, action := range allActions {
		if action == nil || len(action.RepairActions) == 0 {
			continue
		}

		for _, repairAction := range action.RepairActions {
			switch repairAction {
			case apiv1.RepairActionTypeHardwareInspection:
				hasHardwareInspection = true
			case apiv1.RepairActionTypeRebootSystem:
				hasReboot = true
			}
		}
	}

	// Priority order: HW_INSPECTION > REBOOT > nil
	if hasHardwareInspection {
		return &apiv1.SuggestedActions{
			RepairActions: []apiv1.RepairActionType{
				apiv1.RepairActionTypeHardwareInspection,
			},
		}
	}

	if hasReboot {
		return &apiv1.SuggestedActions{
			RepairActions: []apiv1.RepairActionType{
				apiv1.RepairActionTypeRebootSystem,
			},
		}
	}

	// No meaningful actions suggested
	return nil
}
