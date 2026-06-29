package session

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	randv2 "math/rand/v2"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	reconnectSideReader      = "reader"
	reconnectSideWriter      = "writer"
	reconnectSideHealthCheck = "health_check"

	sessionErrorBodyReadLimit = 4 << 10

	reconnectInitialBackoff = 3 * time.Second
	reconnectMaxBackoff     = 60 * time.Second
	reconnectStableWindow   = 2 * time.Minute
	startupJitterMax        = 10 * time.Second
	retryAfterJitterMax     = 5 * time.Second
	tokenReconnectJitterMin = 2 * time.Second
	tokenReconnectJitterMax = 30 * time.Second
	cleanupDrainDelay       = 100 * time.Millisecond
)

type reconnectSignal struct {
	side        string
	statusCode  int
	retryAfter  time.Duration
	reason      string
	authFailure bool
	err         error
}

func (s reconnectSignal) isZero() bool {
	return s.side == "" && s.statusCode == 0 && s.retryAfter == 0 && s.reason == "" && !s.authFailure && s.err == nil
}

func (s reconnectSignal) isOverload() bool {
	return isOverloadStatus(s.statusCode)
}

func (s reconnectSignal) isContextShutdown() bool {
	return errors.Is(s.err, context.Canceled) || errors.Is(s.err, context.DeadlineExceeded)
}

type reconnectBackoff struct {
	attempt int
}

func (b *reconnectBackoff) reset() {
	b.attempt = 0
}

func (b *reconnectBackoff) nextDelay(s *Session, sig reconnectSignal) time.Duration {
	maxDelay := reconnectInitialBackoff
	for i := 0; i < b.attempt && maxDelay < reconnectMaxBackoff; i++ {
		maxDelay *= 2
		if maxDelay > reconnectMaxBackoff {
			maxDelay = reconnectMaxBackoff
		}
	}

	delay := s.jitter(maxDelay)
	if sig.retryAfter > delay {
		delay = sig.retryAfter
	}
	if sig.retryAfter > 0 {
		delay += s.jitter(retryAfterJitterMax)
	}
	b.attempt++
	return delay
}

func sendReconnectSignal(ch chan reconnectSignal, sig reconnectSignal) {
	select {
	case ch <- sig:
	default:
	}
	close(ch)
}

func chooseReconnectSignal(first, second reconnectSignal) reconnectSignal {
	if first.isContextShutdown() && !second.isZero() && !second.isContextShutdown() {
		return second
	}
	if second.isContextShutdown() && !first.isZero() && !first.isContextShutdown() {
		return first
	}

	if first.isOverload() && first.retryAfter > 0 && second.isOverload() && second.retryAfter > 0 {
		if second.retryAfter > first.retryAfter {
			return second
		}
		return first
	}
	if first.isOverload() && first.retryAfter > 0 {
		return first
	}
	if second.isOverload() && second.retryAfter > 0 {
		return second
	}
	if first.authFailure {
		return first
	}
	if second.authFailure {
		return second
	}
	if first.isOverload() {
		return first
	}
	if second.isOverload() {
		return second
	}
	if !first.isZero() {
		return first
	}
	return second
}

func parseRetryAfter(value string, now time.Time) time.Duration {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}

	seconds, err := strconv.ParseInt(value, 10, 64)
	if err == nil {
		if seconds <= 0 {
			return 0
		}
		return time.Duration(seconds) * time.Second
	}

	t, err := http.ParseTime(value)
	if err != nil {
		return 0
	}
	if !t.After(now) {
		return 0
	}
	return t.Sub(now)
}

func readLimitedResponseBody(resp *http.Response, limit int64) string {
	if resp == nil || resp.Body == nil {
		return ""
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if limit <= 0 {
		limit = sessionErrorBodyReadLimit
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit))
	if err != nil {
		return ""
	}
	return string(body)
}

func extractResponseReason(body string) string {
	body = strings.TrimSpace(body)
	if body == "" {
		return ""
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(body), &payload); err == nil {
		for _, key := range []string{"reason", "message", "error_summary", "error", "code"} {
			if value, ok := payload[key]; ok {
				reason := strings.TrimSpace(fmt.Sprint(value))
				if reason != "" {
					return reason
				}
			}
		}
	}
	return body
}

func classifySessionHTTPResponse(side string, resp *http.Response, now time.Time) reconnectSignal {
	sig := reconnectSignal{side: side}
	if resp == nil {
		return sig
	}

	body := readLimitedResponseBody(resp, sessionErrorBodyReadLimit)
	sig.statusCode = resp.StatusCode
	sig.retryAfter = parseRetryAfter(resp.Header.Get("Retry-After"), now)
	sig.reason = extractResponseReason(body)
	if sig.reason == "" {
		sig.reason = resp.Status
	}

	if message, ok := loginFailureStatusMessage(resp.StatusCode, body); ok {
		sig.authFailure = true
		sig.reason = message
	}
	return sig
}

func classifyHealthCheckError(err error) reconnectSignal {
	sig := reconnectSignal{side: reconnectSideHealthCheck, err: err}
	if err == nil {
		return sig
	}

	var httpErr *healthCheckHTTPError
	if errors.As(err, &httpErr) {
		sig.statusCode = httpErr.statusCode
		sig.retryAfter = httpErr.retryAfter
		sig.reason = extractResponseReason(httpErr.body)
		if sig.reason == "" {
			sig.reason = http.StatusText(httpErr.statusCode)
		}
		if message, ok := loginFailureStatusMessage(httpErr.statusCode, httpErr.body); ok {
			sig.authFailure = true
			sig.reason = message
		}
	}
	return sig
}

func isOverloadStatus(statusCode int) bool {
	return statusCode == http.StatusTooManyRequests || statusCode == http.StatusServiceUnavailable
}

func defaultJitter(max time.Duration) time.Duration {
	if max <= 0 {
		return 0
	}
	return time.Duration(randv2.Int64N(int64(max)))
}

func (s *Session) now() time.Time {
	if s != nil && s.nowFunc != nil {
		return s.nowFunc()
	}
	return time.Now()
}

func (s *Session) jitter(max time.Duration) time.Duration {
	if s != nil && s.jitterFunc != nil {
		return s.jitterFunc(max)
	}
	return defaultJitter(max)
}

func (s *Session) waitReconnectDelay(ctx context.Context, delay time.Duration) bool {
	if ctx == nil {
		ctx = context.Background()
	}
	if delay <= 0 {
		return ctx.Err() == nil
	}

	timeAfter := time.After
	if s != nil && s.timeAfterFunc != nil {
		timeAfter = s.timeAfterFunc
	}

	select {
	case <-ctx.Done():
		return false
	case <-timeAfter(delay):
		return true
	}
}
