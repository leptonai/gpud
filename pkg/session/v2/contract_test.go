// SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package sessionv2

import (
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func TestWireContract(t *testing.T) {
	require.Equal(t, "gpud.session.v2", string(File_session_proto.Package()))
	require.Equal(t, "/gpud.session.v2.SessionService/Connect", SessionService_Connect_FullMethodName)
	require.Equal(t, "authorization", MetadataAuthorization)
	require.Equal(t, "x-gpud-machine-id", MetadataMachineID)
	require.Equal(t, "x-gpud-machine-proof", MetadataMachineProof)

	hello := &AgentEnvelope{Payload: &AgentEnvelope_Hello{Hello: &Hello{
		MinProtocolRevision:    1,
		MaxProtocolRevision:    1,
		AgentVersion:           "gpud",
		MaxReceiveMessageBytes: 1024,
		Capabilities:           []string{"heartbeat"},
	}}}
	require.Equal(t, "0a18080110011a04677075642080082a09686561727462656174", marshalWireHex(t, hello))

	command := &ManagerEnvelope{Payload: &ManagerEnvelope_Command{Command: &Command{
		RequestId:     "r1",
		PayloadJson:   []byte(`{}`),
		TimeoutMillis: 1500,
	}}}
	require.Equal(t, "120b0a02723112027b7d18dc0b", marshalWireHex(t, command))
}

func marshalWireHex(t *testing.T, message proto.Message) string {
	t.Helper()
	data, err := proto.MarshalOptions{Deterministic: true}.Marshal(message)
	require.NoError(t, err)
	return hex.EncodeToString(data)
}
