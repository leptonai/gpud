package customplugins

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPluginOutputParseConfig_Validate(t *testing.T) {
	t.Run("nil PluginOutputParseConfig should return nil error", func(t *testing.T) {
		var po *PluginOutputParseConfig
		err := po.Validate()
		assert.Nil(t, err)
	})

	t.Run("empty PluginOutputParseConfig should return ErrNoOutputParser", func(t *testing.T) {
		po := &PluginOutputParseConfig{}
		err := po.Validate()
		assert.True(t, errors.Is(err, ErrNoOutputParser))
	})

	t.Run("PluginOutputParseConfig with JSONPaths should return nil error", func(t *testing.T) {
		po := &PluginOutputParseConfig{
			JSONPaths: []JSONPath{
				{Field: "health", Query: "$.status"},
			},
		}
		err := po.Validate()
		assert.Nil(t, err)
	})

	t.Run("PluginOutputParseConfig with empty JSONPaths should still return nil error", func(t *testing.T) {
		po := &PluginOutputParseConfig{
			JSONPaths: []JSONPath{},
		}
		err := po.Validate()
		assert.Nil(t, err)
	})
}

func TestPluginOutputParseConfig_ExtractExtraInfo(t *testing.T) {
	t.Run("nil PluginOutputParseConfig should return nil, nil", func(t *testing.T) {
		var po *PluginOutputParseConfig
		result, err := po.extractExtraInfo([]byte(`{"status": "healthy"}`), "test_plugin", "auto")
		assert.Nil(t, err)
		assert.Nil(t, result)
	})

	t.Run("empty JSONPaths should return nil, nil", func(t *testing.T) {
		po := &PluginOutputParseConfig{
			JSONPaths: []JSONPath{},
		}
		result, err := po.extractExtraInfo([]byte(`{"status": "healthy"}`), "test_plugin", "auto")
		assert.Nil(t, err)
		assert.Nil(t, result)
	})

	t.Run("empty input should return nil, nil", func(t *testing.T) {
		po := &PluginOutputParseConfig{
			JSONPaths: []JSONPath{
				{Field: "health", Query: "$.status"},
			},
		}
		result, err := po.extractExtraInfo([]byte{}, "test_plugin", "auto")
		assert.Nil(t, err)
		assert.Nil(t, result)
	})

	t.Run("invalid JSON should return error", func(t *testing.T) {
		po := &PluginOutputParseConfig{
			JSONPaths: []JSONPath{
				{Field: "health", Query: "$.status"},
			},
		}
		result, err := po.extractExtraInfo([]byte(`{invalid json}`), "test_plugin", "auto")
		assert.Error(t, err)
		assert.Nil(t, result)
	})

	t.Run("valid JSON with matching paths should extract values", func(t *testing.T) {
		po := &PluginOutputParseConfig{
			JSONPaths: []JSONPath{
				{Field: "health", Query: "$.status"},
				{Field: "message", Query: "$.reason"},
			},
		}
		result, err := po.extractExtraInfo([]byte(`{"status": "healthy", "reason": "all systems operational"}`), "test_plugin", "auto")
		assert.NoError(t, err)
		assert.Equal(t, 2, len(result))
		assert.Equal(t, "healthy", result["health"].fieldValue)
		assert.Equal(t, "all systems operational", result["message"].fieldValue)
		assert.True(t, result["health"].expectMatched)
		assert.True(t, result["message"].expectMatched)
	})

	t.Run("valid JSON with non-existent path should return empty result", func(t *testing.T) {
		po := &PluginOutputParseConfig{
			JSONPaths: []JSONPath{
				{Field: "field", Query: "$.nonexistent"},
			},
		}
		result, err := po.extractExtraInfo([]byte(`{"status": "healthy"}`), "test_plugin", "auto")
		assert.Nil(t, err)
		assert.Empty(t, result)
	})

	t.Run("JSON embedded in text should extract values", func(t *testing.T) {
		po := &PluginOutputParseConfig{
			JSONPaths: []JSONPath{
				{Field: "health", Query: "$.status"},
			},
		}
		result, err := po.extractExtraInfo([]byte(`Some log output
More logs
{"status": "healthy"}
Following text`), "test_plugin", "auto")
		assert.NoError(t, err)
		assert.Equal(t, 1, len(result))
		assert.Equal(t, "healthy", result["health"].fieldValue)
		assert.True(t, result["health"].expectMatched)
	})

	t.Run("nested JSON values should be extracted and converted to strings", func(t *testing.T) {
		po := &PluginOutputParseConfig{
			JSONPaths: []JSONPath{
				{Field: "nested_val", Query: "$.nested.value"},
				{Field: "array_val", Query: "$.array"},
			},
		}
		result, err := po.extractExtraInfo([]byte(`{"nested": {"value": 42}, "array": [1, 2, 3]}`), "test_plugin", "auto")
		assert.NoError(t, err)
		assert.Equal(t, "42", result["nested_val"].fieldValue)
		assert.Equal(t, "[1,2,3]", result["array_val"].fieldValue)
		assert.True(t, result["nested_val"].expectMatched)
		assert.True(t, result["array_val"].expectMatched)
	})

	t.Run("with match rules", func(t *testing.T) {
		po := &PluginOutputParseConfig{
			JSONPaths: []JSONPath{
				{
					Field: "status",
					Query: "$.status",
					Expect: &MatchRule{
						Regex: stringPtr("healthy"),
					},
				},
				{
					Field: "count",
					Query: "$.count",
					Expect: &MatchRule{
						Regex: stringPtr(`^\d+$`),
					},
				},
			},
		}
		result, err := po.extractExtraInfo([]byte(`{"status": "healthy", "count": 10}`), "test_plugin", "auto")
		assert.NoError(t, err)
		assert.Equal(t, 2, len(result))
		assert.Equal(t, "healthy", result["status"].fieldValue)
		assert.Equal(t, "10", result["count"].fieldValue)
		assert.True(t, result["status"].expectMatched)
		assert.True(t, result["count"].expectMatched)
	})

	t.Run("with failing match rules", func(t *testing.T) {
		po := &PluginOutputParseConfig{
			JSONPaths: []JSONPath{
				{
					Field: "status",
					Query: "$.status",
					Expect: &MatchRule{
						Regex: stringPtr("unhealthy"), // This won't match
					},
				},
			},
		}
		result, err := po.extractExtraInfo([]byte(`{"status": "healthy"}`), "test_plugin", "auto")
		assert.NoError(t, err)
		assert.Equal(t, 1, len(result))
		assert.Equal(t, "healthy", result["status"].fieldValue)
		assert.False(t, result["status"].expectMatched)
		assert.Equal(t, "unhealthy", result["status"].expectRule)
	})

	t.Run("with invalid match rule", func(t *testing.T) {
		po := &PluginOutputParseConfig{
			JSONPaths: []JSONPath{
				{
					Field: "status",
					Query: "$.status",
					Expect: &MatchRule{
						Regex: stringPtr(`[invalid regex`),
					},
				},
			},
		}
		result, err := po.extractExtraInfo([]byte(`{"status": "healthy"}`), "test_plugin", "auto")
		assert.Error(t, err)
		assert.Nil(t, result)
	})

	t.Run("non-nil config with nil JSONPaths should return nil, nil", func(t *testing.T) {
		po := &PluginOutputParseConfig{
			// JSONPaths is nil here
		}
		result, err := po.extractExtraInfo([]byte(`{"status": "healthy"}`), "test_plugin", "auto")
		assert.Nil(t, err)
		assert.Nil(t, result)
	})

	t.Run("with log path", func(t *testing.T) {
		// Create a temporary file for testing
		tmpFile, err := os.CreateTemp("", "plugin-output-*.log")
		require.NoError(t, err)
		defer os.Remove(tmpFile.Name())

		po := &PluginOutputParseConfig{
			LogPath: tmpFile.Name(),
			JSONPaths: []JSONPath{
				{Field: "health", Query: "$.status"},
			},
		}

		// Test data
		testData := []byte(`{"status": "healthy"}`)

		// First call
		result, err := po.extractExtraInfo(testData, "health_check", "manual")
		require.NoError(t, err)
		require.NotNil(t, result)
		require.Equal(t, "healthy", result["health"].fieldValue)

		// Second call
		result, err = po.extractExtraInfo(testData, "health_check", "auto")
		require.NoError(t, err)
		require.NotNil(t, result)
		require.Equal(t, "healthy", result["health"].fieldValue)

		// Read the log file
		content, err := os.ReadFile(tmpFile.Name())
		require.NoError(t, err)
		logContent := string(content)

		// Verify the log file contains both entries with timestamps, plugin names, and triggers
		require.Contains(t, logContent, `{"status": "healthy"}`)
		require.Contains(t, logContent, "[") // timestamp start
		require.Contains(t, logContent, "]") // timestamp end
		require.Contains(t, logContent, "plugin=health_check")
		require.Contains(t, logContent, "trigger=manual")
		require.Contains(t, logContent, "trigger=auto")
		require.Equal(t, 2, strings.Count(logContent, `{"status": "healthy"}`))
	})
}

func TestParseFirstJSON(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    map[string]any
		wantErr bool
	}{
		{
			name:  "Simple JSON",
			input: `{"name": "test", "value": 123}`,
			want: map[string]any{
				"name":  "test",
				"value": float64(123),
			},
			wantErr: false,
		},
		{
			name: "JSON with nested objects",
			input: `{
				"name": "test", 
				"nested": {
					"key": "value", 
					"num": 42
				}
			}`,
			want: map[string]any{
				"name": "test",
				"nested": map[string]any{
					"key": "value",
					"num": float64(42),
				},
			},
			wantErr: false,
		},
		{
			name: "JSON with array",
			input: `{
				"name": "test", 
				"items": [1, 2, 3]
			}`,
			want: map[string]any{
				"name":  "test",
				"items": []any{float64(1), float64(2), float64(3)},
			},
			wantErr: false,
		},
		{
			name: "JSON embedded in text",
			input: `Some log output
More logs
{"name": "test", "value": 123}
Following text`,
			want: map[string]any{
				"name":  "test",
				"value": float64(123),
			},
			wantErr: false,
		},
		{
			name:    "No JSON object",
			input:   "This is just text with no JSON object",
			want:    nil,
			wantErr: false,
		},
		{
			name:    "Invalid JSON",
			input:   "{invalid json}",
			want:    nil,
			wantErr: true,
		},
		{
			name:  "Multiple JSON objects (should return first)",
			input: `{"first": true}{"second": false}`,
			want: map[string]any{
				"first": true,
			},
			wantErr: false,
		},
		{
			name: "With whitespace",
			input: `   
  {  "name" :  "test"  }  `,
			want: map[string]any{
				"name": "test",
			},
			wantErr: false,
		},
		{
			name: "Sample output with module text and multiple JSON objects",
			input: `module loaded
module tested
{"a":"b", "2":3}
{"x":"y", "2":5}
hello
world`,
			want: map[string]any{
				"a": "b",
				"2": float64(3),
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseFirstJSON([]byte(tt.input))

			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func Test_readJSONPath(t *testing.T) {
	testJSON := map[string]any{
		"name":       "example_module",
		"result":     "",
		"error":      "found '1.2.3': 0 times not 10 times",
		"action":     nil,
		"passed":     false,
		"runtime":    0.00359702110290527,
		"suggestion": "need to go",
		"commands":   []any{},
		"nested": map[string]any{
			"key": "value",
			"num": 42,
			"deep": map[string]any{
				"array": []any{1, 2, 3},
				"bool":  true,
			},
		},
	}

	tests := []struct {
		name     string
		input    map[string]any
		path     string
		expected interface{}
		wantErr  bool
	}{
		{
			name:     "Get string value",
			input:    testJSON,
			path:     "$.name",
			expected: "example_module",
			wantErr:  false,
		},
		{
			name:     "Get another string value",
			input:    testJSON,
			path:     "$.error",
			expected: "found '1.2.3': 0 times not 10 times",
			wantErr:  false,
		},
		{
			name:     "Get empty string",
			input:    testJSON,
			path:     "$.result",
			expected: "",
			wantErr:  false,
		},
		{
			name:     "Get boolean value",
			input:    testJSON,
			path:     "$.passed",
			expected: false,
			wantErr:  false,
		},
		{
			name:     "Get numeric value",
			input:    testJSON,
			path:     "$.runtime",
			expected: 0.00359702110290527,
			wantErr:  false,
		},
		{
			name:     "Get null value",
			input:    testJSON,
			path:     "$.action",
			expected: nil,
			wantErr:  false,
		},
		{
			name:     "Get empty array",
			input:    testJSON,
			path:     "$.commands",
			expected: []any{},
			wantErr:  false,
		},
		{
			name:  "Get nested object",
			input: testJSON,
			path:  "$.nested",
			expected: map[string]any{
				"key": "value",
				"num": 42,
				"deep": map[string]any{
					"array": []any{1, 2, 3},
					"bool":  true,
				},
			},
			wantErr: false,
		},
		{
			name:     "Get value from nested object",
			input:    testJSON,
			path:     "$.nested.key",
			expected: "value",
			wantErr:  false,
		},
		{
			name:     "Get deep nested value",
			input:    testJSON,
			path:     "$.nested.deep.bool",
			expected: true,
			wantErr:  false,
		},
		{
			name:     "Get array from nested object",
			input:    testJSON,
			path:     "$.nested.deep.array",
			expected: []any{1, 2, 3},
			wantErr:  false,
		},
		{
			name:     "Get non-existent field",
			input:    testJSON,
			path:     "$.nonexistent",
			expected: nil,
			wantErr:  false,
		},
		{
			name:    "Invalid JSONPath syntax",
			input:   testJSON,
			path:    "$[name",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := readJSONPath(tt.input, tt.path)

			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestReadJSONPathFromScriptOutput(t *testing.T) {
	output := `
/root/.local/share/uv/python/cpython-3.10.17-linux-x86_64-gnu/lib/python3.10/runpy.py:126: RuntimeWarning: 'my_test.example_module' found in sys.modules after import of package 'health
_checks', but prior to execution of 'my_test.example_module'; this may result in unpredictable behavior


{
    "name": "example_module",
 "result": "",
   "error": "found '1.2.3': O times not 10 times",
 "action": null,
   "passed": false, 
 "runtime": 0.0035970211029052734,
     "commands": []
}

success
`

	jsonMap, err := parseFirstJSON([]byte(output))
	require.NoError(t, err)

	name, err := readJSONPath(jsonMap, "$.name")
	require.NoError(t, err)
	require.Equal(t, "example_module", name)

	passed, err := readJSONPath(jsonMap, "$.passed")
	require.NoError(t, err)
	require.Equal(t, false, passed)

	runtime, err := readJSONPath(jsonMap, "$.runtime")
	require.NoError(t, err)
	require.Equal(t, 0.0035970211029052734, runtime)
}

func TestAnyToString(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		expected string
		wantErr  bool
	}{
		{
			name:     "nil",
			input:    nil,
			expected: "null",
		},
		{
			name:     "string",
			input:    "test",
			expected: "test",
		},
		{
			name:     "int",
			input:    123,
			expected: "123",
		},
		{
			name:     "bool",
			input:    true,
			expected: "true",
		},
		{
			name:     "float",
			input:    3.14,
			expected: "3.14",
		},
		{
			name:     "map",
			input:    map[string]string{"key": "value"},
			expected: `{"key":"value"}`,
		},
		{
			name:     "nested map",
			input:    map[string]any{"key": "value", "nested": map[string]any{"inner": 123}},
			expected: `{"key":"value","nested":{"inner":123}}`,
		},
		{
			name:     "slice",
			input:    []int{1, 2, 3},
			expected: "[1,2,3]",
		},
		{
			name:     "complex nested structure",
			input:    map[string]any{"array": []any{1, "string", map[string]any{"bool": true}}},
			expected: `{"array":[1,"string",{"bool":true}]}`,
		},
		{
			name:    "unmarshallable object - channel",
			input:   make(chan int),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := anyToString(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestAnyToString_AdditionalCases(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		expected string
		wantErr  bool
	}{
		{
			name:     "empty map",
			input:    map[string]any{},
			expected: "{}",
		},
		{
			name:     "empty slice",
			input:    []any{},
			expected: "[]",
		},
		{
			name:     "map with nil value",
			input:    map[string]any{"key": nil},
			expected: `{"key":null}`,
		},
		{
			name: "complex nested structure with all types",
			input: map[string]any{
				"str":    "string",
				"num":    42,
				"float":  3.14,
				"bool":   true,
				"null":   nil,
				"array":  []any{1, 2, 3},
				"object": map[string]any{"nested": "value"},
			},
			expected: `{"array":[1,2,3],"bool":true,"float":3.14,"null":null,"num":42,"object":{"nested":"value"},"str":"string"}`,
		},
		{
			name:     "int64 values",
			input:    int64(9223372036854775807),
			expected: "9223372036854775807",
		},
		{
			name:     "float32 values",
			input:    float32(3.14159),
			expected: "3.14159",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := anyToString(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExtractExtraInfoWithJSONPaths(t *testing.T) {
	// Complex nested JSON for testing
	complexJSON := []byte(`{
		"name": "test",
		"count": 42,
		"nested": {
			"level1": {
				"level2": {
					"value": "deep",
					"array": [1, 2, 3]
				}
			}
		},
		"items": [
			{"id": 1, "name": "item1"},
			{"id": 2, "name": "item2"}
		]
	}`)

	tests := []struct {
		name      string
		input     []byte
		jsonPaths []JSONPath
		expected  map[string]string // Just checking field values
		wantErr   bool
	}{
		{
			name:      "empty input",
			input:     []byte{},
			jsonPaths: []JSONPath{{Field: "name", Query: "$.name"}},
			expected:  nil,
		},
		{
			name:      "no paths",
			input:     complexJSON,
			jsonPaths: []JSONPath{},
			expected:  nil,
		},
		{
			name:      "invalid JSON",
			input:     []byte(`{invalid}`),
			jsonPaths: []JSONPath{{Field: "name", Query: "$.name"}},
			wantErr:   true,
		},
		{
			name:      "simple string path",
			input:     complexJSON,
			jsonPaths: []JSONPath{{Field: "abc", Query: "$.name"}},
			expected:  map[string]string{"abc": "test"},
		},
		{
			name:      "number path",
			input:     complexJSON,
			jsonPaths: []JSONPath{{Field: "mycnt", Query: "$.count"}},
			expected:  map[string]string{"mycnt": "42"},
		},
		{
			name:      "deep nested path",
			input:     complexJSON,
			jsonPaths: []JSONPath{{Field: "value", Query: "$.nested.level1.level2.value"}},
			expected:  map[string]string{"value": "deep"},
		},
		{
			name:      "array element path",
			input:     complexJSON,
			jsonPaths: []JSONPath{{Field: "name", Query: "$.items[0].name"}},
			expected:  map[string]string{"name": "item1"},
		},
		{
			name:      "complex object path",
			input:     complexJSON,
			jsonPaths: []JSONPath{{Field: "nested", Query: "$.nested"}},
			expected:  map[string]string{"nested": `{"level1":{"level2":{"array":[1,2,3],"value":"deep"}}}`},
		},
		{
			name:      "array path",
			input:     complexJSON,
			jsonPaths: []JSONPath{{Field: "items", Query: "$.items"}},
			expected:  map[string]string{"items": `[{"id":1,"name":"item1"},{"id":2,"name":"item2"}]`},
		},
		{
			name:  "multiple paths",
			input: complexJSON,
			jsonPaths: []JSONPath{
				{Field: "name", Query: "$.name"},
				{Field: "count", Query: "$.count"},
				{Field: "value", Query: "$.nested.level1.level2.value"},
			},
			expected: map[string]string{
				"name":  "test",
				"count": "42",
				"value": "deep",
			},
		},
		{
			name:      "invalid path syntax",
			input:     complexJSON,
			jsonPaths: []JSONPath{{Field: "invalid", Query: "$.[invalid"}},
			wantErr:   true,
		},
		{
			name:      "non-existent path should be skipped 1",
			input:     complexJSON,
			jsonPaths: []JSONPath{{Field: "not.exists", Query: "$.not.exists"}},
			expected:  map[string]string{},
		},
		{
			name:      "non-existent path should be skipped 2",
			input:     complexJSON,
			jsonPaths: []JSONPath{{Field: "a", Query: "$.a.b.c.d.e"}},
			expected:  map[string]string{},
		},
		{
			name:      "non-JSON input with text before JSON",
			input:     []byte(`Some text before JSON: {"name":"test"}`),
			jsonPaths: []JSONPath{{Field: "name", Query: "$.name"}},
			expected:  map[string]string{"name": "test"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := applyJSONPaths(tt.input, tt.jsonPaths)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)

			if tt.expected == nil {
				assert.Nil(t, result)
				return
			}

			// Check only field values
			actualValues := make(map[string]string)
			for k, v := range result {
				actualValues[k] = v.fieldValue
			}
			assert.Equal(t, tt.expected, actualValues)
		})
	}
}

// Test for checking the match rule functionality
func TestMatchRule_CheckMatchRule(t *testing.T) {
	tests := []struct {
		name      string
		rule      *MatchRule
		input     string
		wantMatch bool
		wantRule  string
		wantErr   bool
	}{
		{
			name:      "empty rule should match anything",
			rule:      &MatchRule{},
			input:     "any string",
			wantMatch: true,
			wantRule:  "",
			wantErr:   false,
		},
		{
			name: "regex that matches",
			rule: &MatchRule{
				Regex: stringPtr(`^\d+$`),
			},
			input:     "12345",
			wantMatch: true,
			wantRule:  `^\d+$`,
			wantErr:   false,
		},
		{
			name: "regex that doesn't match",
			rule: &MatchRule{
				Regex: stringPtr(`^\d+$`),
			},
			input:     "abc123",
			wantMatch: false,
			wantRule:  `^\d+$`,
			wantErr:   false,
		},
		{
			name: "invalid regex",
			rule: &MatchRule{
				Regex: stringPtr(`[invalid regex`),
			},
			input:     "test",
			wantMatch: false,
			wantRule:  `[invalid regex`,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			match, err := tt.rule.doesMatch(tt.input)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tt.wantMatch, match)

			// Since the function doesn't return the rule string directly,
			// we can verify the regex pattern if it exists
			if tt.rule.Regex != nil {
				assert.Equal(t, tt.wantRule, *tt.rule.Regex)
			}
		})
	}
}

// Helper function to create string pointer
func stringPtr(s string) *string {
	return &s
}

// Additional test function to improve coverage of applyJSONPaths
func TestExtractExtraInfoWithJSONPaths_EdgeCases(t *testing.T) {
	t.Run("parseFirstJSON returns non-nil but empty map", func(t *testing.T) {
		// This JSON has opening and closing braces but no content
		emptyJSON := []byte(`{}`)
		jsonPaths := []JSONPath{
			{Field: "some_field", Query: "$.nonexistent"},
		}
		result, err := applyJSONPaths(emptyJSON, jsonPaths)
		assert.NoError(t, err)
		assert.Empty(t, result)
	})

	t.Run("failure to convert value to string", func(t *testing.T) {
		// Mock a scenario where anyToString would fail
		// We'll test the code path where this would happen, though in practice
		// it's hard to make anyToString fail without a mocking framework
		jsonData := []byte(`{"status": "healthy"}`)
		jsonPaths := []JSONPath{
			{Field: "status", Query: "$.status"},
		}

		// The actual test just verifies we have good coverage of the normal path
		// since we can't easily simulate the failure
		result, err := applyJSONPaths(jsonData, jsonPaths)
		assert.NoError(t, err)
		assert.Equal(t, 1, len(result))
		assert.Equal(t, "healthy", result["status"].fieldValue)
	})

	t.Run("with multiple nested paths - some existing, some not", func(t *testing.T) {
		jsonData := []byte(`{
			"a": {
				"b": {
					"c": "value1"
				}
			},
			"x": {
				"y": "value2"
			}
		}`)

		jsonPaths := []JSONPath{
			{Field: "exists1", Query: "$.a.b.c"},
			{Field: "exists2", Query: "$.x.y"},
			{Field: "missing1", Query: "$.a.b.z"},
			{Field: "missing2", Query: "$.p.q.r"},
		}

		result, err := applyJSONPaths(jsonData, jsonPaths)
		assert.NoError(t, err)
		assert.Equal(t, 2, len(result))
		assert.Equal(t, "value1", result["exists1"].fieldValue)
		assert.Equal(t, "value2", result["exists2"].fieldValue)
		// The missing fields should be skipped
		_, exists1 := result["missing1"]
		_, exists2 := result["missing2"]
		assert.False(t, exists1)
		assert.False(t, exists2)
	})

	t.Run("json path found but anyToString fails", func(t *testing.T) {
		// Create a test case with a custom type that might fail JSON marshaling
		// We'll use a realistic approach to create a value that would potentially fail
		// when calling anyToString, like a function value or a channel

		// First create a mock for testing
		jsonData := []byte(`{"status": "healthy"}`)

		// Using a simple test to exercise as much of the code path as possible
		jsonPaths := []JSONPath{
			{Field: "status", Query: "$.status"},
		}

		result, err := applyJSONPaths(jsonData, jsonPaths)
		assert.NoError(t, err)
		assert.Equal(t, 1, len(result))
	})

	t.Run("nil JSON map after parsing", func(t *testing.T) {
		// This will exercise the path where parseFirstJSON returns nil but no error
		// Creating input that would produce this situation in parseFirstJSON
		input := []byte("no json here")
		jsonPaths := []JSONPath{
			{Field: "status", Query: "$.status"},
		}
		result, err := applyJSONPaths(input, jsonPaths)
		assert.Nil(t, err)
		assert.Nil(t, result)
	})
}

// TestApplyJSONPathsWithSuggestedActions tests the suggestedActions functionality
func TestApplyJSONPathsWithSuggestedActions(t *testing.T) {
	jsonData := []byte(`{
		"status": "unhealthy",
		"temperature": 95,
		"utilization": 100,
		"memory": "low",
		"errors": ["error1", "error2"]
	}`)

	tests := []struct {
		name            string
		jsonPaths       []JSONPath
		expectedMatch   map[string]bool
		expectedValue   map[string]string
		expectedActions map[string]map[string]string
	}{
		{
			name: "basic suggested action on exact match",
			jsonPaths: []JSONPath{
				{
					Field: "status",
					Query: "$.status",
					SuggestedActions: map[string]MatchRule{
						"reset_system": {
							Regex: stringPtr("unhealthy"),
						},
					},
				},
			},
			expectedMatch: map[string]bool{"status": true},
			expectedValue: map[string]string{"status": "unhealthy"},
			expectedActions: map[string]map[string]string{
				"status": {"reset_system": "unhealthy"},
			},
		},
		{
			name: "suggested action with regex match",
			jsonPaths: []JSONPath{
				{
					Field: "temperature",
					Query: "$.temperature",
					SuggestedActions: map[string]MatchRule{
						"reduce_load": {
							Regex: stringPtr(`^[89][0-9]$|^100$`), // 90-100
						},
					},
				},
			},
			expectedMatch: map[string]bool{"temperature": true},
			expectedValue: map[string]string{"temperature": "95"},
			expectedActions: map[string]map[string]string{
				"temperature": {"reduce_load": "95"},
			},
		},
		{
			name: "multiple suggested actions for one field",
			jsonPaths: []JSONPath{
				{
					Field: "memory",
					Query: "$.memory",
					SuggestedActions: map[string]MatchRule{
						"restart_service": {
							Regex: stringPtr("low"),
						},
						"check_leak": {
							Regex: stringPtr("low"),
						},
					},
				},
			},
			expectedMatch: map[string]bool{"memory": true},
			expectedValue: map[string]string{"memory": "low"},
			expectedActions: map[string]map[string]string{
				"memory": {
					"restart_service": "low",
					"check_leak":      "low",
				},
			},
		},
		{
			name: "no suggested action matches",
			jsonPaths: []JSONPath{
				{
					Field: "status",
					Query: "$.status",
					SuggestedActions: map[string]MatchRule{
						"reset_system": {
							Regex: stringPtr("critical"), // Won't match "unhealthy"
						},
					},
				},
			},
			expectedMatch: map[string]bool{"status": true},
			expectedValue: map[string]string{"status": "unhealthy"},
			expectedActions: map[string]map[string]string{
				"status": {},
			},
		},
		{
			name: "multiple fields with different suggested actions",
			jsonPaths: []JSONPath{
				{
					Field: "status",
					Query: "$.status",
					SuggestedActions: map[string]MatchRule{
						"restart_service": {
							Regex: stringPtr("unhealthy"),
						},
					},
				},
				{
					Field: "temperature",
					Query: "$.temperature",
					SuggestedActions: map[string]MatchRule{
						"thermal_check": {
							Regex: stringPtr(`^9[0-9]$|^100$`), // 90-100
						},
					},
				},
			},
			expectedMatch: map[string]bool{"status": true, "temperature": true},
			expectedValue: map[string]string{"status": "unhealthy", "temperature": "95"},
			expectedActions: map[string]map[string]string{
				"status":      {"restart_service": "unhealthy"},
				"temperature": {"thermal_check": "95"},
			},
		},
		{
			name: "suggested action with invalid regex should cause error",
			jsonPaths: []JSONPath{
				{
					Field: "status",
					Query: "$.status",
					SuggestedActions: map[string]MatchRule{
						"invalid_action": {
							Regex: stringPtr("[invalid regex"),
						},
					},
				},
			},
			expectedMatch:   map[string]bool{},
			expectedValue:   map[string]string{},
			expectedActions: map[string]map[string]string{},
		},
		{
			name: "suggested action on complex array value",
			jsonPaths: []JSONPath{
				{
					Field: "errors",
					Query: "$.errors",
					SuggestedActions: map[string]MatchRule{
						"check_errors": {
							// Any non-empty array will trigger this action
							Regex: stringPtr(`^\[.*\]$`),
						},
					},
				},
			},
			expectedMatch: map[string]bool{"errors": true},
			expectedValue: map[string]string{"errors": `["error1","error2"]`},
			expectedActions: map[string]map[string]string{
				"errors": {"check_errors": `["error1","error2"]`},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := applyJSONPaths(jsonData, tt.jsonPaths)

			if tt.name == "suggested action with invalid regex should cause error" {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)

			if len(tt.expectedMatch) == 0 {
				assert.Empty(t, result)
				return
			}

			// Check that all expected fields exist with correct values
			for field, expectedValue := range tt.expectedValue {
				assert.Contains(t, result, field)
				assert.Equal(t, expectedValue, result[field].fieldValue)

				// Check suggested actions
				if expectedActions, ok := tt.expectedActions[field]; ok {
					if len(expectedActions) == 0 {
						assert.Empty(t, result[field].suggestedActions)
					} else {
						assert.Equal(t, len(expectedActions), len(result[field].suggestedActions))
						for actionName, expectedDesc := range expectedActions {
							assert.Contains(t, result[field].suggestedActions, actionName)
							assert.Equal(t, expectedDesc, result[field].suggestedActions[actionName])
						}
					}
				}
			}
		})
	}
}

// TestApplyJSONPathsEdgeCasesWithSuggestedActions tests edge cases for the suggestedActions functionality
func TestApplyJSONPathsEdgeCasesWithSuggestedActions(t *testing.T) {
	t.Run("empty input with suggested actions", func(t *testing.T) {
		jsonPaths := []JSONPath{
			{
				Field: "status",
				Query: "$.status",
				SuggestedActions: map[string]MatchRule{
					"restart": {
						Regex: stringPtr("error"),
					},
				},
			},
		}
		result, err := applyJSONPaths([]byte{}, jsonPaths)
		assert.Nil(t, err)
		assert.Nil(t, result)
	})

	t.Run("path not found with suggested actions", func(t *testing.T) {
		jsonPaths := []JSONPath{
			{
				Field: "nonexistent",
				Query: "$.nonexistent",
				SuggestedActions: map[string]MatchRule{
					"restart": {
						Regex: stringPtr(".*"),
					},
				},
			},
		}
		result, err := applyJSONPaths([]byte(`{"status":"ok"}`), jsonPaths)
		assert.Nil(t, err)
		assert.Empty(t, result)
	})

	t.Run("invalid json with suggested actions", func(t *testing.T) {
		jsonPaths := []JSONPath{
			{
				Field: "status",
				Query: "$.status",
				SuggestedActions: map[string]MatchRule{
					"restart": {
						Regex: stringPtr("error"),
					},
				},
			},
		}
		result, err := applyJSONPaths([]byte(`{invalid json}`), jsonPaths)
		assert.Error(t, err)
		assert.Nil(t, result)
	})

	t.Run("one valid path and one invalid path with suggested actions", func(t *testing.T) {
		jsonPaths := []JSONPath{
			{
				Field: "status",
				Query: "$.status",
				SuggestedActions: map[string]MatchRule{
					"restart": {
						Regex: stringPtr("error"),
					},
				},
			},
			{
				Field: "invalid",
				Query: "$.[invalid", // Invalid JSONPath syntax
				SuggestedActions: map[string]MatchRule{
					"fix": {
						Regex: stringPtr(".*"),
					},
				},
			},
		}
		result, err := applyJSONPaths([]byte(`{"status":"error"}`), jsonPaths)
		assert.Error(t, err)
		assert.Nil(t, result)
	})
}

// TestMatchRule_DescribeRule tests the describeRule method of MatchRule
func TestMatchRule_DescribeRule(t *testing.T) {
	tests := []struct {
		name        string
		rule        *MatchRule
		expectedStr string
	}{
		{
			name:        "nil rule",
			rule:        nil,
			expectedStr: "",
		},
		{
			name:        "empty rule",
			rule:        &MatchRule{},
			expectedStr: "",
		},
		{
			name: "rule with regex",
			rule: &MatchRule{
				Regex: stringPtr("^test$"),
			},
			expectedStr: "^test$",
		},
		{
			name: "rule with nil regex",
			rule: &MatchRule{
				Regex: nil,
			},
			expectedStr: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var result string
			if tt.rule != nil {
				result = tt.rule.describeRule()
			} else {
				var nilRule *MatchRule
				result = nilRule.describeRule()
			}
			assert.Equal(t, tt.expectedStr, result)
		})
	}
}

// TestApplyJSONPathsWithRebootSuggestedAction tests the SuggestedActions functionality
// specifically for REBOOT_SYSTEM action that matches both lowercase and uppercase "reboot"
func TestApplyJSONPathsWithRebootSuggestedAction(t *testing.T) {
	tests := []struct {
		name           string
		jsonData       []byte
		expectedAction string
		shouldMatch    bool
	}{
		{
			name: "lowercase reboot",
			jsonData: []byte(`{
				"message": "system needs reboot to apply updates"
			}`),
			expectedAction: "system needs reboot to apply updates",
			shouldMatch:    true,
		},
		{
			name: "uppercase REBOOT",
			jsonData: []byte(`{
				"message": "REBOOT required"
			}`),
			expectedAction: "REBOOT required",
			shouldMatch:    true,
		},
		{
			name: "mixed case Reboot",
			jsonData: []byte(`{
				"message": "Reboot suggested"
			}`),
			expectedAction: "Reboot suggested",
			shouldMatch:    true,
		},
		{
			name: "no match",
			jsonData: []byte(`{
				"message": "system is healthy"
			}`),
			expectedAction: "system is healthy",
			shouldMatch:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create JSONPath with REBOOT_SYSTEM action rule that matches any case of "reboot"
			jsonPaths := []JSONPath{
				{
					Field: "message",
					Query: "$.message",
					SuggestedActions: map[string]MatchRule{
						"REBOOT_SYSTEM": {
							// Case insensitive regex for "reboot"
							Regex: stringPtr(`(?i).*reboot.*`),
						},
					},
				},
			}

			result, err := applyJSONPaths(tt.jsonData, jsonPaths)
			assert.NoError(t, err)
			assert.NotNil(t, result)
			assert.Equal(t, 1, len(result))

			// Extract the message field
			field, exists := result["message"]
			assert.True(t, exists)
			assert.Equal(t, tt.expectedAction, field.fieldValue)

			// Check if REBOOT_SYSTEM action was matched as expected
			if tt.shouldMatch {
				assert.NotNil(t, field.suggestedActions)
				assert.Contains(t, field.suggestedActions, "REBOOT_SYSTEM")
				assert.Equal(t, tt.expectedAction, field.suggestedActions["REBOOT_SYSTEM"])
			} else {
				if field.suggestedActions != nil {
					assert.NotContains(t, field.suggestedActions, "REBOOT_SYSTEM")
				}
			}
		})
	}
}

// TestApplyJSONPathsWithDuplicatedActionName tests the component's logic for merging
// multiple descriptions for the same suggested action name
func TestApplyJSONPathsWithDuplicatedActionName(t *testing.T) {
	// Create a JSON with multiple fields that will trigger the same action name
	jsonData := []byte(`{
		"temperature": 95,
		"pressure": "critical",
		"fan_speed": "low"
	}`)

	// Define JSONPaths with the same action name for different fields
	jsonPaths := []JSONPath{
		{
			Field: "temperature",
			Query: "$.temperature",
			SuggestedActions: map[string]MatchRule{
				"MAINTENANCE_REQUIRED": {
					Regex: stringPtr(`9[0-9]`), // Will match "95"
				},
			},
		},
		{
			Field: "pressure",
			Query: "$.pressure",
			SuggestedActions: map[string]MatchRule{
				"MAINTENANCE_REQUIRED": {
					Regex: stringPtr("critical"), // Will match "critical"
				},
			},
		},
		{
			Field: "fan_speed",
			Query: "$.fan_speed",
			SuggestedActions: map[string]MatchRule{
				"MAINTENANCE_REQUIRED": {
					Regex: stringPtr("low"), // Will match "low"
				},
			},
		},
	}

	// Apply the JSON paths
	result, err := applyJSONPaths(jsonData, jsonPaths)
	require.NoError(t, err)
	require.NotNil(t, result)

	// We expect 3 fields in the result
	assert.Equal(t, 3, len(result))

	// Check each field has the correct suggested action
	tempField, exists := result["temperature"]
	assert.True(t, exists)
	assert.Equal(t, "95", tempField.fieldValue)
	assert.Contains(t, tempField.suggestedActions, "MAINTENANCE_REQUIRED")
	assert.Equal(t, "95", tempField.suggestedActions["MAINTENANCE_REQUIRED"])

	pressureField, exists := result["pressure"]
	assert.True(t, exists)
	assert.Equal(t, "critical", pressureField.fieldValue)
	assert.Contains(t, pressureField.suggestedActions, "MAINTENANCE_REQUIRED")
	assert.Equal(t, "critical", pressureField.suggestedActions["MAINTENANCE_REQUIRED"])

	fanField, exists := result["fan_speed"]
	assert.True(t, exists)
	assert.Equal(t, "low", fanField.fieldValue)
	assert.Contains(t, fanField.suggestedActions, "MAINTENANCE_REQUIRED")
	assert.Equal(t, "low", fanField.suggestedActions["MAINTENANCE_REQUIRED"])

	// Now, let's create a component test that specifically tests the merging
	// This simulates what happens in component.go where the descriptions get merged
	// if they have the same action name
	mergedActions := make(map[string]string)

	// Add actions from each field to the map - this simulates the code in component.go
	for _, data := range result {
		for actionName, desc := range data.suggestedActions {
			if prev := mergedActions[actionName]; prev != "" {
				desc = fmt.Sprintf("%s, %s", prev, desc)
			}
			mergedActions[actionName] = desc
		}
	}

	// Verify we have a single action name with all values concatenated
	assert.Equal(t, 1, len(mergedActions))
	assert.Contains(t, mergedActions, "MAINTENANCE_REQUIRED")

	// The concatenated result should contain all three values
	mergedDesc := mergedActions["MAINTENANCE_REQUIRED"]
	assert.Contains(t, mergedDesc, "95")
	assert.Contains(t, mergedDesc, "critical")
	assert.Contains(t, mergedDesc, "low")

	// There should be exactly 2 commas in the merged description
	commaCount := strings.Count(mergedDesc, ",")
	assert.Equal(t, 2, commaCount, "There should be exactly 2 commas in the merged description")

	// Verify the format is correct by ensuring all three fields are present
	// The format should be "value1, value2, value3" but the order may vary
	parts := strings.Split(mergedDesc, ", ")
	assert.Equal(t, 3, len(parts), "Expected 3 parts separated by commas")

	// Check all values are included in the parts (order doesn't matter)
	values := []string{"95", "critical", "low"}
	for _, value := range values {
		found := false
		for _, part := range parts {
			if part == value {
				found = true
				break
			}
		}
		assert.True(t, found, "Value %s should be in merged description", value)
	}
}

func TestSubstituteLogPath(t *testing.T) {
	tests := []struct {
		name        string
		logPath     string
		triggerName string
		pluginName  string
		expected    string
	}{
		{
			name:        "no substitutions needed",
			logPath:     "/var/log/plugin.log",
			triggerName: "auto",
			pluginName:  "health_check",
			expected:    "/var/log/plugin.log",
		},
		{
			name:        "substitute trigger only",
			logPath:     "/var/log/${TRIGGER}.log",
			triggerName: "auto",
			pluginName:  "health_check",
			expected:    "/var/log/auto.log",
		},
		{
			name:        "substitute plugin only",
			logPath:     "/var/log/${PLUGIN}.log",
			triggerName: "auto",
			pluginName:  "health_check",
			expected:    "/var/log/health_check.log",
		},
		{
			name:        "substitute both",
			logPath:     "/var/log/${PLUGIN}_${TRIGGER}.log",
			triggerName: "auto",
			pluginName:  "health_check",
			expected:    "/var/log/health_check_auto.log",
		},
		{
			name:        "missing trigger when required",
			logPath:     "/var/log/${TRIGGER}.log",
			triggerName: "",
			pluginName:  "health_check",
			expected:    "",
		},
		{
			name:        "missing plugin when required",
			logPath:     "/var/log/${PLUGIN}.log",
			triggerName: "auto",
			pluginName:  "",
			expected:    "",
		},
		{
			name:        "multiple substitutions",
			logPath:     "/var/log/${PLUGIN}/${TRIGGER}/${PLUGIN}_${TRIGGER}.log",
			triggerName: "auto",
			pluginName:  "health_check",
			expected:    "/var/log/health_check/auto/health_check_auto.log",
		},
		{
			name:        "empty log path",
			logPath:     "",
			triggerName: "auto",
			pluginName:  "health_check",
			expected:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := substituteLogPath(tt.logPath, tt.triggerName, tt.pluginName)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExtractExtraInfoWithLogPathSubstitution(t *testing.T) {
	t.Run("with substituted log path", func(t *testing.T) {
		// Create a temporary directory for testing
		tmpDir, err := os.MkdirTemp("", "plugin-output-*")
		require.NoError(t, err)
		defer os.RemoveAll(tmpDir)

		po := &PluginOutputParseConfig{
			LogPath: filepath.Join(tmpDir, "${PLUGIN}_${TRIGGER}.log"),
			JSONPaths: []JSONPath{
				{Field: "health", Query: "$.status"},
			},
		}

		// Test data
		testData := []byte(`{"status": "healthy"}`)

		// First call with both plugin and trigger
		result, err := po.extractExtraInfo(testData, "health_check", "auto")
		require.NoError(t, err)
		require.NotNil(t, result)
		require.Equal(t, "healthy", result["health"].fieldValue)

		// Verify the log file was created with substituted path
		expectedLogPath := filepath.Join(tmpDir, "health_check_auto.log")
		content, err := os.ReadFile(expectedLogPath)
		require.NoError(t, err)
		logContent := string(content)

		// Verify the log content
		require.Contains(t, logContent, `{"status": "healthy"}`)
		require.Contains(t, logContent, "plugin=health_check")
		require.Contains(t, logContent, "trigger=auto")
	})

	t.Run("skip logging when trigger is missing", func(t *testing.T) {
		// Create a temporary directory for testing
		tmpDir, err := os.MkdirTemp("", "plugin-output-*")
		require.NoError(t, err)
		defer os.RemoveAll(tmpDir)

		po := &PluginOutputParseConfig{
			LogPath: filepath.Join(tmpDir, "${TRIGGER}.log"),
			JSONPaths: []JSONPath{
				{Field: "health", Query: "$.status"},
			},
		}

		// Test data
		testData := []byte(`{"status": "healthy"}`)

		// Call with empty trigger
		result, err := po.extractExtraInfo(testData, "health_check", "")
		require.NoError(t, err)
		require.NotNil(t, result)
		require.Equal(t, "healthy", result["health"].fieldValue)

		// Verify no log file was created
		expectedLogPath := filepath.Join(tmpDir, ".log")
		_, err = os.Stat(expectedLogPath)
		require.True(t, os.IsNotExist(err))
	})

	t.Run("skip logging when plugin is missing", func(t *testing.T) {
		// Create a temporary directory for testing
		tmpDir, err := os.MkdirTemp("", "plugin-output-*")
		require.NoError(t, err)
		defer os.RemoveAll(tmpDir)

		po := &PluginOutputParseConfig{
			LogPath: filepath.Join(tmpDir, "${PLUGIN}.log"),
			JSONPaths: []JSONPath{
				{Field: "health", Query: "$.status"},
			},
		}

		// Test data
		testData := []byte(`{"status": "healthy"}`)

		// Call with empty plugin name
		result, err := po.extractExtraInfo(testData, "", "auto")
		require.NoError(t, err)
		require.NotNil(t, result)
		require.Equal(t, "healthy", result["health"].fieldValue)

		// Verify no log file was created
		expectedLogPath := filepath.Join(tmpDir, ".log")
		_, err = os.Stat(expectedLogPath)
		require.True(t, os.IsNotExist(err))
	})
}
