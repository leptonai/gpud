package file

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/bytedance/mockey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeFileInfo struct {
	name  string
	mode  os.FileMode
	isDir bool
}

func (f fakeFileInfo) Name() string       { return f.name }
func (f fakeFileInfo) Size() int64        { return 0 }
func (f fakeFileInfo) Mode() os.FileMode  { return f.mode }
func (f fakeFileInfo) ModTime() time.Time { return time.Time{} }
func (f fakeFileInfo) IsDir() bool        { return f.isDir }
func (f fakeFileInfo) Sys() any           { return nil }

func TestLocateExecutable_LookPathErrorWithMockey(t *testing.T) {
	mockey.PatchConvey("LocateExecutable returns LookPath error", t, func() {
		mockey.Mock(exec.LookPath).To(func(file string) (string, error) {
			return "", errors.New("not found")
		}).Build()

		path, err := LocateExecutable("missing")
		require.Error(t, err)
		assert.Empty(t, path)
		assert.Contains(t, err.Error(), "not found")
	})
}

func TestLocateExecutable_CheckExecutableErrorsWithMockey(t *testing.T) {
	mockey.PatchConvey("LocateExecutable returns CheckExecutable error", t, func() {
		mockey.Mock(exec.LookPath).To(func(file string) (string, error) {
			return "/tmp/fake-bin", nil
		}).Build()

		mockey.Mock(os.Stat).To(func(name string) (os.FileInfo, error) {
			return fakeFileInfo{name: "fake-bin", mode: os.ModeDir, isDir: true}, nil
		}).Build()

		path, err := LocateExecutable("fake-bin")
		require.Error(t, err)
		assert.Equal(t, "/tmp/fake-bin", path)
		assert.Contains(t, err.Error(), "directory")
	})

	mockey.PatchConvey("CheckExecutable returns not executable", t, func() {
		mockey.Mock(os.Stat).To(func(name string) (os.FileInfo, error) {
			return fakeFileInfo{name: "fake-bin", mode: 0644, isDir: false}, nil
		}).Build()

		err := CheckExecutable("/tmp/fake-bin")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not executable")
	})
}

func TestGetLimitAndFileHandlesErrorsWithMockey(t *testing.T) {
	mockey.PatchConvey("getLimit read error", t, func() {
		mockey.Mock(os.ReadFile).To(func(name string) ([]byte, error) {
			return nil, errors.New("read failed")
		}).Build()

		_, err := getLimit("/proc/sys/fs/file-max")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "read failed")
	})

	mockey.PatchConvey("getFileHandles invalid fields", t, func() {
		mockey.Mock(os.ReadFile).To(func(name string) ([]byte, error) {
			return []byte("1 2"), nil
		}).Build()

		_, _, err := getFileHandles("/proc/sys/fs/file-nr")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unexpected number of fields")
	})
}

func TestFileAndDirectoryExistsErrorWithMockey(t *testing.T) {
	mockey.PatchConvey("directoryExists returns stat error", t, func() {
		mockey.Mock(os.Stat).To(func(name string) (os.FileInfo, error) {
			return nil, errors.New("stat failed")
		}).Build()

		exists, err := directoryExists("/tmp/dir")
		require.Error(t, err)
		assert.False(t, exists)
	})

	mockey.PatchConvey("fileExists returns stat error", t, func() {
		mockey.Mock(os.Stat).To(func(name string) (os.FileInfo, error) {
			return nil, errors.New("stat failed")
		}).Build()

		exists, err := fileExists("/tmp/file")
		require.Error(t, err)
		assert.False(t, exists)
	})
}

func TestLocateLib_EvalSymlinksErrorWithMockey(t *testing.T) {
	mockey.PatchConvey("locateLib handles EvalSymlinks error", t, func() {
		tmpDir, err := os.MkdirTemp("", "lib-test")
		require.NoError(t, err)
		defer func() { _ = os.RemoveAll(tmpDir) }()

		libPath := tmpDir + string(os.PathSeparator) + "libtest.so"
		require.NoError(t, os.WriteFile(libPath, []byte("x"), 0644))

		mockey.Mock(filepath.EvalSymlinks).To(func(path string) (string, error) {
			return "", errors.New("eval failed")
		}).Build()

		resolved, err := findLibrary(map[string]any{tmpDir: struct{}{}}, "libtest.so", nil)
		require.Error(t, err)
		assert.Equal(t, ErrLibraryNotFound, err)
		assert.Empty(t, resolved)
	})
}

func TestGetFileHandles_ParseErrorsWithMockey(t *testing.T) {
	mockey.PatchConvey("getFileHandles parse error allocated", t, func() {
		mockey.Mock(os.ReadFile).To(func(name string) ([]byte, error) {
			return []byte("abc 1 2"), nil
		}).Build()

		_, _, err := getFileHandles("/proc/sys/fs/file-nr")
		require.Error(t, err)
	})

	mockey.PatchConvey("getFileHandles parse error unused", t, func() {
		mockey.Mock(os.ReadFile).To(func(name string) ([]byte, error) {
			return []byte("1 abc 2"), nil
		}).Build()

		_, _, err := getFileHandles("/proc/sys/fs/file-nr")
		require.Error(t, err)
	})
}
