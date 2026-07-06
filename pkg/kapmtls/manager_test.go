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

type fakeRunner struct {
	calls              []runnerCall
	state              string
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
	assertCall(t, runner.calls, "systemctl", "disable", "--now", AgentService)
	server, tlsServerName, caFingerprint, err = inspectKubeconfig(paths.Kubeconfig)
	require.NoError(t, err)
	assert.Equal(t, "https://kap.example.test", server)
	assert.Empty(t, tlsServerName)
	assert.Equal(t, fingerprintForPEM(t, originalCA), caFingerprint)

	require.NoError(t, manager.UpdateCredentials(context.Background(), "machine-a", credentials))
	assert.NoFileExists(t, filepath.Join(paths.StateDir, DisabledMarkerName))
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
