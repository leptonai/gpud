package components

import (
	"context"
	"fmt"
	"sort"
	"sync"

	nvidiacommon "github.com/leptonai/gpud/pkg/config/common"
	"github.com/leptonai/gpud/pkg/eventstore"
	pkghost "github.com/leptonai/gpud/pkg/host"
	nvidianvml "github.com/leptonai/gpud/pkg/nvidia-query/nvml"
)

// GPUdInstance is the instance of the GPUd dependencies.
type GPUdInstance struct {
	RootCtx context.Context

	NVMLInstance         nvidianvml.InstanceV2
	NVIDIAToolOverwrites nvidiacommon.ToolOverwrites

	EventStore       eventstore.Store
	RebootEventStore pkghost.RebootEventStore
}

// InitFunc is the function that initializes a component.
type InitFunc func(GPUdInstance) (Component, error)

// Registry is the interface for the registry of components.
type Registry interface {
	// MustRegister registers a component with the given name and initialization function.
	// It panics if the component is already registered.
	// It panics if the initialization function returns an error.
	MustRegister(name string, initFunc InitFunc)

	// All returns all registered components.
	All() []Component
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
func (r *registry) MustRegister(name string, initFunc InitFunc) {
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
func (r *registry) registerInit(name string, initFunc InitFunc) error {
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

// All returns all registered components.
func (r *registry) All() []Component {
	all := r.listAll()
	sort.Slice(all, func(i, j int) bool {
		return all[i].Name() < all[j].Name()
	})
	return all
}

// listAll returns all registered components.
func (r *registry) listAll() []Component {
	r.mu.RLock()
	defer r.mu.RUnlock()

	all := make([]Component, 0, len(r.components))
	for _, c := range r.components {
		all = append(all, c)
	}
	return all
}
