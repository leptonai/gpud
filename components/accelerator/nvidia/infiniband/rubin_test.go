package infiniband

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/leptonai/gpud/components/accelerator/nvidia/infiniband/types"
)

func TestRubinReferenceTrayPorts(t *testing.T) {
	for _, name := range []string{"NVIDIA Vera Rubin NVL72", "NVIDIA-Vera-Rubin-NVL72"} {
		ports, err := SupportsInfinibandPortRate(name)
		require.NoError(t, err)
		require.Equal(t, types.ExpectedPortStates{AtLeastPorts: 8, AtLeastRate: 800}, ports)
	}
	// DGX Rubin NVL8 is a documented Rubin system with a different NIC layout.
	for _, name := range []string{"NVIDIA Graphics Device", "NVIDIA Rubin", "NVIDIA DGX Rubin NVL8"} {
		_, err := SupportsInfinibandPortRate(name)
		require.ErrorIs(t, err, ErrNoExpectedPortStates)
	}
}
