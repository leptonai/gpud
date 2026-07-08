// SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

// Package kapmtls manages the node-local KAP mTLS agent credentials and kubelet endpoint cutover.
package kapmtls

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/prometheus/common/expfmt"
	"sigs.k8s.io/yaml"
)

const (
	DefaultAgentBinaryPath = "/usr/local/bin/kaproxy-mtls-agent"
	DefaultAgentUnitPath   = "/etc/systemd/system/kaproxy-mtls-agent.service"
	DefaultKubeconfigPath  = "/root/.kube/config"

	AgentService      = "kaproxy-mtls-agent.service"
	KubeletService    = "kubelet.service"
	LocalEndpoint     = "https://127.0.0.1:6443"
	LocalAgentAddress = "127.0.0.1:6443"
	AgentReadyURL     = "http://127.0.0.1:8440/readyz"
	AgentMetricsURL   = "http://127.0.0.1:8440/metrics"

	ReleasesDirectoryName       = "releases"
	CurrentSymlinkName          = "current"
	AppliedMarkerName           = "applied"
	DisabledMarkerName          = "disabled"
	ActivationPendingMarkerName = "activation-pending"
	KubeconfigPendingMarkerName = "kubeconfig-pending"
	AgentEnvironmentFileName    = "agent.env"
	ClientCertificateFileName   = "client.crt"
	ClientPrivateKeyFileName    = "client.key"
	GatewayCAFileName           = "gateway-ca.crt"

	clientOrganization = "lepton-workerclient-clients"
	gatewayPort        = "8443"
	gatewayRawTCPALPN  = "kaproxy-mtls-tcp/1"
	readyTimeout       = 2 * time.Second
	rollbackTimeout    = 10 * time.Second
	sha256HexLength    = sha256.Size * 2
)

type Credentials struct {
	CertificatePEM       []byte
	PrivateKeyPEM        []byte
	GatewayCAPEM         []byte
	GatewayEndpoint      string
	ServerName           string
	ClientCAFingerprint  string
	GatewayCAFingerprint string
}

type Config struct {
	Enabled                  bool
	Server                   string
	TLSServerName            string
	CertificateAuthorityData []byte
}

type Status struct {
	CredentialsInstalled    bool
	CertificateSerial       string
	CertificateNotAfter     time.Time
	AgentInstalled          bool
	AgentActive             bool
	AgentDisabled           bool
	AgentReady              bool
	AgentVersion            string
	GatewayEndpoint         string
	ServerName              string
	ClientCAFingerprint     string
	GatewayCAFingerprint    string
	KubeconfigServer        string
	KubeconfigTLSServerName string
	KubeconfigCAFingerprint string
	KubeconfigPending       bool
}

type Paths struct {
	StateDir         string
	Kubeconfig       string
	AgentBinary      string
	AgentUnitFile    string
	AgentVersionFile string
}

func DefaultPaths(dataDir string) Paths {
	return Paths{
		StateDir:         filepath.Join(dataDir, "kap-mtls"),
		Kubeconfig:       DefaultKubeconfigPath,
		AgentBinary:      DefaultAgentBinaryPath,
		AgentUnitFile:    DefaultAgentUnitPath,
		AgentVersionFile: filepath.Join(dataDir, "kap-mtls", "version"),
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
	paths                     Paths
	runner                    commandRunner
	httpClient                *http.Client
	readyURL                  string
	metricsURL                string
	localAgentAddress         string
	candidateGatewayAddress   string
	certificateSerialVerifier func(context.Context, string) bool
	candidateVerifier         func(context.Context, Credentials) error
	gatewayVerifier           func(context.Context, Credentials) error
	now                       func() time.Time
}

func NewManager(paths Paths) *Manager {
	return &Manager{
		paths:      paths,
		runner:     execRunner{},
		httpClient: &http.Client{Timeout: readyTimeout},
		readyURL:   AgentReadyURL,
		now:        time.Now,
	}
}

func (m *Manager) Status(ctx context.Context, machineID string) (*Status, error) {
	credentials, err := m.inspectCredentials(machineID)
	if err != nil {
		// A missing, partial, or corrupt credential set is recoverable: report it
		// as absent so the control plane can issue a replacement.
		credentials = credentialStatus{}
	}
	kubeconfigServer, kubeconfigTLSServerName, kubeconfigCAFingerprint, err := inspectKubeconfig(m.paths.Kubeconfig)
	if err != nil {
		return nil, err
	}
	kubeconfigPending, err := m.markerExists(KubeconfigPendingMarkerName)
	if err != nil {
		return nil, err
	}
	agentDisabled, err := m.markerExists(DisabledMarkerName)
	if err != nil {
		return nil, err
	}
	agentInstalled := m.agentInstalled()
	agentActive := false
	agentReady := false
	if agentInstalled {
		agentActive, err = m.serviceActive(ctx, AgentService)
		if err != nil {
			return nil, err
		}
		if agentActive {
			agentReady = m.agentReady(ctx, credentials.releaseID)
		}
	}
	agentVersion, err := m.agentVersion()
	if err != nil {
		return nil, err
	}
	return &Status{
		CredentialsInstalled:    credentials.installed,
		CertificateSerial:       credentials.serial,
		CertificateNotAfter:     credentials.notAfter,
		AgentInstalled:          agentInstalled,
		AgentActive:             agentActive,
		AgentDisabled:           agentDisabled,
		AgentReady:              agentReady,
		AgentVersion:            agentVersion,
		GatewayEndpoint:         credentials.gatewayEndpoint,
		ServerName:              credentials.serverName,
		ClientCAFingerprint:     credentials.clientCAFingerprint,
		GatewayCAFingerprint:    credentials.gatewayCAFingerprint,
		KubeconfigServer:        kubeconfigServer,
		KubeconfigTLSServerName: kubeconfigTLSServerName,
		KubeconfigCAFingerprint: kubeconfigCAFingerprint,
		KubeconfigPending:       kubeconfigPending,
	}, nil
}

func (m *Manager) UpdateCredentials(ctx context.Context, machineID string, credentials Credentials) error {
	appliedID, err := m.readMarker(AppliedMarkerName)
	if err != nil {
		return err
	}
	var previous releaseActivationState
	if appliedID != "" {
		previous, err = m.inspectReleaseActivation(appliedID)
		if err != nil {
			return err
		}
	}
	generation, err := m.stageCredentials(machineID, credentials)
	if err != nil {
		return err
	}
	if err := m.verifyCandidateGateway(ctx, credentials); err != nil {
		return m.discardStagedGeneration(generation.releaseID, fmt.Errorf("verify staged KAP mTLS generation %s: %w", generation.releaseID, err))
	}
	if !m.agentInstalled() {
		if err := m.swapCurrentSymlink(generation.releaseID); err != nil {
			return err
		}
		return m.clearDisabledMarker()
	}
	if err := m.writeMarker(ActivationPendingMarkerName, generation.releaseID); err != nil {
		return err
	}
	if err := m.swapCurrentSymlink(generation.releaseID); err != nil {
		return err
	}
	active, err := m.serviceActive(ctx, AgentService)
	if err != nil {
		return m.rollbackActivation(ctx, appliedID, generation.releaseID, activationRestart, previous.serial, err)
	}
	method := activationRestart
	if appliedID != "" && active && previous.reloadCompatible(generation.activation) {
		method = activationReload
	}
	if method == activationReload {
		if _, err := m.runSystemctl(ctx, "kill", "-s", "HUP", AgentService); err != nil {
			return m.rollbackActivation(ctx, appliedID, generation.releaseID, method, previous.serial, err)
		}
		if !m.waitAgentCertificateSerial(ctx, generation.activation.serial) {
			activationErr := fmt.Errorf("KAP mTLS agent did not load certificate serial %s for generation %s", generation.activation.serial, generation.releaseID)
			return m.rollbackActivation(ctx, appliedID, generation.releaseID, method, previous.serial, activationErr)
		}
	} else {
		if _, err := m.runSystemctl(ctx, "enable", AgentService); err != nil {
			return m.rollbackActivation(ctx, appliedID, generation.releaseID, method, previous.serial, err)
		}
		if _, err := m.runSystemctl(ctx, "restart", AgentService); err != nil {
			return m.rollbackActivation(ctx, appliedID, generation.releaseID, method, previous.serial, err)
		}
		if !m.waitAgentCertificateSerial(ctx, generation.activation.serial) {
			activationErr := fmt.Errorf("KAP mTLS agent did not load certificate serial %s for generation %s after restart", generation.activation.serial, generation.releaseID)
			return m.rollbackActivation(ctx, appliedID, generation.releaseID, method, previous.serial, activationErr)
		}
		if !m.waitAgentReady(ctx) {
			activationErr := fmt.Errorf("KAP mTLS agent did not become ready after loading generation %s", generation.releaseID)
			return m.rollbackActivation(ctx, appliedID, generation.releaseID, method, previous.serial, activationErr)
		}
	}
	if err := m.verifyGateway(ctx, credentials); err != nil {
		activationErr := fmt.Errorf("verify KAP mTLS gateway for generation %s: %w", generation.releaseID, err)
		return m.rollbackActivation(ctx, appliedID, generation.releaseID, method, previous.serial, activationErr)
	}
	if err := m.commitAppliedGeneration(generation.releaseID, appliedID); err != nil {
		return err
	}
	return m.clearDisabledMarker()
}

func (m *Manager) Configure(ctx context.Context, config Config) error {
	if err := validateConfig(config); err != nil {
		return err
	}
	if config.Enabled {
		credentialStatus, err := m.inspectCredentials("")
		if err != nil {
			return err
		}
		if !credentialStatus.installed {
			return fmt.Errorf("KAP mTLS credentials are not installed")
		}
		if !m.now().Before(credentialStatus.notAfter) {
			return fmt.Errorf("KAP mTLS credentials are expired")
		}
		if credentialStatus.serverName != config.TLSServerName {
			return fmt.Errorf("kubeconfig TLS server name %q does not match installed credentials %q", config.TLSServerName, credentialStatus.serverName)
		}
		certificateAuthorities, err := parseCertificateBundle(config.CertificateAuthorityData)
		if err != nil {
			return fmt.Errorf("parse kubeconfig certificate authority data: %w", err)
		}
		if fingerprint := certificateBundleFingerprint(certificateAuthorities); fingerprint != credentialStatus.gatewayCAFingerprint {
			return fmt.Errorf("kubeconfig certificate authority fingerprint %q does not match installed gateway CA fingerprint %q", fingerprint, credentialStatus.gatewayCAFingerprint)
		}
		if !m.agentInstalled() {
			return fmt.Errorf("KAP mTLS agent is not installed")
		}
		active, err := m.serviceActive(ctx, AgentService)
		if err != nil {
			return err
		}
		if !active || !m.agentReady(ctx, credentialStatus.releaseID) {
			return fmt.Errorf("KAP mTLS agent is not ready")
		}
	}

	original, updated, mode, changed, err := prepareKubeconfig(m.paths.Kubeconfig, config)
	if err != nil {
		return err
	}
	pending, err := m.markerExists(KubeconfigPendingMarkerName)
	if err != nil {
		return err
	}
	if changed {
		if !pending {
			if err := m.writeMarker(KubeconfigPendingMarkerName, "pending"); err != nil {
				return err
			}
			pending = true
		}
		if err := atomicWriteFile(m.paths.Kubeconfig, updated, mode); err != nil {
			return err
		}
	}
	if pending {
		if _, err := m.runSystemctl(ctx, "restart", KubeletService); err != nil {
			return m.handleKubeletRestartFailure(ctx, original, mode, changed, err)
		}
		if err := m.clearMarker(KubeconfigPendingMarkerName); err != nil {
			return err
		}
	}

	if !config.Enabled {
		if err := m.writeMarker(DisabledMarkerName, "disabled"); err != nil {
			return err
		}
		if m.agentInstalled() {
			if _, err := m.runSystemctl(ctx, "disable", "--now", AgentService); err != nil {
				return err
			}
		}
		if err := m.clearMarker(ActivationPendingMarkerName); err != nil {
			return err
		}
	}
	return nil
}

func (m *Manager) handleKubeletRestartFailure(
	ctx context.Context,
	original []byte,
	mode os.FileMode,
	changed bool,
	restartErr error,
) error {
	if !changed {
		return fmt.Errorf("restart kubelet after KAP mTLS update: %w; kubeconfig pending marker retained", restartErr)
	}
	restoreErr := atomicWriteFile(m.paths.Kubeconfig, original, mode)
	if restoreErr == nil {
		_, restoreErr = m.runSystemctl(ctx, "restart", KubeletService)
	}
	if restoreErr != nil {
		return fmt.Errorf("restart kubelet after KAP mTLS update: %w; restore previous kubeconfig: %v; kubeconfig pending marker retained", restartErr, restoreErr)
	}
	if err := m.clearMarker(KubeconfigPendingMarkerName); err != nil {
		return fmt.Errorf("restart kubelet after KAP mTLS update: %w; restored previous kubeconfig but clear pending marker: %v", restartErr, err)
	}
	return fmt.Errorf("restart kubelet after KAP mTLS update: %w", restartErr)
}

func (m *Manager) agentInstalled() bool {
	binaryInfo, err := os.Stat(m.paths.AgentBinary)
	if err != nil || binaryInfo.IsDir() || binaryInfo.Mode().Perm()&0111 == 0 {
		return false
	}
	unitInfo, err := os.Stat(m.paths.AgentUnitFile)
	return err == nil && !unitInfo.IsDir()
}

func (m *Manager) serviceActive(ctx context.Context, service string) (bool, error) {
	output, err := m.runner.Run(ctx, "systemctl", "is-active", service)
	state := strings.TrimSpace(string(output))
	switch state {
	case "active":
		return true, nil
	case "activating", "deactivating", "inactive", "failed", "unknown":
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("systemctl is-active %s: %w (output: %s)", service, err, state)
	}
	return false, nil
}

func (m *Manager) runSystemctl(ctx context.Context, args ...string) ([]byte, error) {
	output, err := m.runner.Run(ctx, "systemctl", args...)
	if err != nil {
		return output, fmt.Errorf("systemctl %s: %w (output: %s)", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return output, nil
}

type credentialStatus struct {
	installed            bool
	releaseID            string
	serial               string
	notAfter             time.Time
	gatewayEndpoint      string
	serverName           string
	clientCAFingerprint  string
	gatewayCAFingerprint string
}

type generationResult struct {
	releaseID  string
	activation releaseActivationState
}

type activationMethod int

const (
	activationRestart activationMethod = iota
	activationReload
)

type releaseActivationState struct {
	serial          string
	gatewayEndpoint string
	serverName      string
	gatewayCAPEM    []byte
}

func (state releaseActivationState) reloadCompatible(next releaseActivationState) bool {
	return state.gatewayEndpoint == next.gatewayEndpoint &&
		state.serverName == next.serverName &&
		bytes.Equal(state.gatewayCAPEM, next.gatewayCAPEM)
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

func (m *Manager) stageCredentials(machineID string, credentials Credentials) (generationResult, error) {
	validated, err := validateCredentials(machineID, credentials, m.now(), true)
	if err != nil {
		return generationResult{}, err
	}
	if err := os.MkdirAll(m.paths.StateDir, 0700); err != nil {
		return generationResult{}, fmt.Errorf("create KAP mTLS state directory: %w", err)
	}
	if err := os.Chmod(m.paths.StateDir, 0700); err != nil {
		return generationResult{}, fmt.Errorf("secure KAP mTLS state directory: %w", err)
	}
	releasesDir := filepath.Join(m.paths.StateDir, ReleasesDirectoryName)
	if err := os.MkdirAll(releasesDir, 0700); err != nil {
		return generationResult{}, fmt.Errorf("create KAP mTLS releases directory: %w", err)
	}
	if err := m.cleanupOrphanedReleases(); err != nil {
		return generationResult{}, err
	}

	result := generationResult{
		releaseID: validated.releaseID,
		activation: releaseActivationState{
			serial:          validated.leaf.SerialNumber.Text(16),
			gatewayEndpoint: validated.environment.gatewayEndpoint,
			serverName:      validated.environment.serverName,
			gatewayCAPEM:    append([]byte(nil), credentials.GatewayCAPEM...),
		},
	}

	environment := marshalAgentEnvironment(validated.environment)
	releaseDir := filepath.Join(releasesDir, validated.releaseID)
	exists, err := releaseMatches(releaseDir, credentials, environment)
	if err != nil {
		return generationResult{}, err
	}
	if !exists {
		tempDir, err := os.MkdirTemp(releasesDir, ".pending-")
		if err != nil {
			return generationResult{}, fmt.Errorf("create pending KAP mTLS release: %w", err)
		}
		defer func() { _ = os.RemoveAll(tempDir) }()
		files := []struct {
			name string
			data []byte
			mode os.FileMode
		}{
			{name: ClientCertificateFileName, data: credentials.CertificatePEM, mode: 0600},
			{name: ClientPrivateKeyFileName, data: credentials.PrivateKeyPEM, mode: 0600},
			{name: GatewayCAFileName, data: credentials.GatewayCAPEM, mode: 0600},
			{name: AgentEnvironmentFileName, data: environment, mode: 0600},
		}
		for _, file := range files {
			if err := writeSyncedFile(filepath.Join(tempDir, file.name), file.data, file.mode); err != nil {
				return generationResult{}, err
			}
		}
		if err := syncDirectory(tempDir); err != nil {
			return generationResult{}, err
		}
		if err := os.Rename(tempDir, releaseDir); err != nil {
			return generationResult{}, fmt.Errorf("commit KAP mTLS release %s: %w", validated.releaseID, err)
		}
		if err := syncDirectory(releasesDir); err != nil {
			return generationResult{}, err
		}
	}
	return result, nil
}

func (m *Manager) inspectCredentials(machineID string) (credentialStatus, error) {
	current := filepath.Join(m.paths.StateDir, CurrentSymlinkName)
	if _, err := os.Lstat(current); errors.Is(err, os.ErrNotExist) {
		return credentialStatus{}, nil
	} else if err != nil {
		return credentialStatus{}, fmt.Errorf("inspect active KAP mTLS credentials: %w", err)
	}
	releaseID, err := m.currentReleaseID()
	if err != nil {
		return credentialStatus{}, err
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
	if validated.releaseID != releaseID {
		return credentialStatus{}, fmt.Errorf("active KAP mTLS release ID does not match its contents")
	}
	return credentialStatus{
		installed:            true,
		releaseID:            releaseID,
		serial:               validated.leaf.SerialNumber.Text(16),
		notAfter:             validated.leaf.NotAfter,
		gatewayEndpoint:      environment.gatewayEndpoint,
		serverName:           environment.serverName,
		clientCAFingerprint:  environment.clientCAFingerprint,
		gatewayCAFingerprint: environment.gatewayCAFingerprint,
	}, nil
}

func (m *Manager) inspectReleaseActivation(releaseID string) (releaseActivationState, error) {
	if _, err := validateFingerprint("release ID", releaseID); err != nil {
		return releaseActivationState{}, err
	}
	releaseDir := filepath.Join(m.paths.StateDir, ReleasesDirectoryName, releaseID)
	certificatePEM, err := os.ReadFile(filepath.Join(releaseDir, ClientCertificateFileName))
	if err != nil {
		return releaseActivationState{}, fmt.Errorf("read applied KAP mTLS certificate: %w", err)
	}
	certificateBlock, _ := pem.Decode(certificatePEM)
	if certificateBlock == nil || certificateBlock.Type != "CERTIFICATE" {
		return releaseActivationState{}, fmt.Errorf("parse applied KAP mTLS certificate PEM")
	}
	leaf, err := x509.ParseCertificate(certificateBlock.Bytes)
	if err != nil {
		return releaseActivationState{}, fmt.Errorf("parse applied KAP mTLS certificate: %w", err)
	}
	environment, err := readAgentEnvironment(filepath.Join(releaseDir, AgentEnvironmentFileName))
	if err != nil {
		return releaseActivationState{}, err
	}
	gatewayCAPEM, err := os.ReadFile(filepath.Join(releaseDir, GatewayCAFileName))
	if err != nil {
		return releaseActivationState{}, fmt.Errorf("read applied KAP mTLS gateway CA: %w", err)
	}
	return releaseActivationState{
		serial:          leaf.SerialNumber.Text(16),
		gatewayEndpoint: environment.gatewayEndpoint,
		serverName:      environment.serverName,
		gatewayCAPEM:    gatewayCAPEM,
	}, nil
}

func validateCredentials(machineID string, credentials Credentials, now time.Time, requireCurrent bool) (validatedCredentials, error) {
	if len(credentials.CertificatePEM) == 0 || len(credentials.PrivateKeyPEM) == 0 {
		return validatedCredentials{}, fmt.Errorf("KAP mTLS certificate and private key are required")
	}
	host, port, err := net.SplitHostPort(credentials.GatewayEndpoint)
	if err != nil || host == "" || port != gatewayPort {
		return validatedCredentials{}, fmt.Errorf("KAP mTLS gateway endpoint %q must be a host on port %s", credentials.GatewayEndpoint, gatewayPort)
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
	return validatedCredentials{
		leaf:        leaf,
		environment: environment,
		releaseID:   generationID(credentials, marshalAgentEnvironment(environment)),
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
	for _, value := range [][]byte{
		credentials.CertificatePEM,
		credentials.PrivateKeyPEM,
		credentials.GatewayCAPEM,
		environment,
	} {
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
			return false, fmt.Errorf("existing KAP mTLS release %s does not match its generation ID", filepath.Base(path))
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
	temp, err := os.CreateTemp(m.paths.StateDir, ".current-")
	if err != nil {
		return fmt.Errorf("create temporary KAP mTLS current symlink: %w", err)
	}
	tempPath := temp.Name()
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Remove(tempPath); err != nil {
		return err
	}
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

func (m *Manager) writeMarker(name, value string) error {
	return atomicWriteFile(filepath.Join(m.paths.StateDir, name), []byte(value+"\n"), 0600)
}

func (m *Manager) readMarker(name string) (string, error) {
	data, err := os.ReadFile(filepath.Join(m.paths.StateDir, name))
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read KAP mTLS %s marker: %w", name, err)
	}
	return strings.TrimSpace(string(data)), nil
}

func (m *Manager) markerExists(name string) (bool, error) {
	_, err := os.Stat(filepath.Join(m.paths.StateDir, name))
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("inspect KAP mTLS %s marker: %w", name, err)
	}
	return true, nil
}

func (m *Manager) clearMarker(name string) error {
	path := filepath.Join(m.paths.StateDir, name)
	if err := os.Remove(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("remove KAP mTLS %s marker: %w", name, err)
	}
	return syncDirectory(m.paths.StateDir)
}

func (m *Manager) clearDisabledMarker() error {
	return m.clearMarker(DisabledMarkerName)
}

func (m *Manager) commitAppliedGeneration(releaseID, previousAppliedID string) error {
	if err := m.writeMarker(AppliedMarkerName, releaseID); err != nil {
		return err
	}
	if err := m.clearMarker(ActivationPendingMarkerName); err != nil {
		return err
	}
	return m.garbageCollectReleases(releaseID, previousAppliedID)
}

func (m *Manager) rollbackActivation(
	ctx context.Context,
	previousAppliedID, failedGenerationID string,
	method activationMethod,
	previousSerial string,
	activationErr error,
) error {
	if previousAppliedID == "" {
		if err := m.removeCurrentGeneration(failedGenerationID); err != nil {
			return fmt.Errorf("%w; remove failed initial generation %s: %v", activationErr, failedGenerationID, err)
		}
		if err := m.removeInactiveRelease(failedGenerationID); err != nil {
			return fmt.Errorf("%w; clean failed initial generation %s: %v", activationErr, failedGenerationID, err)
		}
		if err := m.clearMarker(ActivationPendingMarkerName); err != nil {
			return fmt.Errorf("%w; clear activation pending after initial rollback: %v", activationErr, err)
		}
		return activationErr
	}
	rollbackCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), rollbackTimeout)
	defer cancel()
	if err := m.swapCurrentSymlink(previousAppliedID); err != nil {
		return fmt.Errorf("%w; rollback current to generation %s: %v", activationErr, previousAppliedID, err)
	}
	var rollbackErr error
	if method == activationReload {
		if _, err := m.runSystemctl(rollbackCtx, "kill", "-s", "HUP", AgentService); err != nil {
			rollbackErr = fmt.Errorf("reload KAP mTLS agent after rollback: %w", err)
		} else if !m.waitAgentCertificateSerial(rollbackCtx, previousSerial) {
			rollbackErr = fmt.Errorf("KAP mTLS agent did not restore certificate serial %s", previousSerial)
		} else if !m.waitAgentReady(rollbackCtx) {
			rollbackErr = fmt.Errorf("rollback generation %s did not become ready", previousAppliedID)
		}
	} else {
		if _, err := m.runSystemctl(rollbackCtx, "restart", AgentService); err != nil {
			rollbackErr = fmt.Errorf("restart KAP mTLS agent after rollback: %w", err)
		} else if !m.waitAgentCertificateSerial(rollbackCtx, previousSerial) {
			rollbackErr = fmt.Errorf("KAP mTLS agent did not restore certificate serial %s", previousSerial)
		} else if !m.waitAgentReady(rollbackCtx) {
			rollbackErr = fmt.Errorf("rollback generation %s did not become ready", previousAppliedID)
		}
	}
	if rollbackErr != nil {
		return fmt.Errorf("%w; rollback generation %s: %v", activationErr, previousAppliedID, rollbackErr)
	}
	if err := m.removeInactiveRelease(failedGenerationID); err != nil {
		return fmt.Errorf("%w; restored previous applied generation %s; cleanup failed generation: %v", activationErr, previousAppliedID, err)
	}
	if err := m.clearMarker(ActivationPendingMarkerName); err != nil {
		return fmt.Errorf("%w; restored previous applied generation %s; clear activation pending: %v", activationErr, previousAppliedID, err)
	}
	return fmt.Errorf("%w; restored previous applied generation %s", activationErr, previousAppliedID)
}

func (m *Manager) removeCurrentGeneration(releaseID string) error {
	currentID, err := m.currentReleaseID()
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if currentID != releaseID {
		return fmt.Errorf("current generation is %s, expected failed generation %s", currentID, releaseID)
	}
	if err := os.Remove(filepath.Join(m.paths.StateDir, CurrentSymlinkName)); err != nil {
		return fmt.Errorf("remove failed KAP mTLS current symlink: %w", err)
	}
	return syncDirectory(m.paths.StateDir)
}

func (m *Manager) removeInactiveRelease(releaseID string) error {
	if releaseID == "" {
		return nil
	}
	if currentID, err := m.currentReleaseID(); err == nil && currentID == releaseID {
		return nil
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	appliedID, err := m.readMarker(AppliedMarkerName)
	if err != nil {
		return err
	}
	if appliedID == releaseID {
		return nil
	}
	releasesDir := filepath.Join(m.paths.StateDir, ReleasesDirectoryName)
	if err := os.RemoveAll(filepath.Join(releasesDir, releaseID)); err != nil {
		return fmt.Errorf("remove inactive KAP mTLS release %s: %w", releaseID, err)
	}
	return syncDirectory(releasesDir)
}

func (m *Manager) cleanupOrphanedReleases() error {
	keep := make(map[string]struct{}, 3)
	currentID, err := m.currentReleaseID()
	if err == nil {
		keep[currentID] = struct{}{}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	appliedID, err := m.readMarker(AppliedMarkerName)
	if err != nil {
		return err
	}
	if appliedID != "" {
		keep[appliedID] = struct{}{}
	}
	pendingID, err := m.readMarker(ActivationPendingMarkerName)
	if err != nil {
		return err
	}
	if pendingID != "" {
		keep[pendingID] = struct{}{}
	}
	releasesDir := filepath.Join(m.paths.StateDir, ReleasesDirectoryName)
	entries, err := os.ReadDir(releasesDir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("list KAP mTLS releases for orphan cleanup: %w", err)
	}
	for _, entry := range entries {
		if _, ok := keep[entry.Name()]; ok {
			continue
		}
		if err := os.RemoveAll(filepath.Join(releasesDir, entry.Name())); err != nil {
			return fmt.Errorf("remove orphaned KAP mTLS release %s: %w", entry.Name(), err)
		}
	}
	return syncDirectory(releasesDir)
}

func (m *Manager) discardStagedGeneration(releaseID string, activationErr error) error {
	if err := m.cleanupOrphanedReleases(); err != nil {
		return fmt.Errorf("%w; discard staged generation %s: %v", activationErr, releaseID, err)
	}
	return activationErr
}

func (m *Manager) garbageCollectReleases(currentID, previousID string) error {
	releasesDir := filepath.Join(m.paths.StateDir, ReleasesDirectoryName)
	entries, err := os.ReadDir(releasesDir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("list KAP mTLS releases: %w", err)
	}
	keep := map[string]struct{}{currentID: {}}
	if previousID != "" && previousID != currentID {
		keep[previousID] = struct{}{}
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	for _, entry := range entries {
		if _, ok := keep[entry.Name()]; ok {
			continue
		}
		if err := os.RemoveAll(filepath.Join(releasesDir, entry.Name())); err != nil {
			return fmt.Errorf("remove obsolete KAP mTLS release %s: %w", entry.Name(), err)
		}
	}
	return syncDirectory(releasesDir)
}

func (m *Manager) agentReady(ctx context.Context, expectedReleaseID string) bool {
	if expectedReleaseID == "" {
		return false
	}
	pending, err := m.markerExists(ActivationPendingMarkerName)
	if err != nil || pending {
		return false
	}
	currentID, err := m.currentReleaseID()
	if err != nil || currentID != expectedReleaseID {
		return false
	}
	appliedID, err := m.readMarker(AppliedMarkerName)
	if err != nil || appliedID != currentID {
		return false
	}
	activation, err := m.inspectReleaseActivation(currentID)
	if err != nil || !m.probeAgentCertificateSerial(ctx, activation.serial) {
		return false
	}
	return m.probeAgentReady(ctx)
}

func (m *Manager) waitAgentReady(ctx context.Context) bool {
	waitCtx, cancel := context.WithTimeout(ctx, readyTimeout)
	defer cancel()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		if m.probeAgentReady(waitCtx) {
			return true
		}
		select {
		case <-waitCtx.Done():
			return false
		case <-ticker.C:
		}
	}
}

func (m *Manager) probeAgentReady(ctx context.Context) bool {
	readyURL := m.readyURL
	if readyURL == "" {
		readyURL = AgentReadyURL
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, readyURL, nil)
	if err != nil {
		return false
	}
	client := m.httpClient
	if client == nil {
		client = &http.Client{Timeout: readyTimeout}
	}
	response, err := client.Do(request)
	if err != nil {
		return false
	}
	defer func() { _ = response.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
	return response.StatusCode >= http.StatusOK && response.StatusCode < http.StatusMultipleChoices
}

func (m *Manager) waitAgentCertificateSerial(ctx context.Context, expectedSerial string) bool {
	waitCtx, cancel := context.WithTimeout(ctx, readyTimeout)
	defer cancel()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		if m.probeAgentCertificateSerial(waitCtx, expectedSerial) {
			return true
		}
		select {
		case <-waitCtx.Done():
			return false
		case <-ticker.C:
		}
	}
}

func (m *Manager) probeAgentCertificateSerial(ctx context.Context, expectedSerial string) bool {
	if m.certificateSerialVerifier != nil {
		return m.certificateSerialVerifier(ctx, expectedSerial)
	}
	metricsURL := m.metricsURL
	if metricsURL == "" {
		metricsURL = AgentMetricsURL
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, metricsURL, nil)
	if err != nil {
		return false
	}
	client := m.httpClient
	if client == nil {
		client = &http.Client{Timeout: readyTimeout}
	}
	response, err := client.Do(request)
	if err != nil {
		return false
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		return false
	}
	parser := expfmt.TextParser{}
	families, err := parser.TextToMetricFamilies(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return false
	}
	family := families["kaproxy_mtls_agent_cert_info"]
	if family == nil {
		return false
	}
	for _, metric := range family.Metric {
		if metric.GetGauge().GetValue() != 1 {
			continue
		}
		for _, label := range metric.Label {
			if label.GetName() == "serial" && label.GetValue() == expectedSerial {
				return true
			}
		}
	}
	return false
}

func (m *Manager) verifyCandidateGateway(ctx context.Context, credentials Credentials) error {
	if m.candidateVerifier != nil {
		return m.candidateVerifier(ctx, credentials)
	}
	kubeletCertificatePEM, kubeletPrivateKeyPEM, err := readKubeconfigClientCredentials(m.paths.Kubeconfig)
	if err != nil {
		return err
	}
	endpoint := credentials.GatewayEndpoint
	if m.candidateGatewayAddress != "" {
		endpoint = m.candidateGatewayAddress
	}
	return verifyCandidateGatewayTLS(
		ctx,
		endpoint,
		credentials.ServerName,
		credentials.CertificatePEM,
		credentials.PrivateKeyPEM,
		credentials.GatewayCAPEM,
		kubeletCertificatePEM,
		kubeletPrivateKeyPEM,
	)
}

func verifyCandidateGatewayTLS(
	ctx context.Context,
	endpoint, serverName string,
	agentCertificatePEM, agentPrivateKeyPEM, gatewayCAPEM []byte,
	kubeletCertificatePEM, kubeletPrivateKeyPEM []byte,
) error {
	agentCertificate, err := tls.X509KeyPair(agentCertificatePEM, agentPrivateKeyPEM)
	if err != nil {
		return fmt.Errorf("parse staged agent client certificate: %w", err)
	}
	kubeletCertificate, err := tls.X509KeyPair(kubeletCertificatePEM, kubeletPrivateKeyPEM)
	if err != nil {
		return fmt.Errorf("parse kubelet client certificate: %w", err)
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(gatewayCAPEM) {
		return fmt.Errorf("parse gateway CA bundle")
	}
	rawConnection, err := (&net.Dialer{}).DialContext(ctx, "tcp", endpoint)
	if err != nil {
		return err
	}
	outerConnection := tls.Client(rawConnection, &tls.Config{
		MinVersion:   tls.VersionTLS12,
		ServerName:   serverName,
		RootCAs:      roots,
		Certificates: []tls.Certificate{agentCertificate},
		NextProtos:   []string{gatewayRawTCPALPN},
	})
	defer func() { _ = outerConnection.Close() }()
	if err := outerConnection.HandshakeContext(ctx); err != nil {
		return fmt.Errorf("staged agent gateway TLS handshake: %w", err)
	}
	if protocol := outerConnection.ConnectionState().NegotiatedProtocol; protocol != "" && protocol != gatewayRawTCPALPN {
		return fmt.Errorf("staged agent gateway negotiated unsupported ALPN %q", protocol)
	}
	innerConnection := tls.Client(outerConnection, &tls.Config{
		MinVersion:   tls.VersionTLS12,
		ServerName:   serverName,
		RootCAs:      roots,
		Certificates: []tls.Certificate{kubeletCertificate},
	})
	if err := innerConnection.HandshakeContext(ctx); err != nil {
		return fmt.Errorf("kubelet inner TLS handshake through staged agent gateway connection: %w", err)
	}
	_ = innerConnection.Close()
	return nil
}

func (m *Manager) verifyGateway(ctx context.Context, credentials Credentials) error {
	if m.gatewayVerifier != nil {
		return m.gatewayVerifier(ctx, credentials)
	}
	clientCertificatePEM, clientPrivateKeyPEM, err := readKubeconfigClientCredentials(m.paths.Kubeconfig)
	if err != nil {
		return err
	}
	address := m.localAgentAddress
	if address == "" {
		address = LocalAgentAddress
	}
	return verifyLocalAgentTLS(ctx, address, credentials.ServerName, clientCertificatePEM, clientPrivateKeyPEM, credentials.GatewayCAPEM)
}

func verifyLocalAgentTLS(ctx context.Context, endpoint, serverName string, certificatePEM, privateKeyPEM, gatewayCAPEM []byte) error {
	clientCertificate, err := tls.X509KeyPair(certificatePEM, privateKeyPEM)
	if err != nil {
		return fmt.Errorf("parse client certificate: %w", err)
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(gatewayCAPEM) {
		return fmt.Errorf("parse gateway CA bundle")
	}
	dialer := tls.Dialer{Config: &tls.Config{
		MinVersion:   tls.VersionTLS12,
		ServerName:   serverName,
		RootCAs:      roots,
		Certificates: []tls.Certificate{clientCertificate},
	}}
	connection, err := dialer.DialContext(ctx, "tcp", endpoint)
	if err != nil {
		return err
	}
	_ = connection.Close()
	return nil
}

func (m *Manager) agentVersion() (string, error) {
	path := m.paths.AgentVersionFile
	if path == "" {
		path = filepath.Join(m.paths.StateDir, "version")
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

func validateConfig(config Config) error {
	parsed, err := url.Parse(config.Server)
	if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" || parsed.Path != "" ||
		parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return fmt.Errorf("invalid kubeconfig server %q", config.Server)
	}
	if _, err := parseCertificateBundle(config.CertificateAuthorityData); err != nil {
		return fmt.Errorf("invalid kubeconfig certificate authority data: %w", err)
	}
	if config.Enabled {
		if config.Server != LocalEndpoint {
			return fmt.Errorf("enabled KAP mTLS server must be %q", LocalEndpoint)
		}
		if config.TLSServerName == "" {
			return fmt.Errorf("enabled KAP mTLS config requires a TLS server name")
		}
		return nil
	}
	if parsed.Hostname() == "127.0.0.1" || parsed.Hostname() == "::1" {
		return fmt.Errorf("disabled KAP mTLS config requires a remote server")
	}
	return nil
}

func inspectKubeconfig(path string) (string, string, string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", "", "", fmt.Errorf("read kubelet kubeconfig: %w", err)
	}
	document, cluster, err := parseKubeconfig(data)
	if err != nil {
		return "", "", "", err
	}
	_ = document
	server, _ := cluster["server"].(string)
	tlsServerName, _ := cluster["tls-server-name"].(string)
	if server == "" {
		return "", "", "", fmt.Errorf("kubelet kubeconfig cluster has no server")
	}
	caData, err := decodeKubeconfigData(cluster, "certificate-authority-data")
	if err != nil {
		return "", "", "", err
	}
	certificates, err := parseCertificateBundle(caData)
	if err != nil {
		return "", "", "", fmt.Errorf("parse kubelet kubeconfig certificate authority data: %w", err)
	}
	return server, tlsServerName, certificateBundleFingerprint(certificates), nil
}

func prepareKubeconfig(path string, config Config) ([]byte, []byte, os.FileMode, bool, error) {
	original, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, 0, false, fmt.Errorf("read kubelet kubeconfig: %w", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, nil, 0, false, fmt.Errorf("stat kubelet kubeconfig: %w", err)
	}
	document, cluster, err := parseKubeconfig(original)
	if err != nil {
		return nil, nil, 0, false, err
	}
	currentServer, _ := cluster["server"].(string)
	currentTLSServerName, _ := cluster["tls-server-name"].(string)
	currentCAData, _ := decodeKubeconfigData(cluster, "certificate-authority-data")
	if currentServer == config.Server && currentTLSServerName == config.TLSServerName &&
		bytes.Equal(currentCAData, config.CertificateAuthorityData) {
		return original, nil, info.Mode().Perm(), false, nil
	}
	cluster["server"] = config.Server
	cluster["certificate-authority-data"] = base64.StdEncoding.EncodeToString(config.CertificateAuthorityData)
	if config.TLSServerName == "" {
		delete(cluster, "tls-server-name")
	} else {
		cluster["tls-server-name"] = config.TLSServerName
	}
	updated, err := yaml.Marshal(document)
	if err != nil {
		return nil, nil, 0, false, fmt.Errorf("marshal kubelet kubeconfig: %w", err)
	}
	return original, updated, info.Mode().Perm(), true, nil
}

func decodeKubeconfigData(values map[string]any, key string) ([]byte, error) {
	encoded, _ := values[key].(string)
	if encoded == "" {
		return nil, fmt.Errorf("kubelet kubeconfig has no %s", key)
	}
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("decode kubelet kubeconfig %s: %w", key, err)
	}
	return decoded, nil
}

func readKubeconfigClientCredentials(path string) ([]byte, []byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("read kubelet kubeconfig: %w", err)
	}
	var document map[string]any
	if err := yaml.Unmarshal(data, &document); err != nil {
		return nil, nil, fmt.Errorf("parse kubelet kubeconfig: %w", err)
	}
	users, ok := document["users"].([]any)
	if !ok || len(users) == 0 {
		return nil, nil, fmt.Errorf("kubelet kubeconfig has no users")
	}
	var authInfo map[string]any
	for _, item := range users {
		namedUser, ok := item.(map[string]any)
		if !ok || namedUser["name"] != "kubelet" {
			continue
		}
		authInfo, _ = namedUser["user"].(map[string]any)
		break
	}
	if authInfo == nil && len(users) == 1 {
		if namedUser, ok := users[0].(map[string]any); ok {
			authInfo, _ = namedUser["user"].(map[string]any)
		}
	}
	if authInfo == nil {
		return nil, nil, fmt.Errorf("kubelet kubeconfig user %q was not found", "kubelet")
	}
	certificate, err := readKubeconfigCredentialData(path, authInfo, "client-certificate-data", "client-certificate")
	if err != nil {
		return nil, nil, err
	}
	privateKey, err := readKubeconfigCredentialData(path, authInfo, "client-key-data", "client-key")
	if err != nil {
		return nil, nil, err
	}
	return certificate, privateKey, nil
}

func readKubeconfigCredentialData(kubeconfigPath string, authInfo map[string]any, dataKey, fileKey string) ([]byte, error) {
	if encoded, _ := authInfo[dataKey].(string); encoded != "" {
		decoded, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			return nil, fmt.Errorf("decode kubelet kubeconfig %s: %w", dataKey, err)
		}
		return decoded, nil
	}
	credentialPath, _ := authInfo[fileKey].(string)
	if credentialPath == "" {
		return nil, fmt.Errorf("kubelet kubeconfig has no %s or %s", dataKey, fileKey)
	}
	if !filepath.IsAbs(credentialPath) {
		credentialPath = filepath.Join(filepath.Dir(kubeconfigPath), credentialPath)
	}
	data, err := os.ReadFile(credentialPath)
	if err != nil {
		return nil, fmt.Errorf("read kubelet kubeconfig %s: %w", fileKey, err)
	}
	return data, nil
}

func parseKubeconfig(data []byte) (map[string]any, map[string]any, error) {
	var document map[string]any
	if err := yaml.Unmarshal(data, &document); err != nil {
		return nil, nil, fmt.Errorf("parse kubelet kubeconfig: %w", err)
	}
	clusters, ok := document["clusters"].([]any)
	if !ok || len(clusters) == 0 {
		return nil, nil, fmt.Errorf("kubelet kubeconfig has no clusters")
	}
	for _, item := range clusters {
		namedCluster, ok := item.(map[string]any)
		if !ok || namedCluster["name"] != "kubernetes" {
			continue
		}
		cluster, ok := namedCluster["cluster"].(map[string]any)
		if !ok {
			return nil, nil, fmt.Errorf("kubelet kubeconfig cluster %q is invalid", "kubernetes")
		}
		return document, cluster, nil
	}
	if len(clusters) == 1 {
		namedCluster, ok := clusters[0].(map[string]any)
		if ok {
			if cluster, ok := namedCluster["cluster"].(map[string]any); ok {
				return document, cluster, nil
			}
		}
	}
	return nil, nil, fmt.Errorf("kubelet kubeconfig cluster %q was not found", "kubernetes")
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

func atomicWriteFile(path string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("create directory for %s: %w", path, err)
	}
	temp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-")
	if err != nil {
		return fmt.Errorf("create temporary %s: %w", path, err)
	}
	tempPath := temp.Name()
	defer func() { _ = os.Remove(tempPath) }()
	if err := temp.Chmod(mode); err != nil {
		_ = temp.Close()
		return fmt.Errorf("chmod temporary %s: %w", path, err)
	}
	if _, err := temp.Write(data); err != nil {
		_ = temp.Close()
		return fmt.Errorf("write temporary %s: %w", path, err)
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return fmt.Errorf("sync temporary %s: %w", path, err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close temporary %s: %w", path, err)
	}
	if err := os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("replace %s: %w", path, err)
	}
	return syncDirectory(filepath.Dir(path))
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

func containsExtKeyUsage(values []x509.ExtKeyUsage, target x509.ExtKeyUsage) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
