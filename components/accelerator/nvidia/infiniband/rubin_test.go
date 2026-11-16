package infiniband

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRubinRequiresExplicitNICInventory(t *testing.T) {
	for _, name := range []string{"NVIDIA Vera Rubin NVL72", "NVIDIA-Vera-Rubin-NVL72", "NVIDIA Graphics Device", "NVIDIA Rubin", "NVIDIA Vera Rubin", "NVIDIA DGX Rubin NVL8", "NVIDIA Vera Rubin NVL576"} {
		_, err := SupportsInfinibandPortRate(name)
		require.ErrorIs(t, err, ErrNoExpectedPortStates)
	}
}
