package controllers

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/leptonai/gpud/pkg/gpud-manager/packages"
	"github.com/leptonai/gpud/pkg/packagelock"
)

func TestRecoveredPackageRepairsRuntimeExecutable(t *testing.T) {
	for _, binary := range []string{"containerd-shim-runc-v2", "kubelet"} {
		for _, delayedDependency := range []bool{false, true} {
			name := binary + "/ready-dependency"
			if delayedDependency {
				name = binary + "/delayed-dependency"
			}
			t.Run(name, func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
				defer cancel()
				dir := t.TempDir()
				binaryPath := filepath.Join(dir, binary)
				require.NoError(t, os.WriteFile(binaryPath, []byte("#!/bin/sh\nexit 0\n"), 0755))
				script := filepath.Join(dir, "init.sh")
				require.NoError(t, os.WriteFile(script, []byte(strings.ReplaceAll(`#!/bin/bash
set -eu
cd "$(dirname "$0")"
echo "$1" >> calls
case "$1" in
  isInstalled) test -x {{binary}} ;;
  shouldSkip) exit 1 ;;
  install)
    if [ -f fail-install ]; then exit 1; fi
    chmod +x {{binary}}
    ;;
  start) test -x {{binary}} ;;
  status) exit 1 ;;
  version) echo 1.0.0 ;;
  upgrade) exit 0 ;;
  *) exit 2 ;;
esac
`, "{{binary}}", binary)), 0600))
				controller := NewPackageController(make(chan packages.PackageInfo))
				pkg := &packages.PackageStatus{
					Name: t.Name(), ScriptPath: script, TargetVersion: "2.0.0",
					Dependency: [][]string{{"dependency", "*"}}, TotalTime: time.Second,
				}
				controller.packageStatus[pkg.Name] = pkg
				if !delayedDependency {
					controller.packageStatus["dependency"] = &packages.PackageStatus{IsInstalled: true}
				}
				// Wait for asynchronous installs to release the operation lock before
				// inspecting files or advancing to the next reconciliation.
				step := func() {
					controller.installPackage(ctx, pkg)
					mu := packagelock.For(pkg.Name)
					mu.Lock()
					defer mu.Unlock()
					controller.RLock()
					installing := pkg.Installing
					controller.RUnlock()
					require.False(t, installing)
				}
				calls := func() []string {
					data, err := os.ReadFile(filepath.Join(dir, "calls"))
					require.NoError(t, err)
					return strings.Fields(string(data))
				}
				step()
				require.Equal(t, []string{"isInstalled"}, calls())
				if delayedDependency {
					// Recovery still permits upgrades, but must not trigger a fresh
					// install or a service restart while dependencies are unavailable.
					require.False(t, pkg.IsInstalled)
					controller.updatePackage(ctx, pkg)
					controller.statusPackage(ctx, pkg)
					require.Contains(t, calls(), "upgrade")
					require.NotContains(t, calls(), "start")
					require.NoError(t, os.Chmod(binaryPath, 0644))
					step()
					require.NotContains(t, calls(), "install")
					controller.packageStatus["dependency"] = &packages.PackageStatus{IsInstalled: true}
					step()
				}
				require.True(t, pkg.IsInstalled)

				// Damage the executable after startup. A failed reinstall must be
				// retried, and start must only run after installation succeeds.
				require.NoError(t, os.Chmod(binaryPath, 0644))
				failInstall := filepath.Join(dir, "fail-install")
				require.NoError(t, os.WriteFile(failInstall, nil, 0600))
				step()
				require.NotContains(t, calls(), "install", "a single runtime failure must not trigger repair")
				step()
				require.Contains(t, calls(), "install", "cached startup state must not bypass runtime validation")
				require.NotContains(t, calls(), "start")
				require.NoError(t, os.Remove(failInstall))
				step()
				step()
				info, err := os.Stat(binaryPath)
				require.NoError(t, err)
				require.NotZero(t, info.Mode().Perm()&0111)
				require.Contains(t, calls(), "start")
				before := len(calls())
				step()
				require.Equal(t, []string{"shouldSkip", "isInstalled"}, calls()[before:])
			})
		}
	}
}

func TestRecoveredPackageHonorsSkipAfterRecovery(t *testing.T) {
	controller, pkg, step, calls, marker := runtimeValidationFixture(t)
	step()
	require.True(t, pkg.IsInstalled)
	require.False(t, controller.recoveredInstalled[pkg.Name])
	marker("broken", true)
	marker("skip", true)
	before := len(calls())
	step()
	step()
	require.Equal(t, []string{"shouldSkip", "shouldSkip"}, calls()[before:])
	require.True(t, pkg.Skipped)
	require.True(t, pkg.IsInstalled)
}

func TestRuntimeValidationConfirmsFailures(t *testing.T) {
	for _, interruption := range []string{"healthy", "skip", "dependency", "skip-timeout"} {
		t.Run(interruption, func(t *testing.T) {
			controller, pkg, step, calls, marker := runtimeValidationFixture(t)
			step()
			marker("broken", true)
			step()
			require.NotContains(t, calls(), "install")

			switch interruption {
			case "healthy":
				marker("broken", false)
				step()
				marker("broken", true)
			case "skip":
				marker("skip", true)
				step()
				marker("skip", false)
			case "dependency":
				pkg.Dependency = [][]string{{"missing", "*"}}
				step()
				pkg.Dependency = nil
			case "skip-timeout":
				controller.recoveryProbeTimeout = 50 * time.Millisecond
				marker("hang-skip", true)
				step()
				marker("hang-skip", false)
				controller.recoveryProbeTimeout = 10 * time.Second
			}
			step()
			require.NotContains(t, calls(), "install", "an interrupted failure sequence must restart confirmation")
			step()
			require.Equal(t, 1, countCommand(calls(), "install"))
			require.Equal(t, 1, countCommand(calls(), "start"))
			step()
			require.False(t, controller.runtimeValidationFailed[pkg.Name])
		})
	}
}

func TestRuntimeValidationTimeoutAndCancellation(t *testing.T) {
	controller, pkg, step, calls, marker := runtimeValidationFixture(t)
	step()
	controller.recoveryProbeTimeout = 50 * time.Millisecond
	marker("hang-validation", true)
	step()
	require.NotContains(t, calls(), "install", "a single timeout must not trigger repair")
	controller.recoveryProbeTimeout = 10 * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	controller.installPackage(ctx, pkg)
	require.ErrorIs(t, ctx.Err(), context.DeadlineExceeded)
	require.NotContains(t, calls(), "install", "parent cancellation must not trigger repair")
	marker("hang-validation", false)
	step()
	require.False(t, controller.runtimeValidationFailed[pkg.Name])
}

func runtimeValidationFixture(t *testing.T) (*PackageController, *packages.PackageStatus, func(), func() []string, func(string, bool)) {
	t.Helper()
	dir := t.TempDir()
	script := filepath.Join(dir, "init.sh")
	require.NoError(t, os.WriteFile(script, []byte(`#!/bin/bash
set -eu
cd "$(dirname "$0")"
echo "$1" >> calls
case "$1" in
  isInstalled)
    if [ -f hang-validation ]; then exec sleep 5; fi
    test ! -f broken
    ;;
  shouldSkip)
    if [ -f hang-skip ]; then exec sleep 5; fi
    test -f skip
    ;;
  install) rm -f broken ;;
  start) test ! -f broken ;;
  *) exit 2 ;;
esac
`), 0600))
	controller := NewPackageController(make(chan packages.PackageInfo))
	pkg := &packages.PackageStatus{Name: t.Name(), ScriptPath: script, TotalTime: time.Second}
	controller.packageStatus[pkg.Name] = pkg
	step := func() {
		t.Helper()
		controller.installPackage(context.Background(), pkg)
		mu := packagelock.For(pkg.Name)
		mu.Lock()
		defer mu.Unlock()
		controller.RLock()
		defer controller.RUnlock()
		require.False(t, pkg.Installing)
	}
	calls := func() []string {
		t.Helper()
		data, err := os.ReadFile(filepath.Join(dir, "calls"))
		require.NoError(t, err)
		return strings.Fields(string(data))
	}
	marker := func(name string, enabled bool) {
		t.Helper()
		path := filepath.Join(dir, name)
		if enabled {
			require.NoError(t, os.WriteFile(path, nil, 0600))
		} else {
			require.NoError(t, os.Remove(path))
		}
	}
	return controller, pkg, step, calls, marker
}

func countCommand(calls []string, command string) int {
	count := 0
	for _, call := range calls {
		if call == command {
			count++
		}
	}
	return count
}
