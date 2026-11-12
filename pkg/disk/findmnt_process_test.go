package disk

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestFindMntWithCommandConcurrentOutput(t *testing.T) {
	command := filepath.Join(t.TempDir(), "findmnt")
	// Exercise the real process and JSON parsing path with a short-lived
	// command, including the flags appended to command overrides.
	require.NoError(t, os.WriteFile(command, []byte(`#!/bin/sh
[ "$*" = "--target /mnt/nfs-share --json --df" ] || exit 1
printf '%s\n' '{"filesystems":[{"source":"example:/export","target":"/mnt/nfs-share","fstype":"nfs4","size":"100G","used":"1G","avail":"99G","use%":"1%"}]}'
`), 0755))

	// The nested group keeps the temporary executable alive until all
	// parallel workers finish.
	t.Run("workers", func(t *testing.T) {
		for worker := 0; worker < 16; worker++ {
			t.Run("query", func(t *testing.T) {
				t.Parallel()
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				for query := 0; query < 16; query++ {
					out, err := FindMntWithCommand(ctx, "/mnt/nfs-share", command)
					require.NoError(t, err)
					require.Len(t, out.Filesystems, 1)
					require.Equal(t, "/mnt/nfs-share", out.Target)
					require.Equal(t, "/mnt/nfs-share", out.Filesystems[0].MountedPoint)
					require.Equal(t, []string{"example:/export"}, out.Filesystems[0].Sources)
					require.Equal(t, "nfs4", out.Filesystems[0].Fstype)
				}
			})
		}
	})
}
