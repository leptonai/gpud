package xid

import "testing"

func TestDetailsValidation(t *testing.T) {
	for _, d := range details {
		// test critical errors must have repair actions
		if d.CriticalErrorMarkedByGPUd && len(d.SuggestedActionsByGPUd.RepairActions) == 0 {
			t.Errorf("xid %d is marked as critical in GPUd, but has no repair actions", d.Xid)
		}
		if d.SuggestedActionsByGPUd == nil {
			continue
		}

		if len(d.SuggestedActionsByGPUd.Descriptions) > 0 &&
			len(d.SuggestedActionsByGPUd.Descriptions) != len(d.SuggestedActionsByGPUd.RepairActions) {
			t.Errorf("xid %d has %d descriptions and %d repair actions",
				d.Xid,
				len(d.SuggestedActionsByGPUd.Descriptions),
				len(d.SuggestedActionsByGPUd.RepairActions))
		}
	}
}
