package disk

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseFlatten(t *testing.T) {
	t.Parallel()

	for f, expectedDevs := range map[string]int{
		"lsblk.1.json": 23,
		"lsblk.2.json": 10,
		"lsblk.3.json": 20,
		"lsblk.4.json": 39,
	} {
		dat, err := os.ReadFile("testdata/" + f)
		require.NoError(t, err)

		blks, err := parseLsblkJSON(dat)
		require.NoError(t, err)

		flattened := blks.Flatten()
		require.Equal(t, expectedDevs, len(flattened))

		flattened.RenderTable(os.Stdout)
	}
}
