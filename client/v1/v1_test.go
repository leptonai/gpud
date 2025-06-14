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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/pkg/errdefs"
	"github.com/leptonai/gpud/pkg/httputil"
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
			contentType:    httputil.RequestHeaderJSON,
			statusCode:     http.StatusOK,
			expectedResult: testComponents,
		},
		{
			name:           "successful YAML response",
			serverResponse: []byte("- comp1\n- comp2\n- comp3\n"),
			contentType:    httputil.RequestHeaderYAML,
			statusCode:     http.StatusOK,
			expectedResult: testComponents,
		},
		{
			name:           "successful gzipped JSON response",
			serverResponse: []byte(`["comp1","comp2","comp3"]`),
			contentType:    httputil.RequestHeaderJSON,
			acceptEncoding: httputil.RequestHeaderEncodingGzip,
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
			contentType:    httputil.RequestHeaderJSON,
			statusCode:     http.StatusOK,
			expectedError:  "failed to decode json",
		},
		{
			name:           "invalid YAML response",
			serverResponse: []byte(`invalid yaml:`),
			contentType:    httputil.RequestHeaderYAML,
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

				contentType := r.Header.Get(httputil.RequestHeaderContentType)
				if tt.contentType != "" {
					assert.Equal(t, tt.contentType, contentType)
				}
				if tt.acceptEncoding != "" {
					assert.Equal(t, tt.acceptEncoding, r.Header.Get(httputil.RequestHeaderAcceptEncoding))
				}

				// Set response content type
				if tt.contentType != "" {
					w.Header().Set(httputil.RequestHeaderContentType, tt.contentType)
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
			if tt.contentType == httputil.RequestHeaderYAML {
				opts = append(opts, WithRequestContentTypeYAML())
			} else if tt.contentType == httputil.RequestHeaderJSON {
				opts = append(opts, WithRequestContentTypeJSON())
			} else if tt.contentType != "" {
				opts = append(opts, func(op *Op) {
					op.requestContentType = tt.contentType
				})
			}
			if tt.acceptEncoding == httputil.RequestHeaderEncodingGzip {
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
	testInfo := apiv1.GPUdComponentInfos{
		{
			Component: "component1",
			StartTime: now,
			EndTime:   now.Add(time.Hour),
			Info: apiv1.Info{
				States: []apiv1.HealthState{
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
		expectedResult apiv1.GPUdComponentInfos
		useGzip        bool
	}{
		{
			name:           "successful JSON response",
			serverResponse: mustMarshalJSON(t, testInfo),
			contentType:    httputil.RequestHeaderJSON,
			statusCode:     http.StatusOK,
			expectedResult: testInfo,
		},
		{
			name:           "successful YAML response",
			serverResponse: mustMarshalYAML(t, testInfo),
			contentType:    httputil.RequestHeaderYAML,
			statusCode:     http.StatusOK,
			expectedResult: testInfo,
		},
		{
			name:           "with components filter",
			components:     []string{"comp1", "comp2"},
			serverResponse: mustMarshalJSON(t, testInfo),
			contentType:    httputil.RequestHeaderJSON,
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
					assert.Equal(t, tt.contentType, r.Header.Get(httputil.RequestHeaderContentType))
					w.Header().Set(httputil.RequestHeaderContentType, tt.contentType)
				}
				if tt.acceptEncoding != "" {
					assert.Equal(t, tt.acceptEncoding, r.Header.Get(httputil.RequestHeaderAcceptEncoding))
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
			if tt.contentType == httputil.RequestHeaderYAML {
				opts = append(opts, WithRequestContentTypeYAML())
			} else if tt.contentType == httputil.RequestHeaderJSON {
				opts = append(opts, WithRequestContentTypeJSON())
			}
			if tt.acceptEncoding == httputil.RequestHeaderEncodingGzip {
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
	testStates := apiv1.GPUdComponentHealthStates{
		{
			Component: "component1",
			States: []apiv1.HealthState{
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
		expectedResult apiv1.GPUdComponentHealthStates
		useGzip        bool
	}{
		{
			name:           "successful JSON response",
			serverResponse: mustMarshalJSON(t, testStates),
			contentType:    httputil.RequestHeaderJSON,
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
					assert.Equal(t, tt.contentType, r.Header.Get(httputil.RequestHeaderContentType))
					w.Header().Set(httputil.RequestHeaderContentType, tt.contentType)
				}
				if tt.acceptEncoding != "" {
					assert.Equal(t, tt.acceptEncoding, r.Header.Get(httputil.RequestHeaderAcceptEncoding))
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
			if tt.contentType == httputil.RequestHeaderYAML {
				opts = append(opts, WithRequestContentTypeYAML())
			} else if tt.contentType == httputil.RequestHeaderJSON {
				opts = append(opts, WithRequestContentTypeJSON())
			}
			if tt.acceptEncoding == httputil.RequestHeaderEncodingGzip {
				opts = append(opts, WithAcceptEncodingGzip())
			}
			for _, comp := range tt.components {
				opts = append(opts, WithComponent(comp))
			}

			result, err := GetHealthStates(context.Background(), srv.URL, opts...)
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
			contentType:    httputil.RequestHeaderJSON,
			expectedResult: testComponents,
		},
		{
			name:           "read YAML",
			input:          bytes.NewReader(yamlData),
			contentType:    httputil.RequestHeaderYAML,
			expectedResult: testComponents,
		},
		{
			name:           "read gzipped JSON",
			input:          bytes.NewReader(gzipContent(t, jsonData)),
			contentType:    httputil.RequestHeaderJSON,
			acceptEncoding: httputil.RequestHeaderEncodingGzip,
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
				if tt.contentType == httputil.RequestHeaderYAML {
					opts = append(opts, WithRequestContentTypeYAML())
				} else if tt.contentType == httputil.RequestHeaderJSON {
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

func TestDeregisterComponent(t *testing.T) {
	tests := []struct {
		name           string
		componentName  string
		statusCode     int
		contentType    string
		acceptEncoding string
		expectedError  string
	}{
		{
			name:          "successful deregistration",
			componentName: "test-component",
			statusCode:    http.StatusOK,
			contentType:   httputil.RequestHeaderJSON,
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.componentName == "" {
				// Test validation error without creating a server
				err := DeregisterComponent(context.Background(), "http://localhost:8080", tt.componentName)
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
				return
			}

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "/v1/components", r.URL.Path)
				assert.Equal(t, http.MethodDelete, r.Method)
				assert.Equal(t, tt.componentName, r.URL.Query().Get("componentName"))

				if tt.contentType != "" {
					assert.Equal(t, tt.contentType, r.Header.Get(httputil.RequestHeaderContentType))
				}
				if tt.acceptEncoding != "" {
					assert.Equal(t, tt.acceptEncoding, r.Header.Get(httputil.RequestHeaderAcceptEncoding))
				}

				w.WriteHeader(tt.statusCode)
				_, err := w.Write([]byte("ok"))
				require.NoError(t, err)
			}))
			defer srv.Close()

			opts := []OpOption{}
			if tt.contentType == httputil.RequestHeaderYAML {
				opts = append(opts, WithRequestContentTypeYAML())
			} else if tt.contentType == httputil.RequestHeaderJSON {
				opts = append(opts, WithRequestContentTypeJSON())
			}
			if tt.acceptEncoding == httputil.RequestHeaderEncodingGzip {
				opts = append(opts, WithAcceptEncodingGzip())
			}

			err := DeregisterComponent(context.Background(), srv.URL, tt.componentName, opts...)
			if tt.expectedError != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
				return
			}

			require.NoError(t, err)
		})
	}
}

func TestGetEvents(t *testing.T) {
	now := time.Now().UTC()
	testEvents := apiv1.GPUdComponentEvents{
		{
			Component: "component1",
			StartTime: now,
			EndTime:   now.Add(time.Hour),
			Events: []apiv1.Event{
				{
					Name: "test-event",
					Time: metav1.Time{Time: now},
					Type: apiv1.EventTypeInfo,
				},
			},
		},
	}

	tests := []struct {
		name           string
		serverResponse []byte
		contentType    string
		acceptEncoding string
		statusCode     int
		expectedError  string
		expectedResult apiv1.GPUdComponentEvents
		useGzip        bool
	}{
		{
			name:           "successful JSON response",
			serverResponse: mustMarshalJSON(t, testEvents),
			contentType:    httputil.RequestHeaderJSON,
			statusCode:     http.StatusOK,
			expectedResult: testEvents,
		},
		{
			name:           "successful YAML response",
			serverResponse: mustMarshalYAML(t, testEvents),
			contentType:    httputil.RequestHeaderYAML,
			statusCode:     http.StatusOK,
			expectedResult: testEvents,
		},
		{
			name:           "successful gzipped JSON response",
			serverResponse: mustMarshalJSON(t, testEvents),
			contentType:    httputil.RequestHeaderJSON,
			acceptEncoding: httputil.RequestHeaderEncodingGzip,
			statusCode:     http.StatusOK,
			expectedResult: testEvents,
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
			contentType:    httputil.RequestHeaderJSON,
			statusCode:     http.StatusOK,
			expectedError:  "failed to decode json",
		},
		{
			name:           "invalid YAML response",
			serverResponse: []byte(`invalid yaml:`),
			contentType:    httputil.RequestHeaderYAML,
			statusCode:     http.StatusOK,
			expectedError:  "failed to unmarshal yaml",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "/v1/events", r.URL.Path)
				assert.Equal(t, http.MethodGet, r.Method)

				if tt.contentType != "" {
					assert.Equal(t, tt.contentType, r.Header.Get(httputil.RequestHeaderContentType))
					w.Header().Set(httputil.RequestHeaderContentType, tt.contentType)
				}
				if tt.acceptEncoding != "" {
					assert.Equal(t, tt.acceptEncoding, r.Header.Get(httputil.RequestHeaderAcceptEncoding))
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
			if tt.contentType == httputil.RequestHeaderYAML {
				opts = append(opts, WithRequestContentTypeYAML())
			} else if tt.contentType == httputil.RequestHeaderJSON {
				opts = append(opts, WithRequestContentTypeJSON())
			}
			if tt.acceptEncoding == httputil.RequestHeaderEncodingGzip {
				opts = append(opts, WithAcceptEncodingGzip())
			}

			result, err := GetEvents(context.Background(), srv.URL, opts...)
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
				assert.Equal(t, len(tt.expectedResult[i].Events), len(result[i].Events))
				for j := range tt.expectedResult[i].Events {
					assert.Equal(t, tt.expectedResult[i].Events[j].Name, result[i].Events[j].Name)
					assert.Equal(t, tt.expectedResult[i].Events[j].Type, result[i].Events[j].Type)
					assert.WithinDuration(t, tt.expectedResult[i].Events[j].Time.Time, result[i].Events[j].Time.Time, time.Second)
				}
			}
		})
	}

	// Test unsupported content type separately to avoid nil pointer issues
	t.Run("unsupported content type", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/v1/events", r.URL.Path)
			assert.Equal(t, http.MethodGet, r.Method)
			w.Header().Set(httputil.RequestHeaderContentType, "application/xml")
			w.WriteHeader(http.StatusOK)
			_, err := w.Write([]byte(`<events></events>`))
			require.NoError(t, err)
		}))
		defer srv.Close()

		// Create custom option to set a content type we control
		customOption := func(op *Op) {
			op.requestContentType = "application/xml"
		}

		result, err := GetEvents(context.Background(), srv.URL, customOption)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported content type")
		assert.Nil(t, result)
	})
}

func TestReadEvents(t *testing.T) {
	now := time.Now().UTC()
	testEvents := apiv1.GPUdComponentEvents{
		{
			Component: "component1",
			StartTime: now,
			EndTime:   now.Add(time.Hour),
			Events: []apiv1.Event{
				{
					Name: "test-event",
					Time: metav1.Time{Time: now},
					Type: apiv1.EventTypeInfo,
				},
			},
		},
	}
	jsonData := mustMarshalJSON(t, testEvents)
	yamlData := mustMarshalYAML(t, testEvents)

	tests := []struct {
		name           string
		input          io.Reader
		contentType    string
		acceptEncoding string
		expectedError  string
		expectedResult apiv1.GPUdComponentEvents
	}{
		{
			name:           "read JSON",
			input:          bytes.NewReader(jsonData),
			contentType:    httputil.RequestHeaderJSON,
			expectedResult: testEvents,
		},
		{
			name:           "read YAML",
			input:          bytes.NewReader(yamlData),
			contentType:    httputil.RequestHeaderYAML,
			expectedResult: testEvents,
		},
		{
			name:           "read gzipped JSON",
			input:          bytes.NewReader(gzipContent(t, jsonData)),
			contentType:    httputil.RequestHeaderJSON,
			acceptEncoding: httputil.RequestHeaderEncodingGzip,
			expectedResult: testEvents,
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
				if tt.contentType == httputil.RequestHeaderYAML {
					opts = append(opts, WithRequestContentTypeYAML())
				} else if tt.contentType == httputil.RequestHeaderJSON {
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

			result, err := ReadEvents(tt.input, opts...)
			if tt.expectedError != "" {
				require.Error(t, err)
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
				assert.Equal(t, len(tt.expectedResult[i].Events), len(result[i].Events))
				for j := range tt.expectedResult[i].Events {
					assert.Equal(t, tt.expectedResult[i].Events[j].Name, result[i].Events[j].Name)
					assert.Equal(t, tt.expectedResult[i].Events[j].Type, result[i].Events[j].Type)
					assert.WithinDuration(t, tt.expectedResult[i].Events[j].Time.Time, result[i].Events[j].Time.Time, time.Second)
				}
			}
		})
	}
}

func TestGetMetrics(t *testing.T) {
	testMetrics := apiv1.GPUdComponentMetrics{
		{
			Component: "component1",
			Metrics: []apiv1.Metric{
				{
					Name:        "test-metric",
					Value:       42.0,
					UnixSeconds: time.Now().Unix(),
					Labels:      map[string]string{"key": "value"},
				},
			},
		},
	}

	tests := []struct {
		name           string
		serverResponse []byte
		contentType    string
		acceptEncoding string
		statusCode     int
		expectedError  string
		expectedResult apiv1.GPUdComponentMetrics
		useGzip        bool
	}{
		{
			name:           "successful JSON response",
			serverResponse: mustMarshalJSON(t, testMetrics),
			contentType:    httputil.RequestHeaderJSON,
			statusCode:     http.StatusOK,
			expectedResult: testMetrics,
		},
		{
			name:           "successful YAML response",
			serverResponse: mustMarshalYAML(t, testMetrics),
			contentType:    httputil.RequestHeaderYAML,
			statusCode:     http.StatusOK,
			expectedResult: testMetrics,
		},
		{
			name:           "successful gzipped JSON response",
			serverResponse: mustMarshalJSON(t, testMetrics),
			contentType:    httputil.RequestHeaderJSON,
			acceptEncoding: httputil.RequestHeaderEncodingGzip,
			statusCode:     http.StatusOK,
			expectedResult: testMetrics,
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
			contentType:    httputil.RequestHeaderJSON,
			statusCode:     http.StatusOK,
			expectedError:  "failed to decode json",
		},
		{
			name:           "invalid YAML response",
			serverResponse: []byte(`invalid yaml:`),
			contentType:    httputil.RequestHeaderYAML,
			statusCode:     http.StatusOK,
			expectedError:  "failed to unmarshal yaml",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "/v1/metrics", r.URL.Path)
				assert.Equal(t, http.MethodGet, r.Method)

				if tt.contentType != "" {
					assert.Equal(t, tt.contentType, r.Header.Get(httputil.RequestHeaderContentType))
					w.Header().Set(httputil.RequestHeaderContentType, tt.contentType)
				}
				if tt.acceptEncoding != "" {
					assert.Equal(t, tt.acceptEncoding, r.Header.Get(httputil.RequestHeaderAcceptEncoding))
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
			if tt.contentType == httputil.RequestHeaderYAML {
				opts = append(opts, WithRequestContentTypeYAML())
			} else if tt.contentType == httputil.RequestHeaderJSON {
				opts = append(opts, WithRequestContentTypeJSON())
			}
			if tt.acceptEncoding == httputil.RequestHeaderEncodingGzip {
				opts = append(opts, WithAcceptEncodingGzip())
			}

			result, err := GetMetrics(context.Background(), srv.URL, opts...)
			if tt.expectedError != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
				return
			}

			require.NoError(t, err)
			require.Equal(t, len(tt.expectedResult), len(result))
			for i := range tt.expectedResult {
				assert.Equal(t, tt.expectedResult[i].Component, result[i].Component)
				assert.Equal(t, len(tt.expectedResult[i].Metrics), len(result[i].Metrics))
				for j := range tt.expectedResult[i].Metrics {
					assert.Equal(t, tt.expectedResult[i].Metrics[j].Name, result[i].Metrics[j].Name)
					assert.Equal(t, tt.expectedResult[i].Metrics[j].Value, result[i].Metrics[j].Value)
					assert.Equal(t, tt.expectedResult[i].Metrics[j].UnixSeconds, result[i].Metrics[j].UnixSeconds)
					assert.Equal(t, tt.expectedResult[i].Metrics[j].Labels, result[i].Metrics[j].Labels)
				}
			}
		})
	}

	// Test unsupported content type separately to avoid nil pointer issues
	t.Run("unsupported content type", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/v1/metrics", r.URL.Path)
			assert.Equal(t, http.MethodGet, r.Method)

			// Verify that the XML content type is sent
			contentType := r.Header.Get(httputil.RequestHeaderContentType)
			assert.Equal(t, "application/xml", contentType)

			w.Header().Set(httputil.RequestHeaderContentType, "application/xml")
			w.WriteHeader(http.StatusOK)
			_, err := w.Write([]byte(`<metrics></metrics>`))
			require.NoError(t, err)
		}))
		defer srv.Close()

		// Create custom option to set a content type we control
		customOption := func(op *Op) {
			op.requestContentType = "application/xml"
		}

		result, err := GetMetrics(context.Background(), srv.URL, customOption)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported content type")
		assert.Nil(t, result)
	})
}

func TestReadMetrics(t *testing.T) {
	testMetrics := apiv1.GPUdComponentMetrics{
		{
			Component: "component1",
			Metrics: []apiv1.Metric{
				{
					Name:        "test-metric",
					Value:       42.0,
					UnixSeconds: time.Now().Unix(),
					Labels:      map[string]string{"key": "value"},
				},
			},
		},
	}
	jsonData := mustMarshalJSON(t, testMetrics)
	yamlData := mustMarshalYAML(t, testMetrics)

	tests := []struct {
		name           string
		input          io.Reader
		contentType    string
		acceptEncoding string
		expectedError  string
		expectedResult apiv1.GPUdComponentMetrics
	}{
		{
			name:           "read JSON",
			input:          bytes.NewReader(jsonData),
			contentType:    httputil.RequestHeaderJSON,
			expectedResult: testMetrics,
		},
		{
			name:           "read YAML",
			input:          bytes.NewReader(yamlData),
			contentType:    httputil.RequestHeaderYAML,
			expectedResult: testMetrics,
		},
		{
			name:           "read gzipped JSON",
			input:          bytes.NewReader(gzipContent(t, jsonData)),
			contentType:    httputil.RequestHeaderJSON,
			acceptEncoding: httputil.RequestHeaderEncodingGzip,
			expectedResult: testMetrics,
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
				if tt.contentType == httputil.RequestHeaderYAML {
					opts = append(opts, WithRequestContentTypeYAML())
				} else if tt.contentType == httputil.RequestHeaderJSON {
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

			result, err := ReadMetrics(tt.input, opts...)
			if tt.expectedError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
				return
			}

			require.NoError(t, err)
			require.Equal(t, len(tt.expectedResult), len(result))
			for i := range tt.expectedResult {
				assert.Equal(t, tt.expectedResult[i].Component, result[i].Component)
				assert.Equal(t, len(tt.expectedResult[i].Metrics), len(result[i].Metrics))
				for j := range tt.expectedResult[i].Metrics {
					assert.Equal(t, tt.expectedResult[i].Metrics[j].Name, result[i].Metrics[j].Name)
					assert.Equal(t, tt.expectedResult[i].Metrics[j].Value, result[i].Metrics[j].Value)
					assert.Equal(t, tt.expectedResult[i].Metrics[j].UnixSeconds, result[i].Metrics[j].UnixSeconds)
					assert.Equal(t, tt.expectedResult[i].Metrics[j].Labels, result[i].Metrics[j].Labels)
				}
			}
		})
	}
}

func TestTriggerComponentCheckByTag(t *testing.T) {
	tests := []struct {
		name        string
		tagName     string
		serverResp  int
		expectError bool
	}{
		{
			name:        "successful trigger",
			tagName:     "test-tag",
			serverResp:  http.StatusOK,
			expectError: false,
		},
		{
			name:        "server error",
			tagName:     "test-tag",
			serverResp:  http.StatusInternalServerError,
			expectError: true,
		},
		{
			name:        "empty tag name",
			tagName:     "",
			serverResp:  http.StatusBadRequest,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a test server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "GET", r.Method)
				assert.Equal(t, "/v1/components/trigger-tag", r.URL.Path)
				if !tt.expectError {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(tt.serverResp)
					response := struct {
						Components []string `json:"components"`
						Exit       int      `json:"exit"`
						Success    bool     `json:"success"`
					}{
						Components: []string{"component1", "component2"},
						Exit:       0,
						Success:    true,
					}
					_ = json.NewEncoder(w).Encode(response)
				} else {
					w.WriteHeader(tt.serverResp)
				}
			}))
			defer server.Close()

			// Create context with timeout
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			// Call the function
			err := TriggerComponentCheckByTag(ctx, server.URL, tt.tagName)

			// Check the result
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
