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

	hello := &AgentPacket{Payload: &AgentPacket_Hello{Hello: &Hello{
		MinProtocolRevision:    1,
		MaxProtocolRevision:    1,
		AgentVersion:           "gpud",
		MaxReceiveMessageBytes: 1024,
		Capabilities:           []string{"typed-requests"},
	}}}
	require.Equal(t, "0a1d080110011a04677075642080082a0e74797065642d7265717565737473", marshalWireHex(t, hello))

	request := &ManagerPacket{
		RequestId: "r1",
		Payload:   &ManagerPacket_GetHealthStates{GetHealthStates: &GetHealthStatesRequest{}},
	}
	require.Equal(t, "220272315200", marshalWireHex(t, request))
}

func marshalWireHex(t *testing.T, message proto.Message) string {
	t.Helper()
	data, err := proto.MarshalOptions{Deterministic: true}.Marshal(message)
	require.NoError(t, err)
	return hex.EncodeToString(data)
}
