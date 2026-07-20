// SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package session

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/cookiejar"
	"testing"
	"time"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/durationpb"

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

type scriptedSessionV2ClientStream struct {
	grpc.ClientStream
	send func(*sessionv2.AgentEnvelope) error
	recv func() (*sessionv2.ManagerEnvelope, error)
}

func (s *recordingSessionV2ClientStream) Send(envelope *sessionv2.AgentEnvelope) error {
	s.sent = envelope
	return nil
}

func (s *recordingSessionV2ClientStream) Recv() (*sessionv2.ManagerEnvelope, error) {
	return nil, errors.New("not implemented")
}

func (s *scriptedSessionV2ClientStream) Send(envelope *sessionv2.AgentEnvelope) error {
	return s.send(envelope)
}

func (s *scriptedSessionV2ClientStream) Recv() (*sessionv2.ManagerEnvelope, error) {
	return s.recv()
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
	serverResult := make(chan *sessionv2.Response, 1)
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
			ProtocolRevision: sessionv2.ProtocolRevision,
		}}}); err != nil {
			return err
		}
		if err := stream.Send(&sessionv2.ManagerEnvelope{Payload: &sessionv2.ManagerEnvelope_Request{Request: &sessionv2.Request{
			RequestId: "request-1",
			Command:   &sessionv2.Request_Gossip{Gossip: &sessionv2.GossipCommand{}},
		}}}); err != nil {
			return err
		}
		result, err := stream.Recv()
		if err != nil {
			return err
		}
		serverResult <- result.GetResponse()
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
		var request Request
		if err := json.Unmarshal(command.Data, &request); err != nil {
			t.Fatal(err)
		}
		if command.ReqID != "request-1" || request.Method != "gossip" {
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
	tests := []struct {
		name string
		err  error
	}{
		{name: "unimplemented service", err: status.Error(codes.Unimplemented, "unknown service")},
		{
			name: "legacy envoy gateway",
			err: status.Error(codes.Unknown,
				"unexpected HTTP status code received from server: 464 (); malformed header: missing HTTP content-type"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sig := classifyV2Error(tt.err)
			if !errors.Is(sig.err, errV2Unsupported) {
				t.Fatalf("error = %v, want errV2Unsupported", sig.err)
			}
		})
	}
}

func TestClassifyV2Error(t *testing.T) {
	retryStatus, err := status.New(codes.Unavailable, "retry later").WithDetails(&errdetails.RetryInfo{
		RetryDelay: durationpb.New(3 * time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name        string
		err         error
		statusCode  int
		authFailure bool
		retryAfter  time.Duration
	}{
		{name: "nil"},
		{name: "plain", err: errors.New("plain")},
		{name: "resource exhausted", err: status.Error(codes.ResourceExhausted, "full"), statusCode: http.StatusTooManyRequests},
		{name: "unavailable with retry", err: retryStatus.Err(), statusCode: http.StatusServiceUnavailable, retryAfter: 3 * time.Second},
		{name: "unauthenticated", err: status.Error(codes.Unauthenticated, "bad token"), statusCode: http.StatusUnauthorized, authFailure: true},
		{name: "permission denied", err: status.Error(codes.PermissionDenied, "wrong owner"), statusCode: http.StatusForbidden, authFailure: true},
		{name: "unknown non-gateway error", err: status.Error(codes.Unknown, "upstream reset")},
		{name: "http 464 without missing content type marker", err: status.Error(codes.Unknown, "unexpected HTTP status code received from server: 464")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sig := classifyV2Error(tt.err)
			if sig.side != reconnectSideSingle || sig.statusCode != tt.statusCode || sig.authFailure != tt.authFailure || sig.retryAfter != tt.retryAfter {
				t.Fatalf("classifyV2Error(%v) = %+v", tt.err, sig)
			}
			if tt.err != nil && sig.reason == "" {
				t.Fatal("classified error is missing a reason")
			}
		})
	}
}

func TestKeepAliveV2RetriesAndFallsBack(t *testing.T) {
	t.Run("context cancellation during connection stops retries", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		endpoint := startTestSessionV2Server(t, func(stream sessionv2.SessionService_ConnectServer) error {
			if _, err := stream.Recv(); err != nil {
				return err
			}
			cancel()
			return status.Error(codes.Unavailable, "connection canceled")
		})
		s := &Session{
			ctx:            ctx,
			epControlPlane: endpoint,
			reader:         make(chan Body, 1),
			writer:         make(chan Body, 1),
			jitterFunc:     func(time.Duration) time.Duration { return 0 },
		}
		s.keepAliveV2(false)
		if !errors.Is(ctx.Err(), context.Canceled) {
			t.Fatalf("context error = %v", ctx.Err())
		}
	})

	t.Run("retry exits when reconnect wait is canceled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		endpoint := startTestSessionV2Server(t, func(sessionv2.SessionService_ConnectServer) error {
			return status.Error(codes.Unavailable, "try again")
		})
		nowCalls := 0
		s := &Session{
			ctx:            ctx,
			epControlPlane: endpoint,
			reader:         make(chan Body, 1),
			writer:         make(chan Body, 1),
			jitterFunc: func(max time.Duration) time.Duration {
				if max == startupJitterMax {
					return 0
				}
				return time.Millisecond
			},
			timeAfterFunc: func(time.Duration) <-chan time.Time {
				cancel()
				return make(chan time.Time)
			},
			nowFunc: func() time.Time {
				nowCalls++
				if nowCalls == 1 {
					return time.Unix(0, 0)
				}
				return time.Unix(0, 0).Add(reconnectStableWindow)
			},
		}
		s.keepAliveV2(false)
		if ctx.Err() == nil {
			t.Fatal("reconnect wait did not cancel the session context")
		}
	})

	t.Run("auto falls back only on unimplemented", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		endpoint := startTestSessionV2Server(t, func(sessionv2.SessionService_ConnectServer) error {
			return status.Error(codes.Unimplemented, "v2 unavailable")
		})
		s := &Session{
			ctx:            ctx,
			epControlPlane: endpoint,
			reader:         make(chan Body, 1),
			writer:         make(chan Body, 1),
			jitterFunc:     func(time.Duration) time.Duration { return 0 },
			checkServerHealthFunc: func(context.Context, *cookiejar.Jar, string) error {
				cancel()
				return errors.New("stop legacy fallback")
			},
		}
		s.keepAliveV2(true)
	})
}

func TestKeepAliveSelectsV2Protocols(t *testing.T) {
	for _, protocol := range []Protocol{ProtocolV2, ProtocolAuto} {
		t.Run(string(protocol), func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			s := &Session{ctx: ctx, protocol: protocol}
			s.keepAlive()
		})
	}
}

func TestRunV2ConnectionRejectsInvalidManagerMessages(t *testing.T) {
	tests := []struct {
		name       string
		first      *sessionv2.ManagerEnvelope
		followup   *sessionv2.ManagerEnvelope
		reader     chan Body
		statusCode int
		reason     string
	}{
		{name: "invalid hello acknowledgement", first: &sessionv2.ManagerEnvelope{}, reason: "invalid v2 hello acknowledgement"},
		{name: "missing request id", followup: &sessionv2.ManagerEnvelope{Payload: &sessionv2.ManagerEnvelope_Request{Request: &sessionv2.Request{}}}, reason: "invalid v2 request"},
		{name: "full command queue", followup: &sessionv2.ManagerEnvelope{Payload: &sessionv2.ManagerEnvelope_Request{Request: &sessionv2.Request{RequestId: "r1", Command: &sessionv2.Request_Gossip{Gossip: &sessionv2.GossipCommand{}}}}}, reader: make(chan Body), statusCode: http.StatusTooManyRequests, reason: "agent command queue is full"},
		{name: "drain notice", followup: &sessionv2.ManagerEnvelope{Payload: &sessionv2.ManagerEnvelope_DrainNotice{DrainNotice: &sessionv2.DrainNotice{ReconnectAfterMillis: 250}}}, statusCode: http.StatusServiceUnavailable, reason: "manager draining"},
		{name: "unexpected message", followup: &sessionv2.ManagerEnvelope{Payload: &sessionv2.ManagerEnvelope_HelloAck{HelloAck: &sessionv2.HelloAck{ProtocolRevision: sessionv2.ProtocolRevision}}}, reason: "unexpected v2 manager message"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			endpoint := startTestSessionV2Server(t, func(stream sessionv2.SessionService_ConnectServer) error {
				if _, err := stream.Recv(); err != nil {
					return err
				}
				first := tt.first
				if first == nil {
					first = &sessionv2.ManagerEnvelope{Payload: &sessionv2.ManagerEnvelope_HelloAck{HelloAck: &sessionv2.HelloAck{
						ProtocolRevision: sessionv2.ProtocolRevision,
					}}}
				}
				if err := stream.Send(first); err != nil {
					return err
				}
				if tt.followup != nil {
					if err := stream.Send(tt.followup); err != nil {
						return err
					}
				}
				<-stream.Context().Done()
				return stream.Context().Err()
			})
			reader := tt.reader
			if reader == nil {
				reader = make(chan Body, 1)
			}
			s := &Session{epControlPlane: endpoint, reader: reader, writer: make(chan Body, 1)}
			sig := s.runV2Connection(ctx)
			if sig.statusCode != tt.statusCode || sig.reason != tt.reason {
				t.Fatalf("runV2Connection() = %+v", sig)
			}
			if tt.name == "drain notice" && sig.retryAfter != 250*time.Millisecond {
				t.Fatalf("retryAfter = %s", sig.retryAfter)
			}
		})
	}
}

func TestRunV2ConnectionReturnsTransportFailures(t *testing.T) {
	t.Run("invalid endpoint", func(t *testing.T) {
		s := &Session{epControlPlane: "%", reader: make(chan Body, 1), writer: make(chan Body, 1)}
		sig := s.runV2Connection(context.Background())
		if sig.reason == "" || sig.err == nil {
			t.Fatalf("runV2Connection() = %+v", sig)
		}
	})

	t.Run("closed response queue", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		endpoint := startTestSessionV2Server(t, func(stream sessionv2.SessionService_ConnectServer) error {
			if _, err := stream.Recv(); err != nil {
				return err
			}
			if err := stream.Send(&sessionv2.ManagerEnvelope{Payload: &sessionv2.ManagerEnvelope_HelloAck{HelloAck: &sessionv2.HelloAck{
				ProtocolRevision: sessionv2.ProtocolRevision,
			}}}); err != nil {
				return err
			}
			<-stream.Context().Done()
			return stream.Context().Err()
		})
		writer := make(chan Body)
		close(writer)
		s := &Session{epControlPlane: endpoint, reader: make(chan Body, 1), writer: writer}
		sig := s.runV2Connection(ctx)
		if sig.reason != "session response queue closed" {
			t.Fatalf("runV2Connection() = %+v", sig)
		}
	})
}

func TestSendV2Messages(t *testing.T) {
	t.Run("canceled context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		s := &Session{writer: make(chan Body)}
		stream := &scriptedSessionV2ClientStream{send: func(*sessionv2.AgentEnvelope) error { return nil }}
		if err := s.sendV2Messages(ctx, stream, sessionv2.DefaultMaxMessageBytes); !errors.Is(err, context.Canceled) {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("closed response queue", func(t *testing.T) {
		writer := make(chan Body)
		close(writer)
		s := &Session{writer: writer}
		stream := &scriptedSessionV2ClientStream{send: func(*sessionv2.AgentEnvelope) error { return nil }}
		if err := s.sendV2Messages(context.Background(), stream, sessionv2.DefaultMaxMessageBytes); err == nil {
			t.Fatal("closed response queue did not fail")
		}
	})

	t.Run("response is sent", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		s := &Session{writer: make(chan Body, 1)}
		s.writer <- Body{ReqID: "r1", Data: []byte(`{}`)}
		stream := &scriptedSessionV2ClientStream{send: func(envelope *sessionv2.AgentEnvelope) error {
			if envelope.GetResponse().GetRequestId() != "r1" {
				t.Fatalf("unexpected envelope: %v", envelope)
			}
			cancel()
			return nil
		}}
		if err := s.sendV2Messages(ctx, stream, sessionv2.DefaultMaxMessageBytes); !errors.Is(err, context.Canceled) {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("response send error", func(t *testing.T) {
		wantErr := errors.New("send failed")
		s := &Session{writer: make(chan Body, 1)}
		s.writer <- Body{ReqID: "r1"}
		stream := &scriptedSessionV2ClientStream{send: func(*sessionv2.AgentEnvelope) error { return wantErr }}
		if err := s.sendV2Messages(context.Background(), stream, sessionv2.DefaultMaxMessageBytes); !errors.Is(err, wantErr) {
			t.Fatalf("error = %v", err)
		}
	})
}

func TestNewV2ClientConn(t *testing.T) {
	for _, endpoint := range []string{"http://127.0.0.1", "https://gpud-gateway.example.com"} {
		conn, err := newV2ClientConn(endpoint)
		if err != nil {
			t.Fatalf("newV2ClientConn(%q) error = %v", endpoint, err)
		}
		_ = conn.Close()
	}
	for _, endpoint := range []string{"%", "ftp://gpud-gateway.example.com:21"} {
		if conn, err := newV2ClientConn(endpoint); err == nil {
			_ = conn.Close()
			t.Fatalf("newV2ClientConn(%q) succeeded", endpoint)
		}
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

func TestSendAgentV2EnvelopeEnforcesPeerLimit(t *testing.T) {
	stream := &recordingSessionV2ClientStream{}
	envelope := &sessionv2.AgentEnvelope{Payload: &sessionv2.AgentEnvelope_Response{Response: &sessionv2.Response{RequestId: "r1"}}}

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
		{name: "fragment", endpoint: "https://gpud-gateway.example.com#fragment", wantErr: true},
		{name: "unsupported scheme", endpoint: "ftp://gpud-gateway.example.com", wantErr: true},
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
