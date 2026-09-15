// SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package kapmtls

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const hupCommand = "systemctl kill --signal=HUP --kill-who=main " + AgentService

func renewedCredentials(t *testing.T, previous Credentials, serial int64) Credentials {
	t.Helper()
	renewed := newTestCredentials(t, "worker-1", "machine-1", serial)
	renewed.GatewayCAPEM = previous.GatewayCAPEM
	renewed.GatewayCAFingerprint = previous.GatewayCAFingerprint
	renewed.ClientCAFingerprint = previous.ClientCAFingerprint
	renewed.GatewayEndpoint = previous.GatewayEndpoint
	renewed.ServerName = previous.ServerName
	return renewed
}

func TestLeafRenewalSignalsWithoutReadinessWait(t *testing.T) {
	m, runner, paths := newTestManager(t)
	credentials := newTestCredentials(t, "worker-1", "machine-1", 1)
	require.NoError(t, m.UpdateCredentials(context.Background(), "machine-1", credentials))
	runner.startAgent()
	m.readyURL = "http://127.0.0.1:1"
	runner.calls = nil
	for serial := int64(2); serial <= 3; serial++ {
		credentials = renewedCredentials(t, credentials, serial)
		require.NoError(t, m.UpdateCredentials(context.Background(), "machine-1", credentials))
		status, err := m.Status(context.Background(), "machine-1")
		require.NoError(t, err)
		assert.True(t, status.CredentialsInstalled)
		assert.Equal(t, fmt.Sprintf("%x", serial), status.CertificateSerial)
		assert.False(t, status.AgentReady, "signal delivery does not imply readiness")
	}
	assert.Equal(t, 2, strings.Count(strings.Join(runner.calls, "\n"), hupCommand))
	entries, err := os.ReadDir(filepath.Join(paths.StateDir, ReleasesDirectoryName))
	require.NoError(t, err)
	assert.Len(t, entries, 1)

	runner.calls = nil
	require.NoError(t, m.UpdateCredentials(context.Background(), "machine-1", credentials))
	assert.Contains(t, runner.calls, hupCommand)
}

func TestStartupConfigurationDriftRejectedBeforeWriting(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*Credentials)
	}{
		{name: "gateway CA", mutate: func(c *Credentials) {
			other := newTestCredentials(t, "worker-1", "machine-1", 3)
			c.GatewayCAPEM = other.GatewayCAPEM
			c.GatewayCAFingerprint = other.GatewayCAFingerprint
		}},
		{name: "client CA", mutate: func(c *Credentials) { c.ClientCAFingerprint = strings.Repeat("a", sha256HexLength) }},
		{name: "endpoint port", mutate: func(c *Credentials) { c.GatewayEndpoint = "kap.example.test:9443" }},
		{name: "server name", mutate: func(c *Credentials) {
			c.GatewayEndpoint = "other.example.test:8443"
			c.ServerName = "other.example.test"
		}},
	} {
		for _, active := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/active=%t", tc.name, active), func(t *testing.T) {
				m, runner, paths := newTestManager(t)
				initial := newTestCredentials(t, "worker-1", "machine-1", 1)
				require.NoError(t, m.UpdateCredentials(context.Background(), "machine-1", initial))
				if active {
					runner.startAgent()
				}
				before, err := m.currentReleaseID()
				require.NoError(t, err)
				runner.calls = nil
				next := renewedCredentials(t, initial, 2)
				tc.mutate(&next)
				require.ErrorContains(t, m.UpdateCredentials(context.Background(), "machine-1", next), "requires explicit maintenance/restart")
				current, err := m.currentReleaseID()
				require.NoError(t, err)
				assert.Equal(t, before, current)
				entries, err := os.ReadDir(filepath.Join(paths.StateDir, ReleasesDirectoryName))
				require.NoError(t, err)
				assert.Len(t, entries, 1, "rejected drift must not stage a generation")
				assert.Empty(t, runner.calls)
			})
		}
	}
}

func TestExpiredSelectedLeafCanRenewWithSameStartupConfig(t *testing.T) {
	m, runner, _ := newTestManager(t)
	initial := newTestCredentialsWithLeaf(t, "worker-1", "machine-1", 1, func(c *x509.Certificate) {
		c.NotAfter = testNow.Add(time.Hour)
	})
	require.NoError(t, m.UpdateCredentials(context.Background(), "machine-1", initial))
	runner.startAgent()
	m.now = func() time.Time { return testNow.Add(2 * time.Hour) }
	require.NoError(t, m.UpdateCredentials(context.Background(), "machine-1", renewedCredentials(t, initial, 2)))
	assert.Contains(t, runner.calls, hupCommand)
}

func TestReloadFailureRetainsSelectionAndPendingNotification(t *testing.T) {
	for _, canceled := range []bool{false, true} {
		t.Run(fmt.Sprint(canceled), func(t *testing.T) {
			m, runner, paths := newTestManager(t)
			initial := newTestCredentials(t, "worker-1", "machine-1", 1)
			require.NoError(t, m.UpdateCredentials(context.Background(), "machine-1", initial))
			runner.startAgent()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if canceled {
				runner.afterSignal = func(context.Context) { cancel() }
			} else {
				runner.killErr = errors.New("signal unavailable")
			}
			err := m.UpdateCredentials(ctx, "machine-1", renewedCredentials(t, initial, 2))
			require.Error(t, err)
			if canceled {
				require.ErrorIs(t, err, context.Canceled)
			}
			current, err := m.currentReleaseID()
			require.NoError(t, err)
			status, err := m.Status(context.Background(), "machine-1")
			require.NoError(t, err)
			assert.True(t, status.CredentialsInstalled)
			assert.Equal(t, "2", status.CertificateSerial)
			assert.False(t, status.AgentReady)
			entries, err := os.ReadDir(filepath.Join(paths.StateDir, ReleasesDirectoryName))
			require.NoError(t, err)
			assert.Len(t, entries, 2)

			restarted := NewManager(paths)
			restarted.runner, restarted.now = runner, m.now
			runner.afterSignal, runner.killErr, runner.calls = nil, nil, nil
			require.NoError(t, restarted.Activate(context.Background()))
			assert.Equal(t, []string{"systemctl is-active " + AgentService, hupCommand}, runner.calls)
			selected, err := m.currentReleaseID()
			require.NoError(t, err)
			assert.Equal(t, current, selected)
			pending, err := m.reloadPending()
			require.NoError(t, err)
			assert.False(t, pending)
		})
	}
}

func TestPendingNotificationPrecedesSelectionAndSurvivesSyncFailure(t *testing.T) {
	for _, failSync := range []int{1, 2, 3} {
		t.Run(fmt.Sprint(failSync), func(t *testing.T) {
			m, runner, _ := newTestManager(t)
			initial := newTestCredentials(t, "worker-1", "machine-1", 1)
			require.NoError(t, m.UpdateCredentials(context.Background(), "machine-1", initial))
			runner.startAgent()
			require.NoError(t, m.Activate(context.Background()))
			previous, err := m.currentReleaseID()
			require.NoError(t, err)
			syncErr := errors.New("directory sync failed")
			syncs := 0
			m.syncDir = func(path string) error {
				syncs++
				if syncs == 1 {
					info, err := os.Stat(filepath.Join(path, reloadPendingFileName))
					require.NoError(t, err)
					assert.Zero(t, info.Size())
					assert.Equal(t, os.FileMode(0600), info.Mode().Perm())
					current, err := m.currentReleaseID()
					require.NoError(t, err)
					assert.Equal(t, previous, current, "persist intent before selection")
				}
				if syncs == failSync {
					return syncErr
				}
				return syncDirectory(path)
			}
			runner.calls = nil
			next := renewedCredentials(t, initial, 2)
			require.ErrorIs(t, m.UpdateCredentials(context.Background(), "machine-1", next), syncErr)
			current, err := m.currentReleaseID()
			require.NoError(t, err)
			pending, err := m.reloadPending()
			require.NoError(t, err)
			if failSync == 1 {
				assert.Equal(t, previous, current)
			} else {
				assert.NotEqual(t, previous, current)
			}
			if failSync < 3 {
				assert.True(t, pending)
				assert.NotContains(t, runner.calls, hupCommand)
			} else {
				assert.False(t, pending, "notification was delivered before marker removal")
				assert.Contains(t, runner.calls, hupCommand)
			}
			m.syncDir = syncDirectory
			require.NoError(t, m.UpdateCredentials(context.Background(), "machine-1", next))
		})
	}
}

func TestReloadPendingRejectsInvalidMarkerWithoutFollowingSymlinks(t *testing.T) {
	for _, mode := range []string{"directory", "symlink", "nonempty"} {
		t.Run(mode, func(t *testing.T) {
			m, runner, paths := newTestManager(t)
			initial := newTestCredentials(t, "worker-1", "machine-1", 1)
			require.NoError(t, m.UpdateCredentials(context.Background(), "machine-1", initial))
			runner.startAgent()
			require.NoError(t, m.Activate(context.Background()))
			marker := filepath.Join(paths.StateDir, reloadPendingFileName)
			outside := filepath.Join(t.TempDir(), "outside")
			require.NoError(t, os.WriteFile(outside, []byte("unchanged"), 0600))
			switch mode {
			case "directory":
				require.NoError(t, os.Mkdir(marker, 0700))
			case "symlink":
				require.NoError(t, os.Symlink(outside, marker))
			case "nonempty":
				require.NoError(t, os.WriteFile(marker, []byte("invalid"), 0600))
			}
			current, err := m.currentReleaseID()
			require.NoError(t, err)
			runner.calls = nil
			require.ErrorContains(t, m.UpdateCredentials(context.Background(), "machine-1", renewedCredentials(t, initial, 2)), "invalid KAP mTLS reload notification")
			selected, err := m.currentReleaseID()
			require.NoError(t, err)
			assert.Equal(t, current, selected)
			assert.NotContains(t, runner.calls, hupCommand)
			data, err := os.ReadFile(outside)
			require.NoError(t, err)
			assert.Equal(t, "unchanged", string(data))
		})
	}
}
