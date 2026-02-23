package gpudmanager

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/bytedance/mockey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pkgconfig "github.com/leptonai/gpud/pkg/config"
	"github.com/leptonai/gpud/pkg/gpud-manager/controllers"
	"github.com/leptonai/gpud/pkg/gpud-manager/informer"
	"github.com/leptonai/gpud/pkg/gpud-manager/packages"
)

func TestNew(t *testing.T) {
	manager, err := New("/tmp/test-data-dir")
	require.NoError(t, err)
	assert.NotNil(t, manager)
	assert.Equal(t, "/tmp/test-data-dir", manager.dataDir)
}

func TestNew_EmptyDataDir(t *testing.T) {
	manager, err := New("")
	require.NoError(t, err)
	assert.NotNil(t, manager)
	assert.Equal(t, "", manager.dataDir)
}

func TestManager_Start_ResolveDataDirError(t *testing.T) {
	mockey.PatchConvey("resolve data dir error", t, func() {
		mockey.Mock(pkgconfig.ResolveDataDir).To(func(dataDir string) (string, error) {
			return "", errors.New("failed to resolve data dir")
		}).Build()

		manager, err := New("/tmp/test")
		require.NoError(t, err)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		err = manager.Start(ctx)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to resolve data dir")
	})
}

func TestManager_Start_Success(t *testing.T) {
	mockey.PatchConvey("start success", t, func() {
		tmpDir := t.TempDir()

		mockey.Mock(pkgconfig.ResolveDataDir).To(func(dataDir string) (string, error) {
			return tmpDir, nil
		}).Build()

		ch := make(chan packages.PackageInfo)
		mockey.Mock(informer.NewFileInformer).To(func(dataDir string) chan packages.PackageInfo {
			return ch
		}).Build()

		mockController := controllers.NewPackageController(ch)
		mockey.Mock(controllers.NewPackageController).To(func(watcher chan packages.PackageInfo) *controllers.PackageController {
			return mockController
		}).Build()

		manager, err := New("/tmp/test")
		require.NoError(t, err)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		err = manager.Start(ctx)
		require.NoError(t, err)
		assert.Equal(t, tmpDir, manager.dataDir)
		assert.Equal(t, mockController, manager.packageController)
		assert.Equal(t, mockController, GlobalController)
	})
}

func TestManager_Start_WithEmptyDataDir(t *testing.T) {
	mockey.PatchConvey("start with empty data dir", t, func() {
		tmpDir := t.TempDir()

		mockey.Mock(pkgconfig.ResolveDataDir).To(func(dataDir string) (string, error) {
			// When empty dataDir is passed, DefaultDataDir is used
			if dataDir == pkgconfig.DefaultDataDir {
				return tmpDir, nil
			}
			return dataDir, nil
		}).Build()

		ch := make(chan packages.PackageInfo)
		mockey.Mock(informer.NewFileInformer).To(func(dataDir string) chan packages.PackageInfo {
			return ch
		}).Build()

		mockController := controllers.NewPackageController(ch)
		mockey.Mock(controllers.NewPackageController).To(func(watcher chan packages.PackageInfo) *controllers.PackageController {
			return mockController
		}).Build()

		manager, err := New("")
		require.NoError(t, err)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		err = manager.Start(ctx)
		require.NoError(t, err)
		assert.Equal(t, tmpDir, manager.dataDir)
	})
}

func TestManager_Status(t *testing.T) {
	mockey.PatchConvey("status", t, func() {
		tmpDir := t.TempDir()

		mockey.Mock(pkgconfig.ResolveDataDir).To(func(dataDir string) (string, error) {
			return tmpDir, nil
		}).Build()

		ch := make(chan packages.PackageInfo)
		mockey.Mock(informer.NewFileInformer).To(func(dataDir string) chan packages.PackageInfo {
			return ch
		}).Build()

		mockController := controllers.NewPackageController(ch)
		mockey.Mock(controllers.NewPackageController).To(func(watcher chan packages.PackageInfo) *controllers.PackageController {
			return mockController
		}).Build()

		manager, err := New("/tmp/test")
		require.NoError(t, err)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		err = manager.Start(ctx)
		require.NoError(t, err)

		status, err := manager.Status(ctx)
		require.NoError(t, err)
		// Since we're using a fresh controller with no packages, status should be empty (or nil)
		assert.Empty(t, status)
	})
}
