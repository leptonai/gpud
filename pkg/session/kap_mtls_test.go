// SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package session

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pkgkapmtls "github.com/leptonai/gpud/pkg/kapmtls"
)

type fakeKAPMTLSManager struct {
	status            *pkgkapmtls.Status
	err               error
	machineID         string
	credentials       pkgkapmtls.Credentials
	config            pkgkapmtls.Config
	credentialsCalled bool
	configureCalled   bool
}

func (m *fakeKAPMTLSManager) Status(_ context.Context, machineID string) (*pkgkapmtls.Status, error) {
	m.machineID = machineID
	return m.status, m.err
}

func (m *fakeKAPMTLSManager) UpdateCredentials(_ context.Context, machineID string, credentials pkgkapmtls.Credentials) error {
	m.machineID = machineID
	m.credentials = credentials
	m.credentialsCalled = true
	return m.err
}

func (m *fakeKAPMTLSManager) Configure(_ context.Context, config pkgkapmtls.Config) error {
	m.config = config
	m.configureCalled = true
	return m.err
}

func TestKAPMTLSSessionCommands(t *testing.T) {
	now := time.Date(2026, 7, 6, 0, 0, 0, 0, time.UTC)
	manager := &fakeKAPMTLSManager{status: &pkgkapmtls.Status{
		CredentialsInstalled:    true,
		CertificateSerial:       "abc",
		CertificateNotAfter:     now,
		AgentInstalled:          true,
		AgentActive:             true,
		AgentReady:              true,
		AgentVersion:            "v0.3.7",
		ClientCAFingerprint:     "client-fingerprint",
		GatewayCAFingerprint:    "gateway-fingerprint",
		KubeconfigServer:        pkgkapmtls.LocalEndpoint,
		KubeconfigCAFingerprint: "kubeconfig-ca-fingerprint",
		KubeconfigPending:       true,
	}}
	session := &Session{machineID: "machine-a", kapMTLSManager: manager}
	restartExitCode := -1

	statusResponse := &Response{}
	session.processRequest(context.Background(), "status", Request{Method: "kapMTLSStatus"}, statusResponse, &restartExitCode)
	require.NotNil(t, statusResponse.KAPMTLSStatus)
	assert.Equal(t, "machine-a", manager.machineID)
	assert.Equal(t, "abc", statusResponse.KAPMTLSStatus.CertificateSerial)
	assert.True(t, statusResponse.KAPMTLSStatus.AgentReady)
	assert.Equal(t, "v0.3.7", statusResponse.KAPMTLSStatus.AgentVersion)
	assert.Equal(t, "kubeconfig-ca-fingerprint", statusResponse.KAPMTLSStatus.KubeconfigCAFingerprint)
	assert.True(t, statusResponse.KAPMTLSStatus.KubeconfigPending)

	credentialsResponse := &Response{}
	session.processRequest(context.Background(), "credentials", Request{
		Method: "updateKAPMTLSCredentials",
		KAPMTLSCredentials: &KAPMTLSCredentialsRequest{
			CertificatePEM:       []byte("certificate"),
			PrivateKeyPEM:        []byte("private-key"),
			GatewayCAPEM:         []byte("gateway-ca"),
			GatewayEndpoint:      "kap.example.test:8443",
			ServerName:           "kap.example.test",
			ClientCAFingerprint:  "client-fingerprint",
			GatewayCAFingerprint: "gateway-fingerprint",
		},
	}, credentialsResponse, &restartExitCode)
	assert.Empty(t, credentialsResponse.Error)
	assert.True(t, manager.credentialsCalled)
	assert.Equal(t, []byte("private-key"), manager.credentials.PrivateKeyPEM)
	assert.Equal(t, []byte("gateway-ca"), manager.credentials.GatewayCAPEM)
	assert.Equal(t, "client-fingerprint", manager.credentials.ClientCAFingerprint)

	configResponse := &Response{}
	session.processRequest(context.Background(), "config", Request{
		Method: "configureKAPMTLS",
		KAPMTLSConfig: &KAPMTLSConfigRequest{
			Enabled:                  true,
			Server:                   pkgkapmtls.LocalEndpoint,
			TLSServerName:            "kap.example.test",
			CertificateAuthorityData: []byte("kubeconfig-ca"),
		},
	}, configResponse, &restartExitCode)
	assert.Empty(t, configResponse.Error)
	assert.True(t, manager.configureCalled)
	assert.Equal(t, pkgkapmtls.LocalEndpoint, manager.config.Server)
	assert.Equal(t, []byte("kubeconfig-ca"), manager.config.CertificateAuthorityData)
}

func TestKAPMTLSSessionCommandErrors(t *testing.T) {
	session := &Session{kapMTLSManager: &fakeKAPMTLSManager{err: errors.New("manager failed")}}
	restartExitCode := -1

	response := &Response{}
	session.processRequest(context.Background(), "status", Request{Method: "kapMTLSStatus"}, response, &restartExitCode)
	assert.Equal(t, "manager failed", response.Error)

	response = &Response{}
	session.processRequest(context.Background(), "credentials", Request{Method: "updateKAPMTLSCredentials"}, response, &restartExitCode)
	assert.Equal(t, "KAP mTLS credentials are required", response.Error)

	response = &Response{}
	session.processRequest(context.Background(), "config", Request{Method: "configureKAPMTLS"}, response, &restartExitCode)
	assert.Equal(t, "KAP mTLS config is required", response.Error)
}

func TestAuditSessionRequestDataRedactsCredentials(t *testing.T) {
	certificate := []byte("certificate-secret")
	privateKey := []byte("private-key-secret")
	raw, err := json.Marshal(Request{
		Method: "updateKAPMTLSCredentials",
		KAPMTLSCredentials: &KAPMTLSCredentialsRequest{
			CertificatePEM:  certificate,
			PrivateKeyPEM:   privateKey,
			GatewayEndpoint: "kap.example.test:8443",
			ServerName:      "kap.example.test",
		},
	})
	require.NoError(t, err)

	redacted, err := json.Marshal(auditSessionRequestData(raw))
	require.NoError(t, err)
	assert.NotContains(t, string(redacted), base64.StdEncoding.EncodeToString(certificate))
	assert.NotContains(t, string(redacted), base64.StdEncoding.EncodeToString(privateKey))
	assert.Contains(t, string(redacted), "kap.example.test:8443")
	assert.Contains(t, string(redacted), "redacted")
}

func TestAuditSessionRequestDataRedactsCaseVariants(t *testing.T) {
	raw := []byte(`{"KAP_MTLS_CREDENTIALS":{"Certificate_PEM":"certificate-one","certificate_pem":"certificate-two","PRIVATE_KEY_PEM":"key-one","private_key_pem":"key-two","gateway_endpoint":"kap.example.test:8443"}}`)

	redacted, err := json.Marshal(auditSessionRequestData(raw))
	require.NoError(t, err)
	for _, secret := range []string{"certificate-one", "certificate-two", "key-one", "key-two"} {
		assert.NotContains(t, string(redacted), secret)
	}
	assert.Contains(t, string(redacted), "kap.example.test:8443")
}

func TestKAPMTLSWireTypesRoundTrip(t *testing.T) {
	request := Request{
		Method: "updateKAPMTLSCredentials",
		KAPMTLSCredentials: &KAPMTLSCredentialsRequest{
			CertificatePEM:       []byte("certificate"),
			PrivateKeyPEM:        []byte("private-key"),
			GatewayCAPEM:         []byte("gateway-ca"),
			GatewayEndpoint:      "kap.example.test:8443",
			ServerName:           "kap.example.test",
			ClientCAFingerprint:  "client-fingerprint",
			GatewayCAFingerprint: "gateway-fingerprint",
		},
	}
	data, err := json.Marshal(request)
	require.NoError(t, err)
	var decoded Request
	require.NoError(t, json.Unmarshal(data, &decoded))
	require.NotNil(t, decoded.KAPMTLSCredentials)
	assert.Equal(t, request.KAPMTLSCredentials.PrivateKeyPEM, decoded.KAPMTLSCredentials.PrivateKeyPEM)
	assert.Equal(t, request.KAPMTLSCredentials.GatewayCAPEM, decoded.KAPMTLSCredentials.GatewayCAPEM)
	assert.Equal(t, request.KAPMTLSCredentials.GatewayCAFingerprint, decoded.KAPMTLSCredentials.GatewayCAFingerprint)

	configRequest := Request{
		Method: "configureKAPMTLS",
		KAPMTLSConfig: &KAPMTLSConfigRequest{
			Enabled:                  true,
			Server:                   pkgkapmtls.LocalEndpoint,
			TLSServerName:            "kap.example.test",
			CertificateAuthorityData: []byte("kubeconfig-ca"),
		},
	}
	data, err = json.Marshal(configRequest)
	require.NoError(t, err)
	decoded = Request{}
	require.NoError(t, json.Unmarshal(data, &decoded))
	require.NotNil(t, decoded.KAPMTLSConfig)
	assert.Equal(t, configRequest.KAPMTLSConfig.CertificateAuthorityData, decoded.KAPMTLSConfig.CertificateAuthorityData)
}
