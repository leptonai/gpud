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
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type operationLockRunner func(context.Context, string, ...string) ([]byte, error)

func (r operationLockRunner) Run(ctx context.Context, command string, args ...string) ([]byte, error) {
	return r(ctx, command, args...)
}

func assertStateDirectoryLocked(t *testing.T, stateDir string) {
	t.Helper()
	mu := lockForStateDirectory(stateDir)
	if mu.TryLock() {
		mu.Unlock()
		t.Error("KAP operation does not hold the KAP state-directory lock")
	}
}

func TestCredentialReloadHoldsStateDirectoryLockThroughNotificationAndFailure(t *testing.T) {
	for _, failSignal := range []bool{false, true} {
		t.Run(fmt.Sprint(failSignal), func(t *testing.T) {
			manager, runner, paths := newTestManager(t)
			initial := newTestCredentials(t, "worker-1", "machine-1", 1)
			require.NoError(t, manager.UpdateCredentials(context.Background(), "machine-1", initial))
			runner.startAgent()
			signals := 0
			manager.runner = operationLockRunner(func(ctx context.Context, command string, args ...string) ([]byte, error) {
				assertStateDirectoryLocked(t, paths.StateDir)
				if args[0] == "kill" {
					signals++
					if failSignal {
						return nil, errors.New("signal failed")
					}
				}
				return runner.Run(ctx, command, args...)
			})
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
			mu := lockForStateDirectory(manager.paths.StateDir)
			require.True(t, mu.TryLock(), "operation lock leaked")
			mu.Unlock()
		})
	}
}

func TestInactiveStagingReleasesStateDirectoryLockWithoutReadiness(t *testing.T) {
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
	mu := lockForStateDirectory(manager.paths.StateDir)
	require.True(t, mu.TryLock(), "KAP operation must release its state-directory lock")
	runner.startAgent()
	mu.Unlock()
}

func TestManagerOperationsShareStateDirectoryLockAndHonorCancellation(t *testing.T) {
	manager, runner, paths := newTestManager(t)
	otherPaths := paths
	otherPaths.StateDir = filepath.Join(paths.StateDir, "unused") + "/.."
	other := NewManager(otherPaths)
	require.Same(t, lockForStateDirectory(paths.StateDir), manager.updateMu)
	require.Same(t, manager.updateMu, other.updateMu, "cleaned paths share the existing release lock")
	require.NotSame(t, manager.updateMu, NewManager(DefaultPaths(t.TempDir())).updateMu)
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

func TestHotReloadBlocksOtherManagerOnSameStateDirectory(t *testing.T) {
	manager, agent, paths := newContractManager(t)
	other := reopenContractManager(manager, paths)
	initial := newTestCredentials(t, "worker-1", "machine-1", 1)
	require.NoError(t, manager.UpdateCredentials(context.Background(), "machine-1", initial))
	agent.active.Store(true)
	signaling := make(chan struct{})
	resume := make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(resume) }) }
	defer unblock()
	agent.beforeReload = func(ctx context.Context) error {
		assertStateDirectoryLocked(t, paths.StateDir)
		close(signaling)
		select {
		case <-resume:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	next := renewedCredentials(t, initial, 2)
	updated := make(chan error, 1)
	go func() {
		updated <- manager.UpdateCredentials(context.Background(), "machine-1", next)
	}()
	select {
	case <-signaling:
	case <-time.After(time.Second):
		t.Fatal("hot reload did not reach notification")
	}
	probed := make(chan error, 1)
	go func() {
		_, err := other.Status(context.Background(), "machine-1")
		probed <- err
	}()
	select {
	case <-probed:
		t.Fatal("another manager inspected selected state before notification completed")
	case <-time.After(20 * time.Millisecond):
	}
	unblock()
	select {
	case err := <-updated:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("hot reload did not finish")
	}
	select {
	case err := <-probed:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("hot reload leaked state-directory lock")
	}
}
