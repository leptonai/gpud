package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/leptonai/gpud/pkg/config"
)

// TestHandleMachineInfoWithNilGPUdInstance tests the behavior when gpudInstance is nil
func TestHandleMachineInfoWithNilGPUdInstance(t *testing.T) {
	// Setup
	gin.SetMode(gin.TestMode)
	router := gin.New()

	// Create a handler with nil gpudInstance
	handler := &globalHandler{
		gpudInstance: nil,
	}

	// Register the handler
	router.GET("/machine-info", handler.handleMachineInfo)

	// Create a test request
	req, _ := http.NewRequest(http.MethodGet, "/machine-info", nil)
	resp := httptest.NewRecorder()

	// Send the request
	router.ServeHTTP(resp, req)

	// Assert the response status code
	require.Equal(t, http.StatusNotFound, resp.Code)

	// Parse the response
	var response map[string]interface{}
	err := json.Unmarshal(resp.Body.Bytes(), &response)
	require.NoError(t, err)

	// Check that the error message is as expected
	assert.Equal(t, "gpud instance not found", response["message"])
}

func TestHandleMachineInfoRouteRegistration(t *testing.T) {
	// Setup a complete handler to test route registration
	registry := newMockRegistry()
	cfg := &config.Config{}
	store := &mockMetricsStore{}

	handler := newGlobalHandler(cfg, registry, store, nil, nil)

	// Setup router
	router := gin.New()
	v1 := router.Group("/v1")

	// Register machine info route (this would be done in the main server setup)
	v1.GET(URLPathMachineInfo, handler.handleMachineInfo)

	// Create a test server
	server := httptest.NewServer(router)
	defer server.Close()

	// Test the machine info endpoint
	resp, err := http.Get(server.URL + "/v1" + URLPathMachineInfo)
	require.NoError(t, err)
	defer resp.Body.Close()

	// Should get a response (404 because gpudInstance is nil, but route is registered)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestHandleMachineInfoHTTPMethods(t *testing.T) {
	// Setup
	gin.SetMode(gin.TestMode)
	router := gin.New()

	// Create a handler with nil gpudInstance (will return 404)
	handler := &globalHandler{
		gpudInstance: nil,
	}

	// Register the handler
	router.GET("/machine-info", handler.handleMachineInfo)

	testCases := []struct {
		method         string
		expectedStatus int
		shouldHaveBody bool
	}{
		{"GET", http.StatusNotFound, true},
		{"POST", http.StatusNotFound, false},   // Gin returns 404 for unregistered routes
		{"PUT", http.StatusNotFound, false},    // Gin returns 404 for unregistered routes
		{"DELETE", http.StatusNotFound, false}, // Gin returns 404 for unregistered routes
	}

	for _, tc := range testCases {
		t.Run(tc.method, func(t *testing.T) {
			req, _ := http.NewRequest(tc.method, "/machine-info", nil)
			resp := httptest.NewRecorder()

			router.ServeHTTP(resp, req)

			assert.Equal(t, tc.expectedStatus, resp.Code)

			if tc.shouldHaveBody {
				assert.Greater(t, resp.Body.Len(), 0)
			}
		})
	}
}

func TestHandleMachineInfoContentType(t *testing.T) {
	// Setup
	gin.SetMode(gin.TestMode)
	router := gin.New()

	// Create a handler with nil gpudInstance to avoid panic
	handler := &globalHandler{
		gpudInstance: nil,
	}

	// Register the handler
	router.GET("/machine-info", handler.handleMachineInfo)

	// Create a test request
	req, _ := http.NewRequest(http.MethodGet, "/machine-info", nil)
	req.Header.Set("Accept", "application/json")
	resp := httptest.NewRecorder()

	// Send the request
	router.ServeHTTP(resp, req)

	// Should get 404 because gpudInstance is nil, but we can still check the response
	assert.Equal(t, http.StatusNotFound, resp.Code)

	// Check that we get a JSON response even for errors
	assert.Contains(t, resp.Header().Get("Content-Type"), "application/json")
}

func TestHandleMachineInfoErrorResponse(t *testing.T) {
	// Setup
	gin.SetMode(gin.TestMode)
	router := gin.New()

	// Create a handler with nil gpudInstance to trigger error
	handler := &globalHandler{
		gpudInstance: nil,
	}

	// Register the handler
	router.GET("/machine-info", handler.handleMachineInfo)

	// Create a test request
	req, _ := http.NewRequest(http.MethodGet, "/machine-info", nil)
	resp := httptest.NewRecorder()

	// Send the request
	router.ServeHTTP(resp, req)

	// Should get 404 error
	assert.Equal(t, http.StatusNotFound, resp.Code)

	// Parse the error response
	var response map[string]interface{}
	err := json.Unmarshal(resp.Body.Bytes(), &response)
	require.NoError(t, err)

	// Check error structure
	assert.Contains(t, response, "message")
	assert.Contains(t, response, "code")
	assert.Equal(t, "gpud instance not found", response["message"])
}
