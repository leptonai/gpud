package components

import (
	"context"
	"fmt"
	"sync"

	"github.com/leptonai/gpud/pkg/eventstore"
	pkghost "github.com/leptonai/gpud/pkg/host"
	nvidianvml "github.com/leptonai/gpud/pkg/nvidia-query/nvml"
)

// GPUdInstance is the instance of the GPUd dependencies.
type GPUdInstance struct {
	RootCtx context.Context

	NVMLInstance     nvidianvml.InstanceV2
	EventStore       eventstore.Store
	RebootEventStore pkghost.RebootEventStore
}

// Registry is the interface for the registry of components.
type Registry interface {
	// MustRegister registers a component with the given name and initialization function.
	// It panics if the component is already registered.
	// It panics if the initialization function returns an error.
	MustRegister(name string, initFunc func(GPUdInstance) (Component, error))
}

var _ Registry = &registry{}

type registry struct {
	mu           sync.RWMutex
	gpudInstance GPUdInstance
	components   map[string]Component
}

// NewRegistry creates a new registry.
func NewRegistry(gpudInstance GPUdInstance) *registry {
	return &registry{
		gpudInstance: gpudInstance,
		components:   make(map[string]Component),
	}
}

// MustRegister registers a component with the given name and initialization function.
// It panics if the component is already registered.
// It panics if the initialization function returns an error.
func (r *registry) MustRegister(name string, initFunc func(GPUdInstance) (Component, error)) {
	if err := r.registerInit(name, initFunc); err != nil {
		panic(err)
	}
}

// hasRegistered checks if a component with the given name is already registered.
func (r *registry) hasRegistered(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	_, ok := r.components[name]
	return ok
}

// registerInit registers an initialization function for a component with the given name.
func (r *registry) registerInit(name string, initFunc func(GPUdInstance) (Component, error)) error {
	if r.hasRegistered(name) {
		return fmt.Errorf("component %s already registered", name)
	}

	c, err := initFunc(r.gpudInstance)
	if err != nil {
		return err
	}

	r.mu.Lock()
	r.components[name] = c
	r.mu.Unlock()

	return nil
}
