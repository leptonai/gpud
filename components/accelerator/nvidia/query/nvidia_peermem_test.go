package query

import "testing"

func TestHasLsmodInfinibandPeerMem(t *testing.T) {
	t.Parallel()

	input := `

	nvidia_peermem         16384  0
	ib_core               434176  9 rdma_cm,ib_ipoib,nvidia_peermem,iw_cm,ib_umad,rdma_ucm,ib_uverbs,mlx5_ib,ib_cm
	nvidia              56717312  447 nvidia_uvm,nvidia_peermem,nvidia_modeset

`

	if !HasLsmodInfinibandPeerMem(input) {
		t.Fatal("expected true, got false")
	}

	input = `

	nvidia_peermem         16384  0
	ib_core               434176  9 rdma_cm,ib_ipoib,iw_cm,ib_umad,rdma_ucm,ib_uverbs,mlx5_ib,ib_cm
	nvidia              56717312  447 nvidia_uvm,nvidia_peermem,nvidia_modeset

`

	if HasLsmodInfinibandPeerMem(input) {
		t.Fatal("expected false, got true")
	}
}

func TestIsIbcoreExpected(t *testing.T) {
	tests := []struct {
		name                  string
		gpuProductName        string
		ibstatExists          bool
		infinibandClassExists bool
		expectedResult        bool
	}{
		{
			name:                  "H100 SXM with ibstat and infiniband",
			gpuProductName:        "NVIDIA H100 SXM",
			ibstatExists:          true,
			infinibandClassExists: true,
			expectedResult:        true,
		},
		{
			name:                  "A100 SXM with ibstat and infiniband",
			gpuProductName:        "NVIDIA A100 SXM",
			ibstatExists:          true,
			infinibandClassExists: true,
			expectedResult:        true,
		},
		{
			name:                  "H100 PCIe with ibstat and infiniband",
			gpuProductName:        "NVIDIA H100 PCIe",
			ibstatExists:          true,
			infinibandClassExists: true,
			expectedResult:        false,
		},
		{
			name:                  "A100 SXM without ibstat",
			gpuProductName:        "NVIDIA A100 SXM",
			ibstatExists:          false,
			infinibandClassExists: true,
			expectedResult:        false,
		},
		{
			name:                  "H100 SXM without infiniband class",
			gpuProductName:        "NVIDIA H100 SXM",
			ibstatExists:          true,
			infinibandClassExists: false,
			expectedResult:        false,
		},
		{
			name:                  "Other GPU with ibstat and infiniband",
			gpuProductName:        "NVIDIA RTX 3090",
			ibstatExists:          true,
			infinibandClassExists: true,
			expectedResult:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsIbcoreExpected(tt.gpuProductName, tt.ibstatExists, tt.infinibandClassExists)
			if result != tt.expectedResult {
				t.Errorf("IsIbcoreExpected(%q, %v, %v) = %v, want %v",
					tt.gpuProductName, tt.ibstatExists, tt.infinibandClassExists, result, tt.expectedResult)
			}
		})
	}
}
