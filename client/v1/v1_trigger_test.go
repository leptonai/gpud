package v1

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v1 "github.com/leptonai/gpud/api/v1"
)

func TestTriggerComponentCheck(t *testing.T) {
	testHealthStates := v1.HealthStates{
		{
			Name: "test-state",
			ExtraInfo: map[string]string{
				"key": "value",
			},
		},
	}

	tests := []struct {
		name           string
		componentName  string
		serverResponse []byte
		contentType    string
		acceptEncoding string
		statusCode     int
		expectedError  string
		expectedResult v1.HealthStates
	}{
		{
			name:           "successful trigger check",
			componentName:  "test-component",
			serverResponse: mustMarshalJSON(t, testHealthStates),
			contentType:    RequestHeaderJSON,
			statusCode:     http.StatusOK,
			expectedResult: testHealthStates,
		},
		{
			name:          "empty component name",
			componentName: "",
			expectedError: "component name is required",
		},
		{
			name:          "server error",
			componentName: "test-component",
			statusCode:    http.StatusInternalServerError,
			expectedError: "server not ready, response not 200",
		},
		{
			name:           "invalid JSON response",
			componentName:  "test-component",
			serverResponse: []byte(`invalid json`),
			contentType:    RequestHeaderJSON,
			statusCode:     http.StatusOK,
			expectedError:  "failed to decode json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.componentName == "" {
				// Test validation error without creating a server
				result, err := TriggerComponentCheck(context.Background(), "http://localhost:8080", tt.componentName)
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
				assert.Nil(t, result)
				return
			}

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "/v1/components/trigger-check", r.URL.Path)
				assert.Equal(t, http.MethodGet, r.Method)
				assert.Equal(t, tt.componentName, r.URL.Query().Get("componentName"))

				if tt.contentType != "" {
					assert.Equal(t, tt.contentType, r.Header.Get(RequestHeaderContentType))
				}
				if tt.acceptEncoding != "" {
					assert.Equal(t, tt.acceptEncoding, r.Header.Get(RequestHeaderAcceptEncoding))
				}

				w.WriteHeader(tt.statusCode)
				if tt.serverResponse != nil {
					_, err := w.Write(tt.serverResponse)
					require.NoError(t, err)
				}
			}))
			defer srv.Close()

			opts := []OpOption{}
			if tt.contentType == RequestHeaderYAML {
				opts = append(opts, WithRequestContentTypeYAML())
			} else if tt.contentType == RequestHeaderJSON {
				opts = append(opts, WithRequestContentTypeJSON())
			}
			if tt.acceptEncoding == RequestHeaderEncodingGzip {
				opts = append(opts, WithAcceptEncodingGzip())
			}

			result, err := TriggerComponentCheck(context.Background(), srv.URL, tt.componentName, opts...)
			if tt.expectedError != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expectedResult, result)
		})
	}
}
