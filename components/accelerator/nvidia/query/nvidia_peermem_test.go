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
