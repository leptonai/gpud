package query

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func Test_listNVIDIAPCIs(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	command := "cat ./testdata/lspci.gb200"
	lines, err := listNVIDIAPCIs(ctx, command)
	require.NoError(t, err)
	require.Equal(t, 4, len(lines))
	require.Contains(t, lines[0], "NVIDIA")
}
