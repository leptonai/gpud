package customplugins

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/PaesslerAG/jsonpath"
)

var (
	ErrNoOutputParser = errors.New("no output parser is set")

	// TODO: if we support multiple parsers (e.g., jq syntax),
	// return an error if multiple parsers are set
)

func (po *PluginOutputParseConfig) Validate() error {
	if po == nil {
		return nil
	}

	// it not nil, one parser must be set
	switch {
	case po.JSONPaths != nil:
		return nil

	default:
		return ErrNoOutputParser
	}
}

// substituteLogPath replaces ${TRIGGER} and ${PLUGIN} in the log path with their values.
// Returns empty string if required variables are missing.
func substituteLogPath(logPath, triggerName, pluginName string) string {
	if logPath == "" {
		return ""
	}

	// Check if ${TRIGGER} is present and triggerName is empty
	if strings.Contains(logPath, "${TRIGGER}") && triggerName == "" {
		return ""
	}

	// Check if ${PLUGIN} is present and pluginName is empty
	if strings.Contains(logPath, "${PLUGIN}") && pluginName == "" {
		return ""
	}

	// Perform substitutions
	result := logPath
	result = strings.ReplaceAll(result, "${TRIGGER}", triggerName)
	result = strings.ReplaceAll(result, "${PLUGIN}", pluginName)

	return result
}

// extractExtraInfo extracts extra information from the plugin output using JSON paths.
// If LogPath is set, it will append the output to the specified file.
func (po *PluginOutputParseConfig) extractExtraInfo(input []byte, pluginName string, triggerName string) (map[string]extractedField, error) {
	if po == nil {
		return nil, nil
	}

	// Substitute variables in the log path
	substitutedPath := substituteLogPath(po.LogPath, triggerName, pluginName)
	if substitutedPath != "" {
		// Open the log file in append mode, create if it doesn't exist
		f, err := os.OpenFile(substitutedPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return nil, fmt.Errorf("failed to open log file: %w", err)
		}
		defer func() {
			_ = f.Close()
		}()

		// Add timestamp to the log entry
		timestamp := time.Now().UTC().Format(time.RFC3339)
		logEntry := fmt.Sprintf("[%s] plugin=%s trigger=%s\noutput=%s\n", timestamp, pluginName, triggerName, string(input))

		if _, err := f.WriteString(logEntry); err != nil {
			return nil, fmt.Errorf("failed to write to log file: %w", err)
		}
	}

	if po.JSONPaths != nil {
		return applyJSONPaths(input, po.JSONPaths)
	}

	return nil, nil
}

type extractedField struct {
	fieldName     string
	fieldValue    string
	expectRule    string
	expectMatched bool

	// suggestedActions maps from the suggested action name to its description/field value.
	suggestedActions map[string]string
}

// applyJSONPaths extracts extra information from the plugin output
// using JSON paths. The input jsonPaths is a slice of JSONPath defining field names,
// and their corresponding JSON paths, and match rules (optional).
// It returns a map of field names and the corresponding values in string format.
// The second map is the map of field names and boolean values whether the value matches the match rule.
// If the key is not found, it skips to simply ignore the key, and returns no error.
func applyJSONPaths(input []byte, jsonPaths []JSONPath) (map[string]extractedField, error) {
	if len(input) == 0 || len(jsonPaths) == 0 {
		return nil, nil
	}

	m, err := parseFirstJSON(input)
	if err != nil {
		return nil, err
	}

	if m == nil {
		return nil, nil
	}

	results := make(map[string]extractedField)
	for _, jsonPath := range jsonPaths {
		p, err := readJSONPath(m, jsonPath.Query)
		if err != nil {
			return nil, fmt.Errorf("failed to read json path %q: %w", jsonPath.Query, err)
		}

		if p == nil {
			// key not found and match rule is not set, we skip it
			if jsonPath.Expect == nil {
				continue
			}

			// key not found and match rule set, thus treat it as mismatch
			results[jsonPath.Field] = extractedField{
				fieldName:     jsonPath.Field,
				expectRule:    jsonPath.Expect.describeRule(),
				expectMatched: false,
			}
			continue
		}

		matchedValue, err := anyToString(p)
		if err != nil {
			return nil, fmt.Errorf("failed to convert value for path %q to string: %w", jsonPath.Query, err)
		}

		expectMatched, err := jsonPath.Expect.doesMatch(matchedValue)
		if err != nil {
			return nil, fmt.Errorf("failed to check match rule for path %q: %w", jsonPath.Query, err)
		}

		d := extractedField{
			fieldName:     jsonPath.Field,
			fieldValue:    matchedValue,
			expectRule:    jsonPath.Expect.describeRule(),
			expectMatched: expectMatched,
		}

		if len(jsonPath.SuggestedActions) > 0 {
			for k, rule := range jsonPath.SuggestedActions {
				matched, err := rule.doesMatch(matchedValue)
				if err != nil {
					return nil, fmt.Errorf("failed to check match rule for path %q for suggested action %q: %w", jsonPath.Query, k, err)
				}
				if matched {
					if d.suggestedActions == nil {
						d.suggestedActions = make(map[string]string)
					}
					d.suggestedActions[k] = matchedValue
				}
			}
		}

		results[jsonPath.Field] = d
	}

	return results, nil
}

// anyToString converts any value to a string representation.
// If the value is a complex type (map, slice), it's marshaled to JSON.
func anyToString(v any) (string, error) {
	if v == nil {
		return "null", nil
	}

	switch val := v.(type) {
	case string:
		return val, nil
	case bool, int, int64, float64, float32:
		return fmt.Sprintf("%v", val), nil
	default:
		// For complex types (maps, slices, structs), marshal to JSON
		jsonBytes, err := json.Marshal(val)
		if err != nil {
			return "", err
		}
		return string(jsonBytes), nil
	}
}

// parseFirstJSON finds the start of the first JSON object in the input,
// and parses it into a map[string]interface{}.
// And if there are multiple JSON objects in the input, it only parses the first one.
// The second and later ones are discarded.
// If there is no JSON object, it returns nil and no error.
func parseFirstJSON(input []byte) (map[string]any, error) {
	// find the index of the first opening brace, which indicates the start of a JSON object
	startIdx := bytes.IndexByte(input, '{')
	if startIdx == -1 {
		return nil, nil
	}

	// extract data starting from the first JSON object
	jsonData := input[startIdx:]

	// create a decoder to properly handle the JSON data
	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	var rm map[string]any
	if err := decoder.Decode(&rm); err != nil {
		return nil, err
	}

	return rm, nil
}

// readJSONPath queries the target JSON path and returns the corresponding value,
// in the input map. The returned value can be any type: string, nested map, float,
// null, etc..
// And most importantly, if the key is not found, it returns nil and no error.
//
// ref. https://pkg.go.dev/github.com/PaesslerAG/jsonpath#section-readme
// ref. https://en.wikipedia.org/wiki/JSONPath
// ref. https://goessner.net/articles/JsonPath/
func readJSONPath(input map[string]any, path string) (any, error) {
	v, err := jsonpath.Get(path, input)
	if err != nil {
		if strings.Contains(err.Error(), "unknown key") {
			return nil, nil
		}
		return nil, err
	}
	return v, nil
}
