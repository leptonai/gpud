package v1

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/pkg/httputil"
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
	testGPUdHealthStates := v1.GPUdComponentHealthStates{
		{
			Component: "test-component",
			States:    testHealthStates,
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
		expectedResult v1.GPUdComponentHealthStates
	}{
		{
			name:           "successful trigger check",
			componentName:  "test-component",
			serverResponse: mustMarshalJSON(t, testGPUdHealthStates),
			contentType:    httputil.RequestHeaderJSON,
			statusCode:     http.StatusOK,
			expectedResult: testGPUdHealthStates,
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
			contentType:    httputil.RequestHeaderJSON,
			statusCode:     http.StatusOK,
			expectedError:  "failed to decode json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.componentName == "" {
				// Test validation error without creating a server
				result, err := TriggerComponent(context.Background(), "http://localhost:8080", tt.componentName)
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
					assert.Equal(t, tt.contentType, r.Header.Get(httputil.RequestHeaderContentType))
				}
				if tt.acceptEncoding != "" {
					assert.Equal(t, tt.acceptEncoding, r.Header.Get(httputil.RequestHeaderAcceptEncoding))
				}

				w.WriteHeader(tt.statusCode)
				if tt.serverResponse != nil {
					_, err := w.Write(tt.serverResponse)
					require.NoError(t, err)
				}
			}))
			defer srv.Close()

			opts := []OpOption{}
			switch tt.contentType {
			case httputil.RequestHeaderYAML:
				opts = append(opts, WithRequestContentTypeYAML())
			case httputil.RequestHeaderJSON:
				opts = append(opts, WithRequestContentTypeJSON())
			}
			if tt.acceptEncoding == httputil.RequestHeaderEncodingGzip {
				opts = append(opts, WithAcceptEncodingGzip())
			}

			result, err := TriggerComponent(context.Background(), srv.URL, tt.componentName, opts...)
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
