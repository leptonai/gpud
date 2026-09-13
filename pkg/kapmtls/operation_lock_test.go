// SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package kapmtls

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

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

func TestCredentialRotationHoldsPackageLockThroughReadinessAndRollback(t *testing.T) {
	for _, failRestart := range []bool{false, true} {
		name := "ready"
		if failRestart {
			name = "rollback"
		}
		t.Run(name, func(t *testing.T) {
			manager, runner, paths := newTestManager(t)
			require.NoError(t, manager.UpdateCredentials(context.Background(), "machine-1", newTestCredentials(t, "worker-1", "machine-1", 1)))
			previous, err := manager.currentReleaseID()
			require.NoError(t, err)
			restarts := 0
			manager.runner = operationLockRunner(func(ctx context.Context, command string, args ...string) ([]byte, error) {
				assertPackageLocked(t)
				if args[0] == "restart" {
					restarts++
					if failRestart && restarts == 1 {
						return nil, errors.New("restart failed")
					}
					if failRestart {
						current, readErr := manager.currentReleaseID()
						assert.NoError(t, readErr)
						assert.Equal(t, previous, current)
						assert.NoError(t, ctx.Err(), "rollback has its own uncanceled context")
					}
				}
				return runner.Run(ctx, command, args...)
			})
			ready := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assertPackageLocked(t)
				w.WriteHeader(http.StatusOK)
			}))
			defer ready.Close()
			manager.readyURL = ready.URL
			err = manager.UpdateCredentials(context.Background(), "machine-1", newTestCredentials(t, "worker-1", "machine-1", 2))
			if failRestart {
				require.ErrorContains(t, err, "restart failed")
				assert.Equal(t, 2, restarts)
			} else {
				require.NoError(t, err)
				assert.Equal(t, 1, restarts)
				entries, readErr := os.ReadDir(filepath.Join(paths.StateDir, ReleasesDirectoryName))
				require.NoError(t, readErr)
				assert.Len(t, entries, 1)
			}
			mu := packagelock.For(PackageName)
			require.True(t, mu.TryLock(), "operation lock leaked")
			mu.Unlock()
		})
	}
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
