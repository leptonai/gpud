package infiniband

import (
	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/pkg/log"
	"github.com/leptonai/gpud/pkg/nvidia-query/infiniband"
)

var (
	// nothing specified for this machine, gpud MUST skip the ib check
	reasonNoThreshold   = "ports or rate threshold not set (skipped evaluation)"
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

	// just skip the evaluation
	if len(ibports) == 0 {
		cr.health = apiv1.HealthStateTypeHealthy
		cr.reason = reasonNoIbPortData
		log.Logger.Warnw(cr.reason)
		return
	}

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
}
