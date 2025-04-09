// Package components defines the common interfaces for the components.
package components

import (
	"fmt"
	"sync"

	"github.com/leptonai/gpud/pkg/errdefs"
)

var (
	defaultSetMu sync.RWMutex
	defaultSet   = make(map[string]Component)
)

// GetAllComponents returns all the components in the default set.
func GetAllComponents() map[string]Component {
	defaultSetMu.RLock()
	defer defaultSetMu.RUnlock()
	return defaultSet
}

// IsComponentRegistered checks if a component is registered in the default set.
func IsComponentRegistered(name string) bool {
	defaultSetMu.RLock()
	defer defaultSetMu.RUnlock()

	return isComponentRegistered(defaultSet, name)
}

// isComponentRegistered checks if a component is registered in the default set.
func isComponentRegistered(set map[string]Component, name string) bool {
	_, ok := set[name]
	return ok
}

// GetComponent gets a component from the default set.
// It returns an error if the component is not found.
func GetComponent(name string) (Component, error) {
	defaultSetMu.RLock()
	defer defaultSetMu.RUnlock()

	return getComponent(defaultSet, name)
}

// getComponent gets a component from the default set.
// It returns an error if the component is not found.
func getComponent(set map[string]Component, name string) (Component, error) {
	if set == nil {
		return nil, fmt.Errorf("component set not initialized: %w", errdefs.ErrUnavailable)
	}

	v, ok := set[name]
	if !ok {
		return nil, fmt.Errorf("component %s not found: %w", name, errdefs.ErrNotFound)
	}
	return v, nil
}

// RegisterComponent registers a component in the default set.
// It returns an error if the component is already registered.
func RegisterComponent(name string, comp Component) error {
	defaultSetMu.Lock()
	defer defaultSetMu.Unlock()

	return registerComponent(defaultSet, comp)
}

// registerComponent registers a component in the default set.
// It returns an error if the component is already registered.
func registerComponent(set map[string]Component, comp Component) error {
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

// StopDeregisterComponent unregisters and stops a component from the default set.
// It returns nil if the component is not found.
func StopDeregisterComponent(name string) error {
	defaultSetMu.Lock()
	defer defaultSetMu.Unlock()

	return stopDeregisterComponent(defaultSet, name)
}

// stopDeregisterComponent unregisters and stops a component from the default set.
// It returns nil if the component is not found.
func stopDeregisterComponent(set map[string]Component, name string) error {
	if set == nil {
		return fmt.Errorf("component set not initialized: %w", errdefs.ErrUnavailable)
	}

	comp, ok := set[name]
	if !ok {
		return nil
	}
	delete(set, name)

	return comp.Close()
}

// SetComponent sets a component in the default set.
// If it already exists, it will be stopped and replaced.
func SetComponent(name string, comp Component) error {
	defaultSetMu.Lock()
	defer defaultSetMu.Unlock()

	return setComponent(defaultSet, comp)
}

// setComponent sets a component in the default set.
// If it already exists, it will be stopped and replaced.
func setComponent(set map[string]Component, comp Component) error {
	if set == nil {
		return fmt.Errorf("component set not initialized: %w", errdefs.ErrUnavailable)
	}

	name := comp.Name()
	comp, ok := set[name]
	if ok {
		comp.Close()
	}
	set[name] = comp

	return nil
}
