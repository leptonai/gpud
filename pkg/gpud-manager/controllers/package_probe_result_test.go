package controllers

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/leptonai/gpud/pkg/gpud-manager/packages"
	"github.com/leptonai/gpud/pkg/packagelock"
)

func TestProbeExitCode(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "probe.sh")
	require.NoError(t, os.WriteFile(script, []byte(`#!/bin/bash
case "$1" in
  zero) exit 0 ;;
  one) exit 1 ;;
  three) exit 3 ;;
  held) sleep 2 & exit 0 ;;
  killed) kill -KILL $$ ;;
  hang) exec sleep 5 ;;
esac
`), 0600))

	for _, tc := range []struct {
		arg           string
		timeout       time.Duration
		code          int
		exited        bool
		skip          bool
		skipAnswered  bool
		waitDelayOnly bool
	}{
		{arg: "zero", exited: true, skip: true, skipAnswered: true},
		{arg: "one", code: 1, exited: true, skipAnswered: true},
		{arg: "three", code: 3, exited: true},
		// The script exits 0 but a child keeps stdout open past WaitDelay.
		{arg: "held", exited: true, skip: true, skipAnswered: true, waitDelayOnly: true},
		{arg: "killed"},
		{arg: "hang", timeout: 200 * time.Millisecond},
	} {
		for _, capture := range []bool{true, false} {
			name := tc.arg + "/no-capture"
			if capture {
				name = tc.arg + "/capture"
			}
			t.Run(name, func(t *testing.T) {
				timeout := tc.timeout
				if timeout == 0 {
					timeout = 10 * time.Second
				}
				ctx, cancel := context.WithTimeout(context.Background(), timeout)
				defer cancel()
				var result *string
				if capture {
					result = new(string)
				}
				err := runCommand(ctx, script, tc.arg, result)
				if tc.waitDelayOnly && capture {
					require.ErrorIs(t, err, exec.ErrWaitDelay)
				}

				code, exited := probeExitCode(err)
				require.Equal(t, tc.exited, exited, "error: %v", err)
				require.Equal(t, tc.code, code)
				skip, answered := skipProbeResult(err)
				require.Equal(t, tc.skip, skip)
				require.Equal(t, tc.skipAnswered, answered)
			})
		}
	}

	code, exited := probeExitCode(errors.New("fork/exec /bin/bash: resource temporarily unavailable"))
	require.False(t, exited)
	require.Zero(t, code)
}

func TestInstallPackageActsOnlyOnProbeAnswers(t *testing.T) {
	for _, tc := range []struct {
		name      string
		skip      string
		installed string
		wasSkip   bool
		calls     []string
		skipped   bool
	}{
		{name: "skip-with-held-output", skip: "held", installed: "1", calls: []string{"shouldSkip"}, skipped: true},
		{name: "skip-probe-killed", skip: "killed", installed: "1", calls: []string{"shouldSkip"}},
		{name: "skip-probe-unknown-status", skip: "3", installed: "1", calls: []string{"shouldSkip"}},
		{name: "skipped-package-probe-killed", skip: "killed", installed: "1", wasSkip: true, calls: []string{"shouldSkip"}},
		{name: "install-probe-killed", skip: "1", installed: "killed", calls: []string{"shouldSkip", "isInstalled"}},
		{name: "not-installed", skip: "1", installed: "1", calls: []string{"shouldSkip", "isInstalled", "install", "start"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newProbeFixture(t, tc.skip, tc.installed)
			if tc.wasSkip {
				// A package that an earlier cycle skipped keeps IsInstalled set.
				f.pkg.IsInstalled = true
				f.pkg.Skipped = true
			}
			f.install()
			if tc.wasSkip {
				f.install()
				require.Equal(t, append(tc.calls, tc.calls...), f.calls())
			} else {
				require.Equal(t, tc.calls, f.calls())
			}
			require.Equal(t, tc.skipped || tc.wasSkip, f.pkg.Skipped)
		})
	}
}

func TestUpdatePackageActsOnlyOnSkipAnswers(t *testing.T) {
	for _, tc := range []struct {
		skip  string
		calls []string
	}{
		{skip: "killed", calls: []string{"version", "shouldSkip"}},
		{skip: "3", calls: []string{"version", "shouldSkip"}},
		{skip: "0", calls: []string{"version", "shouldSkip"}},
		{skip: "1", calls: []string{"version", "shouldSkip", "upgrade"}},
	} {
		t.Run(tc.skip, func(t *testing.T) {
			f := newProbeFixture(t, tc.skip, "0")
			f.pkg.IsInstalled = true
			f.controller.updatePackage(context.Background(), f.pkg)
			require.Equal(t, tc.calls, f.calls())
		})
	}
}

func TestStatusPackageActsOnlyOnSkipAnswers(t *testing.T) {
	for _, tc := range []struct {
		skip  string
		calls []string
	}{
		{skip: "killed", calls: []string{"shouldSkip"}},
		{skip: "3", calls: []string{"shouldSkip"}},
		{skip: "0", calls: []string{"shouldSkip"}},
		{skip: "1", calls: []string{"shouldSkip", "status"}},
	} {
		t.Run(tc.skip, func(t *testing.T) {
			f := newProbeFixture(t, tc.skip, "0")
			f.pkg.IsInstalled = true
			f.controller.statusPackage(context.Background(), f.pkg)
			require.Equal(t, tc.calls, f.calls())
		})
	}
}

type probeFixture struct {
	t          *testing.T
	dir        string
	controller *PackageController
	pkg        *packages.PackageStatus
}

// newProbeFixture builds a package whose shouldSkip and isInstalled answer with
// the given behavior: an exit status, "held" (exit 0 while a child keeps the
// output pipes open) or "killed" (terminated by SIGKILL).
func newProbeFixture(t *testing.T, skip, installed string) *probeFixture {
	t.Helper()
	dir := t.TempDir()
	script := filepath.Join(dir, "init.sh")
	require.NoError(t, os.WriteFile(script, []byte(`#!/bin/bash
cd "$(dirname "$0")"
echo "$1" >> calls
answer() {
  case "$(cat "$1")" in
    held) sleep 2 & exit 0 ;;
    killed) kill -KILL $$ ;;
    *) exit "$(cat "$1")" ;;
  esac
}
case "$1" in
  shouldSkip) answer skip ;;
  isInstalled) answer installed ;;
  version) echo 1.0.0 ;;
  *) exit 0 ;;
esac
`), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "skip"), []byte(skip), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "installed"), []byte(installed), 0600))

	controller := NewPackageController(make(chan packages.PackageInfo))
	pkg := &packages.PackageStatus{Name: t.Name(), ScriptPath: script, TargetVersion: "2.0.0", TotalTime: time.Second}
	controller.packageStatus[pkg.Name] = pkg
	// Start from steady state: the restart recovery probe is covered elsewhere.
	controller.recoveryProbed[pkg.Name] = true
	return &probeFixture{t: t, dir: dir, controller: controller, pkg: pkg}
}

// install runs one install cycle and waits for an asynchronous install to finish.
func (f *probeFixture) install() {
	f.t.Helper()
	f.controller.installPackage(context.Background(), f.pkg)
	mu := packagelock.For(f.pkg.Name)
	mu.Lock()
	defer mu.Unlock()
	f.controller.RLock()
	defer f.controller.RUnlock()
	require.False(f.t, f.pkg.Installing)
}

func (f *probeFixture) calls() []string {
	f.t.Helper()
	data, err := os.ReadFile(filepath.Join(f.dir, "calls"))
	require.NoError(f.t, err)
	return strings.Fields(string(data))
}

// exitStatus returns the error that runCommand reports for a script that
// exited with code, so mocks can give a probe a real "no" answer.
func exitStatus(code int) error {
	return exec.Command("/bin/sh", "-c", "exit "+strconv.Itoa(code)).Run()
}
