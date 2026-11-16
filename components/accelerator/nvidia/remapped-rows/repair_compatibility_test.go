package remappedrows

import (
	"testing"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/stretchr/testify/require"
)

func TestRepairVersionMismatchIsUnavailable(t *testing.T) {
	for _, query := range []string{"bank remapper", "repair status", "unrepairable memory flag"} {
		supported, err := memoryRepairQueryResult(query, nvml.ERROR_ARGUMENT_VERSION_MISMATCH)
		require.NoError(t, err)
		require.False(t, supported)
	}
}
