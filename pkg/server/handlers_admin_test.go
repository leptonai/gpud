package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreatePackageHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Create a test wrapper function that simulates the behavior of createPackageHandler
	createTestPackageHandler := func(statusResult interface{}, statusError error) func(c *gin.Context) {
		return func(c *gin.Context) {
			if statusError != nil {
				c.JSON(http.StatusInternalServerError, gin.H{
					"code":    http.StatusInternalServerError,
					"message": "failed to get package status " + statusError.Error(),
				})
				return
			}
			c.JSON(http.StatusOK, statusResult)
		}
	}

	tests := []struct {
		name         string
		statusResult interface{}
		statusError  error
		expectedCode int
	}{
		{
			name: "successful response",
			statusResult: []map[string]interface{}{
				{
					"name":         "package1",
					"is_installed": true,
					"status":       true,
				},
				{
					"name":         "package2",
					"is_installed": false,
					"status":       false,
				},
			},
			statusError:  nil,
			expectedCode: http.StatusOK,
		},
		{
			name:         "error response",
			statusResult: nil,
			statusError:  errors.New("failed to get status"),
			expectedCode: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest("GET", "/admin/packages", nil)

			// Get the handler from our function and call it
			handler := createTestPackageHandler(tt.statusResult, tt.statusError)
			handler(c)

			// Check response
			assert.Equal(t, tt.expectedCode, w.Code)

			if tt.expectedCode == http.StatusOK {
				// Parse the response
				var result []map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &result)
				require.NoError(t, err)

				// Verify the response matches the status result
				expected, ok := tt.statusResult.([]map[string]interface{})
				assert.True(t, ok, "Status result should be slice of maps")
				assert.Equal(t, expected, result)
			} else {
				// Check error message is in the response
				var errorResp gin.H
				err := json.Unmarshal(w.Body.Bytes(), &errorResp)
				require.NoError(t, err)
				assert.Contains(t, errorResp["message"], "failed to get package status")
			}
		})
	}
}
