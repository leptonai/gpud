package infiniband

import (
	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components/accelerator/nvidia/infiniband/types"
	"github.com/leptonai/gpud/pkg/log"
)

var (
	// nothing specified for this machine, gpud MUST skip the ib check
	reasonNoThreshold   = "ports or rate threshold not set (skipped evaluation)"
	reasonNoIbPortData  = "no infiniband port data (skipped evaluation)"
	reasonNoIbPortIssue = "ok; no infiniband port issue"
)

// evaluateHealthStateWithThresholds evaluates the current infiniband port states against the thresholds
// and it DOES NOT take historical states into account
func evaluateHealthStateWithThresholds(thresholds types.ExpectedPortStates, ibports []types.IBPort, cr *checkResult) {
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

	unhealthy, err := EvaluatePortsAndRate(ibports, atLeastPorts, atLeastRate)
	if err != nil {
		cr.unhealthyIBPorts = unhealthy
		cr.health = apiv1.HealthStateTypeUnhealthy
		cr.reason = err.Error()

		// NOTE: do not set suggested actions to "apiv1.RepairActionTypeHardwareInspection" here
		// since this port mismatch often self-recovers
		// "apiv1.RepairActionTypeHardwareInspection" is reserved for irrecoverable hardware issues

		return
	}

	cr.health = apiv1.HealthStateTypeHealthy
	cr.suggestedActions = nil
	cr.reason = reasonNoIbPortIssue

	cr.unhealthyIBPorts = nil
}
