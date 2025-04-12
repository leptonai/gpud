package session

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
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
		if r.URL.Path == "/healthz" {
			w.WriteHeader(http.StatusOK)
			return
		}
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

func TestReaderWriterServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Header.Get("session_type") {
		case "write":
			w.WriteHeader(http.StatusInternalServerError)
		case "read":
			w.WriteHeader(http.StatusInternalServerError)
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
	localCtx, localCancel := context.WithCancel(context.Background()) // create local context for each session
	defer localCancel()
	// start reader
	readerExit := make(chan any)
	jar, _ := cookiejar.New(nil)
	go s.startReader(localCtx, readerExit, jar)

	select {
	case <-readerExit:
	case <-time.After(3 * time.Second):
		t.Error("reader timeout")
	}
	// start writer
	writerExit := make(chan any)
	go s.startWriter(localCtx, writerExit, jar)

	select {
	case <-writerExit:
	case <-time.After(3 * time.Second):
		t.Error("writer timeout")
	}

	s.Stop()
	if _, ok := <-s.reader; ok {
		t.Errorf("Reader channel should be closed")
	}
	if _, ok := <-s.writer; ok {
		t.Errorf("Writer channel should be closed")
	}
}

func TestCreateHTTPClient(t *testing.T) {
	jar, _ := cookiejar.New(nil)
	client := createHTTPClient(jar)
	if client == nil {
		t.Fatal("Expected non-nil HTTP client")
	}

	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatal("Expected *http.Transport")
	}

	if transport.DisableKeepAlives != true {
		t.Error("Expected DisableKeepAlives to be true")
	}

	if transport.MaxIdleConns != 10 {
		t.Errorf("Expected MaxIdleConns to be 10, got %d", transport.MaxIdleConns)
	}
}

func TestCreateSessionRequest(t *testing.T) {
	tests := []struct {
		name        string
		ctx         context.Context
		endpoint    string
		machineID   string
		sessionType string
		body        io.Reader
		wantErr     bool
	}{
		{
			name:        "valid request with no body",
			ctx:         context.Background(),
			endpoint:    "http://test.com",
			machineID:   "test-machine",
			sessionType: "read",
			body:        nil,
			wantErr:     false,
		},
		{
			name:        "valid request with body",
			ctx:         context.Background(),
			endpoint:    "http://test.com",
			machineID:   "test-machine",
			sessionType: "write",
			body:        strings.NewReader("test-body"),
			wantErr:     false,
		},
		{
			name:        "invalid endpoint",
			ctx:         context.Background(),
			endpoint:    "://invalid-url",
			machineID:   "test-machine",
			sessionType: "read",
			body:        nil,
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := createSessionRequest(tt.ctx, tt.endpoint, tt.machineID, tt.sessionType, tt.body)
			if (err != nil) != tt.wantErr {
				t.Errorf("createSessionRequest() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if req.Header.Get("machine_id") != tt.machineID {
					t.Errorf("Expected machine_id header %s, got %s", tt.machineID, req.Header.Get("machine_id"))
				}
				if req.Header.Get("session_type") != tt.sessionType {
					t.Errorf("Expected session_type header %s, got %s", tt.sessionType, req.Header.Get("session_type"))
				}
			}
		})
	}
}

func TestWriteBodyToPipe(t *testing.T) {
	tests := []struct {
		name    string
		body    Body
		wantErr bool
	}{
		{
			name: "valid body",
			body: Body{
				ReqID: "test-req",
				Data:  []byte("test-data"),
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader, writer := io.Pipe()
			s := &Session{}

			// Start a goroutine to read from the pipe
			done := make(chan struct{})
			var readErr error
			var readBody Body

			go func() {
				defer close(done)
				decoder := json.NewDecoder(reader)
				readErr = decoder.Decode(&readBody)
			}()

			err := s.writeBodyToPipe(writer, tt.body)
			writer.Close()

			<-done // Wait for reading to complete

			if (err != nil) != tt.wantErr {
				t.Errorf("writeBodyToPipe() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr {
				if readErr != nil {
					t.Errorf("Error reading from pipe: %v", readErr)
				}
				if readBody.ReqID != tt.body.ReqID {
					t.Errorf("Expected ReqID %s, got %s", tt.body.ReqID, readBody.ReqID)
				}
			}
		})
	}
}

func TestTryWriteToReader(t *testing.T) {
	tests := []struct {
		name             string
		setupCloser      bool
		readerBufferSize int
		content          Body
		preloadMessages  int
		expectedResult   bool
	}{
		{
			name:             "successful write",
			readerBufferSize: 1,
			content: Body{
				ReqID: "test",
			},
			expectedResult: true,
		},
		{
			name:        "closed session",
			setupCloser: true,
			content: Body{
				ReqID: "test",
			},
			expectedResult: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert := assert.New(t)

			s := &Session{
				reader: make(chan Body, tt.readerBufferSize),
				closer: &closeOnce{closer: make(chan any)},
			}

			if tt.setupCloser {
				s.closer.Close()
			}

			// Preload messages if needed
			for i := 0; i < tt.preloadMessages; i++ {
				s.reader <- Body{ReqID: "preload"}
			}

			beforeWrite := time.Now()
			result := s.tryWriteToReader(tt.content)

			assert.Equal(tt.expectedResult, result)

			if result && !tt.setupCloser {
				timestamp := s.getLastPackageTimestamp()
				assert.False(timestamp.Before(beforeWrite), "Expected timestamp to be updated after successful write")
			}
		})
	}
}

func TestHandleReaderPipe(t *testing.T) {
	tests := []struct {
		name              string
		setupCloser       bool
		timeoutDuration   time.Duration
		expectedExitAfter time.Duration
	}{
		{
			name:              "normal operation",
			timeoutDuration:   5 * time.Second,
			expectedExitAfter: 10 * time.Second,
		},
		{
			name:              "timeout exceeded",
			timeoutDuration:   100 * time.Millisecond,
			expectedExitAfter: 3 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert := assert.New(t)

			s := &Session{
				closer: &closeOnce{closer: make(chan any)},
			}

			if tt.setupCloser {
				s.closer.Close()
			}

			reader, writer := io.Pipe()
			closec := make(chan any)
			finish := make(chan any)

			// Set initial timestamp
			s.setLastPackageTimestamp(time.Now())

			go s.handleReaderPipe(reader, closec, finish)

			select {
			case <-finish:
				assert.Fail("Pipe handler exited too early")
			case <-time.After(50 * time.Millisecond):
				// Expected - handler should still be running
			}

			if tt.setupCloser {
				select {
				case <-finish:
					// Expected - handler should exit when session is closed
				case <-time.After(tt.expectedExitAfter):
					assert.Fail("Pipe handler didn't exit after session close")
				}
			}

			writer.Close()
			reader.Close()
		})
	}
}

func TestCloseOnce(t *testing.T) {
	c := &closeOnce{
		closer: make(chan any),
	}

	// First close should succeed
	c.Close()

	// Second close should not panic
	c.Close()

	// Channel should be closed
	select {
	case <-c.Done():
		// Expected
	default:
		t.Error("Channel should be closed")
	}
}

func TestSessionKeepAlive(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	s := &Session{
		ctx:          ctx,
		cancel:       cancel,
		endpoint:     server.URL,
		machineID:    "test",
		pipeInterval: 100 * time.Millisecond,
		writer:       make(chan Body, 10),
		reader:       make(chan Body, 10),
		closer:       &closeOnce{closer: make(chan any)},
	}

	go s.keepAlive()

	// Let it run for a bit
	time.Sleep(300 * time.Millisecond)

	// Should be able to stop cleanly
	s.Stop()

	// Channels should be closed
	if _, ok := <-s.reader; ok {
		t.Error("Reader channel should be closed")
	}
	if _, ok := <-s.writer; ok {
		t.Error("Writer channel should be closed")
	}
}

func TestLastPackageTimestamp(t *testing.T) {
	assert := assert.New(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s := &Session{
		ctx:    ctx,
		cancel: cancel,
		closer: &closeOnce{closer: make(chan any)},
	}

	// Test initial state
	initialTime := s.getLastPackageTimestamp()
	assert.True(initialTime.IsZero(), "Expected initial timestamp to be zero")

	// Test setting and getting timestamp
	now := time.Now()
	s.setLastPackageTimestamp(now)
	gotTime := s.getLastPackageTimestamp()
	assert.True(gotTime.Equal(now), "Expected timestamp to match set time")

	// Test concurrent access
	const numGoroutines = 100
	var wg sync.WaitGroup
	wg.Add(numGoroutines * 2) // For both readers and writers

	// Launch multiple goroutines to test concurrent reads
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			_ = s.getLastPackageTimestamp()
		}()
	}

	// Launch multiple goroutines to test concurrent writes
	for i := 0; i < numGoroutines; i++ {
		go func(i int) {
			defer wg.Done()
			s.setLastPackageTimestamp(time.Now().Add(time.Duration(i) * time.Millisecond))
		}(i)
	}

	wg.Wait()

	// Verify timestamp was updated during concurrent operations
	finalTime := s.getLastPackageTimestamp()
	assert.False(finalTime.Equal(now), "Expected timestamp to be updated during concurrent operations")
}
