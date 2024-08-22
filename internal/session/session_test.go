package session

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewSession(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	endpoint := "test-endpoint.com"
	machineID := "test-machine-id"

	session := NewSession(ctx, endpoint, machineID, time.Second, true)
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
		ctx:            ctx,
		cancel:         cancel,
		writer:         make(chan Body, 20),
		reader:         make(chan Body, 20),
		writerCloseCh:  make(chan bool, 2),
		readerCloseCh:  make(chan bool, 2),
		writerClosedCh: make(chan bool),
		readerClosedCh: make(chan bool),
	}

	// Simulate writer and reader running
	go func() {
		<-s.writerCloseCh
		s.writerClosedCh <- true
	}()
	go func() {
		<-s.readerCloseCh
		s.readerClosedCh <- true
	}()

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
		ctx:            ctx,
		cancel:         cancel,
		pipeInterval:   10 * time.Millisecond, // Reduce interval for faster testing
		endpoint:       server.URL,
		machineID:      "test_machine",
		writer:         make(chan Body, 100),
		reader:         make(chan Body, 100),
		writerCloseCh:  make(chan bool, 5),
		readerCloseCh:  make(chan bool, 5),
		writerClosedCh: make(chan bool),
		readerClosedCh: make(chan bool),
	}
	defer s.Stop()

	// start writer and reader
	go s.startWriter()
	go s.startReader()

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

			case <-time.After(time.Second):
				t.Error("reader timeout")
			}
		})
	}
}
