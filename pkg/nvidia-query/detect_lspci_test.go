package query

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func Test_listNVIDIAPCIs_A10(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	command := "cat ./testdata/lspci.nn.a10"

	lines, err := listPCIs(ctx, command, isNVIDIAGPUPCI)
	require.NoError(t, err)
	require.Equal(t, 1, len(lines))
	require.Contains(t, lines[0], "NVIDIA")

	lines, err = listPCIs(ctx, command, isNVIDIANVSwitchPCI)
	require.NoError(t, err)
	require.Equal(t, 0, len(lines))
}

func Test_listNVIDIAPCIs_A100(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	command := "cat ./testdata/lspci.nn.a100"

	lines, err := listPCIs(ctx, command, isNVIDIAGPUPCI)
	require.NoError(t, err)
	require.Equal(t, 8, len(lines))
	require.Contains(t, lines[0], "NVIDIA")

	lines, err = listPCIs(ctx, command, isNVIDIANVSwitchPCI)
	require.NoError(t, err)
	require.Equal(t, 6, len(lines))
	require.Contains(t, lines[0], "NVIDIA")
}
