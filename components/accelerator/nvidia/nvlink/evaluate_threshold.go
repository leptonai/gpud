package nvlink

import (
	"fmt"
	"strings"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/pkg/log"
)

const (
	reasonNoThresholdConfigured = "nvlink threshold not set (skipped evaluation)"
	reasonNoNVLinkData          = "no nvlink data (skipped evaluation)"
)

func appendNVLinkFailureDetails(reason string, cr *checkResult) string {
	if cr == nil {
		return reason
	}

	detailParts := []string{}
	if cr.PeerNVLinkProbePairCount > 0 && cr.PeerNVLinkOKPairCount == 0 && len(cr.PeerNVLinkObservedStatusCodes) > 0 {
		detailParts = append(detailParts, fmt.Sprintf("peer nvlink p2p statuses=%s", strings.Join(cr.PeerNVLinkObservedStatusCodes, ",")))
	}
	if len(cr.InactiveNVLinkUUIDs) > 0 {
		detailParts = append(detailParts, fmt.Sprintf("inactive nvlinks=%s", strings.Join(cr.InactiveNVLinkUUIDs, ",")))
	}
	if len(cr.UnsupportedNVLinkUUIDs) > 0 {
		detailParts = append(detailParts, fmt.Sprintf("unsupported nvlinks=%s", strings.Join(cr.UnsupportedNVLinkUUIDs, ",")))
	}
	if len(detailParts) == 0 {
		return reason
	}
	return fmt.Sprintf("%s (%s)", reason, strings.Join(detailParts, "; "))
}

func setNVLinkSuggestedActions(cr *checkResult) {
	if cr == nil {
		return
	}
	if cr.suggestedActions != nil {
		return
	}
	if len(cr.InactiveNVLinkUUIDs) > 0 ||
		(cr.hasCompletePeerNVLinkProbeCoverage() && cr.PeerNVLinkOKPairCount == 0 && cr.peerNVLinkStatusesSuggestReboot()) {
		cr.suggestedActions = &apiv1.SuggestedActions{
			RepairActions: []apiv1.RepairActionType{
				apiv1.RepairActionTypeRebootSystem,
			},
		}
	}
}

// evaluateHealthStateWithThresholds updates the check result using the configured
// ExpectedLinkStates. This mirrors the InfiniBand threshold evaluation so that
// operators can declare how many GPUs must have NVLink fully active. When all
// links report FEATURE_DISABLED even though NVLink is supported (the scenario
// where nvidia-smi prints "Unable to retrieve NVLink information as all links are inActive"),
// this method surfaces the state as unhealthy. See https://github.com/leptonai/gpud/issues/1085
// for background.
//
// GB200 Architecture Note:
// - GB200 NVL72 has NO direct GPU-to-GPU NVLink connectivity within compute trays
// - All NVLink connections go through external NVSwitches in the rack
// - Each B200 GPU has 18 NVLink ports connecting to NVSwitch chips
// - This threshold measures rack-level switch connectivity, not local GPU links
// - Issue #1085 documented real B200 production failure: "all links are inActive"
// - Root cause unknown (NVSwitch failure, driver issue, or fabric manager problem)
// - The threshold remains valid: it detects when links that SHOULD be active are inactive
// Ref: https://nebius.com/blog/posts/leveraging-nvidia-gb200-nvl72-gpu-interconnect
//
// NVLink Disable Scenarios (Why operators might disable NVLink with multiple GPUs):
// 1. H100x1/A100-80Gx1: Must disable NVLink to run CUDA (per DigitalOcean docs)
// 2. Independent workloads: No benefit when GPUs run separate tasks (hyperparameter search, etc.)
// 3. Benchmarking: Disable to compare performance (via NCCL_P2P_DISABLE=1)
// Ref: https://docs.digitalocean.com/products/paperspace/machines/how-to/manage-nvlink/
func evaluateHealthStateWithThresholds(cr *checkResult) {
	if cr == nil {
		return
	}

	if cr.hasPeerNVLinkP2PFailure() {
		if !cr.hasCompletePeerNVLinkProbeCoverage() {
			// WHY: a subset of `NS` results plus a larger set of probe errors is
			// not equivalent to a confirmed all-`NS` NVLink topology. Keep the
			// peer-P2P failure path conservative and surface the ambiguity in logs
			// instead of failing the whole node from incomplete peer data.
			log.Logger.Warnw(
				"skipping nvlink peer-to-peer failure because peer probe coverage is incomplete",
				"gpuCount", len(cr.NVLinks),
				"expectedPairs", cr.PeerNVLinkExpectedPairCount,
				"probedPairs", cr.PeerNVLinkProbePairCount,
				"missingPairs", cr.missingPeerNVLinkProbePairCount(),
				"observedPeerStatuses", cr.PeerNVLinkObservedStatusCodes,
			)
		} else {
			// WHY: once NVML confirms that every probed GPU pair lacks NVLink P2P
			// connectivity, the node should not remain healthy just because each
			// individual GPU still reports enabled NVLink ports. This must remain
			// true even when operators configure ExpectedLinkStates, because that
			// threshold only counts per-GPU port state and can miss topology-level
			// failures that the peer probe was added to catch.
			cr.health = apiv1.HealthStateTypeUnhealthy
			cr.reason = fmt.Sprintf("no GPU pairs report NVLink P2P connectivity on %d-GPU NVLink-capable system", len(cr.NVLinks))
			cr.reason = appendNVLinkFailureDetails(cr.reason, cr)
			setNVLinkSuggestedActions(cr)
			log.Logger.Warnw(
				"detected system-wide nvlink peer-to-peer failure",
				"gpuCount", len(cr.NVLinks),
				"expectedPairs", cr.PeerNVLinkExpectedPairCount,
				"probedPairs", cr.PeerNVLinkProbePairCount,
				"observedPeerStatuses", cr.PeerNVLinkObservedStatusCodes,
			)
			return
		}
	}

	if cr.ExpectedLinkStates == nil || cr.ExpectedLinkStates.IsZero() {
		// WHY: even without an explicit operator threshold, a multi-GPU
		// NVLink-capable system should not report healthy when *zero* GPUs have
		// active NVLink. A real field report on H100 looked like:
		//
		//   $ nvidia-smi topo -p2p n
		//        GPU0 GPU1 GPU2 GPU3 GPU4 GPU5 GPU6 GPU7
		//   GPU0 X    NS   NS   NS   NS   NS   NS   NS
		//   ...
		//   GPU7 NS   NS   NS   NS   NS   NS   NS   X
		//
		// Legend: `OK` = peer path works, `NS` = not supported. On that host
		// GPUD previously returned healthy because the threshold was unset.
		// This fallback converts only the obvious "all peers broken" case into
		// Unhealthy while still leaving single-GPU systems alone.
		//
		// IMPORTANT: this does NOT make partial degradation unhealthy by default.
		// If 1+ GPUs still have all links active, the implicit fallback stays
		// healthy. Operators must set ExpectedLinkStates when they want GPUD to
		// fail nodes where only some GPUs lost NVLink connectivity.
		//
		// Operators can usually confirm the same failure with:
		//   $ nvidia-smi nvlink -s
		//   Unable to retrieve NVLink information as all links are inActive
		if cr.SystemExpectedNVLink && len(cr.NVLinks) > 0 && len(cr.ActiveNVLinkUUIDs) == 0 {
			cr.health = apiv1.HealthStateTypeUnhealthy
			cr.reason = fmt.Sprintf("no GPUs report active nvlink links on %d-GPU NVLink-capable system", len(cr.NVLinks))
			cr.reason = appendNVLinkFailureDetails(cr.reason, cr)
			setNVLinkSuggestedActions(cr)
			log.Logger.Warnw(
				"detected system-wide nvlink failure without explicit threshold",
				"gpuCount", len(cr.NVLinks),
				"inactiveGPUs", cr.InactiveNVLinkUUIDs,
				"unsupportedGPUs", cr.UnsupportedNVLinkUUIDs,
			)
			return
		}

		if cr.reason == "" {
			cr.reason = reasonNoThresholdConfigured
		}
		log.Logger.Debugw("nvlink threshold evaluation skipped", "expected_link_states", cr.ExpectedLinkStates)
		return
	}

	if len(cr.NVLinks) == 0 {
		cr.health = apiv1.HealthStateTypeHealthy
		cr.reason = reasonNoNVLinkData
		return
	}

	required := cr.ExpectedLinkStates.AtLeastGPUsWithAllLinksFeatureEnabled
	active := len(cr.ActiveNVLinkUUIDs)

	if active >= required {
		cr.health = apiv1.HealthStateTypeHealthy
		cr.reason = fmt.Sprintf("nvlink threshold satisfied: require >=%d GPUs with all links active; got %d", required, active)
		log.Logger.Debugw("nvlink threshold satisfied", "required", required, "active", active)
		return
	}

	cr.health = apiv1.HealthStateTypeUnhealthy
	cr.reason = fmt.Sprintf("nvlink threshold violated: require >=%d GPUs with all links active; got %d", required, active)
	log.Logger.Warnw(cr.reason, "requiredGPUs", required, "activeGPUs", active)
	cr.reason = appendNVLinkFailureDetails(cr.reason, cr)
	setNVLinkSuggestedActions(cr)
}
