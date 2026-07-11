// SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package kapmtls

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testNow = time.Date(2026, time.July, 10, 0, 0, 0, 0, time.UTC)

type fakeRunner struct {
	calls      []string
	active     bool
	restartErr error
}

func (r *fakeRunner) Run(_ context.Context, command string, args ...string) ([]byte, error) {
	call := strings.Join(append([]string{command}, args...), " ")
	r.calls = append(r.calls, call)
	if len(args) >= 1 && args[0] == "is-active" {
		if r.active {
			return []byte("active\n"), nil
		}
		return []byte("inactive\n"), errors.New("inactive")
	}
	if len(args) >= 1 && args[0] == "restart" {
		if r.restartErr != nil {
			return []byte("restart failed"), r.restartErr
		}
		r.active = true
	}
	return nil, nil
}

func TestDefaultPaths(t *testing.T) {
	paths := DefaultPaths("/var/lib/gpud")
	assert.Equal(t, "/var/lib/gpud/kap-mtls", paths.StateDir)
	assert.Equal(t, "/var/lib/gpud/kap-mtls/version", paths.AgentVersionFile)
	assert.Equal(t, DefaultAgentBinaryPath, paths.AgentBinary)
	assert.Equal(t, DefaultAgentUnitPath, paths.AgentUnitFile)
}

func TestUpdateCredentialsActivatesAgent(t *testing.T) {
	manager, runner, paths := newTestManager(t)
	credentials := newTestCredentials(t, "worker-1", "machine-1", 1)

	require.NoError(t, manager.UpdateCredentials(context.Background(), "machine-1", credentials))
	assert.Equal(t, []string{
		"systemctl enable " + AgentService,
		"systemctl restart " + AgentService,
	}, runner.calls)

	target, err := os.Readlink(filepath.Join(paths.StateDir, CurrentSymlinkName))
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(target, ReleasesDirectoryName+string(os.PathSeparator)))
	current := filepath.Join(paths.StateDir, CurrentSymlinkName)
	for _, name := range []string{ClientCertificateFileName, ClientPrivateKeyFileName, GatewayCAFileName, AgentEnvironmentFileName} {
		info, err := os.Stat(filepath.Join(current, name))
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0600), info.Mode().Perm())
	}

	status, err := manager.Status(context.Background(), "machine-1")
	require.NoError(t, err)
	assert.True(t, status.CredentialsInstalled)
	assert.Equal(t, "1", status.CertificateSerial)
	assert.True(t, status.AgentInstalled)
	assert.True(t, status.AgentActive)
	assert.True(t, status.AgentReady)
	assert.Equal(t, "0.1.0", status.AgentVersion)
	assert.Equal(t, credentials.GatewayEndpoint, status.GatewayEndpoint)
	assert.Equal(t, credentials.ServerName, status.ServerName)
}

func TestUpdateCredentialsReplacesAndCleansGeneration(t *testing.T) {
	manager, _, paths := newTestManager(t)
	require.NoError(t, manager.UpdateCredentials(context.Background(), "machine-1", newTestCredentials(t, "worker-1", "machine-1", 1)))
	require.NoError(t, manager.UpdateCredentials(context.Background(), "machine-1", newTestCredentials(t, "worker-1", "machine-1", 2)))

	entries, err := os.ReadDir(filepath.Join(paths.StateDir, ReleasesDirectoryName))
	require.NoError(t, err)
	require.Len(t, entries, 1)
	status, err := manager.Status(context.Background(), "machine-1")
	require.NoError(t, err)
	assert.Equal(t, "2", status.CertificateSerial)
}

func TestUpdateCredentialsRequiresInstalledAgent(t *testing.T) {
	manager, _, paths := newTestManager(t)
	require.NoError(t, os.Remove(paths.AgentBinary))

	err := manager.UpdateCredentials(context.Background(), "machine-1", newTestCredentials(t, "worker-1", "machine-1", 1))
	require.ErrorContains(t, err, "not installed")
}

func TestUpdateCredentialsRejectsWrongMachineIdentity(t *testing.T) {
	manager, _, _ := newTestManager(t)
	err := manager.UpdateCredentials(context.Background(), "machine-2", newTestCredentials(t, "worker-1", "machine-1", 1))
	require.ErrorContains(t, err, "SPIFFE identity")
}

func TestUpdateCredentialsRejectsGatewayFingerprintMismatch(t *testing.T) {
	manager, _, _ := newTestManager(t)
	credentials := newTestCredentials(t, "worker-1", "machine-1", 1)
	credentials.GatewayCAFingerprint = strings.Repeat("0", sha256HexLength)

	err := manager.UpdateCredentials(context.Background(), "machine-1", credentials)
	require.ErrorContains(t, err, "does not match")
}

func TestUpdateCredentialsRollsBackFailedActivation(t *testing.T) {
	manager, runner, paths := newTestManager(t)
	require.NoError(t, manager.UpdateCredentials(context.Background(), "machine-1", newTestCredentials(t, "worker-1", "machine-1", 1)))
	previousTarget, err := os.Readlink(filepath.Join(paths.StateDir, CurrentSymlinkName))
	require.NoError(t, err)
	runner.restartErr = errors.New("systemd unavailable")

	err = manager.UpdateCredentials(context.Background(), "machine-1", newTestCredentials(t, "worker-1", "machine-1", 2))
	require.ErrorContains(t, err, "restart KAP mTLS agent")
	currentTarget, readErr := os.Readlink(filepath.Join(paths.StateDir, CurrentSymlinkName))
	require.NoError(t, readErr)
	assert.Equal(t, previousTarget, currentTarget)

	status, statusErr := manager.Status(context.Background(), "machine-1")
	require.NoError(t, statusErr)
	assert.Equal(t, "1", status.CertificateSerial)
}

func TestUpdateCredentialsRemovesFailedInitialSelection(t *testing.T) {
	manager, runner, paths := newTestManager(t)
	runner.restartErr = errors.New("systemd unavailable")

	err := manager.UpdateCredentials(context.Background(), "machine-1", newTestCredentials(t, "worker-1", "machine-1", 1))
	require.ErrorContains(t, err, "restart KAP mTLS agent")
	_, statErr := os.Lstat(filepath.Join(paths.StateDir, CurrentSymlinkName))
	require.ErrorIs(t, statErr, os.ErrNotExist)
}

func TestActivateRestartsCurrentCredentials(t *testing.T) {
	manager, runner, _ := newTestManager(t)
	require.NoError(t, manager.UpdateCredentials(context.Background(), "machine-1", newTestCredentials(t, "worker-1", "machine-1", 1)))
	runner.calls = nil

	require.NoError(t, manager.Activate(context.Background()))
	assert.Equal(t, []string{
		"systemctl enable " + AgentService,
		"systemctl restart " + AgentService,
	}, runner.calls)
}

func TestConcurrentManagersKeepCurrentRelease(t *testing.T) {
	managerA, _, paths := newTestManager(t)
	managerB := NewManager(paths)
	managerB.runner = &fakeRunner{}
	managerB.httpClient = managerA.httpClient
	managerB.readyURL = managerA.readyURL
	managerB.now = managerA.now

	var wg sync.WaitGroup
	errs := make(chan error, 2)
	credentials := map[int64]Credentials{
		1: newTestCredentials(t, "worker-1", "machine-1", 1),
		2: newTestCredentials(t, "worker-1", "machine-1", 2),
	}
	for serial, manager := range map[int64]*Manager{1: managerA, 2: managerB} {
		wg.Add(1)
		go func(serial int64, manager *Manager) {
			defer wg.Done()
			errs <- manager.UpdateCredentials(context.Background(), "machine-1", credentials[serial])
		}(serial, manager)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}

	target, err := os.Readlink(filepath.Join(paths.StateDir, CurrentSymlinkName))
	require.NoError(t, err)
	_, err = os.Stat(filepath.Join(paths.StateDir, target))
	require.NoError(t, err)
	entries, err := os.ReadDir(filepath.Join(paths.StateDir, ReleasesDirectoryName))
	require.NoError(t, err)
	assert.Len(t, entries, 1)
}

func TestStatusTreatsCorruptGenerationAsMissing(t *testing.T) {
	manager, _, paths := newTestManager(t)
	require.NoError(t, manager.UpdateCredentials(context.Background(), "machine-1", newTestCredentials(t, "worker-1", "machine-1", 1)))
	require.NoError(t, os.WriteFile(filepath.Join(paths.StateDir, CurrentSymlinkName, ClientCertificateFileName), []byte("broken"), 0600))

	status, err := manager.Status(context.Background(), "machine-1")
	require.NoError(t, err)
	assert.False(t, status.CredentialsInstalled)
	assert.True(t, status.AgentInstalled)
}

func newTestManager(t *testing.T) (*Manager, *fakeRunner, Paths) {
	t.Helper()
	root := t.TempDir()
	paths := Paths{
		StateDir:         filepath.Join(root, "kap-mtls"),
		AgentBinary:      filepath.Join(root, "kaproxy-mtls-agent"),
		AgentUnitFile:    filepath.Join(root, "kaproxy-mtls-agent.service"),
		AgentVersionFile: filepath.Join(root, "kap-mtls", agentVersion),
	}
	require.NoError(t, os.MkdirAll(paths.StateDir, 0700))
	require.NoError(t, os.WriteFile(paths.AgentBinary, []byte("binary"), 0755))
	require.NoError(t, os.WriteFile(paths.AgentUnitFile, []byte("unit"), 0644))
	require.NoError(t, os.WriteFile(paths.AgentVersionFile, []byte("0.1.0\n"), 0600))

	readyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(readyServer.Close)
	runner := &fakeRunner{}
	manager := NewManager(paths)
	manager.runner = runner
	manager.httpClient = readyServer.Client()
	manager.readyURL = readyServer.URL
	manager.now = func() time.Time { return testNow }
	return manager, runner, paths
}

func newTestCredentials(t *testing.T, workerCluster, machineID string, serial int64) Credentials {
	t.Helper()
	clientCA, clientCAKey, _ := newTestCA(t, "client-ca")
	gatewayCA, _, gatewayCAPEM := newTestCA(t, "gateway-ca")
	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	identity, err := url.Parse("spiffe://lepton/workercluster/" + workerCluster + "/machine/" + machineID)
	require.NoError(t, err)
	leafTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(serial),
		Subject: pkix.Name{
			CommonName:   "workercluster:" + workerCluster,
			Organization: []string{clientOrganization},
		},
		NotBefore:   testNow.Add(-time.Hour),
		NotAfter:    testNow.Add(5 * 24 * time.Hour),
		KeyUsage:    x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		URIs:        []*url.URL{identity},
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, leafTemplate, clientCA, &leafKey.PublicKey, clientCAKey)
	require.NoError(t, err)
	leafPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: leafDER})
	keyDER, err := x509.MarshalPKCS8PrivateKey(leafKey)
	require.NoError(t, err)
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
	clientFingerprint := sha256.Sum256(clientCA.Raw)
	return Credentials{
		CertificatePEM:       leafPEM,
		PrivateKeyPEM:        keyPEM,
		GatewayCAPEM:         gatewayCAPEM,
		GatewayEndpoint:      "kap.example.test:8443",
		ServerName:           "kap.example.test",
		ClientCAFingerprint:  hex.EncodeToString(clientFingerprint[:]),
		GatewayCAFingerprint: certificateBundleFingerprint([]*x509.Certificate{gatewayCA}),
	}
}

func newTestCA(t *testing.T, commonName string) (*x509.Certificate, *ecdsa.PrivateKey, []byte) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(100),
		Subject:               pkix.Name{CommonName: commonName},
		NotBefore:             testNow.Add(-time.Hour),
		NotAfter:              testNow.Add(30 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	require.NoError(t, err)
	certificate, err := x509.ParseCertificate(der)
	require.NoError(t, err)
	return certificate, key, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}
