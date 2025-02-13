package v1

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/leptonai/gpud/pkg/gpud-manager/packages"
)

func TestGetStatus(t *testing.T) {
	tests := []struct {
		name           string
		serverResponse func(w http.ResponseWriter, r *http.Request)
		wantErr        bool
		expectedData   []packages.PackageStatus
	}{
		{
			name: "successful response",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, http.MethodGet, r.Method)

				response := []packages.PackageStatus{
					{
						Name:           "test-package",
						IsInstalled:    true,
						Installing:     false,
						Progress:       100,
						TotalTime:      1 * time.Hour,
						Status:         true,
						TargetVersion:  "1.0.0",
						CurrentVersion: "1.0.0",
						ScriptPath:     "/path/to/script",
						Dependency:     [][]string{{"dep1", "1.0.0"}},
					},
				}
				assert.NoError(t, json.NewEncoder(w).Encode(response))
			},
			wantErr: false,
			expectedData: []packages.PackageStatus{
				{
					Name:           "test-package",
					IsInstalled:    true,
					Installing:     false,
					Progress:       100,
					TotalTime:      1 * time.Hour,
					Status:         true,
					TargetVersion:  "1.0.0",
					CurrentVersion: "1.0.0",
					ScriptPath:     "/path/to/script",
					Dependency:     [][]string{{"dep1", "1.0.0"}},
				},
			},
		},
		{
			name: "server returns error status",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			},
			wantErr:      true,
			expectedData: nil,
		},
		{
			name: "invalid JSON response",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				_, err := w.Write([]byte("invalid json"))
				assert.NoError(t, err)
			},
			wantErr:      true,
			expectedData: nil,
		},
		{
			name: "context canceled",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				// Simulate a slow response that will be canceled
				time.Sleep(100 * time.Millisecond)
				assert.NoError(t, json.NewEncoder(w).Encode([]packages.PackageStatus{}))
			},
			wantErr:      true,
			expectedData: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test server
			server := httptest.NewTLSServer(http.HandlerFunc(tt.serverResponse))
			defer server.Close()

			var ctx context.Context
			var cancel context.CancelFunc
			if tt.name == "context canceled" {
				ctx, cancel = context.WithTimeout(context.Background(), 50*time.Millisecond)
			} else {
				ctx, cancel = context.WithTimeout(context.Background(), time.Second)
			}
			defer cancel()

			// Call GetStatus with the test server's URL
			status, err := GetPackageStatus(ctx, server.URL)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, status)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedData, status)
			}
		})
	}
}
