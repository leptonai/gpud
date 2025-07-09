package components

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"sync"

	nvidiacommon "github.com/leptonai/gpud/pkg/config/common"
	"github.com/leptonai/gpud/pkg/eventstore"
	pkghost "github.com/leptonai/gpud/pkg/host"
	nvidianvml "github.com/leptonai/gpud/pkg/nvidia-query/nvml"
)

var (
	// ErrAlreadyRegistered is the error returned when a component is already registered.
	ErrAlreadyRegistered = errors.New("component already registered")
)

// GPUdInstance is the instance of the GPUd dependencies.
type GPUdInstance struct {
	RootCtx context.Context

	// MachineID is either the machine ID assigned from the control plane
	// or the unique UUID of the machine.
	// For example, it is used to identify itself for the NFS checker.
	MachineID string

	KernelModulesToCheck []string

	NVMLInstance         nvidianvml.Instance
	NVIDIAToolOverwrites nvidiacommon.ToolOverwrites

	DBRW *sql.DB
	DBRO *sql.DB

	EventStore       eventstore.Store
	RebootEventStore pkghost.RebootEventStore

	MountPoints  []string
	MountTargets []string
}

// InitFunc is the function that initializes a component.
type InitFunc func(*GPUdInstance) (Component, error)

// Registry is the interface for the registry of components.
type Registry interface {
	// MustRegister registers a component with the given initialization function.
	// It panics if the component is already registered.
	// It panics if the initialization function returns an error.
	MustRegister(initFunc InitFunc)

	// Register registers a component with the given initialization function.
	// It returns an error if the component is already registered.
	// It returns an error if the initialization function returns an error.
	Register(initFunc InitFunc) (Component, error)

	// All returns all registered components.
	All() []Component

	// Get returns a component by name.
	// It returns nil if the component is not registered.
	Get(name string) Component

	// Deregister deregisters a component by name, and returns the
	// underlying component if it is registered.
	// It returns nil if the component is not registered.
	// Meaning, it is safe to call it multiple times,
	// and it is also safe to call it with a non-registered name.
	Deregister(name string) Component
}

var _ Registry = &registry{}

type registry struct {
	mu           sync.RWMutex
	gpudInstance *GPUdInstance
	components   map[string]Component
}

// NewRegistry creates a new registry.
func NewRegistry(gpudInstance *GPUdInstance) Registry {
	return &registry{
		gpudInstance: gpudInstance,
		components:   make(map[string]Component),
	}
}

// MustRegister registers a component with the given name and initialization function.
// It panics if the component is already registered.
// It panics if the initialization function returns an error.
func (r *registry) MustRegister(initFunc InitFunc) {
	if _, err := r.Register(initFunc); err != nil {
		panic(err)
	}
}

// registerInit registers an initialization function for a component with the given name.
func (r *registry) Register(initFunc InitFunc) (Component, error) {
	c, err := initFunc(r.gpudInstance)
	if err != nil {
		return nil, err
	}

	if r.hasRegistered(c.Name()) {
		return nil, fmt.Errorf("component %s already registered: %w", c.Name(), ErrAlreadyRegistered)
	}

	r.mu.Lock()
	r.components[c.Name()] = c
	r.mu.Unlock()

	return c, nil
}

// hasRegistered checks if a component with the given name is already registered.
func (r *registry) hasRegistered(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	_, ok := r.components[name]
	return ok
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

func (r *registry) Get(name string) Component {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.components[name]
}

func (r *registry) Deregister(name string) Component {
	r.mu.Lock()
	c := r.components[name]
	if c != nil {
		delete(r.components, name)
	}
	r.mu.Unlock()

	return c
}
