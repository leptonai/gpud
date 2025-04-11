// Package components defines the common interfaces for the components.
package components

import (
	"fmt"
	"maps"
	"sync"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/pkg/errdefs"
)

var (
	defaultSetMu sync.RWMutex
	defaultSet   = make(map[string]apiv1.Component)
)

// GetAllComponents returns all the components in the default set.
func GetAllComponents() map[string]apiv1.Component {
	defaultSetMu.RLock()
	defer defaultSetMu.RUnlock()
	return getAllComponents(defaultSet)
}

// getAllComponents returns the copy of references to the components in the default set.
func getAllComponents(existing map[string]apiv1.Component) map[string]apiv1.Component {
	copied := make(map[string]apiv1.Component)
	maps.Copy(copied, existing)
	return copied
}

// GetComponent gets a component from the default set.
// It returns an error if the component is not found.
func GetComponent(name string) (apiv1.Component, error) {
	defaultSetMu.RLock()
	defer defaultSetMu.RUnlock()

	return getComponent(defaultSet, name)
}

// getComponent gets a component from the default set.
// It returns an error if the component is not found.
func getComponent(set map[string]apiv1.Component, name string) (apiv1.Component, error) {
	v, ok := set[name]
	if !ok {
		return nil, fmt.Errorf("component %s not found: %w", name, errdefs.ErrNotFound)
	}
	return v, nil
}

// RegisterComponent registers a component in the default set.
// It returns an error if the component is already registered.
func RegisterComponent(name string, comp apiv1.Component) error {
	defaultSetMu.Lock()
	defer defaultSetMu.Unlock()

	return registerComponent(defaultSet, comp)
}

// registerComponent registers a component in the default set.
// It returns an error if the component is already registered.
func registerComponent(set map[string]apiv1.Component, comp apiv1.Component) error {
	if set == nil {
		return fmt.Errorf("component set not initialized: %w", errdefs.ErrUnavailable)
	}

	name := comp.Name()
	if _, ok := set[name]; ok {
		return fmt.Errorf("component %s already registered: %w", name, errdefs.ErrAlreadyExists)
	}
	set[name] = comp

	return nil
}
