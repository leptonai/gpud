// Package components defines the common interfaces for the components.
package components

import (
	"context"
	"fmt"
	"sync"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/leptonai/gpud/pkg/common"
	"github.com/leptonai/gpud/pkg/errdefs"
)

// Component represents an individual component of the system.
//
// Each component check is independent of each other.
// But the underlying implementation may share the same data sources
// in order to minimize the querying overhead (e.g., nvidia-smi calls).
//
// Each component implements its own output format inside the State struct.
// And recommended to have a consistent name for its HTTP handler.
// And recommended to define const keys for the State extra information field.
type Component interface {
	// Defines the component name,
	// and used for the HTTP handler registration path.
	// Must be globally unique.
	Name() string

	// Start called upon server start.
	// Implements component-specific poller start logic.
	Start() error

	// Returns the current states of the component.
	States(ctx context.Context) ([]State, error)

	// Returns all the events from "since".
	Events(ctx context.Context, since time.Time) ([]Event, error)

	// Called upon server close.
	// Implements copmonent-specific poller cleanup logic.
	Close() error
}

type State struct {
	Name      string            `json:"name,omitempty"`
	Healthy   bool              `json:"healthy,omitempty"`
	Health    string            `json:"health,omitempty"`     // Healthy, Degraded, Unhealthy
	Reason    string            `json:"reason,omitempty"`     // a detailed and processed reason on why the component is not healthy
	Error     string            `json:"error,omitempty"`      // the unprocessed error returned from the component
	ExtraInfo map[string]string `json:"extra_info,omitempty"` // any extra information the component may want to expose

	SuggestedActions *common.SuggestedActions `json:"suggested_actions,omitempty"`
}

const (
	StateHealthy      = "Healthy"
	StateUnhealthy    = "Unhealthy"
	StateInitializing = "Initializing"
	StateDegraded     = "Degraded"
)

type Event struct {
	Time             metav1.Time              `json:"time"`
	Name             string                   `json:"name,omitempty"`
	Type             common.EventType         `json:"type,omitempty"`
	Message          string                   `json:"message,omitempty"`    // detailed message of the event
	ExtraInfo        map[string]string        `json:"extra_info,omitempty"` // any extra information the component may want to expose
	SuggestedActions *common.SuggestedActions `json:"suggested_actions,omitempty"`
}

type Metric struct {
	UnixSeconds         int64   `json:"unix_seconds"`
	MetricName          string  `json:"metric_name"`
	MetricSecondaryName string  `json:"metric_secondary_name,omitempty"`
	Value               float64 `json:"value"`
}

type Metrics []Metric

type Info struct {
	States  []State `json:"states"`
	Events  []Event `json:"events"`
	Metrics Metrics `json:"metrics"`
}

var (
	defaultSetMu sync.RWMutex
	defaultSet   = make(map[string]Component)
)

func IsComponentRegistered(name string) bool {
	defaultSetMu.RLock()
	defer defaultSetMu.RUnlock()

	_, ok := defaultSet[name]
	return ok
}

func RegisterComponent(name string, comp Component) error {
	defaultSetMu.Lock()
	defer defaultSetMu.Unlock()

	if defaultSet == nil {
		return fmt.Errorf("component set not initialized: %w", errdefs.ErrUnavailable)
	}
	if _, ok := defaultSet[name]; ok {
		return fmt.Errorf("component %s already registered: %w", name, errdefs.ErrAlreadyExists)
	}
	defaultSet[name] = comp
	return nil
}

func GetComponent(name string) (Component, error) {
	defaultSetMu.RLock()
	defer defaultSetMu.RUnlock()

	return getComponent(defaultSet, name)
}

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

func GetAllComponents() map[string]Component {
	defaultSetMu.RLock()
	defer defaultSetMu.RUnlock()
	return defaultSet
}
