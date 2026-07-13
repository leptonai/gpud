// SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

// Package kapmtls manages credentials for the node-local KAP mTLS agent.
package kapmtls

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/binary"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	DefaultAgentBinaryPath = "/usr/local/bin/kaproxy-mtls-agent"
	DefaultAgentUnitPath   = "/etc/systemd/system/kaproxy-mtls-agent.service"

	AgentService    = "kaproxy-mtls-agent.service"
	AgentReadyURL   = "http://127.0.0.1:8440/readyz"
	agentVersion    = "version"
	readyTimeout    = 2 * time.Second
	retryInterval   = 250 * time.Millisecond
	rollbackTimeout = 30 * time.Second
	sha256HexLength = sha256.Size * 2

	ReleasesDirectoryName     = "releases"
	CurrentSymlinkName        = "current"
	AgentEnvironmentFileName  = "agent.env"
	ClientCertificateFileName = "client.crt"
	ClientPrivateKeyFileName  = "client.key"
	GatewayCAFileName         = "gateway-ca.crt"

	clientOrganization = "lepton-workerclient-clients"
)

// Credentials is one complete agent credential generation. Certificate and key
// bytes are sensitive and must not be logged.
type Credentials struct {
	CertificatePEM       []byte
	PrivateKeyPEM        []byte
	GatewayCAPEM         []byte
	GatewayEndpoint      string
	ServerName           string
	ClientCAFingerprint  string
	GatewayCAFingerprint string
}

// Status contains only non-secret state used by gpud-manager reconciliation.
type Status struct {
	CredentialsInstalled bool
	CertificateSerial    string
	CertificateNotAfter  time.Time
	AgentInstalled       bool
	AgentActive          bool
	AgentReady           bool
	AgentVersion         string
	GatewayEndpoint      string
	ServerName           string
	ClientCAFingerprint  string
	GatewayCAFingerprint string
}

type Paths struct {
	StateDir         string
	AgentBinary      string
	AgentUnitFile    string
	AgentVersionFile string
}

func DefaultPaths(dataDir string) Paths {
	stateDir := filepath.Join(dataDir, "kap-mtls")
	return Paths{
		StateDir:         stateDir,
		AgentBinary:      DefaultAgentBinaryPath,
		AgentUnitFile:    DefaultAgentUnitPath,
		AgentVersionFile: filepath.Join(stateDir, agentVersion),
	}
}

type commandRunner interface {
	Run(context.Context, string, ...string) ([]byte, error)
}

type execRunner struct{}

func (execRunner) Run(ctx context.Context, command string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, command, args...).CombinedOutput()
}

type Manager struct {
	paths      Paths
	runner     commandRunner
	httpClient *http.Client
	readyURL   string
	now        func() time.Time
	updateMu   *sync.Mutex
}

var stateDirectoryLocks sync.Map

func NewManager(paths Paths) *Manager {
	return &Manager{
		paths:      paths,
		runner:     execRunner{},
		httpClient: &http.Client{Timeout: readyTimeout},
		readyURL:   AgentReadyURL,
		now:        time.Now,
		updateMu:   lockForStateDirectory(paths.StateDir),
	}
}

func lockForStateDirectory(stateDir string) *sync.Mutex {
	key := filepath.Clean(stateDir)
	lock, _ := stateDirectoryLocks.LoadOrStore(key, &sync.Mutex{})
	return lock.(*sync.Mutex)
}

func (m *Manager) Status(ctx context.Context, machineID string) (*Status, error) {
	m.updateMu.Lock()
	defer m.updateMu.Unlock()

	credentials, err := m.inspectCredentials(machineID)
	if err != nil {
		// A missing, partial, or corrupt generation is recoverable. Reporting it as
		// absent makes gpud-manager issue a fresh credential set.
		credentials = credentialStatus{}
	}

	installed := m.agentInstalled()
	active := false
	ready := false
	if installed {
		active = m.serviceActive(ctx)
		if active {
			ready = m.probeReady(ctx)
		}
	}
	version, err := m.agentVersion()
	if err != nil {
		return nil, err
	}
	return &Status{
		CredentialsInstalled: credentials.installed,
		CertificateSerial:    credentials.serial,
		CertificateNotAfter:  credentials.notAfter,
		AgentInstalled:       installed,
		AgentActive:          active,
		AgentReady:           ready,
		AgentVersion:         version,
		GatewayEndpoint:      credentials.gatewayEndpoint,
		ServerName:           credentials.serverName,
		ClientCAFingerprint:  credentials.clientCAFingerprint,
		GatewayCAFingerprint: credentials.gatewayCAFingerprint,
	}, nil
}

// UpdateCredentials commits one complete immutable generation, atomically points
// current at it, and restarts the agent. Restarting on each five-day certificate
// rotation is intentionally simpler than coordinating hot reload and rollback:
// agent startup validates the selected cert/key before readiness can succeed.
func (m *Manager) UpdateCredentials(ctx context.Context, machineID string, credentials Credentials) error {
	m.updateMu.Lock()
	defer m.updateMu.Unlock()

	if !m.agentInstalled() {
		return fmt.Errorf("KAP mTLS agent is not installed")
	}
	previousReleaseID, hadPreviousRelease, err := m.currentRelease()
	if err != nil {
		return err
	}
	releaseID, err := m.stageCredentials(machineID, credentials)
	if err != nil {
		return err
	}
	if err := m.swapCurrentSymlink(releaseID); err != nil {
		return err
	}
	if _, err := m.runSystemctl(ctx, "enable", AgentService); err != nil {
		return m.rollbackActivation(ctx, previousReleaseID, hadPreviousRelease, fmt.Errorf("enable KAP mTLS agent: %w", err))
	}
	if _, err := m.runSystemctl(ctx, "restart", AgentService); err != nil {
		return m.rollbackActivation(ctx, previousReleaseID, hadPreviousRelease, fmt.Errorf("restart KAP mTLS agent: %w", err))
	}
	if !m.waitReady(ctx) {
		return m.rollbackActivation(ctx, previousReleaseID, hadPreviousRelease, fmt.Errorf("KAP mTLS agent did not become ready"))
	}
	currentReleaseID, err := m.currentReleaseID()
	if err != nil {
		return err
	}
	return m.removeInactiveReleases(currentReleaseID)
}

// Activate restarts the agent against the already selected credential release.
// It does not stage or rotate private key material.
func (m *Manager) Activate(ctx context.Context) error {
	m.updateMu.Lock()
	defer m.updateMu.Unlock()

	if !m.agentInstalled() {
		return fmt.Errorf("KAP mTLS agent is not installed")
	}
	if _, err := m.currentReleaseID(); err != nil {
		return fmt.Errorf("KAP mTLS credentials are not installed: %w", err)
	}
	if _, err := m.runSystemctl(ctx, "enable", AgentService); err != nil {
		return fmt.Errorf("enable KAP mTLS agent: %w", err)
	}
	if _, err := m.runSystemctl(ctx, "restart", AgentService); err != nil {
		return fmt.Errorf("restart KAP mTLS agent: %w", err)
	}
	if !m.waitReady(ctx) {
		return fmt.Errorf("KAP mTLS agent did not become ready")
	}
	return nil
}

func (m *Manager) currentRelease() (string, bool, error) {
	releaseID, err := m.currentReleaseID()
	if errors.Is(err, os.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return releaseID, true, nil
}

func (m *Manager) rollbackActivation(ctx context.Context, previousReleaseID string, hadPreviousRelease bool, activationErr error) error {
	rollbackCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), rollbackTimeout)
	defer cancel()
	if hadPreviousRelease {
		if err := m.swapCurrentSymlink(previousReleaseID); err != nil {
			return errors.Join(activationErr, fmt.Errorf("restore previous KAP mTLS credentials: %w", err))
		}
		if _, err := m.runSystemctl(rollbackCtx, "restart", AgentService); err != nil {
			return errors.Join(activationErr, fmt.Errorf("restart KAP mTLS agent with previous credentials: %w", err))
		}
		return activationErr
	}
	currentPath := filepath.Join(m.paths.StateDir, CurrentSymlinkName)
	if err := os.Remove(currentPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return errors.Join(activationErr, fmt.Errorf("remove failed KAP mTLS credential selection: %w", err))
	}
	if err := syncDirectory(m.paths.StateDir); err != nil {
		return errors.Join(activationErr, err)
	}
	return activationErr
}

type credentialStatus struct {
	installed            bool
	serial               string
	notAfter             time.Time
	gatewayEndpoint      string
	serverName           string
	clientCAFingerprint  string
	gatewayCAFingerprint string
}

type agentEnvironment struct {
	gatewayEndpoint      string
	serverName           string
	clientCAFingerprint  string
	gatewayCAFingerprint string
}

type validatedCredentials struct {
	leaf        *x509.Certificate
	environment agentEnvironment
	releaseID   string
}

func (m *Manager) stageCredentials(machineID string, credentials Credentials) (string, error) {
	validated, err := validateCredentials(machineID, credentials, m.now(), true)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(m.paths.StateDir, 0700); err != nil {
		return "", fmt.Errorf("create KAP mTLS state directory: %w", err)
	}
	if err := os.Chmod(m.paths.StateDir, 0700); err != nil {
		return "", fmt.Errorf("secure KAP mTLS state directory: %w", err)
	}
	releasesDir := filepath.Join(m.paths.StateDir, ReleasesDirectoryName)
	if err := os.MkdirAll(releasesDir, 0700); err != nil {
		return "", fmt.Errorf("create KAP mTLS releases directory: %w", err)
	}

	environment := marshalAgentEnvironment(validated.environment)
	releaseDir := filepath.Join(releasesDir, validated.releaseID)
	exists, err := releaseMatches(releaseDir, credentials, environment)
	if err != nil {
		return "", err
	}
	if exists {
		return validated.releaseID, nil
	}

	tempDir, err := os.MkdirTemp(releasesDir, ".pending-")
	if err != nil {
		return "", fmt.Errorf("create pending KAP mTLS release: %w", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()
	files := []struct {
		name string
		data []byte
	}{
		{name: ClientCertificateFileName, data: credentials.CertificatePEM},
		{name: ClientPrivateKeyFileName, data: credentials.PrivateKeyPEM},
		{name: GatewayCAFileName, data: credentials.GatewayCAPEM},
		{name: AgentEnvironmentFileName, data: environment},
	}
	for _, file := range files {
		if err := writeSyncedFile(filepath.Join(tempDir, file.name), file.data, 0600); err != nil {
			return "", err
		}
	}
	if err := syncDirectory(tempDir); err != nil {
		return "", err
	}
	if err := os.Rename(tempDir, releaseDir); err != nil {
		return "", fmt.Errorf("commit KAP mTLS release %s: %w", validated.releaseID, err)
	}
	if err := syncDirectory(releasesDir); err != nil {
		return "", err
	}
	return validated.releaseID, nil
}

func (m *Manager) inspectCredentials(machineID string) (credentialStatus, error) {
	current := filepath.Join(m.paths.StateDir, CurrentSymlinkName)
	if _, err := os.Lstat(current); errors.Is(err, os.ErrNotExist) {
		return credentialStatus{}, nil
	} else if err != nil {
		return credentialStatus{}, fmt.Errorf("inspect active KAP mTLS credentials: %w", err)
	}
	certificatePEM, err := os.ReadFile(filepath.Join(current, ClientCertificateFileName))
	if err != nil {
		return credentialStatus{}, fmt.Errorf("read active KAP mTLS certificate: %w", err)
	}
	privateKeyPEM, err := os.ReadFile(filepath.Join(current, ClientPrivateKeyFileName))
	if err != nil {
		return credentialStatus{}, fmt.Errorf("read active KAP mTLS private key: %w", err)
	}
	gatewayCAPEM, err := os.ReadFile(filepath.Join(current, GatewayCAFileName))
	if err != nil {
		return credentialStatus{}, fmt.Errorf("read active KAP mTLS gateway CA: %w", err)
	}
	environment, err := readAgentEnvironment(filepath.Join(current, AgentEnvironmentFileName))
	if err != nil {
		return credentialStatus{}, err
	}
	validated, err := validateCredentials(machineID, Credentials{
		CertificatePEM:       certificatePEM,
		PrivateKeyPEM:        privateKeyPEM,
		GatewayCAPEM:         gatewayCAPEM,
		GatewayEndpoint:      environment.gatewayEndpoint,
		ServerName:           environment.serverName,
		ClientCAFingerprint:  environment.clientCAFingerprint,
		GatewayCAFingerprint: environment.gatewayCAFingerprint,
	}, m.now(), false)
	if err != nil {
		return credentialStatus{}, fmt.Errorf("validate active KAP mTLS credentials: %w", err)
	}
	return credentialStatus{
		installed:            true,
		serial:               validated.leaf.SerialNumber.Text(16),
		notAfter:             validated.leaf.NotAfter,
		gatewayEndpoint:      environment.gatewayEndpoint,
		serverName:           environment.serverName,
		clientCAFingerprint:  environment.clientCAFingerprint,
		gatewayCAFingerprint: environment.gatewayCAFingerprint,
	}, nil
}

func validateCredentials(machineID string, credentials Credentials, now time.Time, requireCurrent bool) (validatedCredentials, error) {
	if len(credentials.CertificatePEM) == 0 || len(credentials.PrivateKeyPEM) == 0 {
		return validatedCredentials{}, fmt.Errorf("KAP mTLS certificate and private key are required")
	}
	host, port, err := net.SplitHostPort(credentials.GatewayEndpoint)
	if err != nil || host == "" {
		return validatedCredentials{}, fmt.Errorf("KAP mTLS gateway endpoint %q must be a host and port", credentials.GatewayEndpoint)
	}
	portNumber, err := strconv.ParseUint(port, 10, 16)
	if err != nil || portNumber == 0 {
		return validatedCredentials{}, fmt.Errorf("KAP mTLS gateway endpoint %q has an invalid port", credentials.GatewayEndpoint)
	}
	if strings.ContainsAny(host, "\r\n\t =/@?#") {
		return validatedCredentials{}, fmt.Errorf("KAP mTLS gateway endpoint %q has an invalid host", credentials.GatewayEndpoint)
	}
	if credentials.ServerName == "" || host != credentials.ServerName {
		return validatedCredentials{}, fmt.Errorf("KAP mTLS server name %q does not match gateway host %q", credentials.ServerName, host)
	}
	pair, err := tls.X509KeyPair(credentials.CertificatePEM, credentials.PrivateKeyPEM)
	if err != nil {
		return validatedCredentials{}, fmt.Errorf("parse KAP mTLS certificate and private key: %w", err)
	}
	certificateBlock, _ := pem.Decode(credentials.CertificatePEM)
	if certificateBlock == nil || certificateBlock.Type != "CERTIFICATE" {
		return validatedCredentials{}, fmt.Errorf("parse KAP mTLS certificate PEM")
	}
	leaf, err := x509.ParseCertificate(certificateBlock.Bytes)
	if err != nil {
		return validatedCredentials{}, fmt.Errorf("parse KAP mTLS certificate: %w", err)
	}
	pair.Leaf = leaf
	if requireCurrent && (now.Before(leaf.NotBefore) || !now.Before(leaf.NotAfter)) {
		return validatedCredentials{}, fmt.Errorf("KAP mTLS certificate is not currently valid")
	}
	if !containsExtKeyUsage(leaf.ExtKeyUsage, x509.ExtKeyUsageClientAuth) {
		return validatedCredentials{}, fmt.Errorf("KAP mTLS certificate is not valid for client authentication")
	}
	if !containsString(leaf.Subject.Organization, clientOrganization) {
		return validatedCredentials{}, fmt.Errorf("KAP mTLS certificate has an invalid organization")
	}
	if len(leaf.URIs) != 1 {
		return validatedCredentials{}, fmt.Errorf("KAP mTLS certificate must contain exactly one SPIFFE URI")
	}
	segments := strings.Split(strings.Trim(leaf.URIs[0].Path, "/"), "/")
	if leaf.URIs[0].Scheme != "spiffe" || leaf.URIs[0].Host != "lepton" ||
		len(segments) != 4 || segments[0] != "workercluster" || segments[1] == "" ||
		segments[2] != "machine" || (machineID != "" && segments[3] != machineID) {
		return validatedCredentials{}, fmt.Errorf("KAP mTLS certificate has an invalid SPIFFE identity")
	}
	if leaf.Subject.CommonName != "workercluster:"+segments[1] {
		return validatedCredentials{}, fmt.Errorf("KAP mTLS certificate common name does not match its SPIFFE identity")
	}
	clientFingerprint, err := validateFingerprint("client CA", credentials.ClientCAFingerprint)
	if err != nil {
		return validatedCredentials{}, err
	}
	gatewayCertificates, err := parseCertificateBundle(credentials.GatewayCAPEM)
	if err != nil {
		return validatedCredentials{}, fmt.Errorf("parse KAP mTLS gateway CA bundle: %w", err)
	}
	gatewayFingerprint := certificateBundleFingerprint(gatewayCertificates)
	requestedGatewayFingerprint, err := validateFingerprint("gateway CA", credentials.GatewayCAFingerprint)
	if err != nil {
		return validatedCredentials{}, err
	}
	if requestedGatewayFingerprint != gatewayFingerprint {
		return validatedCredentials{}, fmt.Errorf("KAP mTLS gateway CA fingerprint does not match gateway CA PEM")
	}
	environment := agentEnvironment{
		gatewayEndpoint:      credentials.GatewayEndpoint,
		serverName:           credentials.ServerName,
		clientCAFingerprint:  clientFingerprint,
		gatewayCAFingerprint: gatewayFingerprint,
	}
	environmentBytes := marshalAgentEnvironment(environment)
	return validatedCredentials{
		leaf:        leaf,
		environment: environment,
		releaseID:   generationID(credentials, environmentBytes),
	}, nil
}

func parseCertificateBundle(data []byte) ([]*x509.Certificate, error) {
	remaining := data
	certificates := make([]*x509.Certificate, 0, 1)
	for len(bytes.TrimSpace(remaining)) > 0 {
		block, rest := pem.Decode(remaining)
		if block == nil {
			return nil, fmt.Errorf("invalid PEM data")
		}
		if block.Type != "CERTIFICATE" {
			return nil, fmt.Errorf("unexpected PEM block %q", block.Type)
		}
		certificate, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, err
		}
		if !certificate.IsCA {
			return nil, fmt.Errorf("certificate %q is not a CA", certificate.Subject.CommonName)
		}
		certificates = append(certificates, certificate)
		remaining = rest
	}
	if len(certificates) == 0 {
		return nil, fmt.Errorf("certificate bundle is empty")
	}
	return certificates, nil
}

func certificateBundleFingerprint(certificates []*x509.Certificate) string {
	hash := sha256.New()
	var length [4]byte
	for _, certificate := range certificates {
		binary.BigEndian.PutUint32(length[:], uint32(len(certificate.Raw)))
		_, _ = hash.Write(length[:])
		_, _ = hash.Write(certificate.Raw)
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func validateFingerprint(name, value string) (string, error) {
	if len(value) != sha256HexLength || value != strings.ToLower(value) {
		return "", fmt.Errorf("KAP mTLS %s fingerprint must be %d lowercase hexadecimal characters", name, sha256HexLength)
	}
	decoded, err := hex.DecodeString(value)
	if err != nil || len(decoded) != sha256.Size {
		return "", fmt.Errorf("KAP mTLS %s fingerprint must be %d lowercase hexadecimal characters", name, sha256HexLength)
	}
	return value, nil
}

func generationID(credentials Credentials, environment []byte) string {
	hash := sha256.New()
	for _, value := range [][]byte{credentials.CertificatePEM, credentials.PrivateKeyPEM, credentials.GatewayCAPEM, environment} {
		var length [4]byte
		binary.BigEndian.PutUint32(length[:], uint32(len(value)))
		_, _ = hash.Write(length[:])
		_, _ = hash.Write(value)
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func marshalAgentEnvironment(environment agentEnvironment) []byte {
	return []byte(fmt.Sprintf(
		"KAP_MTLS_GATEWAY_ENDPOINT=%s\nKAP_MTLS_SERVER_NAME=%s\nKAP_MTLS_CLIENT_CA_FINGERPRINT=%s\nKAP_MTLS_GATEWAY_CA_FINGERPRINT=%s\n",
		environment.gatewayEndpoint,
		environment.serverName,
		environment.clientCAFingerprint,
		environment.gatewayCAFingerprint,
	))
}

func readAgentEnvironment(path string) (agentEnvironment, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return agentEnvironment{}, fmt.Errorf("read KAP mTLS agent environment: %w", err)
	}
	values := make(map[string]string)
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return agentEnvironment{}, fmt.Errorf("invalid KAP mTLS agent environment line")
		}
		values[key] = value
	}
	environment := agentEnvironment{
		gatewayEndpoint:      values["KAP_MTLS_GATEWAY_ENDPOINT"],
		serverName:           values["KAP_MTLS_SERVER_NAME"],
		clientCAFingerprint:  values["KAP_MTLS_CLIENT_CA_FINGERPRINT"],
		gatewayCAFingerprint: values["KAP_MTLS_GATEWAY_CA_FINGERPRINT"],
	}
	if environment.gatewayEndpoint == "" || environment.serverName == "" ||
		environment.clientCAFingerprint == "" || environment.gatewayCAFingerprint == "" {
		return agentEnvironment{}, fmt.Errorf("KAP mTLS agent environment is incomplete")
	}
	return environment, nil
}

func releaseMatches(path string, credentials Credentials, environment []byte) (bool, error) {
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("inspect KAP mTLS release: %w", err)
	}
	if !info.IsDir() {
		return false, fmt.Errorf("KAP mTLS release path %s is not a directory", path)
	}
	want := map[string][]byte{
		ClientCertificateFileName: credentials.CertificatePEM,
		ClientPrivateKeyFileName:  credentials.PrivateKeyPEM,
		GatewayCAFileName:         credentials.GatewayCAPEM,
		AgentEnvironmentFileName:  environment,
	}
	for name, expected := range want {
		actual, err := os.ReadFile(filepath.Join(path, name))
		if err != nil {
			return false, fmt.Errorf("read existing KAP mTLS release file %s: %w", name, err)
		}
		if !bytes.Equal(actual, expected) {
			return false, fmt.Errorf("existing KAP mTLS release does not match its generation ID")
		}
	}
	return true, nil
}

func writeSyncedFile(path string, data []byte, mode os.FileMode) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return fmt.Errorf("create %s: %w", path, err)
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return fmt.Errorf("write %s: %w", path, err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return fmt.Errorf("sync %s: %w", path, err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close %s: %w", path, err)
	}
	return nil
}

func (m *Manager) swapCurrentSymlink(releaseID string) error {
	if _, err := validateFingerprint("release ID", releaseID); err != nil {
		return err
	}
	currentPath := filepath.Join(m.paths.StateDir, CurrentSymlinkName)
	if currentID, err := m.currentReleaseID(); err == nil && currentID == releaseID {
		return nil
	}
	if info, err := os.Lstat(currentPath); err == nil && info.Mode()&os.ModeSymlink == 0 {
		return fmt.Errorf("KAP mTLS current path is not a symlink")
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect KAP mTLS current symlink: %w", err)
	}
	tempPath := filepath.Join(m.paths.StateDir, ".current-"+releaseID)
	_ = os.Remove(tempPath)
	defer func() { _ = os.Remove(tempPath) }()
	if err := os.Symlink(filepath.Join(ReleasesDirectoryName, releaseID), tempPath); err != nil {
		return fmt.Errorf("create KAP mTLS current symlink: %w", err)
	}
	if err := os.Rename(tempPath, currentPath); err != nil {
		return fmt.Errorf("commit KAP mTLS current symlink: %w", err)
	}
	return syncDirectory(m.paths.StateDir)
}

func (m *Manager) currentReleaseID() (string, error) {
	target, err := os.Readlink(filepath.Join(m.paths.StateDir, CurrentSymlinkName))
	if err != nil {
		return "", fmt.Errorf("read KAP mTLS current symlink: %w", err)
	}
	cleaned := filepath.Clean(target)
	releaseID := filepath.Base(cleaned)
	if cleaned != filepath.Join(ReleasesDirectoryName, releaseID) {
		return "", fmt.Errorf("KAP mTLS current symlink has an invalid target %q", target)
	}
	if _, err := validateFingerprint("release ID", releaseID); err != nil {
		return "", err
	}
	return releaseID, nil
}

func (m *Manager) removeInactiveReleases(currentID string) error {
	releasesDir := filepath.Join(m.paths.StateDir, ReleasesDirectoryName)
	entries, err := os.ReadDir(releasesDir)
	if err != nil {
		return fmt.Errorf("read KAP mTLS releases: %w", err)
	}
	for _, entry := range entries {
		if entry.Name() == currentID {
			continue
		}
		if err := os.RemoveAll(filepath.Join(releasesDir, entry.Name())); err != nil {
			return fmt.Errorf("remove inactive KAP mTLS release %s: %w", entry.Name(), err)
		}
	}
	return syncDirectory(releasesDir)
}

func (m *Manager) agentInstalled() bool {
	for _, path := range []string{m.paths.AgentBinary, m.paths.AgentUnitFile} {
		info, err := os.Stat(path)
		if err != nil || !info.Mode().IsRegular() {
			return false
		}
	}
	return true
}

func (m *Manager) agentVersion() (string, error) {
	path := m.paths.AgentVersionFile
	if path == "" {
		path = filepath.Join(m.paths.StateDir, agentVersion)
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read KAP mTLS agent version: %w", err)
	}
	return strings.TrimSpace(string(data)), nil
}

func (m *Manager) serviceActive(ctx context.Context) bool {
	_, err := m.runner.Run(ctx, "systemctl", "is-active", "--quiet", AgentService)
	return err == nil
}

func (m *Manager) runSystemctl(ctx context.Context, args ...string) ([]byte, error) {
	output, err := m.runner.Run(ctx, "systemctl", args...)
	if err != nil {
		return output, fmt.Errorf("systemctl %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return output, nil
}

func (m *Manager) waitReady(ctx context.Context) bool {
	if m.probeReady(ctx) {
		return true
	}
	ticker := time.NewTicker(retryInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return false
		case <-ticker.C:
			if m.probeReady(ctx) {
				return true
			}
		}
	}
}

func (m *Manager) probeReady(ctx context.Context) bool {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, m.readyURL, nil)
	if err != nil {
		return false
	}
	response, err := m.httpClient.Do(request)
	if err != nil {
		return false
	}
	defer func() { _ = response.Body.Close() }()
	return response.StatusCode >= http.StatusOK && response.StatusCode < http.StatusMultipleChoices
}

func syncDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open directory %s: %w", path, err)
	}
	defer func() { _ = directory.Close() }()
	if err := directory.Sync(); err != nil {
		return fmt.Errorf("sync directory %s: %w", path, err)
	}
	return nil
}

func containsExtKeyUsage(values []x509.ExtKeyUsage, expected x509.ExtKeyUsage) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func containsString(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}
