package config

import (
	"testing"

	"k8s.io/utils/ptr"
)

func TestConfig(t *testing.T) {
	cfg := &Config{}
	cfg.File = ptr.To("test")
	cfg.Commands = &[][]string{{"test"}}
	if err := cfg.Validate(); err == nil {
		t.Errorf("expected error, got nil")
	}
}
