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
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testNow = time.Date(2026, time.July, 10, 0, 0, 0, 0, time.UTC)

type fakeRunner struct {
	calls           []string
	active          bool
	enableErr       error
	restartErr      error
	restartFailures int
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
	if len(args) >= 1 && args[0] == "enable" && r.enableErr != nil {
		return []byte("enable failed"), r.enableErr
	}
	if len(args) >= 1 && args[0] == "restart" {
		if r.restartFailures > 0 {
			r.restartFailures--
			return []byte("restart failed"), errors.New("restart failed")
		}
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

func TestUpdateCredentialsRollsBackAndRestartsPreviousRelease(t *testing.T) {
	manager, runner, paths := newTestManager(t)
	require.NoError(t, manager.UpdateCredentials(context.Background(), "machine-1", newTestCredentials(t, "worker-1", "machine-1", 1)))
	previousTarget, err := os.Readlink(filepath.Join(paths.StateDir, CurrentSymlinkName))
	require.NoError(t, err)
	runner.restartFailures = 1

	err = manager.UpdateCredentials(context.Background(), "machine-1", newTestCredentials(t, "worker-1", "machine-1", 2))
	require.ErrorContains(t, err, "restart KAP mTLS agent")
	currentTarget, readErr := os.Readlink(filepath.Join(paths.StateDir, CurrentSymlinkName))
	require.NoError(t, readErr)
	assert.Equal(t, previousTarget, currentTarget)
	assert.Equal(t, 0, runner.restartFailures)
}

func TestUpdateCredentialsRollsBackOnEnableAndReadinessFailures(t *testing.T) {
	t.Run("enable", func(t *testing.T) {
		manager, runner, paths := newTestManager(t)
		runner.enableErr = errors.New("enable unavailable")

		err := manager.UpdateCredentials(context.Background(), "machine-1", newTestCredentials(t, "worker-1", "machine-1", 1))
		require.ErrorContains(t, err, "enable KAP mTLS agent")
		_, statErr := os.Lstat(filepath.Join(paths.StateDir, CurrentSymlinkName))
		require.ErrorIs(t, statErr, os.ErrNotExist)
	})

	t.Run("readiness", func(t *testing.T) {
		manager, _, paths := newTestManager(t)
		require.NoError(t, manager.UpdateCredentials(context.Background(), "machine-1", newTestCredentials(t, "worker-1", "machine-1", 1)))
		previousTarget, err := os.Readlink(filepath.Join(paths.StateDir, CurrentSymlinkName))
		require.NoError(t, err)
		manager.readyURL = "http://127.0.0.1:1"
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		defer cancel()

		err = manager.UpdateCredentials(ctx, "machine-1", newTestCredentials(t, "worker-1", "machine-1", 2))
		require.ErrorContains(t, err, "did not become ready")
		currentTarget, readErr := os.Readlink(filepath.Join(paths.StateDir, CurrentSymlinkName))
		require.NoError(t, readErr)
		assert.Equal(t, previousTarget, currentTarget)
	})
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

func TestActivateFailureModes(t *testing.T) {
	t.Run("agent missing", func(t *testing.T) {
		manager, _, paths := newTestManager(t)
		require.NoError(t, os.Remove(paths.AgentBinary))
		require.ErrorContains(t, manager.Activate(context.Background()), "not installed")
	})

	t.Run("credentials missing", func(t *testing.T) {
		manager, _, _ := newTestManager(t)
		require.ErrorContains(t, manager.Activate(context.Background()), "credentials are not installed")
	})

	for _, test := range []struct {
		name      string
		configure func(*fakeRunner)
		want      string
	}{
		{name: "enable", configure: func(r *fakeRunner) { r.enableErr = errors.New("enable failed") }, want: "enable KAP mTLS agent"},
		{name: "restart", configure: func(r *fakeRunner) { r.restartErr = errors.New("restart failed") }, want: "restart KAP mTLS agent"},
	} {
		t.Run(test.name, func(t *testing.T) {
			manager, runner, _ := newTestManager(t)
			require.NoError(t, manager.UpdateCredentials(context.Background(), "machine-1", newTestCredentials(t, "worker-1", "machine-1", 1)))
			test.configure(runner)
			require.ErrorContains(t, manager.Activate(context.Background()), test.want)
		})
	}

	t.Run("readiness", func(t *testing.T) {
		manager, _, _ := newTestManager(t)
		require.NoError(t, manager.UpdateCredentials(context.Background(), "machine-1", newTestCredentials(t, "worker-1", "machine-1", 1)))
		manager.readyURL = "http://127.0.0.1:1"
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		defer cancel()
		require.ErrorContains(t, manager.Activate(ctx), "did not become ready")
	})
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

func TestStatusWithoutCredentialsOrAgentVersion(t *testing.T) {
	manager, _, paths := newTestManager(t)
	require.NoError(t, os.Remove(paths.AgentVersionFile))

	status, err := manager.Status(context.Background(), "machine-1")
	require.NoError(t, err)
	assert.False(t, status.CredentialsInstalled)
	assert.True(t, status.AgentInstalled)
	assert.Empty(t, status.AgentVersion)
}

func TestStatusReturnsAgentVersionReadError(t *testing.T) {
	manager, _, paths := newTestManager(t)
	require.NoError(t, os.Remove(paths.AgentVersionFile))
	require.NoError(t, os.Mkdir(paths.AgentVersionFile, 0700))

	_, err := manager.Status(context.Background(), "machine-1")
	require.ErrorContains(t, err, "read KAP mTLS agent version")
}

func TestValidateCredentialsRejectsMalformedInputs(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Credentials)
		want   string
	}{
		{name: "missing certificate", mutate: func(c *Credentials) { c.CertificatePEM = nil }, want: "certificate and private key are required"},
		{name: "missing endpoint port", mutate: func(c *Credentials) { c.GatewayEndpoint = "kap.example.test" }, want: "must be a host and port"},
		{name: "invalid endpoint port", mutate: func(c *Credentials) { c.GatewayEndpoint = "kap.example.test:not-a-port" }, want: "invalid port"},
		{name: "invalid endpoint host", mutate: func(c *Credentials) { c.GatewayEndpoint = "bad host:8443"; c.ServerName = "bad host" }, want: "invalid host"},
		{name: "server name mismatch", mutate: func(c *Credentials) { c.ServerName = "other.example.test" }, want: "does not match gateway host"},
		{name: "invalid key pair", mutate: func(c *Credentials) { c.PrivateKeyPEM = []byte("not a key") }, want: "parse KAP mTLS certificate and private key"},
		{name: "uppercase client fingerprint", mutate: func(c *Credentials) { c.ClientCAFingerprint = strings.Repeat("A", sha256HexLength) }, want: "lowercase hexadecimal"},
		{name: "non-hex client fingerprint", mutate: func(c *Credentials) { c.ClientCAFingerprint = strings.Repeat("z", sha256HexLength) }, want: "lowercase hexadecimal"},
		{name: "empty gateway bundle", mutate: func(c *Credentials) { c.GatewayCAPEM = nil }, want: "certificate bundle is empty"},
		{name: "invalid gateway PEM", mutate: func(c *Credentials) { c.GatewayCAPEM = []byte("not pem") }, want: "invalid PEM data"},
		{name: "unexpected gateway PEM block", mutate: func(c *Credentials) {
			c.GatewayCAPEM = pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: []byte("key")})
		}, want: "unexpected PEM block"},
		{name: "non-CA gateway certificate", mutate: func(c *Credentials) { c.GatewayCAPEM = c.CertificatePEM }, want: "is not a CA"},
		{name: "invalid gateway fingerprint", mutate: func(c *Credentials) { c.GatewayCAFingerprint = "short" }, want: "lowercase hexadecimal"},
		{name: "gateway fingerprint mismatch", mutate: func(c *Credentials) { c.GatewayCAFingerprint = strings.Repeat("0", sha256HexLength) }, want: "does not match"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			credentials := newTestCredentials(t, "worker-1", "machine-1", 1)
			test.mutate(&credentials)
			_, err := validateCredentials("machine-1", credentials, testNow, true)
			require.ErrorContains(t, err, test.want)
		})
	}
}

func TestValidateCredentialsRejectsInvalidLeafPolicy(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*x509.Certificate)
		want   string
	}{
		{name: "not current", mutate: func(c *x509.Certificate) {
			c.NotBefore = testNow.Add(time.Hour)
			c.NotAfter = testNow.Add(2 * time.Hour)
		}, want: "not currently valid"},
		{name: "no client auth", mutate: func(c *x509.Certificate) { c.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth} }, want: "not valid for client authentication"},
		{name: "wrong organization", mutate: func(c *x509.Certificate) { c.Subject.Organization = []string{"other"} }, want: "invalid organization"},
		{name: "missing URI", mutate: func(c *x509.Certificate) { c.URIs = nil }, want: "exactly one SPIFFE URI"},
		{name: "wrong URI scheme", mutate: func(c *x509.Certificate) { c.URIs[0].Scheme = "https" }, want: "invalid SPIFFE identity"},
		{name: "common name mismatch", mutate: func(c *x509.Certificate) { c.Subject.CommonName = "workercluster:other" }, want: "common name does not match"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			credentials := newTestCredentialsWithLeaf(t, "worker-1", "machine-1", 1, test.mutate)
			_, err := validateCredentials("machine-1", credentials, testNow, true)
			require.ErrorContains(t, err, test.want)
		})
	}
}

func TestAgentEnvironmentAndReleaseHelpers(t *testing.T) {
	environment := agentEnvironment{
		gatewayEndpoint:      "kap.example.test:8443",
		serverName:           "kap.example.test",
		clientCAFingerprint:  strings.Repeat("a", sha256HexLength),
		gatewayCAFingerprint: strings.Repeat("b", sha256HexLength),
	}
	data := marshalAgentEnvironment(environment)
	root := t.TempDir()
	environmentPath := filepath.Join(root, AgentEnvironmentFileName)
	require.NoError(t, os.WriteFile(environmentPath, data, 0600))
	got, err := readAgentEnvironment(environmentPath)
	require.NoError(t, err)
	assert.Equal(t, environment, got)

	for _, test := range []struct {
		name string
		data string
		want string
	}{
		{name: "invalid line", data: "broken", want: "invalid"},
		{name: "incomplete", data: "KAP_MTLS_SERVER_NAME=kap.example.test\n", want: "incomplete"},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), AgentEnvironmentFileName)
			require.NoError(t, os.WriteFile(path, []byte(test.data), 0600))
			_, err := readAgentEnvironment(path)
			require.ErrorContains(t, err, test.want)
		})
	}
	_, err = readAgentEnvironment(filepath.Join(root, "missing"))
	require.ErrorContains(t, err, "read KAP mTLS agent environment")

	credentials := newTestCredentials(t, "worker-1", "machine-1", 1)
	releaseDir := filepath.Join(root, "release")
	matched, err := releaseMatches(releaseDir, credentials, data)
	require.NoError(t, err)
	assert.False(t, matched)
	require.NoError(t, os.Mkdir(releaseDir, 0700))
	for name, content := range map[string][]byte{
		ClientCertificateFileName: credentials.CertificatePEM,
		ClientPrivateKeyFileName:  credentials.PrivateKeyPEM,
		GatewayCAFileName:         credentials.GatewayCAPEM,
		AgentEnvironmentFileName:  data,
	} {
		require.NoError(t, os.WriteFile(filepath.Join(releaseDir, name), content, 0600))
	}
	matched, err = releaseMatches(releaseDir, credentials, data)
	require.NoError(t, err)
	assert.True(t, matched)
	require.NoError(t, os.WriteFile(filepath.Join(releaseDir, AgentEnvironmentFileName), []byte("changed"), 0600))
	_, err = releaseMatches(releaseDir, credentials, data)
	require.ErrorContains(t, err, "does not match")

	notDirectory := filepath.Join(root, "not-directory")
	require.NoError(t, os.WriteFile(notDirectory, []byte("file"), 0600))
	_, err = releaseMatches(notDirectory, credentials, data)
	require.ErrorContains(t, err, "not a directory")
}

func TestFilesystemAndReadinessHelpers(t *testing.T) {
	root := t.TempDir()
	filePath := filepath.Join(root, "synced")
	require.NoError(t, writeSyncedFile(filePath, []byte("data"), 0600))
	info, err := os.Stat(filePath)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0600), info.Mode().Perm())
	require.ErrorContains(t, writeSyncedFile(filePath, []byte("again"), 0600), "create")
	require.Error(t, syncDirectory(filepath.Join(root, "missing")))

	manager, _, paths := newTestManager(t)
	credentials := newTestCredentials(t, "worker-1", "machine-1", 1)
	releaseID, err := manager.stageCredentials("machine-1", credentials)
	require.NoError(t, err)
	repeatedID, err := manager.stageCredentials("machine-1", credentials)
	require.NoError(t, err)
	assert.Equal(t, releaseID, repeatedID)
	require.Error(t, manager.swapCurrentSymlink("invalid"))
	require.NoError(t, manager.swapCurrentSymlink(releaseID))
	require.NoError(t, manager.swapCurrentSymlink(releaseID))

	currentPath := filepath.Join(paths.StateDir, CurrentSymlinkName)
	require.NoError(t, os.Remove(currentPath))
	require.NoError(t, os.WriteFile(currentPath, []byte("not a symlink"), 0600))
	require.ErrorContains(t, manager.swapCurrentSymlink(releaseID), "not a symlink")
	require.NoError(t, os.Remove(currentPath))
	require.NoError(t, os.Symlink("outside/"+releaseID, currentPath))
	_, err = manager.currentReleaseID()
	require.ErrorContains(t, err, "invalid target")

	var probes atomic.Int32
	readyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if probes.Add(1) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer readyServer.Close()
	manager.readyURL = readyServer.URL
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	assert.True(t, manager.waitReady(ctx))
	assert.GreaterOrEqual(t, probes.Load(), int32(2))

	manager.readyURL = "://invalid"
	assert.False(t, manager.probeReady(context.Background()))
	manager.readyURL = "http://127.0.0.1:1"
	assert.False(t, manager.probeReady(context.Background()))
}

func TestExecRunner(t *testing.T) {
	output, err := (execRunner{}).Run(context.Background(), "sh", "-c", "printf ready")
	require.NoError(t, err)
	assert.Equal(t, "ready", string(output))
}

func TestInspectCredentialsReportsIncompleteRelease(t *testing.T) {
	for _, missing := range []string{
		ClientCertificateFileName,
		ClientPrivateKeyFileName,
		GatewayCAFileName,
		AgentEnvironmentFileName,
	} {
		t.Run(missing, func(t *testing.T) {
			manager, _, paths := newTestManager(t)
			credentials := newTestCredentials(t, "worker-1", "machine-1", 1)
			releaseID, err := manager.stageCredentials("machine-1", credentials)
			require.NoError(t, err)
			require.NoError(t, manager.swapCurrentSymlink(releaseID))
			require.NoError(t, os.Remove(filepath.Join(paths.StateDir, CurrentSymlinkName, missing)))

			_, err = manager.inspectCredentials("machine-1")
			require.Error(t, err)
		})
	}
}

func TestReleaseAndRollbackErrorBoundaries(t *testing.T) {
	t.Run("update reports current selection failure after readiness", func(t *testing.T) {
		manager, _, paths := newTestManager(t)
		removeErr := make(chan error, 1)
		readyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			select {
			case removeErr <- os.Remove(filepath.Join(paths.StateDir, CurrentSymlinkName)):
			default:
			}
			w.WriteHeader(http.StatusOK)
		}))
		t.Cleanup(readyServer.Close)
		manager.readyURL = readyServer.URL

		err := manager.UpdateCredentials(context.Background(), "machine-1", newTestCredentials(t, "worker-1", "machine-1", 1))
		require.NoError(t, <-removeErr)
		require.ErrorContains(t, err, "read KAP mTLS current symlink")
	})

	t.Run("update reports current symlink swap failure", func(t *testing.T) {
		manager, _, paths := newTestManager(t)
		manager.now = func() time.Time {
			require.NoError(t, os.WriteFile(filepath.Join(paths.StateDir, CurrentSymlinkName), []byte("regular"), 0600))
			return testNow
		}

		err := manager.UpdateCredentials(context.Background(), "machine-1", newTestCredentials(t, "worker-1", "machine-1", 1))
		require.ErrorContains(t, err, "current path is not a symlink")
	})

	t.Run("update rejects invalid current selection", func(t *testing.T) {
		manager, _, paths := newTestManager(t)
		require.NoError(t, os.WriteFile(filepath.Join(paths.StateDir, CurrentSymlinkName), []byte("regular"), 0600))
		err := manager.UpdateCredentials(context.Background(), "machine-1", newTestCredentials(t, "worker-1", "machine-1", 1))
		require.ErrorContains(t, err, "read KAP mTLS current symlink")
	})

	t.Run("state directory is a file", func(t *testing.T) {
		root := t.TempDir()
		statePath := filepath.Join(root, "state")
		require.NoError(t, os.WriteFile(statePath, []byte("file"), 0600))
		manager := NewManager(Paths{StateDir: statePath})
		manager.now = func() time.Time { return testNow }
		_, err := manager.stageCredentials("machine-1", newTestCredentials(t, "worker-1", "machine-1", 1))
		require.ErrorContains(t, err, "create KAP mTLS state directory")
	})

	t.Run("releases directory is a file", func(t *testing.T) {
		manager, _, paths := newTestManager(t)
		require.NoError(t, os.WriteFile(filepath.Join(paths.StateDir, ReleasesDirectoryName), []byte("file"), 0600))
		_, err := manager.stageCredentials("machine-1", newTestCredentials(t, "worker-1", "machine-1", 1))
		require.ErrorContains(t, err, "create KAP mTLS releases directory")
	})

	t.Run("releases directory is not writable", func(t *testing.T) {
		if os.Geteuid() == 0 {
			t.Skip("root bypasses directory write permissions")
		}
		manager, _, paths := newTestManager(t)
		releasesDir := filepath.Join(paths.StateDir, ReleasesDirectoryName)
		require.NoError(t, os.Mkdir(releasesDir, 0500))
		t.Cleanup(func() { _ = os.Chmod(releasesDir, 0700) })

		_, err := manager.stageCredentials("machine-1", newTestCredentials(t, "worker-1", "machine-1", 1))
		require.ErrorContains(t, err, "create pending KAP mTLS release")
	})

	t.Run("release is missing a file", func(t *testing.T) {
		root := t.TempDir()
		releaseDir := filepath.Join(root, "release")
		require.NoError(t, os.Mkdir(releaseDir, 0700))
		_, err := releaseMatches(releaseDir, newTestCredentials(t, "worker-1", "machine-1", 1), []byte("env"))
		require.ErrorContains(t, err, "read existing KAP mTLS release file")
	})

	t.Run("stage reports an invalid existing release", func(t *testing.T) {
		manager, _, paths := newTestManager(t)
		credentials := newTestCredentials(t, "worker-1", "machine-1", 1)
		validated, err := validateCredentials("machine-1", credentials, testNow, true)
		require.NoError(t, err)
		releasesDir := filepath.Join(paths.StateDir, ReleasesDirectoryName)
		require.NoError(t, os.MkdirAll(releasesDir, 0700))
		require.NoError(t, os.WriteFile(filepath.Join(releasesDir, validated.releaseID), []byte("file"), 0600))

		_, err = manager.stageCredentials("machine-1", credentials)
		require.ErrorContains(t, err, "is not a directory")
	})

	t.Run("release inspection reports filesystem errors", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "loop")
		require.NoError(t, os.Symlink("loop", path))
		_, err := releaseMatches(path, Credentials{}, nil)
		require.ErrorContains(t, err, "inspect KAP mTLS release")
	})

	t.Run("temporary symlink path is occupied", func(t *testing.T) {
		manager, _, paths := newTestManager(t)
		releaseID := strings.Repeat("a", sha256HexLength)
		tempPath := filepath.Join(paths.StateDir, ".current-"+releaseID)
		require.NoError(t, os.Mkdir(tempPath, 0700))
		require.NoError(t, os.WriteFile(filepath.Join(tempPath, "child"), []byte("data"), 0600))
		err := manager.swapCurrentSymlink(releaseID)
		require.ErrorContains(t, err, "create KAP mTLS current symlink")
	})

	t.Run("current symlink cannot be inspected", func(t *testing.T) {
		if os.Geteuid() == 0 {
			t.Skip("root bypasses directory search permissions")
		}
		manager, _, paths := newTestManager(t)
		require.NoError(t, os.Chmod(paths.StateDir, 0000))
		t.Cleanup(func() { _ = os.Chmod(paths.StateDir, 0700) })

		err := manager.swapCurrentSymlink(strings.Repeat("a", sha256HexLength))
		require.ErrorContains(t, err, "inspect KAP mTLS current symlink")
	})

	t.Run("current target has invalid release ID", func(t *testing.T) {
		manager, _, paths := newTestManager(t)
		require.NoError(t, os.Symlink(filepath.Join(ReleasesDirectoryName, "invalid"), filepath.Join(paths.StateDir, CurrentSymlinkName)))
		_, err := manager.currentReleaseID()
		require.ErrorContains(t, err, "release ID fingerprint")
	})

	t.Run("cleanup without releases directory", func(t *testing.T) {
		manager, _, _ := newTestManager(t)
		err := manager.removeInactiveReleases(strings.Repeat("a", sha256HexLength))
		require.ErrorContains(t, err, "read KAP mTLS releases")
	})

	t.Run("cleanup reports removal failure", func(t *testing.T) {
		if os.Geteuid() == 0 {
			t.Skip("root bypasses directory write permissions")
		}
		manager, _, paths := newTestManager(t)
		releasesDir := filepath.Join(paths.StateDir, ReleasesDirectoryName)
		require.NoError(t, os.Mkdir(releasesDir, 0700))
		require.NoError(t, os.WriteFile(filepath.Join(releasesDir, "inactive"), []byte("data"), 0600))
		require.NoError(t, os.Chmod(releasesDir, 0500))
		t.Cleanup(func() { _ = os.Chmod(releasesDir, 0700) })

		err := manager.removeInactiveReleases("current")
		require.ErrorContains(t, err, "remove inactive KAP mTLS release")
	})

	t.Run("rollback cannot restore invalid previous release", func(t *testing.T) {
		manager, _, _ := newTestManager(t)
		err := manager.rollbackActivation(context.Background(), "invalid", true, errors.New("activation failed"))
		require.ErrorContains(t, err, "restore previous KAP mTLS credentials")
	})

	t.Run("rollback cannot remove non-empty current directory", func(t *testing.T) {
		manager, _, paths := newTestManager(t)
		currentPath := filepath.Join(paths.StateDir, CurrentSymlinkName)
		require.NoError(t, os.Mkdir(currentPath, 0700))
		require.NoError(t, os.WriteFile(filepath.Join(currentPath, "child"), []byte("data"), 0600))
		err := manager.rollbackActivation(context.Background(), "", false, errors.New("activation failed"))
		require.ErrorContains(t, err, "remove failed KAP mTLS credential selection")
	})

	t.Run("rollback reports previous service restart failure", func(t *testing.T) {
		manager, runner, _ := newTestManager(t)
		require.NoError(t, manager.UpdateCredentials(context.Background(), "machine-1", newTestCredentials(t, "worker-1", "machine-1", 1)))
		runner.restartFailures = 2

		err := manager.UpdateCredentials(context.Background(), "machine-1", newTestCredentials(t, "worker-1", "machine-1", 2))
		require.ErrorContains(t, err, "restart KAP mTLS agent with previous credentials")
	})

	t.Run("rollback reports state directory sync failure", func(t *testing.T) {
		manager := NewManager(Paths{StateDir: filepath.Join(t.TempDir(), "missing")})
		err := manager.rollbackActivation(context.Background(), "", false, errors.New("activation failed"))
		require.ErrorContains(t, err, "open directory")
	})

	t.Run("credential inspection reports filesystem errors", func(t *testing.T) {
		if os.Geteuid() == 0 {
			t.Skip("root bypasses directory search permissions")
		}
		manager, _, paths := newTestManager(t)
		require.NoError(t, os.Chmod(paths.StateDir, 0000))
		t.Cleanup(func() { _ = os.Chmod(paths.StateDir, 0700) })

		_, err := manager.inspectCredentials("machine-1")
		require.ErrorContains(t, err, "inspect active KAP mTLS credentials")
	})

	t.Run("directory sync reports sync errors", func(t *testing.T) {
		err := syncDirectory(os.DevNull)
		require.ErrorContains(t, err, "sync directory")
	})
}

func TestAgentVersionUsesDefaultPath(t *testing.T) {
	manager, _, _ := newTestManager(t)
	manager.paths.AgentVersionFile = ""
	version, err := manager.agentVersion()
	require.NoError(t, err)
	assert.Equal(t, "0.1.0", version)
}

func TestParseCertificateBundleRejectsInvalidDER(t *testing.T) {
	_, err := parseCertificateBundle(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: []byte("invalid DER")}))
	require.Error(t, err)
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
	return newTestCredentialsWithLeaf(t, workerCluster, machineID, serial, nil)
}

func newTestCredentialsWithLeaf(t *testing.T, workerCluster, machineID string, serial int64, mutate func(*x509.Certificate)) Credentials {
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
	if mutate != nil {
		mutate(leafTemplate)
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
