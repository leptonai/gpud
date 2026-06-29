package session

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestParseRetryAfter(t *testing.T) {
	now := time.Date(2026, 6, 29, 10, 0, 0, 0, time.UTC)
	future := now.Add(2 * time.Minute).Format(http.TimeFormat)
	past := now.Add(-time.Minute).Format(http.TimeFormat)

	tests := []struct {
		name string
		in   string
		want time.Duration
	}{
		{name: "empty", in: "", want: 0},
		{name: "seconds", in: "15", want: 15 * time.Second},
		{name: "zero seconds", in: "0", want: 0},
		{name: "negative seconds", in: "-1", want: 0},
		{name: "future http date", in: future, want: 2 * time.Minute},
		{name: "past http date", in: past, want: 0},
		{name: "invalid", in: "soon", want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseRetryAfter(tt.in, now); got != tt.want {
				t.Fatalf("parseRetryAfter(%q) = %s, want %s", tt.in, got, tt.want)
			}
		})
	}
}

func TestClassifySessionHTTPResponse(t *testing.T) {
	now := time.Date(2026, 6, 29, 10, 0, 0, 0, time.UTC)

	t.Run("overload retry after", func(t *testing.T) {
		resp := &http.Response{
			StatusCode: http.StatusServiceUnavailable,
			Status:     "503 Service Unavailable",
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"message":"control plane busy"}`)),
		}
		resp.Header.Set("Retry-After", "7")

		sig := classifySessionHTTPResponse(reconnectSideReader, resp, now)
		if sig.side != reconnectSideReader {
			t.Fatalf("side = %q, want %q", sig.side, reconnectSideReader)
		}
		if sig.statusCode != http.StatusServiceUnavailable {
			t.Fatalf("statusCode = %d, want %d", sig.statusCode, http.StatusServiceUnavailable)
		}
		if sig.retryAfter != 7*time.Second {
			t.Fatalf("retryAfter = %s, want 7s", sig.retryAfter)
		}
		if sig.reason != "control plane busy" {
			t.Fatalf("reason = %q, want control plane busy", sig.reason)
		}
		if !sig.isOverload() {
			t.Fatal("expected overload signal")
		}
		if sig.authFailure {
			t.Fatal("did not expect auth failure")
		}
	})

	t.Run("token validation internal failure normalized as auth", func(t *testing.T) {
		resp := &http.Response{
			StatusCode: http.StatusInternalServerError,
			Status:     "500 Internal Server Error",
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"code":"InternalFailure","message":"failed to validate token"}`)),
		}

		sig := classifySessionHTTPResponse(reconnectSideWriter, resp, now)
		if !sig.authFailure {
			t.Fatal("expected auth failure")
		}
		if !strings.Contains(sig.reason, "HTTP 401") {
			t.Fatalf("reason %q should contain normalized HTTP 401", sig.reason)
		}
		if !strings.Contains(sig.reason, "InvalidToken") {
			t.Fatalf("reason %q should contain InvalidToken", sig.reason)
		}
	})

	t.Run("falls back to response status", func(t *testing.T) {
		resp := &http.Response{
			StatusCode: http.StatusTooManyRequests,
			Status:     "429 Too Many Requests",
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader("")),
		}

		sig := classifySessionHTTPResponse(reconnectSideReader, resp, now)
		if sig.reason != "429 Too Many Requests" {
			t.Fatalf("reason = %q, want response status", sig.reason)
		}
	})
}

func TestStartReaderRetryAfterSignal(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-GPUD-Session-Type"); got != "read" {
			http.Error(w, "unexpected session type: "+got, http.StatusBadRequest)
			return
		}
		w.Header().Set("Retry-After", "13")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"message":"reader overloaded"}`))
	}))
	defer server.Close()

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("failed to create cookie jar: %v", err)
	}
	s := &Session{
		epControlPlane: server.URL,
		machineID:      "machine-id",
		token:          "token",
		closer:         &closeOnce{closer: make(chan any)},
		nowFunc: func() time.Time {
			return time.Date(2026, 6, 29, 10, 0, 0, 0, time.UTC)
		},
	}

	readerExit := make(chan reconnectSignal, 1)
	s.startReader(context.Background(), readerExit, jar)

	sig := <-readerExit
	if sig.side != reconnectSideReader {
		t.Fatalf("side = %q, want reader", sig.side)
	}
	if sig.statusCode != http.StatusServiceUnavailable {
		t.Fatalf("statusCode = %d, want %d", sig.statusCode, http.StatusServiceUnavailable)
	}
	if sig.retryAfter != 13*time.Second {
		t.Fatalf("retryAfter = %s, want 13s", sig.retryAfter)
	}
	if sig.reason != "reader overloaded" {
		t.Fatalf("reason = %q, want reader overloaded", sig.reason)
	}
}

func TestStartWriterRetryAfterSignal(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-GPUD-Session-Type"); got != "write" {
			http.Error(w, "unexpected session type: "+got, http.StatusBadRequest)
			return
		}
		w.Header().Set("Retry-After", "17")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"message":"writer throttled"}`))
	}))
	defer server.Close()

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("failed to create cookie jar: %v", err)
	}
	s := &Session{
		epControlPlane: server.URL,
		machineID:      "machine-id",
		token:          "token",
		closer:         &closeOnce{closer: make(chan any)},
		writer:         make(chan Body, 1),
		nowFunc: func() time.Time {
			return time.Date(2026, 6, 29, 10, 0, 0, 0, time.UTC)
		},
	}

	writerExit := make(chan reconnectSignal, 1)
	s.startWriter(context.Background(), writerExit, jar)

	sig := <-writerExit
	if sig.side != reconnectSideWriter {
		t.Fatalf("side = %q, want writer", sig.side)
	}
	if sig.statusCode != http.StatusTooManyRequests {
		t.Fatalf("statusCode = %d, want %d", sig.statusCode, http.StatusTooManyRequests)
	}
	if sig.retryAfter != 17*time.Second {
		t.Fatalf("retryAfter = %s, want 17s", sig.retryAfter)
	}
	if sig.reason != "writer throttled" {
		t.Fatalf("reason = %q, want writer throttled", sig.reason)
	}
}

func TestReconnectBackoffNextDelay(t *testing.T) {
	t.Run("exponential jitter window", func(t *testing.T) {
		s := &Session{
			jitterFunc: func(max time.Duration) time.Duration {
				return max / 2
			},
		}
		backoff := reconnectBackoff{}

		if got := backoff.nextDelay(s, reconnectSignal{}); got != reconnectInitialBackoff/2 {
			t.Fatalf("first delay = %s, want %s", got, reconnectInitialBackoff/2)
		}
		if got := backoff.nextDelay(s, reconnectSignal{}); got != reconnectInitialBackoff {
			t.Fatalf("second delay = %s, want %s", got, reconnectInitialBackoff)
		}
	})

	t.Run("retry after overrides jitter and adds spreading jitter", func(t *testing.T) {
		s := &Session{
			jitterFunc: func(max time.Duration) time.Duration {
				if max == retryAfterJitterMax {
					return 2 * time.Second
				}
				return time.Second
			},
		}
		backoff := reconnectBackoff{}

		got := backoff.nextDelay(s, reconnectSignal{
			statusCode: http.StatusServiceUnavailable,
			retryAfter: 20 * time.Second,
		})
		if got != 22*time.Second {
			t.Fatalf("delay = %s, want 22s", got)
		}
	})
}

func TestChooseReconnectSignal(t *testing.T) {
	t.Run("uses longest overload retry after", func(t *testing.T) {
		first := reconnectSignal{side: reconnectSideReader, statusCode: http.StatusTooManyRequests, retryAfter: 5 * time.Second}
		second := reconnectSignal{side: reconnectSideWriter, statusCode: http.StatusServiceUnavailable, retryAfter: 11 * time.Second}

		got := chooseReconnectSignal(first, second)
		if got.side != reconnectSideWriter {
			t.Fatalf("selected side = %q, want writer", got.side)
		}
		if got.retryAfter != 11*time.Second {
			t.Fatalf("selected retryAfter = %s, want 11s", got.retryAfter)
		}
	})

	t.Run("auth failure outranks generic exit", func(t *testing.T) {
		first := reconnectSignal{side: reconnectSideReader}
		second := reconnectSignal{side: reconnectSideWriter, authFailure: true, reason: "bad token"}

		got := chooseReconnectSignal(first, second)
		if got.side != reconnectSideWriter {
			t.Fatalf("selected side = %q, want writer", got.side)
		}
		if !got.authFailure {
			t.Fatal("expected auth failure to be selected")
		}
	})

	t.Run("sibling context shutdown does not hide failure reason", func(t *testing.T) {
		first := reconnectSignal{side: reconnectSideReader, statusCode: http.StatusServiceUnavailable, retryAfter: 30 * time.Second}
		second := reconnectSignal{side: reconnectSideWriter, err: context.Canceled}

		got := chooseReconnectSignal(first, second)
		if got.side != reconnectSideReader {
			t.Fatalf("selected side = %q, want reader", got.side)
		}
		if got.retryAfter != 30*time.Second {
			t.Fatalf("selected retryAfter = %s, want 30s", got.retryAfter)
		}
	})

	t.Run("context shutdown selected when no failure reason exists", func(t *testing.T) {
		first := reconnectSignal{side: reconnectSideReader, err: context.Canceled}
		second := reconnectSignal{}

		got := chooseReconnectSignal(first, second)
		if !errors.Is(got.err, context.Canceled) {
			t.Fatalf("selected err = %v, want context.Canceled", got.err)
		}
	})
}

func TestCheckServerHealthCapturesRetryAfter(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/healthz" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Retry-After", "9")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"message":"slow down"}`))
	}))
	defer server.Close()

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("failed to create cookie jar: %v", err)
	}
	s := &Session{
		epControlPlane: server.URL,
		token:          "token",
		nowFunc: func() time.Time {
			return time.Date(2026, 6, 29, 10, 0, 0, 0, time.UTC)
		},
	}

	err = s.checkServerHealth(context.Background(), jar, "")
	var httpErr *healthCheckHTTPError
	if !errors.As(err, &httpErr) {
		t.Fatalf("expected healthCheckHTTPError, got %v", err)
	}
	if httpErr.statusCode != http.StatusTooManyRequests {
		t.Fatalf("statusCode = %d, want %d", httpErr.statusCode, http.StatusTooManyRequests)
	}
	if httpErr.retryAfter != 9*time.Second {
		t.Fatalf("retryAfter = %s, want 9s", httpErr.retryAfter)
	}
	if httpErr.body != `{"message":"slow down"}` {
		t.Fatalf("body = %q", httpErr.body)
	}
}
