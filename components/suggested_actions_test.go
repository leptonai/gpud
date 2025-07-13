package components

import (
	"fmt"
	"sort"
	"testing"
	"time"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/stretchr/testify/assert"
)

func TestNewSuggestedActionsStore(t *testing.T) {
	store := NewSuggestedActionsStore()
	assert.NotNil(t, store)
}

func TestSuggestedActionsStore_Suggest(t *testing.T) {
	tests := []struct {
		name      string
		component string
		action    apiv1.RepairActionType
		ttl       time.Duration
	}{
		{
			name:      "basic suggestion",
			component: "gpu-monitor",
			action:    apiv1.RepairActionTypeRebootSystem,
			ttl:       time.Hour,
		},
		{
			name:      "suggestion with short TTL",
			component: "disk-monitor",
			action:    apiv1.RepairActionTypeHardwareInspection,
			ttl:       time.Minute,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := NewSuggestedActionsStore()

			// Test that Suggest doesn't panic
			assert.NotPanics(t, func() {
				store.Suggest(tt.component, tt.action, tt.ttl)
			})

			// Verify the suggestion was stored
			components := store.HasSuggested(tt.action)
			assert.Contains(t, components, tt.component)
		})
	}
}

func TestSuggestedActionsStore_HasSuggested(t *testing.T) {
	t.Run("non-existent action", func(t *testing.T) {
		store := NewSuggestedActionsStore()
		components := store.HasSuggested(apiv1.RepairActionTypeIgnoreNoActionRequired)
		assert.Empty(t, components)
	})

	t.Run("valid suggestions", func(t *testing.T) {
		store := NewSuggestedActionsStore()

		store.Suggest("component1", apiv1.RepairActionTypeRebootSystem, time.Hour)
		store.Suggest("component2", apiv1.RepairActionTypeRebootSystem, time.Hour)
		store.Suggest("component3", apiv1.RepairActionTypeHardwareInspection, time.Hour)

		restartComponents := store.HasSuggested(apiv1.RepairActionTypeRebootSystem)
		assert.Len(t, restartComponents, 2)
		assert.Contains(t, restartComponents, "component1")
		assert.Contains(t, restartComponents, "component2")

		cleanupComponents := store.HasSuggested(apiv1.RepairActionTypeHardwareInspection)
		assert.Len(t, cleanupComponents, 1)
		assert.Contains(t, cleanupComponents, "component3")
	})
}

func TestSuggestedActionsStore_TTLExpiration(t *testing.T) {
	mockTime := time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC)

	// Create store with mock time function
	store := &suggestedActionsStore{
		actions:    make(map[apiv1.RepairActionType]map[string]time.Time),
		getTimeNow: func() time.Time { return mockTime },
	}

	// Add suggestion with 1 hour TTL
	store.Suggest("component1", apiv1.RepairActionTypeRebootSystem, time.Hour)

	// Should be present immediately
	components := store.HasSuggested(apiv1.RepairActionTypeRebootSystem)
	assert.Contains(t, components, "component1")

	// Advance time by 30 minutes - should still be present
	store.getTimeNow = func() time.Time { return mockTime.Add(30 * time.Minute) }
	components = store.HasSuggested(apiv1.RepairActionTypeRebootSystem)
	assert.Contains(t, components, "component1")

	// Advance time by 2 hours - should be expired
	store.getTimeNow = func() time.Time { return mockTime.Add(2 * time.Hour) }
	components = store.HasSuggested(apiv1.RepairActionTypeRebootSystem)
	assert.Empty(t, components)
}

func TestSuggestedActionsStore_OverwriteSuggestion(t *testing.T) {
	mockTime := time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC)

	store := &suggestedActionsStore{
		actions:    make(map[apiv1.RepairActionType]map[string]time.Time),
		getTimeNow: func() time.Time { return mockTime },
	}

	// Add suggestion with 1 hour TTL
	store.Suggest("component1", apiv1.RepairActionTypeRebootSystem, time.Hour)

	// Advance time by 30 minutes and add same suggestion with 2 hour TTL
	store.getTimeNow = func() time.Time { return mockTime.Add(30 * time.Minute) }
	store.Suggest("component1", apiv1.RepairActionTypeRebootSystem, 2*time.Hour)

	// Advance time by 1.5 hours from original time (original would be expired)
	store.getTimeNow = func() time.Time { return mockTime.Add(90 * time.Minute) }
	components := store.HasSuggested(apiv1.RepairActionTypeRebootSystem)
	assert.Contains(t, components, "component1", "should still be present due to overwrite with longer TTL")

	// Advance time by 3 hours from original time (new suggestion should be expired)
	store.getTimeNow = func() time.Time { return mockTime.Add(3 * time.Hour) }
	components = store.HasSuggested(apiv1.RepairActionTypeRebootSystem)
	assert.Empty(t, components)
}

func TestSuggestedActionsStore_MultipleComponentsSameAction(t *testing.T) {
	mockTime := time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC)

	store := &suggestedActionsStore{
		actions:    make(map[apiv1.RepairActionType]map[string]time.Time),
		getTimeNow: func() time.Time { return mockTime },
	}

	// Add suggestions from different components for same action with different TTLs
	store.Suggest("component1", apiv1.RepairActionTypeRebootSystem, time.Hour)
	store.Suggest("component2", apiv1.RepairActionTypeRebootSystem, 2*time.Hour)
	store.Suggest("component3", apiv1.RepairActionTypeRebootSystem, 30*time.Minute)

	// Initially all should be present
	components := store.HasSuggested(apiv1.RepairActionTypeRebootSystem)
	assert.Len(t, components, 3)

	// Advance time by 45 minutes - component3 should be expired
	store.getTimeNow = func() time.Time { return mockTime.Add(45 * time.Minute) }
	components = store.HasSuggested(apiv1.RepairActionTypeRebootSystem)
	assert.Len(t, components, 2)
	assert.Contains(t, components, "component1")
	assert.Contains(t, components, "component2")
	assert.NotContains(t, components, "component3")

	// Advance time by 1.5 hours - only component2 should remain
	store.getTimeNow = func() time.Time { return mockTime.Add(90 * time.Minute) }
	components = store.HasSuggested(apiv1.RepairActionTypeRebootSystem)
	assert.Len(t, components, 1)
	assert.Contains(t, components, "component2")
}

func TestSuggestedActionsStore_ConcurrentAccess(t *testing.T) {
	store := NewSuggestedActionsStore()

	done := make(chan bool)

	// Start multiple goroutines that suggest actions
	for i := 0; i < 10; i++ {
		go func(id int) {
			for j := 0; j < 100; j++ {
				store.Suggest("component", apiv1.RepairActionTypeRebootSystem, time.Hour)
				store.HasSuggested(apiv1.RepairActionTypeRebootSystem)
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines to complete
	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify store is in consistent state
	components := store.HasSuggested(apiv1.RepairActionTypeRebootSystem)
	assert.Contains(t, components, "component")
}

func TestSuggestedActionsStore_ExpiredComponentRemoval(t *testing.T) {
	mockTime := time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC)

	store := &suggestedActionsStore{
		actions:    make(map[apiv1.RepairActionType]map[string]time.Time),
		getTimeNow: func() time.Time { return mockTime },
	}

	// Add suggestions with different TTLs
	store.Suggest("component1", apiv1.RepairActionTypeRebootSystem, time.Hour)
	store.Suggest("component2", apiv1.RepairActionTypeRebootSystem, 30*time.Minute)
	store.Suggest("component3", apiv1.RepairActionTypeHardwareInspection, 45*time.Minute)

	// Verify initial state
	restartComponents := store.HasSuggested(apiv1.RepairActionTypeRebootSystem)
	assert.Len(t, restartComponents, 2)
	cleanupComponents := store.HasSuggested(apiv1.RepairActionTypeHardwareInspection)
	assert.Len(t, cleanupComponents, 1)

	// Advance time by 40 minutes - component2 should be expired and removed
	store.getTimeNow = func() time.Time { return mockTime.Add(40 * time.Minute) }
	restartComponents = store.HasSuggested(apiv1.RepairActionTypeRebootSystem)
	assert.Len(t, restartComponents, 1)
	assert.Contains(t, restartComponents, "component1")
	assert.NotContains(t, restartComponents, "component2")

	// Cleanup should still be present
	cleanupComponents = store.HasSuggested(apiv1.RepairActionTypeHardwareInspection)
	assert.Len(t, cleanupComponents, 1)

	// Advance time by 50 minutes - cleanup should be expired and removed
	store.getTimeNow = func() time.Time { return mockTime.Add(50 * time.Minute) }
	cleanupComponents = store.HasSuggested(apiv1.RepairActionTypeHardwareInspection)
	assert.Empty(t, cleanupComponents)

	// Restart should still have component1
	restartComponents = store.HasSuggested(apiv1.RepairActionTypeRebootSystem)
	assert.Len(t, restartComponents, 1)
	assert.Contains(t, restartComponents, "component1")

	// Advance time by 70 minutes - all should be expired
	store.getTimeNow = func() time.Time { return mockTime.Add(70 * time.Minute) }
	restartComponents = store.HasSuggested(apiv1.RepairActionTypeRebootSystem)
	assert.Empty(t, restartComponents)
	cleanupComponents = store.HasSuggested(apiv1.RepairActionTypeHardwareInspection)
	assert.Empty(t, cleanupComponents)
}

func TestSuggestedActionsStore_ExpiredActionRemoval(t *testing.T) {
	mockTime := time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC)

	store := &suggestedActionsStore{
		actions:    make(map[apiv1.RepairActionType]map[string]time.Time),
		getTimeNow: func() time.Time { return mockTime },
	}

	// Add a single component suggestion that will expire
	store.Suggest("component1", apiv1.RepairActionTypeRebootSystem, 30*time.Minute)

	// Verify action exists in internal map
	_, exists := store.actions[apiv1.RepairActionTypeRebootSystem]
	assert.True(t, exists)

	// Advance time past expiration
	store.getTimeNow = func() time.Time { return mockTime.Add(time.Hour) }

	// Call HasSuggested which should clean up expired entries
	components := store.HasSuggested(apiv1.RepairActionTypeRebootSystem)
	assert.Empty(t, components)

	// Verify action is completely removed from internal map
	_, exists = store.actions[apiv1.RepairActionTypeRebootSystem]
	assert.False(t, exists, "expired action should be removed from internal map")
}

func TestSuggestedActionsStore_PartialExpiration(t *testing.T) {
	mockTime := time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC)

	store := &suggestedActionsStore{
		actions:    make(map[apiv1.RepairActionType]map[string]time.Time),
		getTimeNow: func() time.Time { return mockTime },
	}

	// Add multiple components for same action with different TTLs
	store.Suggest("shortLived", apiv1.RepairActionTypeRebootSystem, 30*time.Minute)
	store.Suggest("longLived1", apiv1.RepairActionTypeRebootSystem, 2*time.Hour)
	store.Suggest("longLived2", apiv1.RepairActionTypeRebootSystem, 3*time.Hour)

	// Advance time to expire only the short-lived component
	store.getTimeNow = func() time.Time { return mockTime.Add(45 * time.Minute) }

	components := store.HasSuggested(apiv1.RepairActionTypeRebootSystem)
	assert.Len(t, components, 2)
	assert.Contains(t, components, "longLived1")
	assert.Contains(t, components, "longLived2")
	assert.NotContains(t, components, "shortLived")

	// Verify action still exists but shortLived component is removed
	actionComponents, exists := store.actions[apiv1.RepairActionTypeRebootSystem]
	assert.True(t, exists)
	assert.Len(t, actionComponents, 2)
	_, shortLivedExists := actionComponents["shortLived"]
	assert.False(t, shortLivedExists, "expired component should be removed from internal map")
}

func TestSuggestedActionsStore_ConcurrentExpirationCleanup(t *testing.T) {
	mockTime := time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC)

	store := &suggestedActionsStore{
		actions:    make(map[apiv1.RepairActionType]map[string]time.Time),
		getTimeNow: func() time.Time { return mockTime },
	}

	// Add many suggestions
	for i := 0; i < 100; i++ {
		component := fmt.Sprintf("component%d", i)
		store.Suggest(component, apiv1.RepairActionTypeRebootSystem, time.Hour)
	}

	// Advance time to expire all suggestions
	store.getTimeNow = func() time.Time { return mockTime.Add(2 * time.Hour) }

	done := make(chan bool)

	// Start multiple goroutines calling HasSuggested concurrently
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 50; j++ {
				components := store.HasSuggested(apiv1.RepairActionTypeRebootSystem)
				assert.Empty(t, components)
			}
			done <- true
		}()
	}

	// Wait for all goroutines to complete
	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify final state is clean
	components := store.HasSuggested(apiv1.RepairActionTypeRebootSystem)
	assert.Empty(t, components)
	_, exists := store.actions[apiv1.RepairActionTypeRebootSystem]
	assert.False(t, exists, "action should be removed after all components expire")
}

func TestSuggestedActionsStore_SortedResults(t *testing.T) {
	store := NewSuggestedActionsStore()

	// Add components in non-alphabetical order
	store.Suggest("zebra", apiv1.RepairActionTypeRebootSystem, time.Hour)
	store.Suggest("alpha", apiv1.RepairActionTypeRebootSystem, time.Hour)
	store.Suggest("beta", apiv1.RepairActionTypeRebootSystem, time.Hour)
	store.Suggest("gamma", apiv1.RepairActionTypeRebootSystem, time.Hour)

	components := store.HasSuggested(apiv1.RepairActionTypeRebootSystem)

	// Should return all components
	assert.Len(t, components, 4)

	// Should be sorted alphabetically
	expectedOrder := []string{"alpha", "beta", "gamma", "zebra"}
	assert.Equal(t, expectedOrder, components, "components should be returned in sorted order")
}

func TestSuggestedActionsStore_SortedResultsWithExpiration(t *testing.T) {
	mockTime := time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC)

	store := &suggestedActionsStore{
		actions:    make(map[apiv1.RepairActionType]map[string]time.Time),
		getTimeNow: func() time.Time { return mockTime },
	}

	// Add components with different TTLs in non-alphabetical order
	store.Suggest("zebra", apiv1.RepairActionTypeRebootSystem, time.Hour)      // Will remain
	store.Suggest("alpha", apiv1.RepairActionTypeRebootSystem, 30*time.Minute) // Will expire
	store.Suggest("beta", apiv1.RepairActionTypeRebootSystem, time.Hour)       // Will remain
	store.Suggest("gamma", apiv1.RepairActionTypeRebootSystem, 45*time.Minute) // Will expire
	store.Suggest("delta", apiv1.RepairActionTypeRebootSystem, 2*time.Hour)    // Will remain

	// Advance time to expire some components
	store.getTimeNow = func() time.Time { return mockTime.Add(50 * time.Minute) }

	components := store.HasSuggested(apiv1.RepairActionTypeRebootSystem)

	// Should return only non-expired components
	assert.Len(t, components, 3)

	// Should be sorted alphabetically (beta, delta, zebra)
	expectedOrder := []string{"beta", "delta", "zebra"}
	assert.Equal(t, expectedOrder, components, "remaining components should be returned in sorted order")
}

func TestSuggestedActionsStore_SortedResultsLargeDataset(t *testing.T) {
	store := NewSuggestedActionsStore()

	// Add many components with random-ish names
	componentNames := []string{
		"component99", "component01", "component50", "component25",
		"zebra-component", "alpha-component", "beta-component",
		"aaaa", "zzzz", "component10", "component02", "component100",
	}

	for _, name := range componentNames {
		store.Suggest(name, apiv1.RepairActionTypeRebootSystem, time.Hour)
	}

	components := store.HasSuggested(apiv1.RepairActionTypeRebootSystem)

	// Should return all components
	assert.Len(t, components, len(componentNames))

	// Verify they are sorted
	for i := 1; i < len(components); i++ {
		assert.True(t, components[i-1] < components[i],
			"component %s should come before %s in sorted order",
			components[i-1], components[i])
	}

	// Verify the exact expected order
	expectedSorted := make([]string, len(componentNames))
	copy(expectedSorted, componentNames)
	sort.Strings(expectedSorted)
	assert.Equal(t, expectedSorted, components, "components should match expected sorted order")
}

func TestSuggestedActionsStore_HasSuggested_LineSpecificCoverage(t *testing.T) {
	t.Run("action does not exist", func(t *testing.T) {
		store := NewSuggestedActionsStore()

		// Test line 58: components, exists := s.actions[action]
		// Test lines 59-61: if !exists { return nil }
		result := store.HasSuggested(apiv1.RepairActionTypeRebootSystem)
		assert.Nil(t, result, "should return nil for non-existent action")
	})

	t.Run("expiration logic and component removal", func(t *testing.T) {
		mockTime := time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC)
		store := &suggestedActionsStore{
			actions:    make(map[apiv1.RepairActionType]map[string]time.Time),
			getTimeNow: func() time.Time { return mockTime },
		}

		// Add components with different expiration times
		store.Suggest("expired-component", apiv1.RepairActionTypeRebootSystem, 30*time.Minute)
		store.Suggest("valid-component-1", apiv1.RepairActionTypeRebootSystem, 2*time.Hour)
		store.Suggest("valid-component-2", apiv1.RepairActionTypeRebootSystem, 3*time.Hour)

		// Advance time to expire only the first component
		store.getTimeNow = func() time.Time { return mockTime.Add(45 * time.Minute) }

		// Test line 64: now := s.getTimeNow()
		// Test lines 66-71: iteration with expiration check and deletion
		result := store.HasSuggested(apiv1.RepairActionTypeRebootSystem)

		// Should return only the valid components
		assert.Len(t, result, 2)
		assert.Contains(t, result, "valid-component-1")
		assert.Contains(t, result, "valid-component-2")
		assert.NotContains(t, result, "expired-component")

		// Verify expired component was removed from internal map
		actionComponents := store.actions[apiv1.RepairActionTypeRebootSystem]
		_, expiredExists := actionComponents["expired-component"]
		assert.False(t, expiredExists, "expired component should be removed from internal map")
	})

	t.Run("complete action removal when all components expire", func(t *testing.T) {
		mockTime := time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC)
		store := &suggestedActionsStore{
			actions:    make(map[apiv1.RepairActionType]map[string]time.Time),
			getTimeNow: func() time.Time { return mockTime },
		}

		// Add components that will all expire
		store.Suggest("component-1", apiv1.RepairActionTypeRebootSystem, 30*time.Minute)
		store.Suggest("component-2", apiv1.RepairActionTypeRebootSystem, 45*time.Minute)

		// Verify action exists before expiration
		_, exists := store.actions[apiv1.RepairActionTypeRebootSystem]
		assert.True(t, exists, "action should exist before expiration")

		// Advance time to expire all components
		store.getTimeNow = func() time.Time { return mockTime.Add(time.Hour) }

		// Test lines 73-75: action removal when no components remain
		result := store.HasSuggested(apiv1.RepairActionTypeRebootSystem)
		assert.Empty(t, result, "should return empty slice when all components expire")

		// Verify action was completely removed from internal map
		_, exists = store.actions[apiv1.RepairActionTypeRebootSystem]
		assert.False(t, exists, "action should be removed when all components expire")
	})

	t.Run("sorting of valid components", func(t *testing.T) {
		store := NewSuggestedActionsStore()

		// Add components in reverse alphabetical order
		store.Suggest("zebra", apiv1.RepairActionTypeRebootSystem, time.Hour)
		store.Suggest("yankee", apiv1.RepairActionTypeRebootSystem, time.Hour)
		store.Suggest("alpha", apiv1.RepairActionTypeRebootSystem, time.Hour)
		store.Suggest("beta", apiv1.RepairActionTypeRebootSystem, time.Hour)

		// Test line 77: sort.Strings(validComponents)
		result := store.HasSuggested(apiv1.RepairActionTypeRebootSystem)

		// Should return components in sorted order
		expected := []string{"alpha", "beta", "yankee", "zebra"}
		assert.Equal(t, expected, result, "components should be sorted alphabetically")
	})

	t.Run("edge case: empty components map after partial expiration", func(t *testing.T) {
		mockTime := time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC)
		store := &suggestedActionsStore{
			actions:    make(map[apiv1.RepairActionType]map[string]time.Time),
			getTimeNow: func() time.Time { return mockTime },
		}

		// Add single component that expires
		store.Suggest("single-component", apiv1.RepairActionTypeRebootSystem, 30*time.Minute)

		// Advance time to expire the component
		store.getTimeNow = func() time.Time { return mockTime.Add(time.Hour) }

		// This should trigger the action removal logic
		result := store.HasSuggested(apiv1.RepairActionTypeRebootSystem)
		assert.Empty(t, result)

		// Verify the action key is removed from the map
		_, exists := store.actions[apiv1.RepairActionTypeRebootSystem]
		assert.False(t, exists, "action should be removed when last component expires")
	})

	t.Run("edge case: multiple calls after expiration", func(t *testing.T) {
		mockTime := time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC)
		store := &suggestedActionsStore{
			actions:    make(map[apiv1.RepairActionType]map[string]time.Time),
			getTimeNow: func() time.Time { return mockTime },
		}

		// Add component that expires
		store.Suggest("test-component", apiv1.RepairActionTypeRebootSystem, 30*time.Minute)

		// Advance time to expire
		store.getTimeNow = func() time.Time { return mockTime.Add(time.Hour) }

		// First call should clean up
		result1 := store.HasSuggested(apiv1.RepairActionTypeRebootSystem)
		assert.Empty(t, result1)

		// Second call should handle non-existent action gracefully
		result2 := store.HasSuggested(apiv1.RepairActionTypeRebootSystem)
		assert.Empty(t, result2)
	})
}
