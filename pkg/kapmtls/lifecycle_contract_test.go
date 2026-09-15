// SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package kapmtls

import (
	"context"
	"crypto/sha256"
	"crypto/x509"
	"encoding/pem"
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

	"github.com/leptonai/gpud/pkg/packagelock"
)

type contractInactiveError struct{}

func (contractInactiveError) Error() string { return "inactive" }
func (contractInactiveError) ExitCode() int { return 3 }

type contractAgent struct {
	paths        Paths
	active       atomic.Bool
	metrics      atomic.Value
	requests     atomic.Int64
	metricsCode  atomic.Int64
	mu           sync.Mutex
	commands     []string
	beforeReload func() error
	afterReload  func()
}

func (a *contractAgent) Run(_ context.Context, command string, args ...string) ([]byte, error) {
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
	if call != "systemctl kill --signal=HUP --kill-who=main "+AgentService {
		return nil, fmt.Errorf("credential manager issued forbidden command: %s", call)
	}
	if a.beforeReload != nil {
		if err := a.beforeReload(); err != nil {
			return nil, err
		}
	}
	certificate, err := os.ReadFile(filepath.Join(a.paths.StateDir, CurrentSymlinkName, ClientCertificateFileName))
	if err != nil {
		return nil, err
	}
	if err := a.load(certificate); err != nil {
		return nil, err
	}
	if a.afterReload != nil {
		a.afterReload()
	}
	return nil, nil
}

func (a *contractAgent) load(certificate []byte) error {
	block, _ := pem.Decode(certificate)
	if block == nil {
		return errors.New("missing certificate PEM")
	}
	leaf, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return err
	}
	a.metrics.Store(fmt.Sprintf("# TYPE kaproxy_mtls_agent_cert_info gauge\nkaproxy_mtls_agent_cert_info{serial=%q} 1\n# TYPE kaproxy_mtls_agent_cert_not_after_timestamp_seconds gauge\nkaproxy_mtls_agent_cert_not_after_timestamp_seconds %d\n", leaf.SerialNumber.Text(16), leaf.NotAfter.Unix()))
	return nil
}

func (a *contractAgent) calls() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]string(nil), a.commands...)
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
	a := &contractAgent{paths: paths}
	a.metrics.Store("")
	a.metricsCode.Store(http.StatusOK)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		a.requests.Add(1)
		if r.URL.Path == "/metrics" {
			w.WriteHeader(int(a.metricsCode.Load()))
			_, _ = fmt.Fprint(w, a.metrics.Load().(string))
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)
	m := NewManager(paths)
	m.runner = a
	m.httpClient = server.Client()
	m.readyURL = server.URL + "/readyz"
	m.now = func() time.Time { return testNow }
	m.reloadTimeout = 150 * time.Millisecond
	return m, a, paths
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

func assertContractLockReleased(t *testing.T) {
	t.Helper()
	mu := packagelock.For(PackageName)
	if !assert.True(t, mu.TryLock(), "credential operation must release the package lifecycle lock") {
		return
	}
	mu.Unlock()
}

func assertContractOnlyProbesAndReloads(t *testing.T, a *contractAgent) {
	t.Helper()
	for _, call := range a.calls() {
		assert.Contains(t, []string{"systemctl is-active " + AgentService, "systemctl kill --signal=HUP --kill-who=main " + AgentService}, call)
	}
}

func TestLifecycleContractInactiveStagingAndCanceledLockHandoff(t *testing.T) {
	m, agent, paths := newContractManager(t)
	credentials := newTestCredentials(t, "worker-1", "machine-1", 1)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	require.NoError(t, m.UpdateCredentials(ctx, "machine-1", credentials))
	assert.Zero(t, agent.requests.Load(), "inactive staging must not wait for readiness or certificate metrics")
	assertContractLockReleased(t)
	before := contractSnapshot(t, paths.StateDir)

	status, err := m.Status(ctx, "machine-1")
	require.NoError(t, err)
	assert.True(t, status.CredentialsInstalled)
	assert.False(t, status.AgentActive)
	assert.False(t, status.AgentReady)
	assert.Equal(t, "1", status.CertificateSerial)
	require.Error(t, m.Activate(ctx))
	assert.Equal(t, before, contractSnapshot(t, paths.StateDir))
	assert.Zero(t, agent.requests.Load())
	assert.NotContains(t, agent.calls(), "systemctl kill --signal=HUP --kill-who=main "+AgentService)

	mu := packagelock.For(PackageName)
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
		t.Fatal("canceled credential update did not relinquish package operation")
	}
	assert.Equal(t, before, contractSnapshot(t, paths.StateDir))
	assertContractLockReleased(t)
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
				require.NoError(t, agent.load(initial.CertificatePEM))
				next := renewedCredentials(t, initial, 2)
				mutate(&next)
				before := contractSnapshot(t, paths.StateDir)
				callsBefore := len(agent.calls())
				require.Error(t, m.UpdateCredentials(context.Background(), "machine-1", next))
				assert.Equal(t, before, contractSnapshot(t, paths.StateDir))
				for _, call := range agent.calls()[callsBefore:] {
					assert.Equal(t, "systemctl is-active "+AgentService, call, "rejected drift must not signal or control the service")
				}
				assertContractLockReleased(t)
			})
		}
	}
}

func TestLifecycleContractRetainedStartupConflictCannotBeAcknowledgedByLeaf(t *testing.T) {
	m, agent, paths := newContractManager(t)
	initial := newTestCredentials(t, "worker-1", "machine-1", 1)
	require.NoError(t, m.UpdateCredentials(context.Background(), "machine-1", initial))
	next := renewedCredentials(t, initial, 2)
	next.GatewayEndpoint = "new-gateway.example.test:8443"
	next.ServerName = "new-gateway.example.test"
	other, _, otherPaths := newContractManager(t)
	require.NoError(t, other.UpdateCredentials(context.Background(), "machine-1", next))
	selected, err := os.Readlink(filepath.Join(otherPaths.StateDir, CurrentSymlinkName))
	require.NoError(t, err)
	copyContractRelease(t, filepath.Join(otherPaths.StateDir, selected), filepath.Join(paths.StateDir, selected))
	require.NoError(t, os.Remove(filepath.Join(paths.StateDir, CurrentSymlinkName)))
	require.NoError(t, os.Symlink(selected, filepath.Join(paths.StateDir, CurrentSymlinkName)))
	agent.active.Store(true)
	require.NoError(t, agent.load(next.CertificatePEM))
	before := contractSnapshot(t, paths.StateDir)

	require.Error(t, m.Activate(context.Background()), "B leaf metrics cannot prove B startup configuration")
	_, err = m.Status(context.Background(), "machine-1")
	require.Error(t, err, "retained A startup configuration conflicts with selected B")
	require.Error(t, m.UpdateCredentials(context.Background(), "machine-1", next))
	require.Error(t, m.UpdateCredentials(context.Background(), "machine-1", renewedCredentials(t, next, 3)))
	assert.Equal(t, before, contractSnapshot(t, paths.StateDir))
	assertContractOnlyProbesAndReloads(t, agent)
	assert.NotContains(t, agent.calls(), "systemctl kill --signal=HUP --kill-who=main "+AgentService)
	assertContractLockReleased(t)
}

func copyContractRelease(t *testing.T, source, target string) {
	t.Helper()
	require.NoError(t, os.Mkdir(target, 0700))
	entries, err := os.ReadDir(source)
	require.NoError(t, err)
	for _, entry := range entries {
		data, err := os.ReadFile(filepath.Join(source, entry.Name()))
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(target, entry.Name()), data, 0600))
	}
}

func TestLifecycleContractCorruptRetainedGenerationNeedsMaintenance(t *testing.T) {
	m, agent, paths := newContractManager(t)
	initial := newTestCredentials(t, "worker-1", "machine-1", 1)
	require.NoError(t, m.UpdateCredentials(context.Background(), "machine-1", initial))
	retained := filepath.Join(paths.StateDir, ReleasesDirectoryName, "retained-corrupt")
	copyContractRelease(t, filepath.Join(paths.StateDir, CurrentSymlinkName), retained)
	require.NoError(t, os.WriteFile(filepath.Join(retained, ClientCertificateFileName), []byte("incomplete certificate"), 0600))
	before := contractSnapshot(t, paths.StateDir)
	require.Error(t, m.UpdateCredentials(context.Background(), "machine-1", renewedCredentials(t, initial, 2)))
	require.Error(t, m.Activate(context.Background()))
	_, err := m.Status(context.Background(), "machine-1")
	require.Error(t, err)
	assert.Equal(t, before, contractSnapshot(t, paths.StateDir))
	assertContractOnlyProbesAndReloads(t, agent)
	assertContractLockReleased(t)
}

func TestLifecycleContractFailedReloadPreservesSelectionForRetry(t *testing.T) {
	for _, failure := range []string{"signal failure", "metrics missing", "metrics invalid", "metrics expiry mismatch", "metrics outage", "cancel after selection"} {
		t.Run(failure, func(t *testing.T) {
			m, agent, paths := newContractManager(t)
			initial := newTestCredentials(t, "worker-1", "machine-1", 1)
			require.NoError(t, m.UpdateCredentials(context.Background(), "machine-1", initial))
			agent.active.Store(true)
			require.NoError(t, agent.load(initial.CertificatePEM))
			previous, err := os.Readlink(filepath.Join(paths.StateDir, CurrentSymlinkName))
			require.NoError(t, err)
			next := renewedCredentials(t, initial, 2)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			agent.beforeReload = func() error {
				switch failure {
				case "signal failure":
					return errors.New("injected signal failure")
				case "metrics missing":
					agent.metricsCode.Store(http.StatusNoContent)
				case "metrics invalid":
					agent.afterReload = func() {
						agent.metrics.Store("kaproxy_mtls_agent_cert_info{serial=\"2\"} 1\nkaproxy_mtls_agent_cert_not_after_timestamp_seconds NaN\n")
					}
				case "metrics expiry mismatch":
					agent.afterReload = func() {
						agent.metrics.Store(fmt.Sprintf("kaproxy_mtls_agent_cert_info{serial=\"2\"} 1\nkaproxy_mtls_agent_cert_not_after_timestamp_seconds %d\n", testNow.Add(time.Hour).Unix()))
					}
				case "metrics outage":
					agent.metricsCode.Store(http.StatusServiceUnavailable)
				case "cancel after selection":
					cancel()
					return ctx.Err()
				}
				return nil
			}
			err = m.UpdateCredentials(ctx, "machine-1", next)
			require.Error(t, err)
			if failure == "cancel after selection" {
				require.ErrorIs(t, err, context.Canceled)
			}
			selected, err := os.Readlink(filepath.Join(paths.StateDir, CurrentSymlinkName))
			require.NoError(t, err)
			assert.NotEqual(t, previous, selected, "failed reload must never select the previous generation")
			selectedCertificate, err := os.ReadFile(filepath.Join(paths.StateDir, CurrentSymlinkName, ClientCertificateFileName))
			require.NoError(t, err)
			assert.Equal(t, next.CertificatePEM, selectedCertificate)
			selectedFiles := contractSnapshot(t, filepath.Join(paths.StateDir, selected))
			assertContractLockReleased(t)

			agent.beforeReload = nil
			agent.afterReload = nil
			agent.metricsCode.Store(http.StatusOK)
			require.NoError(t, agent.load(initial.CertificatePEM))
			status, err := m.Status(context.Background(), "machine-1")
			require.NoError(t, err)
			assert.False(t, status.CredentialsInstalled, "running A cannot acknowledge selected B")
			require.NoError(t, m.UpdateCredentials(context.Background(), "machine-1", next))
			afterRetry, err := os.Readlink(filepath.Join(paths.StateDir, CurrentSymlinkName))
			require.NoError(t, err)
			assert.Equal(t, selected, afterRetry, "retry should reuse its complete selected generation")
			assert.Equal(t, selectedFiles, contractSnapshot(t, filepath.Join(paths.StateDir, selected)))
			beforeProbe := contractSnapshot(t, paths.StateDir)
			callsBeforeProbe := len(agent.calls())
			require.NoError(t, m.Activate(context.Background()))
			assert.Equal(t, beforeProbe, contractSnapshot(t, paths.StateDir))
			for _, call := range agent.calls()[callsBeforeProbe:] {
				assert.Equal(t, "systemctl is-active "+AgentService, call, "activation is a side-effect-free probe")
			}
			status, err = m.Status(context.Background(), "machine-1")
			require.NoError(t, err)
			assert.True(t, status.CredentialsInstalled)
			assert.Equal(t, "2", status.CertificateSerial)
			assertContractOnlyProbesAndReloads(t, agent)
			assertContractLockReleased(t)
		})
	}
}
