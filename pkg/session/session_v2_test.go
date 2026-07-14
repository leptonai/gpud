// SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package session

import (
	"context"
	"errors"
	"net"
	"net/http"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	sessionv2 "github.com/leptonai/gpud/pkg/session/v2"
)

type testSessionV2Server struct {
	sessionv2.UnimplementedSessionServiceServer
	connect func(sessionv2.SessionService_ConnectServer) error
}

type recordingSessionV2ClientStream struct {
	grpc.ClientStream
	sent *sessionv2.AgentEnvelope
}

func (s *recordingSessionV2ClientStream) Send(envelope *sessionv2.AgentEnvelope) error {
	s.sent = envelope
	return nil
}

func (s *recordingSessionV2ClientStream) Recv() (*sessionv2.ManagerEnvelope, error) {
	return nil, errors.New("not implemented")
}

func (s testSessionV2Server) Connect(stream sessionv2.SessionService_ConnectServer) error {
	return s.connect(stream)
}

func startTestSessionV2Server(t *testing.T, connect func(sessionv2.SessionService_ConnectServer) error) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := grpc.NewServer()
	sessionv2.RegisterSessionServiceServer(server, testSessionV2Server{connect: connect})
	go func() {
		_ = server.Serve(listener)
	}()
	t.Cleanup(func() {
		server.Stop()
		_ = listener.Close()
	})
	return "http://" + listener.Addr().String()
}

func TestRunV2ConnectionMultiplexesCommandAndResult(t *testing.T) {
	serverResult := make(chan *sessionv2.CommandResult, 1)
	endpoint := startTestSessionV2Server(t, func(stream sessionv2.SessionService_ConnectServer) error {
		md, ok := metadata.FromIncomingContext(stream.Context())
		if !ok {
			return status.Error(codes.InvalidArgument, "missing metadata")
		}
		if got := md.Get(sessionv2.MetadataAuthorization); len(got) != 1 || got[0] != "Bearer token" {
			return status.Errorf(codes.Unauthenticated, "authorization = %v", got)
		}
		if got := md.Get(sessionv2.MetadataMachineID); len(got) != 1 || got[0] != "machine-1" {
			return status.Errorf(codes.InvalidArgument, "machine ID = %v", got)
		}
		if got := md.Get(sessionv2.MetadataMachineProof); len(got) != 1 || got[0] != "proof" {
			return status.Errorf(codes.PermissionDenied, "machine proof = %v", got)
		}
		if got := md.Get("origin"); len(got) != 1 || got[0] != "127.0.0.1" {
			return status.Errorf(codes.PermissionDenied, "origin = %v", got)
		}

		first, err := stream.Recv()
		if err != nil {
			return err
		}
		if first.GetHello() == nil {
			return status.Error(codes.FailedPrecondition, "first message is not hello")
		}
		if err := stream.Send(&sessionv2.ManagerEnvelope{Payload: &sessionv2.ManagerEnvelope_HelloAck{HelloAck: &sessionv2.HelloAck{
			ProtocolRevision:         sessionv2.ProtocolRevision,
			HeartbeatIntervalSeconds: 60,
		}}}); err != nil {
			return err
		}
		if err := stream.Send(&sessionv2.ManagerEnvelope{Payload: &sessionv2.ManagerEnvelope_Command{Command: &sessionv2.Command{
			RequestId:   "request-1",
			PayloadJson: []byte(`{"method":"gossip"}`),
		}}}); err != nil {
			return err
		}
		result, err := stream.Recv()
		if err != nil {
			return err
		}
		serverResult <- result.GetCommandResult()
		return status.Error(codes.Unavailable, "test complete")
	})

	s := &Session{
		epControlPlane: endpoint,
		machineID:      "machine-1",
		machineProof:   "proof",
		token:          "token",
		reader:         make(chan Body, 1),
		writer:         make(chan Body, 1),
	}
	done := make(chan reconnectSignal, 1)
	go func() {
		done <- s.runV2Connection(context.Background())
	}()

	select {
	case command := <-s.reader:
		if command.ReqID != "request-1" || string(command.Data) != `{"method":"gossip"}` {
			t.Fatalf("unexpected command: %+v", command)
		}
		s.writer <- Body{ReqID: command.ReqID, Data: []byte(`{"ok":true}`)}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for v2 command")
	}

	select {
	case result := <-serverResult:
		if result == nil || result.RequestId != "request-1" || string(result.PayloadJson) != `{"ok":true}` {
			t.Fatalf("unexpected result: %+v", result)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for v2 result")
	}

	select {
	case sig := <-done:
		if sig.statusCode != http.StatusServiceUnavailable || sig.reason != "test complete" {
			t.Fatalf("unexpected reconnect signal: %+v", sig)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for v2 connection to close")
	}
}

func TestClassifyV2ErrorMarksUnsupportedForAutoFallback(t *testing.T) {
	sig := classifyV2Error(status.Error(codes.Unimplemented, "unknown service"))
	if !errors.Is(sig.err, errV2Unsupported) {
		t.Fatalf("error = %v, want errV2Unsupported", sig.err)
	}
}

func TestReceiveFirstManagerV2MessageTimesOut(t *testing.T) {
	endpoint := startTestSessionV2Server(t, func(stream sessionv2.SessionService_ConnectServer) error {
		if _, err := stream.Recv(); err != nil {
			return err
		}
		<-stream.Context().Done()
		return stream.Context().Err()
	})
	conn, err := newV2ClientConn(endpoint)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stream, err := sessionv2.NewSessionServiceClient(conn).Connect(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := stream.Send(&sessionv2.AgentEnvelope{Payload: &sessionv2.AgentEnvelope_Hello{Hello: &sessionv2.Hello{}}}); err != nil {
		t.Fatal(err)
	}

	_, err = receiveFirstManagerV2Message(ctx, stream, 20*time.Millisecond)
	if status.Code(err) != codes.DeadlineExceeded {
		t.Fatalf("status = %v, want DeadlineExceeded", status.Code(err))
	}
}

func TestMonitorV2HeartbeatTimesOutWithoutAcknowledgement(t *testing.T) {
	sent := make(chan uint64, 1)
	acks := make(chan uint64, 1)
	sent <- 1

	started := time.Now()
	err := monitorV2Heartbeat(context.Background(), 10*time.Millisecond, sent, acks)
	if err == nil || time.Since(started) > time.Second {
		t.Fatalf("monitorV2Heartbeat error = %v, elapsed = %s", err, time.Since(started))
	}
}

func TestSendAgentV2EnvelopeEnforcesPeerLimit(t *testing.T) {
	stream := &recordingSessionV2ClientStream{}
	envelope := &sessionv2.AgentEnvelope{Payload: &sessionv2.AgentEnvelope_Heartbeat{Heartbeat: &sessionv2.Heartbeat{Sequence: 1}}}

	err := sendAgentV2Envelope(stream, envelope, 1)
	if status.Code(err) != codes.ResourceExhausted {
		t.Fatalf("status = %v, want ResourceExhausted", status.Code(err))
	}
	if stream.sent != nil {
		t.Fatal("oversized envelope was sent")
	}

	if err := sendAgentV2Envelope(stream, envelope, sessionv2.DefaultMaxMessageBytes); err != nil {
		t.Fatal(err)
	}
	if stream.sent != envelope {
		t.Fatal("valid envelope was not sent")
	}
}

func TestNegotiatedV2MessageLimit(t *testing.T) {
	for _, tt := range []struct {
		advertised uint32
		want       int
	}{
		{advertised: 1024, want: 1024},
		{advertised: 0, want: sessionv2.DefaultMaxMessageBytes},
		{advertised: sessionv2.DefaultMaxMessageBytes + 1, want: sessionv2.DefaultMaxMessageBytes},
	} {
		if got := negotiatedV2MessageLimit(tt.advertised); got != tt.want {
			t.Fatalf("negotiatedV2MessageLimit(%d) = %d, want %d", tt.advertised, got, tt.want)
		}
	}
}

func TestParseV2EndpointAddsDefaultPort(t *testing.T) {
	tests := []struct {
		name     string
		endpoint string
		target   string
		wantErr  bool
	}{
		{name: "https", endpoint: "https://gpud-gateway.example.com", target: "gpud-gateway.example.com:443"},
		{name: "explicit port", endpoint: "https://gpud-gateway.example.com:8443", target: "gpud-gateway.example.com:8443"},
		{name: "http ipv6", endpoint: "http://[::1]", target: "[::1]:80"},
		{name: "missing scheme", endpoint: "gpud-gateway.example.com", wantErr: true},
		{name: "userinfo", endpoint: "https://user@gpud-gateway.example.com", wantErr: true},
		{name: "base path", endpoint: "https://gpud-gateway.example.com/base", wantErr: true},
		{name: "query", endpoint: "https://gpud-gateway.example.com?x=1", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, target, err := parseV2Endpoint(tt.endpoint)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("parseV2Endpoint(%q) succeeded, want error", tt.endpoint)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseV2Endpoint(%q) error = %v", tt.endpoint, err)
			}
			if target != tt.target {
				t.Fatalf("parseV2Endpoint(%q) target = %q, want %q", tt.endpoint, target, tt.target)
			}
		})
	}
}
