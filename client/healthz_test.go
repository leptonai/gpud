package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/leptonai/gpud/internal/server"
)

func TestCheckHealthz(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		body       string
		wantErr    bool
	}{
		{"Success", http.StatusOK, `{"status":"ok","version":"v1"}`, false},
		{"WrongStatus", http.StatusInternalServerError, "", true},
		{"WrongBody", http.StatusOK, `{"status":"error"}`, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/healthz" {
					t.Errorf("Expected /healthz path, got %s", r.URL.Path)
					http.NotFound(w, r)
					return
				}
				w.WriteHeader(tt.statusCode)
				if _, err := w.Write([]byte(tt.body)); err != nil {
					t.Errorf("Error writing response: %v", err)
				}
			}))
			defer srv.Close()

			err := CheckHealthz(context.Background(), srv.URL)
			if (err != nil) != tt.wantErr {
				t.Errorf("CheckHealthz() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestCheckHealthzWithCustomClient(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/healthz" {
			t.Errorf("Expected /healthz path, got %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusOK)
		json, _ := server.DefaultHealthz.JSON()
		if _, err := w.Write(json); err != nil {
			t.Errorf("Error writing response: %v", err)
		}
	}))
	defer srv.Close()

	customClient := &http.Client{}
	err := CheckHealthz(context.Background(), srv.URL, WithHTTPClient(customClient))
	if err != nil {
		t.Errorf("CheckHealthz() with custom client error = %v, want nil", err)
	}
}

func TestBlockUntilServerReady(t *testing.T) {
	tests := []struct {
		name           string
		serverBehavior func(w http.ResponseWriter, r *http.Request)
		expectedError  bool
	}{
		{
			name: "Server ready immediately",
			serverBehavior: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				json, _ := server.DefaultHealthz.JSON()
				if _, err := w.Write(json); err != nil {
					t.Errorf("Error writing response: %v", err)
				}
			},
			expectedError: false,
		},
		{
			name: "Server ready after delay",
			serverBehavior: func(w http.ResponseWriter, r *http.Request) {
				time.Sleep(100 * time.Millisecond)
				w.WriteHeader(http.StatusOK)
				json, _ := server.DefaultHealthz.JSON()
				if _, err := w.Write(json); err != nil {
					t.Errorf("Error writing response: %v", err)
				}
			},
			expectedError: false,
		},
		{
			name: "Server never ready",
			serverBehavior: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			},
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(tt.serverBehavior))
			defer server.Close()

			ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
			defer cancel()

			err := BlockUntilServerReady(ctx, server.URL, WithCheckInterval(50*time.Millisecond))
			if (err != nil) != tt.expectedError {
				t.Errorf("BlockUntilServerReady() error = %v, expectedError %v", err, tt.expectedError)
			}
		})
	}
}
