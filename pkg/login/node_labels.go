package login

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"k8s.io/apimachinery/pkg/util/validation"
)

const (
	maxNodeLabels          = 8
	managedNodeLabelPrefix = "user.node.lepton.ai/"
)

// ParseNodeLabelsJSON parses a JSON object of Kubernetes label key/value pairs.
// Keys without the managed prefix are normalized later.
// Omit the flag to send no nodeLabels field, or pass {} to explicitly clear the labels.
func ParseNodeLabelsJSON(raw string) (map[string]string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, fmt.Errorf("must be a JSON object of string:string label pairs")
	}
	if trimmed == "null" {
		return nil, fmt.Errorf("use {} to clear labels or omit the flag to leave labels unchanged")
	}

	var labels map[string]string
	if err := json.Unmarshal([]byte(trimmed), &labels); err != nil {
		return nil, err
	}

	if labels == nil {
		return nil, fmt.Errorf("must be a JSON object of string:string label pairs")
	}

	return labels, nil
}

func normalizeNodeLabels(labels map[string]string) (map[string]string, error) {
	if labels == nil {
		return nil, nil
	}

	keys := make([]string, 0, len(labels))
	for key := range labels {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	normalized := make(map[string]string, len(labels))
	normalizedSources := make(map[string]string, len(labels))
	for _, key := range keys {
		normalizedKey := normalizeNodeLabelKey(key)
		if prevKey, ok := normalizedSources[normalizedKey]; ok {
			return nil, fmt.Errorf("node label keys %q and %q normalize to the same managed key %q", prevKey, key, normalizedKey)
		}

		normalizedSources[normalizedKey] = key
		normalized[normalizedKey] = labels[key]
	}

	if err := ValidateNodeLabels(normalized); err != nil {
		return nil, err
	}
	return normalized, nil
}

func normalizeNodeLabelKey(key string) string {
	if strings.HasPrefix(key, managedNodeLabelPrefix) {
		return key
	}
	return managedNodeLabelPrefix + key
}

// ValidateNodeLabels validates the final Kubernetes label keys and values that GPUd sends
// during login, after managed-prefix normalization has been applied. This means the
// Kubernetes qualified-name length checks run against the fully prefixed key.
func ValidateNodeLabels(labels map[string]string) error {
	if labels == nil {
		return nil
	}

	if len(labels) > maxNodeLabels {
		return fmt.Errorf("at most %d node labels are allowed", maxNodeLabels)
	}

	keys := make([]string, 0, len(labels))
	for key := range labels {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		if err := validateNodeLabelName(key); err != nil {
			return fmt.Errorf("invalid node label key %q: %w", key, err)
		}
		if err := validateNodeLabelValue(labels[key]); err != nil {
			return fmt.Errorf("invalid node label value for %q: %w", key, err)
		}
	}

	return nil
}

func canonicalNodeLabels(labels map[string]string) (string, error) {
	normalized, err := normalizeNodeLabels(labels)
	if err != nil {
		return "", err
	}
	if normalized == nil {
		return "", nil
	}

	b, err := json.Marshal(normalized)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func validateNodeLabelName(key string) error {
	if errs := validation.IsQualifiedName(key); len(errs) > 0 {
		return fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	return nil
}

func validateNodeLabelValue(value string) error {
	if errs := validation.IsValidLabelValue(value); len(errs) > 0 {
		return fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	return nil
}
