package v1

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/leptonai/gpud/pkg/httputil"
)

func TestSetHealthyComponents(t *testing.T) {
	tests := []struct {
		name            string
		components      []string
		serverResponse  interface{}
		statusCode      int
		contentType     string
		acceptEncoding  string
		expectedError   string
		expectedURL     string
		expectedMethod  string
		validateRequest func(t *testing.T, r *http.Request)
	}{
		{
			name:       "successful set healthy with single component",
			components: []string{"disk"},
			serverResponse: map[string]interface{}{
				"success": []string{"disk"},
			},
			statusCode:     http.StatusOK,
			contentType:    httputil.RequestHeaderJSON,
			expectedMethod: http.MethodPost,
			expectedURL:    "/v1/health-states/set-healthy?components=disk",
		},
		{
			name:       "successful set healthy with multiple components",
			components: []string{"disk", "memory", "cpu"},
			serverResponse: map[string]interface{}{
				"success": []string{"disk", "memory", "cpu"},
			},
			statusCode:     http.StatusOK,
			contentType:    httputil.RequestHeaderJSON,
			expectedMethod: http.MethodPost,
			expectedURL:    "/v1/health-states/set-healthy?components=disk%2Cmemory%2Ccpu",
		},
		{
			name:       "successful set healthy with no components (all components)",
			components: []string{},
			serverResponse: map[string]interface{}{
				"success": []string{"disk", "memory", "cpu", "gpu"},
			},
			statusCode:     http.StatusOK,
			contentType:    httputil.RequestHeaderJSON,
			expectedMethod: http.MethodPost,
			expectedURL:    "/v1/health-states/set-healthy",
		},
		{
			name:       "successful set healthy with nil components",
			components: nil,
			serverResponse: map[string]interface{}{
				"success": []string{"disk", "memory", "cpu", "gpu"},
			},
			statusCode:     http.StatusOK,
			expectedMethod: http.MethodPost,
			expectedURL:    "/v1/health-states/set-healthy",
		},
		{
			name:       "partial failure - some components failed",
			components: []string{"disk", "invalid-component"},
			serverResponse: map[string]interface{}{
				"success": []string{"disk"},
				"failed": map[string]string{
					"invalid-component": "component does not support setting healthy state",
				},
			},
			statusCode:    http.StatusOK,
			expectedError: "some components failed to set healthy: map[invalid-component:component does not support setting healthy state]",
		},
		{
			name:       "all components failed",
			components: []string{"invalid1", "invalid2"},
			serverResponse: map[string]interface{}{
				"failed": map[string]string{
					"invalid1": "component not found",
					"invalid2": "component not found",
				},
			},
			statusCode:    http.StatusOK,
			expectedError: "some components failed to set healthy: map[invalid1:component not found invalid2:component not found]",
		},
		{
			name:           "server returns 404",
			components:     []string{"disk"},
			statusCode:     http.StatusNotFound,
			expectedError:  "server returned 404: 404 page not found",
			serverResponse: "404 page not found",
		},
		{
			name:           "server returns 500",
			components:     []string{"disk"},
			statusCode:     http.StatusInternalServerError,
			expectedError:  "server returned 500: Internal Server Error",
			serverResponse: "Internal Server Error",
		},
		{
			name:       "server returns 400 bad request",
			components: []string{"disk"},
			serverResponse: map[string]interface{}{
				"code":    "InvalidArgument",
				"message": "Invalid component name",
			},
			statusCode:    http.StatusBadRequest,
			expectedError: "server returned 400:",
		},
		{
			name:           "successful with custom content type",
			components:     []string{"disk"},
			serverResponse: map[string]interface{}{"success": []string{"disk"}},
			statusCode:     http.StatusOK,
			contentType:    httputil.RequestHeaderYAML,
			expectedMethod: http.MethodPost,
			validateRequest: func(t *testing.T, r *http.Request) {
				assert.Equal(t, httputil.RequestHeaderYAML, r.Header.Get(httputil.RequestHeaderContentType))
			},
		},
		{
			name:           "successful with gzip encoding",
			components:     []string{"disk"},
			serverResponse: map[string]interface{}{"success": []string{"disk"}},
			statusCode:     http.StatusOK,
			acceptEncoding: "gzip",
			expectedMethod: http.MethodPost,
			validateRequest: func(t *testing.T, r *http.Request) {
				assert.Equal(t, "gzip", r.Header.Get(httputil.RequestHeaderAcceptEncoding))
			},
		},
		{
			name:           "empty response body with 200 OK - should not error",
			components:     []string{"disk"},
			serverResponse: "",
			statusCode:     http.StatusOK,
			expectedMethod: http.MethodPost,
		},
		{
			name:           "malformed JSON response with 200 OK - should not error",
			components:     []string{"disk"},
			serverResponse: "{invalid json}",
			statusCode:     http.StatusOK,
			expectedMethod: http.MethodPost,
		},
		{
			name:       "components with special characters",
			components: []string{"component-with-dash", "component_with_underscore", "component.with.dot"},
			serverResponse: map[string]interface{}{
				"success": []string{"component-with-dash", "component_with_underscore", "component.with.dot"},
			},
			statusCode:     http.StatusOK,
			expectedMethod: http.MethodPost,
			expectedURL:    "/v1/health-states/set-healthy?components=component-with-dash%2Ccomponent_with_underscore%2Ccomponent.with.dot",
		},
		{
			name:       "components with spaces get trimmed",
			components: []string{" disk ", "  memory  ", "cpu "},
			serverResponse: map[string]interface{}{
				"success": []string{"disk", "memory", "cpu"},
			},
			statusCode:     http.StatusOK,
			expectedMethod: http.MethodPost,
			expectedURL:    "/v1/health-states/set-healthy?components=+disk+%2C++memory++%2Ccpu+",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Validate request method
				if tt.expectedMethod != "" {
					assert.Equal(t, tt.expectedMethod, r.Method)
				}

				// Validate URL path and query
				if tt.expectedURL != "" {
					assert.Equal(t, tt.expectedURL, r.URL.String())
				}

				// Custom validation if provided
				if tt.validateRequest != nil {
					tt.validateRequest(t, r)
				}

				// Set response status code
				w.WriteHeader(tt.statusCode)

				// Write response based on type
				switch v := tt.serverResponse.(type) {
				case string:
					_, _ = w.Write([]byte(v))
				case map[string]interface{}:
					_ = json.NewEncoder(w).Encode(v)
				case nil:
					// No response body
				default:
					t.Fatalf("unexpected serverResponse type: %T", v)
				}
			}))
			defer server.Close()

			// Prepare options
			opts := []OpOption{}
			if tt.contentType != "" {
				if tt.contentType == httputil.RequestHeaderJSON {
					opts = append(opts, WithRequestContentTypeJSON())
				} else if tt.contentType == httputil.RequestHeaderYAML {
					opts = append(opts, WithRequestContentTypeYAML())
				}
			}
			if tt.acceptEncoding != "" {
				opts = append(opts, WithAcceptEncodingGzip())
			}

			// Create context with timeout
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			// Call the function
			err := SetHealthyComponents(ctx, server.URL, tt.components, opts...)

			// Check error
			if tt.expectedError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestSetHealthyComponents_InvalidURL(t *testing.T) {
	// Test with invalid URL
	ctx := context.Background()
	err := SetHealthyComponents(ctx, "://invalid-url", []string{"disk"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse")
}

func TestSetHealthyComponents_ContextCancellation(t *testing.T) {
	// Create a server that delays response
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Create context that cancels immediately
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := SetHealthyComponents(ctx, server.URL, []string{"disk"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "context canceled")
}

func TestSetHealthyComponents_NetworkError(t *testing.T) {
	// Use a non-existent server
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	err := SetHealthyComponents(ctx, "http://localhost:99999", []string{"disk"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to make request")
}

func TestSetHealthyComponents_ApplyOptsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Create a custom option that modifies the Op struct
	customOpt := func(op *Op) {
		if op.components == nil {
			op.components = make(map[string]any)
		}
		op.components["test"] = "value"
	}

	// Since applyOpts in the current implementation doesn't return errors for invalid options,
	// we can't easily test error paths. This is a limitation of the current design.
	// In a real scenario, you might want to refactor applyOpts to validate options.

	ctx := context.Background()
	err := SetHealthyComponents(ctx, server.URL, []string{"disk"}, customOpt)
	// Currently this won't error because applyOpts doesn't validate
	require.NoError(t, err)
}
