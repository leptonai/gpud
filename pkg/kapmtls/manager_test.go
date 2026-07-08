// SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package kapmtls

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type runnerCall struct {
	command string
	args    []string
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

type commandRunnerFunc func(context.Context, string, ...string) ([]byte, error)

func (f commandRunnerFunc) Run(ctx context.Context, command string, args ...string) ([]byte, error) {
	return f(ctx, command, args...)
}

type fakeRunner struct {
	calls              []runnerCall
	state              string
	failAgentEnable    int
	failAgentDisable   int
	failAgentReload    int
	failAgentRestart   int
	failKubeletRestart int
}

func (r *fakeRunner) Run(_ context.Context, command string, args ...string) ([]byte, error) {
	r.calls = append(r.calls, runnerCall{command: command, args: append([]string(nil), args...)})
	if command != "systemctl" {
		return nil, nil
	}
	if len(args) == 2 && args[0] == "is-active" {
		state := r.state
		if state == "" {
			state = "inactive"
		}
		if state == "active" {
			return []byte(state + "\n"), nil
		}
		return []byte(state + "\n"), errors.New("exit status 3")
	}
	if len(args) == 4 && args[0] == "kill" && args[1] == "-s" && args[2] == "HUP" && args[3] == AgentService && r.failAgentReload > 0 {
		r.failAgentReload--
		return []byte("reload failed"), errors.New("exit status 1")
	}
	if len(args) == 2 && args[0] == "enable" && args[1] == AgentService && r.failAgentEnable > 0 {
		r.failAgentEnable--
		return []byte("enable failed"), errors.New("exit status 1")
	}
	if len(args) == 3 && args[0] == "disable" && args[1] == "--now" && args[2] == AgentService && r.failAgentDisable > 0 {
		r.failAgentDisable--
		return []byte("disable failed"), errors.New("exit status 1")
	}
	if len(args) == 2 && args[0] == "restart" && args[1] == AgentService {
		if r.failAgentRestart > 0 {
			r.failAgentRestart--
			return []byte("restart failed"), errors.New("exit status 1")
		}
		r.state = "active"
	}
	if len(args) == 2 && args[0] == "restart" && args[1] == KubeletService && r.failKubeletRestart > 0 {
		r.failKubeletRestart--
		return []byte("restart failed"), errors.New("exit status 1")
	}
	if len(args) == 3 && args[0] == "disable" && args[1] == "--now" && args[2] == AgentService {
		r.state = "inactive"
	}
	return nil, nil
}

func TestManagerCommitsCredentialsAsAtomicGeneration(t *testing.T) {
	now := time.Date(2026, 7, 6, 0, 0, 0, 0, time.UTC)
	paths := testPaths(t)
	kubeconfigCA := writeTestKubeconfig(t, paths.Kubeconfig, "https://kap.example.test", "")
	manager := newTestManager(paths, &fakeRunner{}, now)
	credentials := testCredentials(t, "worker-a", "machine-a", 1, now.Add(-time.Minute), now.Add(5*24*time.Hour))

	require.NoError(t, manager.UpdateCredentials(context.Background(), "machine-a", credentials))
	currentPath := filepath.Join(paths.StateDir, CurrentSymlinkName)
	currentInfo, err := os.Lstat(currentPath)
	require.NoError(t, err)
	assert.NotZero(t, currentInfo.Mode()&os.ModeSymlink)
	releaseID, err := manager.currentReleaseID()
	require.NoError(t, err)
	releaseDir := filepath.Join(paths.StateDir, ReleasesDirectoryName, releaseID)
	for _, name := range []string{ClientCertificateFileName, ClientPrivateKeyFileName, GatewayCAFileName, AgentEnvironmentFileName} {
		_, err := os.Stat(filepath.Join(releaseDir, name))
		require.NoError(t, err)
	}
	environment, err := os.ReadFile(filepath.Join(releaseDir, AgentEnvironmentFileName))
	require.NoError(t, err)
	assert.Contains(t, string(environment), credentials.ClientCAFingerprint)
	assert.Contains(t, string(environment), credentials.GatewayCAFingerprint)
	keyInfo, err := os.Stat(filepath.Join(currentPath, ClientPrivateKeyFileName))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0600), keyInfo.Mode().Perm())

	status, err := manager.Status(context.Background(), "machine-a")
	require.NoError(t, err)
	assert.True(t, status.CredentialsInstalled)
	assert.Equal(t, "1", status.CertificateSerial)
	assert.Equal(t, credentials.ClientCAFingerprint, status.ClientCAFingerprint)
	assert.Equal(t, credentials.GatewayCAFingerprint, status.GatewayCAFingerprint)
	assert.Equal(t, fingerprintForPEM(t, kubeconfigCA), status.KubeconfigCAFingerprint)
	assert.False(t, status.AgentReady)

	wrongMachine := testCredentials(t, "worker-a", "machine-b", 2, now.Add(-time.Minute), now.Add(5*24*time.Hour))
	err = manager.UpdateCredentials(context.Background(), "machine-a", wrongMachine)
	require.ErrorContains(t, err, "invalid SPIFFE identity")
	currentCertificate, err := os.ReadFile(filepath.Join(currentPath, ClientCertificateFileName))
	require.NoError(t, err)
	assert.Equal(t, credentials.CertificatePEM, currentCertificate)
}

func TestManagerAppliedMarkerAndReadinessTrackLoadedGeneration(t *testing.T) {
	now := time.Date(2026, 7, 6, 0, 0, 0, 0, time.UTC)
	paths := testPaths(t)
	writeTestKubeconfig(t, paths.Kubeconfig, "https://kap.example.test", "")
	installTestAgent(t, paths)
	runner := &fakeRunner{state: "active"}
	manager := newTestManager(paths, runner, now)
	ready := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	defer ready.Close()
	manager.readyURL = ready.URL

	first := testCredentials(t, "worker-a", "machine-a", 1, now.Add(-time.Minute), now.Add(5*24*time.Hour))
	require.NoError(t, manager.UpdateCredentials(context.Background(), "machine-a", first))
	firstID, err := manager.currentReleaseID()
	require.NoError(t, err)
	appliedID, err := manager.readMarker(AppliedMarkerName)
	require.NoError(t, err)
	assert.Equal(t, firstID, appliedID)

	runner.calls = nil
	second := testRenewedCredentials(t, first, "worker-a", "machine-a", 2, now.Add(-time.Minute), now.Add(5*24*time.Hour))
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	err = manager.UpdateCredentials(canceled, "machine-a", second)
	require.ErrorContains(t, err, "did not load certificate serial")
	currentID, err := manager.currentReleaseID()
	require.NoError(t, err)
	assert.Equal(t, firstID, currentID)
	appliedID, err = manager.readMarker(AppliedMarkerName)
	require.NoError(t, err)
	assert.Equal(t, firstID, appliedID)
	status, err := manager.Status(context.Background(), "machine-a")
	require.NoError(t, err)
	assert.True(t, status.AgentActive)
	assert.True(t, status.AgentReady)

	require.NoError(t, manager.UpdateCredentials(context.Background(), "machine-a", second))
	status, err = manager.Status(context.Background(), "machine-a")
	require.NoError(t, err)
	assert.True(t, status.AgentReady)
}

func TestManagerReloadsForLeafRenewalAndGarbageCollectsOldKeys(t *testing.T) {
	now := time.Date(2026, 7, 6, 0, 0, 0, 0, time.UTC)
	paths := testPaths(t)
	writeTestKubeconfig(t, paths.Kubeconfig, "https://kap.example.test", "")
	installTestAgent(t, paths)
	runner := &fakeRunner{state: "active"}
	manager := newTestManager(paths, runner, now)
	ready := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))
	defer ready.Close()
	manager.readyURL = ready.URL
	var releaseIDs []string
	var credentials Credentials
	for serial := int64(1); serial <= 3; serial++ {
		if serial == 1 {
			credentials = testCredentials(t, "worker-a", "machine-a", serial, now.Add(-time.Minute), now.Add(5*24*time.Hour))
		} else {
			credentials = testRenewedCredentials(t, credentials, "worker-a", "machine-a", serial, now.Add(-time.Minute), now.Add(5*24*time.Hour))
		}
		require.NoError(t, manager.UpdateCredentials(context.Background(), "machine-a", credentials))
		releaseID, err := manager.currentReleaseID()
		require.NoError(t, err)
		releaseIDs = append(releaseIDs, releaseID)
	}
	assert.Equal(t, 1, countCall(runner.calls, "systemctl", "restart", AgentService))
	assert.Equal(t, 2, countCall(runner.calls, "systemctl", "kill", "-s", "HUP", AgentService))
	entries, err := os.ReadDir(filepath.Join(paths.StateDir, ReleasesDirectoryName))
	require.NoError(t, err)
	require.Len(t, entries, 2)
	assert.NoFileExists(t, filepath.Join(paths.StateDir, ReleasesDirectoryName, releaseIDs[0]))
	assert.DirExists(t, filepath.Join(paths.StateDir, ReleasesDirectoryName, releaseIDs[1]))
	assert.DirExists(t, filepath.Join(paths.StateDir, ReleasesDirectoryName, releaseIDs[2]))
}

func TestManagerConfiguresAndDisablesKubeletOnlyWhenAgentReady(t *testing.T) {
	now := time.Date(2026, 7, 6, 0, 0, 0, 0, time.UTC)
	paths := testPaths(t)
	originalCA := writeTestKubeconfig(t, paths.Kubeconfig, "https://kap.example.test", "")
	installTestAgent(t, paths)
	runner := &fakeRunner{state: "active"}
	manager := newTestManager(paths, runner, now)
	readyStatus := http.StatusOK
	ready := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(readyStatus) }))
	defer ready.Close()
	manager.readyURL = ready.URL
	credentials := testCredentials(t, "worker-a", "machine-a", 1, now.Add(-time.Minute), now.Add(5*24*time.Hour))
	require.NoError(t, manager.UpdateCredentials(context.Background(), "machine-a", credentials))
	localAgentCA := credentials.GatewayCAPEM

	config := Config{
		Enabled:                  true,
		Server:                   LocalEndpoint,
		TLSServerName:            "kap.example.test",
		CertificateAuthorityData: localAgentCA,
	}
	readyStatus = http.StatusServiceUnavailable
	require.ErrorContains(t, manager.Configure(context.Background(), config), "not ready")
	readyStatus = http.StatusOK
	require.NoError(t, manager.Configure(context.Background(), config))
	server, tlsServerName, caFingerprint, err := inspectKubeconfig(paths.Kubeconfig)
	require.NoError(t, err)
	assert.Equal(t, LocalEndpoint, server)
	assert.Equal(t, "kap.example.test", tlsServerName)
	assert.Equal(t, fingerprintForPEM(t, localAgentCA), caFingerprint)

	require.NoError(t, manager.Configure(context.Background(), Config{
		Server:                   "https://kap.example.test",
		CertificateAuthorityData: originalCA,
	}))
	assert.FileExists(t, filepath.Join(paths.StateDir, DisabledMarkerName))
	status, err := manager.Status(context.Background(), "machine-a")
	require.NoError(t, err)
	assert.True(t, status.AgentDisabled)
	assertCall(t, runner.calls, "systemctl", "disable", "--now", AgentService)
	server, tlsServerName, caFingerprint, err = inspectKubeconfig(paths.Kubeconfig)
	require.NoError(t, err)
	assert.Equal(t, "https://kap.example.test", server)
	assert.Empty(t, tlsServerName)
	assert.Equal(t, fingerprintForPEM(t, originalCA), caFingerprint)

	runner.failAgentRestart = 1
	require.ErrorContains(t, manager.UpdateCredentials(context.Background(), "machine-a", credentials), "restart failed")
	assert.FileExists(t, filepath.Join(paths.StateDir, DisabledMarkerName))

	require.NoError(t, manager.UpdateCredentials(context.Background(), "machine-a", credentials))
	assert.NoFileExists(t, filepath.Join(paths.StateDir, DisabledMarkerName))
	status, err = manager.Status(context.Background(), "machine-a")
	require.NoError(t, err)
	assert.False(t, status.AgentDisabled)
	assertCall(t, runner.calls, "systemctl", "restart", AgentService)
}

func TestManagerReportsAndRetriesPendingKubeconfig(t *testing.T) {
	now := time.Date(2026, 7, 6, 0, 0, 0, 0, time.UTC)
	paths := testPaths(t)
	writeTestKubeconfig(t, paths.Kubeconfig, "https://kap.example.test", "")
	installTestAgent(t, paths)
	runner := &fakeRunner{state: "active"}
	manager := newTestManager(paths, runner, now)
	ready := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ready.Close()
	manager.readyURL = ready.URL
	credentials := testCredentials(t, "worker-a", "machine-a", 1, now.Add(-time.Minute), now.Add(5*24*time.Hour))
	require.NoError(t, manager.UpdateCredentials(context.Background(), "machine-a", credentials))
	config := Config{
		Enabled:                  true,
		Server:                   LocalEndpoint,
		TLSServerName:            credentials.ServerName,
		CertificateAuthorityData: credentials.GatewayCAPEM,
	}
	require.NoError(t, manager.Configure(context.Background(), config))
	require.NoError(t, manager.writeMarker(KubeconfigPendingMarkerName, "pending"))
	runner.calls = nil

	status, err := manager.Status(context.Background(), "machine-a")
	require.NoError(t, err)
	assert.True(t, status.KubeconfigPending)
	require.NoError(t, manager.Configure(context.Background(), config))
	assert.Equal(t, 1, countCall(runner.calls, "systemctl", "restart", KubeletService))
	pending, err := manager.markerExists(KubeconfigPendingMarkerName)
	require.NoError(t, err)
	assert.False(t, pending)
}

func TestManagerRetainsPendingWhenKubeconfigRetryRestartFails(t *testing.T) {
	now := time.Date(2026, 7, 6, 0, 0, 0, 0, time.UTC)
	paths := testPaths(t)
	writeTestKubeconfig(t, paths.Kubeconfig, "https://kap.example.test", "")
	installTestAgent(t, paths)
	runner := &fakeRunner{state: "active"}
	manager := newTestManager(paths, runner, now)
	ready := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ready.Close()
	manager.readyURL = ready.URL
	credentials := testCredentials(t, "worker-a", "machine-a", 1, now.Add(-time.Minute), now.Add(5*24*time.Hour))
	require.NoError(t, manager.UpdateCredentials(context.Background(), "machine-a", credentials))
	config := Config{
		Enabled:                  true,
		Server:                   LocalEndpoint,
		TLSServerName:            credentials.ServerName,
		CertificateAuthorityData: credentials.GatewayCAPEM,
	}
	require.NoError(t, manager.Configure(context.Background(), config))
	require.NoError(t, manager.writeMarker(KubeconfigPendingMarkerName, "pending"))
	runner.calls = nil
	runner.failKubeletRestart = 1

	err := manager.Configure(context.Background(), config)
	require.ErrorContains(t, err, "pending marker retained")
	pending, markerErr := manager.markerExists(KubeconfigPendingMarkerName)
	require.NoError(t, markerErr)
	assert.True(t, pending)
	assert.Equal(t, 1, countCall(runner.calls, "systemctl", "restart", KubeletService))
}

func TestManagerDisableRetriesKubeletBeforeStoppingAgent(t *testing.T) {
	now := time.Date(2026, 7, 6, 0, 0, 0, 0, time.UTC)
	paths := testPaths(t)
	publicCA := writeTestKubeconfig(t, paths.Kubeconfig, "https://kap.example.test", "")
	installTestAgent(t, paths)
	runner := &fakeRunner{state: "active"}
	manager := newTestManager(paths, runner, now)
	ready := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ready.Close()
	manager.readyURL = ready.URL
	credentials := testCredentials(t, "worker-a", "machine-a", 1, now.Add(-time.Minute), now.Add(5*24*time.Hour))
	require.NoError(t, manager.UpdateCredentials(context.Background(), "machine-a", credentials))
	enabled := Config{
		Enabled:                  true,
		Server:                   LocalEndpoint,
		TLSServerName:            credentials.ServerName,
		CertificateAuthorityData: credentials.GatewayCAPEM,
	}
	require.NoError(t, manager.Configure(context.Background(), enabled))
	disabled := Config{Server: "https://kap.example.test", CertificateAuthorityData: publicCA}
	_, updated, mode, changed, err := prepareKubeconfig(paths.Kubeconfig, disabled)
	require.NoError(t, err)
	require.True(t, changed)
	require.NoError(t, manager.writeMarker(KubeconfigPendingMarkerName, "pending"))
	require.NoError(t, atomicWriteFile(paths.Kubeconfig, updated, mode))
	runner.calls = nil

	require.NoError(t, manager.Configure(context.Background(), disabled))
	restartIndex := callIndex(runner.calls, "systemctl", "restart", KubeletService)
	disableIndex := callIndex(runner.calls, "systemctl", "disable", "--now", AgentService)
	require.NotEqual(t, -1, restartIndex)
	require.NotEqual(t, -1, disableIndex)
	assert.Less(t, restartIndex, disableIndex)
	pending, err := manager.markerExists(KubeconfigPendingMarkerName)
	require.NoError(t, err)
	assert.False(t, pending)
}

func TestManagerRejectsKubeconfigCAMismatch(t *testing.T) {
	now := time.Date(2026, 7, 6, 0, 0, 0, 0, time.UTC)
	paths := testPaths(t)
	originalCA := writeTestKubeconfig(t, paths.Kubeconfig, "https://kap.example.test", "")
	installTestAgent(t, paths)
	runner := &fakeRunner{state: "active"}
	manager := newTestManager(paths, runner, now)
	ready := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ready.Close()
	manager.readyURL = ready.URL
	credentials := testCredentials(t, "worker-a", "machine-a", 1, now.Add(-time.Minute), now.Add(5*24*time.Hour))
	require.NoError(t, manager.UpdateCredentials(context.Background(), "machine-a", credentials))
	runner.calls = nil

	err := manager.Configure(context.Background(), Config{
		Enabled:                  true,
		Server:                   LocalEndpoint,
		TLSServerName:            credentials.ServerName,
		CertificateAuthorityData: testCAPEM(t, "wrong-gateway-ca"),
	})
	require.ErrorContains(t, err, "does not match installed gateway CA fingerprint")
	server, tlsServerName, caFingerprint, inspectErr := inspectKubeconfig(paths.Kubeconfig)
	require.NoError(t, inspectErr)
	assert.Equal(t, "https://kap.example.test", server)
	assert.Empty(t, tlsServerName)
	assert.Equal(t, fingerprintForPEM(t, originalCA), caFingerprint)
	assert.Equal(t, 0, countCall(runner.calls, "systemctl", "restart", KubeletService))
}

func TestManagerRestoresKubeconfigWhenKubeletRestartFails(t *testing.T) {
	now := time.Date(2026, 7, 6, 0, 0, 0, 0, time.UTC)
	paths := testPaths(t)
	originalCA := writeTestKubeconfig(t, paths.Kubeconfig, "https://kap.example.test", "")
	installTestAgent(t, paths)
	runner := &fakeRunner{state: "active", failKubeletRestart: 1}
	manager := newTestManager(paths, runner, now)
	ready := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))
	defer ready.Close()
	manager.readyURL = ready.URL
	credentials := testCredentials(t, "worker-a", "machine-a", 1, now.Add(-time.Minute), now.Add(5*24*time.Hour))
	require.NoError(t, manager.UpdateCredentials(context.Background(), "machine-a", credentials))

	err := manager.Configure(context.Background(), Config{
		Enabled:                  true,
		Server:                   LocalEndpoint,
		TLSServerName:            "kap.example.test",
		CertificateAuthorityData: credentials.GatewayCAPEM,
	})
	require.ErrorContains(t, err, "restart kubelet")
	server, tlsServerName, caFingerprint, inspectErr := inspectKubeconfig(paths.Kubeconfig)
	require.NoError(t, inspectErr)
	assert.Equal(t, "https://kap.example.test", server)
	assert.Empty(t, tlsServerName)
	assert.Equal(t, fingerprintForPEM(t, originalCA), caFingerprint)
	pending, markerErr := manager.markerExists(KubeconfigPendingMarkerName)
	require.NoError(t, markerErr)
	assert.False(t, pending)
}

func TestManagerRejectsInvalidCAAndFingerprints(t *testing.T) {
	now := time.Date(2026, 7, 6, 0, 0, 0, 0, time.UTC)
	valid := testCredentials(t, "worker-a", "machine-a", 1, now.Add(-time.Minute), now.Add(5*24*time.Hour))
	tests := []struct {
		name   string
		mutate func(*Credentials)
		want   string
	}{
		{name: "empty gateway CA", mutate: func(c *Credentials) { c.GatewayCAPEM = nil }, want: "bundle is empty"},
		{name: "non CA certificate", mutate: func(c *Credentials) { c.GatewayCAPEM = c.CertificatePEM }, want: "is not a CA"},
		{name: "uppercase client fingerprint", mutate: func(c *Credentials) { c.ClientCAFingerprint = strings.ToUpper(c.ClientCAFingerprint) }, want: "lowercase hexadecimal"},
		{name: "invalid client fingerprint", mutate: func(c *Credentials) { c.ClientCAFingerprint = strings.Repeat("z", 64) }, want: "lowercase hexadecimal"},
		{name: "gateway fingerprint mismatch", mutate: func(c *Credentials) { c.GatewayCAFingerprint = strings.Repeat("0", 64) }, want: "does not match"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			credentials := valid
			test.mutate(&credentials)
			_, err := validateCredentials("machine-a", credentials, now, true)
			require.ErrorContains(t, err, test.want)
		})
	}
	require.ErrorContains(t, validateConfig(Config{
		Enabled:                  true,
		Server:                   LocalEndpoint,
		TLSServerName:            "kap.example.test",
		CertificateAuthorityData: []byte("not a certificate"),
	}), "invalid kubeconfig certificate authority data")
}

func TestManagerDoesNotApplyGenerationWhenGatewayHandshakeFails(t *testing.T) {
	now := time.Date(2026, 7, 6, 0, 0, 0, 0, time.UTC)
	paths := testPaths(t)
	writeTestKubeconfig(t, paths.Kubeconfig, "https://kap.example.test", "")
	installTestAgent(t, paths)
	runner := &fakeRunner{state: "active"}
	manager := newTestManager(paths, runner, now)
	ready := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))
	defer ready.Close()
	manager.readyURL = ready.URL
	manager.gatewayVerifier = func(context.Context, Credentials) error { return errors.New("connection refused") }
	credentials := testCredentials(t, "worker-a", "machine-a", 1, now.Add(-time.Minute), now.Add(5*24*time.Hour))

	err := manager.UpdateCredentials(context.Background(), "machine-a", credentials)
	require.ErrorContains(t, err, "connection refused")
	appliedID, markerErr := manager.readMarker(AppliedMarkerName)
	require.NoError(t, markerErr)
	assert.Empty(t, appliedID)
	currentID, currentErr := manager.currentReleaseID()
	require.ErrorIs(t, currentErr, os.ErrNotExist)
	assert.Empty(t, currentID)
	entries, readErr := os.ReadDir(filepath.Join(paths.StateDir, ReleasesDirectoryName))
	require.NoError(t, readErr)
	assert.Empty(t, entries)
	assert.Equal(t, 1, countCall(runner.calls, "systemctl", "restart", AgentService))
}

func TestManagerConfirmsRestartedSerialBeforeGatewayPreflight(t *testing.T) {
	now := time.Date(2026, 7, 6, 0, 0, 0, 0, time.UTC)
	paths := testPaths(t)
	writeTestKubeconfig(t, paths.Kubeconfig, "https://kap.example.test", "")
	installTestAgent(t, paths)
	manager := newTestManager(paths, &fakeRunner{state: "active"}, now)
	ready := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ready.Close()
	manager.readyURL = ready.URL
	var observedSerials []string
	manager.certificateSerialVerifier = func(_ context.Context, serial string) bool {
		observedSerials = append(observedSerials, serial)
		return true
	}
	manager.gatewayVerifier = func(context.Context, Credentials) error {
		if len(observedSerials) == 0 {
			return errors.New("gateway preflight ran before restart serial confirmation")
		}
		return nil
	}
	credentials := testCredentials(t, "worker-a", "machine-a", 1, now.Add(-time.Minute), now.Add(5*24*time.Hour))

	require.NoError(t, manager.UpdateCredentials(context.Background(), "machine-a", credentials))
	assert.Equal(t, []string{"1"}, observedSerials)
}

func TestManagerRejectsRestartWhenNewSerialIsNotLoaded(t *testing.T) {
	now := time.Date(2026, 7, 6, 0, 0, 0, 0, time.UTC)
	paths := testPaths(t)
	writeTestKubeconfig(t, paths.Kubeconfig, "https://kap.example.test", "")
	installTestAgent(t, paths)
	manager := newTestManager(paths, &fakeRunner{state: "active"}, now)
	ready := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ready.Close()
	manager.readyURL = ready.URL
	manager.certificateSerialVerifier = func(context.Context, string) bool { return false }
	var gatewayCalls atomic.Int32
	manager.gatewayVerifier = func(context.Context, Credentials) error {
		gatewayCalls.Add(1)
		return nil
	}
	credentials := testCredentials(t, "worker-a", "machine-a", 1, now.Add(-time.Minute), now.Add(5*24*time.Hour))

	err := manager.UpdateCredentials(context.Background(), "machine-a", credentials)
	require.ErrorContains(t, err, "did not load certificate serial 1")
	assert.Zero(t, gatewayCalls.Load())
	_, currentErr := manager.currentReleaseID()
	require.ErrorIs(t, currentErr, os.ErrNotExist)
	pending, markerErr := manager.markerExists(ActivationPendingMarkerName)
	require.NoError(t, markerErr)
	assert.False(t, pending)
}

func TestManagerKeepsActivationPendingWhenRestartRollbackSerialIsWrong(t *testing.T) {
	now := time.Date(2026, 7, 6, 0, 0, 0, 0, time.UTC)
	paths := testPaths(t)
	writeTestKubeconfig(t, paths.Kubeconfig, "https://kap.example.test", "")
	installTestAgent(t, paths)
	manager := newTestManager(paths, &fakeRunner{state: "active"}, now)
	ready := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ready.Close()
	manager.readyURL = ready.URL
	first := testCredentials(t, "worker-a", "machine-a", 1, now.Add(-time.Minute), now.Add(5*24*time.Hour))
	require.NoError(t, manager.UpdateCredentials(context.Background(), "machine-a", first))
	firstID, err := manager.currentReleaseID()
	require.NoError(t, err)
	manager.certificateSerialVerifier = func(_ context.Context, serial string) bool {
		return serial == "2"
	}
	manager.gatewayVerifier = func(context.Context, Credentials) error {
		return errors.New("gateway unavailable")
	}
	changedGateway := testCredentials(t, "worker-a", "machine-a", 2, now.Add(-time.Minute), now.Add(5*24*time.Hour))

	err = manager.UpdateCredentials(context.Background(), "machine-a", changedGateway)
	require.ErrorContains(t, err, "did not restore certificate serial 1")
	currentID, currentErr := manager.currentReleaseID()
	require.NoError(t, currentErr)
	assert.Equal(t, firstID, currentID)
	pending, markerErr := manager.markerExists(ActivationPendingMarkerName)
	require.NoError(t, markerErr)
	assert.True(t, pending)
}

func TestManagerRestoresAppliedGenerationWhenRotationGatewayHandshakeFails(t *testing.T) {
	now := time.Date(2026, 7, 6, 0, 0, 0, 0, time.UTC)
	paths := testPaths(t)
	writeTestKubeconfig(t, paths.Kubeconfig, "https://kap.example.test", "")
	installTestAgent(t, paths)
	runner := &fakeRunner{state: "active"}
	manager := newTestManager(paths, runner, now)
	ready := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))
	defer ready.Close()
	manager.readyURL = ready.URL
	var verifyErr error
	manager.gatewayVerifier = func(context.Context, Credentials) error { return verifyErr }

	first := testCredentials(t, "worker-a", "machine-a", 1, now.Add(-time.Minute), now.Add(5*24*time.Hour))
	require.NoError(t, manager.UpdateCredentials(context.Background(), "machine-a", first))
	firstID, err := manager.currentReleaseID()
	require.NoError(t, err)
	runner.calls = nil
	verifyErr = errors.New("gateway unavailable")
	second := testRenewedCredentials(t, first, "worker-a", "machine-a", 2, now.Add(-time.Minute), now.Add(5*24*time.Hour))
	var observedSerials []string
	manager.certificateSerialVerifier = func(_ context.Context, serial string) bool {
		observedSerials = append(observedSerials, serial)
		return true
	}

	err = manager.UpdateCredentials(context.Background(), "machine-a", second)
	require.ErrorContains(t, err, "gateway unavailable")
	require.ErrorContains(t, err, "restored previous applied generation")
	currentID, currentErr := manager.currentReleaseID()
	require.NoError(t, currentErr)
	assert.Equal(t, firstID, currentID)
	appliedID, markerErr := manager.readMarker(AppliedMarkerName)
	require.NoError(t, markerErr)
	assert.Equal(t, firstID, appliedID)
	assert.Equal(t, 0, countCall(runner.calls, "systemctl", "restart", AgentService))
	assert.Equal(t, 2, countCall(runner.calls, "systemctl", "kill", "-s", "HUP", AgentService))
	assert.Equal(t, []string{"2", "1"}, observedSerials)
	entries, readErr := os.ReadDir(filepath.Join(paths.StateDir, ReleasesDirectoryName))
	require.NoError(t, readErr)
	require.Len(t, entries, 1)
	assert.Equal(t, firstID, entries[0].Name())
}

func TestManagerReportsRollbackReadinessFailure(t *testing.T) {
	now := time.Date(2026, 7, 6, 0, 0, 0, 0, time.UTC)
	paths := testPaths(t)
	writeTestKubeconfig(t, paths.Kubeconfig, "https://kap.example.test", "")
	installTestAgent(t, paths)
	runner := &fakeRunner{state: "active"}
	manager := newTestManager(paths, runner, now)
	var readyStatus atomic.Int32
	readyStatus.Store(http.StatusOK)
	ready := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(int(readyStatus.Load()))
	}))
	defer ready.Close()
	manager.readyURL = ready.URL

	first := testCredentials(t, "worker-a", "machine-a", 1, now.Add(-time.Minute), now.Add(5*24*time.Hour))
	require.NoError(t, manager.UpdateCredentials(context.Background(), "machine-a", first))
	firstID, err := manager.currentReleaseID()
	require.NoError(t, err)
	manager.gatewayVerifier = func(context.Context, Credentials) error {
		readyStatus.Store(http.StatusServiceUnavailable)
		return errors.New("gateway unavailable")
	}
	second := testRenewedCredentials(t, first, "worker-a", "machine-a", 2, now.Add(-time.Minute), now.Add(5*24*time.Hour))

	err = manager.UpdateCredentials(context.Background(), "machine-a", second)
	require.ErrorContains(t, err, "gateway unavailable")
	require.ErrorContains(t, err, "rollback generation "+firstID+" did not become ready")
	currentID, currentErr := manager.currentReleaseID()
	require.NoError(t, currentErr)
	assert.Equal(t, firstID, currentID)
	appliedID, markerErr := manager.readMarker(AppliedMarkerName)
	require.NoError(t, markerErr)
	assert.Equal(t, firstID, appliedID)
}

func TestManagerStatusRequiresNoActivationPendingAndLoadedCurrentSerial(t *testing.T) {
	now := time.Date(2026, 7, 6, 0, 0, 0, 0, time.UTC)
	paths := testPaths(t)
	writeTestKubeconfig(t, paths.Kubeconfig, "https://kap.example.test", "")
	installTestAgent(t, paths)
	manager := newTestManager(paths, &fakeRunner{state: "active"}, now)
	ready := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ready.Close()
	manager.readyURL = ready.URL
	credentials := testCredentials(t, "worker-a", "machine-a", 1, now.Add(-time.Minute), now.Add(5*24*time.Hour))
	require.NoError(t, manager.UpdateCredentials(context.Background(), "machine-a", credentials))

	status, err := manager.Status(context.Background(), "machine-a")
	require.NoError(t, err)
	assert.True(t, status.AgentReady)
	require.NoError(t, manager.writeMarker(ActivationPendingMarkerName, "pending-generation"))
	status, err = manager.Status(context.Background(), "machine-a")
	require.NoError(t, err)
	assert.False(t, status.AgentReady)
	require.NoError(t, manager.clearMarker(ActivationPendingMarkerName))
	manager.certificateSerialVerifier = func(context.Context, string) bool { return false }
	status, err = manager.Status(context.Background(), "machine-a")
	require.NoError(t, err)
	assert.False(t, status.AgentReady)
}

func TestManagerRetriesActivationPendingAfterCrashBeforeReload(t *testing.T) {
	now := time.Date(2026, 7, 6, 0, 0, 0, 0, time.UTC)
	paths := testPaths(t)
	writeTestKubeconfig(t, paths.Kubeconfig, "https://kap.example.test", "")
	installTestAgent(t, paths)
	runner := &fakeRunner{state: "active"}
	manager := newTestManager(paths, runner, now)
	ready := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ready.Close()
	manager.readyURL = ready.URL
	first := testCredentials(t, "worker-a", "machine-a", 1, now.Add(-time.Minute), now.Add(5*24*time.Hour))
	require.NoError(t, manager.UpdateCredentials(context.Background(), "machine-a", first))
	second := testRenewedCredentials(t, first, "worker-a", "machine-a", 2, now.Add(-time.Minute), now.Add(5*24*time.Hour))
	generation, err := manager.stageCredentials("machine-a", second)
	require.NoError(t, err)
	require.NoError(t, manager.writeMarker(ActivationPendingMarkerName, generation.releaseID))
	require.NoError(t, manager.swapCurrentSymlink(generation.releaseID))

	status, err := manager.Status(context.Background(), "machine-a")
	require.NoError(t, err)
	assert.False(t, status.AgentReady)
	runner.calls = nil
	require.NoError(t, manager.UpdateCredentials(context.Background(), "machine-a", second))
	assert.Equal(t, 1, countCall(runner.calls, "systemctl", "kill", "-s", "HUP", AgentService))
	pending, err := manager.markerExists(ActivationPendingMarkerName)
	require.NoError(t, err)
	assert.False(t, pending)
	appliedID, err := manager.readMarker(AppliedMarkerName)
	require.NoError(t, err)
	assert.Equal(t, generation.releaseID, appliedID)
}

func TestManagerKeepsActivationPendingWhenRollbackReloadFails(t *testing.T) {
	now := time.Date(2026, 7, 6, 0, 0, 0, 0, time.UTC)
	paths := testPaths(t)
	writeTestKubeconfig(t, paths.Kubeconfig, "https://kap.example.test", "")
	installTestAgent(t, paths)
	runner := &fakeRunner{state: "active"}
	manager := newTestManager(paths, runner, now)
	ready := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ready.Close()
	manager.readyURL = ready.URL
	first := testCredentials(t, "worker-a", "machine-a", 1, now.Add(-time.Minute), now.Add(5*24*time.Hour))
	require.NoError(t, manager.UpdateCredentials(context.Background(), "machine-a", first))
	firstID, err := manager.currentReleaseID()
	require.NoError(t, err)
	manager.gatewayVerifier = func(context.Context, Credentials) error {
		runner.failAgentReload = 1
		return errors.New("gateway unavailable")
	}
	second := testRenewedCredentials(t, first, "worker-a", "machine-a", 2, now.Add(-time.Minute), now.Add(5*24*time.Hour))

	err = manager.UpdateCredentials(context.Background(), "machine-a", second)
	require.ErrorContains(t, err, "reload KAP mTLS agent after rollback")
	currentID, currentErr := manager.currentReleaseID()
	require.NoError(t, currentErr)
	assert.Equal(t, firstID, currentID)
	pending, markerErr := manager.markerExists(ActivationPendingMarkerName)
	require.NoError(t, markerErr)
	assert.True(t, pending)
	status, statusErr := manager.Status(context.Background(), "machine-a")
	require.NoError(t, statusErr)
	assert.False(t, status.AgentReady)
}

func TestManagerCandidatePreflightFailureDoesNotChangeCurrent(t *testing.T) {
	now := time.Date(2026, 7, 6, 0, 0, 0, 0, time.UTC)
	paths := testPaths(t)
	writeTestKubeconfig(t, paths.Kubeconfig, "https://kap.example.test", "")
	installTestAgent(t, paths)
	manager := newTestManager(paths, &fakeRunner{state: "active"}, now)
	ready := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ready.Close()
	manager.readyURL = ready.URL
	first := testCredentials(t, "worker-a", "machine-a", 1, now.Add(-time.Minute), now.Add(5*24*time.Hour))
	require.NoError(t, manager.UpdateCredentials(context.Background(), "machine-a", first))
	firstID, err := manager.currentReleaseID()
	require.NoError(t, err)
	manager.candidateVerifier = func(context.Context, Credentials) error {
		currentID, currentErr := manager.currentReleaseID()
		require.NoError(t, currentErr)
		assert.Equal(t, firstID, currentID)
		return errors.New("post-handshake admission rejected")
	}
	second := testRenewedCredentials(t, first, "worker-a", "machine-a", 2, now.Add(-time.Minute), now.Add(5*24*time.Hour))

	err = manager.UpdateCredentials(context.Background(), "machine-a", second)
	require.ErrorContains(t, err, "post-handshake admission rejected")
	currentID, currentErr := manager.currentReleaseID()
	require.NoError(t, currentErr)
	assert.Equal(t, firstID, currentID)
	pending, markerErr := manager.markerExists(ActivationPendingMarkerName)
	require.NoError(t, markerErr)
	assert.False(t, pending)
	entries, readErr := os.ReadDir(filepath.Join(paths.StateDir, ReleasesDirectoryName))
	require.NoError(t, readErr)
	require.Len(t, entries, 1)
	assert.Equal(t, firstID, entries[0].Name())
}

func TestManagerConfirmsReloadedSerialBeforeGatewayPreflight(t *testing.T) {
	now := time.Date(2026, 7, 6, 0, 0, 0, 0, time.UTC)
	paths := testPaths(t)
	writeTestKubeconfig(t, paths.Kubeconfig, "https://kap.example.test", "")
	installTestAgent(t, paths)
	runner := &fakeRunner{state: "active"}
	manager := newTestManager(paths, runner, now)
	ready := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ready.Close()
	manager.readyURL = ready.URL

	first := testCredentials(t, "worker-a", "machine-a", 1, now.Add(-time.Minute), now.Add(5*24*time.Hour))
	require.NoError(t, manager.UpdateCredentials(context.Background(), "machine-a", first))
	second := testRenewedCredentials(t, first, "worker-a", "machine-a", 2, now.Add(-time.Minute), now.Add(5*24*time.Hour))

	var metricsObserved atomic.Bool
	metrics := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		metricsObserved.Store(true)
		_, _ = fmt.Fprintln(w, "# TYPE kaproxy_mtls_agent_cert_info gauge")
		_, _ = fmt.Fprintln(w, `kaproxy_mtls_agent_cert_info{serial="2"} 1`)
	}))
	defer metrics.Close()
	manager.metricsURL = metrics.URL
	manager.certificateSerialVerifier = nil
	manager.gatewayVerifier = func(context.Context, Credentials) error {
		if !metricsObserved.Load() {
			return errors.New("gateway preflight ran before serial confirmation")
		}
		return nil
	}
	runner.calls = nil

	require.NoError(t, manager.UpdateCredentials(context.Background(), "machine-a", second))
	assert.True(t, metricsObserved.Load())
	assert.Equal(t, 1, countCall(runner.calls, "systemctl", "kill", "-s", "HUP", AgentService))
	assert.Equal(t, 0, countCall(runner.calls, "systemctl", "restart", AgentService))
}

func TestManagerRollsBackAndCleansGenerationWhenRestartFails(t *testing.T) {
	now := time.Date(2026, 7, 6, 0, 0, 0, 0, time.UTC)
	paths := testPaths(t)
	writeTestKubeconfig(t, paths.Kubeconfig, "https://kap.example.test", "")
	installTestAgent(t, paths)
	runner := &fakeRunner{state: "active"}
	manager := newTestManager(paths, runner, now)
	ready := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ready.Close()
	manager.readyURL = ready.URL

	first := testCredentials(t, "worker-a", "machine-a", 1, now.Add(-time.Minute), now.Add(5*24*time.Hour))
	require.NoError(t, manager.UpdateCredentials(context.Background(), "machine-a", first))
	firstID, err := manager.currentReleaseID()
	require.NoError(t, err)
	runner.calls = nil
	runner.failAgentRestart = 1
	changedGateway := testCredentials(t, "worker-a", "machine-a", 2, now.Add(-time.Minute), now.Add(5*24*time.Hour))

	err = manager.UpdateCredentials(context.Background(), "machine-a", changedGateway)
	require.ErrorContains(t, err, "restart failed")
	require.ErrorContains(t, err, "restored previous applied generation")
	currentID, currentErr := manager.currentReleaseID()
	require.NoError(t, currentErr)
	assert.Equal(t, firstID, currentID)
	assert.Equal(t, 2, countCall(runner.calls, "systemctl", "restart", AgentService))
	entries, readErr := os.ReadDir(filepath.Join(paths.StateDir, ReleasesDirectoryName))
	require.NoError(t, readErr)
	require.Len(t, entries, 1)
	assert.Equal(t, firstID, entries[0].Name())
}

func TestManagerRollsBackWhenLeafReloadSignalFails(t *testing.T) {
	now := time.Date(2026, 7, 6, 0, 0, 0, 0, time.UTC)
	paths := testPaths(t)
	writeTestKubeconfig(t, paths.Kubeconfig, "https://kap.example.test", "")
	installTestAgent(t, paths)
	runner := &fakeRunner{state: "active"}
	manager := newTestManager(paths, runner, now)
	ready := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ready.Close()
	manager.readyURL = ready.URL

	first := testCredentials(t, "worker-a", "machine-a", 1, now.Add(-time.Minute), now.Add(5*24*time.Hour))
	require.NoError(t, manager.UpdateCredentials(context.Background(), "machine-a", first))
	firstID, err := manager.currentReleaseID()
	require.NoError(t, err)
	runner.calls = nil
	runner.failAgentReload = 1
	second := testRenewedCredentials(t, first, "worker-a", "machine-a", 2, now.Add(-time.Minute), now.Add(5*24*time.Hour))

	err = manager.UpdateCredentials(context.Background(), "machine-a", second)
	require.ErrorContains(t, err, "reload failed")
	require.ErrorContains(t, err, "restored previous applied generation")
	currentID, currentErr := manager.currentReleaseID()
	require.NoError(t, currentErr)
	assert.Equal(t, firstID, currentID)
	assert.Equal(t, 2, countCall(runner.calls, "systemctl", "kill", "-s", "HUP", AgentService))
}

func TestManagerCleansPendingAndOrphanedReleases(t *testing.T) {
	now := time.Date(2026, 7, 6, 0, 0, 0, 0, time.UTC)
	paths := testPaths(t)
	writeTestKubeconfig(t, paths.Kubeconfig, "https://kap.example.test", "")
	manager := newTestManager(paths, &fakeRunner{}, now)
	first := testCredentials(t, "worker-a", "machine-a", 1, now.Add(-time.Minute), now.Add(5*24*time.Hour))
	require.NoError(t, manager.UpdateCredentials(context.Background(), "machine-a", first))
	firstID, err := manager.currentReleaseID()
	require.NoError(t, err)
	releasesDir := filepath.Join(paths.StateDir, ReleasesDirectoryName)
	require.NoError(t, os.Mkdir(filepath.Join(releasesDir, ".pending-stale"), 0700))
	require.NoError(t, os.Mkdir(filepath.Join(releasesDir, strings.Repeat("f", 64)), 0700))

	second := testRenewedCredentials(t, first, "worker-a", "machine-a", 2, now.Add(-time.Minute), now.Add(5*24*time.Hour))
	require.NoError(t, manager.UpdateCredentials(context.Background(), "machine-a", second))
	secondID, err := manager.currentReleaseID()
	require.NoError(t, err)
	entries, err := os.ReadDir(releasesDir)
	require.NoError(t, err)
	require.Len(t, entries, 2)
	assert.ElementsMatch(t, []string{firstID, secondID}, []string{entries[0].Name(), entries[1].Name()})
}

func TestManagerGatewayPreflightUsesKubeconfigClientCredentials(t *testing.T) {
	now := time.Now()
	kubeletCredentials := testCredentials(t, "worker-a", "machine-a", 1, now.Add(-time.Minute), now.Add(time.Hour))
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	server.TLS = &tls.Config{ClientAuth: tls.RequireAnyClientCert}
	server.StartTLS()
	defer server.Close()
	trustedCA := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw})
	paths := testPaths(t)
	writeTestKubeconfigWithClientData(
		t,
		paths.Kubeconfig,
		"https://remote-kube-apiserver.example.test",
		"",
		testCAPEM(t, "kubeconfig-ca"),
		kubeletCredentials.CertificatePEM,
		kubeletCredentials.PrivateKeyPEM,
	)
	manager := newTestManager(paths, &fakeRunner{}, now)
	manager.gatewayVerifier = nil
	manager.localAgentAddress = server.Listener.Addr().String()
	preflightCredentials := kubeletCredentials
	preflightCredentials.ServerName = "example.com"
	preflightCredentials.GatewayCAPEM = trustedCA

	require.NoError(t, manager.verifyGateway(context.Background(), preflightCredentials))
}

func TestVerifyCandidateGatewayTLSUsesDistinctOuterAndInnerCertificates(t *testing.T) {
	now := time.Now()
	agentCredentials := testCredentials(t, "worker-a", "machine-a", 1, now.Add(-time.Minute), now.Add(time.Hour))
	kubeletCredentials := testCredentials(t, "worker-a", "machine-a", 2, now.Add(-time.Minute), now.Add(time.Hour))
	caPEM, serverCertificate := testGatewayServerCertificate(t, "example.com")
	endpoint, observations := startNestedTLSServer(t, serverCertificate, false, true)

	require.NoError(t, verifyCandidateGatewayTLS(
		context.Background(),
		endpoint,
		"example.com",
		agentCredentials.CertificatePEM,
		agentCredentials.PrivateKeyPEM,
		caPEM,
		kubeletCredentials.CertificatePEM,
		kubeletCredentials.PrivateKeyPEM,
	))
	observation := <-observations
	require.NoError(t, observation.err)
	agentBlock, _ := pem.Decode(agentCredentials.CertificatePEM)
	kubeletBlock, _ := pem.Decode(kubeletCredentials.CertificatePEM)
	require.NotNil(t, agentBlock)
	require.NotNil(t, kubeletBlock)
	assert.Equal(t, agentBlock.Bytes, observation.outerPeerCertificate)
	assert.Equal(t, kubeletBlock.Bytes, observation.innerPeerCertificate)
	assert.NotEqual(t, observation.outerPeerCertificate, observation.innerPeerCertificate)
}

func TestVerifyCandidateGatewayTLSAcceptsNoALPN(t *testing.T) {
	now := time.Now()
	agentCredentials := testCredentials(t, "worker-a", "machine-a", 1, now.Add(-time.Minute), now.Add(time.Hour))
	kubeletCredentials := testCredentials(t, "worker-a", "machine-a", 2, now.Add(-time.Minute), now.Add(time.Hour))
	caPEM, serverCertificate := testGatewayServerCertificate(t, "example.com")
	endpoint, observations := startNestedTLSServer(t, serverCertificate, false, false)

	require.NoError(t, verifyCandidateGatewayTLS(
		context.Background(),
		endpoint,
		"example.com",
		agentCredentials.CertificatePEM,
		agentCredentials.PrivateKeyPEM,
		caPEM,
		kubeletCredentials.CertificatePEM,
		kubeletCredentials.PrivateKeyPEM,
	))
	observation := <-observations
	require.NoError(t, observation.err)
	assert.NotEmpty(t, observation.outerPeerCertificate)
	assert.NotEmpty(t, observation.innerPeerCertificate)
}

func TestVerifyCandidateGatewayTLSRejectsCloseAfterOuterHandshake(t *testing.T) {
	now := time.Now()
	agentCredentials := testCredentials(t, "worker-a", "machine-a", 1, now.Add(-time.Minute), now.Add(time.Hour))
	kubeletCredentials := testCredentials(t, "worker-a", "machine-a", 2, now.Add(-time.Minute), now.Add(time.Hour))
	caPEM, serverCertificate := testGatewayServerCertificate(t, "example.com")
	endpoint, observations := startNestedTLSServer(t, serverCertificate, true, true)

	err := verifyCandidateGatewayTLS(
		context.Background(),
		endpoint,
		"example.com",
		agentCredentials.CertificatePEM,
		agentCredentials.PrivateKeyPEM,
		caPEM,
		kubeletCredentials.CertificatePEM,
		kubeletCredentials.PrivateKeyPEM,
	)
	require.ErrorContains(t, err, "kubelet inner TLS handshake")
	observation := <-observations
	require.NoError(t, observation.err)
	assert.NotEmpty(t, observation.outerPeerCertificate)
	assert.Empty(t, observation.innerPeerCertificate)
}

func TestManagerDoesNotCommitWhenCandidateGatewayClosesAfterOuterHandshake(t *testing.T) {
	now := time.Now()
	paths := testPaths(t)
	writeTestKubeconfig(t, paths.Kubeconfig, "https://kap.example.test", "")
	installTestAgent(t, paths)
	manager := newTestManager(paths, &fakeRunner{state: "active"}, now)
	ready := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ready.Close()
	manager.readyURL = ready.URL
	first := testCredentials(t, "worker-a", "machine-a", 1, now.Add(-time.Minute), now.Add(time.Hour))
	require.NoError(t, manager.UpdateCredentials(context.Background(), "machine-a", first))
	firstID, err := manager.currentReleaseID()
	require.NoError(t, err)
	kubeletCredentials := testCredentials(t, "worker-a", "machine-a", 2, now.Add(-time.Minute), now.Add(time.Hour))
	caPEM, serverCertificate := testGatewayServerCertificate(t, "example.com")
	endpoint, observations := startNestedTLSServer(t, serverCertificate, true, true)
	writeTestKubeconfigWithClientData(
		t,
		paths.Kubeconfig,
		"https://kap.example.test",
		"",
		caPEM,
		kubeletCredentials.CertificatePEM,
		kubeletCredentials.PrivateKeyPEM,
	)
	candidate := testCredentials(t, "worker-a", "machine-a", 3, now.Add(-time.Minute), now.Add(time.Hour))
	candidate.GatewayCAPEM = caPEM
	candidate.GatewayEndpoint = "example.com:8443"
	candidate.ServerName = "example.com"
	candidate.GatewayCAFingerprint = fingerprintForPEM(t, caPEM)
	manager.candidateVerifier = nil
	manager.candidateGatewayAddress = endpoint

	err = manager.UpdateCredentials(context.Background(), "machine-a", candidate)
	require.ErrorContains(t, err, "kubelet inner TLS handshake")
	observation := <-observations
	require.NoError(t, observation.err)
	assert.NotEmpty(t, observation.outerPeerCertificate)
	assert.Empty(t, observation.innerPeerCertificate)
	currentID, currentErr := manager.currentReleaseID()
	require.NoError(t, currentErr)
	assert.Equal(t, firstID, currentID)
	appliedID, markerErr := manager.readMarker(AppliedMarkerName)
	require.NoError(t, markerErr)
	assert.Equal(t, firstID, appliedID)
	pending, markerErr := manager.markerExists(ActivationPendingMarkerName)
	require.NoError(t, markerErr)
	assert.False(t, pending)
	entries, readErr := os.ReadDir(filepath.Join(paths.StateDir, ReleasesDirectoryName))
	require.NoError(t, readErr)
	require.Len(t, entries, 1)
	assert.Equal(t, firstID, entries[0].Name())
}

func TestVerifyLocalAgentTLS(t *testing.T) {
	now := time.Now()
	credentials := testCredentials(t, "worker-a", "machine-a", 1, now.Add(-time.Minute), now.Add(time.Hour))
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	server.TLS = &tls.Config{ClientAuth: tls.RequireAnyClientCert}
	server.StartTLS()
	endpoint := server.Listener.Addr().String()
	serverName := "example.com"
	trustedCA := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw})

	require.NoError(t, verifyLocalAgentTLS(context.Background(), endpoint, serverName, credentials.CertificatePEM, credentials.PrivateKeyPEM, trustedCA))
	require.Error(t, verifyLocalAgentTLS(context.Background(), endpoint, serverName, credentials.CertificatePEM, credentials.PrivateKeyPEM, credentials.GatewayCAPEM))
	server.Close()
	require.Error(t, verifyLocalAgentTLS(context.Background(), endpoint, serverName, credentials.CertificatePEM, credentials.PrivateKeyPEM, trustedCA))
}

func TestManagerReportsVersionAndTransientSystemdStates(t *testing.T) {
	now := time.Date(2026, 7, 6, 0, 0, 0, 0, time.UTC)
	paths := testPaths(t)
	writeTestKubeconfig(t, paths.Kubeconfig, "https://kap.example.test", "")
	installTestAgent(t, paths)
	require.NoError(t, os.MkdirAll(paths.StateDir, 0700))
	require.NoError(t, os.WriteFile(paths.AgentVersionFile, []byte("v0.3.7\n"), 0644))
	runner := &fakeRunner{state: "activating"}
	manager := newTestManager(paths, runner, now)
	status, err := manager.Status(context.Background(), "machine-a")
	require.NoError(t, err)
	assert.False(t, status.AgentActive)
	assert.False(t, status.AgentReady)
	assert.Equal(t, "v0.3.7", status.AgentVersion)
	for _, state := range []string{"activating", "deactivating", "failed", "inactive"} {
		runner.state = state
		active, err := manager.serviceActive(context.Background(), AgentService)
		require.NoError(t, err)
		assert.False(t, active)
	}
}

type nestedTLSObservation struct {
	outerPeerCertificate []byte
	innerPeerCertificate []byte
	err                  error
}

func startNestedTLSServer(
	t *testing.T,
	serverCertificate tls.Certificate,
	closeAfterOuter, advertiseALPN bool,
) (string, <-chan nestedTLSObservation) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = listener.Close() })
	nextProtos := []string(nil)
	if advertiseALPN {
		nextProtos = []string{gatewayRawTCPALPN}
	}
	tlsListener := tls.NewListener(listener, &tls.Config{
		MinVersion:   tls.VersionTLS12,
		Certificates: []tls.Certificate{serverCertificate},
		ClientAuth:   tls.RequireAnyClientCert,
		NextProtos:   nextProtos,
	})
	observations := make(chan nestedTLSObservation, 1)
	go func() {
		connection, err := tlsListener.Accept()
		if err != nil {
			observations <- nestedTLSObservation{err: err}
			return
		}
		outerConnection := connection.(*tls.Conn)
		defer func() { _ = outerConnection.Close() }()
		if err := outerConnection.Handshake(); err != nil {
			observations <- nestedTLSObservation{err: err}
			return
		}
		observation := nestedTLSObservation{}
		if peers := outerConnection.ConnectionState().PeerCertificates; len(peers) > 0 {
			observation.outerPeerCertificate = append([]byte(nil), peers[0].Raw...)
		}
		if closeAfterOuter {
			observations <- observation
			return
		}
		innerConnection := tls.Server(outerConnection, &tls.Config{
			MinVersion:   tls.VersionTLS12,
			Certificates: []tls.Certificate{serverCertificate},
			ClientAuth:   tls.RequireAnyClientCert,
		})
		if err := innerConnection.Handshake(); err != nil {
			observation.err = err
			observations <- observation
			return
		}
		if peers := innerConnection.ConnectionState().PeerCertificates; len(peers) > 0 {
			observation.innerPeerCertificate = append([]byte(nil), peers[0].Raw...)
		}
		_ = innerConnection.Close()
		observations <- observation
	}()
	return listener.Addr().String(), observations
}

func testGatewayServerCertificate(t *testing.T, serverName string) ([]byte, tls.Certificate) {
	t.Helper()
	now := time.Now()
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	caTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(now.UnixNano()),
		Subject:               pkix.Name{CommonName: "gateway-test-ca"},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(24 * time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	require.NoError(t, err)
	caCertificate, err := x509.ParseCertificate(caDER)
	require.NoError(t, err)
	serverKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	serverTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(now.UnixNano() + 1),
		Subject:      pkix.Name{CommonName: serverName},
		DNSNames:     []string{serverName},
		NotBefore:    now.Add(-time.Hour),
		NotAfter:     now.Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	serverDER, err := x509.CreateCertificate(rand.Reader, serverTemplate, caCertificate, &serverKey.PublicKey, caKey)
	require.NoError(t, err)
	serverKeyDER, err := x509.MarshalPKCS8PrivateKey(serverKey)
	require.NoError(t, err)
	serverCertificate, err := tls.X509KeyPair(
		pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: serverDER}),
		pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: serverKeyDER}),
	)
	require.NoError(t, err)
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER}), serverCertificate
}

func testPaths(t *testing.T) Paths {
	t.Helper()
	directory := t.TempDir()
	stateDir := filepath.Join(directory, "state")
	return Paths{
		StateDir:         stateDir,
		Kubeconfig:       filepath.Join(directory, "kubeconfig"),
		AgentBinary:      filepath.Join(directory, "kaproxy-mtls-agent"),
		AgentUnitFile:    filepath.Join(directory, "kaproxy-mtls-agent.service"),
		AgentVersionFile: filepath.Join(stateDir, "version"),
	}
}

func newTestManager(paths Paths, runner *fakeRunner, now time.Time) *Manager {
	return &Manager{
		paths:  paths,
		runner: runner,
		certificateSerialVerifier: func(ctx context.Context, _ string) bool {
			return ctx.Err() == nil
		},
		candidateVerifier: func(context.Context, Credentials) error { return nil },
		gatewayVerifier:   func(context.Context, Credentials) error { return nil },
		now:               func() time.Time { return now },
	}
}

func installTestAgent(t *testing.T, paths Paths) {
	t.Helper()
	require.NoError(t, os.WriteFile(paths.AgentBinary, []byte("binary"), 0755))
	require.NoError(t, os.WriteFile(paths.AgentUnitFile, []byte("unit"), 0644))
}

func writeTestKubeconfig(t *testing.T, path, server, tlsServerName string) []byte {
	t.Helper()
	caPEM := testCAPEM(t, "kubeconfig-ca")
	tlsLine := ""
	if tlsServerName != "" {
		tlsLine = fmt.Sprintf("    tls-server-name: %s\n", tlsServerName)
	}
	content := fmt.Sprintf(`apiVersion: v1
kind: Config
clusters:
- name: kubernetes
  cluster:
    certificate-authority-data: %s
    server: %s
%scontexts:
- name: kubelet-context
  context:
    cluster: kubernetes
    user: kubelet
current-context: kubelet-context
users:
- name: kubelet
  user:
    client-certificate: preserved-client-cert
    client-key: preserved-client-key
`, base64.StdEncoding.EncodeToString(caPEM), server, tlsLine)
	require.NoError(t, os.WriteFile(path, []byte(content), 0600))
	return caPEM
}

func writeTestKubeconfigWithClientData(
	t *testing.T,
	path, server, tlsServerName string,
	caPEM, clientCertificatePEM, clientPrivateKeyPEM []byte,
) {
	t.Helper()
	tlsLine := ""
	if tlsServerName != "" {
		tlsLine = fmt.Sprintf("    tls-server-name: %s\n", tlsServerName)
	}
	content := fmt.Sprintf(`apiVersion: v1
kind: Config
clusters:
- name: kubernetes
  cluster:
    certificate-authority-data: %s
    server: %s
%scontexts:
- name: kubelet-context
  context:
    cluster: kubernetes
    user: kubelet
current-context: kubelet-context
users:
- name: kubelet
  user:
    client-certificate-data: %s
    client-key-data: %s
`,
		base64.StdEncoding.EncodeToString(caPEM),
		server,
		tlsLine,
		base64.StdEncoding.EncodeToString(clientCertificatePEM),
		base64.StdEncoding.EncodeToString(clientPrivateKeyPEM),
	)
	require.NoError(t, os.WriteFile(path, []byte(content), 0600))
}

func testCAPEM(t *testing.T, commonName string) []byte {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	now := time.Now()
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(now.UnixNano()),
		Subject:               pkix.Name{CommonName: commonName},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(24 * time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	require.NoError(t, err)
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}

func fingerprintForPEM(t *testing.T, data []byte) string {
	t.Helper()
	certificates, err := parseCertificateBundle(data)
	require.NoError(t, err)
	return certificateBundleFingerprint(certificates)
}

func testCredentials(t *testing.T, workerCluster, machineID string, serial int64, notBefore, notAfter time.Time) Credentials {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	spiffeURI, err := url.Parse(fmt.Sprintf("spiffe://lepton/workercluster/%s/machine/%s", workerCluster, machineID))
	require.NoError(t, err)
	template := &x509.Certificate{
		SerialNumber: big.NewInt(serial),
		Subject:      pkix.Name{CommonName: "workercluster:" + workerCluster, Organization: []string{clientOrganization}},
		NotBefore:    notBefore, NotAfter: notAfter,
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		URIs: []*url.URL{spiffeURI},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	require.NoError(t, err)
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	require.NoError(t, err)

	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	caTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(serial + 1000),
		Subject:      pkix.Name{CommonName: "gateway-ca"},
		NotBefore:    notBefore, NotAfter: notAfter,
		IsCA: true, BasicConstraintsValid: true,
		KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	require.NoError(t, err)
	ca, err := x509.ParseCertificate(caDER)
	require.NoError(t, err)
	gatewayCAPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER})
	return Credentials{
		CertificatePEM:       pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		PrivateKeyPEM:        pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}),
		GatewayCAPEM:         gatewayCAPEM,
		GatewayEndpoint:      "kap.example.test:8443",
		ServerName:           "kap.example.test",
		ClientCAFingerprint:  strings.Repeat("a", 64),
		GatewayCAFingerprint: certificateBundleFingerprint([]*x509.Certificate{ca}),
	}
}

func testRenewedCredentials(
	t *testing.T,
	previous Credentials,
	workerCluster, machineID string,
	serial int64,
	notBefore, notAfter time.Time,
) Credentials {
	t.Helper()
	renewed := testCredentials(t, workerCluster, machineID, serial, notBefore, notAfter)
	renewed.GatewayCAPEM = append([]byte(nil), previous.GatewayCAPEM...)
	renewed.GatewayEndpoint = previous.GatewayEndpoint
	renewed.ServerName = previous.ServerName
	renewed.GatewayCAFingerprint = previous.GatewayCAFingerprint
	return renewed
}

func TestValidateCredentialsRejectsMalformedInputs(t *testing.T) {
	now := time.Date(2026, 7, 8, 0, 0, 0, 0, time.UTC)
	valid := testCredentials(t, "worker-a", "machine-a", 1, now.Add(-time.Minute), now.Add(time.Hour))
	tests := []struct {
		name   string
		mutate func(*Credentials)
		want   string
	}{
		{name: "missing pair", mutate: func(c *Credentials) { c.CertificatePEM = nil }, want: "certificate and private key are required"},
		{name: "bad endpoint", mutate: func(c *Credentials) { c.GatewayEndpoint = "kap.example.test" }, want: "must be a host on port"},
		{name: "bad host", mutate: func(c *Credentials) { c.GatewayEndpoint = "kap@example.test:8443" }, want: "invalid host"},
		{name: "server mismatch", mutate: func(c *Credentials) { c.ServerName = "other.example.test" }, want: "does not match gateway host"},
		{name: "bad pair", mutate: func(c *Credentials) { c.PrivateKeyPEM = []byte("bad") }, want: "parse KAP mTLS certificate and private key"},
		{name: "bad client fingerprint case", mutate: func(c *Credentials) { c.ClientCAFingerprint = strings.Repeat("A", 64) }, want: "lowercase hexadecimal"},
		{name: "bad client fingerprint hex", mutate: func(c *Credentials) { c.ClientCAFingerprint = strings.Repeat("g", 64) }, want: "lowercase hexadecimal"},
		{name: "empty gateway bundle", mutate: func(c *Credentials) { c.GatewayCAPEM = nil }, want: "certificate bundle is empty"},
		{name: "bad gateway PEM", mutate: func(c *Credentials) { c.GatewayCAPEM = []byte("bad") }, want: "invalid PEM data"},
		{name: "unexpected gateway PEM", mutate: func(c *Credentials) {
			c.GatewayCAPEM = pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: []byte("bad")})
		}, want: "unexpected PEM block"},
		{name: "non CA gateway", mutate: func(c *Credentials) { c.GatewayCAPEM = c.CertificatePEM }, want: "is not a CA"},
		{name: "bad gateway fingerprint", mutate: func(c *Credentials) { c.GatewayCAFingerprint = "bad" }, want: "lowercase hexadecimal"},
		{name: "gateway fingerprint mismatch", mutate: func(c *Credentials) { c.GatewayCAFingerprint = strings.Repeat("b", 64) }, want: "does not match gateway CA PEM"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			credentials := valid
			tt.mutate(&credentials)
			_, err := validateCredentials("machine-a", credentials, now, true)
			require.ErrorContains(t, err, tt.want)
		})
	}

	leafTests := []struct {
		name   string
		mutate func(*x509.Certificate)
		want   string
	}{
		{name: "not yet valid", mutate: func(c *x509.Certificate) { c.NotBefore = now.Add(time.Minute) }, want: "not currently valid"},
		{name: "expired", mutate: func(c *x509.Certificate) { c.NotAfter = now.Add(-time.Minute) }, want: "not currently valid"},
		{name: "no client auth", mutate: func(c *x509.Certificate) { c.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth} }, want: "not valid for client authentication"},
		{name: "bad organization", mutate: func(c *x509.Certificate) { c.Subject.Organization = []string{"other"} }, want: "invalid organization"},
		{name: "no URI", mutate: func(c *x509.Certificate) { c.URIs = nil }, want: "exactly one SPIFFE URI"},
		{name: "multiple URIs", mutate: func(c *x509.Certificate) { c.URIs = append(c.URIs, c.URIs[0]) }, want: "exactly one SPIFFE URI"},
		{name: "bad URI", mutate: func(c *x509.Certificate) {
			c.URIs[0], _ = url.Parse("https://lepton/workercluster/worker-a/machine/machine-a")
		}, want: "invalid SPIFFE identity"},
		{name: "wrong machine", mutate: func(c *x509.Certificate) {
			c.URIs[0], _ = url.Parse("spiffe://lepton/workercluster/worker-a/machine/machine-b")
		}, want: "invalid SPIFFE identity"},
		{name: "bad common name", mutate: func(c *x509.Certificate) { c.Subject.CommonName = "workercluster:other" }, want: "common name does not match"},
	}
	for _, tt := range leafTests {
		t.Run(tt.name, func(t *testing.T) {
			credentials := testCredentialsWithLeaf(t, now, tt.mutate)
			_, err := validateCredentials("machine-a", credentials, now, true)
			require.ErrorContains(t, err, tt.want)
		})
	}
}

func TestKubeconfigAndEnvironmentHelpers(t *testing.T) {
	directory := t.TempDir()
	kubeconfig := filepath.Join(directory, "kubeconfig")
	caPEM := testCAPEM(t, "kubeconfig-ca")

	for _, tt := range []struct {
		name string
		data string
		want string
	}{
		{name: "bad yaml", data: "clusters: [", want: "parse kubelet kubeconfig"},
		{name: "no clusters", data: "kind: Config\n", want: "has no clusters"},
		{name: "invalid named cluster", data: "clusters:\n- name: kubernetes\n  cluster: invalid\n", want: "cluster \"kubernetes\" is invalid"},
		{name: "missing named cluster", data: "clusters:\n- name: one\n  cluster: {}\n- name: two\n  cluster: {}\n", want: "cluster \"kubernetes\" was not found"},
	} {
		t.Run("parse "+tt.name, func(t *testing.T) {
			_, _, err := parseKubeconfig([]byte(tt.data))
			require.ErrorContains(t, err, tt.want)
		})
	}

	document, cluster, err := parseKubeconfig([]byte("clusters:\n- name: custom\n  cluster:\n    server: https://kap.example.test\n"))
	require.NoError(t, err)
	require.NotNil(t, document)
	assert.Equal(t, "https://kap.example.test", cluster["server"])

	_, err = decodeKubeconfigData(map[string]any{}, "certificate-authority-data")
	require.ErrorContains(t, err, "has no certificate-authority-data")
	_, err = decodeKubeconfigData(map[string]any{"certificate-authority-data": "%%%"}, "certificate-authority-data")
	require.ErrorContains(t, err, "decode kubelet kubeconfig")
	decoded, err := decodeKubeconfigData(map[string]any{"certificate-authority-data": base64.StdEncoding.EncodeToString(caPEM)}, "certificate-authority-data")
	require.NoError(t, err)
	assert.Equal(t, caPEM, decoded)

	_, _, err = readKubeconfigClientCredentials(filepath.Join(directory, "missing"))
	require.ErrorContains(t, err, "read kubelet kubeconfig")
	require.NoError(t, os.WriteFile(kubeconfig, []byte("users: ["), 0600))
	_, _, err = readKubeconfigClientCredentials(kubeconfig)
	require.ErrorContains(t, err, "parse kubelet kubeconfig")
	require.NoError(t, os.WriteFile(kubeconfig, []byte("kind: Config\n"), 0600))
	_, _, err = readKubeconfigClientCredentials(kubeconfig)
	require.ErrorContains(t, err, "has no users")
	require.NoError(t, os.WriteFile(kubeconfig, []byte("users:\n- name: one\n  user: {}\n- name: two\n  user: {}\n"), 0600))
	_, _, err = readKubeconfigClientCredentials(kubeconfig)
	require.ErrorContains(t, err, "user \"kubelet\" was not found")

	authInfo := map[string]any{"client-certificate-data": "%%%"}
	_, err = readKubeconfigCredentialData(kubeconfig, authInfo, "client-certificate-data", "client-certificate")
	require.ErrorContains(t, err, "decode kubelet kubeconfig")
	_, err = readKubeconfigCredentialData(kubeconfig, map[string]any{}, "client-certificate-data", "client-certificate")
	require.ErrorContains(t, err, "has no client-certificate-data or client-certificate")
	credentialPath := filepath.Join(directory, "client.crt")
	require.NoError(t, os.WriteFile(credentialPath, []byte("certificate"), 0600))
	credential, err := readKubeconfigCredentialData(kubeconfig, map[string]any{"client-certificate": "client.crt"}, "client-certificate-data", "client-certificate")
	require.NoError(t, err)
	assert.Equal(t, []byte("certificate"), credential)
	_, err = readKubeconfigCredentialData(kubeconfig, map[string]any{"client-certificate": "missing.crt"}, "client-certificate-data", "client-certificate")
	require.ErrorContains(t, err, "read kubelet kubeconfig client-certificate")

	environmentPath := filepath.Join(directory, "agent.env")
	_, err = readAgentEnvironment(filepath.Join(directory, "missing.env"))
	require.ErrorContains(t, err, "read KAP mTLS agent environment")
	require.NoError(t, os.WriteFile(environmentPath, []byte("invalid\n"), 0600))
	_, err = readAgentEnvironment(environmentPath)
	require.ErrorContains(t, err, "invalid KAP mTLS agent environment line")
	require.NoError(t, os.WriteFile(environmentPath, []byte("KAP_MTLS_GATEWAY_ENDPOINT=kap.example.test:8443\n"), 0600))
	_, err = readAgentEnvironment(environmentPath)
	require.ErrorContains(t, err, "environment is incomplete")
	environment := agentEnvironment{
		gatewayEndpoint:      "kap.example.test:8443",
		serverName:           "kap.example.test",
		clientCAFingerprint:  strings.Repeat("a", 64),
		gatewayCAFingerprint: strings.Repeat("b", 64),
	}
	require.NoError(t, os.WriteFile(environmentPath, append([]byte("# generated\n\n"), marshalAgentEnvironment(environment)...), 0600))
	gotEnvironment, err := readAgentEnvironment(environmentPath)
	require.NoError(t, err)
	assert.Equal(t, environment, gotEnvironment)
}

func TestDurableStateHelpers(t *testing.T) {
	t.Run("release matching", func(t *testing.T) {
		directory := t.TempDir()
		credentials := Credentials{CertificatePEM: []byte("cert"), PrivateKeyPEM: []byte("key"), GatewayCAPEM: []byte("ca")}
		environment := []byte("env")
		exists, err := releaseMatches(filepath.Join(directory, "missing"), credentials, environment)
		require.NoError(t, err)
		assert.False(t, exists)
		filePath := filepath.Join(directory, "file")
		require.NoError(t, os.WriteFile(filePath, nil, 0600))
		_, err = releaseMatches(filePath, credentials, environment)
		require.ErrorContains(t, err, "is not a directory")
		releaseDir := filepath.Join(directory, "release")
		require.NoError(t, os.Mkdir(releaseDir, 0700))
		_, err = releaseMatches(releaseDir, credentials, environment)
		require.ErrorContains(t, err, "read existing KAP mTLS release file")
		for name, data := range map[string][]byte{
			ClientCertificateFileName: credentials.CertificatePEM,
			ClientPrivateKeyFileName:  credentials.PrivateKeyPEM,
			GatewayCAFileName:         credentials.GatewayCAPEM,
			AgentEnvironmentFileName:  environment,
		} {
			require.NoError(t, os.WriteFile(filepath.Join(releaseDir, name), data, 0600))
		}
		exists, err = releaseMatches(releaseDir, credentials, environment)
		require.NoError(t, err)
		assert.True(t, exists)
		require.NoError(t, os.WriteFile(filepath.Join(releaseDir, ClientCertificateFileName), []byte("other"), 0600))
		_, err = releaseMatches(releaseDir, credentials, environment)
		require.ErrorContains(t, err, "does not match its generation ID")
	})

	t.Run("symlink and markers", func(t *testing.T) {
		paths := testPaths(t)
		manager := &Manager{paths: paths}
		require.NoError(t, os.MkdirAll(filepath.Join(paths.StateDir, ReleasesDirectoryName), 0700))
		releaseID := strings.Repeat("a", 64)
		require.NoError(t, os.Mkdir(filepath.Join(paths.StateDir, ReleasesDirectoryName, releaseID), 0700))
		require.Error(t, manager.swapCurrentSymlink("bad"))
		require.NoError(t, manager.swapCurrentSymlink(releaseID))
		require.NoError(t, manager.swapCurrentSymlink(releaseID))
		current, err := manager.currentReleaseID()
		require.NoError(t, err)
		assert.Equal(t, releaseID, current)
		require.NoError(t, os.Remove(filepath.Join(paths.StateDir, CurrentSymlinkName)))
		require.NoError(t, os.WriteFile(filepath.Join(paths.StateDir, CurrentSymlinkName), nil, 0600))
		require.ErrorContains(t, manager.swapCurrentSymlink(releaseID), "not a symlink")
		require.NoError(t, os.Remove(filepath.Join(paths.StateDir, CurrentSymlinkName)))
		require.NoError(t, os.Symlink("invalid-target", filepath.Join(paths.StateDir, CurrentSymlinkName)))
		_, err = manager.currentReleaseID()
		require.Error(t, err)

		exists, err := manager.markerExists("marker")
		require.NoError(t, err)
		assert.False(t, exists)
		value, err := manager.readMarker("marker")
		require.NoError(t, err)
		assert.Empty(t, value)
		require.NoError(t, manager.writeMarker("marker", "value"))
		exists, err = manager.markerExists("marker")
		require.NoError(t, err)
		assert.True(t, exists)
		value, err = manager.readMarker("marker")
		require.NoError(t, err)
		assert.Equal(t, "value", value)
		require.NoError(t, manager.clearMarker("marker"))
		require.NoError(t, manager.clearMarker("marker"))
	})

	t.Run("generation cleanup", func(t *testing.T) {
		paths := testPaths(t)
		manager := &Manager{paths: paths}
		releasesDir := filepath.Join(paths.StateDir, ReleasesDirectoryName)
		require.NoError(t, os.MkdirAll(releasesDir, 0700))
		currentID := strings.Repeat("a", 64)
		previousID := strings.Repeat("b", 64)
		orphanID := strings.Repeat("c", 64)
		for _, id := range []string{currentID, previousID, orphanID} {
			require.NoError(t, os.Mkdir(filepath.Join(releasesDir, id), 0700))
		}
		require.NoError(t, manager.garbageCollectReleases(currentID, previousID))
		assert.DirExists(t, filepath.Join(releasesDir, currentID))
		assert.DirExists(t, filepath.Join(releasesDir, previousID))
		assert.NoDirExists(t, filepath.Join(releasesDir, orphanID))
		require.NoError(t, manager.removeInactiveRelease(""))
		require.NoError(t, manager.writeMarker(AppliedMarkerName, previousID))
		require.NoError(t, manager.removeInactiveRelease(previousID))
		require.NoError(t, os.MkdirAll(filepath.Join(releasesDir, orphanID), 0700))
		require.NoError(t, manager.removeInactiveRelease(orphanID))
		assert.NoDirExists(t, filepath.Join(releasesDir, orphanID))
	})
}

func TestManagerValidationAndProbeErrors(t *testing.T) {
	caPEM := testCAPEM(t, "kubeconfig-ca")
	for _, tt := range []struct {
		name   string
		config Config
		want   string
	}{
		{name: "invalid URL", config: Config{Server: "://", CertificateAuthorityData: caPEM}, want: "invalid kubeconfig server"},
		{name: "invalid CA", config: Config{Server: "https://kap.example.test", CertificateAuthorityData: []byte("bad")}, want: "invalid kubeconfig certificate authority data"},
		{name: "enabled remote", config: Config{Enabled: true, Server: "https://kap.example.test", CertificateAuthorityData: caPEM, TLSServerName: "kap.example.test"}, want: "enabled KAP mTLS server"},
		{name: "enabled without server name", config: Config{Enabled: true, Server: LocalEndpoint, CertificateAuthorityData: caPEM}, want: "requires a TLS server name"},
		{name: "disabled loopback", config: Config{Server: LocalEndpoint, CertificateAuthorityData: caPEM}, want: "requires a remote server"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			require.ErrorContains(t, validateConfig(tt.config), tt.want)
		})
	}
	require.NoError(t, validateConfig(Config{Server: "https://kap.example.test", CertificateAuthorityData: caPEM}))
	require.NoError(t, validateConfig(Config{Enabled: true, Server: LocalEndpoint, TLSServerName: "kap.example.test", CertificateAuthorityData: caPEM}))

	manager := &Manager{httpClient: http.DefaultClient}
	manager.readyURL = "://"
	assert.False(t, manager.probeAgentReady(context.Background()))
	manager.metricsURL = "://"
	assert.False(t, manager.probeAgentCertificateSerial(context.Background(), "1"))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/ready" {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte("not prometheus"))
	}))
	defer server.Close()
	manager.readyURL = server.URL + "/ready"
	assert.False(t, manager.probeAgentReady(context.Background()))
	manager.metricsURL = server.URL + "/metrics"
	assert.False(t, manager.probeAgentCertificateSerial(context.Background(), "1"))
}

func TestManagerConstructionAndStatusErrors(t *testing.T) {
	directory := t.TempDir()
	paths := DefaultPaths(directory)
	assert.Equal(t, filepath.Join(directory, "kap-mtls"), paths.StateDir)
	assert.Equal(t, DefaultKubeconfigPath, paths.Kubeconfig)
	assert.Equal(t, DefaultAgentBinaryPath, paths.AgentBinary)
	assert.Equal(t, DefaultAgentUnitPath, paths.AgentUnitFile)
	assert.Equal(t, filepath.Join(directory, "kap-mtls", "version"), paths.AgentVersionFile)
	manager := NewManager(paths)
	require.NotNil(t, manager.runner)
	require.NotNil(t, manager.httpClient)
	require.NotNil(t, manager.now)
	output, err := (execRunner{}).Run(context.Background(), "/bin/sh", "-c", "printf ok")
	require.NoError(t, err)
	assert.Equal(t, "ok", string(output))

	t.Run("bad kubeconfig", func(t *testing.T) {
		paths := testPaths(t)
		require.NoError(t, os.WriteFile(paths.Kubeconfig, []byte("clusters: ["), 0600))
		_, err := (&Manager{paths: paths, runner: &fakeRunner{}}).Status(context.Background(), "machine-a")
		require.ErrorContains(t, err, "parse kubelet kubeconfig")
	})
	t.Run("pending marker stat error", func(t *testing.T) {
		paths := testPaths(t)
		writeTestKubeconfig(t, paths.Kubeconfig, "https://kap.example.test", "")
		require.NoError(t, os.WriteFile(paths.StateDir, []byte("not a directory"), 0600))
		_, err := (&Manager{paths: paths, runner: &fakeRunner{}}).Status(context.Background(), "machine-a")
		require.ErrorContains(t, err, "inspect KAP mTLS kubeconfig-pending marker")
	})
	t.Run("disabled marker stat error", func(t *testing.T) {
		paths := testPaths(t)
		writeTestKubeconfig(t, paths.Kubeconfig, "https://kap.example.test", "")
		require.NoError(t, os.MkdirAll(paths.StateDir, 0700))
		require.NoError(t, os.Symlink("disabled", filepath.Join(paths.StateDir, DisabledMarkerName)))
		_, err := (&Manager{paths: paths, runner: &fakeRunner{}}).Status(context.Background(), "machine-a")
		require.ErrorContains(t, err, "inspect KAP mTLS disabled marker")
	})
	t.Run("systemd error", func(t *testing.T) {
		paths := testPaths(t)
		writeTestKubeconfig(t, paths.Kubeconfig, "https://kap.example.test", "")
		installTestAgent(t, paths)
		_, err := (&Manager{paths: paths, runner: &fakeRunner{state: "unexpected"}}).Status(context.Background(), "machine-a")
		require.ErrorContains(t, err, "systemctl is-active")
	})
	t.Run("version read error", func(t *testing.T) {
		paths := testPaths(t)
		writeTestKubeconfig(t, paths.Kubeconfig, "https://kap.example.test", "")
		require.NoError(t, os.MkdirAll(paths.AgentVersionFile, 0700))
		_, err := (&Manager{paths: paths, runner: &fakeRunner{}}).Status(context.Background(), "machine-a")
		require.ErrorContains(t, err, "read KAP mTLS agent version")
	})
}

func TestManagerConfigureRejectsUnsafeTransitions(t *testing.T) {
	now := time.Date(2026, 7, 8, 0, 0, 0, 0, time.UTC)
	t.Run("credentials absent", func(t *testing.T) {
		paths := testPaths(t)
		caPEM := writeTestKubeconfig(t, paths.Kubeconfig, "https://kap.example.test", "")
		manager := newTestManager(paths, &fakeRunner{state: "active"}, now)
		err := manager.Configure(context.Background(), Config{Enabled: true, Server: LocalEndpoint, TLSServerName: "kap.example.test", CertificateAuthorityData: caPEM})
		require.ErrorContains(t, err, "credentials are not installed")
	})

	setup := func(t *testing.T) (*Manager, Paths, *fakeRunner, Credentials, Config) {
		t.Helper()
		paths := testPaths(t)
		credentials := testCredentials(t, "worker-a", "machine-a", 1, now.Add(-time.Minute), now.Add(time.Hour))
		writeTestKubeconfigWithClientData(t, paths.Kubeconfig, "https://kap.example.test", "", credentials.GatewayCAPEM, credentials.CertificatePEM, credentials.PrivateKeyPEM)
		runner := &fakeRunner{state: "active"}
		manager := newTestManager(paths, runner, now)
		generation, err := manager.stageCredentials("machine-a", credentials)
		require.NoError(t, err)
		require.NoError(t, manager.swapCurrentSymlink(generation.releaseID))
		config := Config{Enabled: true, Server: LocalEndpoint, TLSServerName: credentials.ServerName, CertificateAuthorityData: credentials.GatewayCAPEM}
		return manager, paths, runner, credentials, config
	}

	t.Run("expired credentials", func(t *testing.T) {
		manager, _, _, _, config := setup(t)
		manager.now = func() time.Time { return now.Add(2 * time.Hour) }
		require.ErrorContains(t, manager.Configure(context.Background(), config), "credentials are expired")
	})
	t.Run("server name mismatch", func(t *testing.T) {
		manager, _, _, _, config := setup(t)
		config.TLSServerName = "other.example.test"
		require.ErrorContains(t, manager.Configure(context.Background(), config), "does not match installed credentials")
	})
	t.Run("CA fingerprint mismatch", func(t *testing.T) {
		manager, _, _, _, config := setup(t)
		config.CertificateAuthorityData = testCAPEM(t, "other-ca")
		require.ErrorContains(t, manager.Configure(context.Background(), config), "does not match installed gateway CA fingerprint")
	})
	t.Run("agent absent", func(t *testing.T) {
		manager, _, _, _, config := setup(t)
		require.ErrorContains(t, manager.Configure(context.Background(), config), "agent is not installed")
	})
	t.Run("agent inactive", func(t *testing.T) {
		manager, paths, runner, _, config := setup(t)
		installTestAgent(t, paths)
		runner.state = "inactive"
		require.ErrorContains(t, manager.Configure(context.Background(), config), "agent is not ready")
	})
	t.Run("agent readiness incomplete", func(t *testing.T) {
		manager, paths, _, _, config := setup(t)
		installTestAgent(t, paths)
		require.ErrorContains(t, manager.Configure(context.Background(), config), "agent is not ready")
	})
	t.Run("missing kubeconfig", func(t *testing.T) {
		paths := testPaths(t)
		manager := newTestManager(paths, &fakeRunner{}, now)
		err := manager.Configure(context.Background(), Config{Server: "https://kap.example.test", CertificateAuthorityData: testCAPEM(t, "ca")})
		require.ErrorContains(t, err, "read kubelet kubeconfig")
	})
}

func TestCredentialAndReleaseInspectionErrors(t *testing.T) {
	now := time.Date(2026, 7, 8, 0, 0, 0, 0, time.UTC)
	releaseID := strings.Repeat("a", 64)
	t.Run("current is not symlink", func(t *testing.T) {
		paths := testPaths(t)
		require.NoError(t, os.MkdirAll(paths.StateDir, 0700))
		require.NoError(t, os.WriteFile(filepath.Join(paths.StateDir, CurrentSymlinkName), nil, 0600))
		_, err := (&Manager{paths: paths}).inspectCredentials("machine-a")
		require.ErrorContains(t, err, "read KAP mTLS current symlink")
	})
	for _, tt := range []struct {
		name  string
		files map[string][]byte
		want  string
	}{
		{name: "missing certificate", files: map[string][]byte{}, want: "read active KAP mTLS certificate"},
		{name: "missing key", files: map[string][]byte{ClientCertificateFileName: []byte("cert")}, want: "read active KAP mTLS private key"},
		{name: "missing gateway CA", files: map[string][]byte{ClientCertificateFileName: []byte("cert"), ClientPrivateKeyFileName: []byte("key")}, want: "read active KAP mTLS gateway CA"},
		{name: "missing environment", files: map[string][]byte{ClientCertificateFileName: []byte("cert"), ClientPrivateKeyFileName: []byte("key"), GatewayCAFileName: []byte("ca")}, want: "read KAP mTLS agent environment"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			paths := testPaths(t)
			releaseDir := filepath.Join(paths.StateDir, ReleasesDirectoryName, releaseID)
			require.NoError(t, os.MkdirAll(releaseDir, 0700))
			for name, data := range tt.files {
				require.NoError(t, os.WriteFile(filepath.Join(releaseDir, name), data, 0600))
			}
			require.NoError(t, os.Symlink(filepath.Join(ReleasesDirectoryName, releaseID), filepath.Join(paths.StateDir, CurrentSymlinkName)))
			_, err := (&Manager{paths: paths, now: func() time.Time { return now }}).inspectCredentials("machine-a")
			require.ErrorContains(t, err, tt.want)
		})
	}

	t.Run("release ID does not match contents", func(t *testing.T) {
		paths := testPaths(t)
		manager := &Manager{paths: paths, now: func() time.Time { return now }}
		credentials := testCredentials(t, "worker-a", "machine-a", 1, now.Add(-time.Minute), now.Add(time.Hour))
		generation, err := manager.stageCredentials("machine-a", credentials)
		require.NoError(t, err)
		wrongID := strings.Repeat("b", 64)
		require.NoError(t, os.Rename(
			filepath.Join(paths.StateDir, ReleasesDirectoryName, generation.releaseID),
			filepath.Join(paths.StateDir, ReleasesDirectoryName, wrongID),
		))
		require.NoError(t, os.Symlink(filepath.Join(ReleasesDirectoryName, wrongID), filepath.Join(paths.StateDir, CurrentSymlinkName)))
		_, err = manager.inspectCredentials("machine-a")
		require.ErrorContains(t, err, "release ID does not match its contents")
	})

	for _, tt := range []struct {
		name string
		cert []byte
		env  []byte
		ca   []byte
		want string
	}{
		{name: "missing certificate", want: "read applied KAP mTLS certificate"},
		{name: "bad certificate PEM", cert: []byte("bad"), want: "parse applied KAP mTLS certificate PEM"},
		{name: "bad certificate DER", cert: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: []byte("bad")}), want: "parse applied KAP mTLS certificate"},
		{name: "missing environment", cert: testCredentials(t, "worker-a", "machine-a", 1, now.Add(-time.Minute), now.Add(time.Hour)).CertificatePEM, want: "read KAP mTLS agent environment"},
	} {
		t.Run("activation "+tt.name, func(t *testing.T) {
			paths := testPaths(t)
			releaseDir := filepath.Join(paths.StateDir, ReleasesDirectoryName, releaseID)
			require.NoError(t, os.MkdirAll(releaseDir, 0700))
			if tt.cert != nil {
				require.NoError(t, os.WriteFile(filepath.Join(releaseDir, ClientCertificateFileName), tt.cert, 0600))
			}
			_, err := (&Manager{paths: paths}).inspectReleaseActivation(releaseID)
			require.ErrorContains(t, err, tt.want)
		})
	}
	_, err := (&Manager{}).inspectReleaseActivation("bad")
	require.Error(t, err)
}

func TestStateCleanupAndTLSFailures(t *testing.T) {
	t.Run("file and marker errors", func(t *testing.T) {
		directory := t.TempDir()
		path := filepath.Join(directory, "synced")
		require.NoError(t, writeSyncedFile(path, []byte("data"), 0600))
		require.ErrorContains(t, writeSyncedFile(path, []byte("data"), 0600), "create")
		manager := &Manager{paths: Paths{StateDir: directory}}
		require.NoError(t, os.Mkdir(filepath.Join(directory, "marker"), 0700))
		_, err := manager.readMarker("marker")
		require.ErrorContains(t, err, "read KAP mTLS marker marker")
		require.NoError(t, os.WriteFile(filepath.Join(directory, "marker", "child"), nil, 0600))
		require.ErrorContains(t, manager.clearMarker("marker"), "remove KAP mTLS marker marker")
	})

	t.Run("current generation removal", func(t *testing.T) {
		paths := testPaths(t)
		manager := &Manager{paths: paths}
		require.NoError(t, os.MkdirAll(filepath.Join(paths.StateDir, ReleasesDirectoryName), 0700))
		require.NoError(t, manager.removeCurrentGeneration(strings.Repeat("a", 64)))
		currentID := strings.Repeat("b", 64)
		require.NoError(t, os.Symlink(filepath.Join(ReleasesDirectoryName, currentID), filepath.Join(paths.StateDir, CurrentSymlinkName)))
		require.ErrorContains(t, manager.removeCurrentGeneration(strings.Repeat("a", 64)), "expected failed generation")
		require.NoError(t, manager.removeCurrentGeneration(currentID))
		assert.NoFileExists(t, filepath.Join(paths.StateDir, CurrentSymlinkName))
	})

	t.Run("cleanup missing and corrupt stores", func(t *testing.T) {
		paths := testPaths(t)
		manager := &Manager{paths: paths}
		require.NoError(t, os.MkdirAll(paths.StateDir, 0700))
		require.NoError(t, manager.cleanupOrphanedReleases())
		require.NoError(t, manager.garbageCollectReleases(strings.Repeat("a", 64), ""))
		require.NoError(t, os.WriteFile(filepath.Join(paths.StateDir, ReleasesDirectoryName), []byte("file"), 0600))
		require.ErrorContains(t, manager.cleanupOrphanedReleases(), "list KAP mTLS releases")
		require.ErrorContains(t, manager.garbageCollectReleases(strings.Repeat("a", 64), ""), "list KAP mTLS releases")
	})

	credentials := testCredentials(t, "worker-a", "machine-a", 1, time.Now().Add(-time.Minute), time.Now().Add(time.Hour))
	err := verifyCandidateGatewayTLS(context.Background(), "127.0.0.1:1", "kap.example.test", []byte("bad"), credentials.PrivateKeyPEM, credentials.GatewayCAPEM, credentials.CertificatePEM, credentials.PrivateKeyPEM)
	require.ErrorContains(t, err, "parse staged agent client certificate")
	err = verifyCandidateGatewayTLS(context.Background(), "127.0.0.1:1", "kap.example.test", credentials.CertificatePEM, credentials.PrivateKeyPEM, credentials.GatewayCAPEM, []byte("bad"), credentials.PrivateKeyPEM)
	require.ErrorContains(t, err, "parse kubelet client certificate")
	err = verifyCandidateGatewayTLS(context.Background(), "127.0.0.1:1", "kap.example.test", credentials.CertificatePEM, credentials.PrivateKeyPEM, []byte("bad"), credentials.CertificatePEM, credentials.PrivateKeyPEM)
	require.ErrorContains(t, err, "parse gateway CA bundle")
	err = verifyCandidateGatewayTLS(context.Background(), "127.0.0.1:1", "kap.example.test", credentials.CertificatePEM, credentials.PrivateKeyPEM, credentials.GatewayCAPEM, credentials.CertificatePEM, credentials.PrivateKeyPEM)
	require.Error(t, err)
	err = verifyLocalAgentTLS(context.Background(), "127.0.0.1:1", "kap.example.test", []byte("bad"), credentials.PrivateKeyPEM, credentials.GatewayCAPEM)
	require.ErrorContains(t, err, "parse client certificate")
	err = verifyLocalAgentTLS(context.Background(), "127.0.0.1:1", "kap.example.test", credentials.CertificatePEM, credentials.PrivateKeyPEM, []byte("bad"))
	require.ErrorContains(t, err, "parse gateway CA bundle")
	err = verifyLocalAgentTLS(context.Background(), "127.0.0.1:1", "kap.example.test", credentials.CertificatePEM, credentials.PrivateKeyPEM, credentials.GatewayCAPEM)
	require.Error(t, err)
}

func TestRemainingDurabilityAndProbeBranches(t *testing.T) {
	now := time.Date(2026, 7, 8, 0, 0, 0, 0, time.UTC)
	credentials := testCredentials(t, "worker-a", "machine-a", 1, now.Add(-time.Minute), now.Add(time.Hour))

	t.Run("stage failures", func(t *testing.T) {
		paths := testPaths(t)
		manager := &Manager{paths: paths, now: func() time.Time { return now }}
		_, err := manager.stageCredentials("machine-a", Credentials{})
		require.Error(t, err)
		require.NoError(t, os.WriteFile(paths.StateDir, []byte("file"), 0600))
		_, err = manager.stageCredentials("machine-a", credentials)
		require.ErrorContains(t, err, "create KAP mTLS state directory")

		paths = testPaths(t)
		manager.paths = paths
		require.NoError(t, os.MkdirAll(paths.StateDir, 0700))
		require.NoError(t, os.WriteFile(filepath.Join(paths.StateDir, ReleasesDirectoryName), []byte("file"), 0600))
		_, err = manager.stageCredentials("machine-a", credentials)
		require.Error(t, err)

		paths = testPaths(t)
		manager.paths = paths
		require.NoError(t, os.MkdirAll(paths.StateDir, 0700))
		require.NoError(t, os.Symlink("invalid", filepath.Join(paths.StateDir, CurrentSymlinkName)))
		_, err = manager.stageCredentials("machine-a", credentials)
		require.Error(t, err)
	})

	t.Run("update control failures", func(t *testing.T) {
		paths := testPaths(t)
		writeTestKubeconfigWithClientData(t, paths.Kubeconfig, "https://kap.example.test", "", credentials.GatewayCAPEM, credentials.CertificatePEM, credentials.PrivateKeyPEM)
		require.NoError(t, os.MkdirAll(filepath.Join(paths.StateDir, AppliedMarkerName), 0700))
		manager := newTestManager(paths, &fakeRunner{}, now)
		require.ErrorContains(t, manager.UpdateCredentials(context.Background(), "machine-a", credentials), "read KAP mTLS applied marker")

		paths = testPaths(t)
		writeTestKubeconfigWithClientData(t, paths.Kubeconfig, "https://kap.example.test", "", credentials.GatewayCAPEM, credentials.CertificatePEM, credentials.PrivateKeyPEM)
		installTestAgent(t, paths)
		runner := &fakeRunner{state: "unexpected"}
		manager = newTestManager(paths, runner, now)
		require.ErrorContains(t, manager.UpdateCredentials(context.Background(), "machine-a", credentials), "systemctl is-active")

		paths = testPaths(t)
		writeTestKubeconfigWithClientData(t, paths.Kubeconfig, "https://kap.example.test", "", credentials.GatewayCAPEM, credentials.CertificatePEM, credentials.PrivateKeyPEM)
		installTestAgent(t, paths)
		runner = &fakeRunner{state: "inactive", failAgentEnable: 1}
		manager = newTestManager(paths, runner, now)
		require.ErrorContains(t, manager.UpdateCredentials(context.Background(), "machine-a", credentials), "enable failed")
	})

	t.Run("disable control failures", func(t *testing.T) {
		paths := testPaths(t)
		caPEM := writeTestKubeconfig(t, paths.Kubeconfig, "https://kap.example.test", "")
		installTestAgent(t, paths)
		runner := &fakeRunner{state: "active", failAgentDisable: 1}
		manager := newTestManager(paths, runner, now)
		err := manager.Configure(context.Background(), Config{Server: "https://kap.example.test", CertificateAuthorityData: caPEM})
		require.ErrorContains(t, err, "disable failed")

		paths = testPaths(t)
		caPEM = writeTestKubeconfig(t, paths.Kubeconfig, "https://kap.example.test", "")
		require.NoError(t, os.MkdirAll(filepath.Join(paths.StateDir, ActivationPendingMarkerName), 0700))
		require.NoError(t, os.WriteFile(filepath.Join(paths.StateDir, ActivationPendingMarkerName, "child"), nil, 0600))
		manager = newTestManager(paths, &fakeRunner{}, now)
		err = manager.Configure(context.Background(), Config{Server: "https://kap.example.test", CertificateAuthorityData: caPEM})
		require.ErrorContains(t, err, "remove KAP mTLS activation-pending marker")
	})

	t.Run("kubelet restore failures", func(t *testing.T) {
		paths := testPaths(t)
		require.NoError(t, os.MkdirAll(paths.Kubeconfig, 0700))
		manager := newTestManager(paths, &fakeRunner{}, now)
		err := manager.handleKubeletRestartFailure(context.Background(), []byte("original"), 0600, true, errors.New("restart"))
		require.ErrorContains(t, err, "restore previous kubeconfig")

		paths = testPaths(t)
		require.NoError(t, os.WriteFile(paths.Kubeconfig, []byte("current"), 0600))
		require.NoError(t, os.MkdirAll(filepath.Join(paths.StateDir, KubeconfigPendingMarkerName), 0700))
		require.NoError(t, os.WriteFile(filepath.Join(paths.StateDir, KubeconfigPendingMarkerName, "child"), nil, 0600))
		manager = newTestManager(paths, &fakeRunner{}, now)
		err = manager.handleKubeletRestartFailure(context.Background(), []byte("original"), 0600, true, errors.New("restart"))
		require.ErrorContains(t, err, "clear pending marker")
	})

	t.Run("commit and cleanup failures", func(t *testing.T) {
		paths := testPaths(t)
		manager := &Manager{paths: paths}
		require.NoError(t, os.WriteFile(paths.StateDir, []byte("file"), 0600))
		require.Error(t, manager.commitAppliedGeneration(strings.Repeat("a", 64), ""))

		paths = testPaths(t)
		manager.paths = paths
		require.NoError(t, os.MkdirAll(paths.StateDir, 0700))
		require.NoError(t, os.Symlink("invalid", filepath.Join(paths.StateDir, CurrentSymlinkName)))
		err := manager.discardStagedGeneration(strings.Repeat("a", 64), errors.New("activation"))
		require.ErrorContains(t, err, "discard staged generation")
	})

	t.Run("kubeconfig inspection errors", func(t *testing.T) {
		directory := t.TempDir()
		_, _, _, err := inspectKubeconfig(filepath.Join(directory, "missing"))
		require.ErrorContains(t, err, "read kubelet kubeconfig")
		path := filepath.Join(directory, "kubeconfig")
		require.NoError(t, os.WriteFile(path, []byte("clusters:\n- name: kubernetes\n  cluster: {}\n"), 0600))
		_, _, _, err = inspectKubeconfig(path)
		require.ErrorContains(t, err, "cluster has no server")
		require.NoError(t, os.WriteFile(path, []byte("clusters:\n- name: kubernetes\n  cluster:\n    server: https://kap.example.test\n"), 0600))
		_, _, _, err = inspectKubeconfig(path)
		require.ErrorContains(t, err, "has no certificate-authority-data")
		require.NoError(t, os.WriteFile(path, []byte("clusters:\n- name: kubernetes\n  cluster:\n    server: https://kap.example.test\n    certificate-authority-data: YmFk\n"), 0600))
		_, _, _, err = inspectKubeconfig(path)
		require.ErrorContains(t, err, "parse kubelet kubeconfig certificate authority data")
		_, _, _, _, err = prepareKubeconfig(filepath.Join(directory, "missing"), Config{})
		require.ErrorContains(t, err, "read kubelet kubeconfig")
		require.NoError(t, os.WriteFile(path, []byte("clusters: ["), 0600))
		_, _, _, _, err = prepareKubeconfig(path, Config{})
		require.ErrorContains(t, err, "parse kubelet kubeconfig")
	})

	t.Run("single user fallback", func(t *testing.T) {
		directory := t.TempDir()
		path := filepath.Join(directory, "kubeconfig")
		content := fmt.Sprintf("users:\n- name: custom\n  user:\n    client-certificate-data: %s\n    client-key-data: %s\n", base64.StdEncoding.EncodeToString([]byte("cert")), base64.StdEncoding.EncodeToString([]byte("key")))
		require.NoError(t, os.WriteFile(path, []byte(content), 0600))
		certificate, key, err := readKubeconfigClientCredentials(path)
		require.NoError(t, err)
		assert.Equal(t, []byte("cert"), certificate)
		assert.Equal(t, []byte("key"), key)
	})

	t.Run("metrics variants", func(t *testing.T) {
		responses := []string{
			"# TYPE other gauge\nother 1\n",
			"# TYPE kaproxy_mtls_agent_cert_info gauge\nkaproxy_mtls_agent_cert_info{serial=\"1\"} 0\n",
			"# TYPE kaproxy_mtls_agent_cert_info gauge\nkaproxy_mtls_agent_cert_info{serial=\"2\"} 1\n",
		}
		for _, body := range responses {
			body := body
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(body)) }))
			manager := &Manager{metricsURL: server.URL, httpClient: server.Client()}
			assert.False(t, manager.probeAgentCertificateSerial(context.Background(), "1"))
			server.Close()
		}
	})

	t.Run("sync directory open error", func(t *testing.T) {
		require.ErrorContains(t, syncDirectory(filepath.Join(t.TempDir(), "missing")), "open directory")
	})
}

func TestConfigureDurableApplyFailures(t *testing.T) {
	now := time.Date(2026, 7, 8, 0, 0, 0, 0, time.UTC)
	caPEM := testCAPEM(t, "kubeconfig-ca")
	t.Run("invalid config", func(t *testing.T) {
		manager := &Manager{}
		require.Error(t, manager.Configure(context.Background(), Config{}))
	})
	t.Run("credential inspection error", func(t *testing.T) {
		paths := testPaths(t)
		writeTestKubeconfig(t, paths.Kubeconfig, "https://kap.example.test", "")
		require.NoError(t, os.MkdirAll(paths.StateDir, 0700))
		require.NoError(t, os.WriteFile(filepath.Join(paths.StateDir, CurrentSymlinkName), nil, 0600))
		manager := newTestManager(paths, &fakeRunner{}, now)
		err := manager.Configure(context.Background(), Config{Enabled: true, Server: LocalEndpoint, TLSServerName: "kap.example.test", CertificateAuthorityData: caPEM})
		require.ErrorContains(t, err, "read KAP mTLS current symlink")
	})
	t.Run("service inspection error", func(t *testing.T) {
		paths := testPaths(t)
		credentials := testCredentials(t, "worker-a", "machine-a", 1, now.Add(-time.Minute), now.Add(time.Hour))
		writeTestKubeconfigWithClientData(t, paths.Kubeconfig, "https://kap.example.test", "", credentials.GatewayCAPEM, credentials.CertificatePEM, credentials.PrivateKeyPEM)
		installTestAgent(t, paths)
		manager := newTestManager(paths, &fakeRunner{state: "unexpected"}, now)
		generation, err := manager.stageCredentials("machine-a", credentials)
		require.NoError(t, err)
		require.NoError(t, manager.swapCurrentSymlink(generation.releaseID))
		err = manager.Configure(context.Background(), Config{Enabled: true, Server: LocalEndpoint, TLSServerName: credentials.ServerName, CertificateAuthorityData: credentials.GatewayCAPEM})
		require.ErrorContains(t, err, "systemctl is-active")
	})
	t.Run("pending marker stat error", func(t *testing.T) {
		paths := testPaths(t)
		writeTestKubeconfig(t, paths.Kubeconfig, "https://kap.example.test", "")
		require.NoError(t, os.WriteFile(paths.StateDir, []byte("file"), 0600))
		manager := newTestManager(paths, &fakeRunner{}, now)
		err := manager.Configure(context.Background(), Config{Server: "https://kap.example.test", CertificateAuthorityData: caPEM})
		require.ErrorContains(t, err, "inspect KAP mTLS kubeconfig-pending marker")
	})
	t.Run("pending marker write error", func(t *testing.T) {
		paths := testPaths(t)
		writeTestKubeconfig(t, paths.Kubeconfig, "https://old.example.test", "")
		require.NoError(t, os.MkdirAll(filepath.Join(paths.StateDir, KubeconfigPendingMarkerName), 0700))
		require.NoError(t, os.WriteFile(filepath.Join(paths.StateDir, KubeconfigPendingMarkerName, "child"), nil, 0600))
		manager := newTestManager(paths, &fakeRunner{}, now)
		err := manager.Configure(context.Background(), Config{Server: "https://kap.example.test", CertificateAuthorityData: caPEM})
		require.Error(t, err)
	})
	t.Run("pending marker clear error", func(t *testing.T) {
		paths := testPaths(t)
		ca := writeTestKubeconfig(t, paths.Kubeconfig, "https://kap.example.test", "")
		require.NoError(t, os.MkdirAll(filepath.Join(paths.StateDir, KubeconfigPendingMarkerName), 0700))
		require.NoError(t, os.WriteFile(filepath.Join(paths.StateDir, KubeconfigPendingMarkerName, "child"), nil, 0600))
		manager := newTestManager(paths, &fakeRunner{}, now)
		err := manager.Configure(context.Background(), Config{Server: "https://kap.example.test", CertificateAuthorityData: ca})
		require.Error(t, err)
	})
	t.Run("disabled marker write error", func(t *testing.T) {
		paths := testPaths(t)
		ca := writeTestKubeconfig(t, paths.Kubeconfig, "https://kap.example.test", "")
		require.NoError(t, os.MkdirAll(filepath.Join(paths.StateDir, DisabledMarkerName), 0700))
		require.NoError(t, os.WriteFile(filepath.Join(paths.StateDir, DisabledMarkerName, "child"), nil, 0600))
		manager := newTestManager(paths, &fakeRunner{}, now)
		err := manager.Configure(context.Background(), Config{Server: "https://kap.example.test", CertificateAuthorityData: ca})
		require.Error(t, err)
	})
}

func TestRollbackActivationFailureDetails(t *testing.T) {
	activationErr := errors.New("activation failed")
	releaseID := strings.Repeat("a", 64)
	t.Run("initial current mismatch", func(t *testing.T) {
		paths := testPaths(t)
		require.NoError(t, os.MkdirAll(paths.StateDir, 0700))
		require.NoError(t, os.Symlink(filepath.Join(ReleasesDirectoryName, strings.Repeat("b", 64)), filepath.Join(paths.StateDir, CurrentSymlinkName)))
		manager := &Manager{paths: paths}
		err := manager.rollbackActivation(context.Background(), "", releaseID, activationRestart, "1", activationErr)
		require.ErrorContains(t, err, "remove failed initial generation")
	})
	t.Run("initial cleanup failure", func(t *testing.T) {
		paths := testPaths(t)
		require.NoError(t, os.MkdirAll(filepath.Join(paths.StateDir, ReleasesDirectoryName, releaseID), 0700))
		require.NoError(t, os.Symlink(filepath.Join(ReleasesDirectoryName, releaseID), filepath.Join(paths.StateDir, CurrentSymlinkName)))
		require.NoError(t, os.Mkdir(filepath.Join(paths.StateDir, AppliedMarkerName), 0700))
		manager := &Manager{paths: paths}
		err := manager.rollbackActivation(context.Background(), "", releaseID, activationRestart, "1", activationErr)
		require.ErrorContains(t, err, "clean failed initial generation")
	})
	t.Run("initial pending clear failure", func(t *testing.T) {
		paths := testPaths(t)
		require.NoError(t, os.MkdirAll(filepath.Join(paths.StateDir, ReleasesDirectoryName, releaseID), 0700))
		require.NoError(t, os.Symlink(filepath.Join(ReleasesDirectoryName, releaseID), filepath.Join(paths.StateDir, CurrentSymlinkName)))
		require.NoError(t, os.Mkdir(filepath.Join(paths.StateDir, ActivationPendingMarkerName), 0700))
		require.NoError(t, os.WriteFile(filepath.Join(paths.StateDir, ActivationPendingMarkerName, "child"), nil, 0600))
		manager := &Manager{paths: paths}
		err := manager.rollbackActivation(context.Background(), "", releaseID, activationRestart, "1", activationErr)
		require.ErrorContains(t, err, "clear activation pending after initial rollback")
	})
	t.Run("previous current is invalid", func(t *testing.T) {
		paths := testPaths(t)
		require.NoError(t, os.MkdirAll(paths.StateDir, 0700))
		require.NoError(t, os.WriteFile(filepath.Join(paths.StateDir, CurrentSymlinkName), nil, 0600))
		manager := &Manager{paths: paths}
		err := manager.rollbackActivation(context.Background(), releaseID, strings.Repeat("b", 64), activationRestart, "1", activationErr)
		require.ErrorContains(t, err, "rollback current")
	})
	t.Run("reload rollback fails", func(t *testing.T) {
		paths := testPaths(t)
		require.NoError(t, os.MkdirAll(filepath.Join(paths.StateDir, ReleasesDirectoryName, releaseID), 0700))
		manager := &Manager{paths: paths, runner: &fakeRunner{failAgentReload: 1}}
		err := manager.rollbackActivation(context.Background(), releaseID, strings.Repeat("b", 64), activationReload, "1", activationErr)
		require.ErrorContains(t, err, "reload KAP mTLS agent after rollback")
	})
	t.Run("restart rollback fails", func(t *testing.T) {
		paths := testPaths(t)
		require.NoError(t, os.MkdirAll(filepath.Join(paths.StateDir, ReleasesDirectoryName, releaseID), 0700))
		manager := &Manager{paths: paths, runner: &fakeRunner{failAgentRestart: 1}}
		err := manager.rollbackActivation(context.Background(), releaseID, strings.Repeat("b", 64), activationRestart, "1", activationErr)
		require.ErrorContains(t, err, "restart KAP mTLS agent after rollback")
	})
}

func TestAgentReadyEarlyExitsAndDefaults(t *testing.T) {
	manager := &Manager{
		httpClient: &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			assert.Equal(t, AgentMetricsURL, request.URL.String())
			return nil, errors.New("connection refused")
		})},
	}
	assert.False(t, manager.agentReady(context.Background(), ""))
	assert.False(t, manager.agentReady(context.Background(), strings.Repeat("a", 64)))
	assert.False(t, manager.probeAgentCertificateSerial(context.Background(), "1"))
	paths := testPaths(t)
	manager.paths = paths
	version, err := manager.agentVersion()
	require.NoError(t, err)
	assert.Empty(t, version)
}

func TestAdditionalStateAndWrapperBranches(t *testing.T) {
	now := time.Date(2026, 7, 8, 0, 0, 0, 0, time.UTC)
	credentials := testCredentials(t, "worker-a", "machine-a", 1, now.Add(-time.Minute), now.Add(time.Hour))

	t.Run("unknown successful systemd state", func(t *testing.T) {
		manager := &Manager{runner: commandRunnerFunc(func(context.Context, string, ...string) ([]byte, error) {
			return []byte("mystery\n"), nil
		})}
		active, err := manager.serviceActive(context.Background(), AgentService)
		require.NoError(t, err)
		assert.False(t, active)
	})

	t.Run("invalid certificate DER", func(t *testing.T) {
		_, err := parseCertificateBundle(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: []byte("bad")}))
		require.Error(t, err)
	})

	t.Run("release stat error", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "loop")
		require.NoError(t, os.Symlink("loop", path))
		_, err := releaseMatches(path, Credentials{}, nil)
		require.ErrorContains(t, err, "inspect KAP mTLS release")
	})

	t.Run("active credential validation error", func(t *testing.T) {
		paths := testPaths(t)
		manager := &Manager{paths: paths, now: func() time.Time { return now }}
		generation, err := manager.stageCredentials("machine-a", credentials)
		require.NoError(t, err)
		require.NoError(t, manager.swapCurrentSymlink(generation.releaseID))
		environmentPath := filepath.Join(paths.StateDir, CurrentSymlinkName, AgentEnvironmentFileName)
		require.NoError(t, os.WriteFile(environmentPath, []byte("KAP_MTLS_GATEWAY_ENDPOINT=bad\nKAP_MTLS_SERVER_NAME=bad\nKAP_MTLS_CLIENT_CA_FINGERPRINT="+strings.Repeat("a", 64)+"\nKAP_MTLS_GATEWAY_CA_FINGERPRINT="+credentials.GatewayCAFingerprint+"\n"), 0600))
		_, err = manager.inspectCredentials("machine-a")
		require.ErrorContains(t, err, "validate active KAP mTLS credentials")
	})

	t.Run("applied release missing gateway CA", func(t *testing.T) {
		paths := testPaths(t)
		releaseID := strings.Repeat("a", 64)
		releaseDir := filepath.Join(paths.StateDir, ReleasesDirectoryName, releaseID)
		require.NoError(t, os.MkdirAll(releaseDir, 0700))
		require.NoError(t, os.WriteFile(filepath.Join(releaseDir, ClientCertificateFileName), credentials.CertificatePEM, 0600))
		environment := agentEnvironment{gatewayEndpoint: credentials.GatewayEndpoint, serverName: credentials.ServerName, clientCAFingerprint: credentials.ClientCAFingerprint, gatewayCAFingerprint: credentials.GatewayCAFingerprint}
		require.NoError(t, os.WriteFile(filepath.Join(releaseDir, AgentEnvironmentFileName), marshalAgentEnvironment(environment), 0600))
		_, err := (&Manager{paths: paths}).inspectReleaseActivation(releaseID)
		require.ErrorContains(t, err, "read applied KAP mTLS gateway CA")
	})

	t.Run("commit garbage collection error", func(t *testing.T) {
		paths := testPaths(t)
		require.NoError(t, os.MkdirAll(paths.StateDir, 0700))
		require.NoError(t, os.WriteFile(filepath.Join(paths.StateDir, ReleasesDirectoryName), []byte("file"), 0600))
		err := (&Manager{paths: paths}).commitAppliedGeneration(strings.Repeat("a", 64), "")
		require.ErrorContains(t, err, "list KAP mTLS releases")
	})

	for _, marker := range []string{AppliedMarkerName, ActivationPendingMarkerName} {
		t.Run("cleanup unreadable "+marker, func(t *testing.T) {
			paths := testPaths(t)
			require.NoError(t, os.MkdirAll(filepath.Join(paths.StateDir, ReleasesDirectoryName), 0700))
			require.NoError(t, os.Mkdir(filepath.Join(paths.StateDir, marker), 0700))
			err := (&Manager{paths: paths}).cleanupOrphanedReleases()
			require.Error(t, err)
		})
	}

	t.Run("agent readiness state mismatches", func(t *testing.T) {
		paths := testPaths(t)
		manager := &Manager{paths: paths}
		require.NoError(t, os.MkdirAll(paths.StateDir, 0700))
		currentID := strings.Repeat("a", 64)
		require.NoError(t, os.Symlink(filepath.Join(ReleasesDirectoryName, currentID), filepath.Join(paths.StateDir, CurrentSymlinkName)))
		assert.False(t, manager.agentReady(context.Background(), strings.Repeat("b", 64)))
		require.NoError(t, os.Mkdir(filepath.Join(paths.StateDir, AppliedMarkerName), 0700))
		assert.False(t, manager.agentReady(context.Background(), currentID))
		require.NoError(t, os.Remove(filepath.Join(paths.StateDir, AppliedMarkerName)))
		require.NoError(t, manager.writeMarker(AppliedMarkerName, currentID))
		assert.False(t, manager.agentReady(context.Background(), currentID))
	})

	t.Run("default readiness URL", func(t *testing.T) {
		manager := &Manager{httpClient: &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			assert.Equal(t, AgentReadyURL, request.URL.String())
			return &http.Response{StatusCode: http.StatusServiceUnavailable, Body: http.NoBody}, nil
		})}}
		assert.False(t, manager.probeAgentReady(context.Background()))
	})

	t.Run("default metrics client and non-success response", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte("not ready"))
		}))
		defer server.Close()
		manager := &Manager{metricsURL: server.URL}
		assert.False(t, manager.probeAgentCertificateSerial(context.Background(), "1"))
	})

	t.Run("gateway wrappers propagate kubeconfig errors", func(t *testing.T) {
		manager := &Manager{paths: Paths{Kubeconfig: filepath.Join(t.TempDir(), "missing")}}
		require.Error(t, manager.verifyCandidateGateway(context.Background(), credentials))
		require.Error(t, manager.verifyGateway(context.Background(), credentials))
	})

	t.Run("atomic write path failures", func(t *testing.T) {
		directory := t.TempDir()
		parentFile := filepath.Join(directory, "parent")
		require.NoError(t, os.WriteFile(parentFile, []byte("file"), 0600))
		require.ErrorContains(t, atomicWriteFile(filepath.Join(parentFile, "child"), []byte("data"), 0600), "create directory")
		targetDirectory := filepath.Join(directory, "target")
		require.NoError(t, os.Mkdir(targetDirectory, 0700))
		require.NoError(t, os.WriteFile(filepath.Join(targetDirectory, "child"), nil, 0600))
		require.ErrorContains(t, atomicWriteFile(targetDirectory, []byte("data"), 0600), "replace")
	})

	t.Run("rollback cleanup failures", func(t *testing.T) {
		ready := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))
		defer ready.Close()
		previousID := strings.Repeat("a", 64)
		failedID := strings.Repeat("b", 64)
		for _, pendingFailure := range []bool{false, true} {
			paths := testPaths(t)
			require.NoError(t, os.MkdirAll(filepath.Join(paths.StateDir, ReleasesDirectoryName, previousID), 0700))
			require.NoError(t, os.MkdirAll(filepath.Join(paths.StateDir, ReleasesDirectoryName, failedID), 0700))
			manager := &Manager{paths: paths, runner: &fakeRunner{}, readyURL: ready.URL, certificateSerialVerifier: func(context.Context, string) bool { return true }}
			if pendingFailure {
				require.NoError(t, os.Mkdir(filepath.Join(paths.StateDir, ActivationPendingMarkerName), 0700))
				require.NoError(t, os.WriteFile(filepath.Join(paths.StateDir, ActivationPendingMarkerName, "child"), nil, 0600))
			} else {
				require.NoError(t, os.Mkdir(filepath.Join(paths.StateDir, AppliedMarkerName), 0700))
			}
			err := manager.rollbackActivation(context.Background(), previousID, failedID, activationRestart, "1", errors.New("activation"))
			if pendingFailure {
				require.ErrorContains(t, err, "clear activation pending")
			} else {
				require.ErrorContains(t, err, "cleanup failed generation")
			}
		}
	})
}

func testCredentialsWithLeaf(t *testing.T, now time.Time, mutate func(*x509.Certificate)) Credentials {
	t.Helper()
	credentials := testCredentials(t, "worker-a", "machine-a", 1, now.Add(-time.Minute), now.Add(time.Hour))
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	spiffeURI, err := url.Parse("spiffe://lepton/workercluster/worker-a/machine/machine-a")
	require.NoError(t, err)
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName:   "workercluster:worker-a",
			Organization: []string{clientOrganization},
		},
		NotBefore:   now.Add(-time.Minute),
		NotAfter:    now.Add(time.Hour),
		KeyUsage:    x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		URIs:        []*url.URL{spiffeURI},
	}
	mutate(template)
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	require.NoError(t, err)
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	require.NoError(t, err)
	credentials.CertificatePEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	credentials.PrivateKeyPEM = pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
	return credentials
}

func assertCall(t *testing.T, calls []runnerCall, command string, args ...string) {
	t.Helper()
	want := command + " " + strings.Join(args, " ")
	for _, call := range calls {
		if call.command+" "+strings.Join(call.args, " ") == want {
			return
		}
	}
	t.Fatalf("command %q not found in calls: %#v", want, calls)
}

func countCall(calls []runnerCall, command string, args ...string) int {
	want := command + " " + strings.Join(args, " ")
	count := 0
	for _, call := range calls {
		if call.command+" "+strings.Join(call.args, " ") == want {
			count++
		}
	}
	return count
}

func callIndex(calls []runnerCall, command string, args ...string) int {
	want := command + " " + strings.Join(args, " ")
	for i, call := range calls {
		if call.command+" "+strings.Join(call.args, " ") == want {
			return i
		}
	}
	return -1
}
