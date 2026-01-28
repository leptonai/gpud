package informer

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bytedance/mockey"
	"github.com/fsnotify/fsnotify"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/leptonai/gpud/pkg/gpud-manager/packages"
)

func TestResolvePackage_StatErrorWithMockey(t *testing.T) {
	mockey.PatchConvey("resolvePackage returns stat error", t, func() {
		mockey.Mock((*exec.Cmd).CombinedOutput).To(func(cmd *exec.Cmd) ([]byte, error) {
			if len(cmd.Args) > 0 && cmd.Args[0] == "stat" {
				return nil, errors.New("stat failed")
			}
			return []byte(""), nil
		}).Build()

		_, _, _, err := resolvePackage("/tmp/init.sh")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "stat failed")
	})
}

func TestResolvePackage_VersionErrorWithMockey(t *testing.T) {
	mockey.PatchConvey("resolvePackage returns version grep error", t, func() {
		mockey.Mock((*exec.Cmd).CombinedOutput).To(func(cmd *exec.Cmd) ([]byte, error) {
			if len(cmd.Args) > 0 && cmd.Args[0] == "stat" {
				return []byte(""), nil
			}
			if len(cmd.Args) >= 3 && cmd.Args[0] == "bash" && strings.Contains(cmd.Args[2], "GPUD_PACKAGE_VERSION") {
				return []byte(""), errors.New("grep version failed")
			}
			return []byte(""), nil
		}).Build()

		_, _, _, err := resolvePackage("/tmp/init.sh")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "get version failed")
	})
}

func TestHandleFileEvent_CreateDirectoryWithMockey(t *testing.T) {
	mockey.PatchConvey("handleFileEvent adds directory watch on create", t, func() {
		tempDir := t.TempDir()
		watcher, err := fsnotify.NewWatcher()
		require.NoError(t, err)
		defer func() {
			_ = watcher.Close()
		}()

		called := false
		mockey.Mock(addDirectory).To(func(w *fsnotify.Watcher, dir string) error {
			called = true
			assert.Equal(t, tempDir, dir)
			return nil
		}).Build()

		fi := &fileInformer{packagesDir: tempDir, rootDir: tempDir}
		fi.handleFileEvent(watcher, fsnotify.Event{Name: tempDir, Op: fsnotify.Create}, make(chan packages.PackageInfo))

		assert.True(t, called)
	})
}

func TestHandleFileEvent_WriteInitShWithMockey(t *testing.T) {
	mockey.PatchConvey("handleFileEvent parses init.sh writes", t, func() {
		rootDir := t.TempDir()
		packagesDir := filepath.Join(rootDir, "packages")
		require.NoError(t, os.MkdirAll(filepath.Join(packagesDir, "pkg1"), 0755))

		mockey.Mock(resolvePackage).To(func(path string) (string, [][]string, time.Duration, error) {
			return "1.2.3", [][]string{{"dep", "1.0.0"}}, 10 * time.Second, nil
		}).Build()

		fi := &fileInformer{packagesDir: packagesDir, rootDir: rootDir}
		ch := make(chan packages.PackageInfo, 1)
		initPath := filepath.Join(packagesDir, "pkg1", "init.sh")

		fi.handleFileEvent(nil, fsnotify.Event{Name: initPath, Op: fsnotify.Write}, ch)

		select {
		case info := <-ch:
			assert.Equal(t, "pkg1", info.Name)
			assert.Equal(t, initPath, info.ScriptPath)
			assert.Equal(t, "1.2.3", info.TargetVersion)
			assert.Equal(t, time.Second*10, info.TotalTime)
		default:
			t.Fatal("expected package info to be sent")
		}
	})
}

func TestProcessInitialPackages_WithMockey(t *testing.T) {
	mockey.PatchConvey("processInitialPackages emits package info", t, func() {
		fi := &fileInformer{packagesDir: "/tmp/packages"}

		mockey.Mock((*fileInformer).listPackages).To(func(_ *fileInformer) ([]byte, error) {
			return []byte("pkgA\npkgB\n"), nil
		}).Build()
		mockey.Mock(resolvePackage).To(func(path string) (string, [][]string, time.Duration, error) {
			return "0.1.0", nil, 0, nil
		}).Build()

		ch := make(chan packages.PackageInfo, 2)
		fi.processInitialPackages(ch)

		require.Len(t, ch, 2)
		first := <-ch
		second := <-ch
		assert.Equal(t, "pkgA", first.Name)
		assert.Equal(t, "pkgB", second.Name)
	})
}
