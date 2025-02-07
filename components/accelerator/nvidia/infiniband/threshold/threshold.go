// Package threshold contains the threshold for the infiniband component.
package threshold

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/components/common"
	events_store "github.com/leptonai/gpud/components/db"
)

// Defines the minimum number of ports and the expected rate in Gb/sec.
type Threshold struct {
	// The minimum number of ports.
	// If not set, it defaults to the number of GPUs.
	AtLeastPorts int `json:"at_least_ports"`

	// The expected rate in Gb/sec.
	// If not set, it defaults to 200.
	AtLeastRate int `json:"at_least_rate"`
}

func (th Threshold) JSON() ([]byte, error) {
	return json.Marshal(th)
}

func Parse(b []byte) (*Threshold, error) {
	var th Threshold
	err := json.Unmarshal(b, &th)
	if err != nil {
		return nil, err
	}
	return &th, nil
}

func (th Threshold) Event(time time.Time) (components.Event, error) {
	b, err := th.JSON()
	if err != nil {
		return components.Event{}, err
	}
	return components.Event{
		Time:             metav1.NewTime(time),
		Name:             "infiniband_threshold",
		Type:             common.EventTypeInfo,
		Message:          fmt.Sprintf("infiniband threshold update: %d ports, %d Gb/sec", th.AtLeastPorts, th.AtLeastRate),
		ExtraInfo:        map[string]string{"data": string(b)},
		SuggestedActions: nil,
	}, nil
}

type Store struct {
	mu          sync.RWMutex
	eventsStore events_store.Store
}

func NewStore(eventsStore events_store.Store) *Store {
	return &Store{
		eventsStore: eventsStore,
	}
}

// Returns the latest threshold.
// Returns nil if no threshold is set.
func (s *Store) Latest(ctx context.Context) (*Threshold, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ev, err := s.eventsStore.Latest(ctx)
	if err != nil {
		return nil, err
	}
	if ev == nil {
		return nil, nil
	}
	return Parse([]byte(ev.ExtraInfo["data"]))
}

// Updates the current threshold, if and only if the current threshold is not found
// or the new threshold is different from the current threshold.
func (s *Store) CompareAndSet(ctx context.Context, threshold Threshold) error {
	latest, err := s.Latest(ctx)
	if err != nil {
		return err
	}

	// the latest threshold is already set the same as the new threshold
	if latest != nil && latest.AtLeastPorts == threshold.AtLeastPorts && latest.AtLeastRate == threshold.AtLeastRate {
		return nil
	}

	ev, err := threshold.Event(time.Now().UTC())
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	return s.eventsStore.Insert(ctx, ev)
}

var (
	defaultStoreMu sync.RWMutex
	defaultStore   *Store
)

func GetDefaultStore() *Store {
	defaultStoreMu.RLock()
	defer defaultStoreMu.RUnlock()
	return defaultStore
}

func SetDefaultStore(store *Store) {
	defaultStoreMu.Lock()
	defer defaultStoreMu.Unlock()
	defaultStore = store
}
