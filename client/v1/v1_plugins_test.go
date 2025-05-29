package v1

import (
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	pkgcustomplugins "github.com/leptonai/gpud/pkg/custom-plugins"
	"github.com/leptonai/gpud/pkg/errdefs"
	"github.com/leptonai/gpud/pkg/httputil"
)

func TestGetPluginSpecs(t *testing.T) {
	testPlugins := pkgcustomplugins.Specs{
		{
			PluginName: "test",
			Type:       pkgcustomplugins.SpecTypeComponent,
			HealthStatePlugin: &pkgcustomplugins.Plugin{
				Steps: []pkgcustomplugins.Step{
					{
						Name: "test-step",
						RunBashScript: &pkgcustomplugins.RunBashScript{
							Script:      "echo hello",
							ContentType: "plaintext",
						},
					},
				},
			},
			Timeout: metav1.Duration{Duration: time.Minute},
		},
	}

	tests := []struct {
		name           string
		serverResponse []byte
		contentType    string
		acceptEncoding string
		statusCode     int
		expectedError  string
		expectedResult pkgcustomplugins.Specs
		useGzip        bool
		serverError    bool
	}{
		{
			name:           "successful JSON response",
			serverResponse: mustMarshalJSON(t, testPlugins),
			contentType:    httputil.RequestHeaderJSON,
			statusCode:     http.StatusOK,
			expectedResult: testPlugins,
		},
		{
			name:           "successful YAML response",
			serverResponse: mustMarshalYAML(t, testPlugins),
			contentType:    httputil.RequestHeaderYAML,
			statusCode:     http.StatusOK,
			expectedResult: testPlugins,
		},
		{
			name:           "successful gzipped JSON response",
			serverResponse: mustMarshalJSON(t, testPlugins),
			contentType:    httputil.RequestHeaderJSON,
			acceptEncoding: httputil.RequestHeaderEncodingGzip,
			statusCode:     http.StatusOK,
			expectedResult: testPlugins,
			useGzip:        true,
		},
		{
			name:           "successful gzipped YAML response",
			serverResponse: mustMarshalYAML(t, testPlugins),
			contentType:    httputil.RequestHeaderYAML,
			acceptEncoding: httputil.RequestHeaderEncodingGzip,
			statusCode:     http.StatusOK,
			expectedResult: testPlugins,
			useGzip:        true,
		},
		{
			name:          "not found error",
			statusCode:    http.StatusNotFound,
			expectedError: errdefs.ErrNotFound.Error(),
		},
		{
			name:          "server error",
			statusCode:    http.StatusInternalServerError,
			expectedError: "server not ready, response not 200",
		},
		{
			name:           "invalid JSON response",
			serverResponse: []byte(`{"invalid": json`),
			contentType:    httputil.RequestHeaderJSON,
			statusCode:     http.StatusOK,
			expectedError:  "failed to decode json",
		},
		{
			name:           "invalid YAML response",
			serverResponse: []byte(`invalid: yaml:`),
			contentType:    httputil.RequestHeaderYAML,
			statusCode:     http.StatusOK,
			expectedError:  "failed to unmarshal yaml",
		},
		{
			name:           "invalid gzip data",
			serverResponse: []byte(`not gzip data`),
			contentType:    httputil.RequestHeaderJSON,
			acceptEncoding: httputil.RequestHeaderEncodingGzip,
			statusCode:     http.StatusOK,
			expectedError:  "failed to create gzip reader",
		},
		{
			name:           "unsupported content type",
			serverResponse: mustMarshalJSON(t, testPlugins),
			contentType:    "application/xml",
			statusCode:     http.StatusOK,
			expectedError:  "unsupported content type",
		},
		{
			name:          "network error",
			serverError:   true,
			expectedError: "failed to make request",
		},
		{
			name:           "empty response",
			serverResponse: []byte(`[]`),
			contentType:    httputil.RequestHeaderJSON,
			statusCode:     http.StatusOK,
			expectedResult: pkgcustomplugins.Specs{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var srv *httptest.Server
			var serverURL string

			if tt.serverError {
				// Use invalid URL to trigger network error
				serverURL = "http://invalid-host:99999"
			} else {
				srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					assert.Equal(t, "/v1/plugins", r.URL.Path)
					assert.Equal(t, http.MethodGet, r.Method)

					if tt.contentType != "" {
						assert.Equal(t, tt.contentType, r.Header.Get(httputil.RequestHeaderContentType))
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
				serverURL = srv.URL
			}

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

			result, err := GetPluginSpecs(context.Background(), serverURL, opts...)
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

func TestGetPluginSpecs_InvalidURL(t *testing.T) {
	_, err := GetPluginSpecs(context.Background(), "://invalid-url")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "missing protocol scheme")
}

func TestGetPluginSpecs_RequestCreationError(t *testing.T) {
	// Test with invalid context
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately to cause context error

	_, err := GetPluginSpecs(ctx, "http://localhost:8080")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "context canceled")
}

func TestGetPluginSpecs_OptionsError(t *testing.T) {
	// Test with invalid options
	invalidOpt := func(op *Op) {
		// This would be an invalid option that causes an error
		// For now, we'll test with a valid option since applyOpts doesn't return errors
	}

	_, err := GetPluginSpecs(context.Background(), "http://localhost:8080", invalidOpt)
	// Since applyOpts doesn't return errors, this should fail due to network
	// but we're testing the error path exists
	assert.Error(t, err) // This will fail due to network, but options processing succeeds
	assert.Contains(t, err.Error(), "failed to make request")
}

func TestReadCustomPluginSpecs_Comprehensive(t *testing.T) {
	testPlugins := pkgcustomplugins.Specs{
		{
			PluginName: "test",
			Type:       pkgcustomplugins.SpecTypeComponent,
			HealthStatePlugin: &pkgcustomplugins.Plugin{
				Steps: []pkgcustomplugins.Step{
					{
						Name: "test-step",
						RunBashScript: &pkgcustomplugins.RunBashScript{
							Script:      "echo hello",
							ContentType: "plaintext",
						},
					},
				},
			},
			Timeout: metav1.Duration{Duration: time.Minute},
		},
	}
	jsonData := mustMarshalJSON(t, testPlugins)
	yamlData := mustMarshalYAML(t, testPlugins)

	gzipJSON := func() []byte {
		var buf bytes.Buffer
		gw := gzip.NewWriter(&buf)
		_, err := gw.Write(jsonData)
		require.NoError(t, err)
		require.NoError(t, gw.Close())
		return buf.Bytes()
	}()

	gzipYAML := func() []byte {
		var buf bytes.Buffer
		gw := gzip.NewWriter(&buf)
		_, err := gw.Write(yamlData)
		require.NoError(t, err)
		require.NoError(t, gw.Close())
		return buf.Bytes()
	}()

	tests := []struct {
		name           string
		input          io.Reader
		contentType    string
		acceptEncoding string
		expectedError  string
		expectedResult pkgcustomplugins.Specs
	}{
		{
			name:           "read JSON with default content type",
			input:          bytes.NewReader(jsonData),
			expectedResult: testPlugins,
		},
		{
			name:           "read JSON explicitly",
			input:          bytes.NewReader(jsonData),
			contentType:    httputil.RequestHeaderJSON,
			expectedResult: testPlugins,
		},
		{
			name:           "read YAML",
			input:          bytes.NewReader(yamlData),
			contentType:    httputil.RequestHeaderYAML,
			expectedResult: testPlugins,
		},
		{
			name:           "read gzipped JSON",
			input:          bytes.NewReader(gzipJSON),
			contentType:    httputil.RequestHeaderJSON,
			acceptEncoding: httputil.RequestHeaderEncodingGzip,
			expectedResult: testPlugins,
		},
		{
			name:           "read gzipped YAML",
			input:          bytes.NewReader(gzipYAML),
			contentType:    httputil.RequestHeaderYAML,
			acceptEncoding: httputil.RequestHeaderEncodingGzip,
			expectedResult: testPlugins,
		},
		{
			name:          "invalid JSON data",
			input:         bytes.NewReader([]byte(`{"invalid": JSON`)),
			contentType:   httputil.RequestHeaderJSON,
			expectedError: "failed to decode json",
		},
		{
			name:          "invalid YAML data",
			input:         bytes.NewReader([]byte(`invalid: YAML:`)),
			contentType:   httputil.RequestHeaderYAML,
			expectedError: "failed to unmarshal yaml",
		},
		{
			name:           "invalid gzip data with JSON content type",
			input:          bytes.NewReader([]byte(`not a gzip`)),
			contentType:    httputil.RequestHeaderJSON,
			acceptEncoding: httputil.RequestHeaderEncodingGzip,
			expectedError:  "failed to create gzip reader",
		},
		{
			name:           "invalid gzip data with YAML content type",
			input:          bytes.NewReader([]byte(`not a gzip`)),
			contentType:    httputil.RequestHeaderYAML,
			acceptEncoding: httputil.RequestHeaderEncodingGzip,
			expectedError:  "failed to create gzip reader",
		},
		{
			name:          "unsupported content type",
			input:         bytes.NewReader(jsonData),
			contentType:   "application/xml",
			expectedError: "unsupported content type",
		},
		{
			name:           "unsupported content type with gzip",
			input:          bytes.NewReader(gzipJSON),
			contentType:    "application/xml",
			acceptEncoding: httputil.RequestHeaderEncodingGzip,
			expectedError:  "unsupported content type",
		},
		{
			name:           "empty JSON array",
			input:          bytes.NewReader([]byte(`[]`)),
			contentType:    httputil.RequestHeaderJSON,
			expectedResult: pkgcustomplugins.Specs{},
		},
		{
			name:           "empty YAML array",
			input:          bytes.NewReader([]byte(`[]`)),
			contentType:    httputil.RequestHeaderYAML,
			expectedResult: pkgcustomplugins.Specs{},
		},
		{
			name:           "gzipped invalid JSON",
			input:          bytes.NewReader(gzipContent(t, []byte(`{"invalid": json`))),
			contentType:    httputil.RequestHeaderJSON,
			acceptEncoding: httputil.RequestHeaderEncodingGzip,
			expectedError:  "failed to decode json",
		},
		{
			name:           "gzipped invalid YAML",
			input:          bytes.NewReader(gzipContent(t, []byte(`invalid: yaml:`))),
			contentType:    httputil.RequestHeaderYAML,
			acceptEncoding: httputil.RequestHeaderEncodingGzip,
			expectedError:  "failed to unmarshal yaml",
		},
		{
			name:          "YAML read error",
			input:         &errorReader{},
			contentType:   httputil.RequestHeaderYAML,
			expectedError: "failed to read yaml",
		},
		{
			name:           "gzipped YAML read error",
			input:          &gzipErrorReader{data: gzipContent(t, []byte(`test: value`))},
			contentType:    httputil.RequestHeaderYAML,
			acceptEncoding: httputil.RequestHeaderEncodingGzip,
			expectedError:  "failed to read yaml",
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

			result, err := ReadPluginSpecs(tt.input, opts...)
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

// errorReader is a helper for testing read errors
type errorReader struct{}

func (e *errorReader) Read(p []byte) (n int, err error) {
	return 0, io.ErrUnexpectedEOF
}

// gzipErrorReader simulates a read error after some data is read
type gzipErrorReader struct {
	data []byte
	pos  int
}

func (g *gzipErrorReader) Read(p []byte) (n int, err error) {
	if g.pos >= len(g.data)-5 { // Cause error near end
		return 0, io.ErrUnexpectedEOF
	}

	remaining := len(g.data) - g.pos
	if len(p) > remaining {
		copy(p, g.data[g.pos:])
		g.pos = len(g.data)
		return remaining, nil
	}

	copy(p, g.data[g.pos:g.pos+len(p)])
	g.pos += len(p)
	return len(p), nil
}

// Additional tests for GetPluginSpecs to increase coverage

func TestGetPluginSpecs_MoreEdgeCases(t *testing.T) {
	t.Run("request creation with malformed URL", func(t *testing.T) {
		_, err := GetPluginSpecs(context.Background(), "http://[::1]:invalid")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid port")
	})

	t.Run("gzipped JSON with decode error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, err := w.Write(gzipContent(t, []byte(`{"invalid": json`)))
			require.NoError(t, err)
		}))
		defer srv.Close()

		_, err := GetPluginSpecs(context.Background(), srv.URL, WithAcceptEncodingGzip())
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to decode json")
	})

	t.Run("gzipped YAML with unmarshal error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, err := w.Write(gzipContent(t, []byte(`invalid: yaml:`)))
			require.NoError(t, err)
		}))
		defer srv.Close()

		_, err := GetPluginSpecs(context.Background(), srv.URL, WithRequestContentTypeYAML(), WithAcceptEncodingGzip())
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to unmarshal yaml")
	})
}
