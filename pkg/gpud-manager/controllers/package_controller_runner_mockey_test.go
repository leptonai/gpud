package controllers

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/bytedance/mockey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/leptonai/gpud/pkg/gpud-manager/packages"
)

func TestUpdateRunner_WithMockedRunCommand(t *testing.T) {
	mockey.PatchConvey("update runner handles skip/same/upgrade/version errors", t, func() {
		controller := NewPackageController(make(chan packages.PackageInfo))
		controller.syncPeriod = 10 * time.Millisecond

		controller.packageStatus["badversion"] = &packages.PackageStatus{
			Name:          "badversion",
			IsInstalled:   true,
			TargetVersion: "2.0.0",
			ScriptPath:    "/tmp/badversion.sh",
			TotalTime:     200 * time.Millisecond,
		}
		controller.packageStatus["skip"] = &packages.PackageStatus{
			Name:          "skip",
			IsInstalled:   true,
			TargetVersion: "2.0.0",
			ScriptPath:    "/tmp/skip.sh",
			TotalTime:     200 * time.Millisecond,
		}
		controller.packageStatus["same"] = &packages.PackageStatus{
			Name:          "same",
			IsInstalled:   true,
			TargetVersion: "1.0.0",
			ScriptPath:    "/tmp/same.sh",
			TotalTime:     200 * time.Millisecond,
		}
		controller.packageStatus["upgrade"] = &packages.PackageStatus{
			Name:          "upgrade",
			IsInstalled:   true,
			TargetVersion: "2.0.0",
			ScriptPath:    "/tmp/upgrade.sh",
			TotalTime:     200 * time.Millisecond,
		}

		upgradeCalled := make(chan struct{}, 1)

		mockey.Mock(runCommand).To(func(ctx context.Context, script, arg string, result *string) error {
			switch filepath.Base(script) {
			case "badversion.sh":
				if arg == "version" {
					return errors.New("version failed")
				}
			case "skip.sh":
				if arg == "version" {
					if result != nil {
						*result = "1.0.0"
					}
					return nil
				}
				if arg == "shouldSkip" {
					return nil
				}
			case "same.sh":
				if arg == "version" {
					if result != nil {
						*result = "1.0.0"
					}
					return nil
				}
				if arg == "shouldSkip" {
					return errors.New("no skip")
				}
			case "upgrade.sh":
				if arg == "version" {
					if result != nil {
						*result = "1.0.0"
					}
					return nil
				}
				if arg == "shouldSkip" {
					return errors.New("no skip")
				}
				if arg == "upgrade" {
					select {
					case upgradeCalled <- struct{}{}:
					default:
					}
					return nil
				}
			}
			return nil
		}).Build()

		ctx, cancel := context.WithCancel(context.Background())
		go controller.updateRunner(ctx)

		select {
		case <-upgradeCalled:
		case <-time.After(2 * time.Second):
			cancel()
			t.Fatal("timed out waiting for upgrade to run")
		}
		cancel()

		require.Eventually(t, func() bool {
			controller.RLock()
			defer controller.RUnlock()
			return controller.packageStatus["skip"].Skipped
		}, time.Second, 10*time.Millisecond)

		require.Eventually(t, func() bool {
			controller.RLock()
			defer controller.RUnlock()
			return controller.packageStatus["upgrade"].Progress == 100
		}, time.Second, 10*time.Millisecond)

		controller.RLock()
		same := controller.packageStatus["same"]
		controller.RUnlock()
		assert.False(t, same.Installing)
	})
}

func TestInstallRunner_WithMockedRunCommand(t *testing.T) {
	mockey.PatchConvey("install runner handles deps/skip/install paths", t, func() {
		controller := NewPackageController(make(chan packages.PackageInfo))
		controller.syncPeriod = 10 * time.Millisecond

		controller.packageStatus["dep"] = &packages.PackageStatus{
			Name:           "dep",
			IsInstalled:    true,
			CurrentVersion: "2.0.0",
		}

		controller.packageStatus["needs-missing-dep"] = &packages.PackageStatus{
			Name:       "needs-missing-dep",
			Dependency: [][]string{{"missing", "*"}},
			ScriptPath: "/tmp/missing.sh",
			TotalTime:  200 * time.Millisecond,
		}
		controller.packageStatus["needs-version-dep"] = &packages.PackageStatus{
			Name:       "needs-version-dep",
			Dependency: [][]string{{"dep", "3.0.0"}},
			ScriptPath: "/tmp/version.sh",
			TotalTime:  200 * time.Millisecond,
		}
		controller.packageStatus["skip"] = &packages.PackageStatus{
			Name:       "skip",
			ScriptPath: "/tmp/skip-install.sh",
			TotalTime:  200 * time.Millisecond,
		}
		controller.packageStatus["installed"] = &packages.PackageStatus{
			Name:       "installed",
			ScriptPath: "/tmp/installed.sh",
			TotalTime:  200 * time.Millisecond,
		}
		controller.packageStatus["install"] = &packages.PackageStatus{
			Name:       "install",
			ScriptPath: "/tmp/install.sh",
			TotalTime:  200 * time.Millisecond,
		}

		installCalled := make(chan struct{}, 1)
		startCalled := make(chan struct{}, 1)

		mockey.Mock(runCommand).To(func(ctx context.Context, script, arg string, result *string) error {
			switch filepath.Base(script) {
			case "skip-install.sh":
				if arg == "shouldSkip" {
					return nil
				}
				return errors.New("unexpected")
			case "installed.sh":
				if arg == "shouldSkip" {
					return errors.New("no skip")
				}
				if arg == "isInstalled" {
					return nil
				}
			case "install.sh":
				if arg == "shouldSkip" {
					return errors.New("no skip")
				}
				if arg == "isInstalled" {
					return errors.New("not installed")
				}
				if arg == "install" {
					select {
					case installCalled <- struct{}{}:
					default:
					}
					return nil
				}
				if arg == "start" {
					select {
					case startCalled <- struct{}{}:
					default:
					}
					return errors.New("start failed")
				}
			}
			return nil
		}).Build()

		ctx, cancel := context.WithCancel(context.Background())
		go controller.installRunner(ctx)

		select {
		case <-installCalled:
		case <-time.After(2 * time.Second):
			cancel()
			t.Fatal("timed out waiting for install to run")
		}

		select {
		case <-startCalled:
		case <-time.After(2 * time.Second):
			cancel()
			t.Fatal("timed out waiting for start to run")
		}
		cancel()

		require.Eventually(t, func() bool {
			controller.RLock()
			defer controller.RUnlock()
			return controller.packageStatus["skip"].Skipped && controller.packageStatus["skip"].IsInstalled
		}, time.Second, 10*time.Millisecond)

		require.Eventually(t, func() bool {
			controller.RLock()
			defer controller.RUnlock()
			return controller.packageStatus["installed"].IsInstalled
		}, time.Second, 10*time.Millisecond)

		require.Eventually(t, func() bool {
			controller.RLock()
			defer controller.RUnlock()
			return controller.packageStatus["install"].Progress == 100
		}, time.Second, 10*time.Millisecond)

		controller.RLock()
		missing := controller.packageStatus["needs-missing-dep"]
		version := controller.packageStatus["needs-version-dep"]
		controller.RUnlock()

		assert.False(t, missing.IsInstalled)
		assert.False(t, version.IsInstalled)
	})
}

func TestStatusAndDeleteRunner_WithMockedRunCommand(t *testing.T) {
	mockey.PatchConvey("status and delete runners handle skip/status/restart/delete", t, func() {
		controller := NewPackageController(make(chan packages.PackageInfo))
		controller.syncPeriod = 10 * time.Millisecond

		controller.packageStatus["skip"] = &packages.PackageStatus{
			Name:        "skip",
			IsInstalled: true,
			ScriptPath:  "/tmp/skip-status.sh",
		}
		controller.packageStatus["ok"] = &packages.PackageStatus{
			Name:        "ok",
			IsInstalled: true,
			ScriptPath:  "/tmp/ok.sh",
		}
		controller.packageStatus["restart-stop"] = &packages.PackageStatus{
			Name:        "restart-stop",
			IsInstalled: true,
			ScriptPath:  "/tmp/restart-stop.sh",
		}
		controller.packageStatus["restart-start"] = &packages.PackageStatus{
			Name:        "restart-start",
			IsInstalled: true,
			ScriptPath:  "/tmp/restart-start.sh",
		}
		controller.packageStatus["delete-skip"] = &packages.PackageStatus{
			Name:       "delete-skip",
			ScriptPath: "/tmp/delete-skip.sh",
		}
		controller.packageStatus["delete"] = &packages.PackageStatus{
			Name:       "delete",
			ScriptPath: "/tmp/delete.sh",
		}

		stopCalled := make(chan struct{}, 1)
		startCalled := make(chan struct{}, 1)
		deleteCalled := make(chan struct{}, 1)

		mockey.Mock(runCommand).To(func(ctx context.Context, script, arg string, result *string) error {
			switch filepath.Base(script) {
			case "skip-status.sh":
				if arg == "shouldSkip" {
					return nil
				}
			case "ok.sh":
				if arg == "shouldSkip" {
					return errors.New("no skip")
				}
				if arg == "status" {
					return nil
				}
			case "restart-stop.sh":
				if arg == "shouldSkip" {
					return errors.New("no skip")
				}
				if arg == "status" {
					return errors.New("status failed")
				}
				if arg == "stop" {
					select {
					case stopCalled <- struct{}{}:
					default:
					}
					return errors.New("stop failed")
				}
			case "restart-start.sh":
				if arg == "shouldSkip" {
					return errors.New("no skip")
				}
				if arg == "status" {
					return errors.New("status failed")
				}
				if arg == "stop" {
					return nil
				}
				if arg == "start" {
					select {
					case startCalled <- struct{}{}:
					default:
					}
					return errors.New("start failed")
				}
			case "delete-skip.sh":
				if arg == "needDelete" {
					return errors.New("no delete")
				}
			case "delete.sh":
				if arg == "needDelete" {
					return nil
				}
				if arg == "delete" {
					select {
					case deleteCalled <- struct{}{}:
					default:
					}
					return errors.New("delete failed")
				}
			}
			return nil
		}).Build()

		ctx, cancel := context.WithCancel(context.Background())
		go controller.statusRunner(ctx)
		go controller.deleteRunner(ctx)

		select {
		case <-startCalled:
		case <-time.After(2 * time.Second):
			cancel()
			t.Fatal("timed out waiting for status restart")
		}

		select {
		case <-deleteCalled:
		case <-time.After(2 * time.Second):
			cancel()
			t.Fatal("timed out waiting for delete")
		}

		cancel()

		require.Eventually(t, func() bool {
			controller.RLock()
			defer controller.RUnlock()
			return controller.packageStatus["skip"].Skipped && controller.packageStatus["skip"].Status
		}, time.Second, 10*time.Millisecond)

		require.Eventually(t, func() bool {
			controller.RLock()
			defer controller.RUnlock()
			return controller.packageStatus["ok"].Status
		}, time.Second, 10*time.Millisecond)
	})
}
