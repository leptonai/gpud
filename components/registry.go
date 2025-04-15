package components

import (
	"context"
	"sync"

	"github.com/leptonai/gpud/pkg/eventstore"
	pkghost "github.com/leptonai/gpud/pkg/host"
	nvidianvml "github.com/leptonai/gpud/pkg/nvidia-query/nvml"
)

type Registry interface {
	MustRegister(name string, initFunc func(GPUdInstance) (Component, error))
}

type GPUdInstance struct {
	RootCtx          context.Context
	NVMLInstance     nvidianvml.InstanceV2
	EventStore       eventstore.Store
	RebootEventStore pkghost.RebootEventStore
}

var _ Registry = &registry{}

type registry struct {
	mu           sync.RWMutex
	gpudInstance GPUdInstance
	components   map[string]Component
}

func NewRegistry(gpudInstance GPUdInstance) *registry {
	return &registry{
		gpudInstance: gpudInstance,
		components:   make(map[string]Component),
	}
}

func (r *registry) MustRegister(name string, initFunc func(GPUdInstance) (Component, error)) {
	c, err := initFunc(r.gpudInstance)
	if err != nil {
		panic(err)
	}
	r.mu.Lock()
	r.components[name] = c
	r.mu.Unlock()
}
