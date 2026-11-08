package v1

// Evaluation distinguishes an observed health result from unavailable evidence.
// A nil Evaluated field identifies a producer that predates this contract.
type Evaluation struct {
	Evaluated          *bool  `json:"evaluated,omitempty"`
	NotEvaluatedReason string `json:"not_evaluated_reason,omitempty"`
}

const (
	NotEvaluatedExpectedValues = "no_expected_values"
	NotEvaluatedUnsupported    = "not_supported_on_product"
	NotEvaluatedPortStates     = "no_expected_port_states"
	NotEvaluatedLinkStates     = "no_expected_link_states"
	NotEvaluatedNoData         = "no_data_yet"
	NotEvaluatedCollection     = "collection_failed"
	NotEvaluatedPartial        = "partial_data"
)

func Evaluated() Evaluation {
	value := true
	return Evaluation{Evaluated: &value}
}

func NotEvaluated(reason string) Evaluation {
	value := false
	return Evaluation{Evaluated: &value, NotEvaluatedReason: reason}
}
