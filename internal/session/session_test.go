package session

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestApplyOpts(t *testing.T) {
	tests := []struct {
		name    string
		opts    []OpOption
		wantErr bool
	}{
		{
			name:    "Default options",
			opts:    []OpOption{},
			wantErr: false,
		},
		{
			name: "Enable auto update",
			opts: []OpOption{
				WithEnableAutoUpdate(true),
			},
			wantErr: false,
		},
		{
			name: "Disable auto update",
			opts: []OpOption{
				WithEnableAutoUpdate(false),
			},
			wantErr: false,
		},
		{
			name: "Set auto update by exit code with auto update enabled",
			opts: []OpOption{
				WithEnableAutoUpdate(true),
				WithAutoUpdateExitCode(1),
			},
			wantErr: false,
		},
		{
			name: "Set auto update by exit code with auto update disabled",
			opts: []OpOption{
				WithEnableAutoUpdate(false),
				WithAutoUpdateExitCode(1),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			op := &Op{}
			err := op.applyOpts(tt.opts)
			if (err != nil) != tt.wantErr {
				t.Errorf("applyOpts() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr && err != ErrAutoUpdateDisabledButExitCodeSet {
				t.Errorf("applyOpts() expected error %v, got %v", ErrAutoUpdateDisabledButExitCodeSet, err)
			}
		})
	}
}

func TestNewSession(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	endpoint := "test-endpoint.com"
	machineID := "test-machine-id"

	session, err := NewSession(ctx, endpoint, WithMachineID(machineID), WithPipeInterval(time.Second), WithEnableAutoUpdate(true))
	if err != nil {
		t.Fatalf("error creating session: %v", err)
	}
	defer session.Stop()

	if session == nil {
		t.Fatal("Expected non-nil session")
	}
	if session.endpoint != endpoint {
		t.Errorf("expected endpoint %s, got %s", endpoint, session.endpoint)
	}
	if session.machineID != machineID {
		t.Errorf("expected machineID %s, got %s", machineID, session.machineID)
	}
}

func TestStop(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	s := &Session{
		ctx:    ctx,
		cancel: cancel,
		writer: make(chan Body, 20),
		reader: make(chan Body, 20),
		closer: &closeOnce{closer: make(chan any)},
	}

	s.Stop()

	// check if channels are closed
	if _, ok := <-s.reader; ok {
		t.Errorf("Reader channel should be closed")
	}
	if _, ok := <-s.writer; ok {
		t.Errorf("Writer channel should be closed")
	}
}

func TestStartWriterAndReader(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Header.Get("session_type") {
		case "write":
			var body Body
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, "Failed to decode request body", http.StatusBadRequest)
				return
			}
			w.WriteHeader(http.StatusOK)

		case "read":
			// always return a predefined response
			if err := json.NewEncoder(w).Encode(Body{ReqID: "server_response_id"}); err != nil {
				http.Error(w, "Failed to encode response body", http.StatusInternalServerError)
				return
			}

		default:
			http.Error(w, "Invalid session type", http.StatusBadRequest)
		}
	}))
	defer server.Close()

	// create session
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s := &Session{
		ctx:          ctx,
		cancel:       cancel,
		pipeInterval: 10 * time.Millisecond, // Reduce interval for faster testing
		endpoint:     server.URL,
		machineID:    "test_machine",
		writer:       make(chan Body, 100),
		reader:       make(chan Body, 100),
		closer:       &closeOnce{closer: make(chan any)},
	}

	// start writer reader keepAlive
	go s.keepAlive()

	// allow some time for the goroutines to start
	time.Sleep(50 * time.Millisecond)

	// Test cases
	testCases := []struct {
		name      string
		sendReqID string
	}{
		{"request1", "client_req_1"},
		{"request2", "client_req_2"},
		{"request3", "client_req_3"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			select {
			case s.writer <- Body{ReqID: tc.sendReqID}:
			default:
				t.Fatal("writer timeout")
			}

			select {
			case body := <-s.reader:
				if body.ReqID != "server_response_id" {
					t.Errorf("expected ReqID 'server_response_id', got '%s'", body.ReqID)
				}

			case <-time.After(3 * time.Second):
				t.Error("reader timeout")
			}
		})
	}

	s.Stop()
	if _, ok := <-s.reader; ok {
		t.Errorf("Reader channel should be closed")
	}
	if _, ok := <-s.writer; ok {
		t.Errorf("Writer channel should be closed")
	}
}
