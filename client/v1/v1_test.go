package v1

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/yaml"

	components "github.com/leptonai/gpud/api/v1"
	v1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/pkg/errdefs"
	"github.com/leptonai/gpud/pkg/server"
)

func gzipContent(t *testing.T, data []byte) []byte {
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	_, err := gw.Write(data)
	require.NoError(t, err)
	require.NoError(t, gw.Close())
	return buf.Bytes()
}

func TestGetComponents(t *testing.T) {
	testComponents := []string{"comp1", "comp2", "comp3"}

	tests := []struct {
		name           string
		serverResponse []byte
		contentType    string
		acceptEncoding string
		statusCode     int
		expectedError  string
		expectedResult []string
		useGzip        bool
	}{
		{
			name:           "successful JSON response",
			serverResponse: []byte(`["comp1","comp2","comp3"]`),
			contentType:    server.RequestHeaderJSON,
			statusCode:     http.StatusOK,
			expectedResult: testComponents,
		},
		{
			name:           "successful YAML response",
			serverResponse: []byte("- comp1\n- comp2\n- comp3\n"),
			contentType:    server.RequestHeaderYAML,
			statusCode:     http.StatusOK,
			expectedResult: testComponents,
		},
		{
			name:           "successful gzipped JSON response",
			serverResponse: []byte(`["comp1","comp2","comp3"]`),
			contentType:    server.RequestHeaderJSON,
			acceptEncoding: server.RequestHeaderEncodingGzip,
			statusCode:     http.StatusOK,
			expectedResult: testComponents,
			useGzip:        true,
		},
		{
			name:           "server error",
			serverResponse: []byte(`internal error`),
			statusCode:     http.StatusInternalServerError,
			expectedError:  "server not ready, response not 200",
		},
		{
			name:           "invalid JSON response",
			serverResponse: []byte(`invalid json`),
			contentType:    server.RequestHeaderJSON,
			statusCode:     http.StatusOK,
			expectedError:  "failed to decode json",
		},
		{
			name:           "invalid YAML response",
			serverResponse: []byte(`invalid yaml:`),
			contentType:    server.RequestHeaderYAML,
			statusCode:     http.StatusOK,
			expectedError:  "failed to unmarshal yaml",
		},
		{
			name:           "unsupported content type",
			serverResponse: []byte(`[]`),
			contentType:    "application/xml",
			statusCode:     http.StatusOK,
			expectedError:  "unsupported content type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "/v1/components", r.URL.Path)
				assert.Equal(t, http.MethodGet, r.Method)

				contentType := r.Header.Get(server.RequestHeaderContentType)
				if tt.contentType != "" {
					assert.Equal(t, tt.contentType, contentType)
				}
				if tt.acceptEncoding != "" {
					assert.Equal(t, tt.acceptEncoding, r.Header.Get(server.RequestHeaderAcceptEncoding))
				}

				// Set response content type
				if tt.contentType != "" {
					w.Header().Set(server.RequestHeaderContentType, tt.contentType)
				}

				w.WriteHeader(tt.statusCode)
				if tt.useGzip {
					_, err := w.Write(gzipContent(t, tt.serverResponse))
					require.NoError(t, err)
				} else {
					_, err := w.Write(tt.serverResponse)
					require.NoError(t, err)
				}
			}))
			defer srv.Close()

			opts := []OpOption{}
			if tt.contentType == server.RequestHeaderYAML {
				opts = append(opts, WithRequestContentTypeYAML())
			} else if tt.contentType == server.RequestHeaderJSON {
				opts = append(opts, WithRequestContentTypeJSON())
			} else if tt.contentType != "" {
				opts = append(opts, func(op *Op) {
					op.requestContentType = tt.contentType
				})
			}
			if tt.acceptEncoding == server.RequestHeaderEncodingGzip {
				opts = append(opts, WithAcceptEncodingGzip())
			}

			result, err := GetComponents(context.Background(), srv.URL, opts...)
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

func TestGetInfo(t *testing.T) {
	now := time.Now().UTC()
	testInfo := v1.LeptonInfo{
		{
			Component: "component1",
			StartTime: now,
			EndTime:   now.Add(time.Hour),
			Info: components.Info{
				States: []components.State{
					{
						Name:      "test",
						ExtraInfo: map[string]string{"key": "value"},
					},
				},
			},
		},
	}

	tests := []struct {
		name           string
		components     []string
		serverResponse []byte
		contentType    string
		acceptEncoding string
		statusCode     int
		expectedError  string
		expectedResult v1.LeptonInfo
		useGzip        bool
	}{
		{
			name:           "successful JSON response",
			serverResponse: mustMarshalJSON(t, testInfo),
			contentType:    server.RequestHeaderJSON,
			statusCode:     http.StatusOK,
			expectedResult: testInfo,
		},
		{
			name:           "successful YAML response",
			serverResponse: mustMarshalYAML(t, testInfo),
			contentType:    server.RequestHeaderYAML,
			statusCode:     http.StatusOK,
			expectedResult: testInfo,
		},
		{
			name:           "with components filter",
			components:     []string{"comp1", "comp2"},
			serverResponse: mustMarshalJSON(t, testInfo),
			contentType:    server.RequestHeaderJSON,
			statusCode:     http.StatusOK,
			expectedResult: testInfo,
		},
		{
			name:           "server error",
			serverResponse: []byte(`internal error`),
			statusCode:     http.StatusInternalServerError,
			expectedError:  "server not ready, response not 200",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "/v1/info", r.URL.Path)
				assert.Equal(t, http.MethodGet, r.Method)

				if tt.components != nil {
					assert.Contains(t, r.URL.RawQuery, "components=")
				}

				if tt.contentType != "" {
					assert.Equal(t, tt.contentType, r.Header.Get(server.RequestHeaderContentType))
					w.Header().Set(server.RequestHeaderContentType, tt.contentType)
				}
				if tt.acceptEncoding != "" {
					assert.Equal(t, tt.acceptEncoding, r.Header.Get(server.RequestHeaderAcceptEncoding))
				}

				w.WriteHeader(tt.statusCode)
				if tt.useGzip {
					_, err := w.Write(gzipContent(t, tt.serverResponse))
					require.NoError(t, err)
				} else {
					_, err := w.Write(tt.serverResponse)
					require.NoError(t, err)
				}
			}))
			defer srv.Close()

			opts := []OpOption{}
			if tt.contentType == server.RequestHeaderYAML {
				opts = append(opts, WithRequestContentTypeYAML())
			} else if tt.contentType == server.RequestHeaderJSON {
				opts = append(opts, WithRequestContentTypeJSON())
			}
			if tt.acceptEncoding == server.RequestHeaderEncodingGzip {
				opts = append(opts, WithAcceptEncodingGzip())
			}
			for _, comp := range tt.components {
				opts = append(opts, WithComponent(comp))
			}

			result, err := GetInfo(context.Background(), srv.URL, opts...)
			if tt.expectedError != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
				return
			}

			require.NoError(t, err)
			// Compare fields individually to avoid time comparison issues
			require.Equal(t, len(tt.expectedResult), len(result))
			for i := range tt.expectedResult {
				assert.Equal(t, tt.expectedResult[i].Component, result[i].Component)
				assert.WithinDuration(t, tt.expectedResult[i].StartTime, result[i].StartTime, time.Second)
				assert.WithinDuration(t, tt.expectedResult[i].EndTime, result[i].EndTime, time.Second)
				assert.Equal(t, tt.expectedResult[i].Info, result[i].Info)
			}
		})
	}
}

func TestGetStates(t *testing.T) {
	testStates := v1.LeptonStates{
		{
			Component: "component1",
			States: []components.State{
				{
					Name:      "test",
					ExtraInfo: map[string]string{"state": "running"},
				},
			},
		},
	}

	tests := []struct {
		name           string
		components     []string
		serverResponse []byte
		contentType    string
		acceptEncoding string
		statusCode     int
		expectedError  string
		expectedResult v1.LeptonStates
		useGzip        bool
	}{
		{
			name:           "successful JSON response",
			serverResponse: mustMarshalJSON(t, testStates),
			contentType:    server.RequestHeaderJSON,
			statusCode:     http.StatusOK,
			expectedResult: testStates,
		},
		{
			name:           "not found error",
			serverResponse: []byte(`not found`),
			statusCode:     http.StatusNotFound,
			expectedError:  errdefs.ErrNotFound.Error(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "/v1/states", r.URL.Path)
				assert.Equal(t, http.MethodGet, r.Method)

				if tt.components != nil {
					assert.Contains(t, r.URL.RawQuery, "components=")
				}

				if tt.contentType != "" {
					assert.Equal(t, tt.contentType, r.Header.Get(server.RequestHeaderContentType))
					w.Header().Set(server.RequestHeaderContentType, tt.contentType)
				}
				if tt.acceptEncoding != "" {
					assert.Equal(t, tt.acceptEncoding, r.Header.Get(server.RequestHeaderAcceptEncoding))
				}

				w.WriteHeader(tt.statusCode)
				if tt.useGzip {
					_, err := w.Write(gzipContent(t, tt.serverResponse))
					require.NoError(t, err)
				} else {
					_, err := w.Write(tt.serverResponse)
					require.NoError(t, err)
				}
			}))
			defer srv.Close()

			opts := []OpOption{}
			if tt.contentType == server.RequestHeaderYAML {
				opts = append(opts, WithRequestContentTypeYAML())
			} else if tt.contentType == server.RequestHeaderJSON {
				opts = append(opts, WithRequestContentTypeJSON())
			}
			if tt.acceptEncoding == server.RequestHeaderEncodingGzip {
				opts = append(opts, WithAcceptEncodingGzip())
			}
			for _, comp := range tt.components {
				opts = append(opts, WithComponent(comp))
			}

			result, err := GetStates(context.Background(), srv.URL, opts...)
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

func TestReadComponents(t *testing.T) {
	testComponents := []string{"comp1", "comp2", "comp3"}
	jsonData := mustMarshalJSON(t, testComponents)
	yamlData := mustMarshalYAML(t, testComponents)

	tests := []struct {
		name           string
		input          io.Reader
		contentType    string
		acceptEncoding string
		expectedError  string
		expectedResult []string
	}{
		{
			name:           "read JSON",
			input:          bytes.NewReader(jsonData),
			contentType:    server.RequestHeaderJSON,
			expectedResult: testComponents,
		},
		{
			name:           "read YAML",
			input:          bytes.NewReader(yamlData),
			contentType:    server.RequestHeaderYAML,
			expectedResult: testComponents,
		},
		{
			name:           "read gzipped JSON",
			input:          bytes.NewReader(gzipContent(t, jsonData)),
			contentType:    server.RequestHeaderJSON,
			acceptEncoding: server.RequestHeaderEncodingGzip,
			expectedResult: testComponents,
		},
		{
			name:          "invalid content type",
			input:         bytes.NewReader(jsonData),
			contentType:   "invalid",
			expectedError: "unsupported content type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := []OpOption{}
			if tt.contentType != "" {
				if tt.contentType == server.RequestHeaderYAML {
					opts = append(opts, WithRequestContentTypeYAML())
				} else if tt.contentType == server.RequestHeaderJSON {
					opts = append(opts, WithRequestContentTypeJSON())
				} else {
					opts = append(opts, func(op *Op) {
						op.requestContentType = tt.contentType
					})
				}
			}
			if tt.acceptEncoding != "" {
				opts = append(opts, WithAcceptEncodingGzip())
			}

			result, err := ReadComponents(tt.input, opts...)
			if tt.expectedError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expectedResult, result)
		})
	}
}

// Helper functions for marshaling test data
func mustMarshalJSON(t *testing.T, v interface{}) []byte {
	data, err := json.Marshal(v)
	require.NoError(t, err)
	return data
}

func mustMarshalYAML(t *testing.T, v interface{}) []byte {
	data, err := yaml.Marshal(v)
	require.NoError(t, err)
	return data
}
