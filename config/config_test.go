package config

import (
	"testing"
)

func TestLoadConfigYAML(t *testing.T) {
	t.Parallel()

	config, err := LoadConfigYAML("testdata/test.0.yaml")
	if err != nil {
		t.Fatalf("failed to parse config: %v", err)
	}

	if config.Components["systemd"] == nil {
		t.Fatalf("systemd component is nil")
	}

	b, err := config.YAML()
	if err != nil {
		t.Fatalf("failed to marshal config: %v", err)
	}
	t.Logf("config:\n%s", string(b))
}

func TestLoadConfigYAMLNull(t *testing.T) {
	t.Parallel()

	config, err := LoadConfigYAML("testdata/test.1.gpu.yaml")
	if err != nil {
		t.Fatalf("failed to parse config: %v", err)
	}
	for k, v := range config.Components {
		if v != nil {
			t.Errorf("key: %s, value: %v", k, v)
		}
	}
	b, err := config.YAML()
	if err != nil {
		t.Fatalf("failed to marshal config: %v", err)
	}
	t.Logf("config:\n%s", string(b))
}
