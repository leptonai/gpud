// SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package session

import "fmt"

// Protocol selects the transport used for the control-plane session.
type Protocol string

const (
	ProtocolV1   Protocol = "v1"
	ProtocolV2   Protocol = "v2"
	ProtocolAuto Protocol = "auto"
)

func parseProtocol(value string) (Protocol, error) {
	protocol := Protocol(value)
	if protocol == "" {
		protocol = ProtocolAuto
	}
	switch protocol {
	case ProtocolV1, ProtocolV2, ProtocolAuto:
		return protocol, nil
	default:
		return "", fmt.Errorf("unsupported session protocol %q", value)
	}
}
