package components

import (
	"errors"
	"testing"

	"github.com/leptonai/gpud/pkg/errdefs"
)

func TestGetComponentErrors(t *testing.T) {
	if _, err := getComponent(nil, "nvidia"); !errors.Is(err, errdefs.ErrUnavailable) {
		t.Errorf("expected ErrUnavailable, got %v", err)
	}
	if _, err := getComponent(map[string]Component{}, "nvidia"); !errors.Is(err, errdefs.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}
