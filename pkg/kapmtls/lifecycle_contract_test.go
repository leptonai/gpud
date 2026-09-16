// SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package kapmtls

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const contractHUP = "systemctl kill --signal=HUP --kill-who=main " + AgentService

type contractInactiveError struct{}

func (contractInactiveError) Error() string { return "inactive" }
func (contractInactiveError) ExitCode() int { return 3 }

type contractAgent struct {
	active       atomic.Bool
	requests     atomic.Int64
	metrics      atomic.Int64
	mu           sync.Mutex
	commands     []string
	beforeReload func(context.Context) error
}

func (a *contractAgent) Run(ctx context.Context, command string, args ...string) ([]byte, error) {
	call := strings.Join(append([]string{command}, args...), " ")
	a.mu.Lock()
	a.commands = append(a.commands, call)
	a.mu.Unlock()
	if call == "systemctl is-active "+AgentService {
		if a.active.Load() {
			return []byte("active\n"), nil
		}
		return []byte("inactive\n"), contractInactiveError{}
	}
	if call != contractHUP {
		return nil, fmt.Errorf("credential manager issued forbidden command: %s", call)
	}
	if a.beforeReload != nil {
		return nil, a.beforeReload(ctx)
	}
	return nil, ctx.Err()
}

func (a *contractAgent) calls() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]string(nil), a.commands...)
}

func (a *contractAgent) reloads() int {
	count := 0
	for _, call := range a.calls() {
		if call == contractHUP {
			count++
		}
	}
	return count
}

func newContractManager(t *testing.T) (*Manager, *contractAgent, Paths) {
	t.Helper()
	root := t.TempDir()
	paths := DefaultPaths(root)
	paths.AgentBinary = filepath.Join(root, "agent")
	paths.AgentUnitFile = filepath.Join(root, "agent.service")
	require.NoError(t, os.MkdirAll(paths.StateDir, 0700))
	require.NoError(t, os.WriteFile(paths.AgentBinary, []byte("test binary"), 0700))
	require.NoError(t, os.WriteFile(paths.AgentUnitFile, []byte("test unit"), 0600))
	a := &contractAgent{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		a.requests.Add(1)
		if r.URL.Path == "/readyz" {
			w.WriteHeader(http.StatusOK)
			return
		}
		a.metrics.Add(1)
		http.Error(w, "no certificate metrics available", http.StatusNotFound)
	}))
	t.Cleanup(server.Close)
	m := NewManager(paths)
	m.runner = a
	m.httpClient = server.Client()
	m.readyURL = server.URL + "/readyz"
	m.now = func() time.Time { return testNow }
	return m, a, paths
}

func reopenContractManager(m *Manager, paths Paths) *Manager {
	reopened := NewManager(paths)
	reopened.runner, reopened.httpClient = m.runner, m.httpClient
	reopened.readyURL, reopened.now = m.readyURL, m.now
	return reopened
}

func contractSnapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	result := make(map[string]string)
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 {
			target, err := os.Readlink(path)
			result[relative] = "link:" + target
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		result[relative] = info.Mode().String()
		if !entry.IsDir() {
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			result[relative] += fmt.Sprintf(":%x", sha256.Sum256(data))
		}
		return nil
	})
	require.NoError(t, err)
	return result
}

func assertContractLockReleased(t *testing.T, m *Manager) {
	t.Helper()
	mu := lockForStateDirectory(m.paths.StateDir)
	if !assert.True(t, mu.TryLock(), "credential operation must release the KAP state-directory lock") {
		return
	}
	mu.Unlock()
}

func assertContractOnlyProbesAndReloads(t *testing.T, a *contractAgent) {
	t.Helper()
	for _, call := range a.calls() {
		assert.Contains(t, []string{"systemctl is-active " + AgentService, contractHUP}, call)
	}
	assert.Zero(t, a.metrics.Load(), "certificate notification must not depend on runtime metrics")
}

func TestLifecycleContractActiveUpdateOnlyNotifies(t *testing.T) {
	m, agent, paths := newContractManager(t)
	initial := newTestCredentials(t, "worker-1", "machine-1", 1)
	require.NoError(t, m.UpdateCredentials(context.Background(), "machine-1", initial))
	agent.active.Store(true)
	credentials := renewedCredentials(t, initial, 2)
	agent.beforeReload = func(ctx context.Context) error {
		deadline, bounded := ctx.Deadline()
		assert.True(t, bounded, "HUP delivery must have a bounded deadline")
		assert.True(t, deadline.After(time.Now()))
		selected, err := os.ReadFile(filepath.Join(paths.StateDir, CurrentSymlinkName, ClientCertificateFileName))
		require.NoError(t, err)
		assert.Equal(t, credentials.CertificatePEM, selected, "complete credentials must be selected before notification")
		key, err := os.ReadFile(filepath.Join(paths.StateDir, CurrentSymlinkName, ClientPrivateKeyFileName))
		require.NoError(t, err)
		_, err = tls.X509KeyPair(selected, key)
		require.NoError(t, err)
		return nil
	}

	require.NoError(t, m.UpdateCredentials(context.Background(), "machine-1", credentials))
	assert.Equal(t, 1, agent.reloads())
	assert.Zero(t, agent.requests.Load(), "successful notification does not wait for ready or loaded proof")
	before := contractSnapshot(t, paths.StateDir)
	status, err := m.Status(context.Background(), "machine-1")
	require.NoError(t, err)
	assert.True(t, status.CredentialsInstalled)
	assert.True(t, status.AgentReady)
	require.NoError(t, m.Activate(context.Background()))
	assert.Equal(t, 1, agent.reloads(), "healthy probes must not resend HUP")
	assert.Equal(t, before, contractSnapshot(t, paths.StateDir))
	assertContractLockReleased(t, m)
	assertContractOnlyProbesAndReloads(t, agent)
}

func TestLifecycleContractInactiveStagingAndCanceledLockHandoff(t *testing.T) {
	m, agent, paths := newContractManager(t)
	credentials := newTestCredentials(t, "worker-1", "machine-1", 1)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	require.NoError(t, m.UpdateCredentials(ctx, "machine-1", credentials))
	require.NoError(t, ctx.Err(), "inactive staging must return without waiting for package startup")
	assert.Zero(t, agent.requests.Load())
	assertContractLockReleased(t, m)
	before := contractSnapshot(t, paths.StateDir)

	status, err := m.Status(ctx, "machine-1")
	require.NoError(t, err)
	assert.True(t, status.CredentialsInstalled)
	assert.False(t, status.AgentActive)
	assert.False(t, status.AgentReady)
	assert.Equal(t, "1", status.CertificateSerial)
	require.Error(t, m.Activate(ctx))
	require.NoError(t, ctx.Err())
	assert.Equal(t, before, contractSnapshot(t, paths.StateDir))
	assert.Zero(t, agent.requests.Load())
	assert.Zero(t, agent.reloads())

	mu := lockForStateDirectory(m.paths.StateDir)
	mu.Lock()
	canceled, stop := context.WithCancel(context.Background())
	stop()
	done := make(chan error, 1)
	go func() { done <- m.UpdateCredentials(canceled, "machine-1", credentials) }()
	mu.Unlock()
	select {
	case err := <-done:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(time.Second):
		t.Fatal("canceled credential update did not relinquish the KAP operation")
	}
	assert.Equal(t, before, contractSnapshot(t, paths.StateDir))
	assertContractLockReleased(t, m)

	// Package startup is external; the remaining notification is delivered once active.
	agent.active.Store(true)
	m = reopenContractManager(m, paths)
	status, err = m.Status(context.Background(), "machine-1")
	require.NoError(t, err)
	assert.True(t, status.CredentialsInstalled)
	assert.False(t, status.AgentReady)
	assert.Zero(t, agent.reloads(), "Status must leave notification retries to Activate")
	require.NoError(t, m.Activate(context.Background()))
	assert.Equal(t, 1, agent.reloads())
	status, err = m.Status(context.Background(), "machine-1")
	require.NoError(t, err)
	assert.True(t, status.AgentReady)
	assertContractOnlyProbesAndReloads(t, agent)
}

func TestLifecycleContractStartupDriftNeverWrites(t *testing.T) {
	mutations := map[string]func(*Credentials){
		"endpoint": func(c *Credentials) { c.GatewayEndpoint = "kap.example.test:8444" },
		"TLS name": func(c *Credentials) {
			c.GatewayEndpoint, c.ServerName = "other.example.test:8443", "other.example.test"
		},
		"client CA": func(c *Credentials) { c.ClientCAFingerprint = strings.Repeat("a", 64) },
		"gateway CA": func(c *Credentials) {
			other := newTestCredentials(t, "worker-1", "machine-1", 3)
			c.GatewayCAPEM, c.GatewayCAFingerprint = other.GatewayCAPEM, other.GatewayCAFingerprint
		},
	}
	for name, mutate := range mutations {
		for _, active := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/active=%t", name, active), func(t *testing.T) {
				m, agent, paths := newContractManager(t)
				initial := newTestCredentials(t, "worker-1", "machine-1", 1)
				require.NoError(t, m.UpdateCredentials(context.Background(), "machine-1", initial))
				agent.active.Store(active)
				next := renewedCredentials(t, initial, 2)
				mutate(&next)
				before := contractSnapshot(t, paths.StateDir)
				require.Error(t, m.UpdateCredentials(context.Background(), "machine-1", next))
				assert.Equal(t, before, contractSnapshot(t, paths.StateDir))
				assert.Zero(t, agent.reloads(), "rejected startup drift must not notify or control the service")
				assertContractLockReleased(t, m)
				assertContractOnlyProbesAndReloads(t, agent)
			})
		}
	}
}

func TestLifecycleContractUnselectedHistoryDoesNotBlockRenewal(t *testing.T) {
	m, agent, paths := newContractManager(t)
	initial := newTestCredentials(t, "worker-1", "machine-1", 1)
	require.NoError(t, m.UpdateCredentials(context.Background(), "machine-1", initial))
	retained := filepath.Join(paths.StateDir, ReleasesDirectoryName, "unselected-incomplete")
	require.NoError(t, os.Mkdir(retained, 0700))
	require.NoError(t, os.WriteFile(filepath.Join(retained, ClientCertificateFileName), []byte("incomplete"), 0600))
	agent.active.Store(true)

	require.NoError(t, m.UpdateCredentials(context.Background(), "machine-1", renewedCredentials(t, initial, 2)))
	status, err := m.Status(context.Background(), "machine-1")
	require.NoError(t, err)
	assert.Equal(t, "2", status.CertificateSerial)
	assert.True(t, status.AgentReady)
	assert.Equal(t, 1, agent.reloads())
	assertContractOnlyProbesAndReloads(t, agent)
}

func TestLifecycleContractNotificationRetrySurvivesManagerRestart(t *testing.T) {
	for _, failure := range []string{"signal failure", "cancel after selection", "crash after selection"} {
		t.Run(failure, func(t *testing.T) {
			m, agent, paths := newContractManager(t)
			initial := newTestCredentials(t, "worker-1", "machine-1", 1)
			require.NoError(t, m.UpdateCredentials(context.Background(), "machine-1", initial))
			agent.active.Store(true)
			require.NoError(t, m.Activate(context.Background()))
			previous, err := os.Readlink(filepath.Join(paths.StateDir, CurrentSymlinkName))
			require.NoError(t, err)
			next := renewedCredentials(t, initial, 2)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			agent.beforeReload = func(context.Context) error {
				switch failure {
				case "cancel after selection":
					cancel()
					return ctx.Err()
				case "crash after selection":
					panic("simulated process loss before HUP delivery")
				default:
					return errors.New("injected signal failure")
				}
			}
			if failure == "crash after selection" {
				require.Panics(t, func() { _ = m.UpdateCredentials(ctx, "machine-1", next) })
			} else {
				err = m.UpdateCredentials(ctx, "machine-1", next)
				require.Error(t, err)
				if failure == "cancel after selection" {
					require.ErrorIs(t, err, context.Canceled)
				}
			}
			assertContractLockReleased(t, m)
			selected, err := os.Readlink(filepath.Join(paths.StateDir, CurrentSymlinkName))
			require.NoError(t, err)
			assert.NotEqual(t, previous, selected, "notification failure must not roll back selected credentials")
			selectedFiles := contractSnapshot(t, filepath.Join(paths.StateDir, selected))

			agent.beforeReload = nil
			m = reopenContractManager(m, paths)
			beforeStatus := contractSnapshot(t, paths.StateDir)
			attempts := agent.reloads()
			status, err := m.Status(context.Background(), "machine-1")
			require.NoError(t, err)
			assert.True(t, status.CredentialsInstalled, "persisted credentials are not loaded-certificate proof")
			assert.Equal(t, "2", status.CertificateSerial)
			assert.True(t, status.AgentActive)
			assert.False(t, status.AgentReady, "an undelivered notification must remain retryable")
			assert.Equal(t, attempts, agent.reloads(), "Status must not deliver HUP")
			assert.Equal(t, beforeStatus, contractSnapshot(t, paths.StateDir))

			agent.beforeReload = func(context.Context) error { return errors.New("retry signal failed") }
			require.Error(t, m.Activate(context.Background()))
			assert.Equal(t, beforeStatus, contractSnapshot(t, paths.StateDir))
			m = reopenContractManager(m, paths)
			status, err = m.Status(context.Background(), "machine-1")
			require.NoError(t, err)
			assert.False(t, status.AgentReady, "a failed retry must retain the notification obligation")
			attempts = agent.reloads()
			agent.beforeReload = nil
			require.NoError(t, m.Activate(context.Background()))
			assert.Equal(t, attempts+1, agent.reloads())
			afterRetry, err := os.Readlink(filepath.Join(paths.StateDir, CurrentSymlinkName))
			require.NoError(t, err)
			assert.Equal(t, selected, afterRetry, "retry must not issue or select another certificate")
			assert.Equal(t, selectedFiles, contractSnapshot(t, filepath.Join(paths.StateDir, selected)))
			selectedCertificate, err := os.ReadFile(filepath.Join(paths.StateDir, selected, ClientCertificateFileName))
			require.NoError(t, err)
			assert.Equal(t, next.CertificatePEM, selectedCertificate)
			status, err = m.Status(context.Background(), "machine-1")
			require.NoError(t, err)
			assert.True(t, status.CredentialsInstalled)
			assert.True(t, status.AgentReady)
			require.NoError(t, m.Activate(context.Background()))
			assert.Equal(t, attempts+1, agent.reloads(), "delivered notification must not be retried again")
			assertContractOnlyProbesAndReloads(t, agent)
			assertContractLockReleased(t, m)
		})
	}
}
