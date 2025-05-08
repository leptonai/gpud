package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
