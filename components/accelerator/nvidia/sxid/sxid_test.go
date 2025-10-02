package sxid

import "testing"

func TestDetailsValidation(t *testing.T) {
	for _, d := range details {
		// test critical/fatal errors must have repair actions
		if (d.EventType == "critical" || d.EventType == "fatal") && (d.SuggestedActionsByGPUd == nil || len(d.SuggestedActionsByGPUd.RepairActions) == 0) {
			t.Errorf("sxid %d has critical/fatal event type, but has no repair actions", d.SXid)
		}
		if d.SuggestedActionsByGPUd == nil {
			continue
		}
	}
}
