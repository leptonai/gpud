package fuse

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeConnectionFiles(t *testing.T, dir, congestionThreshold, maxBackground, waiting string) {
	t.Helper()

	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "congestion_threshold"), []byte(congestionThreshold), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "max_background"), []byte(maxBackground), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "waiting"), []byte(waiting), 0o644))
}

func TestListConnectionsNonDirectoryEntriesIgnored(t *testing.T) {
	base := t.TempDir()

	writeConnectionFiles(t, filepath.Join(base, "42"), "100\n", "200\n", "10\n")
	require.NoError(t, os.WriteFile(filepath.Join(base, "not-a-dir"), []byte("ignore me"), 0o644))

	infos, err := listConnections(base)
	require.NoError(t, err)
	require.Len(t, infos, 1)
	assert.Equal(t, 42, infos[0].Device)
}

func TestListConnectionsErrors(t *testing.T) {
	tests := []struct {
		name       string
		build      func(t *testing.T, root string)
		errSnippet string
	}{
		{
			name: "invalid device directory name",
			build: func(t *testing.T, root string) {
				writeConnectionFiles(t, filepath.Join(root, "abc"), "100\n", "200\n", "10\n")
			},
			errSnippet: "invalid syntax",
		},
		{
			name: "missing waiting file",
			build: func(t *testing.T, root string) {
				dir := filepath.Join(root, "12")
				require.NoError(t, os.MkdirAll(dir, 0o755))
				require.NoError(t, os.WriteFile(filepath.Join(dir, "congestion_threshold"), []byte("100\n"), 0o644))
				require.NoError(t, os.WriteFile(filepath.Join(dir, "max_background"), []byte("200\n"), 0o644))
			},
			errSnippet: "waiting",
		},
		{
			name: "invalid waiting content",
			build: func(t *testing.T, root string) {
				writeConnectionFiles(t, filepath.Join(root, "13"), "100\n", "200\n", "not-an-int\n")
			},
			errSnippet: "invalid syntax",
		},
		{
			name: "invalid congestion threshold content",
			build: func(t *testing.T, root string) {
				writeConnectionFiles(t, filepath.Join(root, "14"), "bad\n", "200\n", "10\n")
			},
			errSnippet: "invalid syntax",
		},
		{
			name: "invalid max background content",
			build: func(t *testing.T, root string) {
				writeConnectionFiles(t, filepath.Join(root, "15"), "100\n", "bad\n", "10\n")
			},
			errSnippet: "invalid syntax",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			base := t.TempDir()
			tt.build(t, base)

			_, err := listConnections(base)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.errSnippet)
		})
	}
}

func TestListConnectionsWithFinderMixedResults(t *testing.T) {
	base := t.TempDir()
	writeConnectionFiles(t, filepath.Join(base, "100"), "100\n", "200\n", "10\n")
	writeConnectionFiles(t, filepath.Join(base, "101"), "150\n", "300\n", "15\n")

	finder := func(minor int) (string, string, error) {
		if minor == 100 {
			return "", "", fmt.Errorf("simulated lookup failure")
		}
		return "fuse.test", "test-device", nil
	}

	infos, err := listConnectionsWithFinder(finder, base)
	require.NoError(t, err)
	require.Len(t, infos, 2)

	byDevice := make(map[int]ConnectionInfo, len(infos))
	for _, info := range infos {
		byDevice[info.Device] = info
	}

	assert.Empty(t, byDevice[100].Fstype)
	assert.Empty(t, byDevice[100].DeviceName)
	assert.Equal(t, "fuse.test", byDevice[101].Fstype)
	assert.Equal(t, "test-device", byDevice[101].DeviceName)
}
