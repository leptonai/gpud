package session

import (
	"context"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"sync"
	"testing"

	pkgmetadata "github.com/leptonai/gpud/pkg/metadata"
	pkgsqlite "github.com/leptonai/gpud/pkg/sqlite"
)

func TestProcessUpdateToken(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		initialToken   string
		requestToken   string
		expectedToken  string
		expectError    bool
		mockHTTPStatus int
		skipValidation bool
	}{
		{
			name:           "update token successfully with valid token",
			initialToken:   "old-token",
			requestToken:   "new-valid-token",
			expectedToken:  "new-valid-token",
			expectError:    false,
			mockHTTPStatus: http.StatusOK,
		},
		{
			name:           "empty token should fail",
			initialToken:   "old-token",
			requestToken:   "",
			expectedToken:  "old-token",
			expectError:    true,
			skipValidation: true,
		},
		{
			name:           "same token should skip update",
			initialToken:   "same-token",
			requestToken:   "same-token",
			expectedToken:  "same-token",
			expectError:    false,
			skipValidation: true,
		},
		{
			name:           "invalid token should fail validation",
			initialToken:   "old-token",
			requestToken:   "invalid-token",
			expectedToken:  "old-token",
			expectError:    true,
			mockHTTPStatus: http.StatusUnauthorized,
		},
		{
			name:           "forbidden token should fail validation",
			initialToken:   "old-token",
			requestToken:   "forbidden-token",
			expectedToken:  "old-token",
			expectError:    true,
			mockHTTPStatus: http.StatusForbidden,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()

			// Create mock HTTP server for token validation
			var mockServer *httptest.Server
			if !tt.skipValidation {
				mockServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path == "/healthz" {
						w.WriteHeader(tt.mockHTTPStatus)
						return
					}
					w.WriteHeader(http.StatusNotFound)
				}))
				defer mockServer.Close()
			}

			// Create test database
			dbRW, dbRO, cleanup := pkgsqlite.OpenTestDB(t)
			defer cleanup()

			// Create metadata table
			if err := pkgmetadata.CreateTableMetadata(ctx, dbRW); err != nil {
				t.Fatalf("failed to create metadata table: %v", err)
			}

			// Set initial token
			if tt.initialToken != "" {
				if err := pkgmetadata.SetMetadata(ctx, dbRW, pkgmetadata.MetadataKeyToken, tt.initialToken); err != nil {
					t.Fatalf("failed to set initial token: %v", err)
				}
			}

			epControlPlane := ""
			if mockServer != nil {
				epControlPlane = mockServer.URL
			}

			s := &Session{
				token:          tt.initialToken,
				dbRW:           dbRW,
				dbRO:           dbRO,
				epControlPlane: epControlPlane,
				closer:         &closeOnce{closer: make(chan any)},
				checkServerHealthFunc: func(ctx context.Context, jar *cookiejar.Jar, token string) error {
					return nil // Mock function that always succeeds for non-error test cases
				},
			}

			// For specific test cases, override the mock to return errors
			if tt.mockHTTPStatus == http.StatusUnauthorized || tt.mockHTTPStatus == http.StatusForbidden {
				s.checkServerHealthFunc = func(ctx context.Context, jar *cookiejar.Jar, token string) error {
					req, _ := http.NewRequestWithContext(ctx, "GET", epControlPlane+"/healthz", nil)
					req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
					client := &http.Client{}
					resp, err := client.Do(req)
					if err != nil {
						return err
					}
					defer func() {
						_ = resp.Body.Close()
					}()
					if resp.StatusCode != http.StatusOK {
						return fmt.Errorf("server health check failed: %d %s", resp.StatusCode, resp.Status)
					}
					return nil
				}
			}

			payload := Request{
				Token: tt.requestToken,
			}
			response := &Response{}

			s.processUpdateToken(payload, response)

			if tt.expectError {
				if response.Error == "" {
					t.Errorf("expected error but got none")
				}
			} else {
				if response.Error != "" {
					t.Errorf("unexpected error: %s", response.Error)
				}

				// For successful updates, verify token was updated in database
				if tt.requestToken != tt.initialToken && tt.requestToken != "" {
					dbToken, err := pkgmetadata.ReadToken(ctx, dbRO)
					if err != nil {
						t.Errorf("failed to read token from database: %v", err)
					}
					if dbToken != tt.expectedToken {
						t.Errorf("expected database token %q, got %q", tt.expectedToken, dbToken)
					}
				}
			}

			// Verify in-memory token
			actualToken := s.getToken()
			if actualToken != tt.expectedToken {
				t.Errorf("expected in-memory token %q, got %q", tt.expectedToken, actualToken)
			}
		})
	}
}

func TestProcessGetToken(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		currentToken  string
		expectedToken string
	}{
		{
			name:          "get token successfully",
			currentToken:  "test-token",
			expectedToken: "test-token",
		},
		{
			name:          "get empty token",
			currentToken:  "",
			expectedToken: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()

			// Create test database
			dbRW, dbRO, cleanup := pkgsqlite.OpenTestDB(t)
			defer cleanup()

			// Create metadata table
			if err := pkgmetadata.CreateTableMetadata(ctx, dbRW); err != nil {
				t.Fatalf("failed to create metadata table: %v", err)
			}

			// Set token in database
			if tt.currentToken != "" {
				if err := pkgmetadata.SetMetadata(ctx, dbRW, pkgmetadata.MetadataKeyToken, tt.currentToken); err != nil {
					t.Fatalf("failed to set token: %v", err)
				}
			}

			s := &Session{
				dbRW: dbRW,
				dbRO: dbRO,
				checkServerHealthFunc: func(ctx context.Context, jar *cookiejar.Jar, token string) error {
					return nil // Mock function that always succeeds for non-error test cases
				},
			}

			response := &Response{}

			s.processGetToken(response)

			if response.Error != "" {
				t.Errorf("unexpected error: %s", response.Error)
			}

			if response.Token != tt.expectedToken {
				t.Errorf("expected token %q, got %q", tt.expectedToken, response.Token)
			}
		})
	}
}

func TestProcessUpdateToken_NoDatabase(t *testing.T) {
	t.Parallel()

	s := &Session{
		dbRW: nil,
		dbRO: nil,
		checkServerHealthFunc: func(ctx context.Context, jar *cookiejar.Jar, token string) error {
			return nil // Mock function that always succeeds for non-error test cases
		},
	}

	payload := Request{
		Token: "test-token",
	}
	response := &Response{}

	s.processUpdateToken(payload, response)

	if response.Error == "" {
		t.Error("expected error when database is not available")
	}
}

func TestProcessGetToken_NoDatabase(t *testing.T) {
	t.Parallel()

	s := &Session{
		dbRW: nil,
		dbRO: nil,
		checkServerHealthFunc: func(ctx context.Context, jar *cookiejar.Jar, token string) error {
			return nil // Mock function that always succeeds for non-error test cases
		},
	}

	response := &Response{}

	s.processGetToken(response)

	if response.Error == "" {
		t.Error("expected error when database is not available")
	}
}

func TestTokenConcurrency(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	// Create mock HTTP server for token validation
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer mockServer.Close()

	// Create test database
	dbRW, dbRO, cleanup := pkgsqlite.OpenTestDB(t)
	defer cleanup()

	// Create metadata table
	if err := pkgmetadata.CreateTableMetadata(ctx, dbRW); err != nil {
		t.Fatalf("failed to create metadata table: %v", err)
	}

	// Set initial token
	if err := pkgmetadata.SetMetadata(ctx, dbRW, pkgmetadata.MetadataKeyToken, "initial-token"); err != nil {
		t.Fatalf("failed to set initial token: %v", err)
	}

	s := &Session{
		token:          "initial-token",
		dbRW:           dbRW,
		dbRO:           dbRO,
		epControlPlane: mockServer.URL,
		closer:         &closeOnce{closer: make(chan any)},
		checkServerHealthFunc: func(ctx context.Context, jar *cookiejar.Jar, token string) error {
			return nil // Mock function that always succeeds for non-error test cases
		},
	}

	// Test concurrent reads and writes
	done := make(chan bool)
	var wg sync.WaitGroup

	// Start multiple goroutines writing tokens
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			payload := Request{
				Token: "token-" + string(rune('0'+n)),
			}
			response := &Response{}
			s.processUpdateToken(payload, response)
			done <- true
		}(i)
	}

	// Start multiple goroutines reading tokens
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			response := &Response{}
			s.processGetToken(response)
			done <- true
		}()
	}

	// Wait for all goroutines to finish
	go func() {
		wg.Wait()
		close(done)
	}()

	count := 0
	for range done {
		count++
	}

	if count != 10 {
		t.Errorf("expected 10 operations, got %d", count)
	}

	// Verify that we can still read the token without panic
	response := &Response{}
	s.processGetToken(response)
	if response.Error != "" {
		t.Errorf("unexpected error: %s", response.Error)
	}
	if response.Token == "" {
		t.Error("final token should not be empty")
	}
}
