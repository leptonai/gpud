package disk

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseFlatten(t *testing.T) {
	t.Parallel()

	for _, f := range []string{
		"lsblk.1.json",
		"lsblk.2.json",
		"lsblk.3.json",
	} {
		dat, err := os.ReadFile("testdata/" + f)
		require.NoError(t, err)

		blks, err := parseLsblkJSON(dat)
		require.NoError(t, err)

		blks.Flatten().RenderTable(os.Stdout)
	}
}
