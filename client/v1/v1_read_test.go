package v1

import (
	"bytes"
	"compress/gzip"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/pkg/httputil"
)

func TestReadHealthStates_Comprehensive(t *testing.T) {
	testStates := v1.GPUdComponentHealthStates{
		{
			Component: "component1",
			States: []v1.HealthState{
				{
					Name:      "test",
					ExtraInfo: map[string]string{"state": "running"},
				},
			},
		},
	}
	jsonData := mustMarshalJSON(t, testStates)
	yamlData := mustMarshalYAML(t, testStates)

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
		expectedResult v1.GPUdComponentHealthStates
	}{
		{
			name:           "read JSON with default content type",
			input:          bytes.NewReader(jsonData),
			expectedResult: testStates,
		},
		{
			name:           "read JSON explicitly",
			input:          bytes.NewReader(jsonData),
			contentType:    httputil.RequestHeaderJSON,
			expectedResult: testStates,
		},
		{
			name:           "read YAML",
			input:          bytes.NewReader(yamlData),
			contentType:    httputil.RequestHeaderYAML,
			expectedResult: testStates,
		},
		{
			name:           "read gzipped JSON",
			input:          bytes.NewReader(gzipJSON),
			contentType:    httputil.RequestHeaderJSON,
			acceptEncoding: httputil.RequestHeaderEncodingGzip,
			expectedResult: testStates,
		},
		{
			name:           "read gzipped YAML",
			input:          bytes.NewReader(gzipYAML),
			contentType:    httputil.RequestHeaderYAML,
			acceptEncoding: httputil.RequestHeaderEncodingGzip,
			expectedResult: testStates,
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
			name:          "unsupported content type",
			input:         bytes.NewReader(jsonData),
			contentType:   "application/xml",
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

			result, err := ReadHealthStates(tt.input, opts...)
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

func TestReadInfo_Comprehensive(t *testing.T) {
	now := time.Now().UTC()
	testInfo := v1.GPUdComponentInfos{
		{
			Component: "component1",
			StartTime: now,
			EndTime:   now.Add(time.Hour),
			Info: v1.Info{
				States: []v1.HealthState{
					{
						Name:      "test",
						ExtraInfo: map[string]string{"key": "value"},
					},
				},
			},
		},
	}
	jsonData := mustMarshalJSON(t, testInfo)
	yamlData := mustMarshalYAML(t, testInfo)

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
		expectedResult v1.GPUdComponentInfos
	}{
		{
			name:           "read JSON with default content type",
			input:          bytes.NewReader(jsonData),
			expectedResult: testInfo,
		},
		{
			name:           "read JSON explicitly",
			input:          bytes.NewReader(jsonData),
			contentType:    httputil.RequestHeaderJSON,
			expectedResult: testInfo,
		},
		{
			name:           "read YAML",
			input:          bytes.NewReader(yamlData),
			contentType:    httputil.RequestHeaderYAML,
			expectedResult: testInfo,
		},
		{
			name:           "read gzipped JSON",
			input:          bytes.NewReader(gzipJSON),
			contentType:    httputil.RequestHeaderJSON,
			acceptEncoding: httputil.RequestHeaderEncodingGzip,
			expectedResult: testInfo,
		},
		{
			name:           "read gzipped YAML",
			input:          bytes.NewReader(gzipYAML),
			contentType:    httputil.RequestHeaderYAML,
			acceptEncoding: httputil.RequestHeaderEncodingGzip,
			expectedResult: testInfo,
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
			name:          "unsupported content type",
			input:         bytes.NewReader(jsonData),
			contentType:   "application/xml",
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

			result, err := ReadInfo(tt.input, opts...)
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
				assert.Equal(t, tt.expectedResult[i].Info, result[i].Info)
			}
		})
	}
}
