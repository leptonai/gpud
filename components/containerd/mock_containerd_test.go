package containerd

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/bytedance/mockey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pkgfile "github.com/leptonai/gpud/pkg/file"
)

func TestCheckContainerdInstalledOnHost_NoHostRootUsesPATH(t *testing.T) {
	missingRoot := filepath.Join(t.TempDir(), "does-not-exist")
	missingSocket := filepath.Join(t.TempDir(), "containerd.sock")

	mockey.PatchConvey("without a host root, PATH decides", t, func() {
		mockey.Mock(pkgfile.LocateExecutable).To(func(_ string) (string, error) {
			return "/usr/bin/containerd", nil
		}).Build()
		assert.True(t, checkContainerdInstalledOnHost(missingRoot, missingSocket))
	})

	mockey.PatchConvey("without a host root, a socket alone does not count", t, func() {
		mockey.Mock(pkgfile.LocateExecutable).To(func(_ string) (string, error) {
			return "", errors.New("not found")
		}).Build()
		socket := listenUnix(t)
		assert.False(t, checkContainerdInstalledOnHost(missingRoot, socket))
	})
}

func TestCheckContainerdInstalledOnHost_HostRoot(t *testing.T) {
	missingSocket := filepath.Join(t.TempDir(), "containerd.sock")

	mockey.PatchConvey("a containerd in the gpud image PATH does not count", t, func() {
		mockey.Mock(pkgfile.LocateExecutable).To(func(_ string) (string, error) {
			return "/usr/bin/containerd", nil
		}).Build()
		assert.False(t, checkContainerdInstalledOnHost(t.TempDir(), missingSocket))
	})

	for _, dir := range []string{"usr/bin", "usr/local/bin", "usr/sbin", "bin", "sbin"} {
		t.Run("binary in "+dir, func(t *testing.T) {
			root := t.TempDir()
			writeExecutable(t, filepath.Join(root, dir, "containerd"))
			assert.True(t, checkContainerdInstalledOnHost(root, missingSocket))

			p, err := locateContainerd(root)
			require.NoError(t, err)
			assert.Equal(t, filepath.Join(root, dir, "containerd"), p)
		})
	}

	t.Run("directory named containerd does not count", func(t *testing.T) {
		root := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(root, "usr", "bin", "containerd"), 0o755))
		assert.False(t, checkContainerdInstalledOnHost(root, missingSocket))

		_, err := locateContainerd(root)
		assert.ErrorIs(t, err, errContainerdNotFound)
	})

	t.Run("socket without a standard binary counts", func(t *testing.T) {
		assert.True(t, checkContainerdInstalledOnHost(t.TempDir(), listenUnix(t)))
	})
}

func writeExecutable(t *testing.T, path string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte("#!/bin/sh\n"), 0o755))
}

// listenUnix returns the path of a live unix socket. It uses a short directory
// because unix socket paths are limited to ~104 bytes on darwin.
func listenUnix(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "ctrd")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	p := filepath.Join(dir, "c.sock")
	l, err := net.Listen("unix", p)
	require.NoError(t, err)
	t.Cleanup(func() { _ = l.Close() })
	return p
}
