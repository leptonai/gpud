package login

import (
	"encoding/json"
	"fmt"
	"maps"
	"regexp"
	"sort"
	"strings"
)

const maxNodeLabels = 8

var (
	kubernetesLabelNameRegexp  = regexp.MustCompile(`^[A-Za-z0-9]([-A-Za-z0-9_.]*[A-Za-z0-9])?$`)
	kubernetesLabelValueRegexp = regexp.MustCompile(`^[A-Za-z0-9]([-A-Za-z0-9_.]*[A-Za-z0-9])?$`)
)

// ParseNodeLabelsJSON parses a JSON object of unprefixed Kubernetes label name/value pairs.
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

	cloned := maps.Clone(labels)
	if err := ValidateNodeLabels(cloned); err != nil {
		return nil, err
	}
	return cloned, nil
}

// ValidateNodeLabels validates the unprefixed label names and values that GPUd sends
// during login. The control plane is responsible for adding the managed prefix later.
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
	if key == "" {
		return fmt.Errorf("must not be empty")
	}
	if strings.Contains(key, "/") {
		return fmt.Errorf("must be unprefixed; do not include a DNS prefix or '/'")
	}
	if len(key) > 63 {
		return fmt.Errorf("must be 63 characters or less")
	}
	if !kubernetesLabelNameRegexp.MatchString(key) {
		return fmt.Errorf("must start and end with an alphanumeric character and may contain '-', '_' or '.' in between")
	}
	return nil
}

func validateNodeLabelValue(value string) error {
	if value == "" {
		return nil
	}
	if len(value) > 63 {
		return fmt.Errorf("must be 63 characters or less")
	}
	if !kubernetesLabelValueRegexp.MatchString(value) {
		return fmt.Errorf("must start and end with an alphanumeric character and may contain '-', '_' or '.' in between")
	}
	return nil
}
