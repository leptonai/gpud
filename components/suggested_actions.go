package components

import (
	"sort"
	"sync"
	"time"

	apiv1 "github.com/leptonai/gpud/api/v1"
)

type SuggestedActionsStore interface {
	// Suggest suggests an action for a component with a TTL.
	// After TTL has elapsed, the action will be removed from the store.
	Suggest(component string, action apiv1.RepairActionType, ttl time.Duration)

	// HasSuggested returns the components that have suggested the action
	// that still have valid TTL for those actions.
	// If the specified action has not suggested, it returns nil.
	HasSuggested(action apiv1.RepairActionType) []string
}

var _ SuggestedActionsStore = &suggestedActionsStore{}

type suggestedActionsStore struct {
	mu sync.RWMutex

	// actions maps action -> component -> expiration time
	actions map[apiv1.RepairActionType]map[string]time.Time

	getTimeNow func() time.Time
}

func NewSuggestedActionsStore() SuggestedActionsStore {
	return &suggestedActionsStore{
		actions: make(map[apiv1.RepairActionType]map[string]time.Time),
		getTimeNow: func() time.Time {
			return time.Now().UTC()
		},
	}
}

func (s *suggestedActionsStore) Suggest(component string, action apiv1.RepairActionType, ttl time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()

	expirationTime := s.getTimeNow().Add(ttl)

	if s.actions[action] == nil {
		s.actions[action] = make(map[string]time.Time)
	}
	s.actions[action][component] = expirationTime
}

func (s *suggestedActionsStore) HasSuggested(action apiv1.RepairActionType) []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	components, exists := s.actions[action]
	if !exists {
		return nil
	}

	now := s.getTimeNow()
	var validComponents []string
	for component, expiresAt := range components {
		if now.After(expiresAt) {
			delete(components, component)
			continue
		}
		validComponents = append(validComponents, component)
	}

	// If no components remain for this action, remove the action entirely
	if len(components) == 0 {
		delete(s.actions, action)
	}

	sort.Strings(validComponents)
	return validComponents
}
