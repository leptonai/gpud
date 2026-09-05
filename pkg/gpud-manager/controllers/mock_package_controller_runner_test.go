package controllers

import (
	"context"
	"errors"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bytedance/mockey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/leptonai/gpud/pkg/gpud-manager/packages"
)

// waitMockQuiescent blocks until the mockey-patched function has not been
// called for a full quiet window, proving the runner goroutine has left the
// patched code before PatchConvey unpatches it. Without this, the goroutine
// can still be executing the mock trampoline during mockey teardown, which
// the race detector flags as a data race on mockey internals.
func waitMockQuiescent(t *testing.T, calls *atomic.Int64) {
	t.Helper()
	require.Eventually(t, func() bool {
		before := calls.Load()
		time.Sleep(100 * time.Millisecond)
		return calls.Load() == before
	}, 10*time.Second, 50*time.Millisecond)
}

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

		var mockCalls atomic.Int64
		mockey.Mock(runCommand).To(func(ctx context.Context, script, arg string, result *string) error {
			mockCalls.Add(1)
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
		defer cancel()
		go controller.updateRunner(ctx)

		select {
		case <-upgradeCalled:
		case <-time.After(5 * time.Second):
			t.Fatal("timed out waiting for upgrade to run")
		}

		// Assert all expected state BEFORE canceling. updateRunner iterates
		// packageStatus in random map order, so the "skip" package may be
		// processed on a later tick than "upgrade". Canceling first races
		// that tick and flakes the test ("Condition never satisfied").
		require.Eventually(t, func() bool {
			controller.RLock()
			defer controller.RUnlock()
			return controller.packageStatus["skip"].Skipped
		}, 10*time.Second, 10*time.Millisecond)

		require.Eventually(t, func() bool {
			controller.RLock()
			defer controller.RUnlock()
			return controller.packageStatus["upgrade"].Progress == 100
		}, 10*time.Second, 10*time.Millisecond)

		cancel()
		// Ensure the runner goroutine has left the patched runCommand before
		// PatchConvey unpatches it on scope exit (avoids a teardown data race).
		waitMockQuiescent(t, &mockCalls)

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
			ScriptPath:     "/tmp/dep.sh",
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
		controller.packageStatus["uninstalled-dep"] = &packages.PackageStatus{
			Name:       "uninstalled-dep",
			Dependency: [][]string{{"missing", "*"}},
			ScriptPath: "/tmp/uninstalled-dep.sh",
			TotalTime:  200 * time.Millisecond,
		}
		controller.packageStatus["needs-uninstalled-dep"] = &packages.PackageStatus{
			Name:       "needs-uninstalled-dep",
			Dependency: [][]string{{"uninstalled-dep", "*"}},
			ScriptPath: "/tmp/needs-uninstalled-dep.sh",
			TotalTime:  200 * time.Millisecond,
		}
		controller.packageStatus["skip"] = &packages.PackageStatus{
			Name:       "skip",
			ScriptPath: "/tmp/skip-install.sh",
			TotalTime:  200 * time.Millisecond,
		}
		controller.packageStatus["installed-with-missing-dep"] = &packages.PackageStatus{
			Name:       "installed-with-missing-dep",
			Dependency: [][]string{{"missing", "*"}},
			ScriptPath: "/tmp/installed-with-missing-dep.sh",
			TotalTime:  200 * time.Millisecond,
		}
		controller.packageStatus["install"] = &packages.PackageStatus{
			Name:       "install",
			Dependency: [][]string{{"dep", "2.0.0"}},
			ScriptPath: "/tmp/install.sh",
			TotalTime:  200 * time.Millisecond,
		}
		controller.packageStatus["installing"] = &packages.PackageStatus{
			Name:       "installing",
			Installing: true,
			ScriptPath: "/tmp/installing.sh",
			TotalTime:  200 * time.Millisecond,
		}

		installCalled := make(chan struct{}, 1)
		startCalled := make(chan struct{}, 1)

		var mockCalls atomic.Int64
		var unexpectedCalls atomic.Int64
		var installedProbeCalls atomic.Int64
		var installedInstallCalls atomic.Int64
		var installedStartCalls atomic.Int64
		var missingProbeCalls atomic.Int64
		var missingInstallCalls atomic.Int64
		var missingStartCalls atomic.Int64
		var oldVersionProbeCalls atomic.Int64
		var oldVersionInstallCalls atomic.Int64
		var oldVersionStartCalls atomic.Int64
		var uninstalledDepProbeCalls atomic.Int64
		var uninstalledDepInstallCalls atomic.Int64
		var uninstalledDepStartCalls atomic.Int64
		var dependentProbeCalls atomic.Int64
		var dependentInstallCalls atomic.Int64
		var dependentStartCalls atomic.Int64
		var installingShouldSkipCalls atomic.Int64
		var installingProbeCalls atomic.Int64
		var installingInstallCalls atomic.Int64
		var installingStartCalls atomic.Int64
		mockey.Mock(runCommand).To(func(ctx context.Context, script, arg string, result *string) error {
			mockCalls.Add(1)
			switch filepath.Base(script) {
			case "dep.sh":
				switch arg {
				case "shouldSkip":
					return errors.New("no skip")
				case "isInstalled":
					return nil
				}
			case "missing.sh":
				switch arg {
				case "shouldSkip":
					return errors.New("no skip")
				case "isInstalled":
					missingProbeCalls.Add(1)
					return errors.New("not installed")
				case "install":
					missingInstallCalls.Add(1)
					return nil
				case "start":
					missingStartCalls.Add(1)
					return nil
				}
			case "version.sh":
				switch arg {
				case "shouldSkip":
					return errors.New("no skip")
				case "isInstalled":
					oldVersionProbeCalls.Add(1)
					return errors.New("not installed")
				case "install":
					oldVersionInstallCalls.Add(1)
					return nil
				case "start":
					oldVersionStartCalls.Add(1)
					return nil
				}
			case "uninstalled-dep.sh":
				switch arg {
				case "shouldSkip":
					return errors.New("no skip")
				case "isInstalled":
					uninstalledDepProbeCalls.Add(1)
					return errors.New("not installed")
				case "install":
					uninstalledDepInstallCalls.Add(1)
					return nil
				case "start":
					uninstalledDepStartCalls.Add(1)
					return nil
				}
			case "needs-uninstalled-dep.sh":
				switch arg {
				case "shouldSkip":
					return errors.New("no skip")
				case "isInstalled":
					dependentProbeCalls.Add(1)
					return errors.New("not installed")
				case "install":
					dependentInstallCalls.Add(1)
					return nil
				case "start":
					dependentStartCalls.Add(1)
					return nil
				}
			case "skip-install.sh":
				if arg == "shouldSkip" {
					return nil
				}
			case "installed-with-missing-dep.sh":
				if arg == "shouldSkip" {
					return errors.New("no skip")
				}
				if arg == "isInstalled" {
					installedProbeCalls.Add(1)
					return nil
				}
				if arg == "install" {
					installedInstallCalls.Add(1)
					return nil
				}
				if arg == "start" {
					installedStartCalls.Add(1)
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
			case "installing.sh":
				switch arg {
				case "shouldSkip":
					installingShouldSkipCalls.Add(1)
					return errors.New("no skip")
				case "isInstalled":
					installingProbeCalls.Add(1)
					return nil
				case "install":
					installingInstallCalls.Add(1)
					return nil
				case "start":
					installingStartCalls.Add(1)
					return nil
				}
			}
			unexpectedCalls.Add(1)
			return errors.New("unexpected command")
		}).Build()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go controller.installRunner(ctx)

		select {
		case <-installCalled:
		case <-time.After(5 * time.Second):
			t.Fatal("timed out waiting for install to run")
		}

		select {
		case <-startCalled:
		case <-time.After(5 * time.Second):
			t.Fatal("timed out waiting for start to run")
		}

		// Assert state before canceling: installRunner iterates packageStatus in
		// random map order, so dependent packages may be processed on later ticks.
		require.Eventually(t, func() bool {
			controller.RLock()
			defer controller.RUnlock()
			return controller.packageStatus["skip"].Skipped && controller.packageStatus["skip"].IsInstalled
		}, 10*time.Second, 10*time.Millisecond)

		require.Eventually(t, func() bool {
			controller.RLock()
			defer controller.RUnlock()
			installed := controller.packageStatus["installed-with-missing-dep"]
			return installed.IsInstalled && installed.Progress == 100
		}, 10*time.Second, 10*time.Millisecond)

		require.Eventually(t, func() bool {
			controller.RLock()
			defer controller.RUnlock()
			return controller.packageStatus["install"].Progress == 100
		}, 10*time.Second, 10*time.Millisecond)

		require.Eventually(t, func() bool {
			return missingProbeCalls.Load() > 0 &&
				oldVersionProbeCalls.Load() > 0 &&
				uninstalledDepProbeCalls.Load() > 0 &&
				dependentProbeCalls.Load() > 0 &&
				installingShouldSkipCalls.Load() > 0
		}, 10*time.Second, 10*time.Millisecond)

		cancel()
		// Ensure the runner goroutine has left the patched runCommand before
		// PatchConvey unpatches it on scope exit (avoids a teardown data race).
		waitMockQuiescent(t, &mockCalls)

		controller.RLock()
		missingInstalled := controller.packageStatus["needs-missing-dep"].IsInstalled
		versionInstalled := controller.packageStatus["needs-version-dep"].IsInstalled
		uninstalledDependencyInstalled := controller.packageStatus["uninstalled-dep"].IsInstalled
		dependentInstalled := controller.packageStatus["needs-uninstalled-dep"].IsInstalled
		controller.RUnlock()

		assert.False(t, missingInstalled)
		assert.False(t, versionInstalled)
		assert.False(t, uninstalledDependencyInstalled)
		assert.False(t, dependentInstalled)
		assert.Positive(t, installedProbeCalls.Load())
		assert.Zero(t, installedInstallCalls.Load())
		assert.Zero(t, installedStartCalls.Load())
		assert.Zero(t, missingInstallCalls.Load())
		assert.Zero(t, missingStartCalls.Load())
		assert.Positive(t, missingProbeCalls.Load())
		assert.Zero(t, oldVersionInstallCalls.Load())
		assert.Zero(t, oldVersionStartCalls.Load())
		assert.Positive(t, oldVersionProbeCalls.Load())
		assert.Zero(t, uninstalledDepInstallCalls.Load())
		assert.Zero(t, uninstalledDepStartCalls.Load())
		assert.Positive(t, uninstalledDepProbeCalls.Load())
		assert.Zero(t, dependentInstallCalls.Load())
		assert.Zero(t, dependentStartCalls.Load())
		assert.Positive(t, dependentProbeCalls.Load())
		assert.Positive(t, installingShouldSkipCalls.Load())
		assert.Zero(t, installingProbeCalls.Load())
		assert.Zero(t, installingInstallCalls.Load())
		assert.Zero(t, installingStartCalls.Load())
		assert.Zero(t, unexpectedCalls.Load())
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

		var mockCalls atomic.Int64
		mockey.Mock(runCommand).To(func(ctx context.Context, script, arg string, result *string) error {
			mockCalls.Add(1)
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
		defer cancel()
		go controller.statusRunner(ctx)
		go controller.deleteRunner(ctx)

		select {
		case <-startCalled:
		case <-time.After(5 * time.Second):
			t.Fatal("timed out waiting for status restart")
		}

		select {
		case <-deleteCalled:
		case <-time.After(5 * time.Second):
			t.Fatal("timed out waiting for delete")
		}

		// Assert state before canceling: both runners iterate packageStatus in
		// random map order, so dependent packages may be processed on later ticks.
		require.Eventually(t, func() bool {
			controller.RLock()
			defer controller.RUnlock()
			return controller.packageStatus["skip"].Skipped && controller.packageStatus["skip"].Status
		}, 10*time.Second, 10*time.Millisecond)

		require.Eventually(t, func() bool {
			controller.RLock()
			defer controller.RUnlock()
			return controller.packageStatus["ok"].Status
		}, 10*time.Second, 10*time.Millisecond)

		cancel()
		// Ensure the runner goroutines have left the patched runCommand before
		// PatchConvey unpatches it on scope exit (avoids a teardown data race).
		waitMockQuiescent(t, &mockCalls)
	})
}
