// SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package session

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	pkgkapmtls "github.com/leptonai/gpud/pkg/kapmtls"
)

type kapMTLSManager interface {
	Status(context.Context, string) (*pkgkapmtls.Status, error)
	UpdateCredentials(context.Context, string, pkgkapmtls.Credentials) error
	Activate(context.Context) error
}

func (s *Session) processActivateKAPMTLS(ctx context.Context, response *Response) {
	if err := s.getKAPMTLSManager().Activate(ctx); err != nil {
		response.Error = err.Error()
	}
}

func (s *Session) getKAPMTLSManager() kapMTLSManager {
	if s.kapMTLSManager == nil {
		s.kapMTLSManager = pkgkapmtls.NewManager(pkgkapmtls.DefaultPaths(s.dataDir))
	}
	return s.kapMTLSManager
}

func (s *Session) processKAPMTLSStatus(ctx context.Context, response *Response) {
	status, err := s.getKAPMTLSManager().Status(ctx, s.machineID)
	if err != nil {
		response.Error = err.Error()
		return
	}
	response.KAPMTLSStatus = &KAPMTLSStatus{
		CredentialsInstalled: status.CredentialsInstalled,
		CertificateSerial:    status.CertificateSerial,
		CertificateNotAfter:  status.CertificateNotAfter,
		AgentInstalled:       status.AgentInstalled,
		AgentActive:          status.AgentActive,
		AgentReady:           status.AgentReady,
		AgentVersion:         status.AgentVersion,
		GatewayEndpoint:      status.GatewayEndpoint,
		ServerName:           status.ServerName,
		ClientCAFingerprint:  status.ClientCAFingerprint,
		GatewayCAFingerprint: status.GatewayCAFingerprint,
	}
}

func (s *Session) processUpdateKAPMTLSCredentials(ctx context.Context, request Request, response *Response) {
	if request.KAPMTLSCredentials == nil {
		response.Error = "KAP mTLS credentials are required"
		return
	}
	credentials := request.KAPMTLSCredentials
	if err := s.getKAPMTLSManager().UpdateCredentials(ctx, s.machineID, pkgkapmtls.Credentials{
		CertificatePEM:       credentials.CertificatePEM,
		PrivateKeyPEM:        credentials.PrivateKeyPEM,
		GatewayCAPEM:         credentials.GatewayCAPEM,
		GatewayEndpoint:      credentials.GatewayEndpoint,
		ServerName:           credentials.ServerName,
		ClientCAFingerprint:  credentials.ClientCAFingerprint,
		GatewayCAFingerprint: credentials.GatewayCAFingerprint,
	}); err != nil {
		response.Error = err.Error()
	}
}

// auditSessionRequestData returns a structured copy with credential material
// removed. It is used before and after typed decoding so neither audit stage can
// emit a private key.
func auditSessionRequestData(data []byte) any {
	var value any
	if err := json.Unmarshal(data, &value); err != nil {
		return fmt.Sprintf("<invalid session request: %v>", err)
	}
	redactSessionCredentials(value)
	return value
}

func redactSessionCredentials(value any) {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if strings.EqualFold(key, "token") {
				typed[key] = "<redacted>"
				continue
			}
			if strings.EqualFold(key, "kap_mtls_credentials") {
				credentials, ok := child.(map[string]any)
				if !ok {
					typed[key] = "<redacted>"
					continue
				}
				for credentialKey := range credentials {
					if strings.EqualFold(credentialKey, "certificate_pem") || strings.EqualFold(credentialKey, "private_key_pem") {
						credentials[credentialKey] = "<redacted>"
					}
				}
				continue
			}
			redactSessionCredentials(child)
		}
	case []any:
		for _, child := range typed {
			redactSessionCredentials(child)
		}
	}
}
