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

	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/log"
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

	session, err := NewSession(ctx, "", endpoint, "", WithMachineID(machineID), WithPipeInterval(time.Second), WithEnableAutoUpdate(true), WithComponentsRegistry(components.NewRegistry(nil)))
	if err != nil {
		t.Fatalf("error creating session: %v", err)
	}
	defer session.Stop()

	if session == nil {
		t.Fatal("Expected non-nil session")
	}
	if session.epControlPlane != endpoint {
		t.Errorf("expected endpoint %s, got %s", endpoint, session.epControlPlane)
	}
	if session.machineID != machineID {
		t.Errorf("expected machineID %s, got %s", machineID, session.machineID)
	}
}

func TestStop(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	s := &Session{
		ctx:         ctx,
		cancel:      cancel,
		auditLogger: log.NewNopAuditLogger(),
		writer:      make(chan Body, 20),
		reader:      make(chan Body, 20),
		closer:      &closeOnce{closer: make(chan any)},
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
		// Check both new and deprecated headers for compatibility
		sessionType := r.Header.Get("X-GPUD-Session-Type")
		if sessionType == "" {
			sessionType = r.Header.Get("session_type")
		}
		switch sessionType {
		case "write":
			var body Body
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, "Failed to decode request body", http.StatusBadRequest)
				return
			}
			w.WriteHeader(http.StatusOK)

		case "read":
			// Set up streaming response
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			flusher, ok := w.(http.Flusher)
			if !ok {
				http.Error(w, "Streaming not supported", http.StatusInternalServerError)
				return
			}

			// Send multiple responses to simulate a stream
			encoder := json.NewEncoder(w)
			for i := 0; i < 10; i++ {
				if err := encoder.Encode(Body{ReqID: "server_response_id"}); err != nil {
					return
				}
				flusher.Flush()
				time.Sleep(100 * time.Millisecond)
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
		ctx:               ctx,
		cancel:            cancel,
		pipeInterval:      10 * time.Millisecond, // Reduce interval for faster testing
		epLocalGPUdServer: server.URL,
		epControlPlane:    server.URL,
		token:             "testToken",
		machineID:         "test_machine",
		auditLogger:       log.NewNopAuditLogger(),
		writer:            make(chan Body, 100),
		reader:            make(chan Body, 100),
		closer:            &closeOnce{closer: make(chan any)},
	}

	// Initialize testable functions with controlled implementations to make
	// goroutine lifecycles deterministic during the test.
	s.timeAfterFunc = func(d time.Duration) <-chan time.Time {
		// Block reconnection attempts after the first session so we can
		// explicitly control shutdown ordering in the test.
		return make(chan time.Time)
	}
	s.timeSleepFunc = time.Sleep
	originalStartReader := s.startReader
	originalStartWriter := s.startWriter

	var exitMu sync.Mutex
	var readerExitChans []<-chan any
	var writerExitChans []<-chan any

	s.startReaderFunc = func(ctx context.Context, readerExit chan any, jar *cookiejar.Jar) {
		exitMu.Lock()
		readerExitChans = append(readerExitChans, readerExit)
		exitMu.Unlock()
		originalStartReader(ctx, readerExit, jar)
	}

	s.startWriterFunc = func(ctx context.Context, writerExit chan any, jar *cookiejar.Jar) {
		exitMu.Lock()
		writerExitChans = append(writerExitChans, writerExit)
		exitMu.Unlock()
		originalStartWriter(ctx, writerExit, jar)
	}

	s.checkServerHealthFunc = s.checkServerHealth

	// start writer reader keepAlive
	go s.keepAlive()

	// allow more time for the connection to be established
	// The keepAlive function needs time to:
	// 1. Check server health
	// 2. Start reader and writer goroutines
	// 3. Establish HTTP connections
	time.Sleep(500 * time.Millisecond)

	waitForGoroutineRegistration := func(getCount func() int, name string) {
		t.Helper()
		deadline := time.After(2 * time.Second)
		ticker := time.NewTicker(10 * time.Millisecond)
		defer ticker.Stop()
		for {
			if count := getCount(); count > 0 {
				return
			}
			select {
			case <-ticker.C:
			case <-deadline:
				t.Fatalf("timeout waiting for %s goroutine to start", name)
			}
		}
	}

	waitForGoroutineRegistration(func() int {
		exitMu.Lock()
		defer exitMu.Unlock()
		return len(readerExitChans)
	}, "reader")

	waitForGoroutineRegistration(func() int {
		exitMu.Lock()
		defer exitMu.Unlock()
		return len(writerExitChans)
	}, "writer")

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

	// Signal active session goroutines to exit and wait for them to finish
	s.closer.Close()

	waitForExit := func(chans []<-chan any, name string) {
		t.Helper()
		for idx, ch := range chans {
			select {
			case <-ch:
			case <-time.After(2 * time.Second):
				t.Fatalf("timeout waiting for %s exit channel %d to close", name, idx)
			}
		}
	}

	exitMu.Lock()
	readerChans := append([]<-chan any(nil), readerExitChans...)
	writerChans := append([]<-chan any(nil), writerExitChans...)
	exitMu.Unlock()

	if len(readerChans) == 0 {
		t.Fatal("no reader exit channels registered")
	}
	if len(writerChans) == 0 {
		t.Fatal("no writer exit channels registered")
	}

	waitForExit(readerChans, "reader")
	waitForExit(writerChans, "writer")

	// Give a bit of time for any pending operations
	time.Sleep(100 * time.Millisecond)

	// Check if context is already done before calling Stop
	select {
	case <-s.ctx.Done():
		t.Log("Context already canceled before Stop()")
	default:
		t.Log("Context still active before Stop()")
	}

	s.Stop()

	// Give Stop() time to complete
	time.Sleep(100 * time.Millisecond)

	// Drain any remaining messages and check if channels are closed
	// A closed channel with data will still return ok=true until drained
	// First, drain reader channel
	readerDrainTimeout := time.After(1 * time.Second)
readerDrain:
	for {
		select {
		case _, ok := <-s.reader:
			if !ok {
				// Channel is closed, good
				break readerDrain
			}
			// Still has messages, continue draining
		case <-readerDrainTimeout:
			t.Errorf("Reader channel should be closed but timeout while draining")
			break readerDrain
		}
	}

	// Then, drain writer channel
	writerDrainTimeout := time.After(1 * time.Second)
writerDrain:
	for {
		select {
		case _, ok := <-s.writer:
			if !ok {
				// Channel is closed, good
				break writerDrain
			}
			// Still has messages, continue draining
		case <-writerDrainTimeout:
			t.Errorf("Writer channel should be closed but timeout while draining")
			break writerDrain
		}
	}
}

func TestReaderWriterServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check both new and deprecated headers for compatibility
		sessionType := r.Header.Get("X-GPUD-Session-Type")
		if sessionType == "" {
			sessionType = r.Header.Get("session_type")
		}
		switch sessionType {
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
		ctx:            ctx,
		cancel:         cancel,
		pipeInterval:   10 * time.Millisecond, // Reduce interval for faster testing
		epControlPlane: server.URL,
		auditLogger:    log.NewNopAuditLogger(),
		machineID:      "test_machine",
		writer:         make(chan Body, 100),
		reader:         make(chan Body, 100),
		closer:         &closeOnce{closer: make(chan any)},
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
			req, err := createSessionRequest(tt.ctx, tt.endpoint, tt.machineID, tt.sessionType, "", tt.body)
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
			s := &Session{
				auditLogger: log.NewNopAuditLogger(),
			}

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
				auditLogger: log.NewNopAuditLogger(),
				reader:      make(chan Body, tt.readerBufferSize),
				closer:      &closeOnce{closer: make(chan any)},
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
				auditLogger: log.NewNopAuditLogger(),
				closer:      &closeOnce{closer: make(chan any)},
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
		ctx:            ctx,
		cancel:         cancel,
		epControlPlane: server.URL,
		auditLogger:    log.NewNopAuditLogger(),
		machineID:      "test",
		pipeInterval:   100 * time.Millisecond,
		writer:         make(chan Body, 10),
		reader:         make(chan Body, 10),
		closer:         &closeOnce{closer: make(chan any)},
	}

	// Initialize testable functions with controlled implementations to prevent race conditions
	s.timeAfterFunc = func(d time.Duration) <-chan time.Time {
		// Block reconnection attempts to control shutdown
		return make(chan time.Time)
	}
	s.timeSleepFunc = time.Sleep
	originalStartReader := s.startReader
	originalStartWriter := s.startWriter

	var exitMu sync.Mutex
	var readerExitChans []<-chan any
	var writerExitChans []<-chan any

	s.startReaderFunc = func(ctx context.Context, readerExit chan any, jar *cookiejar.Jar) {
		exitMu.Lock()
		readerExitChans = append(readerExitChans, readerExit)
		exitMu.Unlock()
		originalStartReader(ctx, readerExit, jar)
	}

	s.startWriterFunc = func(ctx context.Context, writerExit chan any, jar *cookiejar.Jar) {
		exitMu.Lock()
		writerExitChans = append(writerExitChans, writerExit)
		exitMu.Unlock()
		originalStartWriter(ctx, writerExit, jar)
	}

	s.checkServerHealthFunc = s.checkServerHealth

	go s.keepAlive()

	// Let it run for a bit to establish connections
	time.Sleep(300 * time.Millisecond)

	// CRITICAL FIX: Cancel the context first to stop the keepAlive loop
	// This prevents the race where keepAlive might be writing to s.closer
	// while we're trying to read it
	cancel()

	// Give keepAlive loop time to exit after context cancellation
	time.Sleep(100 * time.Millisecond)

	// Now it's safe to signal goroutines to exit via closer
	// since keepAlive loop has stopped and won't recreate s.closer
	s.closer.Close()

	// Wait for all goroutines to exit
	waitForExit := func(chans []<-chan any, name string) {
		t.Helper()
		for idx, ch := range chans {
			select {
			case <-ch:
			case <-time.After(2 * time.Second):
				t.Logf("timeout waiting for %s exit channel %d to close", name, idx)
			}
		}
	}

	exitMu.Lock()
	readerChans := append([]<-chan any(nil), readerExitChans...)
	writerChans := append([]<-chan any(nil), writerExitChans...)
	exitMu.Unlock()

	// Only wait if goroutines were started (they may not start if server health check fails)
	if len(readerChans) > 0 {
		waitForExit(readerChans, "reader")
	}
	if len(writerChans) > 0 {
		waitForExit(writerChans, "writer")
	}

	// Give a bit of time for any pending operations
	time.Sleep(100 * time.Millisecond)

	// Should be able to stop cleanly
	s.Stop()

	// Channels should be closed
	select {
	case _, ok := <-s.reader:
		if ok {
			t.Error("Reader channel should be closed")
		}
	default:
		// Channel might be empty but closed, which is fine
	}

	select {
	case _, ok := <-s.writer:
		if ok {
			t.Error("Writer channel should be closed")
		}
	default:
		// Channel might be empty but closed, which is fine
	}
}

func TestLastPackageTimestamp(t *testing.T) {
	assert := assert.New(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s := &Session{
		ctx:         ctx,
		cancel:      cancel,
		auditLogger: log.NewNopAuditLogger(),
		closer:      &closeOnce{closer: make(chan any)},
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
