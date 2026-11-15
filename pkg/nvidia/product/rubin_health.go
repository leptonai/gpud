package product

import "os"

// EnvEnableRubinHealthRules opts in to unqualified Rubin remediation rules.
// Leave unset during bring-up: raw link, configuration and repair telemetry is
// still collected, but these new signals alone must not trigger remediation.
const EnvEnableRubinHealthRules = "GPUD_ENABLE_RUBIN_HEALTH_RULES"

// RubinHealthRulesEnabled should be enabled only after production-tray scans
// and controlled fault qualification establish the expected recovery behavior.
func RubinHealthRulesEnabled() bool {
	return os.Getenv(EnvEnableRubinHealthRules) == "true"
}
