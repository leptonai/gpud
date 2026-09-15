// SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package kapmtls

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/leptonai/gpud/pkg/packagelock"
)

type operationLockRunner func(context.Context, string, ...string) ([]byte, error)

func (r operationLockRunner) Run(ctx context.Context, command string, args ...string) ([]byte, error) {
	return r(ctx, command, args...)
}

func assertPackageLocked(t *testing.T) {
	t.Helper()
	mu := packagelock.For(PackageName)
	if mu.TryLock() {
		mu.Unlock()
		t.Error("KAP operation does not hold the managed package lock")
	}
}

func TestCredentialReloadHoldsPackageLockThroughVerificationAndFailure(t *testing.T) {
	for _, failSignal := range []bool{false, true} {
		t.Run(fmt.Sprint(failSignal), func(t *testing.T) {
			manager, runner, paths := newTestManager(t)
			initial := newTestCredentials(t, "worker-1", "machine-1", 1)
			require.NoError(t, manager.UpdateCredentials(context.Background(), "machine-1", initial))
			runner.startAgent()
			signals := 0
			manager.runner = operationLockRunner(func(ctx context.Context, command string, args ...string) ([]byte, error) {
				assertPackageLocked(t)
				if args[0] == "kill" {
					signals++
					if failSignal {
						return nil, errors.New("signal failed")
					}
				}
				return runner.Run(ctx, command, args...)
			})
			ready := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assertPackageLocked(t)
				if r.URL.Path == "/metrics" {
					loaded := runner.loaded.Load()
					_, _ = w.Write([]byte(metricText(loaded.serial, loaded.notAfter)))
				}
			}))
			defer ready.Close()
			manager.readyURL = ready.URL
			err := manager.UpdateCredentials(context.Background(), "machine-1", renewedCredentials(t, initial, 2))
			if failSignal {
				require.ErrorContains(t, err, "signal failed")
			} else {
				require.NoError(t, err)
				entries, readErr := os.ReadDir(filepath.Join(paths.StateDir, ReleasesDirectoryName))
				require.NoError(t, readErr)
				assert.Len(t, entries, 1)
			}
			assert.Equal(t, 1, signals)
			mu := packagelock.For(PackageName)
			require.True(t, mu.TryLock(), "operation lock leaked")
			mu.Unlock()
		})
	}
}

func TestInactiveStagingReleasesPackageLockWithoutReadiness(t *testing.T) {
	manager, runner, _ := newTestManager(t)
	ready := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("inactive staging must not probe readiness or metrics")
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer ready.Close()
	manager.readyURL = ready.URL
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	require.NoError(t, manager.UpdateCredentials(ctx, "machine-1", newTestCredentials(t, "worker-1", "machine-1", 1)))
	require.NoError(t, ctx.Err())
	assert.False(t, runner.active)
	mu := packagelock.For(PackageName)
	require.True(t, mu.TryLock(), "package controller must be free to start the agent")
	runner.startAgent()
	mu.Unlock()
}

func TestManagerOperationsSharePackageLockAndHonorCancellation(t *testing.T) {
	manager, runner, paths := newTestManager(t)
	other := NewManager(DefaultPaths(t.TempDir()))
	require.Same(t, packagelock.For("kap-mtls-agent"), manager.updateMu)
	require.Same(t, manager.updateMu, other.updateMu, "systemd service operations share a lock even across state directories")
	credentials := newTestCredentials(t, "worker-1", "machine-1", 1)
	operations := []func(context.Context) error{
		func(ctx context.Context) error { return manager.UpdateCredentials(ctx, "machine-1", credentials) },
		manager.Activate,
		func(ctx context.Context) error {
			_, err := manager.Status(ctx, "machine-1")
			return err
		},
	}
	for _, operation := range operations {
		manager.updateMu.Lock()
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		go func() { done <- operation(ctx) }()
		cancel()
		manager.updateMu.Unlock()
		require.ErrorIs(t, <-done, context.Canceled)
	}
	assert.Empty(t, runner.calls)
	_, err := os.Lstat(filepath.Join(paths.StateDir, CurrentSymlinkName))
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestHotReloadBlocksConcurrentPackageOperation(t *testing.T) {
	manager, runner, _ := newTestManager(t)
	initial := newTestCredentials(t, "worker-1", "machine-1", 1)
	require.NoError(t, manager.UpdateCredentials(context.Background(), "machine-1", initial))
	runner.startAgent()
	manager.reloadTimeout = time.Second
	signaling := make(chan struct{})
	resume := make(chan struct{})
	runner.afterSignal = func(context.Context) {
		assertPackageLocked(t)
		close(signaling)
		<-resume
	}
	next := renewedCredentials(t, initial, 2)
	updated := make(chan error, 1)
	go func() {
		updated <- manager.UpdateCredentials(context.Background(), "machine-1", next)
	}()
	<-signaling
	packageOperation := make(chan struct{})
	go func() {
		mu := packagelock.For(PackageName)
		mu.Lock()
		defer mu.Unlock()
		close(packageOperation)
	}()
	select {
	case <-packageOperation:
		t.Error("package operation entered during hot reload")
	case <-time.After(20 * time.Millisecond):
	}
	close(resume)
	require.NoError(t, <-updated)
	select {
	case <-packageOperation:
	case <-time.After(time.Second):
		t.Fatal("hot reload leaked package lock")
	}
}
