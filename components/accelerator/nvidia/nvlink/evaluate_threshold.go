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

	if cr.ExpectedLinkStates == nil || cr.ExpectedLinkStates.IsZero() {
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

	detailParts := []string{}
	if len(cr.InactiveNVLinkUUIDs) > 0 {
		detailParts = append(detailParts, fmt.Sprintf("inactive nvlinks=%s", strings.Join(cr.InactiveNVLinkUUIDs, ",")))
	}
	if len(cr.UnsupportedNVLinkUUIDs) > 0 {
		detailParts = append(detailParts, fmt.Sprintf("unsupported nvlinks=%s", strings.Join(cr.UnsupportedNVLinkUUIDs, ",")))
	}
	if len(detailParts) > 0 {
		cr.reason = fmt.Sprintf("%s (%s)", cr.reason, strings.Join(detailParts, "; "))
	}

	if cr.suggestedActions == nil && len(cr.InactiveNVLinkUUIDs) > 0 {
		cr.suggestedActions = &apiv1.SuggestedActions{
			RepairActions: []apiv1.RepairActionType{
				apiv1.RepairActionTypeRebootSystem,
			},
		}
	}
}
