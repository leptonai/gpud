package query

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func Test_countSMINVSwitches_A10(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	command := "cat ./testdata/nvidia-smi.nvlink.status.a10"

	lines, err := countSMINVSwitches(ctx, command)
	require.NoError(t, err)
	require.Equal(t, 0, len(lines))
}

func Test_countSMINVSwitches_A100(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	command := "cat ./testdata/nvidia-smi.nvlink.status.a100"

	lines, err := countSMINVSwitches(ctx, command)
	require.NoError(t, err)
	require.Equal(t, 8, len(lines))
	require.Contains(t, lines[0], "NVIDIA")
}
