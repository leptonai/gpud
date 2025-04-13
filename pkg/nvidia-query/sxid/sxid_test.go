package sxid

import "testing"

func TestDetailsValidation(t *testing.T) {
	for _, d := range details {
		// test critical errors must have repair actions
		if d.CriticalErrorMarkedByGPUd && len(d.SuggestedActionsByGPUd.RepairActions) == 0 {
			t.Errorf("sxid %d is marked as critical in GPUd, but has no repair actions", d.SXid)
		}
		if d.SuggestedActionsByGPUd == nil {
			continue
		}

		if len(d.SuggestedActionsByGPUd.DeprecatedDescriptions) > 0 &&
			len(d.SuggestedActionsByGPUd.DeprecatedDescriptions) != len(d.SuggestedActionsByGPUd.RepairActions) {
			t.Errorf("sxid %d has %d descriptions and %d repair actions",
				d.SXid,
				len(d.SuggestedActionsByGPUd.DeprecatedDescriptions),
				len(d.SuggestedActionsByGPUd.RepairActions))
		}
	}
}
