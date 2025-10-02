package nvlink

import (
	"fmt"
	"strings"

	apiv1 "github.com/leptonai/gpud/api/v1"
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
func evaluateHealthStateWithThresholds(cr *checkResult) {
	if cr == nil {
		return
	}

	thresholds := ExpectedLinkStates{}
	if cr.ExpectedLinkStates != nil {
		thresholds = *cr.ExpectedLinkStates
	}

	if thresholds.IsZero() {
		if cr.reason == "" {
			cr.reason = reasonNoThresholdConfigured
		}
		return
	}

	if len(cr.NVLinks) == 0 {
		cr.health = apiv1.HealthStateTypeHealthy
		cr.reason = reasonNoNVLinkData
		return
	}

	required := thresholds.AtLeastGPUsWithAllLinksFeatureEnabled
	active := 0
	for _, nv := range cr.NVLinks {
		if !nv.Supported {
			continue
		}
		if len(nv.States) == 0 {
			continue
		}
		if nv.States.AllFeatureEnabled() {
			active++
		}
	}

	if active >= required {
		cr.health = apiv1.HealthStateTypeHealthy
		cr.reason = fmt.Sprintf("nvlink threshold satisfied: require >=%d GPUs with all links active; got %d", required, active)
		return
	}

	cr.health = apiv1.HealthStateTypeUnhealthy
	detailParts := []string{}
	if len(cr.InactiveNVLinkUUIDs) > 0 {
		detailParts = append(detailParts, fmt.Sprintf("inactive nvlinks=%s", strings.Join(cr.InactiveNVLinkUUIDs, ",")))
	}
	if len(cr.UnsupportedNVLinkUUIDs) > 0 {
		detailParts = append(detailParts, fmt.Sprintf("unsupported nvlinks=%s", strings.Join(cr.UnsupportedNVLinkUUIDs, ",")))
	}
	if len(detailParts) > 0 {
		cr.reason = fmt.Sprintf("nvlink threshold violated: require >=%d GPUs with all links active; got %d (%s)", required, active, strings.Join(detailParts, "; "))
		return
	}
	cr.reason = fmt.Sprintf("nvlink threshold violated: require >=%d GPUs with all links active; got %d", required, active)
}
