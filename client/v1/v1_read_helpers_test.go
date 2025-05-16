package v1

import (
	"bytes"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1 "github.com/leptonai/gpud/api/v1"
)

func TestReadComponents_Comprehensive(t *testing.T) {
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
			name:           "read JSON with default content type",
			input:          bytes.NewReader(jsonData),
			expectedResult: testComponents,
		},
		{
			name:           "read JSON explicitly",
			input:          bytes.NewReader(jsonData),
			contentType:    RequestHeaderJSON,
			expectedResult: testComponents,
		},
		{
			name:           "read YAML",
			input:          bytes.NewReader(yamlData),
			contentType:    RequestHeaderYAML,
			expectedResult: testComponents,
		},
		{
			name:           "read gzipped JSON",
			input:          bytes.NewReader(gzipContent(t, jsonData)),
			contentType:    RequestHeaderJSON,
			acceptEncoding: RequestHeaderEncodingGzip,
			expectedResult: testComponents,
		},
		{
			name:           "read gzipped YAML",
			input:          bytes.NewReader(gzipContent(t, yamlData)),
			contentType:    RequestHeaderYAML,
			acceptEncoding: RequestHeaderEncodingGzip,
			expectedResult: testComponents,
		},
		{
			name:          "invalid JSON data",
			input:         bytes.NewReader([]byte(`{"invalid": JSON`)),
			contentType:   RequestHeaderJSON,
			expectedError: "failed to decode json",
		},
		{
			name:          "invalid YAML data",
			input:         bytes.NewReader([]byte(`invalid: YAML:`)),
			contentType:   RequestHeaderYAML,
			expectedError: "failed to unmarshal yaml",
		},
		{
			name:           "invalid gzip data with JSON content type",
			input:          bytes.NewReader([]byte(`not a gzip`)),
			contentType:    RequestHeaderJSON,
			acceptEncoding: RequestHeaderEncodingGzip,
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
				if tt.contentType == RequestHeaderYAML {
					opts = append(opts, WithRequestContentTypeYAML())
				} else if tt.contentType == RequestHeaderJSON {
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

func TestReadEvents_Comprehensive(t *testing.T) {
	now := time.Now().UTC()
	testEvents := v1.GPUdComponentEvents{
		{
			Component: "component1",
			StartTime: now,
			EndTime:   now.Add(time.Hour),
			Events: []v1.Event{
				{
					Name: "test-event",
					Time: metav1.Time{Time: now},
					Type: v1.EventTypeInfo,
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
		expectedResult v1.GPUdComponentEvents
	}{
		{
			name:           "read JSON with default content type",
			input:          bytes.NewReader(jsonData),
			expectedResult: testEvents,
		},
		{
			name:           "read JSON explicitly",
			input:          bytes.NewReader(jsonData),
			contentType:    RequestHeaderJSON,
			expectedResult: testEvents,
		},
		{
			name:           "read YAML",
			input:          bytes.NewReader(yamlData),
			contentType:    RequestHeaderYAML,
			expectedResult: testEvents,
		},
		{
			name:           "read gzipped JSON",
			input:          bytes.NewReader(gzipContent(t, jsonData)),
			contentType:    RequestHeaderJSON,
			acceptEncoding: RequestHeaderEncodingGzip,
			expectedResult: testEvents,
		},
		{
			name:           "read gzipped YAML",
			input:          bytes.NewReader(gzipContent(t, yamlData)),
			contentType:    RequestHeaderYAML,
			acceptEncoding: RequestHeaderEncodingGzip,
			expectedResult: testEvents,
		},
		{
			name:          "invalid JSON data",
			input:         bytes.NewReader([]byte(`{"invalid": JSON`)),
			contentType:   RequestHeaderJSON,
			expectedError: "failed to decode json",
		},
		{
			name:          "invalid YAML data",
			input:         bytes.NewReader([]byte(`invalid: YAML:`)),
			contentType:   RequestHeaderYAML,
			expectedError: "failed to unmarshal yaml",
		},
		{
			name:           "invalid gzip data with JSON content type",
			input:          bytes.NewReader([]byte(`not a gzip`)),
			contentType:    RequestHeaderJSON,
			acceptEncoding: RequestHeaderEncodingGzip,
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
				if tt.contentType == RequestHeaderYAML {
					opts = append(opts, WithRequestContentTypeYAML())
				} else if tt.contentType == RequestHeaderJSON {
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

func TestReadMetrics_Comprehensive(t *testing.T) {
	testMetrics := v1.GPUdComponentMetrics{
		{
			Component: "component1",
			Metrics: []v1.Metric{
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
		expectedResult v1.GPUdComponentMetrics
	}{
		{
			name:           "read JSON with default content type",
			input:          bytes.NewReader(jsonData),
			expectedResult: testMetrics,
		},
		{
			name:           "read JSON explicitly",
			input:          bytes.NewReader(jsonData),
			contentType:    RequestHeaderJSON,
			expectedResult: testMetrics,
		},
		{
			name:           "read YAML",
			input:          bytes.NewReader(yamlData),
			contentType:    RequestHeaderYAML,
			expectedResult: testMetrics,
		},
		{
			name:           "read gzipped JSON",
			input:          bytes.NewReader(gzipContent(t, jsonData)),
			contentType:    RequestHeaderJSON,
			acceptEncoding: RequestHeaderEncodingGzip,
			expectedResult: testMetrics,
		},
		{
			name:           "read gzipped YAML",
			input:          bytes.NewReader(gzipContent(t, yamlData)),
			contentType:    RequestHeaderYAML,
			acceptEncoding: RequestHeaderEncodingGzip,
			expectedResult: testMetrics,
		},
		{
			name:          "invalid JSON data",
			input:         bytes.NewReader([]byte(`{"invalid": JSON`)),
			contentType:   RequestHeaderJSON,
			expectedError: "failed to decode json",
		},
		{
			name:          "invalid YAML data",
			input:         bytes.NewReader([]byte(`invalid: YAML:`)),
			contentType:   RequestHeaderYAML,
			expectedError: "failed to unmarshal yaml",
		},
		{
			name:           "invalid gzip data with JSON content type",
			input:          bytes.NewReader([]byte(`not a gzip`)),
			contentType:    RequestHeaderJSON,
			acceptEncoding: RequestHeaderEncodingGzip,
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
				if tt.contentType == RequestHeaderYAML {
					opts = append(opts, WithRequestContentTypeYAML())
				} else if tt.contentType == RequestHeaderJSON {
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
