package remappedrows

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRepairTableDistinguishesUnavailableTelemetry(t *testing.T) {
	for _, tc := range []struct {
		name        string
		status      MemoryRepairStatus
		unavailable int
	}{
		{name: "all APIs unavailable", unavailable: 7},
		{name: "only bank query succeeded", status: MemoryRepairStatus{BankRemapperSupported: true}, unavailable: 3},
		{name: "only repair query succeeded", status: MemoryRepairStatus{RepairStatusSupported: true}, unavailable: 5},
		{name: "only unrepairable query succeeded", status: MemoryRepairStatus{UnrepairableMemorySupported: true}, unavailable: 6},
		{name: "healthy supported zero values", status: MemoryRepairStatus{BankRemapperSupported: true, RepairStatusSupported: true, UnrepairableMemorySupported: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.status.UUID = "GPU-rubin"
			cr := &checkResult{MemoryRepairStatuses: []MemoryRepairStatus{tc.status}}
			output := cr.String()
			require.Contains(t, output, "GPU-rubin")
			require.Equal(t, tc.unavailable, strings.Count(output, "N/A"), output)
			if tc.unavailable == 0 {
				require.Contains(t, output, "false")
			}
		})
	}
}
