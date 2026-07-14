// SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package session

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	"github.com/leptonai/gpud/pkg/log"
	sessionv2 "github.com/leptonai/gpud/pkg/session/v2"
	"github.com/leptonai/gpud/version"
)

const reconnectSideSingle = "single"

const v2HandshakeTimeout = 30 * time.Second

var errV2Unsupported = errors.New("single-connection session protocol is unsupported")

type managerV2ReceiveResult struct {
	envelope *sessionv2.ManagerEnvelope
	err      error
}

func (s *Session) keepAliveV2(allowLegacyFallback bool) {
	backoff := reconnectBackoff{}
	if !s.waitReconnectDelay(s.ctx, s.jitter(startupJitterMax)) {
		return
	}

	for {
		if s.ctx.Err() != nil {
			return
		}
		s.drainReaderChannel()
		connectedAt := s.now()
		sig := s.runV2Connection(s.ctx)
		if allowLegacyFallback && errors.Is(sig.err, errV2Unsupported) {
			log.Logger.Infow("single-connection session unsupported; falling back to legacy session")
			s.keepAliveLegacy()
			return
		}
		if s.ctx.Err() != nil {
			return
		}
		if sig.authFailure {
			s.persistLoginStatus(s.ctx, false, sig.reason)
		}
		if s.now().Sub(connectedAt) >= reconnectStableWindow {
			backoff.reset()
		}
		delay := backoff.nextDelay(s, sig)
		log.Logger.Debugw("single-connection session reconnect scheduled", "delay", delay.String(), "reason", sig.reason, "retryAfter", sig.retryAfter.String())
		if !s.waitReconnectDelay(s.ctx, delay) {
			return
		}
	}
}

func (s *Session) runV2Connection(ctx context.Context) reconnectSignal {
	connectionCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	releaseConnection := s.registerConnectionCancel(cancel)
	defer releaseConnection()

	conn, err := newV2ClientConn(s.epControlPlane)
	if err != nil {
		return classifyV2Error(err)
	}
	defer func() {
		_ = conn.Close()
	}()
	origin, err := controlPlaneOrigin(s.epControlPlane)
	if err != nil {
		return classifyV2Error(err)
	}

	outgoing := metadata.Pairs(
		sessionv2.MetadataAuthorization, "Bearer "+s.getToken(),
		sessionv2.MetadataMachineID, s.machineID,
		"origin", origin,
	)
	if s.machineProof != "" {
		outgoing.Append(sessionv2.MetadataMachineProof, s.machineProof)
	}
	stream, err := sessionv2.NewSessionServiceClient(conn).Connect(metadata.NewOutgoingContext(connectionCtx, outgoing))
	if err != nil {
		return classifyV2Error(err)
	}
	defer func() {
		_ = stream.CloseSend()
	}()

	hello := &sessionv2.AgentEnvelope{Payload: &sessionv2.AgentEnvelope_Hello{Hello: &sessionv2.Hello{
		MinProtocolRevision:    sessionv2.ProtocolRevision,
		MaxProtocolRevision:    sessionv2.ProtocolRevision,
		AgentVersion:           version.Version,
		MaxReceiveMessageBytes: sessionv2.DefaultMaxMessageBytes,
		Capabilities:           []string{"command-json", "heartbeat"},
	}}}
	if err := stream.Send(hello); err != nil {
		return classifyV2Error(err)
	}

	first, err := receiveFirstManagerV2Message(connectionCtx, stream, v2HandshakeTimeout)
	if err != nil {
		return classifyV2Error(err)
	}
	ack := first.GetHelloAck()
	if ack == nil || ack.ProtocolRevision != sessionv2.ProtocolRevision {
		return reconnectSignal{side: reconnectSideSingle, reason: "invalid v2 hello acknowledgement", err: errors.New("invalid v2 hello acknowledgement")}
	}

	heartbeatInterval := time.Duration(ack.HeartbeatIntervalSeconds) * time.Second
	if heartbeatInterval <= 0 {
		heartbeatInterval = sessionv2.DefaultHeartbeatIntervalSecs * time.Second
	}
	maxManagerReceiveBytes := negotiatedV2MessageLimit(ack.MaxReceiveMessageBytes)
	heartbeatSent := make(chan uint64, 1)
	heartbeatAcks := make(chan uint64, 1)
	sendErr := make(chan error, 1)
	go func() {
		err := s.sendV2Messages(connectionCtx, stream, heartbeatInterval, maxManagerReceiveBytes, heartbeatSent)
		sendErr <- err
		cancel()
	}()
	watchdogErr := make(chan error, 1)
	go func() {
		err := monitorV2Heartbeat(connectionCtx, heartbeatInterval, heartbeatSent, heartbeatAcks)
		watchdogErr <- err
		if err != nil {
			cancel()
		}
	}()

	for {
		envelope, err := stream.Recv()
		if err != nil {
			select {
			case err = <-watchdogErr:
				if err != nil {
					return classifyV2Error(err)
				}
			default:
			}
			select {
			case senderErr := <-sendErr:
				if senderErr != nil && !errors.Is(senderErr, context.Canceled) {
					return classifyV2Error(senderErr)
				}
			default:
			}
			return classifyV2Error(err)
		}

		switch payload := envelope.Payload.(type) {
		case *sessionv2.ManagerEnvelope_Command:
			if payload.Command == nil || payload.Command.RequestId == "" {
				return reconnectSignal{side: reconnectSideSingle, reason: "invalid v2 command", err: errors.New("v2 command is missing request ID")}
			}
			body := Body{ReqID: payload.Command.RequestId, Data: payload.Command.PayloadJson}
			select {
			case s.reader <- body:
			case <-connectionCtx.Done():
				return classifyV2Error(connectionCtx.Err())
			default:
				return reconnectSignal{side: reconnectSideSingle, statusCode: http.StatusTooManyRequests, reason: "agent command queue is full", err: errors.New("agent command queue is full")}
			}
		case *sessionv2.ManagerEnvelope_HeartbeatAck:
			if payload.HeartbeatAck == nil {
				return reconnectSignal{side: reconnectSideSingle, reason: "invalid v2 heartbeat acknowledgement", err: errors.New("v2 heartbeat acknowledgement is missing")}
			}
			select {
			case heartbeatAcks <- payload.HeartbeatAck.Sequence:
			default:
				return reconnectSignal{side: reconnectSideSingle, statusCode: http.StatusTooManyRequests, reason: "heartbeat acknowledgement queue is full", err: errors.New("heartbeat acknowledgement queue is full")}
			}
		case *sessionv2.ManagerEnvelope_DrainNotice:
			retryAfter := time.Duration(payload.DrainNotice.GetReconnectAfterMillis()) * time.Millisecond
			return reconnectSignal{side: reconnectSideSingle, statusCode: http.StatusServiceUnavailable, retryAfter: retryAfter, reason: "manager draining"}
		default:
			return reconnectSignal{side: reconnectSideSingle, reason: "unexpected v2 manager message", err: errors.New("unexpected v2 manager message")}
		}
	}
}

func (s *Session) sendV2Messages(
	ctx context.Context,
	stream sessionv2.SessionService_ConnectClient,
	heartbeatInterval time.Duration,
	maxManagerReceiveBytes int,
	heartbeatSent chan<- uint64,
) error {
	ticker := time.NewTicker(heartbeatInterval)
	defer ticker.Stop()

	var heartbeatSequence uint64
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case body, ok := <-s.writer:
			if !ok {
				return errors.New("session response queue closed")
			}
			envelope := &sessionv2.AgentEnvelope{Payload: &sessionv2.AgentEnvelope_CommandResult{CommandResult: &sessionv2.CommandResult{
				RequestId:   body.ReqID,
				PayloadJson: body.Data,
			}}}
			if err := sendAgentV2Envelope(stream, envelope, maxManagerReceiveBytes); err != nil {
				return err
			}
		case <-ticker.C:
			heartbeatSequence++
			select {
			case heartbeatSent <- heartbeatSequence:
			case <-ctx.Done():
				return ctx.Err()
			}
			envelope := &sessionv2.AgentEnvelope{Payload: &sessionv2.AgentEnvelope_Heartbeat{Heartbeat: &sessionv2.Heartbeat{Sequence: heartbeatSequence}}}
			if err := sendAgentV2Envelope(stream, envelope, maxManagerReceiveBytes); err != nil {
				return err
			}
		}
	}
}

func negotiatedV2MessageLimit(advertised uint32) int {
	if advertised == 0 || advertised > sessionv2.DefaultMaxMessageBytes {
		return sessionv2.DefaultMaxMessageBytes
	}
	return int(advertised)
}

func sendAgentV2Envelope(stream sessionv2.SessionService_ConnectClient, envelope *sessionv2.AgentEnvelope, limit int) error {
	if proto.Size(envelope) > limit {
		return status.Error(codes.ResourceExhausted, "agent session message exceeds manager receive limit")
	}
	return stream.Send(envelope)
}

func receiveFirstManagerV2Message(
	ctx context.Context,
	stream sessionv2.SessionService_ConnectClient,
	timeout time.Duration,
) (*sessionv2.ManagerEnvelope, error) {
	received := make(chan managerV2ReceiveResult, 1)
	go func() {
		envelope, err := stream.Recv()
		received <- managerV2ReceiveResult{envelope: envelope, err: err}
	}()

	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case result := <-received:
		return result.envelope, result.err
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-timer.C:
		return nil, status.Error(codes.DeadlineExceeded, "session hello acknowledgement timeout")
	}
}

func monitorV2Heartbeat(
	ctx context.Context,
	heartbeatInterval time.Duration,
	heartbeatSent <-chan uint64,
	heartbeatAcks <-chan uint64,
) error {
	timer := time.NewTimer(time.Hour)
	if !timer.Stop() {
		<-timer.C
	}
	defer timer.Stop()

	var timerC <-chan time.Time
	var pendingSequence uint64
	var highestAck uint64
	for {
		select {
		case <-ctx.Done():
			return nil
		case sequence := <-heartbeatSent:
			if pendingSequence == 0 && sequence > highestAck {
				pendingSequence = sequence
				timer.Reset(2 * heartbeatInterval)
				timerC = timer.C
			}
		case sequence := <-heartbeatAcks:
			if sequence > highestAck {
				highestAck = sequence
			}
			if pendingSequence != 0 && highestAck >= pendingSequence {
				pendingSequence = 0
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				timerC = nil
			}
		case <-timerC:
			return errors.New("timed out waiting for v2 heartbeat acknowledgement")
		}
	}
}

func newV2ClientConn(endpoint string) (*grpc.ClientConn, error) {
	u, target, err := parseV2Endpoint(endpoint)
	if err != nil {
		return nil, err
	}

	var transportCredentials credentials.TransportCredentials
	switch strings.ToLower(u.Scheme) {
	case "https":
		transportCredentials = credentials.NewTLS(&tls.Config{
			MinVersion: tls.VersionTLS12,
			ServerName: u.Hostname(),
		})
	case "http":
		transportCredentials = insecure.NewCredentials()
	default:
		return nil, fmt.Errorf("unsupported session endpoint scheme %q", u.Scheme)
	}

	return grpc.NewClient(
		target,
		grpc.WithTransportCredentials(transportCredentials),
		grpc.WithDefaultCallOptions(
			grpc.MaxCallRecvMsgSize(sessionv2.DefaultMaxMessageBytes),
			grpc.MaxCallSendMsgSize(sessionv2.DefaultMaxMessageBytes),
		),
	)
}

func parseV2Endpoint(endpoint string) (*url.URL, string, error) {
	u, err := url.Parse(endpoint)
	if err != nil {
		return nil, "", fmt.Errorf("parse session endpoint: %w", err)
	}
	if u.Hostname() == "" {
		return nil, "", fmt.Errorf("session endpoint %q has no host", endpoint)
	}
	if u.User != nil {
		return nil, "", errors.New("session endpoint must not contain user information")
	}
	if (u.EscapedPath() != "" && u.EscapedPath() != "/") || u.RawQuery != "" || u.Fragment != "" {
		return nil, "", errors.New("session endpoint must not contain a path, query, or fragment")
	}

	port := u.Port()
	if port == "" {
		switch strings.ToLower(u.Scheme) {
		case "https":
			port = "443"
		case "http":
			port = "80"
		default:
			return nil, "", fmt.Errorf("unsupported session endpoint scheme %q", u.Scheme)
		}
	}
	return u, net.JoinHostPort(u.Hostname(), port), nil
}

func classifyV2Error(err error) reconnectSignal {
	sig := reconnectSignal{side: reconnectSideSingle, err: err}
	if err == nil {
		return sig
	}

	st, ok := status.FromError(err)
	if !ok {
		sig.reason = err.Error()
		return sig
	}
	sig.reason = st.Message()
	switch st.Code() {
	case codes.Unimplemented:
		sig.err = fmt.Errorf("%w: %v", errV2Unsupported, err)
	case codes.ResourceExhausted:
		sig.statusCode = http.StatusTooManyRequests
	case codes.Unavailable:
		sig.statusCode = http.StatusServiceUnavailable
	case codes.Unauthenticated:
		sig.statusCode = http.StatusUnauthorized
		sig.authFailure = true
	case codes.PermissionDenied:
		sig.statusCode = http.StatusForbidden
		sig.authFailure = true
	}
	for _, detail := range st.Details() {
		if retry, ok := detail.(*errdetails.RetryInfo); ok && retry.RetryDelay != nil {
			sig.retryAfter = retry.RetryDelay.AsDuration()
		}
	}
	return sig
}
